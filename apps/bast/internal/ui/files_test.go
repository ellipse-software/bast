package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bast/internal/files"
	"bast/internal/platform"
	"bast/internal/vault"
)

func applyFilesList(m *App, pane int, cwd string, entries []files.Entry) {
	m.files.panes[pane].cwd = cwd
	m.files.panes[pane].listGen++
	m.Update(filesListMsg{
		pane:    pane,
		cwd:     cwd,
		gen:     m.files.panes[pane].listGen,
		entries: entries,
	})
}

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
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)

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
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)

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
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)

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
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)

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
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
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

func TestFilesLocalPaneKeepsCwd(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	_ = m.enterFilesSection()
	m.files.panes[0].cwd = dir
	m.updateFilesKeys("L")
	if m.files.panes[0].cwd != dir {
		t.Fatalf("L on local pane should keep cwd, got %q", m.files.panes[0].cwd)
	}
}

func TestFilesPathEditAcceptsSpace(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	m.files.panes[0].cwd = t.TempDir()
	m.updateFilesKeys("/")
	m.updateFilesKeys("space")
	if !strings.Contains(m.files.panes[0].pathInput.Value(), " ") {
		t.Fatalf("path input should accept space, got %q", m.files.panes[0].pathInput.Value())
	}
}

func TestFilesConnectCancelInvalidatesGeneration(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	pane := &m.files.panes[0]
	pane.kind = filesPaneRemote
	pane.connecting = true
	pane.connectGen = 3
	cancelled := false
	pane.connectCancel = func() { cancelled = true }

	m.updateFilesKeys("esc")
	if !cancelled {
		t.Fatal("esc should cancel in-flight connect")
	}
	if pane.connecting {
		t.Fatal("esc should clear connecting state")
	}
	if pane.connectGen != 4 {
		t.Fatalf("connectGen = %d, want 4", pane.connectGen)
	}

	m.Update(filesConnectMsg{pane: 0, gen: 3, alias: "stale", session: &files.Session{}})
	if pane.session != nil {
		t.Fatal("stale successful connect should not attach")
	}
	if pane.connecting {
		t.Fatal("stale connect should not restore connecting")
	}
}

func TestFilesStaleListMsgIgnored(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	staleGen := m.files.panes[0].listGen
	m.files.panes[0].listGen++
	m.Update(filesListMsg{pane: 0, cwd: dir, gen: staleGen, entries: []files.Entry{{Name: "stale"}}})
	if len(m.files.panes[0].entries) == 1 && m.files.panes[0].entries[0].Name == "stale" {
		t.Fatal("stale list message should be ignored")
	}
}

func TestFilesChmodMenuToggleAndApply(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local POSIX permissions are hidden on Windows")
	}
	m := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.env")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "secret.env" {
			m.files.panes[0].cursor = i
			break
		}
	}

	m.updateFilesKeys("p")
	if !m.files.chmod.active {
		t.Fatal("p should open permissions menu")
	}
	if m.files.chmod.mode.Perm() != 0o600 {
		t.Fatalf("seed mode = %04o", m.files.chmod.mode.Perm())
	}
	body := m.renderFiles(m.styles())
	if !strings.Contains(body, "secret.env") {
		t.Fatalf("chmod body:\n%s", body)
	}
	if !strings.Contains(body, "Owner") || !strings.Contains(body, "[x]") {
		t.Fatalf("expected permission grid:\n%s", body)
	}

	// Owner already has read+write; move to group read and enable it.
	m.updateFilesKeys("g")
	m.updateFilesKeys("space")
	if m.files.chmod.mode.Perm()&0040 == 0 {
		t.Fatalf("group read should be on, mode=%04o", m.files.chmod.mode.Perm())
	}
	// Set other to 4 via digit.
	m.updateFilesKeys("o")
	m.updateFilesKeys("4")
	if m.files.chmod.mode.Perm()&0007 != 0004 {
		t.Fatalf("other should be r--, mode=%04o", m.files.chmod.mode.Perm())
	}

	m.updateFilesKeys("enter")
	if m.files.chmod.active {
		t.Fatal("enter should close chmod menu")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("applied mode = %04o, want 0644", info.Mode().Perm())
	}
}

func TestFilesChmodMenuCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local POSIX permissions are hidden on Windows")
	}
	m := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	m.updateFilesKeys("p")
	m.updateFilesKeys("7")
	m.updateFilesKeys("esc")
	if m.files.chmod.active {
		t.Fatal("esc should cancel")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("cancel should not change mode, got %04o", info.Mode().Perm())
	}
}

func TestFilesChmodRecursiveOptionForDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local POSIX permissions are hidden on Windows")
	}
	m := testApp(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "nested" {
			m.files.panes[0].cursor = i
			break
		}
	}
	m.updateFilesKeys("p")
	if !m.files.chmod.hasDir {
		t.Fatal("directory selection should offer recursive")
	}
	body := m.renderFiles(m.styles())
	if !strings.Contains(body, "contents") {
		t.Fatalf("expected recursive option:\n%s", body)
	}
}

func TestFilesChmodPreservesSpecialBitsOnToggle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local POSIX permissions are hidden on Windows")
	}
	m := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "suid.bin")
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "suid.bin" {
			m.files.panes[0].entries[i].Mode = 0o755 | os.ModeSetuid
			m.files.panes[0].cursor = i
			break
		}
	}

	m.updateFilesKeys("p")
	if m.files.chmod.mode&os.ModeSetuid == 0 {
		t.Fatal("seed should keep setuid")
	}
	if got := files.FormatModeOctal(m.files.chmod.mode); got != "4755" {
		t.Fatalf("octal = %q", got)
	}
	// Toggle other execute off; setuid must remain.
	m.updateFilesKeys("o")
	m.updateFilesKeys("right")
	m.updateFilesKeys("right")
	m.updateFilesKeys("space")
	if m.files.chmod.mode&os.ModeSetuid == 0 {
		t.Fatal("toggling rwx should preserve setuid")
	}
	if m.files.chmod.mode.Perm() != 0o754 {
		t.Fatalf("perm = %04o want 0754", m.files.chmod.mode.Perm())
	}
}

func TestClearFilesOverlaysOnLeave(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	m.files.info = true
	m.files.chmod = filesChmod{active: true, mode: 0o644}
	m.clearFilesOverlays()
	if m.files.info || m.files.chmod.active {
		t.Fatal("overlays should clear when leaving Files")
	}
}

func TestFilesMouseFocusSelectAndEnter(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	m.files.focus = 0

	layout := m.panelLayout()
	// Click right pane header: focus remote picker.
	m.Update(tea.MouseClickMsg(tea.Mouse{
		X: layout.listWidth + 1, Y: layout.detailTop, Button: tea.MouseLeft,
	}))
	if m.files.focus != 1 {
		t.Fatalf("focus = %d, want right pane", m.files.focus)
	}

	// Click left pane first row to select nested/.
	nestedIdx := -1
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "nested" {
			nestedIdx = i
			break
		}
	}
	if nestedIdx < 0 {
		t.Fatal("nested dir missing")
	}
	// Ensure nested is visible at a known row: put cursor there first so offset is correct.
	m.files.panes[0].cursor = nestedIdx
	m.files.panes[0].ensureVisible(max(1, layout.listHeight-2))
	row := 1 + (nestedIdx - m.files.panes[0].offset)
	m.files.focus = 1
	m.Update(tea.MouseClickMsg(tea.Mouse{
		X: 2, Y: layout.listTop + row, Button: tea.MouseLeft,
	}))
	if m.files.focus != 0 {
		t.Fatalf("focus = %d, want left", m.files.focus)
	}
	if m.files.panes[0].cursor != nestedIdx {
		t.Fatalf("cursor = %d, want %d", m.files.panes[0].cursor, nestedIdx)
	}

	// Second click on same row enters the directory.
	m.Update(tea.MouseClickMsg(tea.Mouse{
		X: 2, Y: layout.listTop + row, Button: tea.MouseLeft,
	}))
	if m.files.panes[0].cwd != sub {
		t.Fatalf("cwd = %q, want %q", m.files.panes[0].cwd, sub)
	}
}

