package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	cloudsync "bast/internal/cloud/sync"
	"bast/internal/connectbanner"
	keymodel "bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
	"bast/internal/vault"
)

func testApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &App{paths: p, openSSH: openssh.Default(), metadata: store, hosts: []sshconfig.Host{{Alias: "alpha"}, {Alias: "beta"}}, width: 100, height: 30, dark: true}
}

func processFixtureCommand(t *testing.T, behavior string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestConnectionProcessFixture$", "--", behavior)
	cmd.Env = append(os.Environ(), "BAST_CONNECTION_PROCESS_FIXTURE=1")
	return cmd
}

func TestConnectionProcessFixture(t *testing.T) {
	if os.Getenv("BAST_CONNECTION_PROCESS_FIXTURE") != "1" {
		return
	}
	behavior := os.Args[len(os.Args)-1]
	switch behavior {
	case "session-output":
		fmt.Fprint(os.Stdout, "session-output")
		os.Exit(0)
	case "ssh-failure":
		fmt.Fprintln(os.Stdout, "host key verification failed")
		os.Exit(255)
	case "should-not-run":
		fmt.Fprint(os.Stdout, "should-not-run")
		os.Exit(0)
	case "exit-255":
		os.Exit(255)
	default:
		fmt.Fprintf(os.Stderr, "unknown connection process fixture: %s\n", behavior)
		os.Exit(2)
	}
}

func press(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: []rune(text)[0], Text: text})
}

func ctrlEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "\x00", Mod: tea.ModCtrl})
}

func ctrlJ() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'j', Mod: tea.ModCtrl})
}

func ctrlC() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
}

func requireQuit(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("command did not return tea.Quit")
	}
}

func hostHubIndex(t *testing.T, m *App, id string) int {
	t.Helper()
	for i, item := range hostHubItems(m.form) {
		if item.id == id {
			return i
		}
	}
	t.Fatalf("hub item %q not found", id)
	return -1
}

func enterHostFormSection(t *testing.T, m *App, section string) {
	t.Helper()
	for i, item := range hostHubItems(m.form) {
		if item.section == section {
			m.form.hubIndex = i
			m.enterHostSection(section)
			return
		}
	}
	t.Fatalf("section %q not found in host form", section)
}

func enterAdvancedSubsection(t *testing.T, m *App, section string) {
	t.Helper()
	enterHostFormSection(t, m, formSectionAdvanced)
	for i, item := range advancedHubList {
		if item.section == section {
			m.form.hubIndex = i
			m.enterAdvancedSubsection(section)
			return
		}
	}
	t.Fatalf("advanced subsection %q not found", section)
}

func formFieldByLabel(m *App, label string) field {
	t := m.form.fieldByLabel(label)
	if t == nil {
		return field{}
	}
	return *t
}

func selectHostAlias(t *testing.T, m *App, alias string) {
	t.Helper()
	for i, row := range m.hostListRows() {
		if !row.header && row.suggestion == nil && row.host.Alias == alias {
			m.cursor = i
			return
		}
	}
	t.Fatalf("host %q not found", alias)
}

func selectProviderHost(t *testing.T, m *App, alias string) {
	t.Helper()
	life, _ := m.providerActionLayout()
	for i, row := range m.providerInventoryRows() {
		if !row.header && row.host.Alias == alias {
			m.syncCursor = len(life) + i
			return
		}
	}
	t.Fatalf("provider host %q not found", alias)
}

func TestNumberedNavigationAndSearch(t *testing.T) {
	m := testApp(t)
	m.section = hostsSection
	m.Update(press("2"))
	if m.section != keysSection {
		t.Fatal("2 did not open keys")
	}
	m.Update(press("1"))
	m.Update(press("/"))
	m.Update(press("b"))
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := m.filteredHosts(); len(got) != 1 || got[0].Alias != "beta" {
		t.Fatalf("filter = %+v", got)
	}
}

func TestCtrlCQuitsFromEveryBastContext(t *testing.T) {
	contexts := map[string]func(*App){
		"root":   func(*App) {},
		"help":   func(m *App) { m.help = true },
		"about":  func(m *App) { m.credits = true },
		"search": func(m *App) { m.search = "\x00query" },
		"error":  func(m *App) { m.status, m.statusError = "failed", true },
		"host form": func(m *App) {
			m.openAddHostForm()
		},
		"generic form": func(m *App) {
			m.openGenerateForm()
		},
	}
	for name, setup := range contexts {
		t.Run(name, func(t *testing.T) {
			m := testApp(t)
			setup(m)
			_, cmd := m.Update(ctrlC())
			requireQuit(t, cmd)
		})
	}
}

func TestQQuitsOutsideTextInput(t *testing.T) {
	for _, context := range []string{"root", "help", "about"} {
		t.Run(context, func(t *testing.T) {
			m := testApp(t)
			m.help = context == "help"
			m.credits = context == "about"
			_, cmd := m.Update(press("q"))
			requireQuit(t, cmd)
		})
	}

	m := testApp(t)
	m.search = "\x00"
	_, cmd := m.Update(press("q"))
	if cmd != nil || m.search != "\x00q" {
		t.Fatalf("q should type in search: search=%q cmd=%v", m.search, cmd)
	}

	m = testApp(t)
	m.openGenerateForm()
	_, cmd = m.Update(press("q"))
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("q quit from a generic text field")
		}
	}
	if m.form.input.Value() != "q" {
		t.Fatalf("q should type in a generic form: value=%q", m.form.input.Value())
	}

	m = testApp(t)
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub"}}
	m.openInstallKeyForm()
	_, cmd = m.Update(press("q"))
	requireQuit(t, cmd)
}

func TestEscapeReturnsToParentThenQuitsAtRoot(t *testing.T) {
	for _, section := range []section{hostsSection, keysSection, syncSection} {
		t.Run(fmt.Sprint(section), func(t *testing.T) {
			m := testApp(t)
			m.section = section
			_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
			requireQuit(t, cmd)
		})
	}

	m := testApp(t)
	m.section, m.syncProvider = syncSection, "gcp"
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if cmd != nil || m.syncProvider != "" {
		t.Fatalf("Esc should return from provider: provider=%q cmd=%v", m.syncProvider, cmd)
	}

	m = testApp(t)
	m.help = true
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.help {
		t.Fatal("Esc did not close help")
	}

	m = testApp(t)
	m.search = "\x00query"
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.search != "" {
		t.Fatalf("Esc did not close search: %q", m.search)
	}

	m = testApp(t)
	m.openGenerateForm()
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.form != nil {
		t.Fatal("Esc did not close a generic form")
	}
}

func TestBackspaceQuitsAtRootAndReturnsFromSyncProvider(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}),
		tea.KeyPressMsg(tea.Key{Code: 'h', Mod: tea.ModCtrl}),
	} {
		for _, section := range []section{hostsSection, keysSection, syncSection} {
			m := testApp(t)
			m.section = section
			_, cmd := m.Update(key)
			requireQuit(t, cmd)
		}

		m := testApp(t)
		m.section, m.syncProvider = syncSection, "gcp"
		_, cmd := m.Update(key)
		if cmd != nil || m.syncProvider != "" {
			t.Fatalf("backspace should return from Sync provider: provider=%q cmd=%v", m.syncProvider, cmd)
		}
	}
}

func TestNewlyCreatedItemsAreSelectedAfterReload(t *testing.T) {
	m := testApp(t)
	m.search = "old filter"
	m.selectAfterLoadSection, m.selectAfterLoadName = hostsSection, "new_server"
	m.Update(loadedMsg{hosts: []sshconfig.Host{{Alias: "alpha"}, {Alias: "new_server"}}, keys: nil})
	if m.section != hostsSection || m.search != "" || m.cursor != 1 {
		t.Fatalf("new host was not selected: section=%v search=%q cursor=%d", m.section, m.search, m.cursor)
	}
	if err := m.metadata.SetHost("new_server", metadata.Host{Group: "New group"}); err != nil {
		t.Fatal(err)
	}
	m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = hostsSection, "New group", true
	m.Update(loadedMsg{hosts: m.hosts, keys: nil})
	rows := m.hostRows()
	if m.cursor >= len(rows) || !rows[m.cursor].header || rows[m.cursor].group != "New group" {
		t.Fatalf("new group was not selected: rows=%+v cursor=%d", rows, m.cursor)
	}

	m.selectAfterLoadSection, m.selectAfterLoadName = keysSection, "new key"
	m.Update(loadedMsg{hosts: m.hosts, keys: []keymodel.Key{{Name: "existing"}, {Name: "new key"}}})
	if m.section != keysSection || m.cursor != 1 {
		t.Fatalf("new key was not selected: section=%v cursor=%d", m.section, m.cursor)
	}
}

func TestHostGroupsAreVisuallySeparatedAndCollapsible(t *testing.T) {
	m := testApp(t)
	m.hosts = append(m.hosts, sshconfig.Host{Alias: "gamma"})
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("gamma", metadata.Host{Group: "Personal"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()

	rows := m.hostRows()
	if len(rows) != 5 || !rows[0].header || rows[0].group != "Work" || !rows[3].header || rows[3].group != "Personal" {
		t.Fatalf("group rows = %+v", rows)
	}
	rendered := m.renderHosts(m.styles())
	if !strings.Contains(rendered, "▾ Work") || !strings.Contains(rendered, "▾ Personal") {
		t.Fatalf("group headers are not visually distinct:\n%s", rendered)
	}

	m.cursor = 1
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if !m.collapsedGroups["Work"] || m.cursor != 0 {
		t.Fatalf("Space did not collapse and select Work: collapsed=%v cursor=%d", m.collapsedGroups, m.cursor)
	}
	rows = m.hostRows()
	if len(rows) != 3 || !rows[0].header {
		t.Fatalf("collapsed rows = %+v", rows)
	}
	if collapsed := m.renderHosts(m.styles()); !strings.Contains(collapsed, "▸ Work") || strings.Contains(collapsed, "alpha") {
		t.Fatalf("collapsed group was not rendered correctly:\n%s", collapsed)
	}

	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if m.collapsedGroups["Work"] || len(m.hostRows()) != 5 {
		t.Fatal("Space did not expand Work")
	}
}

func TestCloudSyncRootShowsRightAlignedErrorIcon(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Google Cloud/project-a"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("beta", metadata.Host{Group: "Google Cloud/project-b"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, LastSyncError: "gcloud exited with status 1"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()

	rendered := m.renderHosts(m.styles())
	if strings.Count(rendered, "⚠") != 1 {
		t.Fatalf("expected one error icon on the provider root:\n%s", rendered)
	}
	rootLine := strings.Split(rendered, "\n")[0]
	iconIndex := strings.Index(rootLine, "⚠")
	if iconIndex < 0 || lipgloss.Width(rootLine[:iconIndex]) != m.panelLayout().listWidth-1 {
		t.Fatalf("error icon is not right aligned: %q", rootLine)
	}

	gcp := m.metadata.GCP()
	gcp.LastSyncError = ""
	if err := m.metadata.SetGCP(gcp); err != nil {
		t.Fatal(err)
	}
	if rendered = m.renderHosts(m.styles()); strings.Contains(rendered, "⚠") {
		t.Fatalf("cleared sync error still shows the icon:\n%s", rendered)
	}
}

func TestCloudSyncRootShowsCurrentCLIError(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Amazon EC2/default/eu-west-2"}); err != nil {
		t.Fatal(err)
	}
	m.syncStatus.AWS.AWSCLIError = "aws executable not found"
	m.sortHosts()

	if rendered := m.renderHosts(m.styles()); strings.Count(rendered, "⚠") != 1 {
		t.Fatalf("expected one error icon for the current CLI error:\n%s", rendered)
	}
}

func TestFooterHidesConnectWhenGroupSelected(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = 0

	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "␣") || !strings.Contains(footer, "?") {
		t.Fatalf("group footer should show collapse: %q", footer)
	}
	if strings.Contains(footer, "enter") || strings.Contains(footer, "Connect") {
		t.Fatalf("group footer should not advertise connect: %q", footer)
	}

	m.cursor = 1
	footer = m.renderFooter(m.styles())
	if !strings.Contains(footer, "enter") || !strings.Contains(footer, "e edit") || !strings.Contains(footer, "F files") {
		t.Fatalf("host footer = %q", footer)
	}
	if strings.Contains(footer, "␣") {
		t.Fatalf("host footer should not show collapse: %q", footer)
	}
}

func TestRenameGroupCascadesToHosts(t *testing.T) {
	m := testApp(t)
	m.hosts = append(m.hosts, sshconfig.Host{Alias: "gamma"})
	for alias, group := range map[string]string{
		"alpha": "Work/Production",
		"beta":  "Work/Production/web",
		"gamma": "Work/Staging",
	} {
		if err := m.metadata.SetHost(alias, metadata.Host{Group: group}); err != nil {
			t.Fatal(err)
		}
	}
	m.sortHosts()
	for i, row := range m.hostRows() {
		if row.header && row.group == "Work/Production" {
			m.cursor = i
			break
		}
	}
	m.openEditGroupForm()
	if m.form == nil || m.form.action != "group_edit" {
		t.Fatalf("form = %+v", m.form)
	}
	m.form.input.SetValue("Prod")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form != nil {
		t.Fatal("Enter did not save and close the group edit form")
	}
	if got := m.metadata.Host("alpha").Group; got != "Work/Prod" {
		t.Fatalf("alpha group = %q", got)
	}
	if got := m.metadata.Host("beta").Group; got != "Work/Prod/web" {
		t.Fatalf("beta group = %q", got)
	}
	if got := m.metadata.Host("gamma").Group; got != "Work/Staging" {
		t.Fatalf("gamma group = %q", got)
	}
}

func TestHostGroupsSupportFiveNestedLevels(t *testing.T) {
	m := testApp(t)
	m.hosts = append(m.hosts, sshconfig.Host{Alias: "gamma"}, sshconfig.Host{Alias: "delta"})
	groups := map[string]string{
		"alpha": "test/abc",
		"beta":  "test/def",
		"gamma": "test",
		"delta": "one/two/three/four/five",
	}
	for alias, group := range groups {
		if err := m.metadata.SetHost(alias, metadata.Host{Group: group}); err != nil {
			t.Fatal(err)
		}
	}
	m.sortHosts()

	rows := m.hostRows()
	want := []struct {
		group  string
		alias  string
		header bool
		depth  int
		count  int
	}{
		{group: "test", header: true, depth: 0, count: 3},
		{group: "test", alias: "gamma", depth: 1},
		{group: "test/abc", header: true, depth: 1, count: 1},
		{group: "test/abc", alias: "alpha", depth: 2},
		{group: "test/def", header: true, depth: 1, count: 1},
		{group: "test/def", alias: "beta", depth: 2},
		{group: "one/two/three/four/five", header: true, depth: 0, count: 1},
		{group: "one/two/three/four/five", alias: "delta", depth: 1},
	}
	if len(rows) != len(want) {
		t.Fatalf("nested rows = %+v", rows)
	}
	for i, expected := range want {
		if rows[i].group != expected.group || rows[i].host.Alias != expected.alias || rows[i].header != expected.header || rows[i].depth != expected.depth || rows[i].count != expected.count {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], expected)
		}
	}

	rendered := m.renderHosts(m.styles())
	if !strings.Contains(rendered, "▾ test") || !strings.Contains(rendered, "  ▾ abc") {
		t.Fatalf("nested group indentation is missing:\n%s", rendered)
	}
	if !strings.Contains(rendered, "▾ one/two/three/four/five") {
		t.Fatalf("single-child group chain was not compacted:\n%s", rendered)
	}

	m.cursor = 3
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if !m.collapsedGroups["test/abc"] || m.cursor != 2 {
		t.Fatalf("subgroup did not collapse independently: collapsed=%v cursor=%d", m.collapsedGroups, m.cursor)
	}
	siblingVisible := false
	for _, row := range m.hostRows() {
		if row.host.Alias == "alpha" {
			t.Fatal("collapsed subgroup still shows its host")
		}
		if row.host.Alias == "beta" {
			siblingVisible = true
		}
	}
	if !siblingVisible {
		t.Fatal("collapsing test/abc also hid test/def")
	}
	m.cursor = 0
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if !m.collapsedGroups["test"] {
		t.Fatal("parent group did not collapse")
	}
	for _, row := range m.hostRows() {
		if strings.HasPrefix(row.group, "test/") || row.host.Alias == "gamma" {
			t.Fatalf("collapsed parent still shows a descendant: %+v", row)
		}
	}
}

