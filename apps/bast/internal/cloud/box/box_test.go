package box

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDiscoverParsesRunningAndStopped(t *testing.T) {
	ip := "203.0.113.10"
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "--version"):
				return []byte("box 1.0.0"), nil
			case strings.Contains(cmd, "status"):
				return []byte(`{"ok":true,"user":{"login":"octocat","email":"o@example.com"}}`), nil
			case strings.Contains(cmd, "list"):
				if !strings.Contains(cmd, "--filter srt") && !strings.Contains(cmd, "--filter") {
					return nil, fmt.Errorf("discover should list stopping boxes too: %s", cmd)
				}
				if strings.Contains(cmd, "--filter") && !strings.Contains(cmd, "srt") && !strings.Contains(cmd, "--all") {
					return nil, fmt.Errorf("discover filter should include t=stopping: %s", cmd)
				}
				payload := map[string]any{
					"boxes": []map[string]any{
						{"id": "bx_running1", "name": "Dev", "state": "idle", "ip": ip, "snapshotAvailable": true, "type": "default"},
						{"id": "bx_stopped1", "name": "Old", "state": "stopped", "ip": nil, "snapshotAvailable": true, "type": "small"},
						{"id": "bx_archiving", "name": "Snap", "state": "archiving", "ip": nil, "snapshotAvailable": false, "type": "default"},
						{"id": "bx_norunip", "name": "NoIP", "state": "running", "ip": nil, "snapshotAvailable": false},
					},
				}
				return json.Marshal(payload)
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 4 {
		t.Fatalf("instances = %d, want 4", len(discovery.Instances))
	}
	if discovery.Instances[0].SyncID != "bx_running1" || discovery.Instances[0].HostName != ip || !discovery.Instances[0].Running {
		t.Fatalf("running instance = %+v", discovery.Instances[0])
	}
	if discovery.Instances[1].SyncID != "bx_norunip" || !discovery.Instances[1].Running || discovery.Instances[1].HostName != stoppedHostName {
		t.Fatalf("running-without-ip instance = %+v", discovery.Instances[1])
	}
	// Non-running sort by name: Old (stopped) then Snap (archiving).
	if discovery.Instances[2].SyncID != "bx_stopped1" || discovery.Instances[2].Running || discovery.Instances[2].HostName != stoppedHostName {
		t.Fatalf("stopped instance = %+v", discovery.Instances[2])
	}
	if discovery.Instances[3].SyncID != "bx_archiving" || discovery.Instances[3].Running || discovery.Instances[3].State != "stopping" {
		t.Fatalf("archiving instance = %+v", discovery.Instances[3])
	}
	if GroupPath(discovery.Instances[0]) != "Box" || GroupPath(discovery.Instances[3]) != "Box" {
		t.Fatal("group paths mismatch")
	}
	if !HostLooksStopped(discovery.Instances[2].HostName, discovery.Instances[2].Tags) {
		t.Fatal("stopped instance should look stopped")
	}
	if !HostLooksStopped(discovery.Instances[3].HostName, discovery.Instances[3].Tags) {
		t.Fatal("archiving instance should look stopped")
	}
	if HostLooksStopped(discovery.Instances[0].HostName, discovery.Instances[0].Tags) {
		t.Fatal("running instance should not look stopped")
	}
	if HostLooksStopped(discovery.Instances[1].HostName, discovery.Instances[1].Tags) {
		t.Fatal("running-without-ip should not look stopped")
	}
	if IsTerminalStoppedState("archiving") || IsTerminalStoppedState("stopping") {
		t.Fatal("archiving must not count as terminal stopped")
	}
	if !IsTerminalStoppedState("archived") || !IsTerminalStoppedState("stopped") {
		t.Fatal("archived/stopped should be terminal")
	}
	host := ToSyncHost(discovery.Instances[0], "box_dev")
	if host.User != SSHUser || host.IdentityFile != IdentityFile || host.SyncSource != ProviderName {
		t.Fatalf("sync host = %+v", host)
	}
}

