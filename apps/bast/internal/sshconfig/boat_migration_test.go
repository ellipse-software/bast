package sshconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoatInventoryMigration(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			m := testManager(t)
			m.SyncBoatConfig = filepath.Join(m.ManagedDir, "sync", "boat", "config")
			legacy := filepath.Join(m.ManagedDir, "sync", "box", "config")
			if err := m.EnsureManaged(); err != nil {
				t.Fatal(err)
			}
			if enabled {
				if err := m.EnsureSyncInclude(legacy); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteSyncConfig(legacy, []SyncHostInput{{Alias: "box_dev", SyncSource: "box", SyncID: "bx_dev0001", HostName: "box.stopped.invalid", User: "user", IdentityFile: "~/.ssh/ascii_box_ed25519", IdentitiesOnly: true, ExtraOptions: []string{"ServerAliveInterval 30"}}}); err != nil {
				t.Fatal(err)
			}
			if err := m.MigrateBoatSync(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(legacy); !os.IsNotExist(err) {
				t.Fatalf("old inventory remains: %v", err)
			}
			blocks, err := LoadSyncHosts(m.SyncBoatConfig)
			if err != nil || len(blocks) != 1 {
				t.Fatalf("blocks=%+v err=%v", blocks, err)
			}
			block := blocks[0]
			if block.Alias != "box_dev" || block.SyncSource != "boat" || block.SyncID != "bx_dev0001" || block.IdentityFile != "~/.ssh/ascii_box_ed25519" || block.HostName != "boat.stopped.invalid" {
				t.Fatalf("inventory changed: %+v", block)
			}
			content, err := os.ReadFile(m.SyncBoatConfig)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), "ServerAliveInterval 30") {
				t.Fatal("lost SSH option")
			}
			hosts, err := m.Discover()
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if enabled {
				want = 1
			}
			if len(hosts) != want {
				t.Fatalf("enabled=%t hosts=%+v", enabled, hosts)
			}
			if enabled && (hosts[0].Alias != "box_dev" || hosts[0].SyncSource != "boat") {
				t.Fatalf("host changed: %+v", hosts[0])
			}
			if err := m.MigrateBoatSync(); err != nil {
				t.Fatalf("retry: %v", err)
			}
			after, err := os.ReadFile(m.SyncBoatConfig)
			if err != nil || !bytes.Equal(content, after) {
				t.Fatalf("retry changed inventory: %v", err)
			}
			if err := m.RemoveSyncInclude(m.SyncBoatConfig); err != nil {
				t.Fatal(err)
			}
			hosts, err = m.Discover()
			if err != nil || len(hosts) != 0 {
				t.Fatalf("disconnect left hosts: %+v %v", hosts, err)
			}
		})
	}
}

func TestBoatMigrationRecoversAfterCopyAndRejectsConflicts(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name := "interrupted"
		if conflict {
			name = "conflict"
		}
		t.Run(name, func(t *testing.T) {
			m := testManager(t)
			m.SyncBoatConfig = filepath.Join(m.ManagedDir, "sync", "boat", "config")
			legacy := filepath.Join(m.ManagedDir, "sync", "box", "config")
			if err := m.EnsureSyncInclude(legacy); err != nil {
				t.Fatal(err)
			}
			if err := WriteSyncConfig(legacy, []SyncHostInput{{Alias: "box_dev", SyncSource: "box", SyncID: "bx_dev0001", HostName: "203.0.113.4"}}); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(legacy)
			if err != nil {
				t.Fatal(err)
			}
			target := migrateBoatInventory(original)
			if conflict {
				target = []byte("Host boat_other\n HostName 203.0.113.5\n")
			}
			if err := atomicWrite(m.SyncBoatConfig, target, 0600); err != nil {
				t.Fatal(err)
			}
			err = m.MigrateBoatSync()
			if conflict {
				if err == nil || !strings.Contains(err.Error(), "conflicting inventories") {
					t.Fatalf("expected conflict, got %v", err)
				}
				left, _ := os.ReadFile(legacy)
				right, _ := os.ReadFile(m.SyncBoatConfig)
				if !bytes.Equal(left, original) || !bytes.Equal(right, target) {
					t.Fatal("conflict overwrote inventory")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacyBoatMarkersReadAsBoat(t *testing.T) {
	m := testManager(t)
	legacy := filepath.Join(m.ManagedDir, "sync", "box", "config")
	if err := m.EnsureSyncInclude(legacy); err != nil {
		t.Fatal(err)
	}
	if err := WriteSyncConfig(legacy, []SyncHostInput{{Alias: "box_dev", SyncSource: "box", SyncID: "bx_dev0001", HostName: "203.0.113.4"}}); err != nil {
		t.Fatal(err)
	}
	hosts, err := m.Discover()
	if err != nil || len(hosts) != 1 || hosts[0].SyncSource != "boat" {
		t.Fatalf("legacy hosts=%+v err=%v", hosts, err)
	}
	blocks, err := LoadSyncHosts(legacy)
	if err != nil || len(blocks) != 1 || blocks[0].SyncSource != "boat" {
		t.Fatalf("legacy blocks=%+v err=%v", blocks, err)
	}
}
