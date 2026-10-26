package ui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bast/internal/cloud/boat"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

func TestBoatConflictAllowsStartupAndSuppressesAutomaticSync(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "auto-connect"
		if enabled {
			name = "auto-sync"
		}
		t.Run(name, func(t *testing.T) {
			p := paths.ForHome(t.TempDir())
			legacy := filepath.Join(p.ManagedDir, "sync", "box", "config")
			cfg := sshconfig.Manager{Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir, ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys}
			for path, alias := range map[string]string{legacy: "box_old", p.SyncBoatConfig: "boat_new"} {
				if err := cfg.EnsureSyncInclude(path); err != nil {
					t.Fatal(err)
				}
				if err := sshconfig.WriteSyncConfig(path, []sshconfig.SyncHostInput{{Alias: alias, SyncSource: "boat", SyncID: "bx_" + alias, HostName: boat.StoppedHostName}}); err != nil {
					t.Fatal(err)
				}
			}
			store, err := metadata.Open(p.StateFile)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SetBoat(metadata.BoatIntegration{Enabled: enabled, AutoSync: enabled}); err != nil {
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
			m, err := New(p, openssh.Default(), "test")
			if err != nil {
				t.Fatalf("conflict prevented startup: %v", err)
			}
			if m.boatMigrationConflict == nil {
				t.Fatal("migration conflict was not retained")
			}
			if cmd := m.autoSyncCmds(); cmd != nil || m.syncingProviders["boat"] {
				t.Fatal("Boat was scheduled during conflict")
			}
			if !strings.Contains(m.providerDetail("boat").lastSyncError, "conflicting inventories") {
				t.Fatal("provider page hid the conflict")
			}
			m.syncProvider = "boat"
			for _, action := range []string{"enable", "sync", "disable", "auto_on", "auto_off"} {
				if _, cmd := m.runSyncAction(action); cmd != nil || !m.statusError {
					t.Fatalf("allowed Boat action %s", action)
				}
			}
			m.syncer.Boat.Run = func(context.Context, []string, []string) ([]byte, error) {
				t.Error("files preparation contacted Boat during conflict")
				return nil, errors.New("unexpected provider call")
			}
			host := sshconfig.Host{Alias: "box_old", Synced: true, SyncSource: "boat", SyncID: "bx_box_old", Resolved: sshconfig.Resolved{HostName: boat.StoppedHostName}}
			var conflict *sshconfig.BoatMigrationConflict
			if err := m.filesPrepareFn(host)(nil); !errors.As(err, &conflict) {
				t.Fatalf("files preparation error=%v", err)
			}
			for path, original := range before {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(original, after) {
					t.Fatalf("changed %s during conflict: %v", path, err)
				}
			}
			// Other providers still schedule normally.
			if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, AutoSync: true}); err != nil {
				t.Fatal(err)
			}
			m.autoSyncStarted = false
			if cmd := m.autoSyncCmds(); cmd == nil || !m.syncingProviders["gcp"] || m.syncingProviders["boat"] {
				t.Fatal("conflict suppressed unrelated provider")
			}
		})
	}
}

func TestNewRejectsBoatMigrationIOErrors(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	if err := os.MkdirAll(filepath.Join(p.ManagedDir, "sync", "box", "config"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := New(p, openssh.Client{}, "test"); err == nil || !strings.Contains(err.Error(), "read legacy Boat inventory") {
		t.Fatalf("expected startup I/O error, got %v", err)
	}
}
