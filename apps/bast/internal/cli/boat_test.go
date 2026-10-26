package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

func TestBoatCommandsAndLegacyAliases(t *testing.T) {
	for _, provider := range []string{"boat", "box"} {
		for _, args := range [][]string{{provider, "new", "--help"}, {"sync", provider, "--help"}, {"sync", "disable", provider, "--help"}} {
			out, errOut, err := runTestCLI(t, t.TempDir(), openssh.Client{}, args...)
			if err != nil || errOut != "" || !strings.Contains(out, "boat") || strings.Contains(out, "bast box") {
				t.Fatalf("args=%v out=%q stderr=%q err=%v", args, out, errOut, err)
			}
		}
	}
	home := t.TempDir()
	writeCompleteFixture(t, home)
	for _, provider := range []string{"boat", "box"} {
		values, _ := runComplete(t, home, provider, "fork", "")
		if !containsValue(values, "ascii_boat_dev") {
			t.Fatalf("%s completion lost hosts: %v", provider, values)
		}
	}
}

func TestLegacyDisableMigratesInventoryAndKeepsOptOut(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	legacy := filepath.Join(p.ManagedDir, "sync", "box", "config")
	m := sshconfig.Manager{Home: home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir, ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys}
	if err := m.EnsureSyncInclude(legacy); err != nil {
		t.Fatal(err)
	}
	if err := sshconfig.WriteSyncConfig(legacy, []sshconfig.SyncHostInput{{Alias: "box_dev", SyncSource: "box", SyncID: "bx_dev0001", HostName: "203.0.113.4"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.StateFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.StateFile, []byte(`{"version":7,"hosts":{"box_dev":{"group":"Box"}},"integrations":{"box":{"enabled":true,"autoSync":true}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runTestCLI(t, home, fakeOpenSSH(t), "--json", "sync", "disable", "box")
	if err != nil || errOut != "" || !strings.Contains(out, `"provider":"boat"`) {
		t.Fatalf("out=%q stderr=%q err=%v", out, errOut, err)
	}
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if store.Boat().Enabled || !store.Boat().Disabled {
		t.Fatalf("opt-out lost: %+v", store.Boat())
	}
	for _, path := range []string{legacy, p.SyncBoatConfig} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("inventory still present: %s %v", path, err)
		}
	}
	hosts, err := m.Discover()
	if err != nil || len(hosts) != 0 {
		t.Fatalf("hosts remain: %+v %v", hosts, err)
	}
}

func TestBoatConflictAllowsDoctorAndHostListing(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	legacy := filepath.Join(p.ManagedDir, "sync", "box", "config")
	m := sshconfig.Manager{Home: home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir, ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys}
	for path, alias := range map[string]string{legacy: "box_old", p.SyncBoatConfig: "boat_new"} {
		if err := m.EnsureSyncInclude(path); err != nil {
			t.Fatal(err)
		}
		if err := sshconfig.WriteSyncConfig(path, []sshconfig.SyncHostInput{{Alias: alias, SyncSource: "boat", SyncID: "bx_" + alias, HostName: "203.0.113.4"}}); err != nil {
			t.Fatal(err)
		}
	}
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"box_old", "boat_new"} {
		if err := store.SetHost(alias, metadata.Host{Group: "Boat", Notes: "preserve " + alias}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetBoat(metadata.BoatIntegration{Enabled: true, AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, path := range []string{legacy, p.SyncBoatConfig, p.MainConfig, p.ManagedConfig, p.StateFile} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = content
	}
	client := fakeOpenSSH(t)
	for _, fix := range []bool{false, true} {
		args := []string{"--json", "doctor", "--category", "sync"}
		if fix {
			args = append(args, "--fix")
		}
		out, stderr, err := runTestCLI(t, home, client, args...)
		if code, ok := ExitCode(err); !ok || code != 1 || stderr != "" || !strings.Contains(out, "sync.boat_migration_conflict") {
			t.Fatalf("doctor out=%q stderr=%q err=%v", out, stderr, err)
		}
	}
	out, stderr, err := runTestCLI(t, home, client, "--json", "hosts", "list")
	if err != nil || stderr != "" || !strings.Contains(out, "box_old") || !strings.Contains(out, "boat_new") {
		t.Fatalf("hosts out=%q stderr=%q err=%v", out, stderr, err)
	}
	for _, args := range [][]string{{"sync", "boat"}, {"sync", "disable", "boat"}, {"sync", "disable", "box"}, {"boat", "new"}, {"box", "delete", "--yes", "bx_box_old"}} {
		_, stderr, err := runTestCLI(t, home, client, append([]string{"--json"}, args...)...)
		if err == nil || !strings.Contains(stderr, "conflicting inventories") {
			t.Fatalf("args=%v stderr=%q err=%v", args, stderr, err)
		}
	}
	for path, original := range before {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(original, after) {
			t.Fatalf("changed %s during conflict: %v", path, err)
		}
	}
}

func TestNewStillRejectsBoatMigrationIOErrors(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	// Reading a directory is an I/O error on every supported OS, even as root.
	if err := os.MkdirAll(filepath.Join(p.ManagedDir, "sync", "box", "config"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := New(p, openssh.Client{}, strings.NewReader(""), io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "read legacy Boat inventory") {
		t.Fatalf("expected startup I/O error, got %v", err)
	}
}
