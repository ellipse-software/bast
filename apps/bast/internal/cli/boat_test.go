package cli

import (
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