func TestSyncedProviderRootStaysSeparateFromCompactedChildren(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{
		{Alias: "azure_vm", Synced: true, SyncSource: "azure"},
		{Alias: "ordinary"},
	}
	if err := m.metadata.SetHost("azure_vm", metadata.Host{Group: "Microsoft Azure/Production/apps"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("ordinary", metadata.Host{Group: "Work/Production/API"}); err != nil {
		t.Fatal(err)
	}

	rows := m.hostRows()
	if len(rows) != 5 {
		t.Fatalf("rows = %+v", rows)
	}
	if !rows[0].header || rows[0].group != "Microsoft Azure" || rows[0].label != "Microsoft Azure" || rows[0].depth != 0 {
		t.Fatalf("provider root = %+v", rows[0])
	}
	if !rows[1].header || rows[1].group != "Microsoft Azure/Production/apps" || rows[1].label != "Production/apps" || rows[1].depth != 1 {
		t.Fatalf("compacted provider children = %+v", rows[1])
	}
	if !rows[3].header || rows[3].group != "Work/Production/API" || rows[3].label != "Work/Production/API" || rows[3].depth != 0 {
		t.Fatalf("ordinary compacted group = %+v", rows[3])
	}
}

func TestLabelPathSetsGroupAndLeaf(t *testing.T) {
	m := testApp(t)
	m.openEditHostForm()
	for i := range m.form.fields {
		if m.form.fields[i].label == "Label" {
			m.form.fields[i].value = "abc/test"
		}
	}
	m.submitForm()
	if m.form != nil || m.statusError {
		t.Fatalf("save failed: %q", m.status)
	}
	meta := m.metadata.Host("alpha")
	if meta.Group != "abc" || meta.Label != "test" {
		t.Fatalf("saved metadata = %+v", meta)
	}
}

func TestLabelPathDisplayShowsLeafWhenInactive(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Label: "test", Group: "abc"}); err != nil {
		t.Fatal(err)
	}
	for i, row := range m.hostRows() {
		if !row.header && row.host.Alias == "alpha" {
			m.cursor = i
			break
		}
	}
	m.openEditHostForm()
	if m.form == nil {
		t.Fatal("edit form did not open")
	}
	if got := formFieldByLabel(m, "Label").value; got != "abc/test" {
		t.Fatalf("composed label path = %q", got)
	}
	m.form.hubIndex = 1
	m.focusHostHubItem()
	inactive := m.renderForm(m.styles())
	if !strings.Contains(inactive, "Label  test") {
		t.Fatalf("inactive label should show leaf only:\n%s", inactive)
	}
	if strings.Contains(inactive, "Label  abc/test") {
		t.Fatalf("inactive label should not show full path:\n%s", inactive)
	}
	m.form.hubIndex = 0
	m.focusHostHubItem()
	m.form.input.SetValue("abc/test")
	m.form.input.SetCursor(len([]rune("abc/test")))
	s := m.styles()
	focused := m.renderLabelPathInput(s)
	wantFocused := s.muted.Render("a") + s.muted.Render("b") + s.muted.Render("c") +
		s.muted.Render("/") +
		s.value.Render("t") + s.value.Render("e") + s.value.Render("s") + s.value.Render("t") +
		lipgloss.NewStyle().Reverse(true).Render(" ")
	if focused != wantFocused {
		t.Fatalf("focused label path:\ngot  %q\nwant %q", focused, wantFocused)
	}
}

func TestLabelPathMutesInactiveSideAndKeepsSlashMuted(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	m.form.hubIndex = 0
	m.focusHostHubItem()
	s := m.styles()
	m.form.input.SetValue("abc/test")

	m.form.input.SetCursor(4) // on leaf
	inLeaf := m.renderLabelPathInput(s)
	wantLeaf := s.muted.Render("a") + s.muted.Render("b") + s.muted.Render("c") +
		s.muted.Render("/") +
		lipgloss.NewStyle().Reverse(true).Render("t") +
		s.value.Render("e") + s.value.Render("s") + s.value.Render("t")
	if inLeaf != wantLeaf {
		t.Fatalf("cursor in leaf:\ngot  %q\nwant %q", inLeaf, wantLeaf)
	}

	m.form.input.SetCursor(1) // on group
	inGroup := m.renderLabelPathInput(s)
	wantGroup := s.value.Render("a") +
		lipgloss.NewStyle().Reverse(true).Render("b") +
		s.value.Render("c") +
		s.muted.Render("/") +
		s.muted.Render("t") + s.muted.Render("e") + s.muted.Render("s") + s.muted.Render("t")
	if inGroup != wantGroup {
		t.Fatalf("cursor in group:\ngot  %q\nwant %q", inGroup, wantGroup)
	}

	m.form.input.SetCursor(3) // on slash
	onSlash := m.renderLabelPathInput(s)
	wantSlash := s.value.Render("a") + s.value.Render("b") + s.value.Render("c") +
		lipgloss.NewStyle().Reverse(true).Render("/") +
		s.muted.Render("t") + s.muted.Render("e") + s.muted.Render("s") + s.muted.Render("t")
	if onSlash != wantSlash {
		t.Fatalf("cursor on slash:\ngot  %q\nwant %q", onSlash, wantSlash)
	}
}

func TestAddHostFormPrefillsGroupFromSelection(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work/Production"}); err != nil {
		t.Fatal(err)
	}
	m.hosts = []sshconfig.Host{{Alias: "alpha"}}
	for i, row := range m.hostRows() {
		if !row.header && row.host.Alias == "alpha" {
			m.cursor = i
			break
		}
	}
	m.openAddHostForm()
	if got := formFieldByLabel(m, "Label").value; got != "Work/Production/" {
		t.Fatalf("label path prefill = %q", got)
	}
	summary := m.hostFormSummary(formSectionMetadata)
	if summary != "-" {
		t.Fatalf("metadata summary should not include group path, got %q", summary)
	}
	if got := metadata.LabelGroup(formFieldByLabel(m, "Label").value); got != "Work/Production" {
		t.Fatalf("label path group = %q", got)
	}
}

func TestGroupPathsAreNormalizedAndLimitedToFiveLevels(t *testing.T) {
	m := testApp(t)
	m.openEditHostForm()
	for i := range m.form.fields {
		if m.form.fields[i].label == "Label" {
			m.form.fields[i].value = "one/two/three/four/five/six/leaf"
		}
	}
	m.submitForm()
	if !m.statusError || m.form == nil || !strings.Contains(m.status, "at most 5 levels") {
		t.Fatalf("six-level group was accepted: status=%q", m.status)
	}
	if got := m.metadata.Host("alpha").Group; got != "" {
		t.Fatalf("invalid group was saved as %q", got)
	}

	m.status, m.statusError = "", false
	for i := range m.form.fields {
		if m.form.fields[i].label == "Label" {
			m.form.fields[i].value = " one / two / three / four / five / leaf "
		}
	}
	m.submitForm()
	if m.form != nil || m.statusError {
		t.Fatalf("five-level group was rejected: %q", m.status)
	}
	if got := m.metadata.Host("alpha").Group; got != "one/two/three/four/five" {
		t.Fatalf("normalized group = %q", got)
	}
	if got := m.metadata.Host("alpha").Label; got != "leaf" {
		t.Fatalf("leaf label = %q", got)
	}
}

func TestQuickGroupAssignmentSupportsExistingNewAndNoGroup(t *testing.T) {
	t.Run("fuzzy existing ancestor", func(t *testing.T) {
		m := testApp(t)
		if err := m.metadata.SetHost("alpha", metadata.Host{Notes: "keep"}); err != nil {
			t.Fatal(err)
		}
		if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work/Production/API"}); err != nil {
			t.Fatal(err)
		}
		if err := m.metadata.SetHost("removed", metadata.Host{Group: "Removed/Phantom"}); err != nil {
			t.Fatal(err)
		}
		selectHostAlias(t, m, "alpha")
		m.Update(press("m"))
		if m.form == nil || m.form.action != "group_assign" || !m.form.input.Focused() {
			t.Fatalf("m did not open a focused searchable group picker: %#v", m.form)
		}
		if rendered := m.renderGroupAssignmentForm(m.styles()); !strings.Contains(rendered, "No group") || !strings.Contains(rendered, "Current: No group") {
			t.Fatalf("initial picker is incorrect:\n%s", rendered)
		}
		for _, key := range []string{"w", "p", "a"} {
			m.Update(press(key))
		}
		choices := m.groupPickerChoices()
		if len(choices) < 2 || choices[0].value != "Work/Production/API" || !choices[1].create {
			t.Fatalf("fuzzy choices for wpa = %#v", choices)
		}
		for _, choice := range choices {
			if choice.value == "Removed/Phantom" {
				t.Fatalf("picker includes metadata for an absent host: %#v", choices)
			}
		}
		m.collapsedGroups = map[string]bool{"Work": true, "Work/Production": true, "Work/Production/API": true}
		if err := m.metadata.SetCollapsedGroups([]string{"Work", "Work/Production", "Work/Production/API"}); err != nil {
			t.Fatal(err)
		}
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if m.form != nil {
			t.Fatal("existing group assignment did not close the picker")
		}
		got := m.metadata.Host("alpha")
		if got.Group != "Work/Production/API" || got.Notes != "keep" {
			t.Fatalf("metadata after assignment = %#v", got)
		}
		if len(m.collapsedGroups) != 0 {
			t.Fatalf("destination path stayed collapsed: %#v", m.collapsedGroups)
		}
		if got := m.metadata.Preferences().CollapsedGroups; len(got) != 0 {
			t.Fatalf("persisted destination path stayed collapsed: %#v", got)
		}
		m.selectAfterLoad()
		selected, ok := m.selectedHost()
		if !ok || selected.Alias != "alpha" {
			t.Fatalf("selected host after assignment = %#v, ok=%t", selected, ok)
		}
	})

	t.Run("current group selected", func(t *testing.T) {
		m := testApp(t)
		if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work/Production/API"}); err != nil {
			t.Fatal(err)
		}
		selectHostAlias(t, m, "alpha")
		m.updateKeys(press("m"))
		group := m.form.fieldByLabel("Group")
		choices := m.groupPickerChoices()
		if got := choices[group.selected].value; got != "Work/Production/API" {
			t.Fatalf("selected group = %q", got)
		}
		m.updateForm(press("x"))
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
		choices = m.groupPickerChoices()
		if got := choices[group.selected].value; got != "Work/Production/API" {
			t.Fatalf("selected group after clearing search = %q", got)
		}
	})

	t.Run("new normalized path", func(t *testing.T) {
		m := testApp(t)
		selectHostAlias(t, m, "alpha")
		m.updateKeys(press("m"))
		m.updateFormPaste(tea.PasteMsg{Content: "Team / Platform"})
		choices := m.groupPickerChoices()
		if len(choices) != 1 || !choices[0].create {
			t.Fatalf("new-path choices = %#v", choices)
		}
		if rendered := m.renderGroupAssignmentForm(m.styles()); !strings.Contains(rendered, `Create "Team / Platform"`) {
			t.Fatalf("picker does not offer creation:\n%s", rendered)
		}
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got := m.metadata.Host("alpha").Group; got != "Team/Platform" {
			t.Fatalf("new group = %q", got)
		}
	})

	t.Run("normalized exact match", func(t *testing.T) {
		m := testApp(t)
		if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work/Production"}); err != nil {
			t.Fatal(err)
		}
		selectHostAlias(t, m, "alpha")
		m.updateKeys(press("m"))
		m.updateFormPaste(tea.PasteMsg{Content: "Work / Production"})
		choices := m.groupPickerChoices()
		if len(choices) != 1 || choices[0].value != "Work/Production" || choices[0].create {
			t.Fatalf("normalized exact choices = %#v", choices)
		}
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got := m.metadata.Host("alpha").Group; got != "Work/Production" {
			t.Fatalf("matched group = %q", got)
		}
	})

	t.Run("custom alongside fuzzy match", func(t *testing.T) {
		m := testApp(t)
		if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work/Production"}); err != nil {
			t.Fatal(err)
		}
		selectHostAlias(t, m, "alpha")
		m.updateKeys(press("m"))
		m.updateFormPaste(tea.PasteMsg{Content: "prod"})
		choices := m.groupPickerChoices()
		if len(choices) < 2 || choices[0].value != "Work/Production" || !choices[1].create {
			t.Fatalf("pick/create choices = %#v", choices)
		}
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got := m.metadata.Host("alpha").Group; got != "prod" {
			t.Fatalf("custom group = %q", got)
		}
	})

	t.Run("no group", func(t *testing.T) {
		m := testApp(t)
		if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
			t.Fatal(err)
		}
		selectHostAlias(t, m, "alpha")
		m.updateKeys(press("m"))
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got := m.metadata.Host("alpha").Group; got != "" {
			t.Fatalf("cleared group = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		m := testApp(t)
		selectHostAlias(t, m, "alpha")
		m.updateKeys(press("m"))
		m.updateFormPaste(tea.PasteMsg{Content: "one/two/three/four/five/six"})
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if m.form == nil || !strings.Contains(m.form.validationError, "at most 5") {
			t.Fatalf("invalid group did not stay in the picker: %#v", m.form)
		}
	})

	t.Run("reserved cloud group", func(t *testing.T) {
		m := testApp(t)
		if err := m.metadata.SetHost("beta", metadata.Host{Group: "Google Cloud/project"}); err != nil {
			t.Fatal(err)
		}
		selectHostAlias(t, m, "alpha")
		m.updateKeys(press("m"))
		for _, option := range m.form.fieldByLabel("Group").options {
			if cloudsync.IsSyncedGroup(option.value) {
				t.Fatalf("picker includes reserved cloud group %q", option.value)
			}
		}
		m.updateFormPaste(tea.PasteMsg{Content: "Google Cloud/custom"})
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if m.form == nil || !strings.Contains(m.form.validationError, "read-only") {
			t.Fatalf("reserved group did not stay in the picker: %#v", m.form)
		}
		if got := m.metadata.Host("alpha").Group; got != "" {
			t.Fatalf("host moved into reserved group %q", got)
		}
	})
}

func TestGroupPickerRoutesPasteAndEscape(t *testing.T) {
	m := testApp(t)
	selectHostAlias(t, m, "alpha")
	m.Update(press("m"))
	m.Update(tea.PasteMsg{Content: "Team/Platform"})
	if m.form == nil || m.form.input.Value() != "Team/Platform" {
		t.Fatalf("paste was not routed to group search: %#v", m.form)
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.form != nil {
		t.Fatal("Esc did not cancel the group picker")
	}
}

func TestFuzzyGroupScoreUsesBestAlignment(t *testing.T) {
	short, shortOK := fuzzyGroupScore("P/Prod", "prod")
	long, longOK := fuzzyGroupScore("X/ProdLong", "prod")
	if !shortOK || !longOK || short <= long {
		t.Fatalf("fuzzy scores: P/Prod=%d (%t), X/ProdLong=%d (%t)", short, shortOK, long, longOK)
	}
}

func TestQuickGroupAssignmentRejectsSyncedHosts(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{Alias: "cloud", Synced: true}}
	selectHostAlias(t, m, "cloud")
	m.updateKeys(press("m"))
	if m.form != nil || !m.statusError || !strings.Contains(m.status, "read-only") {
		t.Fatalf("synced assignment: form=%#v status=%q", m.form, m.status)
	}
}

func TestHostEditorSaveIsVisiblePortableAndClickable(t *testing.T) {
	t.Run("Ctrl+J", func(t *testing.T) {
		m := testApp(t)
		m.openEditHostForm()
		m.form.input.SetValue("Portable label")
		m.updateForm(ctrlJ())
		if m.form != nil {
			t.Fatal("Ctrl+J did not save the host form")
		}
		if got := m.metadata.Host("alpha").Label; got != "Portable label" {
			t.Fatalf("saved label = %q", got)
		}
	})

	t.Run("click", func(t *testing.T) {
		m := testApp(t)
		m.openEditHostForm()
		if view := m.renderHostForm(m.styles()); !strings.Contains(view, m.styles().title.Render(hostFormSaveButtonLabel)) {
			t.Fatalf("host form has no Connect-style Save target:\n%s", view)
		}
		if m.View().MouseMode != tea.MouseModeCellMotion {
			t.Fatal("host form did not enable mouse reporting")
		}
		x, y, width := m.hostFormSaveButtonBounds()
		lines := strings.Split(m.render(), "\n")
		if y >= len(lines) || !strings.Contains(lines[y], hostFormSaveButtonLabel) {
			t.Fatalf("Save bounds row %d does not contain the button", y)
		}
		buttonOffset := strings.Index(lines[y], hostFormSaveButtonLabel)
		if got := lipgloss.Width(lines[y][:buttonOffset]); got != x {
			t.Fatalf("Save bounds x = %d, rendered x = %d", x, got)
		}
		m.form.input.SetValue("Clicked label")
		m.Update(tea.MouseClickMsg(tea.Mouse{X: x + width/2, Y: y, Button: tea.MouseLeft}))
		if m.form != nil {
			t.Fatal("clicking Save did not close the host form")
		}
		if got := m.metadata.Host("alpha").Label; got != "Clicked label" {
			t.Fatalf("saved label = %q", got)
		}
	})

	t.Run("wide title", func(t *testing.T) {
		m := testApp(t)
		m.width = 30
		if err := m.metadata.SetHost("alpha", metadata.Host{Label: "生产服务器"}); err != nil {
			t.Fatal(err)
		}
		m.openEditHostForm()
		x, y, width := m.hostFormSaveButtonBounds()
		lines := strings.Split(m.render(), "\n")
		buttonOffset := strings.Index(lines[y], hostFormSaveButtonLabel)
		if buttonOffset < 0 || lipgloss.Width(lines[y][:buttonOffset]) != x {
			t.Fatalf("wide title shifted Save away from x=%d: %q", x, lines[y])
		}
		m.Update(tea.MouseClickMsg(tea.Mouse{X: x + width/2, Y: y, Button: tea.MouseLeft}))
		if m.form != nil {
			t.Fatal("clicking Save with a wide title did not close the form")
		}
	})

	t.Run("unrelated form", func(t *testing.T) {
		m := testApp(t)
		m.hosts[0].Managed = true
		m.openDeleteHostForm()
		if m.View().MouseMode != tea.MouseModeNone {
			t.Fatal("unrelated form unexpectedly enabled mouse reporting")
		}
	})
}

func TestHostEditorSaveHintAlternatesWithoutStaleTicks(t *testing.T) {
	m := testApp(t)
	_, cmd := m.Update(press("a"))
	if cmd == nil {
		t.Fatal("opening the host editor did not schedule the save hint")
	}
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "Ctrl+J save") || strings.Contains(footer, "Ctrl+↵ save") {
		t.Fatalf("initial save hint = %q", footer)
	}

	hintID := m.hostSaveHintID
	_, cmd = m.Update(hostSaveHintTickMsg(hintID))
	if cmd == nil {
		t.Fatal("active host editor did not schedule the next save hint")
	}
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "Ctrl+↵ save") || strings.Contains(footer, "Ctrl+J save") {
		t.Fatalf("alternate save hint = %q", footer)
	}

	m.form = nil
	_, cmd = m.Update(hostSaveHintTickMsg(hintID))
	if cmd != nil {
		t.Fatal("closed host editor kept the save hint timer alive")
	}
}

