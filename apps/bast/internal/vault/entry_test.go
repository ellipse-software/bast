package vault

import (
	"testing"

	"bast/internal/metadata"
)

func TestMergeEntriesLastWriteAndTombstone(t *testing.T) {
	local := Document{Entries: []DesktopEntry{{
		Entry: metadata.Entry{ID: "a", Kind: metadata.EntryRDP, Label: "DC01", Address: "10.0.0.5", UpdatedAt: 10},
	}, {
		Entry: metadata.Entry{ID: "b", Kind: metadata.EntryWeb, Label: "Firewall", URL: "https://192.168.0.1/", UpdatedAt: 5},
	}}}
	remote := Document{
		Entries: []DesktopEntry{{
			Entry: metadata.Entry{ID: "a", Kind: metadata.EntryRDP, Label: "DC01", Address: "10.0.0.8", UpdatedAt: 20},
		}},
		Tombstones: Tombstones{Entries: map[string]int64{"b": 9}},
	}
	merged := Merge(local, remote, MergeModeMerge)
	if len(merged.Document.Entries) != 1 || merged.Document.Entries[0].Address != "10.0.0.8" {
		t.Fatalf("entries = %+v", merged.Document.Entries)
	}
	remote.Tombstones.Entries["a"] = 30
	merged = Merge(local, remote, MergeModeMerge)
	if len(merged.Document.Entries) != 0 {
		t.Fatalf("tombstone did not win: %+v", merged.Document.Entries)
	}
}