func TestParseNewJSONL(t *testing.T) {
	out := []byte(`{"event":"created","id":"bx_newbox01"}
{"event":"state","id":"bx_newbox01","state":"provisioning"}
{"event":"ready","id":"bx_newbox01"}
`)
	id, err := parseNewJSONL(out)
	if err != nil || id != "bx_newbox01" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestParseNewJSONLReturnsIDWithError(t *testing.T) {
	out := []byte(`{"event":"created","id":"bx_partial01"}
{"ok":false,"error":"quota exceeded"}
`)
	id, err := parseNewJSONL(out)
	if id != "bx_partial01" {
		t.Fatalf("id=%q, want bx_partial01", id)
	}
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitStoppedReturnsWhenStopping(t *testing.T) {
	infoCalls := 0
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, " stop "):
				return []byte(`{"ok":true,"id":"bx_archive1"}`), nil
			case strings.Contains(cmd, " info "):
				infoCalls++
				return []byte(`{"box":{"id":"bx_archive1","name":"snap","state":"archiving"}}`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Stop(ctx, "bx_archive1"); err != nil {
		t.Fatal(err)
	}
	if infoCalls < 1 {
		t.Fatal("expected WaitStopped to poll info")
	}
}

func TestWaitStoppedTimesOutIfStillRunning(t *testing.T) {
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			if strings.Contains(cmd, "info") {
				return []byte(`{"box":{"id":"bx_run","name":"live","state":"idle","ip":"203.0.113.10"}}`), nil
			}
			return nil, fmt.Errorf("unexpected %s", cmd)
		},
	}
	err := client.WaitStopped(context.Background(), "bx_run", 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v", err)
	}
}

func TestStopSurfacesRejectedResponse(t *testing.T) {
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "stop"):
				return []byte(`{"ok":false,"error":"already stopping"}`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	err := client.Stop(context.Background(), "bx_stopme01")
	if err == nil || !strings.Contains(err.Error(), "already stopping") {
		t.Fatalf("err = %v", err)
	}
}

func TestResumeSurfacesRejectedResponse(t *testing.T) {
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "resume"):
				return []byte(`{"ok":false,"error":"not stopped"}`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	err := client.Resume(context.Background(), "bx_resumeme", ResumeOpts{})
	if err == nil || !strings.Contains(err.Error(), "not stopped") {
		t.Fatalf("err = %v", err)
	}
}

func TestForkRequiresSnapshot(t *testing.T) {
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "--version"):
				return []byte("box 1.0.0"), nil
			case strings.Contains(cmd, "info"):
				return []byte(`{"box":{"id":"bx_source01","name":"Src","state":"idle","ip":"1.2.3.4","snapshotAvailable":false}}`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	_, err := client.Fork(context.Background(), "bx_source01", ForkOpts{})
	if err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("err = %v", err)
	}
}

func TestAccountNotLoggedIn(t *testing.T) {
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			if strings.Contains(strings.Join(args, " "), "--version") {
				return []byte("box 1.0.0"), nil
			}
			return nil, fmt.Errorf("box: unauthorized")
		},
	}
	status, err := client.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Authenticated {
		t.Fatal("expected unauthenticated")
	}
}

func TestAccountParsesRealStatusJSON(t *testing.T) {
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			if strings.Contains(cmd, "--version") {
				return []byte("box 0.1.145"), nil
			}
			if strings.Contains(cmd, "status") {
				return []byte(`{"account":{"identifier":"hi@ted.ac","loginState":"active","plan":"no plan","status":"active","suspension":null},"api":{"healthy":true},"config":{"apiUrl":"https://ascii.dev","path":"/tmp/config.json"}}`), nil
			}
			return nil, fmt.Errorf("unexpected %s", cmd)
		},
	}
	status, err := client.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Authenticated || status.Login != "hi@ted.ac" {
		t.Fatalf("status = %+v", status)
	}
}

func TestResolveBoxBinPrefersAsciiInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX executable fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, ".ascii", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "box")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("BOX_CLI", "")
	// Clear PATH so LookPath fails.
	t.Setenv("PATH", "/nonexistent")
	got := resolveBoxBin()
	if got != path {
		t.Fatalf("resolveBoxBin = %q, want %q", got, path)
	}
}

func TestAliasFor(t *testing.T) {
	alias := AliasFor(Instance{Name: "Box 2026-05-28", SyncID: "bx_abc12345"})
	if !strings.HasPrefix(alias, "box_") {
		t.Fatalf("alias = %q", alias)
	}
}