func TestMouseSelectsTabsAndListRowsOnly(t *testing.T) {
	m := testApp(t)
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("mouse reporting is not enabled")
	}

	m.Update(tea.MouseClickMsg(tea.Mouse{X: 21, Y: 0, Button: tea.MouseLeft}))
	if m.section != keysSection {
		t.Fatal("clicking the Keys tab did not switch sections")
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 9, Y: 0, Button: tea.MouseLeft}))
	if m.section != hostsSection {
		t.Fatal("clicking the Hosts tab did not switch sections")
	}

	m.Update(tea.MouseClickMsg(tea.Mouse{X: 2, Y: 3, Button: tea.MouseLeft}))
	if m.cursor != 1 {
		t.Fatalf("clicking the second host selected row %d", m.cursor)
	}
	listWidth, _, _ := m.columnDimensions()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: listWidth + 2, Y: 2, Button: tea.MouseLeft}))
	if m.cursor != 1 {
		t.Fatal("clicking the details panel changed the selection")
	}
}
func TestExternalHostEditOnlyOffersMetadataAndIsFullyRevealed(t *testing.T) {
	m := testApp(t)
	m.openEditHostForm()
	if m.form == nil || m.form.action != "metadata_edit" {
		t.Fatalf("form = %+v", m.form)
	}
	rendered := m.renderForm(m.styles())
	if !strings.Contains(rendered, "Label") || !strings.Contains(rendered, "Metadata") || strings.Contains(rendered, "Authentication") || strings.Contains(rendered, "Hostname") {
		t.Fatalf("metadata editor layout is incorrect:\n%s", rendered)
	}
	enterHostFormSection(t, m, formSectionMetadata)
	rendered = m.renderForm(m.styles())
	if strings.Contains(rendered, "Group") || !strings.Contains(rendered, "Tags") {
		t.Fatalf("metadata section was not opened:\n%s", rendered)
	}
	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "Ctrl+J save") || strings.Contains(footer, "connect") {
		t.Fatalf("metadata editor footer is incorrect: %q", footer)
	}
}

func TestFooterShowsControlsForTheActiveFormState(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "Enter next") || !strings.Contains(footer, "Ctrl+J save") || !strings.Contains(footer, "Esc cancel") || strings.Contains(footer, "connect") {
		t.Fatalf("create footer = %q", footer)
	}

	m.openEditHostForm()
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "Ctrl+J save") || !strings.Contains(footer, "Tab move") || strings.Contains(footer, "connect") {
		t.Fatalf("edit footer = %q", footer)
	}
	enterHostFormSection(t, m, formSectionMetadata)
	if footer := m.renderFooter(m.styles()); strings.Contains(footer, "q quit") {
		t.Fatalf("plain-text host field advertises q as quit: %q", footer)
	}

	m.hosts[0].Managed = true
	m.openEditHostForm()
	enterHostFormSection(t, m, formSectionAuth)
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "q quit") {
		t.Fatalf("selectable host field does not advertise q as quit: %q", footer)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "choose") || !strings.Contains(footer, "Enter select") || !strings.Contains(footer, "Esc back") {
		t.Fatalf("choice footer = %q", footer)
	}
}

func TestEditFormUsesArrowsToMoveAndEnterToSave(t *testing.T) {
	m := testApp(t)
	m.openEditHostForm()
	enterHostFormSection(t, m, formSectionMetadata)
	if m.form == nil || m.form.fields[m.form.index].label != "Tags" {
		t.Fatal("metadata section did not open on the tags field")
	}
	m.form.input.SetValue("api")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.form == nil || m.form.fields[m.form.index].label != "Tags" {
		t.Fatal("Up did not return to the tags field")
	}
	m.updateForm(ctrlEnter())
	if m.form != nil {
		t.Fatal("Ctrl+Enter did not save and close the edit form")
	}
	if got := m.metadata.Host("alpha").Tags; len(got) != 1 || got[0] != "api" {
		t.Fatalf("saved tags = %v", got)
	}
}

func TestEditFormUsesSpaceToChangeAChoice(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{Alias: "alpha", Managed: true, ManagedID: "alpha"}}
	m.openEditHostForm()
	enterHostFormSection(t, m, formSectionAuth)
	if m.form.fields[m.form.index].label != methodFieldLabel || m.form.selecting {
		t.Fatal("auth section did not open on the method field")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if !m.form.selecting {
		t.Fatal("Space did not open the method choices")
	}
	m.updateForm(press("j"))
	m.updateForm(press("j"))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form.fields[m.form.index].label != passwordFieldLabel {
		t.Fatal("choosing Password did not move to the password field")
	}
	if got := formFieldByLabel(m, methodFieldLabel).value; got != passwordOnlyIdentity {
		t.Fatalf("selected method = %q", got)
	}
}

func TestHostFormQuitKeysWorkFromMenus(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	m.form.hubIndex = hostHubIndex(t, m, "auth")
	m.focusHostHubItem()
	_, cmd := m.updateForm(press("q"))
	if cmd == nil {
		t.Fatal("q on hub menu did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q on hub menu did not request quit")
	}

	m = testApp(t)
	m.openAddHostForm()
	_, cmd = m.updateForm(press("q"))
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("q on label field should type, not quit")
		}
	}
	if got := m.form.input.Value(); got != "q" {
		t.Fatalf("q on label field = %q", got)
	}

	m = testApp(t)
	m.openAddHostForm()
	_, cmd = m.updateForm(ctrlC())
	if cmd == nil {
		t.Fatal("ctrl+c did not quit from host form")
	}
}

func TestHostFormBackspaceNavigatesSubmenus(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	enterHostFormSection(t, m, formSectionMetadata)
	m.form.input.SetValue("")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form.screen != formSectionMetadata {
		t.Fatalf("backspace in an empty text field navigated away: screen=%q", m.form.screen)
	}

	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m.form.hubIndex = 1
	m.focusHostHubItem()
	m.form.input.SetValue("prod")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form.hubIndex != 1 || m.form.input.Value() != "pro" {
		t.Fatalf("backspace should edit text before navigating: hubIndex=%d value=%q", m.form.hubIndex, m.form.input.Value())
	}
	m.form.input.SetValue("")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form == nil || m.form.hubIndex != 1 {
		t.Fatal("backspace in an empty hostname field should remain in the field")
	}

	m.form.hubIndex = hostHubIndex(t, m, "auth")
	m.focusHostHubItem()
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form != nil {
		t.Fatal("backspace on a host hub menu should close the form")
	}

	m.openAddHostForm()
	enterHostFormSection(t, m, formSectionAdvanced)
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "Enter open section") || !strings.Contains(footer, "⌫/Esc back") {
		t.Fatalf("advanced hub footer = %q", footer)
	}
	m.form.hubIndex = 3
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form.screen != "hub" {
		t.Fatalf("backspace on advanced hub did not return to host hub: screen=%q", m.form.screen)
	}
}

