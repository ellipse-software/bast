package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/vault"
)

func vaultDAVServer(t *testing.T) *vault.WebDAVConfig {
	t.Helper()
	var mu sync.Mutex
	files := make(map[string][]byte)
	revisions := make(map[string]string)
	counter := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		user, password, ok := r.BasicAuth()
		if !ok || user != "alice" || password != "app-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, exists := files[r.URL.Path]
		switch r.Method {
		case http.MethodGet:
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", revisions[r.URL.Path])
			w.Write(body)
		case http.MethodPut:
			if (r.Header.Get("If-None-Match") == "*" && exists) ||
				(r.Header.Get("If-Match") != "" && r.Header.Get("If-Match") != revisions[r.URL.Path]) {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			files[r.URL.Path], _ = io.ReadAll(r.Body)
			counter++
			revisions[r.URL.Path] = fmt.Sprintf(`"r%d"`, counter)
			w.Header().Set("ETag", revisions[r.URL.Path])
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			delete(files, r.URL.Path)
			delete(revisions, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	return &vault.WebDAVConfig{URL: server.URL + "/", Username: "alice", Password: "app-password"}
}

func webDAVRunner(t *testing.T) (*Runner, *bytes.Buffer) {
	t.Helper()
	var out, stderr bytes.Buffer
	p := paths.ForHome(t.TempDir())
	r, err := New(p, openssh.Default(), strings.NewReader(""), &out, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	r.store, err = metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	return r, &out
}

func testWebDAVLifecycle(t *testing.T, cfg *vault.WebDAVConfig) {
	t.Helper()
	t.Setenv("BAST_NO_TELEMETRY", "1")
	t.Setenv("BAST_VAULT_API", "https://unused-api.example.invalid")
	a, _ := webDAVRunner(t)
	b, outputB := webDAVRunner(t)
	client := vault.ClientForSession(vault.Session{Backend: "webdav", WebDAV: cfg})
	if err := client.CheckWebDAV(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.hostAdd([]string{"Alpha", "--hostname", "alpha.example"}); err != nil {
		t.Fatal(err)
	}
	session := vault.Session{Backend: "webdav", WebDAV: cfg}
	if err := a.vaultLink(session, "encryption-secret", "merge"); err != nil {
		t.Fatal(err)
	}
	if err := b.vaultLink(session, "encryption-secret", "merge"); err != nil {
		t.Fatal(err)
	}
	managed, err := os.ReadFile(b.Paths.ManagedConfig)
	if err != nil || !bytes.Contains(managed, []byte("alpha.example")) {
		t.Fatalf("second device did not receive host: %s, %v", managed, err)
	}
	if err := b.hostAdd([]string{"Beta", "--hostname", "beta.example"}); err != nil {
		t.Fatal(err)
	}
	if err := b.vaultPush(nil); err != nil {
		t.Fatal(err)
	}
	if err := a.vaultPull(nil); err != nil {
		t.Fatal(err)
	}
	managed, err = os.ReadFile(a.Paths.ManagedConfig)
	if err != nil || !bytes.Contains(managed, []byte("beta.example")) || !bytes.Contains(managed, []byte("alpha.example")) {
		t.Fatalf("first device did not merge both hosts: %s, %v", managed, err)
	}
	remote, err := client.GetVault(context.Background(), "")
	if err != nil || bytes.Contains(remote.Ciphertext, []byte("alpha.example")) || bytes.Contains(remote.Ciphertext, []byte(cfg.Password)) {
		t.Fatalf("remote ciphertext or transport failed: %v", err)
	}
	outputB.Reset()
	b.JSON = true
	if err := b.vaultStatus(nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outputB.String(), `"backend":"webdav"`) || strings.Contains(outputB.String(), cfg.Password) {
		t.Fatal("status omitted backend or exposed credentials")
	}
	if err := b.vaultLogout(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(vault.SessionPath(b.Paths.StateFile)); !os.IsNotExist(err) {
		t.Fatalf("logout kept session: %v", err)
	}
	if _, err := os.Stat(vault.PassphrasePath(b.Paths.StateFile)); !os.IsNotExist(err) {
		t.Fatalf("logout kept passphrase: %v", err)
	}
	if _, err := client.GetVault(context.Background(), ""); err != nil {
		t.Fatalf("logout damaged shared vault: %v", err)
	}
	// A remote deletion must not silently recreate a vault on the next push.
	request, err := http.NewRequest(http.MethodDelete, strings.TrimRight(cfg.URL, "/")+"/bast-vault.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth(cfg.Username, cfg.Password)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		t.Fatalf("test cleanup returned HTTP %d", response.StatusCode)
	}
	if err := a.vaultPush(nil); err == nil || !strings.Contains(err.Error(), "remote vault missing") {
		t.Fatalf("push recreated deleted remote: %v", err)
	}
	// Credentials live beside state, never in the metadata or encrypted document.
	state, err := os.ReadFile(filepath.Clean(a.Paths.StateFile))
	if err != nil || bytes.Contains(state, []byte(cfg.Password)) {
		t.Fatalf("credentials leaked into metadata: %v", err)
	}
}

func TestVaultWebDAVLifecycle(t *testing.T) {
	testWebDAVLifecycle(t, vaultDAVServer(t))
}

// Run only against a dedicated empty directory; this test creates a shared vault.
func TestVaultWebDAVLiveLifecycle(t *testing.T) {
	directory := os.Getenv("BAST_WEBDAV_TEST_URL")
	if directory == "" {
		t.Skip("set BAST_WEBDAV_TEST_URL, BAST_WEBDAV_TEST_USERNAME and BAST_WEBDAV_TEST_PASSWORD for a dedicated WebDAV test directory")
	}
	testWebDAVLifecycle(t, &vault.WebDAVConfig{
		URL: directory, Username: os.Getenv("BAST_WEBDAV_TEST_USERNAME"), Password: os.Getenv("BAST_WEBDAV_TEST_PASSWORD"),
	})
}

func TestVaultWebDAVFlagsAndHelp(t *testing.T) {
	r, _ := webDAVRunner(t)
	for _, args := range [][]string{
		{"--webdav", "https://dav.example.com", "--email", "alice@example.com"},
		{"--webdav", "https://dav.example.com", "--api", "https://api.example.com"},
		{"--webdav", "https://dav.example.com", "--accept-terms"},
		{"--username", "alice"},
	} {
		if err := r.vaultLogin(args); err == nil {
			t.Fatalf("invalid flags accepted: %v", args)
		}
	}
	if usage := commandUsage("vault", "login"); !strings.Contains(usage, "--webdav") {
		t.Fatalf("help missing WebDAV: %s", usage)
	}
}