func TestFilesMouseHostPickerSelect(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	m.files.focus = 0
	layout := m.panelLayout()
	// Click first host from the other pane: focus + select (do not connect).
	m.Update(tea.MouseClickMsg(tea.Mouse{
		X: layout.listWidth + 1, Y: layout.detailTop + 1, Button: tea.MouseLeft,
	}))
	if m.files.focus != 1 {
		t.Fatal("expected right pane focused")
	}
	if m.files.panes[1].hostCursor != 0 {
		t.Fatalf("hostCursor = %d", m.files.panes[1].hostCursor)
	}
	if m.files.panes[1].connecting {
		t.Fatal("first click from other pane should not connect")
	}
	// Click second host.
	m.Update(tea.MouseClickMsg(tea.Mouse{
		X: layout.listWidth + 1, Y: layout.detailTop + 2, Button: tea.MouseLeft,
	}))
	if m.files.panes[1].hostCursor != 1 {
		t.Fatalf("hostCursor = %d, want 1", m.files.panes[1].hostCursor)
	}
}

func TestFilesMouseWheelFocusesPaneUnderCursor(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	m.files.focus = 0
	layout := m.panelLayout()
	m.Update(tea.MouseWheelMsg(tea.Mouse{
		X: layout.listWidth + 1, Y: layout.detailTop + 1, Button: tea.MouseWheelDown,
	}))
	if m.files.focus != 1 {
		t.Fatalf("wheel should focus pane under cursor, focus=%d", m.files.focus)
	}
}

func TestFilesInfoInline(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.env")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "secret.env" {
			m.files.panes[0].cursor = i
			break
		}
	}

	m.updateFilesKeys("i")
	if !m.files.info {
		t.Fatal("i should open file info")
	}
	body := m.renderFiles(m.styles())
	if !strings.Contains(body, "secret.env") {
		t.Fatalf("expected name:\n%s", body)
	}
	if !strings.Contains(body, "file") {
		t.Fatalf("expected type:\n%s", body)
	}
	if platform.SupportsPOSIXPermissions() {
		if !strings.Contains(body, "0600") {
			t.Fatalf("expected mode:\n%s", body)
		}
	} else if strings.Contains(body, "Mode") {
		t.Fatalf("local mode should be hidden:\n%s", body)
	}
	if strings.Contains(body, "0755") && !strings.Contains(body, "Name") {
		t.Fatalf("mode column should not appear in listing while info open:\n%s", body)
	}

	m.updateFilesKeys("i")
	if m.files.info {
		t.Fatal("i should toggle info closed")
	}
}

func TestFilesEscReturnsToHosts(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	if m.section != filesSection {
		t.Fatal("expected files section")
	}
	m.updateFilesKeys("esc")
	if m.section != hostsSection {
		t.Fatalf("esc should return to hosts, got %v", m.section)
	}
}

func TestFilesTransferProgressHint(t *testing.T) {
	m := testApp(t)
	_ = m.enterFilesSection()
	m.files.transfer = filesTransfer{active: true, move: false, preparing: true}
	hint := m.filesFooterHint()
	if !strings.Contains(hint, "preparing") {
		t.Fatalf("preparing hint = %q", hint)
	}
	m.files.transfer.preparing = false
	m.files.transfer.name = "big.bin"
	m.files.transfer.done = 2
	m.files.transfer.total = 5
	m.files.transfer.bytes = 1536
	hint = m.filesFooterHint()
	if !strings.Contains(hint, "big.bin") || !strings.Contains(hint, "2/5") || !strings.Contains(hint, "1.5 KB") {
		t.Fatalf("progress hint = %q", hint)
	}
}

func TestFilesConnectErrorNotice(t *testing.T) {
	got := filesConnectErrorNotice(fmt.Errorf("sftp handshake: eof; unlock keys with ssh-add or connect once from Hosts to accept the host key"))
	if !strings.Contains(got, "ssh-add") {
		t.Fatalf("notice = %q", got)
	}
	got = filesConnectErrorNotice(fmt.Errorf("Permission denied (publickey)"))
	if !strings.Contains(got, "Auth failed") {
		t.Fatalf("auth notice = %q", got)
	}
}

func TestVaultConflictFormOpens(t *testing.T) {
	m := testApp(t)
	m.Update(vaultConflictMsg{count: 2, interactive: true})
	if m.form == nil || m.form.action != "vault_resolve" {
		t.Fatalf("expected vault_resolve form, got %#v", m.form)
	}
	if m.vaultConflict == nil || m.vaultConflict.count != 2 {
		t.Fatalf("pending conflict = %#v", m.vaultConflict)
	}
	footer := m.renderFooter(m.styles())
	if strings.Contains(footer, "please wait") {
		t.Fatalf("conflict form should not lock footer: %q", footer)
	}
}