func TestBackspaceReturnsFromNonTextSubmenus(t *testing.T) {
	m := testApp(t)
	m.section, m.syncProvider = syncSection, "gcp"
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.syncProvider != "" {
		t.Fatalf("backspace did not leave Sync provider: %q", m.syncProvider)
	}

	m = testApp(t)
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub"}}
	m.openInstallKeyForm()
	if m.form == nil || !m.form.selecting {
		t.Fatal("install form did not open its server chooser")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form == nil || m.form.selecting {
		t.Fatal("backspace did not return from the server chooser")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form != nil {
		t.Fatal("backspace on a non-text form field did not close the form")
	}

	m = testApp(t)
	m.credits = true
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.credits {
		t.Fatal("backspace did not close About")
	}

	m = testApp(t)
	m.status, m.statusError = "failed", true
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.status != "" || m.statusError {
		t.Fatal("backspace did not dismiss the error overlay")
	}

	m = testApp(t)
	m.openGenerateForm()
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form == nil || m.form.input.Value() != "" {
		t.Fatal("backspace in an empty generic text field navigated away")
	}

	m = testApp(t)
	m.openAddHostForm()
	enterAdvancedSubsection(t, m, formSectionAdvancedJump)
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form == nil {
		t.Fatal("backspace closed the host form from an advanced subsection")
	}
	if m.form.screen != formScreenAdvancedHub {
		t.Fatalf("backspace did not return to the advanced hub: screen=%q", m.form.screen)
	}
}

func TestAdvancedHubCanRenderAndSaveWithoutAnActiveField(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	enterHostFormSection(t, m, formSectionAdvanced)

	if m.form.index != -1 {
		t.Fatalf("advanced hub index = %d, want -1", m.form.index)
	}
	_ = m.render()
	m.updateForm(ctrlEnter())
	if !m.statusError || !strings.Contains(m.status, "label") {
		t.Fatalf("Ctrl+Enter should validate the form without panicking: status=%q", m.status)
	}
}

func TestAdvancedHostFormSavesEverySection(t *testing.T) {
	m := testApp(t)
	m.config = sshconfig.Manager{
		Home: m.paths.Home, MainConfig: m.paths.MainConfig, ManagedDir: m.paths.ManagedDir,
		ManagedConfig: m.paths.ManagedConfig, ManagedKeys: m.paths.ManagedKeys,
	}
	m.openAddHostForm()
	values := map[string]string{
		"Label": "Advanced host", "Hostname": "advanced.example", "Proxy jump": "bastion",
		"Agent forwarding": "yes", "Startup command": "tmux attach", "Request TTY": "force",
		"Local forwards": "8080 localhost:80", "Remote forwards": "9090 localhost:90",
		"Dynamic forward": "1080", "Compression": "yes", "Keepalive (seconds)": "30",
		"Environment variables": "FOO=bar; CSV=a,b", "Custom SSH flags": "TCPKeepAlive yes",
	}
	for label, value := range values {
		item := m.form.fieldByLabel(label)
		if item == nil {
			t.Fatalf("missing form field %q", label)
		}
		item.value = value
	}
	m.submitForm()

	config, err := os.ReadFile(m.paths.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Host Advanced_host", "ProxyJump bastion", "ForwardAgent yes", `RemoteCommand "tmux attach"`,
		"RequestTTY force", "LocalForward 8080 localhost:80", "RemoteForward 9090 localhost:90",
		"DynamicForward 1080", "Compression yes", "ServerAliveInterval 30", "SetEnv FOO=bar",
		"SetEnv CSV=a,b", "TCPKeepAlive yes",
	} {
		if !strings.Contains(string(config), want) {
			t.Fatalf("managed config missing %q:\n%s", want, config)
		}
	}
}

func TestFormRevealsFieldsProgressivelyAndRevisitsThem(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	initial := m.renderForm(m.styles())
	if !strings.Contains(initial, "Label") || !strings.Contains(initial, "Authentication") || !strings.Contains(initial, "User") {
		t.Fatalf("initial hub form layout is incorrect:\n%s", initial)
	}
	m.form.input.SetValue("prod")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	second := m.renderForm(m.styles())
	if !strings.Contains(second, "Label  prod") || !strings.Contains(second, "› Hostname") {
		t.Fatalf("Enter did not advance to hostname:\n%s", second)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.form.hubIndex != 0 {
		t.Fatalf("up did not revisit label: hubIndex=%d", m.form.hubIndex)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.form.hubIndex != 1 {
		t.Fatalf("down did not return to hostname: hubIndex=%d", m.form.hubIndex)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.form.hubIndex != hostHubIndex(t, m, "user") {
		t.Fatalf("down did not move to user: hubIndex=%d", m.form.hubIndex)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	third := m.renderForm(m.styles())
	if !strings.Contains(third, "› Authentication") {
		t.Fatal("down did not move to authentication menu")
	}
}

func TestHostFormExplainsOptionalConnectionFields(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()

	initial := m.renderForm(m.styles())
	if !strings.Contains(initial, "use / for groups") {
		t.Fatalf("label description is missing:\n%s", initial)
	}

	enterAdvancedSubsection(t, m, formSectionAdvancedJump)
	proxy := m.renderForm(m.styles())
	if !strings.Contains(proxy, "Route through a jump host") {
		t.Fatalf("proxy jump is not clearly explained as optional:\n%s", proxy)
	}
}

func TestHostLabelsKeepSpacesWhileSSHNamesUseUnderscores(t *testing.T) {
	m := testApp(t)
	m.config = sshconfig.Manager{
		Home: m.paths.Home, MainConfig: m.paths.MainConfig, ManagedDir: m.paths.ManagedDir,
		ManagedConfig: m.paths.ManagedConfig, ManagedKeys: m.paths.ManagedKeys,
	}
	m.openAddHostForm()
	for i := range m.form.fields {
		switch m.form.fields[i].label {
		case "Label":
			m.form.fields[i].value = "Production web"
		case "Hostname":
			m.form.fields[i].value = "prod.example"
		}
	}
	m.submitForm()

	config, err := os.ReadFile(m.paths.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "Host Production_web") {
		t.Fatalf("managed config does not use the safe SSH name:\n%s", config)
	}
	if got := m.metadata.Host("Production_web").Label; got != "Production web" {
		t.Fatalf("friendly label = %q", got)
	}

	host := sshconfig.Host{Alias: "Production_web", Managed: true}
	m.hosts = []sshconfig.Host{host}
	list := m.renderHosts(m.styles())
	if !strings.Contains(list, "Production web") || !strings.Contains(list, "SSH name") || !strings.Contains(list, "Production_web") {
		t.Fatalf("host view does not distinguish the friendly label and SSH name:\n%s", list)
	}
	m.openEditHostForm()
	if got := formFieldByLabel(m, "Label").value; got != "Production web" {
		t.Fatalf("edit form label = %q", got)
	}
	m.form = nil
	m.openDeleteHostForm()
	if m.form.input.Placeholder != "Production web" {
		t.Fatalf("delete placeholder = %q", m.form.input.Placeholder)
	}
}

func TestContrastingTextColor(t *testing.T) {
	tests := []struct {
		background string
		want       string
		valid      bool
	}{
		{background: "#000000", want: "#FFFFFF", valid: true},
		{background: "#fff", want: "#111827", valid: true},
		{background: "#7C3AED", want: "#FFFFFF", valid: true},
		{background: "#FFFF00", want: "#111827", valid: true},
		{background: "ffffff", valid: false},
		{background: "purple", valid: false},
	}
	for _, test := range tests {
		t.Run(test.background, func(t *testing.T) {
			got, ok := contrastingTextColor(test.background)
			if ok != test.valid || got != test.want {
				t.Fatalf("contrastingTextColor(%q) = %q, %v; want %q, %v", test.background, got, ok, test.want, test.valid)
			}
		})
	}
}

func TestHostFormSelectsDetectedKeysAndKeepsManualPathOption(t *testing.T) {
	m := testApp(t)
	managedPath := filepath.Join(m.paths.ManagedKeys, "work")
	m.keys = []keymodel.Key{
		{Name: "work", PrivatePath: managedPath, Managed: true, Algorithm: "ED25519"},
		{Name: "legacy", PrivatePath: filepath.Join(m.paths.SSHDir, "id_rsa")},
		{Name: "agent-only", Fingerprint: "SHA256:agent", InAgent: true},
	}
	m.openAddHostForm()
	enterHostFormSection(t, m, formSectionAuth)
	if m.form.fields[m.form.index].label != methodFieldLabel {
		t.Fatalf("method field was not focused: %+v", m.form)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.form.selecting {
		t.Fatal("Enter did not open the method picker")
	}
	view := m.renderForm(m.styles())
	if !strings.Contains(view, "OpenSSH defaults / agent") || !strings.Contains(view, "work · ~/.ssh/bast/keys/work") || !strings.Contains(view, "Manual path…") || !strings.Contains(view, "Password") {
		t.Fatalf("method picker is missing expected choices:\n%s", view)
	}
	if strings.Contains(view, "agent-only") {
		t.Fatalf("agent-only key was offered as a method:\n%s", view)
	}
	method := formFieldByLabel(m, methodFieldLabel)
	if method.options[len(method.options)-1].value != passwordOnlyIdentity {
		t.Fatalf("Password should be last: %+v", method.options)
	}

	m.updateForm(press("j"))
	if option := m.form.fields[m.form.index].options[m.form.fields[m.form.index].selected]; option.value != "~/.ssh/bast/keys/work" {
		t.Fatalf("j did not select the first key: %+v", option)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := formFieldByLabel(m, methodFieldLabel).value; got != "~/.ssh/bast/keys/work" {
		t.Fatalf("selected method = %q", got)
	}
	if !formFieldByLabel(m, passwordFieldLabel).hidden {
		t.Fatal("password field should stay hidden for a key")
	}

	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	for m.form.fields[m.form.index].options[m.form.fields[m.form.index].selected].value != passwordOnlyIdentity {
		prev := m.form.fields[m.form.index].selected
		m.updateForm(press("j"))
		if m.form.fields[m.form.index].selected == prev {
			t.Fatal("could not reach Password in the method picker")
		}
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form.fields[m.form.index].label != passwordFieldLabel {
		t.Fatal("choosing Password did not focus the password field")
	}
	if formFieldByLabel(m, passwordFieldLabel).hidden {
		t.Fatal("password field stayed hidden")
	}

	m = testApp(t)
	manualTestPath := filepath.Join(m.paths.ManagedKeys, "work")
	m.keys = []keymodel.Key{{Name: "work", PrivatePath: manualTestPath}}
	m.openAddHostForm()
	enterHostFormSection(t, m, formSectionAuth)
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m.updateForm(press("j"))
	m.updateForm(press("j"))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form.selecting || m.form.fields[m.form.index].label != methodFieldLabel {
		t.Fatal("manual choice did not switch the picker to path entry")
	}
	m.form.input.SetValue("~/.ssh/special_key")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	methodIdx := m.form.fieldIndex(methodFieldLabel)
	if !m.form.selecting || m.form.fields[methodIdx].customValue != "~/.ssh/special_key" {
		t.Fatal("Esc did not return manual path entry to the key choices")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := formFieldByLabel(m, methodFieldLabel).value; got != "~/.ssh/special_key" {
		t.Fatalf("manual identity = %q", got)
	}

	m = testApp(t)
	editPath := filepath.Join(m.paths.ManagedKeys, "work")
	m.keys = []keymodel.Key{{Name: "work", PrivatePath: editPath}}
	m.hosts = []sshconfig.Host{{Alias: "alpha", Managed: true, ManagedID: "alpha", Resolved: sshconfig.Resolved{IdentityFiles: []string{editPath}}}}
	m.openEditHostForm()
	method = formFieldByLabel(m, methodFieldLabel)
	if method.options[method.selected].value != "~/.ssh/bast/keys/work" {
		t.Fatalf("existing detected identity was not preselected: %+v", method)
	}
	enterHostFormSection(t, m, formSectionMetadata)
	rendered := m.renderForm(m.styles())
	if !strings.Contains(rendered, "Notes") {
		t.Fatal("metadata section did not reveal every metadata field")
	}

	m.hosts[0].Resolved = sshconfig.Resolved{PubkeyAuthentication: "no", PasswordAuthentication: "yes"}
	m.openEditHostForm()
	method = formFieldByLabel(m, methodFieldLabel)
	if method.options[method.selected].value != passwordOnlyIdentity {
		t.Fatalf("password authentication was not preselected: %+v", method)
	}
}

func TestHostFormStoresAndClearsPasswords(t *testing.T) {
	m := testApp(t)
	m.config = sshconfig.Manager{
		Home: m.paths.Home, MainConfig: m.paths.MainConfig, ManagedDir: m.paths.ManagedDir,
		ManagedConfig: m.paths.ManagedConfig, ManagedKeys: m.paths.ManagedKeys,
	}
	m.openAddHostForm()
	m.form.fieldByLabel("Label").value = "legacy"
	m.form.fieldByLabel("Hostname").value = "legacy.example"
	m.form.fieldByLabel(methodFieldLabel).value = passwordOnlyIdentity
	pwd := m.form.fieldByLabel(passwordFieldLabel)
	pwd.value = "s3cret"
	pwd.hidden = false
	m.submitForm()
	if m.statusError {
		t.Fatalf("save failed: %s", m.status)
	}
	hosts, err := m.config.Discover()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("hosts = %v err=%v", hosts, err)
	}
	id := hosts[0].ManagedID
	got, err := os.ReadFile(filepath.Join(m.paths.PasswordsDir, id))
	if err != nil || strings.TrimSpace(string(got)) != "s3cret" {
		t.Fatalf("stored password = %q err=%v", got, err)
	}
	config, err := os.ReadFile(m.paths.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), "PubkeyAuthentication no") || strings.Contains(string(config), "s3cret") {
		t.Fatalf("managed config = %s", config)
	}

	m.hosts = hosts
	m.hosts[0].Resolved = sshconfig.Resolved{HostName: "legacy.example", PubkeyAuthentication: "no", PasswordAuthentication: "yes"}
	m.openEditHostForm()
	if formFieldByLabel(m, passwordFieldLabel).hidden {
		t.Fatal("stored password did not reveal the password field")
	}
	if formFieldByLabel(m, passwordFieldLabel).value != passwordKeepValue {
		t.Fatal("edit did not default to keeping the stored password")
	}
	m.form.fieldByLabel(passwordFieldLabel).value = ""
	m.submitForm()
	got, err = os.ReadFile(filepath.Join(m.paths.PasswordsDir, id))
	if err != nil || strings.TrimSpace(string(got)) != "s3cret" {
		t.Fatalf("blank edit cleared the password: %q err=%v", got, err)
	}

	m.openEditHostForm()
	method := m.form.fieldByLabel(methodFieldLabel)
	method.value = ""
	method.selected = 0
	m.syncHostPasswordField()
	m.submitForm()
	if _, err := os.Stat(filepath.Join(m.paths.PasswordsDir, id)); !os.IsNotExist(err) {
		t.Fatalf("switching method left the password file: %v", err)
	}
}

func TestSelectedPublicKeyCanOpenServerPickerFromKeyOrMouse(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", PublicPath: filepath.Join(m.paths.ManagedKeys, "work.pub")}}
	m.hosts[0].Resolved = sshconfig.Resolved{HostName: "alpha.example", User: "deploy", Port: "22"}

	m.Update(press("u"))
	if m.form == nil || m.form.action != "key_install" || len(m.form.fields[1].options) != 2 {
		t.Fatalf("server picker was not opened: %+v", m.form)
	}
	if got := m.form.fields[1].options[0]; got.value != "alpha" || !strings.Contains(got.label, "deploy@alpha.example") {
		t.Fatalf("first server option = %+v", got)
	}

	m.form = nil
	listWidth, _, _ := m.columnDimensions()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: listWidth + 3, Y: keyInstallActionRow + 2, Button: tea.MouseLeft}))
	if m.form == nil || m.form.action != "key_install" {
		t.Fatal("clicking Add to server did not open the server picker")
	}
}

func TestImportFormDetectsPastedPrivateKey(t *testing.T) {
	m := testApp(t)
	m.openImportForm()
	content := "-----BEGIN OPENSSH PRIVATE KEY-----\nsecret-data\n-----END OPENSSH PRIVATE KEY-----\n"
	m.Update(tea.PasteMsg{Content: content})
	if m.form.pastedPrivateKey != content || m.form.fields[m.form.index].label != "Public key" {
		t.Fatal("pasted private key was not detected and advanced to the public-key field")
	}
	view := m.renderForm(m.styles())
	if strings.Contains(view, "secret-data") || !strings.Contains(view, "Pasted private key") {
		t.Fatal("the form exposed pasted key contents")
	}
	public := "ssh-ed25519 AAA-public imported\n"
	m.Update(tea.PasteMsg{Content: public})
	if m.form.pastedPublicKey != public || m.form.fields[m.form.index].label != "Comment" {
		t.Fatal("pasted public key was not detected and advanced to the comment field")
	}

	m = testApp(t)
	m.openImportForm()
	m.Update(tea.PasteMsg{Content: content})
	m.Update(tea.PasteMsg{Content: "ssh-ed25519 AAA-public existing comment"})
	if m.form.input.Value() != "existing comment" {
		t.Fatalf("existing public-key comment was not offered for editing: %q", m.form.input.Value())
	}
}

func TestKeysDoNotPresentAgentLoadingAsAPrimaryAction(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", PrivatePath: "/tmp/work"}}
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil {
		t.Fatal("Enter unexpectedly created an ssh-agent command")
	}
	help := m.renderHelp(m.styles())
	if strings.Contains(help, "load/unload") || strings.Contains(help, "l load") {
		t.Fatal("agent loading is still presented as a key action")
	}
}

func TestSSHProcessPreservesTerminalOutput(t *testing.T) {
	if !strings.Contains(connectbanner.Banner, "Press Enter, then ~.") {
		t.Fatal("connection banner does not explain how to force-close a stuck session")
	}
	var output bytes.Buffer
	cmd := processFixtureCommand(t, "session-output")
	cmd.Stdout = &output
	prepared := false
	process := &connectionProcess{cmd: cmd, prepare: func(status func(string)) error {
		if got := output.String(); got != connectbanner.Banner {
			t.Fatalf("connection banner was not shown before preparation: %q", got)
		}
		status("Publishing Google SSH key to the VM. This can take a few seconds…")
		prepared = true
		return nil
	}}
	if err := process.Run(); err != nil {
		t.Fatal(err)
	}
	if !prepared {
		t.Fatal("connection preparation did not run")
	}
	var want bytes.Buffer
	connectbanner.Write(&want)
	connectbanner.Status(&want)("Publishing Google SSH key to the VM. This can take a few seconds…")
	want.WriteString("\r\nsession-output")
	if got := output.String(); got != want.String() {
		t.Fatalf("output = %q\nwant %q", got, want.String())
	}
	if strings.Contains(output.String(), "Press any key to continue") {
		t.Fatal("successful session should not show the continue prompt")
	}
	if !strings.Contains(output.String(), "\x1b[38;2;107;114;128m Publishing") {
		t.Fatal("status line should be muted and indented with a leading space")
	}
}

func TestSSHProcessPausesOnFailure(t *testing.T) {
	var output bytes.Buffer
	cmd := processFixtureCommand(t, "ssh-failure")
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.Stdin = strings.NewReader("k")
	process := &connectionProcess{cmd: cmd}
	err := process.Run()
	if err == nil {
		t.Fatal("expected SSH process failure")
	}
	got := output.String()
	if !strings.Contains(got, "host key verification failed") {
		t.Fatalf("missing SSH output: %q", got)
	}
	idx := strings.Index(got, "host key verification failed")
	if idx < 0 || !strings.Contains(got[idx:], connectbanner.ContinuePrompt) {
		t.Fatalf("continue prompt should appear after SSH error output:\n%q", got)
	}
}

func TestSSHProcessPausesOnPrepareFailure(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "")
	var output bytes.Buffer
	cmd := processFixtureCommand(t, "should-not-run")
	cmd.Stdout = &output
	cmd.Stdin = strings.NewReader("k")
	process := &connectionProcess{cmd: cmd, prepare: func(status func(string)) error {
		status("Publishing Google SSH key…")
		return errors.New("gcp access denied")
	}}
	err := process.Run()
	if err == nil || !strings.Contains(err.Error(), "gcp access denied") {
		t.Fatalf("prepare error = %v", err)
	}
	got := output.String()
	if strings.Contains(got, "should-not-run") {
		t.Fatal("ssh should not run after prepare failure")
	}
	if !strings.Contains(got, telemetry.ReportPrompt) {
		t.Fatalf("missing error report prompt after prepare failure:\n%q", got)
	}
	failure := "Connection failed: gcp access denied"
	if !strings.Contains(got, failure) || strings.Index(got, failure) > strings.Index(got, telemetry.ReportPrompt) {
		t.Fatalf("prepare error should appear before the report prompt:\n%q", got)
	}
}

func TestSSHSessionCompletionReturnsToBast(t *testing.T) {
	exitCmd := processFixtureCommand(t, "exit-255")
	exitErr := exitCmd.Run()
	for name, sessionErr := range map[string]error{
		"logout":          nil,
		"connection lost": errors.New("connection lost"),
		"exit 255":        exitErr,
	} {
		t.Run(name, func(t *testing.T) {
			m := testApp(t)
			m.status, m.statusError = "Connected", true
			_, cmd := m.Update(processDoneMsg{name: "SSH session", err: sessionErr, sshSession: true})
			if cmd == nil {
				t.Fatal("SSH completion did not reload hosts")
			}
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Fatal("SSH completion quit Bast")
			}
			if !m.loading || m.statusError || m.status != "" {
				t.Fatalf("SSH completion did not return quietly: loading=%v status=%q error=%v", m.loading, m.status, m.statusError)
			}
		})
	}

	m := testApp(t)
	_, cmd := m.Update(processDoneMsg{name: "Key generation", err: errors.New("keygen failed")})
	if cmd == nil || !m.statusError || !strings.Contains(m.status, "keygen failed") {
		t.Fatalf("non-SSH process failure should still show an error overlay, status=%q", m.status)
	}
}

func TestManagedKeyCommentCanBeEditedAfterImport(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", Comment: "old comment", Managed: true, PublicPath: filepath.Join(m.paths.ManagedKeys, "work.pub")}}
	m.Update(press("e"))
	if m.form == nil || m.form.action != "key_comment" || m.form.input.Value() != "old comment" {
		t.Fatalf("key comment form was not opened: %+v", m.form)
	}
	m.form = nil
	m.Update(press("d"))
	deleteForm := m.renderForm(m.styles())
	if !strings.Contains(deleteForm, "Type the name to confirm") {
		t.Fatalf("delete confirmation copy is incorrect:\n%s", deleteForm)
	}
	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "󰌑 delete") || strings.Contains(footer, "󰌑 save") {
		t.Fatalf("delete footer is incorrect: %q", footer)
	}
	if m.form.input.Placeholder != "work" || !strings.Contains(deleteForm, "work") {
		t.Fatalf("delete confirmation does not show the required name as a placeholder:\n%s", deleteForm)
	}
}

func TestDeletionFormsUseTheExactConfirmationAsAPlaceholder(t *testing.T) {
	m := testApp(t)
	m.hosts[0].Managed = true
	m.openDeleteHostForm()
	if m.form.input.Placeholder != "alpha" {
		t.Fatalf("host deletion placeholder = %q", m.form.input.Placeholder)
	}

	m.form = nil
	m.openKnownHostForm()
	if m.form.input.Placeholder != "alpha" {
		t.Fatalf("known-host deletion placeholder = %q", m.form.input.Placeholder)
	}
}

func TestDeletionConfirmationMismatchStaysInline(t *testing.T) {
	m := testApp(t)
	m.hosts[0].Managed = true
	m.openDeleteHostForm()
	m.form.input.SetValue("wrong-name")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.statusError || m.form == nil {
		t.Fatal("confirmation mismatch left the form")
	}

	rendered := m.render()
	for _, expected := range []string{"Delete host: alpha", "Name to type", "alpha", "Name does not match the host label", "Type the name to confirm"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("inline confirmation error is missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "Action failed") {
		t.Fatalf("confirmation mismatch opened the global error screen:\n%s", rendered)
	}

	m.Update(press("backspace"))
	if m.form.validationError != "" {
		t.Fatal("editing did not clear the inline confirmation error")
	}
}

func TestDeletionConfirmationNameCanBeCopied(t *testing.T) {
	m := testApp(t)
	m.hosts[0].Managed = true
	m.openDeleteHostForm()
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("delete form captured mouse input instead of allowing terminal text selection")
	}

	_, cmd := m.Update(press("ctrl+y"))
	if cmd == nil {
		t.Fatal("copying the confirmation name returned no command")
	}
	if rendered := m.render(); !strings.Contains(rendered, "Name to type") || !strings.Contains(rendered, "Ctrl+Y copy name") {
		t.Fatalf("copy affordance is not visible:\n%s", rendered)
	}

	m.form = nil
	m.section = keysSection
	m.keys = []keymodel.Key{{Name: "work", Managed: true}}
	m.openDeleteKeyForm()
	if target := destructiveConfirmationTarget(m.form); target != "work" {
		t.Fatalf("key confirmation copy target = %q", target)
	}
	m.form.input.SetValue("wrong-name")
	m.Update(press("enter"))
	if m.form == nil || m.statusError || !strings.Contains(m.render(), "Name does not match the key name") {
		t.Fatal("key confirmation mismatch did not stay inline")
	}
}

func TestErrorOverlaySpaceSendsReport(t *testing.T) {
	t.Setenv("BAST_NO_TELEMETRY", "")

	got := make(chan telemetry.Report, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body telemetry.Report
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		got <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(telemetry.SetErrorEndpoint(server.URL))

	m := testApp(t)
	m.version = "v1.2.3"
	m.status, m.statusError = "sync failed", true
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if m.statusError || m.status != "Sending report…" {
		t.Fatalf("Space should start async send: status=%q error=%v", m.status, m.statusError)
	}
	if cmd == nil {
		t.Fatal("expected report command")
	}
	msg := cmd()
	result, ok := msg.(reportResultMsg)
	if !ok {
		t.Fatalf("unexpected msg %T", msg)
	}
	if result.err != nil {
		t.Fatalf("report error: %v", result.err)
	}
	_, cmd = m.Update(result)
	if cmd == nil {
		t.Fatal("expected notice command")
	}
	_ = cmd()
	if m.statusError || m.status != "Report sent" {
		t.Fatalf("report result should show notice: status=%q error=%v", m.status, m.statusError)
	}

	select {
	case body := <-got:
		if body.Message != "sync failed" || body.Context != "tui" || body.Version != "v1.2.3" {
			t.Fatalf("unexpected report: %+v", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error report")
	}

	t.Setenv("BAST_NO_TELEMETRY", "1")
	m.status, m.statusError = "sync failed", true
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if m.statusError || m.status != "" {
		t.Fatalf("Space with telemetry disabled should dismiss: status=%q error=%v", m.status, m.statusError)
	}
}

func TestDotToggleKeepsHostCursor(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Hidden: true}); err != nil {
		t.Fatal(err)
	}
	m.cursor = 0
	host, ok := m.selectedHost()
	if !ok || host.Alias != "beta" {
		t.Fatalf("expected beta selected, got %+v ok=%v", host, ok)
	}
	m.Update(press("."))
	host, ok = m.selectedHost()
	if !ok || host.Alias != "beta" {
		t.Fatalf(". jumped cursor to %+v ok=%v cursor=%d", host, ok, m.cursor)
	}
}

func TestSyncGridIndentsEveryTileLine(t *testing.T) {
	m := testApp(t)
	m.width = 80
	m.section = syncSection
	body := m.renderSync(m.styles())
	var boxLines []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "┌") || strings.Contains(line, "│") || strings.Contains(line, "└") {
			boxLines = append(boxLines, line)
		}
	}
	if len(boxLines) < 4 {
		t.Fatalf("expected boxed grid:\n%s", body)
	}
	leading := func(s string) int {
		n := 0
		for _, r := range s {
			if r != ' ' {
				break
			}
			n++
		}
		return n
	}
	want := leading(boxLines[0])
	if want < 2 {
		t.Fatalf("expected indented tiles, got %d\n%s", want, body)
	}
	for _, line := range boxLines {
		if got := leading(line); got != want {
			t.Fatalf("indent %d != %d\n%q\n%s", got, want, line, body)
		}
	}
}

func TestHelpUsesTerminalWidthWithoutWrapping(t *testing.T) {
	m := testApp(t)
	m.width = 100
	if got := m.helpContentWidth(); got < 80 {
		t.Fatalf("helpContentWidth = %d", got)
	}
	for _, line := range m.helpLines(m.styles()) {
		if strings.Contains(line, "\n") {
			t.Fatalf("help line wrapped internally: %q", line)
		}
	}
}

func TestSyncGridStaysBoxedOnMobile(t *testing.T) {
	m := testApp(t)
	m.width = 50
	m.section = syncSection
	if !m.isMobileLayout() {
		t.Fatal("expected mobile layout")
	}
	if m.syncGridCols() != 1 {
		t.Fatalf("mobile grid cols = %d", m.syncGridCols())
	}
	body := m.renderSync(m.styles())
	if strings.Count(body, "┌") != 5 {
		t.Fatalf("mobile should keep one boxed tile per provider:\n%s", body)
	}
	if !strings.Contains(body, " Cloud") || !strings.Contains(body, "Box") || !strings.Contains(body, "Upstash") {
		t.Fatalf("mobile grid body:\n%s", body)
	}
}

func TestSyncTileLinesAlignWhenSelected(t *testing.T) {
	m := testApp(t)
	item := syncMenuItem{label: "GCP", detail: "disabled", provider: "gcp"}
	selected := m.renderSyncTile(m.styles(), 0, item, 24)
	plain := m.renderSyncTile(m.styles(), 1, item, 24)
	for _, tile := range []string{selected, plain} {
		lines := strings.Split(strings.TrimRight(tile, "\n"), "\n")
		if len(lines) != 4 {
			t.Fatalf("tile lines = %d\n%s", len(lines), tile)
		}
		width := lipgloss.Width(lines[0])
		for i, line := range lines {
			if lipgloss.Width(line) != width {
				t.Fatalf("line %d width %d != %d\n%s", i, lipgloss.Width(line), width, tile)
			}
		}
		if !strings.Contains(tile, "┌") || !strings.Contains(tile, " Cloud") {
			t.Fatalf("expected boxed branded tile:\n%s", tile)
		}
	}
}

func TestHostsCanBeHiddenAndTemporarilyShown(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Hidden: true}); err != nil {
		t.Fatal(err)
	}
	visible := m.filteredHosts()
	if len(visible) != 1 || visible[0].Alias != "beta" {
		t.Fatalf("hidden host remained visible: %+v", visible)
	}
	m.Update(press("."))
	if !m.showHidden || len(m.filteredHosts()) != 2 {
		t.Fatal(". did not reveal hidden hosts")
	}
	if !strings.Contains(m.renderHosts(m.styles()), "◌ alpha") {
		t.Fatal("hidden host was not marked in the list")
	}
	m.cursor = 0
	m.Update(press("h"))
	if m.metadata.Host("alpha").Hidden {
		t.Fatal("h did not restore the selected host")
	}
}

