package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEntryRoundTripAndVersion9(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{\"version\":9,\"hosts\":{}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Entries()) != 0 {
		t.Fatalf("entries = %#v", store.Entries())
	}
	saved, err := store.PutEntry(Entry{
		Kind: EntryRDP, Label: "DC01", Address: "10.0.0.5", User: `CORP\alice`, Group: "Work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.User != "alice" || saved.Domain != "CORP" || saved.Address != "10.0.0.5" || saved.Port != "" {
		t.Fatalf("saved = %+v", saved)
	}
	if _, err := store.PutEntry(Entry{Kind: EntryRDP, Label: "dc01", Address: "10.0.0.6"}); err == nil {
		t.Fatal("duplicate label was stored")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.EntryByLabel(EntryRDP, "DC01")
	if err != nil || got.ID != saved.ID || got.Domain != "CORP" {
		t.Fatalf("reopened = %+v err=%v", got, err)
	}
	if err := reopened.SaveGroupEdit("Work", "Office", map[string]bool{}, nil, false, GroupAppearance{}); err != nil {
		t.Fatal(err)
	}
	moved, _ := reopened.Entry(saved.ID)
	if moved.Group != "Office" {
		t.Fatalf("group = %q", moved.Group)
	}
}

func TestWebEntryRejectsUserinfo(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutEntry(Entry{Kind: EntryWeb, Label: "Firewall", URL: "https://root:secret@192.168.0.1/"}); err == nil {
		t.Fatal("userinfo was stored")
	}
	if _, err := store.PutEntry(Entry{Kind: EntryWeb, Label: "Firewall", URL: "https://192.168.0.1/"}); err != nil {
		t.Fatal(err)
	}
}
