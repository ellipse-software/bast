package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type davFile struct {
	body []byte
	etag string
}

type davFixture struct {
	mu                sync.Mutex
	files             map[string]davFile
	next              int
	ignoreConditions  bool
	omitPutETag       bool
	overwriteAfterPut bool
	getETag           string
	weakGets          int
}

func newDAVFixture(t *testing.T) (*Client, *davFixture) {
	t.Helper()
	f := &davFixture{files: make(map[string]davFile)}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return &Client{WebDAV: &WebDAVConfig{URL: server.URL + "/vault/", Username: "alice", Password: "app-secret"}}, f
}

func (f *davFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user, password, ok := r.BasicAuth()
	if !ok || user != "alice" || password != "app-secret" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/vault/") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Header.Get("Accept-Encoding") != "identity" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	file, exists := f.files[r.URL.Path]
	switch r.Method {
	case http.MethodGet:
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("If-None-Match") == file.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		etag := file.etag
		if f.getETag != "" {
			etag = f.getETag
		}
		if f.weakGets > 0 {
			etag = "W/" + etag
			f.weakGets--
		}
		w.Header().Set("ETag", etag)
		w.Write(file.body)
	case http.MethodPut:
		if !f.ignoreConditions {
			if (r.Header.Get("If-None-Match") == "*" && exists) ||
				(r.Header.Get("If-Match") != "" && (!exists || r.Header.Get("If-Match") != file.etag)) {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
		body, _ := io.ReadAll(r.Body)
		f.next++
		file = davFile{body: body, etag: fmt.Sprintf(`"rev-%d"`, f.next)}
		f.files[r.URL.Path] = file
		if !f.omitPutETag {
			w.Header().Set("ETag", file.etag)
		}
		if f.overwriteAfterPut {
			f.files[r.URL.Path] = davFile{body: []byte("another writer"), etag: `"another-revision"`}
		}
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		delete(f.files, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func TestWebDAVEncryptedRoundTripAndConcurrentUpdates(t *testing.T) {
	c, _ := newDAVFixture(t)
	ctx := context.Background()
	if err := c.CheckWebDAV(ctx); err != nil {
		t.Fatal(err)
	}
	blob, err := Encrypt(Document{Version: DocumentVersion, Hosts: []HostEntry{{ManagedID: "a", Alias: "secret-host"}}}, "encryption-secret")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte("secret-host")) {
		t.Fatal("plaintext leaked into the remote blob")
	}
	meta, err := c.PutVault(ctx, blob, "")
	if err != nil {
		t.Fatal(err)
	}
	remote, err := c.GetVault(ctx, "")
	if err != nil || !bytes.Equal(remote.Ciphertext, blob) || remote.Meta.Revision != meta.Revision {
		t.Fatalf("GET = %+v, %v", remote.Meta, err)
	}
	doc, err := Decrypt(remote.Ciphertext, "encryption-secret")
	if err != nil || doc.Hosts[0].Alias != "secret-host" {
		t.Fatalf("decrypted doc = %+v, %v", doc, err)
	}
	unchanged, err := c.GetVault(ctx, meta.Revision)
	if err != nil || !unchanged.NotModified {
		t.Fatalf("conditional GET = %+v, %v", unchanged, err)
	}
	if _, err := c.PutVault(ctx, blob, ""); !errors.Is(err, ErrRemoteUpdated) {
		t.Fatalf("duplicate creation = %v", err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := c.PutVault(ctx, blob, meta.Revision)
			results <- err
		}()
	}
	var succeeded, conflicted int
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrRemoteUpdated):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent updates: succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}

func TestWebDAVCapabilityCheckRejectsUnsafeServerAndCleansProbe(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		t.Run(fmt.Sprint(ignore), func(t *testing.T) {
			c, f := newDAVFixture(t)
			f.ignoreConditions = ignore
			original := davFile{body: []byte("existing vault"), etag: `"existing"`}
			f.files["/vault/"+webDAVFilename] = original
			err := c.CheckWebDAV(context.Background())
			if (err != nil) != ignore {
				t.Fatalf("capability check = %v", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.files) != 1 || !bytes.Equal(f.files["/vault/"+webDAVFilename].body, original.body) {
				t.Fatalf("probe leaked or existing vault changed: %+v", f.files)
			}
		})
	}
}

func TestWebDAVPutWithoutETagBindsRevisionToUploadedBytes(t *testing.T) {
	for _, overwritten := range []bool{false, true} {
		t.Run(fmt.Sprint(overwritten), func(t *testing.T) {
			c, f := newDAVFixture(t)
			f.omitPutETag = true
			f.overwriteAfterPut = overwritten
			meta, err := c.PutVault(context.Background(), []byte(`{"ciphertext":"encrypted"}`), "")
			if overwritten {
				if !errors.Is(err, ErrRemoteUpdated) {
					t.Fatalf("overwrite race = %v", err)
				}
			} else if err != nil || !strongWebDAVETag(meta.Revision) {
				t.Fatalf("PUT = %+v, %v", meta, err)
			}
		})
	}
}

func TestWebDAVRejectsWeakOrMissingETagAndOversize(t *testing.T) {
	for _, etag := range []string{`W/"weak"`, "", `"strong"`} {
		t.Run(etag, func(t *testing.T) {
			c, f := newDAVFixture(t)
			f.files["/vault/"+webDAVFilename] = davFile{body: []byte("encrypted"), etag: etag}
			_, err := c.GetVault(context.Background(), "")
			if (err == nil) != (etag == `"strong"`) {
				t.Fatalf("ETag %q: %v", etag, err)
			}
		})
	}
	c, f := newDAVFixture(t)
	f.files["/vault/"+webDAVFilename] = davFile{body: make([]byte, MaxVaultBytes+1), etag: `"large"`}
	if _, err := c.GetVault(context.Background(), ""); err == nil {
		t.Fatal("oversize GET accepted")
	}
	if _, err := c.PutVault(context.Background(), make([]byte, MaxVaultBytes+1), ""); err == nil {
		t.Fatal("oversize PUT accepted")
	}
	if _, err := c.PutVault(context.Background(), []byte("encrypted"), `W/"weak"`); err == nil {
		t.Fatal("weak If-Match accepted")
	}
}

func TestWebDAVWaitsForStrongETagAndPreservesEscapedDirectory(t *testing.T) {
	c, f := newDAVFixture(t)
	c.WebDAV.URL += "space%20dir"
	f.weakGets = 1
	f.files["/vault/space dir/"+webDAVFilename] = davFile{body: []byte("encrypted"), etag: `"opaque\tag"`}
	remote, err := c.GetVault(context.Background(), "")
	if err != nil || remote.Meta.Revision != `"opaque\tag"` {
		t.Fatalf("transient weak ETag or escaped directory: %+v, %v", remote.Meta, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetVault(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("request ignored cancellation: %v", err)
	}
}

func TestWebDAVDoesNotFollowRedirectsOrAcceptBadCredentials(t *testing.T) {
	var destinationCalls int
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c := &Client{WebDAV: &WebDAVConfig{URL: redirect.URL, Username: "alice", Password: "app-secret"}}
	if _, err := c.GetVault(context.Background(), ""); err == nil || destinationCalls != 0 {
		t.Fatalf("redirect err=%v destination calls=%d", err, destinationCalls)
	}
	c, _ = newDAVFixture(t)
	c.WebDAV.Password = "wrong"
	if _, err := c.GetVault(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad credentials = %v", err)
	}
}

func TestWebDAVConfigAndSessionIsolation(t *testing.T) {
	for _, invalid := range []string{"", "/relative", "ftp://example.com/", "http://example.com/", "https://user:secret@example.com/", "https://example.com/?token=secret", "https://example.com/#fragment"} {
		cfg := WebDAVConfig{URL: invalid, Username: "alice", Password: "secret"}
		if cfg.Validate() == nil {
			t.Errorf("accepted URL %q", invalid)
		}
	}
	cfg := &WebDAVConfig{URL: "https://dav.example.com/space%20dir/", Username: "alice", Password: "app-secret"}
	t.Setenv("BAST_VAULT_API", "https://api-override.example.com")
	session := Session{Backend: "webdav", WebDAV: cfg, Revision: `"opaque\etag"`}
	path := filepath.Join(t.TempDir(), "vault-session.json")
	if err := SaveSession(path, session); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession(path)
	if err != nil || !loaded.Linked() || loaded.WebDAV.Password != "app-secret" {
		t.Fatalf("loaded session = %+v, %v", loaded, err)
	}
	client := ClientForSession(loaded)
	if client.WebDAV == nil || client.WebDAV.URL != cfg.URL {
		t.Fatal("API override changed WebDAV transport")
	}
	if err := client.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !(Session{Token: "legacy-token"}).Linked() {
		t.Fatal("legacy API session is no longer linked")
	}
	if err := os.WriteFile(path, []byte(`{"backend":"unknown","token":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSession(path); err == nil {
		t.Fatal("unknown backend fell back to the hosted API")
	}
}