func TestHiddenHostsConcealedStatusClears(t *testing.T) {
	if noticeDuration < 3*time.Second || noticeDuration > 5*time.Second {
		t.Fatalf("notice duration = %s", noticeDuration)
	}
	m := testApp(t)
	m.showHidden = true
	_, cmd := m.Update(press("."))
	if cmd == nil || m.status != "Hidden and stopped hosts concealed" {
		t.Fatal("concealing hidden hosts did not schedule the status to clear")
	}
	statusID := m.statusID
	m.Update(clearStatusMsg(statusID - 1))
	if m.status == "" {
		t.Fatal("a stale timer cleared the current status")
	}
	m.Update(clearStatusMsg(statusID))
	if m.status != "" {
		t.Fatalf("status was not cleared: %q", m.status)
	}
}

func TestMobileLayoutStacksPanelsVertically(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 40, 24
	body := m.renderHosts(m.styles())
	if strings.Contains(body, "│") {
		t.Fatalf("mobile layout should not use a vertical divider:\n%s", body)
	}
	firstAlpha := strings.Index(body, "alpha")
	secondAlpha := strings.Index(body[firstAlpha+len("alpha"):], "alpha")
	if firstAlpha < 0 || secondAlpha < 0 {
		t.Fatal("host list and details were not rendered")
	}
	if secondAlpha <= firstAlpha {
		t.Fatalf("details should appear below the list in mobile layout:\n%s", body)
	}
	if !strings.Contains(body, connectAction) {
		t.Fatalf("mobile host details are missing the connect button:\n%s", body)
	}
	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "enter connect") || !strings.Contains(footer, "?") {
		t.Fatalf("mobile footer = %q", footer)
	}
	if strings.Contains(footer, "enter/click Connect") || strings.Contains(footer, "F files") {
		t.Fatalf("mobile footer should stay compact: %q", footer)
	}
}

func TestMobileListScrollsWithArrowKeys(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 40, 24
	for i := range 20 {
		m.hosts = append(m.hosts, sshconfig.Host{Alias: fmt.Sprintf("host-%02d", i)})
	}
	m.cursor = 0
	for range 8 {
		m.Update(press("j"))
	}
	if m.cursor != 8 {
		t.Fatalf("j did not move the cursor in mobile layout: cursor=%d", m.cursor)
	}
	layout := m.panelLayout()
	start := scrollStart(m.cursor, m.itemCount(), layout.listHeight)
	if start == 0 {
		t.Fatal("mobile list did not scroll to keep the cursor visible")
	}
}

func TestMobileListHasClickableScrollbar(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 40, 24
	for i := range 20 {
		m.hosts = append(m.hosts, sshconfig.Host{Alias: fmt.Sprintf("host-%02d", i)})
	}
	layout := m.panelLayout()
	body := m.renderHosts(m.styles())
	if !strings.Contains(body, "↑") || !strings.Contains(body, "┃") || !strings.Contains(body, "↓") {
		t.Fatalf("mobile scrollbar is missing:\n%s", body)
	}

	x := layout.listWidth - 1
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: layout.listTop + layout.listHeight - 1, Button: tea.MouseLeft}))
	if m.cursor != m.itemCount()-1 {
		t.Fatalf("scrollbar down button selected row %d, want %d", m.cursor, m.itemCount()-1)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: layout.listTop + layout.listHeight/2, Button: tea.MouseLeft}))
	if m.cursor <= 0 || m.cursor >= m.itemCount()-1 {
		t.Fatalf("scrollbar track selected unexpected row %d", m.cursor)
	}
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x - 2, Y: layout.listTop + layout.listHeight - 2, Button: tea.MouseLeft}))
	if m.cursor != m.itemCount()-1 {
		t.Fatalf("dragging the scrollbar selected row %d, want %d", m.cursor, m.itemCount()-1)
	}
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: x - 2, Y: layout.listTop + layout.listHeight - 2, Button: tea.MouseLeft}))
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: layout.listTop + 1, Button: tea.MouseLeft}))
	if m.cursor != m.itemCount()-1 {
		t.Fatalf("scrollbar kept dragging after release: row %d", m.cursor)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: layout.listTop, Button: tea.MouseLeft}))
	if m.cursor != 0 {
		t.Fatalf("scrollbar up button selected row %d, want 0", m.cursor)
	}
}

func TestMobileListScrollsWithTouchWheel(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 40, 24
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 2, Y: m.panelLayout().listTop, Button: tea.MouseWheelDown}))
	if m.cursor != 1 {
		t.Fatalf("wheel down selected row %d, want 1", m.cursor)
	}
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 2, Y: m.panelLayout().listTop, Button: tea.MouseWheelUp}))
	if m.cursor != 0 {
		t.Fatalf("wheel up selected row %d, want 0", m.cursor)
	}
}

func TestDesktopConnectButtonIsVisible(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 24
	layout := m.panelLayout()
	if difference := layout.listWidth - layout.detailWidth; difference < -1 || difference > 1 {
		t.Fatalf("desktop panels are not equal width: list=%d detail=%d", layout.listWidth, layout.detailWidth)
	}
	body := m.renderHosts(m.styles())
	if !strings.Contains(body, connectAction) {
		t.Fatalf("desktop host details are missing the connect button:\n%s", body)
	}
}

func TestDesktopConnectButtonIsClickable(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 24
	layout := m.panelLayout()
	btnX, btnY, _ := m.connectButtonBounds(layout)
	if btnX <= layout.listWidth {
		t.Fatalf("connect button x=%d should be in the detail panel (listWidth=%d)", btnX, layout.listWidth)
	}
	if btnY != layout.detailTop+connectActionRow {
		t.Fatalf("connect chip y=%d, want %d", btnY, layout.detailTop+connectActionRow)
	}
	_, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: btnX, Y: btnY, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("desktop connect button click did not trigger connect")
	}
}

func TestMobileConnectButtonIsClickable(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 40, 24
	layout := m.panelLayout()
	btnX, btnY, _ := m.connectButtonBounds(layout)
	m.Update(tea.MouseClickMsg(tea.Mouse{X: btnX, Y: btnY, Button: tea.MouseLeft}))
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 2, Y: layout.listTop + 1, Button: tea.MouseLeft}))
	if m.cursor != 1 {
		t.Fatalf("mobile list click selected row %d, want 1", m.cursor)
	}
}

func TestMainLayoutKeepsDetailsRightAndFooterAtBottom(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 24
	body := m.renderHosts(m.styles())
	if strings.Count(body, "│") != m.height-3 {
		t.Fatalf("divider height = %d, want %d", strings.Count(body, "│"), m.height-3)
	}
	firstAlias := strings.Index(body, "alpha")
	divider := strings.Index(body, "│")
	secondAlias := strings.Index(body[firstAlias+len("alpha"):], "alpha")
	if firstAlias < 0 || divider < firstAlias || secondAlias < 0 {
		t.Fatalf("host list and details were not rendered side by side:\n%s", body)
	}
	rendered := m.render()
	if strings.Count(rendered, "┬") != 1 {
		t.Fatalf("header rule is missing its column junction:\n%s", rendered)
	}
	if lipgloss.Height(rendered) != m.height {
		t.Fatalf("render height = %d, want %d", lipgloss.Height(rendered), m.height)
	}
	lines := strings.Split(rendered, "\n")
	if !strings.Contains(lines[len(lines)-1], "?") {
		t.Fatalf("footer was not on the bottom row: %q", lines[len(lines)-1])
	}
}

func TestInjectedVersionAppearsAtTopRight(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 24
	m.version = "v1.2.3"
	header := strings.Split(m.render(), "\n")[0]
	if !strings.Contains(header, "v1.2.3") || lipgloss.Width(header) != m.width {
		t.Fatalf("versioned header = %q", header)
	}

	m.version = "dev"
	header = strings.Split(m.render(), "\n")[0]
	if strings.Contains(header, "dev") {
		t.Fatalf("development header exposed version: %q", header)
	}
}

func TestCreditsScreenShowsAttributionAndBuildDetails(t *testing.T) {
	m := testApp(t)
	m.version = "v1.2.3"
	m.Update(press("v"))
	if !m.credits {
		t.Fatal("v did not open the credits screen")
	}
	rendered := m.render()
	for _, text := range []string{
		"██████╗  █████╗",
		"Created by", "@tedbrine",
		"https://bast.sh",
		"github.com/ellipse-software/bast",
		"MIT License",
		"v1.2.3",
		"v / Esc / ⌫ close",
	} {
		if !strings.Contains(rendered, text) {
			t.Fatalf("credits screen does not contain %q:\n%s", text, rendered)
		}
	}
	for _, label := range []string{"Created by", "Website", "Repository", "License", "Version"} {
		for _, line := range strings.Split(rendered, "\n") {
			if strings.Contains(line, label) && lipgloss.Width(strings.TrimSpace(line)) != 52 {
				t.Fatalf("%s row is not fixed width:\n%s", label, line)
			}
		}
	}

	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.credits {
		t.Fatal("Esc did not close the credits screen")
	}
}

func TestHelpScreenIsSpacedAndScrollable(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 18
	m.Update(press("?"))
	if !m.help {
		t.Fatal("? did not open help")
	}
	rendered := m.render()
	for _, text := range []string{
		"Keyboard shortcuts",
		"Navigation",
		"Hosts",
		"Move selection",
		"↑/↓ scroll",
		"? / Esc / ⌫ close",
	} {
		if !strings.Contains(rendered, text) {
			t.Fatalf("help screen does not contain %q:\n%s", text, rendered)
		}
	}
	if strings.Contains(rendered, "Files") && strings.Contains(rendered, "Fuzzy jump") {
		t.Fatalf("hosts help should not include Files section:\n%s", rendered)
	}
	if !m.helpCanScroll() {
		t.Fatal("expected help content to require scrolling")
	}

	m.Update(press("j"))
	if m.helpOffset != 1 {
		t.Fatalf("j should scroll help down: offset=%d", m.helpOffset)
	}
	m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	if m.helpOffset != 4 {
		t.Fatalf("mouse wheel should scroll help: offset=%d", m.helpOffset)
	}
	m.Update(press("G"))
	if m.helpOffset != m.maxHelpOffset() {
		t.Fatalf("G should jump to end: offset=%d want=%d", m.helpOffset, m.maxHelpOffset())
	}
	bottom := m.render()
	if !strings.Contains(bottom, "During SSH") || !strings.Contains(bottom, "Force-close a stuck session") {
		t.Fatalf("scrolled help is missing the SSH section:\n%s", bottom)
	}
	m.Update(press("g"))
	if m.helpOffset != 0 {
		t.Fatalf("g should jump to top: offset=%d", m.helpOffset)
	}

	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.help || m.helpOffset != 0 {
		t.Fatalf("Esc should close help and reset scroll: help=%v offset=%d", m.help, m.helpOffset)
	}
}

func TestAvailableUpdateAppearsInFooterAndCredits(t *testing.T) {
	m := testApp(t)
	m.version = "v1.2.3"
	if m.checkForUpdateCmd() == nil {
		t.Fatal("stable release did not schedule an update check")
	}
	m.Update(updateAvailableMsg{version: "v1.3.0", suggestion: "https://bast.sh"})
	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "Update v1.3.0 · https://bast.sh") || lipgloss.Width(footer) > m.width {
		t.Fatalf("update reminder width=%d, terminal width=%d: %q", lipgloss.Width(footer), m.width, footer)
	}
	m.credits = true
	credits := m.renderCredits(m.styles())
	if !strings.Contains(credits, "Update") || !strings.Contains(credits, "v1.3.0 · https://bast.sh") {
		t.Fatalf("update reminder is missing from credits:\n%s", credits)
	}
	for _, line := range strings.Split(credits, "\n") {
		if strings.Contains(line, "Update") && lipgloss.Width(strings.TrimSpace(line)) != 52 {
			t.Fatalf("Update row is not fixed width:\n%s", line)
		}
	}

	m.version = "dev"
	if m.checkForUpdateCmd() != nil {
		t.Fatal("development build scheduled an update check")
	}
}

func TestEmptyHostListInvitesFirstHost(t *testing.T) {
	m := testApp(t)
	m.hosts = nil
	view := m.renderHosts(m.styles())
	if !strings.Contains(view, "No hosts yet") || !strings.Contains(view, "Press a to add your first destination") {
		t.Fatalf("empty host state is not helpful:\n%s", view)
	}
}

func TestDetailsAreCompactAndOmitEmptyMetadata(t *testing.T) {
	m := testApp(t)
	host := m.renderHostDetail(m.styles(), m.hosts[0], 50)
	if strings.Contains(host, "Label") || strings.Contains(host, "Color") || strings.Contains(host, "Group") || strings.Contains(host, "About") {
		t.Fatalf("host details contain redundant or empty fields:\n%s", host)
	}
	if !strings.Contains(host, "Access") || !strings.Contains(host, "Auth") {
		t.Fatalf("host details missing Access section:\n%s", host)
	}
	if !strings.Contains(host, "[p] Promote to Bast managed") {
		t.Fatalf("external host details missing promote action:\n%s", host)
	}
	if lipgloss.Height(host) > 13 {
		t.Fatalf("host details are too tall: %d lines\n%s", lipgloss.Height(host), host)
	}
	hostLines := strings.Split(host, "\n")
	if len(hostLines) < 3 || strings.Contains(hostLines[0], "Connect") || !strings.Contains(hostLines[2], "Connect") {
		t.Fatalf("Connect should sit under the host title:\n%s", host)
	}
	if strings.TrimSpace(hostLines[1]) != "" || (len(hostLines) > 3 && strings.TrimSpace(hostLines[3]) != "") {
		t.Fatalf("Connect chip should have a blank line above and below:\n%s", host)
	}
	key := m.renderKeyDetail(m.styles(), keymodel.Key{Name: "work", Algorithm: "ED25519", Fingerprint: "SHA256:test", PrivatePath: "/tmp/work", Managed: true}, 50)
	if strings.Contains(key, "Name") || strings.Contains(key, "Public") || strings.Contains(key, "Used by") {
		t.Fatalf("key details contain redundant or empty fields:\n%s", key)
	}
	if !strings.Contains(key, keyInstallAction) {
		t.Fatalf("key details do not show the server action:\n%s", key)
	}
	if lipgloss.Height(key) > 8 {
		t.Fatalf("key details are too tall: %d lines\n%s", lipgloss.Height(key), key)
	}
}

func TestHostDetailShowsConnectReadySections(t *testing.T) {
	m := testApp(t)
	used := time.Date(2026, 7, 20, 14, 2, 0, 0, time.Local)
	if err := m.metadata.SetHost("alpha", metadata.Host{
		Label: "Prod API", Favorite: true, Group: "Work/Prod", Environment: "production",
		Tags: []string{"web", "api"}, Notes: "via jump", LastUsedAt: &used, ConnectionCount: 12,
	}); err != nil {
		t.Fatal(err)
	}
	host := sshconfig.Host{
		Alias: "alpha", Managed: true, KnownHost: true,
		Resolved: sshconfig.Resolved{
			HostName: "api.prod.example", User: "ubuntu", Port: "22",
			IdentityFiles: []string{"~/.ssh/id_ed25519"}, ProxyJump: "bastion",
		},
	}
	detail := m.renderHostDetail(m.styles(), host, 60)
	for _, want := range []string{
		"Prod API", "ubuntu@api.prod.example", "◆ favorite", "Bast managed", "known host",
		"Access", "Auth", "~/.ssh/id_ed25519", "Jump", "bastion", "SSH name", "alpha",
		"About", "Group", "Work/Prod", "Env", "production", "Tags", "api, web", "Used", "Notes", "via jump",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail missing %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "Source") || strings.Contains(detail, "Meta") {
		t.Fatalf("detail still shows demoted fields:\n%s", detail)
	}
}

