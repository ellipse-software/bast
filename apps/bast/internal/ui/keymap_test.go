package ui

import (
	"strings"
	"testing"

	keymodel "bast/internal/keys"
)

func TestCatalogKeysAreUniquePerScope(t *testing.T) {
	seen := map[Scope]map[string]Action{}
	for _, b := range catalog() {
		if b.HelpOnly || b.SkipMatch || len(b.Keys) == 0 {
			continue
		}
		if seen[b.Scope] == nil {
			seen[b.Scope] = map[string]Action{}
		}
		for _, key := range b.Keys {
			if prev, ok := seen[b.Scope][key]; ok && prev != b.ID && b.When == nil {
				if other := bindingWhenNil(b.Scope, key); other != b.ID && other != 0 {
					t.Fatalf("scope %d key %q bound to %d and %d", b.Scope, key, other, b.ID)
				}
			}
			if b.When == nil {
				seen[b.Scope][key] = b.ID
			}
		}
	}
}

func bindingWhenNil(scope Scope, key string) Action {
	for _, b := range catalog() {
		if b.HelpOnly || b.SkipMatch || b.Scope != scope || b.When != nil {
			continue
		}
		if keyIn(b.Keys, key) {
			return b.ID
		}
	}
	return 0
}

func TestMatchableBindingsDoNotStealKeys(t *testing.T) {
	type keyScope struct {
		scope Scope
		key   string
	}
	first := map[keyScope]Action{}
	for _, b := range catalog() {
		if b.HelpOnly || b.SkipMatch {
			continue
		}
		for _, key := range b.Keys {
			id := keyScope{b.Scope, key}
			if prev, ok := first[id]; ok && prev != b.ID {
				t.Fatalf("scope %d key %q can resolve to %d or %d", b.Scope, key, prev, b.ID)
			}
			if _, ok := first[id]; !ok {
				first[id] = b.ID
			}
		}
	}
}

func TestHelpUpScrollsUpNotDown(t *testing.T) {
	m := testApp(t)
	m.help = true
	if b, ok := m.matchBinding("up"); !ok || b.ID != ActionHelpScrollUp {
		t.Fatalf("help up = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("down"); !ok || b.ID != ActionHelpScrollDown {
		t.Fatalf("help down = %+v ok=%v", b, ok)
	}
}

func TestSyncLMovesRight(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	if b, ok := m.matchBinding("l"); !ok || b.ID != ActionMoveRight {
		t.Fatalf("sync l = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("h"); !ok || b.ID != ActionMoveLeft {
		t.Fatalf("sync h = %+v ok=%v", b, ok)
	}
}

func TestKeysRemapGenerateAndInstall(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub", PrivatePath: "/tmp/work"}}

	if b, ok := m.matchBinding("g"); !ok || b.ID != ActionGenerateKey {
		t.Fatalf("keys g = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("a"); !ok || b.ID != ActionInstallKey {
		t.Fatalf("keys a = %+v ok=%v", b, ok)
	}
	if _, ok := m.matchBinding("u"); ok {
		t.Fatal("keys u should be unbound")
	}

	m.section = hostsSection
	if b, ok := m.matchBinding("g"); !ok || b.ID != ActionJumpTop {
		t.Fatalf("hosts g = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("a"); !ok || b.ID != ActionAddHost {
		t.Fatalf("hosts a = %+v ok=%v", b, ok)
	}
}

func TestHelpOnKeysOmitsShadowedJumpTop(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	var chords []string
	for _, group := range m.helpGroups() {
		for _, row := range group.rows {
			chords = append(chords, row.chord+" "+row.name)
		}
	}
	joined := strings.Join(chords, "\n")
	if !strings.Contains(joined, "Generate") {
		t.Fatalf("keys help missing generate:\n%s", joined)
	}
	if !strings.Contains(joined, "Add to server") {
		t.Fatalf("keys help missing add to server:\n%s", joined)
	}
	if strings.Contains(joined, "g  Top") || strings.Contains(joined, "g Top") {
		t.Fatalf("keys help still lists g as top:\n%s", joined)
	}
	if !strings.Contains(joined, "Home") {
		t.Fatalf("keys help should keep Home for top:\n%s", joined)
	}
}

func TestKeysGOpensGenerateAndAInstalls(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub", PrivatePath: "/tmp/work"}}
	m.Update(press("g"))
	if m.form == nil || m.form.action != "key_generate" {
		t.Fatalf("g should generate: %+v", m.form)
	}
	m.form = nil
	m.Update(press("a"))
	if m.form == nil || m.form.action != "key_install" {
		t.Fatalf("a should install: %+v", m.form)
	}
	m.form = nil
	m.Update(press("u"))
	if m.form != nil {
		t.Fatal("u should not install")
	}
}

func TestHelpGDoesNotGenerate(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.help = true
	m.helpOffset = 3
	if b, ok := m.matchBinding("g"); !ok || b.ID != ActionHelpTop {
		t.Fatalf("help g = %+v ok=%v", b, ok)
	}
	m.Update(press("g"))
	if m.form != nil {
		t.Fatal("g in help opened a form")
	}
	if m.helpOffset != 0 {
		t.Fatalf("help g should jump to top: offset=%d", m.helpOffset)
	}
}
