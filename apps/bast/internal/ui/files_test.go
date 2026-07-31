package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bast/internal/files"
)

func TestFilesTabNavigationAndMarks(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	_ = m.enterFilesSection()
	if m.section != filesSection {
		t.Fatalf("section = %v", m.section)
	}
	m.files.panes[0].cwd = dir
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(filesListMsg{pane: 0, cwd: dir, entries: entries})

	body := m.renderFiles(m.styles())
	if !strings.Contains(body, "local") || !strings.Contains(body, "remote") {
		t.Fatalf("files body:\n%s", body)
	}
	if !strings.Contains(body, "a.txt") || !strings.Contains(body, "sub/") {
		t.Fatalf("expected local entries:\n%s", body)
	}

	m.updateFilesKeys("space")
	if len(m.files.panes[0].marked) != 1 {
		t.Fatalf("marked = %v", m.files.panes[0].marked)
	}
	m.updateFilesKeys("tab")
	if m.files.focus != 1 {
		t.Fatal("expected right focus")
	}
	m.updateFilesKeys("tab")
	if m.files.focus != 0 {
		t.Fatal("expected left focus")
	}

	m.Update(press("4"))
	if m.section != filesSection {
		t.Fatal("key 4 should open Files")
	}
	m.section = hostsSection
	m.cursor = 0
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'F', Text: "F"}))
	if m.section != filesSection {
		t.Fatal("F from Hosts should open Files")
	}
	if m.files.focus != 1 {
		t.Fatal("F should focus remote pane")
	}
}

func TestFilesPathEditCancel(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	m.files.panes[0].cwd = t.TempDir()
	m.updateFilesKeys("/")
	if !m.files.panes[0].pathEdit {
		t.Fatal("expected path edit")
	}
	m.updateFilesKeys("esc")
	if m.files.panes[0].pathEdit {
		t.Fatal("esc should cancel path edit")
	}
}

func TestFilesPaneKindSwitchAndSwap(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	m.updateFilesKeys("R")
	if m.files.panes[0].kind != filesPaneRemote || !m.files.panes[0].pickingHost() {
		t.Fatal("R should make focused pane a remote host picker")
	}
	m.updateFilesKeys("L")
	if m.files.panes[0].kind != filesPaneLocal {
		t.Fatal("L should make focused pane local")
	}
	m.updateFilesKeys("tab")
	m.updateFilesKeys("L")
	if m.files.panes[1].kind != filesPaneLocal {
		t.Fatal("right pane should become local")
	}
	leftKind := m.files.panes[0].kind
	m.updateFilesKeys("w")
	if m.files.panes[1].kind != leftKind {
		t.Fatal("w should swap panes")
	}
}

func TestFilesJumpAllowsTypingFullName(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	for _, name := range []string{"bast", "backup", "other"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = m.enterFilesSection()
	m.files.panes[0].cwd = dir
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(filesListMsg{pane: 0, cwd: dir, entries: entries})

	m.updateFilesKeys("f")
	m.updateFilesKeys("b")
	// After "b", both bast and backup match. Continuations include a,s,t,c,k,u,p…
	// Labels must not use those, so typing "a" continues the query toward "bast".
	if _, labeledA := m.files.jump.labels["a"]; labeledA {
		t.Fatal("label must not steal continuation key a")
	}
	m.updateFilesKeys("a")
	if !m.files.jump.active {
		t.Fatal("typing a should continue search, not jump")
	}
	if m.files.jump.query != "ba" {
		t.Fatalf("query = %q", m.files.jump.query)
	}
	m.updateFilesKeys("s")
	m.updateFilesKeys("t")
	if m.files.jump.active {
		t.Fatal("unique match bast should auto-jump")
	}
	got := m.files.panes[0].entries[m.files.panes[0].cursor].Name
	if got != "bast" {
		t.Fatalf("cursor on %q, want bast", got)
	}
}

func TestFilesJumpAutoJumpsSingleMatch(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	for _, name := range []string{"unique-only", "zzz"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = m.enterFilesSection()
	m.files.panes[0].cwd = dir
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(filesListMsg{pane: 0, cwd: dir, entries: entries})

	m.updateFilesKeys("f")
	m.updateFilesKeys("u")
	if m.files.jump.active {
		t.Fatal("single match should auto-jump")
	}
	if m.files.panes[0].entries[m.files.panes[0].cursor].Name != "unique-only" {
		t.Fatalf("landed on %q", m.files.panes[0].entries[m.files.panes[0].cursor].Name)
	}
}

func TestFilesJumpLabelStillJumps(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	for _, name := range []string{"alpha", "alpine", "other"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = m.enterFilesSection()
	m.files.panes[0].cwd = dir
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(filesListMsg{pane: 0, cwd: dir, entries: entries})

	m.updateFilesKeys("f")
	m.updateFilesKeys("a")
	m.updateFilesKeys("l")
	if len(m.files.jump.labels) == 0 {
		t.Fatal("expected disambiguation labels")
	}
	var label string
	var target int
	for lab, idx := range m.files.jump.labels {
		label, target = lab, idx
		break
	}
	m.updateFilesKeys(label)
	if m.files.jump.active {
		t.Fatal("label should jump")
	}
	if m.files.panes[0].cursor != target {
		t.Fatalf("cursor = %d, want %d", m.files.panes[0].cursor, target)
	}
}

func TestFilesLEntersDirectory(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = m.enterFilesSection()
	m.files.panes[0].cwd = dir
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(filesListMsg{pane: 0, cwd: dir, entries: entries})
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "nested" {
			m.files.panes[0].cursor = i
			break
		}
	}
	m.updateFilesKeys("l")
	if m.files.panes[0].cwd != sub {
		t.Fatalf("l should enter directory, cwd=%q", m.files.panes[0].cwd)
	}
}