func TestVaultBusyBlocksSyncMenu(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.beginVaultBusy("Sending code…")

	if !m.vaultBusyBlocksSync() {
		t.Fatal("expected vault busy to block the sync body")
	}
	busy := m.renderVaultBusy(m.styles())
	if !strings.Contains(busy, "Sending code…") {
		t.Fatalf("expected busy label:\n%s", busy)
	}
	if strings.Contains(busy, "Link account") {
		t.Fatalf("busy body should not include vault menu:\n%s", busy)
	}

	m.section = hostsSection
	if m.vaultBusyBlocksSync() {
		t.Fatal("hosts section should not swap to the vault busy body")
	}
	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "Sending code…") {
		t.Fatalf("hosts section should still show footer busy: %q", footer)
	}
}

func TestVaultLinkKeepsBusyThroughReload(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.beginVaultBusy("Linking vault…")

	next, cmd := m.Update(vaultPullMsg{
		changed:     true,
		interactive: true,
		notice:      "Vault linked",
		revision:    "rev1",
		session:     &vault.Session{Email: "you@example.com", Token: "t", Revision: "rev1"},
		passphrase:  "secret",
	})
	app := next.(*App)
	if app.vaultBusy != "Linking vault…" {
		t.Fatalf("expected linking busy to survive pull success, got %q", app.vaultBusy)
	}
	if !app.vaultBusyHoldLoad || !app.loading {
		t.Fatalf("expected hold+loading after link, hold=%v loading=%v", app.vaultBusyHoldLoad, app.loading)
	}
	if !app.vaultBusyBlocksSync() {
		t.Fatal("vault menu should stay blocked while hosts reload")
	}
	if cmd == nil {
		t.Fatal("expected loadCmd after link")
	}

	next, _ = app.Update(loadedMsg{hosts: app.hosts, keys: app.keys})
	app = next.(*App)
	if app.vaultBusy != "" || app.vaultBusyHoldLoad {
		t.Fatalf("busy should clear after load, busy=%q hold=%v", app.vaultBusy, app.vaultBusyHoldLoad)
	}
}

func TestVaultLinkErrorsAreInteractive(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.beginVaultBusy("Linking vault…")

	next, _ := m.Update(vaultPullMsg{
		err:         fmt.Errorf("link boom"),
		interactive: true,
	})
	app := next.(*App)
	if app.vaultBusy != "" {
		t.Fatalf("busy should clear on link error, got %q", app.vaultBusy)
	}
	if !app.statusError || !strings.Contains(app.status, "link boom") {
		t.Fatalf("expected visible link error, status=%q error=%v", app.status, app.statusError)
	}
}

func TestVaultLinkErrorKeepsSessionWhenPresent(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.beginVaultBusy("Linking vault…")

	next, _ := m.Update(vaultPullMsg{
		err:         fmt.Errorf("put failed"),
		interactive: true,
		linked:      true,
		passphrase:  "secret",
		session:     &vault.Session{Email: "you@example.com", Token: "tok", Revision: ""},
	})
	app := next.(*App)
	if !app.vaultLinked() {
		t.Fatal("expected linked after auth session is present even when link push failed")
	}
	if app.vaultSession == nil || app.vaultSession.Email != "you@example.com" {
		t.Fatalf("session = %#v", app.vaultSession)
	}
	if app.vaultPassphrase != "secret" {
		t.Fatalf("passphrase = %q", app.vaultPassphrase)
	}
	if app.vaultBusy != "" {
		t.Fatalf("busy should clear on error, got %q", app.vaultBusy)
	}
}

func TestVaultConflictDuringLinkKeepsSession(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.beginVaultBusy("Linking vault…")

	next, _ := m.Update(vaultConflictMsg{
		count:       2,
		interactive: true,
		forLink:     true,
		passphrase:  "secret",
		session:     &vault.Session{Email: "you@example.com", Token: "tok"},
	})
	app := next.(*App)
	if !app.vaultLinked() {
		t.Fatal("expected linked while resolving first-link conflicts")
	}
	if app.vaultPassphrase != "secret" {
		t.Fatalf("passphrase = %q", app.vaultPassphrase)
	}
	if app.form == nil || app.form.action != "vault_resolve" {
		t.Fatalf("expected conflict form, got %#v", app.form)
	}
}

