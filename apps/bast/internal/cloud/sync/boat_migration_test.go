package sync

import (
	"context"
	"path/filepath"
	"testing"

	"bast/internal/cloud/sandboxfake"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func TestBoatSyncPreservesMigratedAliasAndMetadata(t *testing.T) {
	engine, p, store := testEngine(t)
	legacy := filepath.Join(p.ManagedDir, "sync", "box", "config")
	if err := engine.Config.EnsureSyncInclude(legacy); err != nil {
		t.Fatal(err)
	}
	if err := sshconfig.WriteSyncConfig(legacy, []sshconfig.SyncHostInput{{Alias: "box_dev", SyncSource: "box", SyncID: "bx_dev0001", HostName: "203.0.113.4"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetHost("box_dev", metadata.Host{Label: "Dev", Group: "Boat", Notes: "keep me", ConnectionCount: 3}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Config.MigrateBoatSync(); err != nil {
		t.Fatal(err)
	}
	fake := sandboxfake.NewBoat()
	fake.Put(sandboxfake.BoatRec{ID: "bx_dev0001", Name: "Renamed", State: "idle", IP: "203.0.113.5"})
	engine.Boat.Run = fake.Runner()
	result, err := engine.SyncBoat(context.Background())
	if err != nil || result.Provider != "boat" || len(result.Aliases) != 1 || result.Aliases[0] != "box_dev" {
		t.Fatalf("sync=%+v err=%v", result, err)
	}
	blocks := loadProviderConfig(t, p.SyncBoatConfig)
	if len(blocks) != 1 || blocks[0].Alias != "box_dev" || blocks[0].SyncSource != "boat" || blocks[0].HostName != "203.0.113.5" {
		t.Fatalf("blocks=%+v", blocks)
	}
	host := store.Host("box_dev")
	if host.Notes != "keep me" || host.ConnectionCount != 3 || host.Group != "Boat" || host.Label != "Renamed" {
		t.Fatalf("metadata=%+v", host)
	}
}

func TestBoatSyncPreservesInventoryOnIncompleteResponses(t *testing.T) {
	for _, payload := range []string{`{"boxes":[]}`, `{"sandboxes":[],"pageInfo":{"hasMore":true}}`} {
		engine, p, store := testEngine(t)
		if err := engine.Config.EnsureSyncInclude(p.SyncBoatConfig); err != nil {
			t.Fatal(err)
		}
		if err := sshconfig.WriteSyncConfig(p.SyncBoatConfig, []sshconfig.SyncHostInput{{Alias: "box_dev", SyncSource: "boat", SyncID: "bx_dev0001", HostName: "203.0.113.4"}}); err != nil {
			t.Fatal(err)
		}
		if err := store.SetHost("box_dev", metadata.Host{Group: "Boat", Notes: "keep me"}); err != nil {
			t.Fatal(err)
		}
		engine.Boat.Run = func(_ context.Context, args []string, _ []string) ([]byte, error) {
			if args[1] == "list" {
				return []byte(payload), nil
			}
			return []byte(`{"ok":true,"user":{"login":"test"}}`), nil
		}
		result, err := engine.SyncBoat(context.Background())
		if err == nil && result.Error == "" {
			t.Fatalf("missing incomplete-inventory error: %+v", result)
		}
		if blocks := loadProviderConfig(t, p.SyncBoatConfig); len(blocks) != 1 || blocks[0].Alias != "box_dev" {
			t.Fatalf("inventory deleted: %+v", blocks)
		}
		if store.Host("box_dev").Notes != "keep me" {
			t.Fatal("metadata deleted")
		}
	}
}
