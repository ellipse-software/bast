package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "bast", "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetHost("prod", Host{Favorite: true, Hidden: true, Tags: []string{"web", "web", "prod"}, Group: "work"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUse("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSort("group"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	host := reopened.Host("prod")
	if !host.Favorite || !host.Hidden || host.ConnectionCount != 1 || len(host.Tags) != 2 || reopened.Preferences().Sort != "group" {
		t.Fatalf("unexpected state: %+v %+v", host, reopened.Preferences())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestStoreRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"hosts":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("expected newer schema error")
	}
}
