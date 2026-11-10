package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeIconAndColor(t *testing.T) {
	iconTests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "", want: ""},
		{raw: "  ", want: ""},
		{raw: "🇩🇪", want: "🇩🇪", ok: true},
		{raw: "🇩🇪 ", want: "🇩🇪", ok: true},
		{raw: "A", want: "A", ok: true},
		{raw: "ab", ok: false},
		{raw: "a\nb", ok: false},
	}
	for _, test := range iconTests {
		got, err := NormalizeIcon(test.raw)
		if test.ok || test.raw == "" || test.raw == "  " {
			if err != nil || got != test.want {
				t.Fatalf("NormalizeIcon(%q) = %q, %v", test.raw, got, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("NormalizeIcon(%q) = %q, want error", test.raw, got)
		}
	}

	colorTests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "", want: ""},
		{raw: "Red", want: "red", ok: true},
		{raw: "GRAY", want: "gray", ok: true},
		{raw: "#abc", want: "#AABBCC", ok: true},
		{raw: "#aabbcc", want: "#AABBCC", ok: true},
		{raw: "nope", ok: false},
		{raw: "#12", ok: false},
		{raw: "#zzzzzz", ok: false},
	}
	for _, test := range colorTests {
		got, err := NormalizeColor(test.raw)
		if (test.ok || test.raw == "") && (err != nil || got != test.want) {
			t.Fatalf("NormalizeColor(%q) = %q, %v", test.raw, got, err)
		}
		if !test.ok && test.raw != "" && err == nil {
			t.Fatalf("NormalizeColor(%q) = %q, want error", test.raw, got)
		}
	}
	if code, ok := ANSICode("gray"); !ok || code != 8 {
		t.Fatalf("gray code = %d, %v", code, ok)
	}
	if _, ok := ANSICode("black"); ok {
		t.Fatal("black is not in the palette")
	}
}

func TestGroupAppearanceRoundTripRenameAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version":8,"hosts":{"alpha":{"group":"Work/EU"},"beta":{"group":"Other"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"version":8`) && !strings.Contains(string(raw), `"version": 8`) {
		t.Fatalf("opening v8 should not rewrite the file: %s", raw)
	}

	aliases := map[string]bool{"alpha": true, "beta": true}
	if err := store.SetGroups(map[string]GroupAppearance{
		"Work":    {Icon: "🇩🇪", Color: "Blue"},
		"Work/EU": {Icon: "🇪🇺", Color: "#abc"},
		"Other":   {Icon: "★", Color: "green"},
		"Empty":   {},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveGroupEdit("Work", "Prod", aliases, []string{"Prod"}, true, GroupAppearance{Icon: "🇩🇪", Color: "red"}); err != nil {
		t.Fatal(err)
	}
	if got := store.Host("alpha").Group; got != "Prod/EU" {
		t.Fatalf("alpha group = %q", got)
	}
	if got := store.Host("beta").Group; got != "Other" {
		t.Fatalf("beta group = %q", got)
	}
	got := store.Groups()
	want := map[string]GroupAppearance{
		"Prod":    {Icon: "🇩🇪", Color: "red"},
		"Prod/EU": {Icon: "🇪🇺", Color: "#AABBCC"},
		"Other":   {Icon: "★", Color: "green"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %#v", got)
	}
	if collapsed := store.Preferences().CollapsedGroups; !reflect.DeepEqual(collapsed, []string{"Prod"}) {
		t.Fatalf("collapsed = %#v", collapsed)
	}

	if err := store.SaveGroupEdit("Prod", "Prod", aliases, nil, false, GroupAppearance{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Groups()["Prod"]; ok {
		t.Fatalf("empty appearance should delete the key: %#v", store.Groups())
	}
	if store.Groups()["Other"].Color != "green" || store.Host("alpha").Group != "Prod/EU" {
		t.Fatalf("clearing one group changed unrelated state: %#v %q", store.Groups(), store.Host("alpha").Group)
	}
	if err := store.SaveGroupEdit("Prod", "Prod", aliases, nil, false, GroupAppearance{Color: "nope"}); err == nil {
		t.Fatal("invalid colour was stored")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state State
	if err := json.Unmarshal(saved, &state); err != nil {
		t.Fatal(err)
	}
	if state.Version != CurrentVersion {
		t.Fatalf("saved version = %d", state.Version)
	}
	if reopened.Host("alpha").Group != "Prod/EU" || reopened.Groups()["Prod/EU"].Color != "#AABBCC" {
		t.Fatalf("reopened = %#v %#v", reopened.Host("alpha"), reopened.Groups())
	}
}
