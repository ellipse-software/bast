package vercel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	tokenFile := filepath.Join(t.TempDir(), "vercel-token")
	if err := WriteTokenFile(tokenFile, "vercel_test_token"); err != nil {
		t.Fatal(err)
	}
	return &Client{
		BaseURL:   server.URL,
		TokenFile: tokenFile,
		TeamID:    "team_1",
		ProjectID: "prj_1",
		PollWait:  time.Millisecond,
		HTTP:      server.Client(),
	}
}

func TestListAndDiscover(t *testing.T) {
	pages := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vercel_test_token" {
			t.Errorf("missing bearer")
		}
		if r.URL.Query().Get("teamId") != "team_1" || r.URL.Query().Get("project") != "prj_1" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if r.URL.Path != "/v2/sandboxes" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		pages++
		if r.URL.Query().Get("cursor") == "" {
			next := "page2"
			raw := sandboxListResponse{
				Sandboxes: []Sandbox{
					{Name: "dev", Status: "running", Persistent: true, VCPUs: 2, Runtime: "node24", CurrentSessionID: "sbx_1"},
				},
			}
			raw.Pagination.Count = 1
			raw.Pagination.Next = &next
			_ = json.NewEncoder(w).Encode(raw)
			return
		}
		_ = json.NewEncoder(w).Encode(sandboxListResponse{
			Sandboxes: []Sandbox{
				{Name: "idle", Status: "stopped", Persistent: true, VCPUs: 1, CurrentSessionID: "sbx_2"},
				{Name: "dead", Status: "failed"},
			},
		})
	})
	discovery, err := client.Discover(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if pages < 2 {
		t.Fatalf("pages = %d", pages)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(discovery.Instances))
	}
	if discovery.Instances[0].SyncID != "prj_1/dev" || !discovery.Instances[0].Running {
		t.Fatalf("running = %+v", discovery.Instances[0])
	}
	if discovery.Instances[1].SyncID != "prj_1/idle" || discovery.Instances[1].Running {
		t.Fatalf("stopped = %+v", discovery.Instances[1])
	}
	if !HostLooksStopped(discovery.Instances[1].Tags) {
		t.Fatal("stopped should look stopped")
	}
	if AliasFor(discovery.Instances[0]) != "vercel_dev" {
		t.Fatalf("alias = %s", AliasFor(discovery.Instances[0]))
	}
	host := ToSyncHost(discovery.Instances[0], "")
	if host.HostName != StoppedHost || host.SyncSource != ProviderName || host.User != "" {
		t.Fatalf("sync host = %+v", host)
	}
}

func TestCreatePollsUntilReady(t *testing.T) {
	gets := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sandboxes":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["runtime"] != "node24" || body["persistent"] != true {
				t.Errorf("create body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(sandboxSessionResponse{
				Sandbox: Sandbox{Name: "worker", Status: "pending"},
				Session: Session{ID: "sbx_new", Status: "pending"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/sandboxes/worker":
			gets++
			status := "pending"
			if gets >= 2 {
				status = "running"
			}
			_ = json.NewEncoder(w).Encode(sandboxSessionResponse{
				Sandbox: Sandbox{Name: "worker", Status: status, Runtime: "node24", CurrentSessionID: "sbx_new"},
				Session: Session{ID: "sbx_new", Status: status},
			})
		default:
			http.NotFound(w, r)
		}
	})
	box, err := client.Create(context.Background(), CreateOpts{Name: "worker", VCPUs: 2, Timeout: time.Hour, Persistent: true})
	if err != nil {
		t.Fatal(err)
	}
	if box.Name != "worker" || box.Status != "running" {
		t.Fatalf("box = %+v", box)
	}
}