func TestVaultRemoteUpdatedTriggersPull(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.vaultSession = &vault.Session{Email: "you@example.com", Token: "t", Revision: "old"}
	m.vaultPassphrase = "secret"

	next, cmd := m.Update(vaultPushMsg{err: vault.ErrRemoteUpdated, synced: true})
	app := next.(*App)
	if app.vaultStatus != "Remote vault changed" {
		t.Fatalf("vaultStatus = %q", app.vaultStatus)
	}
	if app.vaultBusy == "" {
		t.Fatal("expected sync busy after remote-updated push")
	}
	if !app.vaultRemoteRetry {
		t.Fatal("expected remote-retry armed after first ErrRemoteUpdated")
	}
	if cmd == nil {
		t.Fatal("expected pull+push recovery cmd")
	}
}

func TestVaultRemoteUpdatedDoesNotLoop(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.vaultSession = &vault.Session{Email: "you@example.com", Token: "t", Revision: "old"}
	m.vaultPassphrase = "secret"
	m.beginVaultBusy("Syncing vault…")
	m.vaultRemoteRetry = true
	opGen := m.vaultOpGen

	next, cmd := m.Update(vaultPushMsg{err: vault.ErrRemoteUpdated, synced: true, opGen: opGen})
	app := next.(*App)
	if app.vaultBusy != "" {
		t.Fatalf("second remote-updated should not restart busy, got %q", app.vaultBusy)
	}
	if app.vaultRemoteRetry {
		t.Fatal("remote-retry should clear after giving up")
	}
	if app.vaultStatus != "Remote vault changed" {
		t.Fatalf("vaultStatus = %q", app.vaultStatus)
	}
	if cmd == nil {
		t.Fatal("expected notice cmd")
	}
}

func TestVaultPushIgnoresStaleOpGen(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.beginVaultBusy("Syncing vault…")
	stale := m.vaultOpGen
	m.cancelVaultOp()

	next, cmd := m.Update(vaultPushMsg{
		err:    vault.ErrRemoteUpdated,
		synced: true,
		opGen:  stale,
	})
	app := next.(*App)
	if app.vaultBusy != "" {
		t.Fatalf("stale push should not restore busy after cancel, got %q", app.vaultBusy)
	}
	if app.vaultRemoteRetry {
		t.Fatal("stale push should not arm remote-retry")
	}
	if cmd != nil {
		t.Fatal("stale push should be ignored")
	}
}

func TestVaultPassphraseCopyOmitsModeHint(t *testing.T) {
	m := testApp(t)
	m.openVaultPassphraseForm("you@example.com", "tok", "uid", "dev", "https://example", true)
	if m.form == nil {
		t.Fatal("expected passphrase form")
	}
	for _, f := range m.form.fields {
		if strings.Contains(f.description, "0600") || strings.Contains(strings.ToLower(f.description), "saved unlocked") {
			t.Fatalf("passphrase field should not mention local save mode: %q", f.description)
		}
	}
}

func TestVaultAPIBaseFormPersistsPreference(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vault"
	m.openVaultAPIBaseForm()
	if m.form == nil || m.form.action != "vault_api_base" {
		t.Fatalf("form = %#v", m.form)
	}
	for i := range m.form.fields {
		if m.form.fields[i].label == "API base" {
			m.form.fields[i].value = "https://vault.example.com/"
		}
	}
	next, _ := m.submitForm()
	app := next.(*App)
	if app.form != nil {
		t.Fatalf("expected form closed, validation=%q", app.form.validationError)
	}
	session, err := vault.LoadSession(app.vaultSessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if session.APIBase != "https://vault.example.com" {
		t.Fatalf("APIBase = %q", session.APIBase)
	}
	if app.vaultLinked() {
		t.Fatal("API base preference alone should not mark vault linked")
	}
	if got := app.preferredVaultAPIBase(); got != "https://vault.example.com" {
		t.Fatalf("preferred = %q", got)
	}
}
