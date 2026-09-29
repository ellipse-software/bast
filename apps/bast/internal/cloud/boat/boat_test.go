package boat

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
				return []byte("boat 1.0.0"), nil
			case strings.Contains(cmd, "status"):
				return []byte(`{"ok":true,"user":{"login":"octocat","email":"o@example.com"}}`), nil
			case strings.Contains(cmd, "list"):
				if !strings.Contains(cmd, "--filter srt") && !strings.Contains(cmd, "--filter") {
					return nil, fmt.Errorf("discover should list stopping sandboxes too: %s", cmd)
				}
				if strings.Contains(cmd, "--filter") && !strings.Contains(cmd, "srt") && !strings.Contains(cmd, "--all") {
					return nil, fmt.Errorf("discover filter should include t=stopping: %s", cmd)
				}
				payload := map[string]any{
					"sandboxes": []map[string]any{
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
	if GroupPath(discovery.Instances[0]) != "Boat" || GroupPath(discovery.Instances[3]) != "Boat" {
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
	host := ToSyncHost(discovery.Instances[0], "boat_dev")
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
				return []byte(`{"sandbox":{"id":"bx_archive1","name":"snap","state":"archiving"}}`), nil
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
				return []byte(`{"sandbox":{"id":"bx_run","name":"live","state":"idle","ip":"203.0.113.10"}}`), nil
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

func TestDeleteWaitsUntilGone(t *testing.T) {
	gone := false
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "--version"):
				return []byte("boat 1.0.0"), nil
			case strings.Contains(cmd, "delete"):
				gone = true
				return []byte(`{"ok":true,"id":"bx_gone001"}`), nil
			case strings.Contains(cmd, "info"):
				if gone {
					return nil, fmt.Errorf("boat info: not found")
				}
				return []byte(`{"sandbox":{"id":"bx_gone001","name":"Gone","state":"idle"}}`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	if err := client.Delete(context.Background(), "bx_gone001"); err != nil {
		t.Fatal(err)
	}
}

func TestListAndDeleteSnapshots(t *testing.T) {
	named := []NamedSnapshot{{SnapshotID: "snap_named", SourceSandboxID: "bx_source01", Name: "web-stack"}}
	snaps := []Snapshot{{ID: "snap_inc", SandboxID: "bx_source01", Kind: "incremental", Status: "completed"}}
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "snapshots"):
				return json.Marshal(map[string]any{"snapshots": snaps, "named": named})
			case strings.Contains(cmd, "snapshot delete"):
				return []byte(`{"ok":true,"id":"snap_inc"}`), nil
			case strings.Contains(cmd, "snapshot rm"):
				return []byte(`{"ok":true,"id":"snap_named"}`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	list, err := client.ListSnapshots(context.Background(), "bx_source01")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Snapshots) != 1 || len(list.Named) != 1 {
		t.Fatalf("list = %+v", list)
	}
	if err := client.DeleteSnapshot(context.Background(), "snap_inc"); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveNamedSnapshot(context.Background(), "web-stack"); err != nil {
		t.Fatal(err)
	}
}

func TestForkRequiresSnapshot(t *testing.T) {
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "--version"):
				return []byte("boat 1.0.0"), nil
			case strings.Contains(cmd, "info"):
				return []byte(`{"sandbox":{"id":"bx_source01","name":"Src","state":"idle","ip":"1.2.3.4","snapshotAvailable":false}}`), nil
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
				return []byte("boat 1.0.0"), nil
			}
			return nil, fmt.Errorf("boat: unauthorized")
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
				return []byte("boat 0.1.145"), nil
			}
			if strings.Contains(cmd, "status") {
				return []byte(`{"account":{"identifier":"hi@ted.ac","loginState":"active","plan":"no plan","status":"active","suspension":null},"api":{"healthy":true},"config":{"apiUrl":"https://boat.dev","path":"/tmp/config.json"}}`), nil
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

func TestResolveBoatBinPrefersAsciiInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX executable fixture")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, ".ascii", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "boat")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("BOAT_CLI", "")
	// Clear PATH so LookPath fails.
	t.Setenv("PATH", "/nonexistent")
	got := resolveBoatBin()
	if got != path {
		t.Fatalf("resolveBoatBin = %q, want %q", got, path)
	}
}

func TestAliasFor(t *testing.T) {
	alias := AliasFor(Instance{Name: "Boat 2026-05-28", SyncID: "bx_abc12345"})
	if !strings.HasPrefix(alias, "boat_") {
		t.Fatalf("alias = %q", alias)
	}
}

