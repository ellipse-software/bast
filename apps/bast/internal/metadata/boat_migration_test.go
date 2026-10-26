package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyBoatStateMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	before := []byte(`{"version":7,"hosts":{"box_dev":{"label":"Dev","group":"Box/Stopped","notes":"keep me","connectionCount":4}},"preferences":{"collapsedGroups":["Box","Box/Stopped","Boat","Boxing"]},"integrations":{"box":{"enabled":false,"disabled":true,"lastInstanceCount":2,"lastSyncAt":"2026-09-15T12:00:00Z","lastSyncError":"offline"},"upstash":{"enabled":true}}}`)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	boat := store.Boat()
	if boat.Enabled || !boat.Disabled || boat.LastInstanceCount != 2 || boat.LastSyncAt == nil || boat.LastSyncError != "offline" {
		t.Fatalf("lost legacy preferences: %+v", boat)
	}
	host := store.Host("box_dev")
	if host.Group != "Boat/Stopped" || host.Label != "Dev" || host.Notes != "keep me" || host.ConnectionCount != 4 {
		t.Fatalf("host changed: %+v", host)
	}
	if got := store.Preferences().CollapsedGroups; !reflect.DeepEqual(got, []string{"Boat", "Boat/Stopped", "Boxing"}) {
		t.Fatalf("collapsed groups: %v", got)
	}
	if !store.Upstash().Enabled {
		t.Fatal("Upstash preference lost")
	}
	if err := store.SetBoat(boat); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Version      int                        `json:"version"`
		Integrations map[string]json.RawMessage `json:"integrations"`
	}
	if err := json.Unmarshal(persisted, &state); err != nil {
		t.Fatal(err)
	}
	if state.Version != CurrentVersion || state.Integrations["boat"] == nil || state.Integrations["box"] != nil {
		t.Fatalf("migration not persisted: %s", persisted)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.Boat(), boat) || !reflect.DeepEqual(reopened.Host("box_dev"), host) {
		t.Fatal("migration was not idempotent")
	}
}

func TestBoatPreferencesTakePrecedenceOverLegacyBox(t *testing.T) {
	var integrations Integrations
	if err := json.Unmarshal([]byte(`{"box":{"enabled":true,"autoSync":true},"boat":{"enabled":false,"disabled":true}}`), &integrations); err != nil {
		t.Fatal(err)
	}
	if integrations.Boat == nil || integrations.Boat.Enabled || integrations.Boat.AutoSync || !integrations.Boat.Disabled {
		t.Fatalf("Boat settings overridden: %+v", integrations.Boat)
	}
}