func TestHostAuthSummary(t *testing.T) {
	if got := hostAuthSummary(sshconfig.Host{}, false); got != "agent/defaults" {
		t.Fatalf("empty = %q", got)
	}
	if got := hostAuthSummary(sshconfig.Host{Synced: true}, false); got != "SSH access ensured on connect" {
		t.Fatalf("synced empty = %q", got)
	}
	got := hostAuthSummary(sshconfig.Host{Synced: true, Resolved: sshconfig.Resolved{
		User: "ubuntu", IdentityFiles: []string{"~/.ssh/bast/keys/IRIS"},
	}}, false)
	if got != "~/.ssh/bast/keys/IRIS" {
		t.Fatalf("synced key = %q", got)
	}
	passwordHost := sshconfig.Host{Resolved: sshconfig.Resolved{PubkeyAuthentication: "no", PasswordAuthentication: "yes"}}
	if got := hostAuthSummary(passwordHost, false); got != "password" {
		t.Fatalf("password = %q", got)
	}
	if got := hostAuthSummary(passwordHost, true); got != "password · saved" {
		t.Fatalf("saved password = %q", got)
	}
}

func TestSyncedAuthSummary(t *testing.T) {
	if got := syncedAuthSummary(sshconfig.Host{}); got != "SSH access ensured on connect" {
		t.Fatalf("empty = %q", got)
	}
	got := syncedAuthSummary(sshconfig.Host{Resolved: sshconfig.Resolved{
		User: "ubuntu", IdentityFiles: []string{"~/.ssh/bast/keys/IRIS"},
	}})
	if got != "ubuntu · ~/.ssh/bast/keys/IRIS" {
		t.Fatalf("matched = %q", got)
	}
}

func TestSyncedHostsAreReadOnly(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "gcp_demo_web", Synced: true, SyncSource: "gcp",
		SyncID:   "projects/demo/zones/us-central1-a/instances/web",
		Resolved: sshconfig.Resolved{HostName: "web", User: "ubuntu"},
	}}
	_ = m.metadata.SetHost("gcp_demo_web", metadata.Host{Label: "web", Group: "Google Cloud/Demo"})
	m.cursor = -1
	for i, row := range m.hostRows() {
		if !row.header && row.host.Alias == "gcp_demo_web" {
			m.cursor = i
			break
		}
	}
	if m.cursor < 0 {
		t.Fatal("synced host row not found")
	}
	m.openEditHostForm()
	if m.form != nil {
		t.Fatal("expected synced host edit to be blocked")
	}
	if !m.statusError || !strings.Contains(m.status, "read-only") {
		t.Fatalf("status = %q", m.status)
	}
	m.status, m.statusError = "", false
	m.openDeleteHostForm()
	if m.form != nil || !strings.Contains(m.status, "cannot be deleted") {
		t.Fatalf("delete status = %q form=%v", m.status, m.form != nil)
	}
	detail := m.renderHostDetail(m.styles(), m.hosts[0], 60)
	if !strings.Contains(detail, "GCP synced") {
		t.Fatalf("detail missing owner:\n%s", detail)
	}
}

func TestSyncedGroupRenameBlocked(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{Alias: "gcp_demo_web", Synced: true, SyncSource: "gcp"}}
	_ = m.metadata.SetHost("gcp_demo_web", metadata.Host{Label: "web", Group: "Google Cloud/Demo"})
	m.collapsedGroups = map[string]bool{}
	rows := m.hostRows()
	for i, row := range rows {
		if row.header && row.group == "Google Cloud/Demo" {
			m.cursor = i
			break
		}
	}
	m.openEditGroupForm()
	if m.form != nil || !strings.Contains(m.status, "cannot be renamed") {
		t.Fatalf("status = %q form=%v", m.status, m.form != nil)
	}
}

func TestEnterTogglesSelectedGroup(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}
	for i, row := range m.hostRows() {
		if row.header && row.group == "Work" {
			m.cursor = i
			break
		}
	}

	enter := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	m.updateKeys(enter)
	if !m.collapsedGroups["Work"] {
		t.Fatal("Enter did not collapse the selected group")
	}
	m.updateKeys(enter)
	if m.collapsedGroups["Work"] {
		t.Fatal("Enter did not expand the selected group")
	}

	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "␣ collapse") || !strings.Contains(footer, "?") {
		t.Fatalf("footer = %q", footer)
	}
	m.updateKeys(enter)
	footer = m.renderFooter(m.styles())
	if !strings.Contains(footer, "␣ expand") {
		t.Fatalf("collapsed footer = %q", footer)
	}
}