func TestStopResumeForkDelete(t *testing.T) {
	stopped := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/sandboxes/dev" && r.URL.Query().Get("resume") == "true":
			stopped = false
			_ = json.NewEncoder(w).Encode(sandboxSessionResponse{
				Sandbox: Sandbox{Name: "dev", Status: "running", CurrentSessionID: "sbx_dev2"},
				Session: Session{ID: "sbx_dev2", Status: "running"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/sandboxes/dev":
			status := "running"
			if stopped {
				status = "stopped"
			}
			_ = json.NewEncoder(w).Encode(sandboxSessionResponse{
				Sandbox: Sandbox{Name: "dev", Status: status, CurrentSessionID: "sbx_dev"},
				Session: Session{ID: "sbx_dev", Status: status},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sandboxes/sessions/sbx_dev/stop":
			stopped = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/sandboxes/dev/fork":
			_ = json.NewEncoder(w).Encode(sandboxSessionResponse{
				Sandbox: Sandbox{Name: "dev-fork", Status: "running", CurrentSessionID: "sbx_fork"},
				Session: Session{ID: "sbx_fork", Status: "running"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/sandboxes/dev-fork":
			_ = json.NewEncoder(w).Encode(sandboxSessionResponse{
				Sandbox: Sandbox{Name: "dev-fork", Status: "running", CurrentSessionID: "sbx_fork"},
				Session: Session{ID: "sbx_fork", Status: "running"},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/sandboxes/dev":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	if err := client.Stop(context.Background(), "prj_1/dev"); err != nil {
		t.Fatal(err)
	}
	if err := client.Resume(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	id, err := client.Fork(context.Background(), "prj_1/dev", "dev-fork")
	if err != nil {
		t.Fatal(err)
	}
	if id != "prj_1/dev-fork" {
		t.Fatalf("fork id = %s", id)
	}
	if err := client.Delete(context.Background(), "prj_1/dev"); err != nil {
		t.Fatal(err)
	}
}

func TestUnauthorizedRedactsToken(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"nope vercel_test_token"}}`)
	})
	account, err := client.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.Authenticated || !strings.Contains(account.Error, "401") {
		t.Fatalf("account = %+v", account)
	}
	if strings.Contains(account.Error, "vercel_test_token") {
		t.Fatalf("leaked token: %s", account.Error)
	}
}

func TestWriteAndReadTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := WriteTokenFile(path, " tok_abc \n"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTokenFile(path)
	if err != nil || got != "tok_abc" {
		t.Fatalf("got %q err %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("perm = %o", info.Mode().Perm())
	}
}

func TestParseSyncID(t *testing.T) {
	project, name, err := ParseSyncID("prj_abc/my-sandbox")
	if err != nil || project != "prj_abc" || name != "my-sandbox" {
		t.Fatalf("got %q %q %v", project, name, err)
	}
	project, name, err = ParseSyncID("my-sandbox")
	if err != nil || project != "" || name != "my-sandbox" {
		t.Fatalf("bare = %q %q %v", project, name, err)
	}
	if _, _, err := ParseSyncID("../etc/passwd"); err == nil {
		t.Fatal("expected reject")
	}
	if _, _, err := ParseSyncID("has space"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestStartAndResizeFrames(t *testing.T) {
	start, err := EncodeStartFrame("/bin/bash", nil, []string{"TERM=xterm-256color"}, "/vercel/sandbox", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(start), `"type":"start"`) || !strings.Contains(string(start), `"command":"/bin/bash"`) {
		t.Fatalf("start = %s", start)
	}
	resize, err := EncodeResizeFrame(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resize), `"type":"resize"`) {
		t.Fatalf("resize = %s", resize)
	}
	got, err := InteractiveURL("wss://example/pty", "tok/+/x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "token=tok") || strings.Contains(got, "tok/+/x") && !strings.Contains(got, "tok%2F") && !strings.Contains(got, "token=tok") {
		if !strings.Contains(got, "token=tok") {
			t.Fatalf("url = %s", got)
		}
	}
}

func TestProcessStdinEscapeDisconnects(t *testing.T) {
	forward, disconnect, next := processStdinBytes(stdinAfterNL, []byte("~."))
	if !disconnect || len(forward) != 0 {
		t.Fatalf("start ~. forward=%q disconnect=%t next=%v", forward, disconnect, next)
	}
	forward, disconnect, next = processStdinBytes(stdinNormal, []byte("ls\r~."))
	if !disconnect || string(forward) != "ls\r" {
		t.Fatalf("enter ~. forward=%q disconnect=%t", forward, disconnect)
	}
	forward, disconnect, next = processStdinBytes(stdinAfterNL, []byte("~x"))
	if disconnect || string(forward) != "~x" || next != stdinNormal {
		t.Fatalf("~x forward=%q disconnect=%t next=%v", forward, disconnect, next)
	}
	forward, disconnect, next = processStdinBytes(stdinAfterNL, []byte("~~."))
	if disconnect || string(forward) != "~." {
		t.Fatalf("~~. should send a tilde then a dot, forward=%q disconnect=%t", forward, disconnect)
	}
}

func TestIsSessionClose(t *testing.T) {
	if !isSessionClose(nil) || !isSessionClose(errLocalDisconnect) || !isSessionClose(io.EOF) {
		t.Fatal("expected clean closes")
	}
	if !isSessionClose(&websocket.CloseError{Code: websocket.CloseAbnormalClosure, Text: "unexpected EOF"}) {
		t.Fatal("expected websocket close")
	}
	if !isSessionClose(fmt.Errorf("websocket: close 1005 (no status)")) {
		t.Fatal("expected close 1005")
	}
	if isSessionClose(fmt.Errorf("vercel sandbox start: boom")) {
		t.Fatal("start errors must still surface")
	}
}

func TestParseShellOptions(t *testing.T) {
	opts, err := ParseShellOptions([]string{"--name", "dev", "--project", "prj_1", "--team", "team_1"})
	if err != nil || opts.Name != "dev" || opts.ProjectID != "prj_1" || opts.TeamID != "team_1" {
		t.Fatalf("opts = %+v err %v", opts, err)
	}
	if _, err := ParseShellOptions([]string{"--name", "dev"}); err == nil {
		t.Fatal("expected reject")
	}
}
