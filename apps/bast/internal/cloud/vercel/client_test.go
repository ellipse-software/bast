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
	deleted := map[string]int{}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vercel_test_token" {
			t.Errorf("missing bearer")
		}
		if r.URL.Query().Get("teamId") != "team_1" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		name := strings.TrimPrefix(r.URL.Path, "/v2/sandboxes/")
		switch {
		case r.URL.Path == "/v2/sandboxes" && r.Method == http.MethodGet:
			if r.URL.Query().Get("project") == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "project required"}})
				return
			}
			if r.URL.Query().Get("project") != "prj_1" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			pages++
			if r.URL.Query().Get("cursor") == "" {
				next := "page2"
				raw := sandboxListResponse{
					Sandboxes: []Sandbox{
						{Name: "dev", Status: "running", Persistent: true, VCPUs: 2, Runtime: "node24", CurrentSessionID: "sbx_1"},
						{Name: "paused", Status: "stopped", Persistent: true, VCPUs: 2, CurrentSnapshot: "snap_paused"},
					},
				}
				raw.Pagination.Count = 2
				raw.Pagination.Next = &next
				_ = json.NewEncoder(w).Encode(raw)
				return
			}
			_ = json.NewEncoder(w).Encode(sandboxListResponse{
				Sandboxes: []Sandbox{
					{Name: "idle", Status: "stopped", Persistent: true, VCPUs: 1, CurrentSessionID: "sbx_2"},
					{Name: "temp", Status: "stopped", Persistent: false},
					{Name: "dead", Status: "failed"},
					{Name: "saving", Status: "snapshotting", Persistent: true},
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v2/sandboxes/"):
			if deleted[name] > 0 {
				http.NotFound(w, r)
				return
			}
			box := Sandbox{Name: name, Persistent: true}
			switch name {
			case "idle":
				box.Status = "stopped"
			case "paused":
				box.Status = "stopped"
				box.CurrentSnapshot = "snap_paused"
			case "temp":
				box.Status = "stopped"
				box.Persistent = false
			case "dead":
				box.Status = "failed"
			default:
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(sandboxSessionResponse{Sandbox: box})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v2/sandboxes/"):
			deleted[name]++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	discovery, err := client.Discover(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 {
		t.Fatalf("pages = %d, want 2 (list once)", pages)
	}
	if len(deleted) != 0 {
		t.Fatalf("discover must not delete: %v", deleted)
	}
	if got := strings.Join(discovery.Unrestorable, ","); got != "prj_1/dead,prj_1/idle,prj_1/temp" {
		t.Fatalf("unrestorable = %v", discovery.Unrestorable)
	}
	if len(discovery.Instances) != 3 {
		t.Fatalf("instances = %d, want 3: %+v", len(discovery.Instances), discovery.Instances)
	}
	if discovery.Instances[0].SyncID != "prj_1/dev" || !discovery.Instances[0].Running {
		t.Fatalf("running = %+v", discovery.Instances[0])
	}
	byName := map[string]Instance{}
	for _, inst := range discovery.Instances {
		byName[inst.Name] = inst
	}
	if _, ok := byName["paused"]; !ok || byName["paused"].Running {
		t.Fatalf("paused = %+v", byName["paused"])
	}
	if !HostLooksStopped(byName["paused"].Tags) {
		t.Fatal("paused should look stopped")
	}
	if _, ok := byName["saving"]; !ok || byName["saving"].Running {
		t.Fatalf("saving = %+v", byName["saving"])
	}
	if _, ok := byName["idle"]; ok {
		t.Fatal("stopped without snapshot should not be listed")
	}
	if AliasFor(discovery.Instances[0]) != "vercel_dev" {
		t.Fatalf("alias = %s", AliasFor(discovery.Instances[0]))
	}
	host := ToSyncHost(discovery.Instances[0], "")
	if host.HostName != StoppedHost || host.SyncSource != ProviderName || host.User != "" {
		t.Fatalf("sync host = %+v", host)
	}

	cleaned, err := client.CleanupUnrestorable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cleaned, ",") != "dead,idle,temp" {
		t.Fatalf("cleaned = %v", cleaned)
	}
	if deleted["idle"] == 0 || deleted["temp"] == 0 || deleted["dead"] == 0 {
		t.Fatalf("cleanup deleted = %v", deleted)
	}
	if deleted["paused"] != 0 || deleted["saving"] != 0 || deleted["dev"] != 0 {
		t.Fatalf("cleanup deleted restorable sandboxes: %v", deleted)
	}
}

func TestUnrestorableOffline(t *testing.T) {
	cases := []struct {
		box  Sandbox
		want bool
	}{
		{Sandbox{Status: "stopped"}, true},
		{Sandbox{Status: "stopped", CurrentSnapshot: "snap_1"}, false},
		{Sandbox{Status: "snapshotting"}, false},
		{Sandbox{Status: "stopping"}, false},
		{Sandbox{Status: "running"}, false},
		{Sandbox{Status: "pending"}, false},
		{Sandbox{Status: "failed"}, true},
		{Sandbox{Status: "aborted"}, true},
		{Sandbox{Status: "failed", CurrentSnapshot: "snap_1"}, false},
	}
	for _, tc := range cases {
		if got := unrestorableOffline(tc.box); got != tc.want {
			t.Errorf("%+v: got %v want %v", tc.box, got, tc.want)
		}
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

func TestParseProjectList(t *testing.T) {
	got := ParseProjectList("prj_a, prj_b", "prj_a;prj_c")
	if strings.Join(got, ",") != "prj_a,prj_b,prj_c" {
		t.Fatalf("got %v", got)
	}
}

func TestListTeamWideWhenProjectOmitted(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/sandboxes" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("project") != "" {
			t.Errorf("team-wide list should omit project, query=%s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(sandboxListResponse{
			Sandboxes: []Sandbox{
				{Name: "alpha", Status: "running", Persistent: true, ProjectID: "prj_a", CurrentSessionID: "sbx_a"},
				{Name: "beta", Status: "running", Persistent: true, ProjectID: "prj_b", CurrentSessionID: "sbx_b"},
			},
		})
	})
	client.ProjectID = ""
	client.ProjectIDs = nil
	t.Setenv(ProjectEnv, "")
	discovery, err := client.Discover(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %d", len(discovery.Instances))
	}
	if discovery.Instances[0].ProjectID != "prj_a" || discovery.Instances[1].ProjectID != "prj_b" {
		t.Fatalf("projects = %+v %+v", discovery.Instances[0], discovery.Instances[1])
	}
	if GroupPath(discovery.Instances[0]) != "Vercel/prj_a" || GroupPath(discovery.Instances[1]) != "Vercel/prj_b" {
		t.Fatalf("groups = %s %s", GroupPath(discovery.Instances[0]), GroupPath(discovery.Instances[1]))
	}
}

func TestListFallsBackToMultipleProjects(t *testing.T) {
	listed := []string{}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/sandboxes" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		project := r.URL.Query().Get("project")
		if project == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "project required"}})
			return
		}
		listed = append(listed, project)
		name := "box-" + project
		_ = json.NewEncoder(w).Encode(sandboxListResponse{
			Sandboxes: []Sandbox{{Name: name, Status: "running", Persistent: true, CurrentSessionID: "sbx_" + project}},
		})
	})
	client.ProjectID = ""
	client.ProjectIDs = []string{"prj_a", "prj_b"}
	t.Setenv(ProjectEnv, "")
	discovery, err := client.Discover(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(listed, ",") != "prj_a,prj_b" {
		t.Fatalf("listed = %v", listed)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
	if discovery.Instances[0].SyncID != "prj_a/box-prj_a" || discovery.Instances[1].SyncID != "prj_b/box-prj_b" {
		t.Fatalf("ids = %s %s", discovery.Instances[0].SyncID, discovery.Instances[1].SyncID)
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
	if got != "wss://example/pty?token=tok%2F%2B%2Fx" {
		t.Fatalf("url = %s", got)
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
	opts, err = ParseShellOptions([]string{"--name", "dev"})
	if err != nil || opts.Name != "dev" || opts.ProjectID != "" {
		t.Fatalf("name-only = %+v err %v", opts, err)
	}
	if _, err := ParseShellOptions([]string{}); err == nil {
		t.Fatal("expected reject")
	}
}

func TestScopedNameFallsBackToProject(t *testing.T) {
	project, name, err := ScopedName("prj_abc/dev", "prj_other")
	if err != nil || project != "prj_abc" || name != "dev" {
		t.Fatalf("scoped = %q %q %v", project, name, err)
	}
	project, name, err = ScopedName("dev", "prj_fallback")
	if err != nil || project != "prj_fallback" || name != "dev" {
		t.Fatalf("legacy = %q %q %v", project, name, err)
	}
}

func TestIsReadyStateRequiresRunning(t *testing.T) {
	if isReadyState("pending") || isReadyState("PENDING") {
		t.Fatal("pending must not be ready")
	}
	if !isReadyState("running") || !isReadyState("Running") {
		t.Fatal("running must be ready")
	}
}