func TestCollapsedGroupsPersistAcrossSessions(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	m := &App{
		paths: p, openSSH: openssh.Default(), metadata: store,
		hosts: []sshconfig.Host{{Alias: "alpha"}, {Alias: "beta"}},
		width: 100, height: 30, dark: true, collapsedGroups: map[string]bool{},
	}
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = 0
	m.updateKeys(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if !m.collapsedGroups["Work"] {
		t.Fatal("expected Work to be collapsed")
	}
	prefs := m.metadata.Preferences().CollapsedGroups
	if len(prefs) != 1 || prefs[0] != "Work" {
		t.Fatalf("persisted collapsed groups = %v", prefs)
	}

	reopened, err := New(p, openssh.Default(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.collapsedGroups["Work"] {
		t.Fatalf("reopened session lost collapse state: %v", reopened.collapsedGroups)
	}
}

func TestCollapseAndExpandAllGroups(t *testing.T) {
	m := testApp(t)
	m.hosts = append(m.hosts, sshconfig.Host{Alias: "gamma"})
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("gamma", metadata.Host{Group: "Personal"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()

	m.updateKeys(tea.KeyPressMsg(tea.Key{Text: "["}))
	if !m.collapsedGroups["Work"] || !m.collapsedGroups["Personal"] {
		t.Fatalf("collapse all failed: %v", m.collapsedGroups)
	}
	for _, row := range m.hostRows() {
		if !row.header {
			t.Fatalf("collapse all still shows host row: %+v", row)
		}
	}
	prefs := m.metadata.Preferences().CollapsedGroups
	if len(prefs) != 2 || prefs[0] != "Personal" || prefs[1] != "Work" {
		t.Fatalf("persisted collapse all = %v", prefs)
	}

	m.updateKeys(tea.KeyPressMsg(tea.Key{Text: "]"}))
	if len(m.collapsedGroups) != 0 {
		t.Fatalf("expand all failed: %v", m.collapsedGroups)
	}
	if got := m.metadata.Preferences().CollapsedGroups; len(got) != 0 {
		t.Fatalf("persisted expand all = %v", got)
	}
}

func TestGoogleCloudGroupNameUsesGoogleColors(t *testing.T) {
	rendered := renderManagedGroupName("Google Cloud", lipgloss.NewStyle(), false)
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected ANSI colours: %q", rendered)
	}
	whiteCloud := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(" Cloud")
	if !strings.Contains(rendered, whiteCloud) {
		t.Fatalf("Cloud is not white: %q", rendered)
	}
	if width := lipgloss.Width(rendered); width != len("Google Cloud") {
		t.Fatalf("width = %d", width)
	}
	if got := renderManagedGroupName("Work", lipgloss.NewStyle(), false); got != "Work" {
		t.Fatalf("ordinary group changed: %q", got)
	}
}

func TestAmazonEC2GroupNameUsesOneBrandColor(t *testing.T) {
	restStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8B5CF6"))
	rendered := renderManagedGroupName("Amazon EC2/default/eu-west-2", restStyle, false)
	provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF9900")).Render("Amazon EC2")
	if !strings.HasPrefix(rendered, provider) {
		t.Fatalf("Amazon EC2 brand colour was not applied uniformly: %q", rendered)
	}
	if !strings.Contains(rendered, restStyle.Render("/default/eu-west-2")) {
		t.Fatalf("Amazon EC2 group path did not retain the normal style: %q", rendered)
	}
}

func TestMicrosoftAzureGroupNameUsesOneBrandColor(t *testing.T) {
	restStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	rendered := renderManagedGroupName("Microsoft Azure/Production/apps", restStyle, false)
	provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0078D4")).Render("Microsoft Azure")
	if !strings.HasPrefix(rendered, provider) {
		t.Fatalf("Microsoft Azure brand colour was not applied uniformly: %q", rendered)
	}
	if !strings.Contains(rendered, restStyle.Render("/Production/apps")) {
		t.Fatalf("Microsoft Azure group path did not retain the normal style: %q", rendered)
	}
}

func TestBoxGroupNameIsWhite(t *testing.T) {
	rendered := renderManagedGroupName("Box", lipgloss.NewStyle(), false)
	provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render("Box")
	if rendered != provider {
		t.Fatalf("Box brand colour was not applied: %q", rendered)
	}
}

func TestUpstashGroupNameUsesBrandColor(t *testing.T) {
	restStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	rendered := renderManagedGroupName("Upstash/dev", restStyle, false)
	provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00E9A3")).Render("Upstash")
	if !strings.HasPrefix(rendered, provider) {
		t.Fatalf("Upstash brand colour was not applied uniformly: %q", rendered)
	}
	if !strings.Contains(rendered, restStyle.Render("/dev")) {
		t.Fatalf("Upstash group path did not retain the normal style: %q", rendered)
	}
}

func TestManagedGroupNameCopiesRestBackground(t *testing.T) {
	rest := lipgloss.NewStyle().Background(lipgloss.Color("#1F2937"))
	rendered := renderManagedGroupName("Upstash", rest, false)
	want := brandText("#00E9A3", "Upstash", rest)
	if rendered != want {
		t.Fatalf("got %q want %q", rendered, want)
	}
	if !strings.Contains(rendered, "48;2;31;41;55") {
		t.Fatalf("expected rest background under Upstash brand text: %q", rendered)
	}
	icon := managedGroupIcon("Upstash", true)
	withIcon := renderManagedGroupName("Upstash", rest, true)
	if !strings.Contains(withIcon, brandText("#00E9A3", icon+"Upstash", rest)) {
		t.Fatalf("expected rest background under Upstash icon: %q", withIcon)
	}
}

func TestSelectedProviderGroupKeepsBackgroundUnderBrand(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "upstash_dev", Synced: true, SyncSource: "upstash",
		Resolved: sshconfig.Resolved{HostName: "dev.upstash.io", User: "root"},
	}}
	if err := m.metadata.SetHost("upstash_dev", metadata.Host{Label: "dev", Group: "Upstash"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = 0
	s := m.styles()
	body := m.renderHosts(s)
	want := brandText("#00E9A3", "Upstash", s.selected)
	if !strings.Contains(body, want) {
		t.Fatalf("selected Upstash label missing selected background:\nwant %q\n%s", want, body)
	}

	m.nerdFont = true
	body = m.renderHosts(s)
	icon := managedGroupIcon("Upstash", true)
	want = brandText("#00E9A3", icon+"Upstash", s.selected)
	if !strings.Contains(body, want) {
		t.Fatalf("selected Upstash icon missing selected background:\nwant %q\n%s", want, body)
	}
}

func TestSyncTileSelectedBorderUsesProviderColor(t *testing.T) {
	m := testApp(t)
	gcp := syncMenuItem{label: "GCP", detail: "disabled", provider: "gcp"}
	selected := m.renderSyncTile(m.styles(), 0, gcp, 24)
	plain := m.renderSyncTile(m.styles(), 1, gcp, 24)
	if !strings.Contains(selected, "38;2;66;133;244") {
		t.Fatalf("selected GCP tile should use Google blue border:\n%q", selected)
	}
	if strings.Contains(selected, "38;2;139;92;246") {
		t.Fatalf("selected GCP tile still uses purple:\n%q", selected)
	}
	if !strings.Contains(plain, "38;2;107;114;128") {
		t.Fatalf("unselected GCP tile should stay muted:\n%q", plain)
	}

	awsSel := m.renderSyncTile(m.styles(), 0, syncMenuItem{provider: "aws"}, 24)
	if !strings.Contains(awsSel, "38;2;255;153;0") {
		t.Fatalf("selected AWS tile should use orange border:\n%q", awsSel)
	}

	upSel := m.renderSyncTile(m.styles(), 0, syncMenuItem{provider: "upstash"}, 24)
	if !strings.Contains(upSel, "38;2;0;233;163") {
		t.Fatalf("selected Upstash tile should use green border:\n%q", upSel)
	}
}

func TestProviderGroupRowsUseBrandColor(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{
		{
			Alias: "gcp_demo_web", Synced: true, SyncSource: "gcp",
			Resolved: sshconfig.Resolved{HostName: "web", User: "ubuntu"},
		},
		{Alias: "alpha"},
	}
	if err := m.metadata.SetHost("gcp_demo_web", metadata.Host{Label: "web", Group: "Google Cloud/Demo"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	for i, row := range m.hostListRows() {
		if row.header && row.group == "Work" {
			m.cursor = i
			break
		}
	}
	body := m.renderHosts(m.styles())
	blue := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4285F4"))
	if !strings.Contains(body, blue.Render("▾ ")) {
		t.Fatalf("provider root arrow should be brand colour:\n%s", body)
	}
	if !strings.Contains(body, blue.Render("  ▾ ")) {
		t.Fatalf("nested provider group arrow should be brand colour:\n%s", body)
	}
	if !strings.Contains(body, blue.Render("Demo")) {
		t.Fatalf("nested provider group name should be brand colour:\n%s", body)
	}
	if strings.Contains(body, blue.Render("      web")) || strings.Contains(body, blue.Render("web")) {
		t.Fatalf("host name in the list should not use brand colour:\n%s", body)
	}
	if !strings.Contains(body, "web") {
		t.Fatalf("host name missing from list:\n%s", body)
	}
	if !strings.Contains(body, "▾ Work") {
		t.Fatalf("user group should stay unbranded:\n%s", body)
	}
	var gcpHost sshconfig.Host
	for _, host := range m.hosts {
		if host.Alias == "gcp_demo_web" {
			gcpHost = host
			break
		}
	}
	detail := m.renderHostDetail(m.styles(), gcpHost, 50)
	if !strings.Contains(detail, blue.Render("web")) {
		t.Fatalf("host title in the detail pane should use brand colour:\n%s", detail)
	}
}

func TestBoxCreateBusyThenSelectsHost(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	m.beginSyncBusy("Creating box…")
	m.syncingProviders = map[string]bool{"box": true}
	if !m.vaultBusyBlocksBody() {
		t.Fatal("expected create busy to replace the sync body")
	}
	busy := m.renderVaultBusy(m.styles())
	if !strings.Contains(busy, "box.ascii.dev") || !strings.Contains(busy, "Creating box…") {
		t.Fatalf("create busy body:\n%s", busy)
	}

	if err := m.metadata.SetHost("box_sunny", metadata.Host{Label: "sunny", Group: "Box"}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{"Box": true}

	_, _ = m.Update(syncDoneMsg{
		provider:   "box",
		result:     cloudsync.Result{Provider: "box", Count: 1},
		focusAlias: "box_sunny",
	})
	if m.syncBusy != "" {
		t.Fatalf("busy should clear after create, got %q", m.syncBusy)
	}
	if m.section != hostsSection || m.syncProvider != "" {
		t.Fatalf("expected Hosts after create, section=%v provider=%q", m.section, m.syncProvider)
	}
	if m.selectAfterLoadName != "box_sunny" {
		t.Fatalf("selectAfterLoadName = %q", m.selectAfterLoadName)
	}

	_, _ = m.Update(loadedMsg{hosts: []sshconfig.Host{{
		Alias: "box_sunny", Synced: true, SyncSource: "box",
		Resolved: sshconfig.Resolved{HostName: "1.2.3.4", User: "user"},
	}}})
	host, ok := m.selectedHost()
	if !ok || host.Alias != "box_sunny" {
		t.Fatalf("new box was not selected: ok=%v host=%+v cursor=%d", ok, host, m.cursor)
	}
	if m.collapsedGroups["Box"] {
		t.Fatalf("Box group should be expanded for selection: %v", m.collapsedGroups)
	}
}

func TestStoppedBoxHiddenUntilToggled(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{
		{
			Alias: "box_live", Synced: true, SyncSource: "box",
			Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"},
		},
		{
			Alias: "box_idle", Synced: true, SyncSource: "box",
			Resolved: sshconfig.Resolved{HostName: "box.stopped.invalid", User: "user"},
		},
	}
	if err := m.metadata.SetHost("box_live", metadata.Host{Label: "live", Group: "Box", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("box_idle", metadata.Host{Label: "idle", Group: "Box", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}

	var sawStopped, sawRunning bool
	for _, row := range m.hostRows() {
		if row.header {
			continue
		}
		switch row.host.Alias {
		case "box_live":
			sawRunning = true
		case "box_idle":
			sawStopped = true
		}
	}
	if !sawRunning {
		t.Fatal("running box should stay visible")
	}
	if sawStopped {
		t.Fatal("stopped box should be hidden by default")
	}

	m.showHidden = true
	sawStopped, sawRunning = false, false
	for _, row := range m.hostRows() {
		if row.header {
			continue
		}
		switch row.host.Alias {
		case "box_live":
			sawRunning = true
		case "box_idle":
			sawStopped = true
			if !m.hostLooksStopped(row.host) {
				t.Fatal("stopped box should look stopped")
			}
		}
	}
	if !sawRunning || !sawStopped {
		t.Fatalf("expected both boxes after ., running=%v stopped=%v", sawRunning, sawStopped)
	}

	m.showHidden = false
	m.search = "idle"
	sawStopped = false
	for _, row := range m.hostRows() {
		if !row.header && row.host.Alias == "box_idle" {
			sawStopped = true
		}
	}
	if !sawStopped {
		t.Fatal("search should reveal matching stopped boxes")
	}

	detail := m.renderHostDetail(m.styles(), m.hosts[1], 60)
	if !strings.Contains(detail, "stopped") || !strings.Contains(detail, "Box stopped") {
		t.Fatalf("stopped detail:\n%s", detail)
	}
	if !strings.Contains(detail, resumeAction) || strings.Contains(detail, connectAction) {
		t.Fatalf("stopped detail should offer Resume, not Connect:\n%s", detail)
	}
}

func TestBoxResumeActionsAreStateAware(t *testing.T) {
	m := testApp(t)
	running := sshconfig.Host{
		Alias: "box_live", Synced: true, SyncSource: "box", SyncID: "bx_live01",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"},
	}
	stopped := sshconfig.Host{
		Alias: "box_idle", Synced: true, SyncSource: "box", SyncID: "bx_idle01",
		Resolved: sshconfig.Resolved{HostName: "box.stopped.invalid", User: "user"},
	}
	m.hosts = []sshconfig.Host{running, stopped}
	if err := m.metadata.SetHost("box_live", metadata.Host{Label: "live", Group: "Box", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("box_idle", metadata.Host{Label: "idle", Group: "Box", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}
	m.showHidden = true

	selectHost := func(alias string) {
		t.Helper()
		m.cursor = -1
		for i, row := range m.hostRows() {
			if !row.header && row.host.Alias == alias {
				m.cursor = i
				return
			}
		}
		t.Fatalf("host %q not found", alias)
	}

	selectHost("box_live")
	footer := strings.Join(m.hostsFooterParts(), " · ")
	if strings.Contains(footer, "resume") || !strings.Contains(footer, "o stop") {
		t.Fatalf("running footer = %q", footer)
	}
	if m.hostPrimaryAction(running) != connectAction {
		t.Fatalf("running primary action = %q", m.hostPrimaryAction(running))
	}
	if cmd := m.resumeSelectedBox(running, false); cmd == nil {
		t.Fatal("expected notice cmd when resuming a running box")
	}
	// r on a running box should reload, not resume
	m.syncingProviders = map[string]bool{}
	_, _ = m.updateKeys(press("r"))
	if m.syncingProviders["box"] {
		t.Fatal("r on running box should not start resume")
	}

	selectHost("box_idle")
	footer = strings.Join(m.hostsFooterParts(), " · ")
	if !strings.Contains(footer, "enter connect") || !strings.Contains(footer, "r resume") || strings.Contains(footer, "o stop") {
		t.Fatalf("stopped footer = %q", footer)
	}
	if m.hostPrimaryAction(stopped) != resumeAction {
		t.Fatalf("stopped primary action = %q", m.hostPrimaryAction(stopped))
	}
	_, cmd := m.connectSelected()
	if !m.syncingProviders["box"] || m.syncActivity != "resuming…" {
		t.Fatalf("enter should resume with activity label, syncing=%v activity=%q", m.syncingProviders["box"], m.syncActivity)
	}
	if m.boxConnectAfter != "box_idle" {
		t.Fatalf("enter should queue SSH after resume, got %q", m.boxConnectAfter)
	}
	if cmd == nil {
		t.Fatal("expected resume command")
	}
	footerLeft := m.renderFooter(m.styles())
	if !strings.Contains(footerLeft, "resuming…") || strings.Contains(footerLeft, "syncing…") {
		t.Fatalf("footer during resume = %q", footerLeft)
	}

	// After a successful resume reload, connect into the woken box.
	m.syncingProviders = map[string]bool{}
	m.syncActivity = ""
	m.hosts = []sshconfig.Host{{
		Alias: "box_idle", Synced: true, SyncSource: "box", SyncID: "bx_idle01",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.20", User: "user"},
	}}
	if err := m.metadata.SetHost("box_idle", metadata.Host{Label: "idle", Group: "Box", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	m.selectAfterLoadSection, m.selectAfterLoadName = hostsSection, "box_idle"
	_, cmd = m.Update(loadedMsg{hosts: m.hosts})
	if m.boxConnectAfter != "" {
		t.Fatal("boxConnectAfter should clear once connect is queued")
	}
	if cmd == nil {
		t.Fatal("expected SSH connect command after resume reload")
	}
}

func TestSyncTabRenders(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	body := m.renderSync(m.styles())
	if strings.Contains(body, "Vault") {
		t.Fatalf("sync grid should not include Vault:\n%s", body)
	}
	if !strings.Contains(body, " Cloud") || !strings.Contains(body, "Amazon EC2") ||
		!strings.Contains(body, "Microsoft Azure") || !strings.Contains(body, "Box") {
		t.Fatalf("sync grid body:\n%s", body)
	}
	if strings.Contains(body, "Sync now") {
		t.Fatalf("sync grid should not show submenu actions:\n%s", body)
	}
	if !strings.Contains(body, "Import Compute Engine") {
		t.Fatalf("selected GCP tile should show description:\n%s", body)
	}

	m.updateSyncKeys("l")
	if m.syncCursor != 1 {
		t.Fatalf("l should move to AWS tile, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 3 {
		t.Fatalf("j should move to Box tile, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("h")
	if m.syncCursor != 2 {
		t.Fatalf("h should move to Azure tile, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("enter")
	if m.syncProvider != "azure" {
		t.Fatalf("expected Azure provider, got %q", m.syncProvider)
	}
	body = m.renderSync(m.styles())
	if !strings.Contains(body, "Subscription filter") || !strings.Contains(body, "Resource group filter") {
		t.Fatalf("azure provider body:\n%s", body)
	}
	if !strings.Contains(body, "Connect") {
		t.Fatalf("disabled provider should offer Connect:\n%s", body)
	}
	m.updateSyncKeys("esc")
	if m.syncProvider != "" {
		t.Fatalf("expected esc to return to sync grid, got %q", m.syncProvider)
	}
	m.syncCursor = 0
	m.updateSyncKeys("enter")
	if m.syncProvider != "gcp" {
		t.Fatalf("expected GCP provider, got %q", m.syncProvider)
	}
	body = m.renderSync(m.styles())
	if !strings.Contains(body, "Connect") || !strings.Contains(body, "Project filter") {
		t.Fatalf("gcp provider body:\n%s", body)
	}
	if strings.Contains(body, "Sync now") {
		t.Fatalf("primary should be Connect, not a Sync now list item:\n%s", body)
	}
	tabs := m.renderTabs(m.styles())
	if !strings.Contains(tabs, "[3] Vault") || !strings.Contains(tabs, "[4] Sync") || !strings.Contains(tabs, "[5] Files") {
		t.Fatalf("tabs = %q", tabs)
	}
}

func TestBoxProviderLifecycleRow(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	m.syncCursor = 0
	m.syncStatus.Box.Authenticated = true
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "New box") {
		t.Fatalf("box page should offer New box:\n%s", body)
	}
	if !strings.Contains(body, "disabled") && !strings.Contains(body, "enabled") {
		t.Fatalf("box page should show status:\n%s", body)
	}
	sameRow := false
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "New box") && (strings.Contains(line, "Sync") || strings.Contains(line, "Connect")) {
			sameRow = true
			break
		}
	}
	if !sameRow {
		t.Fatalf("Sync/Connect and New box should share a row:\n%s", body)
	}
	m.updateSyncKeys("l")
	if m.syncCursor != 1 {
		t.Fatalf("l should move to New box, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("enter")
	if m.form == nil || m.form.action != "box_new" {
		t.Fatalf("enter on New box should open form, got %#v", m.form)
	}
}

func TestUpstashProviderLifecycleRow(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "upstash"
	m.syncCursor = 0
	m.syncStatus.Upstash.HasKey = true
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "New box") {
		t.Fatalf("upstash page should offer New box:\n%s", body)
	}
	if !strings.Contains(body, "API key") {
		t.Fatalf("upstash page should offer API key:\n%s", body)
	}
	m.updateSyncKeys("l")
	if m.syncCursor != 1 {
		t.Fatalf("l should move to New box, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("enter")
	if m.form == nil || m.form.action != "upstash_new" {
		t.Fatalf("enter on New box should open form, got %#v", m.form)
	}
}

func TestUpstashDeleteUsesRemoteConfirm(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "upstash_dev", Synced: true, SyncSource: "upstash", SyncID: "current-wasp-05510",
		Resolved: sshconfig.Resolved{HostName: "us-east-1.box.upstash.com", User: "current-wasp-05510"},
	}}
	if err := m.metadata.SetHost("upstash_dev", metadata.Host{Label: "dev", Group: "Upstash", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	m.section = hostsSection
	selectHostAlias(t, m, "upstash_dev")
	_, _ = m.updateKeys(press("d"))
	if m.form == nil || m.form.action != "upstash_delete" {
		t.Fatalf("d on upstash host should confirm remote delete, got %#v", m.form)
	}
}

func TestProviderPageJMovesToConfig(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "gcp"
	m.syncCursor = 0
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "disabled") {
		t.Fatalf("expected status identity:\n%s", body)
	}
	if !strings.Contains(body, "Connect") {
		t.Fatalf("expected Connect chip:\n%s", body)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 1 {
		t.Fatalf("j from chips should move to config, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("k")
	if m.syncCursor != 0 {
		t.Fatalf("k should return to chips, cursor=%d", m.syncCursor)
	}
}

func TestVaultMenuMouse(t *testing.T) {
	m := testApp(t)
	m.section = vaultSection
	m.syncCursor = -1
	y := m.vaultMenuOriginY()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: y, Button: tea.MouseLeft}))
	if m.syncCursor != 0 {
		t.Fatalf("first click should select API base, cursor=%d", m.syncCursor)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: y, Button: tea.MouseLeft}))
	if m.form == nil || m.form.action != "vault_api_base" {
		t.Fatalf("second click should open API base, form=%#v", m.form)
	}
}

func TestVaultTermsFormMouse(t *testing.T) {
	m := testApp(t)
	m.openVaultTermsForm("you@example.com", "https://bast.sh")
	if m.form == nil || !m.form.selecting {
		t.Fatal("terms form should start selecting")
	}
	y := m.formOptionListOriginY()
	if y < 0 {
		t.Fatal("expected option list")
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 6, Y: y + 1, Button: tea.MouseLeft}))
	if m.form == nil || m.form.fields[m.form.index].selected != 1 {
		t.Fatalf("first click should select Cancel, form=%#v", m.form)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 6, Y: y + 1, Button: tea.MouseLeft}))
	if m.form != nil {
		t.Fatal("second click on Cancel should close terms")
	}
}

func TestProviderInventoryGroupsByStatus(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	m.syncCursor = -1
	m.hosts = []sshconfig.Host{
		{Alias: "box_run", Synced: true, SyncSource: "box", SyncID: "bx_run001", Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"}},
		{Alias: "box_stop", Synced: true, SyncSource: "box", SyncID: "bx_stop01", Resolved: sshconfig.Resolved{HostName: "box.stopped.invalid", User: "user"}},
	}
	if err := m.metadata.SetHost("box_run", metadata.Host{Label: "alpha-box", Group: "Box", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("box_stop", metadata.Host{Label: "idle-box", Group: "Box", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "Running") || !strings.Contains(body, "Stopped") {
		t.Fatalf("expected status groups:\n%s", body)
	}
	if !strings.Contains(body, "alpha-box") {
		t.Fatalf("running host should be visible:\n%s", body)
	}
	if strings.Contains(body, "idle-box") {
		t.Fatalf("stopped group should start collapsed:\n%s", body)
	}

	m.updateSyncKeys("j")
	if m.syncCursor != 1 {
		t.Fatalf("j should focus Running header, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 2 {
		t.Fatalf("j should focus running host, cursor=%d", m.syncCursor)
	}
	if got := m.browseFooterHint(80); !strings.Contains(got, "enter connect") || !strings.Contains(got, "o stop") || !strings.Contains(got, "n fork") {
		t.Fatalf("host footer = %q", got)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 3 {
		t.Fatalf("j should focus Stopped header, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("space")
	body = m.renderSync(m.styles())
	if !strings.Contains(body, "idle-box") {
		t.Fatalf("space should expand stopped:\n%s", body)
	}

	m.updateSyncKeys("j")
	_, cmd := m.updateSyncKeys("enter")
	if m.boxConnectAfter != "box_stop" {
		t.Fatalf("enter on stopped box should resume+connect, after=%q", m.boxConnectAfter)
	}
	if cmd == nil {
		t.Fatal("expected resume command")
	}
}

func TestProviderInventorySandboxLifecycle(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	m.syncStatus.Box.Authenticated = true
	m.hosts = []sshconfig.Host{
		{Alias: "box_run", Synced: true, SyncSource: "box", SyncID: "bx_run001", Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"}},
		{Alias: "box_stop", Synced: true, SyncSource: "box", SyncID: "bx_stop01", Resolved: sshconfig.Resolved{HostName: "box.stopped.invalid", User: "user"}},
	}
	if err := m.metadata.SetHost("box_run", metadata.Host{Label: "alpha-box", Group: "Box", Tags: []string{"state:running", "snapshot"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("box_stop", metadata.Host{Label: "idle-box", Group: "Box", Tags: []string{"state:stopped", "snapshot"}}); err != nil {
		t.Fatal(err)
	}
	m.toggleProviderInv(invGroupStopped)

	selectProviderHost(t, m, "box_run")
	_, _ = m.updateKeys(press("o"))
	if m.form == nil || m.form.action != "box_stop" {
		t.Fatalf("o on running inventory host should stop, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "box_fork" {
		t.Fatalf("n on running inventory host should fork, got %#v", m.form)
	}

	m.form = nil
	m.syncCursor = 0
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "box_new" {
		t.Fatalf("n on chips should still create a box, got %#v", m.form)
	}

	m.form = nil
	selectProviderHost(t, m, "box_stop")
	footer := m.browseFooterHint(80)
	if !strings.Contains(footer, "r resume") || strings.Contains(footer, "o stop") {
		t.Fatalf("stopped inventory footer = %q", footer)
	}
	_, cmd := m.updateKeys(press("r"))
	if cmd == nil {
		t.Fatal("r on stopped inventory host should resume")
	}
	if m.boxConnectAfter != "" {
		t.Fatalf("r should resume without connecting, after=%q", m.boxConnectAfter)
	}
	if m.syncActivity != "resuming…" {
		t.Fatalf("resume activity = %q", m.syncActivity)
	}

	m.syncingProviders = map[string]bool{}
	m.syncActivity = ""
	m.syncProvider = "upstash"
	m.syncStatus.Upstash.HasKey = true
	m.hosts = []sshconfig.Host{{
		Alias: "upstash_dev", Synced: true, SyncSource: "upstash", SyncID: "current-wasp-05510",
		Resolved: sshconfig.Resolved{HostName: "us-east-1.box.upstash.com", User: "root"},
	}}
	if err := m.metadata.SetHost("upstash_dev", metadata.Host{Label: "dev", Group: "Upstash", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	selectProviderHost(t, m, "upstash_dev")
	_, _ = m.updateKeys(press("o"))
	if m.form == nil || m.form.action != "upstash_stop" {
		t.Fatalf("o on upstash inventory host should pause, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "upstash_fork" {
		t.Fatalf("n on upstash inventory host should fork, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("d"))
	if m.form == nil || m.form.action != "upstash_delete" {
		t.Fatalf("d on upstash inventory host should delete, got %#v", m.form)
	}

	m.form = nil
	m.syncCursor = 0
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "upstash_new" {
		t.Fatalf("n on upstash chips should create a box, got %#v", m.form)
	}
}

func TestProviderInstancesGroup(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "gcp"
	m.syncCursor = -1
	m.hosts = []sshconfig.Host{
		{Alias: "gcp_web", Synced: true, SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/web"},
		{Alias: "gcp_api", Synced: true, SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/api"},
	}
	if err := m.metadata.SetHost("gcp_web", metadata.Host{Label: "web", Group: "Google Cloud/prod"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("gcp_api", metadata.Host{Label: "api", Group: "Google Cloud/prod"}); err != nil {
		t.Fatal(err)
	}
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "Instances") || !strings.Contains(body, "web") || !strings.Contains(body, "api") {
		t.Fatalf("gcp inventory:\n%s", body)
	}
	if strings.Contains(body, "Running") || strings.Contains(body, "Stopped") {
		t.Fatalf("gcp should not split running/stopped:\n%s", body)
	}
}

func TestProviderNavChipThenInventory(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	m.syncCursor = -1
	m.syncStatus.Box.Authenticated = true
	m.hosts = []sshconfig.Host{
		{Alias: "box_run", Synced: true, SyncSource: "box", SyncID: "bx_run001", Resolved: sshconfig.Resolved{HostName: "203.0.113.10"}},
	}
	if err := m.metadata.SetHost("box_run", metadata.Host{Label: "alpha-box", Group: "Box", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	m.updateSyncKeys("l")
	if m.syncCursor != 1 {
		t.Fatalf("l from Sync/Connect should hit New box, cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 2 {
		t.Fatalf("j from chips should hit inventory, cursor=%d", m.syncCursor)
	}
}

func TestProviderPageMouse(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "gcp"
	m.syncCursor = -1
	var configHit providerPageHit
	found := false
	for _, h := range m.providerPageHits() {
		if h.kind == "config" && h.index == 0 {
			configHit = h
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected config hit")
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: configHit.y0, Button: tea.MouseLeft}))
	if m.syncCursor < 0 {
		t.Fatalf("first click should select config, cursor=%d", m.syncCursor)
	}
	_, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: configHit.y0, Button: tea.MouseLeft}))
	if m.form == nil && cmd == nil {
		t.Fatal("second click should run config action")
	}
}

func TestBoxLifecycleChipMouse(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	m.syncCursor = -1
	m.syncStatus.Box.Authenticated = true
	life, _ := m.providerActionLayout()
	newIdx := -1
	for i, item := range life {
		if item.action == "box_new" {
			newIdx = i
			break
		}
	}
	if newIdx < 0 {
		t.Fatal("expected New box action")
	}
	var chip providerPageHit
	found := false
	for _, h := range m.providerPageHits() {
		if h.kind == "chip" && h.index == newIdx {
			chip = h
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected New box chip")
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: chip.x0 + 1, Y: chip.y0, Button: tea.MouseLeft}))
	if m.syncCursor != newIdx {
		t.Fatalf("first click should select New box, cursor=%d", m.syncCursor)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: chip.x0 + 1, Y: chip.y0, Button: tea.MouseLeft}))
	if m.form == nil || m.form.action != "box_new" {
		t.Fatalf("second click should open New box, form=%#v", m.form)
	}
}

func TestProviderInventoryMouseToggle(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "box"
	m.syncCursor = -1
	m.hosts = []sshconfig.Host{
		{Alias: "box_stop", Synced: true, SyncSource: "box", SyncID: "bx_stop01", Resolved: sshconfig.Resolved{HostName: "box.stopped.invalid", User: "user"}},
	}
	if err := m.metadata.SetHost("box_stop", metadata.Host{Label: "idle-box", Group: "Box", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.renderSync(m.styles()), "idle-box") {
		t.Fatal("stopped host should start hidden")
	}
	var header providerPageHit
	found := false
	for _, h := range m.providerPageHits() {
		if h.kind == "inv" && h.index == 0 {
			header = h
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected stopped header hit")
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: header.y0, Button: tea.MouseLeft}))
	if !strings.Contains(m.renderSync(m.styles()), "idle-box") {
		t.Fatal("clicking Stopped should expand the group")
	}
}

func TestProviderInventoryCollapseIsPerProvider(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "gcp"
	m.hosts = []sshconfig.Host{
		{Alias: "gcp_web", Synced: true, SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/web"},
		{Alias: "aws_web", Synced: true, SyncSource: "aws", SyncID: "arn:aws:ec2:eu-west-1:1:instance/i-1"},
	}
	if err := m.metadata.SetHost("gcp_web", metadata.Host{Label: "web", Group: "Google Cloud"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("aws_web", metadata.Host{Label: "api", Group: "Amazon EC2"}); err != nil {
		t.Fatal(err)
	}
	m.toggleProviderInv(invGroupInstances)
	if !m.providerInvCollapsed(invGroupInstances) {
		t.Fatal("gcp instances should collapse")
	}
	m.syncProvider = "aws"
	if m.providerInvCollapsed(invGroupInstances) {
		t.Fatal("aws instances should not inherit gcp collapse")
	}
}

func TestVaultTabRenders(t *testing.T) {
	m := testApp(t)
	m.section = vaultSection
	m.syncCursor = -1
	body := m.renderVault(m.styles())
	if !strings.Contains(body, "Vault") || !strings.Contains(body, "not linked") {
		t.Fatalf("vault body:\n%s", body)
	}
	if !strings.Contains(body, "Link") {
		t.Fatalf("vault should show Link:\n%s", body)
	}
	if strings.Contains(body, "Link account") {
		t.Fatalf("primary should not be duplicated in the list:\n%s", body)
	}
	if strings.Contains(body, "GCP") {
		t.Fatalf("vault tab should not list cloud providers:\n%s", body)
	}
	if strings.Contains(body, "Does not sync external") {
		t.Fatalf("vault should not narrate the interface:\n%s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "Vault") && strings.Contains(line, "Link") {
			t.Fatalf("Link should not sit on the Vault title row:\n%s", body)
		}
	}
}

func TestVaultLinkChipIsClickable(t *testing.T) {
	m := testApp(t)
	m.section = vaultSection
	x, y, _ := m.vaultActionButtonBounds()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	if m.form == nil || m.form.action != "vault_login" {
		t.Fatalf("link chip should open login, form=%#v", m.form)
	}
}

func TestVaultLinkChipUnhighlightsWhenMenuFocused(t *testing.T) {
	m := testApp(t)
	m.section = vaultSection
	chips := []syncMenuItem{m.vaultPrimaryChip()}
	highlighted := m.renderActionChips(m.styles(), chips, 0)
	rest := m.renderActionChips(m.styles(), chips, -1)
	if highlighted == rest {
		t.Fatal("chip highlight style should differ from rest")
	}

	m.syncCursor = -1
	onChip := m.renderVault(m.styles())
	if !strings.Contains(onChip, highlighted) || strings.Contains(onChip, rest) {
		t.Fatalf("link chip should be highlighted when focused:\n%s", onChip)
	}

	m.syncCursor = 0
	onMenu := m.renderVault(m.styles())
	if strings.Contains(onMenu, highlighted) {
		t.Fatalf("link chip should not stay highlighted when API base is selected:\n%s", onMenu)
	}
	if !strings.Contains(onMenu, rest) {
		t.Fatalf("link chip should use rest style when API base is selected:\n%s", onMenu)
	}
}

func TestUnlinkedVaultHasFirstRunSeal(t *testing.T) {
	m := testApp(t)
	m.section = vaultSection
	m.syncCursor = -1
	body := m.renderVault(m.styles())
	for _, want := range []string{
		"No vault yet",
		"Sync hosts and keys between your computers.",
		"The passphrase never leaves this machine.",
		"not linked",
		"API base URL",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Press") || strings.Contains(body, "Click") {
		t.Fatalf("unlinked vault should not narrate the action:\n%s", body)
	}

	for _, width := range []int{40, 60, 100} {
		m.width = width
		view := m.renderVault(m.styles())
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("line wider than %d (%d): %q", width, lipgloss.Width(line), line)
			}
		}
	}
	m.width = 40
	narrow := m.renderVault(m.styles())
	if !strings.Contains(narrow, "No vault yet") || !strings.Contains(narrow, "not linked") {
		t.Fatalf("narrow unlinked vault:\n%s", narrow)
	}
	if strings.Contains(narrow, "Sync hosts and keys between your computers.") {
		t.Fatalf("narrow identity should wrap, not overflow:\n%s", narrow)
	}
}

func TestWrapWords(t *testing.T) {
	got := wrapWords("Sync hosts and keys between your computers.", 24)
	if len(got) < 2 || strings.Join(got, " ") != "Sync hosts and keys between your computers." {
		t.Fatalf("wrap=%q", got)
	}
	if wrapWords("", 20)[0] != "" {
		t.Fatal("empty should yield a blank line")
	}
	if got := wrapWords("supercalifragilisticexpialidocious", 8); len(got) != 1 || !strings.HasSuffix(got[0], "…") {
		t.Fatalf("long token=%q", got)
	}
}

func TestLinkedVaultOmitsFirstRunSeal(t *testing.T) {
	m := testApp(t)
	m.section = vaultSection
	m.vaultSessionChecked = true
	m.vaultSession = &vault.Session{Email: "you@example.com", Token: "t", Revision: "abcdefghij"}
	m.vaultPassphrase = "secret"
	body := m.renderVault(m.styles())
	if strings.Contains(body, "No vault yet") || strings.Contains(body, "between your computers") {
		t.Fatalf("linked vault should not use the first-run seal:\n%s", body)
	}
	if !strings.Contains(body, "you@example.com") || !strings.Contains(body, "unlocked") {
		t.Fatalf("linked vault:\n%s", body)
	}
}

func TestVaultHostedTermsForm(t *testing.T) {
	m := testApp(t)
	m.form = &form{
		action: "vault_login",
		fields: []field{
			{label: "Email", value: "you@example.com"},
			{label: "API base", value: "https://bast.sh"},
		},
	}
	if cmd := m.submitVaultLogin(); cmd != nil {
		t.Fatal("hosted login should open terms before sending OTP")
	}
	if m.form == nil || m.form.action != "vault_terms" {
		t.Fatalf("expected vault_terms form, got %#v", m.form)
	}
	if !strings.Contains(m.form.fields[2].description, "legal/terms") {
		t.Fatalf("terms form should cite URLs: %+v", m.form.fields)
	}
	m.form.fields[2].value = "cancel"
	if cmd := m.submitVaultTerms(); cmd == nil {
		t.Fatal("cancel should notice")
	}
	if m.form != nil {
		t.Fatal("cancel should close the form")
	}

	m.form = &form{
		action: "vault_login",
		fields: []field{
			{label: "Email", value: "you@example.com"},
			{label: "API base", value: "https://vault.example"},
		},
	}
	if cmd := m.submitVaultLogin(); cmd == nil {
		t.Fatal("self-host login should send OTP without terms")
	}
	if m.form != nil && m.form.action == "vault_terms" {
		t.Fatal("self-host should skip terms")
	}
}

func TestSyncActionIgnoredWhileSyncing(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "gcp"
	m.syncCursor = 0
	m.syncingProviders = map[string]bool{"gcp": true}

	_, cmd := m.updateSyncKeys("enter")
	if cmd != nil {
		t.Fatal("sync action should be disabled while a sync is running")
	}
}

func TestSyncActionIgnoredWhileOtherProviderSyncing(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "aws"
	m.syncCursor = 0
	m.syncingProviders = map[string]bool{"gcp": true}

	_, cmd := m.updateSyncKeys("enter")
	if cmd != nil {
		t.Fatal("AWS sync action should be disabled while GCP sync is running")
	}
	if m.syncingProviders["aws"] {
		t.Fatal("AWS sync was marked active while GCP sync is running")
	}
}

func TestInitDefersProviderAutoSyncUntilHostsDiscovered(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetAWS(metadata.AWSIntegration{Enabled: true, AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetAzure(metadata.AzureIntegration{Enabled: true, AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetBox(metadata.BoxIntegration{Disabled: true}); err != nil {
		t.Fatal(err)
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil")
	}
	if m.syncingProviders["gcp"] || m.syncingProviders["aws"] || m.syncingProviders["azure"] {
		t.Fatalf("Init marked providers syncing before hosts painted: %v", m.syncingProviders)
	}

	next := m.autoSyncCmds()
	if next == nil {
		t.Fatal("autoSyncCmds returned nil")
	}
	autoSync := next()
	if got := reflect.TypeOf(autoSync).String(); got != "tea.sequenceMsg" {
		t.Fatalf("auto-sync command = %s, want tea.sequenceMsg", got)
	}
	if got := reflect.ValueOf(autoSync).Len(); got != 3 {
		t.Fatalf("auto-sync sequence length = %d, want 3", got)
	}
	if !m.syncingProviders["gcp"] || !m.syncingProviders["aws"] || !m.syncingProviders["azure"] {
		t.Fatalf("syncing providers = %v", m.syncingProviders)
	}
}

func TestAutoSyncDoesNotRestartAfterSyncReload(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, AutoSync: true}); err != nil {
		t.Fatal(err)
	}

	if cmd := m.autoSyncCmds(); cmd == nil {
		t.Fatal("initial auto-sync command is nil")
	}
	if !m.syncingProviders["gcp"] {
		t.Fatal("GCP was not marked syncing")
	}

	_, cmd := m.Update(syncDoneMsg{provider: "gcp"})
	if cmd == nil {
		t.Fatal("sync completion did not schedule reload")
	}
	if m.syncingProviders["gcp"] {
		t.Fatal("GCP remained marked syncing after completion")
	}

	_, cmd = m.Update(loadedMsg{})
	if cmd != nil {
		t.Fatal("host reload restarted auto-sync")
	}
	if m.syncingProviders["gcp"] {
		t.Fatal("GCP was marked syncing again after reload")
	}
}

func TestBoxAutoConnectSkipsExplicitAutoSyncOff(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetBox(metadata.BoxIntegration{Enabled: true, AutoSync: false}); err != nil {
		t.Fatal(err)
	}
	if cmd := m.autoSyncCmds(); cmd != nil {
		t.Fatal("enabled Box with auto-sync off should not auto-connect or sync")
	}
	if m.syncingProviders["box"] {
		t.Fatal("Box should not be marked syncing")
	}
}

func TestBoxAutoConnectRunsWhenNotEnabled(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetBox(metadata.BoxIntegration{}); err != nil {
		t.Fatal(err)
	}
	if cmd := m.autoSyncCmds(); cmd == nil {
		t.Fatal("unconfigured Box should schedule auto-connect")
	}
	if !m.syncingProviders["box"] {
		t.Fatal("Box should be marked syncing for auto-connect")
	}
}

func TestStaleBoxSyncDoneDoesNotClearNewerOp(t *testing.T) {
	m := testApp(t)
	old := m.beginProviderOp("box")
	m.syncActivity = "stopping…"
	newer := m.beginProviderOp("box")
	if newer == old {
		t.Fatal("expected a new op generation")
	}
	m.Update(syncDoneMsg{provider: "box", opGen: old})
	if !m.syncingProviders["box"] {
		t.Fatal("stale completion cleared an in-flight box op")
	}
	if m.syncActivity != "stopping…" {
		t.Fatalf("stale completion cleared activity: %q", m.syncActivity)
	}
	m.Update(syncDoneMsg{provider: "box", opGen: newer})
	if m.syncingProviders["box"] {
		t.Fatal("current completion should clear the box op")
	}
}

func TestSyncCompletionNoticeIncludesEveryEnabledProvider(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, LastInstanceCount: 0}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetAWS(metadata.AWSIntegration{Enabled: true, LastInstanceCount: 12}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetAzure(metadata.AzureIntegration{Enabled: true, LastInstanceCount: 4}); err != nil {
		t.Fatal(err)
	}

	if got := m.syncCompletionNotice("gcp", 3); got != "GCP 3 · AWS 12 · Azure 4" {
		t.Fatalf("multi-provider notice = %q", got)
	}

	if err := m.metadata.SetAWS(metadata.AWSIntegration{}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetAzure(metadata.AzureIntegration{}); err != nil {
		t.Fatal(err)
	}
	if got := m.syncCompletionNotice("gcp", 3); got != "Synced 3 GCP instances" {
		t.Fatalf("single-provider notice = %q", got)
	}
}

func TestPrepareTimeoutForHost(t *testing.T) {
	if got := prepareTimeoutForHost(sshconfig.Host{Synced: true, SyncSource: "box"}); got != boxPrepareTimeout {
		t.Fatalf("box timeout = %v, want %v", got, boxPrepareTimeout)
	}
	if got := prepareTimeoutForHost(sshconfig.Host{Synced: true, SyncSource: "gcp"}); got != cloudPrepareTimeout {
		t.Fatalf("gcp timeout = %v, want %v", got, cloudPrepareTimeout)
	}
	if got := prepareTimeoutForHost(sshconfig.Host{}); got != cloudPrepareTimeout {
		t.Fatalf("local timeout = %v, want %v", got, cloudPrepareTimeout)
	}
	if boxPrepareTimeout <= 3*time.Minute {
		t.Fatalf("box prepare timeout %v must exceed Resume WaitReady (3m)", boxPrepareTimeout)
	}
}

func TestFavoriteAndHiddenAllowedForNonBoxSyncedHosts(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "gcp_demo_web", Synced: true, SyncSource: "gcp",
		SyncID:   "projects/demo/zones/us-central1-a/instances/web",
		Resolved: sshconfig.Resolved{HostName: "web", User: "ubuntu"},
	}, {
		Alias: "box_sunny", Synced: true, SyncSource: "box", SyncID: "bx_sunny01",
		Resolved: sshconfig.Resolved{HostName: "1.2.3.4", User: "user"},
	}}
	_ = m.metadata.SetHost("gcp_demo_web", metadata.Host{Label: "web"})
	_ = m.metadata.SetHost("box_sunny", metadata.Host{Label: "sunny"})

	selectHostAlias(t, m, "gcp_demo_web")
	m.updateKeys(press("f"))
	if !m.metadata.Host("gcp_demo_web").Favorite {
		t.Fatal("GCP synced host should allow favorite toggle")
	}
	m.updateKeys(press("h"))
	if !m.metadata.Host("gcp_demo_web").Hidden {
		t.Fatal("GCP synced host should allow hidden toggle")
	}

	selectHostAlias(t, m, "box_sunny")
	m.status = ""
	m.updateKeys(press("f"))
	if m.metadata.Host("box_sunny").Favorite {
		t.Fatal("Box host should not toggle favorite")
	}
	if !strings.Contains(m.status, "Synced sandbox hosts are read-only") {
		t.Fatalf("box favorite status = %q", m.status)
	}
	m.status = ""
	m.updateKeys(press("h"))
	if m.metadata.Host("box_sunny").Hidden {
		t.Fatal("Box host should not toggle hidden")
	}
	if !strings.Contains(m.status, "Synced sandbox hosts are read-only") {
		t.Fatalf("box hidden status = %q", m.status)
	}
}

func TestTabKeysOpenVaultSyncFiles(t *testing.T) {
	m := testApp(t)
	m.Update(press("3"))
	if m.section != vaultSection {
		t.Fatalf("3 should open Vault, got %v", m.section)
	}
	m.Update(press("4"))
	if m.section != syncSection {
		t.Fatalf("4 should open Sync, got %v", m.section)
	}
	m.Update(press("5"))
	if m.section != filesSection {
		t.Fatalf("5 should open Files, got %v", m.section)
	}
	m.Update(press("1"))
	if m.section != hostsSection {
		t.Fatalf("1 should open Hosts, got %v", m.section)
	}
}

func TestProviderGroupShowsCreate(t *testing.T) {
	m := testApp(t)
	m.hosts = nil
	if err := m.metadata.SetBox(metadata.BoxIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}
	rows := m.hostRows()
	if len(rows) == 0 || !rows[0].header || rows[0].group != "Box" {
		t.Fatalf("expected injected Box group, rows=%+v", rows)
	}
	m.cursor = 0
	detail := m.renderGroupDetail(m.styles(), rows[0], 60)
	if !strings.Contains(detail, "New box") {
		t.Fatalf("Box group should offer New box:\n%s", detail)
	}
	lines := strings.Split(detail, "\n")
	if len(lines) < 3 || !strings.Contains(lines[0], "Box") || strings.Contains(lines[0], "New box") {
		t.Fatalf("New box should sit under the Box title:\n%s", detail)
	}
	if !strings.Contains(lines[2], "New box") {
		t.Fatalf("New box chip should be on the action row:\n%s", detail)
	}
	_, cmd := m.updateKeys(press("n"))
	_ = cmd
	if m.form == nil || m.form.action != "box_new" {
		t.Fatalf("n on Box group should open new form, got %#v", m.form)
	}
	if len(m.form.fields) < 3 || len(m.form.fields[0].options) != 3 || m.form.fields[0].selected != 1 {
		t.Fatalf("new box form should offer constrained type options, got %#v", m.form.fields)
	}
}
