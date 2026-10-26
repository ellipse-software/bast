package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

func TestStoreRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "bast", "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetHost("prod", Host{Label: "Production web", Favorite: true, Hidden: true, Tags: []string{" web ", "web", "prod"}, Group: "work"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUse("prod"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSort("group"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCollapsedGroups([]string{"Work", "Personal", "Work", " "}); err != nil {
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
	if got := reopened.Preferences().CollapsedGroups; !reflect.DeepEqual(got, []string{"Personal", "Work"}) {
		t.Fatalf("collapsed groups = %v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestHistoryImportRoundTripAndAcceptance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	history := HistoryImport{
		Sources: map[string]HistorySource{"/home/test/.zsh_history": {Offset: 42, TailHash: "tail", Anchors: []string{"one", "two"}}},
		Pending: []HistorySuggestion{{ID: "one", Alias: "dev-example.com", HostName: "example.com", User: "dev", Source: "zsh"}},
	}
	if err := store.SetHistoryImport(history); err != nil {
		t.Fatal(err)
	}
	history.Pending[0].Alias = "mutated"
	history.Sources["/home/test/.zsh_history"] = HistorySource{}
	if got := store.HistoryImport(); got.Pending[0].Alias != "dev-example.com" || got.Sources["/home/test/.zsh_history"].Offset != 42 {
		t.Fatalf("store retained caller-owned state: %+v", got)
	}
	if err := store.AcceptHistorySuggestion("dev-example.com", Host{Label: "Development"}, "one"); err != nil {
		t.Fatal(err)
	}
	if got := store.HistoryImport(); len(got.Pending) != 0 {
		t.Fatalf("accepted suggestion remained pending: %+v", got.Pending)
	}
	if got := store.Host("dev-example.com").Label; got != "Development" {
		t.Fatalf("accepted host label = %q", got)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.HistoryImport(); len(got.Pending) != 0 || got.Sources["/home/test/.zsh_history"].TailHash != "tail" {
		t.Fatalf("reopened history = %+v", got)
	}
}

func TestDismissHistorySuggestionLeavesOtherPendingEntries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetHistoryImport(HistoryImport{Pending: []HistorySuggestion{{ID: "one"}, {ID: "two"}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.DismissHistorySuggestion("one"); err != nil {
		t.Fatal(err)
	}
	if got := store.HistoryImport().Pending; len(got) != 1 || got[0].ID != "two" {
		t.Fatalf("pending = %+v", got)
	}
}

func TestHistoryScanCommitDoesNotResurrectDismissedSuggestion(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	original := HistoryImport{Pending: []HistorySuggestion{{ID: "one"}}}
	if err := store.SetHistoryImport(original); err != nil {
		t.Fatal(err)
	}
	_, revision := store.HistoryImportSnapshot()
	if err := store.DismissHistorySuggestion("one"); err != nil {
		t.Fatal(err)
	}
	committed, ok, err := store.CommitHistoryImport(revision, original)
	if err != nil {
		t.Fatal(err)
	}
	if ok || len(committed.Pending) != 0 || len(store.HistoryImport().Pending) != 0 {
		t.Fatalf("stale scan committed: ok=%v state=%+v", ok, committed)
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

func TestUpdateHostsPersistsOneConsistentRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetHost("old", Host{Favorite: true, Tags: []string{"web"}}); err != nil {
		t.Fatal(err)
	}
	before := store.HostRevision()
	if err := store.UpdateHosts(func(hosts map[string]Host) {
		renamed := hosts["old"]
		delete(hosts, "old")
		renamed.Label = "Production"
		hosts["new"] = renamed
		hosts["worker"] = Host{Tags: []string{"queue", "queue"}}
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.HostRevision(); got != before+1 {
		t.Fatalf("revision = %d, want %d", got, before+1)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if old := reopened.Host("old"); old.Label != "" || old.Favorite || old.Group != "" || len(old.Tags) != 0 {
		t.Fatalf("old metadata remains: %+v", old)
	}
	renamed := reopened.Host("new")
	if renamed.Label != "Production" || !renamed.Favorite || len(renamed.Tags) != 1 {
		t.Fatalf("renamed metadata = %+v", renamed)
	}
	if tags := reopened.Host("worker").Tags; len(tags) != 1 || tags[0] != "queue" {
		t.Fatalf("worker tags = %v", tags)
	}
}

func TestFailedHostMutationRestoresStateAndRevision(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Store) error
	}{
		{name: "set", mutate: func(store *Store) error {
			return store.SetHost("source", Host{Label: "changed"})
		}},
		{name: "move", mutate: func(store *Store) error {
			return store.MoveHost("source", "Moved", []string{"Moved"})
		}},
		{name: "delete", mutate: func(store *Store) error {
			return store.DeleteHost("source")
		}},
		{name: "delete with tombstone", mutate: func(store *Store) error {
			return store.DeleteHostWithTombstone("source", "src-id")
		}},
		{name: "rename", mutate: func(store *Store) error {
			return store.RenameHost("source", "destination")
		}},
		{name: "favorite", mutate: func(store *Store) error {
			_, err := store.ToggleFavorite("source")
			return err
		}},
		{name: "hidden", mutate: func(store *Store) error {
			_, err := store.ToggleHidden("source")
			return err
		}},
		{name: "record use", mutate: func(store *Store) error {
			return store.RecordUse("source")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := Open(filepath.Join(root, "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateHosts(func(hosts map[string]Host) {
				hosts["source"] = Host{Label: "Source", Favorite: true, Tags: []string{"one"}}
				hosts["destination"] = Host{Label: "Destination", Hidden: true, Tags: []string{"two"}}
			}); err != nil {
				t.Fatal(err)
			}
			beforeHosts, beforeRevision := store.HostsSnapshot()
			beforePreferences := store.Preferences()
			beforeTombs := store.VaultTombstones()
			blockedPath := filepath.Join(root, "blocked")
			if err := os.Mkdir(blockedPath, 0700); err != nil {
				t.Fatal(err)
			}
			store.path = blockedPath

			if err := test.mutate(store); err == nil {
				t.Fatal("mutation unexpectedly succeeded")
			}
			afterHosts, afterRevision := store.HostsSnapshot()
			if !reflect.DeepEqual(afterHosts, beforeHosts) {
				t.Fatalf("hosts changed after failed save: before=%+v after=%+v", beforeHosts, afterHosts)
			}
			if afterRevision != beforeRevision {
				t.Fatalf("revision changed after failed save: before=%d after=%d", beforeRevision, afterRevision)
			}
			if afterPreferences := store.Preferences(); !reflect.DeepEqual(afterPreferences, beforePreferences) {
				t.Fatalf("preferences changed after failed save: before=%+v after=%+v", beforePreferences, afterPreferences)
			}
			if afterTombs := store.VaultTombstones(); !reflect.DeepEqual(afterTombs, beforeTombs) {
				t.Fatalf("vault tombstones changed after failed save: before=%+v after=%+v", beforeTombs, afterTombs)
			}
		})
	}
}

func TestFailedPreferenceAndIntegrationMutationsRestoreState(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSort("smart"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetGCP(GCPIntegration{Enabled: true, DefaultSSHUser: "before"}); err != nil {
		t.Fatal(err)
	}
	blockedPath := filepath.Join(root, "blocked")
	if err := os.Mkdir(blockedPath, 0700); err != nil {
		t.Fatal(err)
	}
	store.path = blockedPath

	if err := store.SetSort("group"); err == nil {
		t.Fatal("sort mutation unexpectedly succeeded")
	}
	if got := store.Preferences().Sort; got != "smart" {
		t.Fatalf("sort changed after failed save: %q", got)
	}
	if err := store.SetGCP(GCPIntegration{Enabled: true, DefaultSSHUser: "after"}); err == nil {
		t.Fatal("GCP mutation unexpectedly succeeded")
	}
	if got := store.GCP().DefaultSSHUser; got != "before" {
		t.Fatalf("GCP state changed after failed save: %q", got)
	}
}

func TestVaultTombstoneRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetHost("prod", Host{Label: "Production"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteHostWithTombstone("prod", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordKeyTombstone("fp1"); err != nil {
		t.Fatal(err)
	}
	if got := store.Host("prod"); got.Label != "" {
		t.Fatalf("host metadata remained: %+v", got)
	}
	tombs := store.VaultTombstones()
	if tombs.Hosts["abc123"] == 0 || tombs.Keys["fp1"] == 0 {
		t.Fatalf("tombstones = %+v", tombs)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.VaultTombstones()
	if got.Hosts["abc123"] != tombs.Hosts["abc123"] || got.Keys["fp1"] != tombs.Keys["fp1"] {
		t.Fatalf("reopened tombstones = %+v want %+v", got, tombs)
	}
	if VaultKeyTombstoneID("fp", "work") != "fp" {
		t.Fatalf("fingerprint id = %q", VaultKeyTombstoneID("fp", "work"))
	}
	if VaultKeyTombstoneID("", "work") != "name:work" {
		t.Fatalf("name id = %q", VaultKeyTombstoneID("", "work"))
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

func TestOpenRenamesLegacyAWSGroups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	raw := []byte(`{"version":4,"hosts":{"web":{"group":"AWS/default/eu-west-2"},"root":{"group":"AWS"}},"preferences":{}}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Host("web").Group; got != "Amazon EC2/default/eu-west-2" {
		t.Fatalf("group = %q", got)
	}
	if got := store.Host("root").Group; got != "Amazon EC2" {
		t.Fatalf("root group = %q", got)
	}
}

func TestFreshStoreIsEligibleForOnboarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.ShouldOnboard() || !store.Onboarding().Eligible {
		t.Fatal("missing state file should be eligible for onboarding")
	}

	if err := store.SetHistoryImport(HistoryImport{Pending: []HistorySuggestion{{ID: "one"}}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.ShouldOnboard() {
		t.Fatal("history scan save should keep first-run eligibility")
	}

	if err := reopened.DismissOnboarding(); err != nil {
		t.Fatal(err)
	}
	if reopened.ShouldOnboard() || reopened.Onboarding().DismissedAt == nil {
		t.Fatalf("dismissed = %+v", reopened.Onboarding())
	}
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.ShouldOnboard() {
		t.Fatal("dismissed onboarding should not return")
	}
}

func TestExistingStateIsNotEligibleForOnboarding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version":7,"hosts":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.ShouldOnboard() || store.Onboarding().Eligible {
		t.Fatal("existing state from older Bast should not show onboarding")
	}
}

func TestNoOnboardingEnvDisablesCensus(t *testing.T) {
	t.Setenv("BAST_NO_ONBOARDING", "1")
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if store.ShouldOnboard() {
		t.Fatal("BAST_NO_ONBOARDING should skip the census")
	}
}