func TestBoatWireResponses(t *testing.T) {
	for _, payload := range []string{
		`{"sandbox":{"id":"bx_wire01","state":"idle","ip":"203.0.113.3"}}`,
		`{"id":"bx_wire01","state":"idle","ip":"203.0.113.3"}`,
	} {
		client := &Client{Run: func(context.Context, []string, []string) ([]byte, error) { return []byte(payload), nil }}
		inst, err := client.Info(context.Background(), "bx_wire01")
		if err != nil || inst.SyncID != "bx_wire01" || !inst.Running {
			t.Fatalf("info=%+v err=%v", inst, err)
		}
	}
	for _, payload := range []string{`{"type":"sandbox.created","sandbox":{"id":"bx_wire01"}}`, `{"type":"sandbox.fork","sandbox":{"id":"bx_wire01"}}`} {
		id, err := parseNewJSONL([]byte(payload))
		if err != nil || id != "bx_wire01" {
			t.Fatalf("create id=%q err=%v", id, err)
		}
		id, err = parseActionID([]byte(payload), "fork")
		if err != nil || id != "bx_wire01" {
			t.Fatalf("fork id=%q err=%v", id, err)
		}
	}
	client := &Client{Run: func(context.Context, []string, []string) ([]byte, error) {
		return []byte(`{"snapshots":[{"id":"snap_1","sandboxId":"bx_wire01","kind":"incremental"}],"named":[{"name":"base","snapshotId":"snap_1","sourceSandboxId":"bx_wire01","status":"ready"}]}`), nil
	}}
	list, err := client.ListSnapshots(context.Background(), "")
	if err != nil || len(list.Snapshots) != 1 || len(list.Named) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if list.Snapshots[0].SandboxID != "bx_wire01" || list.Named[0].SourceSandboxID != "bx_wire01" || list.Named[0].SnapshotID != "snap_1" {
		t.Fatalf("lost snapshot ownership: %+v", list)
	}
	if !sandboxMissing(fmt.Errorf("sandbox_not_found")) {
		t.Fatal("Boat error code not recognized")
	}
}

func TestDiscoverRejectsUnexpectedInventory(t *testing.T) {
	for _, payload := range []string{`{}`, `{"boxes":[]}`, `{"sandboxes":null}`, `{"sandboxes":[{"name":"missing id"}]}`, `{"ok":false,"sandboxes":[]}`} {
		client := &Client{Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			if args[1] == "list" {
				return []byte(payload), nil
			}
			return []byte(`{"ok":true,"user":{"login":"test"}}`), nil
		}}
		discovery, err := client.Discover(context.Background(), DiscoverConfig{})
		if err == nil || discovery.Complete {
			t.Fatalf("unsafe inventory accepted: %s %+v %v", payload, discovery, err)
		}
	}
}

func TestDiscoverMarksPaginatedInventoryIncomplete(t *testing.T) {
	client := &Client{Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
		if args[1] == "list" {
			return []byte(`{"sandboxes":[{"id":"bx_wire01","state":"idle"}],"pageInfo":{"hasMore":true,"nextCursor":"next"}}`), nil
		}
		return []byte(`{"ok":true,"user":{"login":"test"}}`), nil
	}}
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil || discovery.Complete || len(discovery.Warnings) == 0 || len(discovery.Instances) != 1 {
		t.Fatalf("discovery=%+v err=%v", discovery, err)
	}
}

func TestBoatCLIOverride(t *testing.T) {
	t.Setenv("BOAT_CLI", filepath.Join(t.TempDir(), "custom-boat"))
	t.Setenv("BOX_CLI", "must-not-run-box")
	if got := New().Boat; got != os.Getenv("BOAT_CLI") {
		t.Fatalf("CLI=%q", got)
	}
}

func TestDiscoverReportsListFailureBeforeMissingInventory(t *testing.T) {
	for _, test := range []struct {
		payload string
		want    string
	}{
		{`{"ok":false,"error":"quota exceeded"}`, "boat list failed: quota exceeded"},
		{`{"ok":false,"error":"not authorized","sandboxes":[]}`, "boat list failed: not authorized"},
		{`{"ok":false,"error":"  "}`, "boat list failed; run boat list --json for details"},
		{`{"ok":true}`, "parse boat list: missing sandboxes array"},
	} {
		t.Run(test.payload, func(t *testing.T) {
			client := &Client{Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
				if args[1] == "list" {
					return []byte(test.payload), nil
				}
				return []byte(`{"ok":true,"user":{"login":"test"}}`), nil
			}}
			discovery, err := client.Discover(context.Background(), DiscoverConfig{})
			if err == nil || !strings.HasPrefix(err.Error(), test.want) || discovery.Complete {
				t.Fatalf("discovery=%+v error=%v, want %q", discovery, err, test.want)
			}
		})
	}
}
