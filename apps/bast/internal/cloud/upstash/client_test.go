package upstash

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	keyFile := filepath.Join(t.TempDir(), "upstash-box-api-key")
	if err := WriteKeyFile(keyFile, "box_testkey"); err != nil {
		t.Fatal(err)
	}
	return &Client{
		BaseURL:  server.URL,
		KeyFile:  keyFile,
		PollWait: time.Millisecond,
		HTTP:     server.Client(),
	}
}

func TestListAndDiscover(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Box-Api-Key") != "box_testkey" {
			t.Errorf("missing api key header")
		}
		if r.URL.Path != "/v2/box" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]BoxData{
			{ID: "current-wasp-05510", Name: "dev", Status: "idle", Size: "small", Runtime: "node"},
			{ID: "paused-fox-1", Name: "idle", Status: "paused", Size: "medium", KeepAlive: false},
			{ID: "gone", Status: "deleted"},
		})
	})
	discovery, err := client.Discover(context.Background(), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(discovery.Instances))
	}
	if discovery.Instances[0].SyncID != "current-wasp-05510" || !discovery.Instances[0].Running {
		t.Fatalf("running = %+v", discovery.Instances[0])
	}
	if discovery.Instances[0].User != "current-wasp-05510" {
		t.Fatalf("ssh user = %q", discovery.Instances[0].User)
	}
	if discovery.Instances[1].SyncID != "paused-fox-1" || discovery.Instances[1].Running {
		t.Fatalf("paused = %+v", discovery.Instances[1])
	}
	if !HostLooksStopped(discovery.Instances[1].Tags) {
		t.Fatal("paused should look stopped")
	}
	if HostLooksStopped(discovery.Instances[0].Tags) {
		t.Fatal("running should not look stopped")
	}
	if AliasFor(discovery.Instances[0]) != "upstash_dev" {
		t.Fatalf("alias = %s", AliasFor(discovery.Instances[0]))
	}
	host := ToSyncHost(discovery.Instances[0], "")
	if !host.PasswordOnly || host.User != "current-wasp-05510" || host.SyncSource != ProviderName {
		t.Fatalf("sync host = %+v", host)
	}
}

func TestCreatePollsUntilReady(t *testing.T) {
	gets := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/box":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["runtime"] != "python" || body["size"] != "large" || body["keep_alive"] != true {
				t.Errorf("create body = %#v", body)
			}
			_ = json.NewEncoder(w).Encode(BoxData{ID: "new-box-1", Status: "creating"})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/box/new-box-1":
			gets++
			status := "creating"
			if gets >= 2 {
				status = "idle"
			}
			_ = json.NewEncoder(w).Encode(BoxData{ID: "new-box-1", Name: "worker", Status: status, Runtime: "python", Size: "large", KeepAlive: true})
		default:
			http.NotFound(w, r)
		}
	})
	box, err := client.Create(context.Background(), CreateOpts{Runtime: "python", Size: "large", KeepAlive: true})
	if err != nil {
		t.Fatal(err)
	}
	if box.ID != "new-box-1" || box.Status != "idle" {
		t.Fatalf("box = %+v", box)
	}
}

func TestPauseKeepAliveRejected(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/box/live-box" && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(BoxData{ID: "live-box", Status: "idle", KeepAlive: true})
			return
		}
		http.Error(w, "should not pause", 500)
	})
	err := client.Pause(context.Background(), "live-box")
	if err == nil || !strings.Contains(err.Error(), "keep-alive") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteAndUnauthorized(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v2/box/old-box" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/v2/box" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"nope box_testkey"}`)
			return
		}
		http.NotFound(w, r)
	})
	if err := client.Delete(context.Background(), "old-box"); err != nil {
		t.Fatal(err)
	}
	account, err := client.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.Authenticated || !strings.Contains(account.Error, "401") {
		t.Fatalf("account = %+v", account)
	}
	if strings.Contains(account.Error, "box_testkey") {
		t.Fatalf("leaked key: %s", account.Error)
	}
}

func TestForkSnapshotRestore(t *testing.T) {
	gets := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/box/src-box":
			_ = json.NewEncoder(w).Encode(BoxData{ID: "src-box", Status: "idle", Size: "small", Runtime: "node"})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/box/src-box/snapshots":
			_ = json.NewEncoder(w).Encode(Snapshot{ID: "snap_1", Name: "bast-fork", Status: "creating"})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/box/src-box/snapshots":
			_ = json.NewEncoder(w).Encode(map[string]any{"snapshots": []Snapshot{{ID: "snap_1", Status: "ready"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/box/from-snapshot":
			_ = json.NewEncoder(w).Encode(BoxData{ID: "fork-box", Status: "creating"})
		case r.Method == http.MethodGet && r.URL.Path == "/v2/box/fork-box":
			gets++
			status := "creating"
			if gets >= 1 {
				status = "idle"
			}
			_ = json.NewEncoder(w).Encode(BoxData{ID: "fork-box", Status: status})
		default:
			http.NotFound(w, r)
		}
	})
	id, err := client.Fork(context.Background(), "src-box")
	if err != nil {
		t.Fatal(err)
	}
	if id != "fork-box" {
		t.Fatalf("id = %s", id)
	}
}

func TestWriteAndReadKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := WriteKeyFile(path, " box_abc \n"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadKeyFile(path)
	if err != nil || got != "box_abc" {
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
	if _, err := ParseSyncID("current-wasp-05510"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSyncID("box_abc123"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSyncID("../etc/passwd"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := ParseSyncID("has space"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestApplyAskPass(t *testing.T) {
	cmd := exec.Command("true")
	cmd.Env = []string{"FOO=bar", "SSH_ASKPASS=old"}
	ApplyAskPass(cmd, "/usr/bin/bast")
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, AskPassEnv+"="+AskPassValue) {
		t.Fatalf("env = %s", joined)
	}
	if !strings.Contains(joined, "SSH_ASKPASS=/usr/bin/bast") || strings.Contains(joined, "SSH_ASKPASS=old") {
		t.Fatalf("askpass env = %s", joined)
	}
	if !strings.Contains(joined, "SSH_ASKPASS_REQUIRE=force") {
		t.Fatal("missing SSH_ASKPASS_REQUIRE")
	}
}
