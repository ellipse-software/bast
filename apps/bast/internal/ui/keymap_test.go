package ui

import (
	"strings"
	"testing"

	keymodel "bast/internal/keys"
)

func TestCatalogKeysAreUniquePerScope(t *testing.T) {
	seen := map[Scope]map[string]Action{}
	for _, b := range catalog() {
		if len(b.Keys) == 0 || b.Available != nil {
			continue
		}
		if seen[b.Scope] == nil {
			seen[b.Scope] = map[string]Action{}
		}
		for _, key := range b.Keys {
			if prev, ok := seen[b.Scope][key]; ok && prev != b.ID {
				t.Fatalf("scope %d key %q bound to %d and %d", b.Scope, key, prev, b.ID)
			}
			seen[b.Scope][key] = b.ID
		}
	}
}

func TestMatchableBindingsDoNotStealKeys(t *testing.T) {
	type keyScope struct {
		scope Scope
		key   string
	}
	first := map[keyScope]Action{}
	for _, b := range catalog() {
		for _, key := range b.Keys {
			id := keyScope{b.Scope, key}
			if prev, ok := first[id]; ok && prev != b.ID && b.Available == nil {
				t.Fatalf("scope %d key %q can resolve to %d or %d", b.Scope, key, prev, b.ID)
			}
			if _, ok := first[id]; !ok {
				first[id] = b.ID
			}
		}
	}
}

func TestFilesPreviewAndOpenDirBindings(t *testing.T) {
	m := testApp(t)
	m.section = filesSection
	m.initFilesState()
	m.files.panes[m.files.focus].kind = filesPaneLocal
	if b, ok := m.matchBinding("o"); !ok || b.ID != ActionFilesPreview {
		t.Fatalf("files o = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("l"); !ok || b.ID != ActionFilesOpenDir {
		t.Fatalf("files l = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("enter"); !ok || b.ID != ActionFilesEnter {
		t.Fatalf("files enter = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("right"); !ok || b.ID != ActionFilesOpenDir {
		t.Fatalf("files right = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("left"); !ok || b.ID != ActionFilesParent {
		t.Fatalf("files left = %+v ok=%v", b, ok)
	}
}

func TestDoctorKeyDoesNotStealFilesDisconnect(t *testing.T) {
	m := testApp(t)
	if b, ok := m.matchBinding("D"); !ok || b.ID != ActionDoctorOpen {
		t.Fatalf("hosts D = %+v ok=%v", b, ok)
	}
	m.section = filesSection
	if b, ok := m.matchBinding("D"); !ok || b.ID != ActionFilesDisconnect {
		t.Fatalf("files D = %+v ok=%v", b, ok)
	}
	m.section = hostsSection
	m.doctor = true
	if b, ok := m.matchBinding("esc"); !ok || b.ID != ActionDoctorClose {
		t.Fatalf("doctor esc = %+v ok=%v", b, ok)
	}
}

func TestSyncLMovesRight(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "boat"
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

	if b, ok := m.matchBinding("n"); !ok || b.ID != ActionGenerateKey {
		t.Fatalf("keys n = %+v ok=%v", b, ok)
	}
	if b, ok := m.matchBinding("g"); !ok || b.ID != ActionJumpTop {
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

func TestKeysSheetKeepsJumpAndGenerate(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub", PrivatePath: "/tmp/work"}}
	joined := strings.Join(m.paletteContent(m.styles(), 120), "\n")
	if !strings.Contains(joined, "generate") {
		t.Fatalf("keys sheet missing generate:\n%s", joined)
	}
	if !strings.Contains(joined, "add") {
		t.Fatalf("keys sheet missing add:\n%s", joined)
	}
	if !strings.Contains(joined, "g") || !strings.Contains(joined, "top") {
		t.Fatalf("keys sheet should keep jump to top:\n%s", joined)
	}
}

func TestKeysGOpensGenerateAndAInstalls(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub", PrivatePath: "/tmp/work"}}
	m.Update(press("n"))
	if m.form == nil || m.form.action != "key_generate" {
		t.Fatalf("n should generate: %+v", m.form)
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

func TestSlashTypesDuringSearch(t *testing.T) {
	m := testApp(t)
	m.Update(press("ctrl+f"))
	if !strings.HasPrefix(m.search, "\x00") {
		t.Fatal("ctrl+f should start the filter")
	}
	m.Update(press("/"))
	if m.palette || m.search != "\x00/" {
		t.Fatalf("slash should type into the filter: palette=%v search=%q", m.palette, m.search)
	}
	m.Update(press("?"))
	if !m.palette {
		t.Fatal("? should open the sheet while typing")
	}
}

func TestSheetKeyRunsAndCloses(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub", PrivatePath: "/tmp/work"}}
	m.Update(press("/"))
	if !m.palette {
		t.Fatal("/ should open the sheet")
	}
	m.Update(press("n"))
	if m.palette {
		t.Fatal("running a key should close the sheet")
	}
	if m.form == nil || m.form.action != "key_generate" {
		t.Fatalf("n from the sheet should generate: %+v", m.form)
	}
}
