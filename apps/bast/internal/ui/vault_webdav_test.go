package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/keys"
	"bast/internal/sshconfig"
	"bast/internal/vault"
)

func TestVaultWebDAVEntryAndSecretFields(t *testing.T) {
	m := testApp(t)
	found := false
	for _, item := range m.vaultMenuItems() {
		if item.action == "vault_webdav" {
			found = true
		}
	}
	if !found {
		t.Fatal("unlinked vault has no WebDAV action")
	}
	m.runVaultAction("vault_webdav")
	if m.form == nil || m.form.action != "vault_webdav" {
		t.Fatal("WebDAV action did not open setup form")
	}
	for _, label := range []string{"Password", "Passphrase", "Confirm"} {
		if f := m.form.fieldByLabel(label); f == nil || !f.secret {
			t.Fatalf("%s is not a secret field", label)
		}
	}
	m.form.fieldByLabel("Directory URL").value = "https://dav.example.com/bast/"
	m.form.fieldByLabel("Username").value = "alice"
	m.form.fieldByLabel("Password").value = "app-password"
	m.form.fieldByLabel("Passphrase").value = "encryption-secret"
	m.form.fieldByLabel("Confirm").value = "different"
	if _, _, err := m.webDAVSubmission(); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("mismatched passphrases accepted: %v", err)
	}
	if _, err := os.Stat(m.vaultSessionPath()); !os.IsNotExist(err) {
		t.Fatal("invalid form persisted a session")
	}
}

func TestVaultWebDAVSessionCacheAndLogout(t *testing.T) {
	m := testApp(t)
	session := vault.Session{Backend: "webdav", WebDAV: &vault.WebDAVConfig{
		URL: "https://dav.example.com/bast/", Username: "alice", Password: "app-password",
	}}
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		t.Fatal(err)
	}
	m.refreshVaultSessionCache()
	if !m.vaultLinked() {
		t.Fatal("WebDAV session without API token is not linked")
	}
	body := m.renderVaultStatus(m.styles())
	if !strings.Contains(body, "WebDAV") || !strings.Contains(body, "dav.example.com") || strings.Contains(body, "app-password") {
		t.Fatalf("unsafe or missing WebDAV status: %s", body)
	}
	for _, item := range m.vaultMenuItems() {
		if item.action == "vault_api_base" || item.action == "vault_webdav" {
			t.Fatalf("linked WebDAV exposes backend switch: %+v", item)
		}
	}
	m.runVaultAction("vault_logout")
	if m.vaultLinked() {
		t.Fatal("logout retained linked WebDAV state")
	}
	if _, err := os.Stat(m.vaultSessionPath()); !os.IsNotExist(err) {
		t.Fatal("logout retained WebDAV credentials")
	}
}

func TestVaultWebDAVLogoutKeepsAPIPreference(t *testing.T) {
	m := testApp(t)
	session := vault.Session{
		Backend: "webdav",
		APIBase: "https://self.example",
		WebDAV: &vault.WebDAVConfig{
			URL: "https://dav.example.com/bast/", Username: "alice", Password: "app-password",
		},
	}
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		t.Fatal(err)
	}
	m.refreshVaultSessionCache()
	m.runVaultAction("vault_logout")
	loaded, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.APIBase != "https://self.example" || loaded.WebDAV != nil || loaded.Token != "" || loaded.Backend != "" {
		t.Fatalf("logout session = %+v", loaded)
	}
	if m.vaultLinked() {
		t.Fatal("logout retained linked WebDAV state")
	}
}

func TestVaultWebDAVCancelDoesNotSaveSession(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		if r.Method == http.MethodPut {
			select {
			case <-started:
			default:
				close(started)
			}
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(func() {
		unblock()
		server.Close()
	})

	m := testApp(t)
	m.beginVaultBusy("Checking and linking WebDAV…")
	session := vault.Session{Backend: "webdav", WebDAV: &vault.WebDAVConfig{
		URL: server.URL + "/", Username: "alice", Password: "app-password",
	}}
	cmd := m.linkVaultWebDAV(session, "encryption-secret")
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("WebDAV check did not start")
	}
	m.cancelVaultOp()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled WebDAV link did not return")
	}
	if _, err := os.Stat(m.vaultSessionPath()); !os.IsNotExist(err) {
		t.Fatal("cancelled link saved a session")
	}
	pull, ok := msg.(vaultPullMsg)
	if !ok || pull.opGen == 0 || pull.opGen == m.vaultOpGen {
		t.Fatalf("cancelled link was applied: %#v gen=%d", msg, m.vaultOpGen)
	}
}

func TestVaultWebDAVLinkKeepsAPIPreference(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "alice" || password != "app-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			http.NotFound(w, r)
		case http.MethodPut:
			w.Header().Set("ETag", `"rev-1"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	m := testApp(t)
	m.config = sshconfig.Manager{Home: m.paths.Home, MainConfig: m.paths.MainConfig, ManagedDir: m.paths.ManagedDir, ManagedConfig: m.paths.ManagedConfig, ManagedKeys: m.paths.ManagedKeys}
	m.keyring = keys.Manager{Paths: m.paths}
	if err := vault.SaveSession(m.vaultSessionPath(), vault.Session{APIBase: "https://self.example"}); err != nil {
		t.Fatal(err)
	}
	session := vault.Session{Backend: "webdav", WebDAV: &vault.WebDAVConfig{
		URL: server.URL + "/", Username: "alice", Password: "app-password",
	}}
	msg := m.vaultCompleteLink(session, "encryption-secret")
	pull, ok := msg.(vaultPullMsg)
	if !ok || pull.err != nil {
		t.Fatalf("link = %#v", msg)
	}
	loaded, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Backend != "webdav" || loaded.APIBase != "https://self.example" || loaded.WebDAV == nil || loaded.WebDAV.Password != "app-password" {
		t.Fatalf("session = %+v", loaded)
	}
	if strings.Contains(loaded.APIBase, "bast.sh") {
		t.Fatalf("WebDAV session stored the hosted API: %+v", loaded)
	}
}
