package sync

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	boatcloud "bast/internal/cloud/boat"
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

func TestBoatConflictBlocksMutationsWithoutChangingState(t *testing.T) {
	engine, p, store := testEngine(t)
	legacy := filepath.Join(p.ManagedDir, "sync", "box", "config")
	for path, alias := range map[string]string{legacy: "box_old", p.SyncBoatConfig: "boat_new"} {
		if err := engine.Config.EnsureSyncInclude(path); err != nil {
			t.Fatal(err)
		}
		if err := sshconfig.WriteSyncConfig(path, []sshconfig.SyncHostInput{{Alias: alias, SyncSource: "boat", SyncID: "bx_" + alias, HostName: "203.0.113.4"}}); err != nil {
			t.Fatal(err)
		}
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
	engine.Boat.Run = func(context.Context, []string, []string) ([]byte, error) {
		t.Error("Boat CLI was called during an inventory conflict")
		return nil, errors.New("unexpected provider call")
	}
	ctx := context.Background()
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{"sync", func() error { _, err := engine.SyncBoat(ctx); return err }},
		{"auto-connect", func() error {
			_, ran, err := engine.MaybeAutoConnectBoat(ctx)
			if ran {
				t.Error("auto-connect ran")
			}
			return err
		}},
		{"new", func() error { _, _, err := engine.NewBoat(ctx, boatcloud.NewOpts{}); return err }},
		{"fork", func() error { _, _, err := engine.ForkBoat(ctx, "bx_box_old", boatcloud.ForkOpts{}); return err }},
		{"stop", func() error { _, err := engine.StopBoat(ctx, "bx_box_old"); return err }},
		{"resume", func() error { _, err := engine.ResumeBoat(ctx, "bx_box_old", boatcloud.ResumeOpts{}); return err }},
		{"delete", func() error { _, err := engine.DeleteBoat(ctx, "bx_box_old"); return err }},
		{"delete snapshot", func() error { return engine.DeleteBoatSnapshot(ctx, "snap_old") }},
		{"remove named snapshot", func() error { return engine.RemoveBoatNamedSnapshot(ctx, "base") }},
		{"access", func() error {
			return engine.EnsureBoatAccess(ctx, sshconfig.Host{Alias: "box_old", Synced: true, SyncSource: "boat", SyncID: "bx_box_old"}, nil)
		}},
		{"disable", func() error { return engine.DisableBoat(ctx) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var conflict *sshconfig.BoatMigrationConflict
			if err := test.run(); !errors.As(err, &conflict) {
				t.Fatalf("expected migration conflict, got %v", err)
			}
			for path, original := range before {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(original, after) {
					t.Fatalf("changed %s during conflict: %v", path, err)
				}
			}
		})
	}
	// An unrelated provider remains usable and cannot delete Boat metadata.
	if err := engine.DisableAWS(ctx); err != nil {
		t.Fatal(err)
	}
	if store.Host("box_old").Notes != "preserve box_old" || store.Host("boat_new").Notes != "preserve boat_new" {
		t.Fatal("unrelated disable changed Boat metadata")
	}
}
