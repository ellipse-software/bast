package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "bast", "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetHost("prod", Host{Label: "Production web", Favorite: true, Hidden: true, Tags: []string{"web", "web", "prod"}, Group: "work"}); err != nil {
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
	if host.Label != "Production web" || !host.Favorite || !host.Hidden || host.ConnectionCount != 1 || len(host.Tags) != 2 || reopened.Preferences().Sort != "group" {
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

func TestStoreSerializesConcurrentWrites(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}

	const count = 20
	errs := make(chan error, count)
	var writes sync.WaitGroup
	for i := range count {
		writes.Add(1)
		go func() {
			defer writes.Done()
			alias := fmt.Sprintf("host-%d", i)
			errs <- store.SetHost(alias, Host{Label: alias})
		}()
	}
	writes.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if hosts := store.Hosts(); len(hosts) != count {
		t.Fatalf("hosts = %d, want %d", len(hosts), count)
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

func TestOpenRenamesLegacyGCPGroups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	raw := []byte(`{"version":3,"hosts":{"web":{"group":"GCP/Production"}},"preferences":{}}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Host("web").Group; got != "Google Cloud/Production" {
		t.Fatalf("group = %q", got)
	}
}
