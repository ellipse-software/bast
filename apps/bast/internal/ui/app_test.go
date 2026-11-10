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

	"bast/internal/cloud"
	cloudsync "bast/internal/cloud/sync"
	"bast/internal/connectbanner"
	"bast/internal/doctor"
	keymodel "bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/rdp"
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
	return &App{paths: p, openSSH: openssh.Default(), metadata: store, syncer: cloudsync.New(p, store), hosts: []sshconfig.Host{{Alias: "alpha"}, {Alias: "beta"}}, width: 100, height: 30, dark: true}
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

func ctrlS() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl})
}

func superS() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModSuper})
}

func ctrlO() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl})
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

func focusHostFormField(t *testing.T, m *App, label string) {
	t.Helper()
	for i, item := range hostHubItems(m.form) {
		if item.title == label {
			m.form.screen = "hub"
			m.form.hubIndex = i
			m.form.selecting = false
			m.focusHostHubItem()
			return
		}
	}
	t.Fatalf("field %q not on the host form", label)
}

func enterHostFormSection(t *testing.T, m *App, section string) {
	t.Helper()
	if section == formSectionAuth {
		focusHostFormField(t, m, methodFieldLabel)
		return
	}
	if section == formSectionMetadata {
		section = formSectionCustomisation
	}
	if section == formSectionAdvanced {
		m.enterHostSection(section)
		return
	}
	for _, item := range hostHubItems(m.form) {
		if item.section == section && item.id != "advanced" {
			focusHostFormField(t, m, item.title)
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

func activateSyncRow(t *testing.T, m *App, kind cloud.Kind, id string) {
	t.Helper()
	m.section = syncSection
	m.syncProvider = string(kind)
	rows := m.syncSettingRows(kind)
	for i, row := range rows {
		if row.id == id {
			m.syncCursor = i
			m.updateSyncKeys("enter")
			return
		}
	}
	t.Fatalf("no sync row %s for %s: %+v", id, kind, rows)
}

func syncDetailClick(t *testing.T, m *App, kind cloud.Kind, id string) (x, y int) {
	t.Helper()
	layout := m.panelLayout()
	lines, rowLine := m.syncDetailLines(kind, layout.detailWidth)
	rows := m.syncSettingRows(kind)
	idx := -1
	for i, row := range rows {
		if row.id == id {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(rowLine) {
		t.Fatalf("no sync row %s for %s: %+v", id, kind, rows)
	}
	focus := 0
	if m.syncProvider != "" && m.syncCursor >= 0 && m.syncCursor < len(rowLine) {
		focus = rowLine[m.syncCursor]
	}
	start := scrollStart(focus, len(lines), layout.detailHeight)
	y = layout.detailTop + (rowLine[idx] - start)
	x = 4
	if !layout.mobile {
		x = layout.listWidth + 4
	}
	return x, y
}

func TestNumberedNavigationAndSearch(t *testing.T) {
	m := testApp(t)
	m.section = hostsSection
	m.Update(press("2"))
	if m.section != keysSection {
		t.Fatal("2 did not open keys")
	}
	m.Update(press("1"))
	m.Update(press("ctrl+f"))
	m.Update(press("b"))
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := m.filteredHosts(); len(got) != 1 || got[0].Alias != "beta" {
		t.Fatalf("filter = %+v", got)
	}
}

func TestCtrlCQuitsFromEveryBastContext(t *testing.T) {
	contexts := map[string]func(*App){
		"root":       func(*App) {},
		"sheet":      func(m *App) { m.palette = true },
		"about":      func(m *App) { m.credits = true },
		"onboarding": func(m *App) { m.onboarding = true },
		"doctor":     func(m *App) { m.doctor = true },
		"search":     func(m *App) { m.search = "\x00query" },
		"error":      func(m *App) { m.status, m.statusError = "failed", true },
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
	for _, context := range []string{"root", "sheet", "about", "doctor"} {
		t.Run(context, func(t *testing.T) {
			m := testApp(t)
			m.palette = context == "sheet"
			m.credits = context == "about"
			m.doctor = context == "doctor"
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

func TestEscapeReturnsToParentAndDoesNotQuitAtRoot(t *testing.T) {
	for _, section := range []section{hostsSection, keysSection, syncSection} {
		t.Run(fmt.Sprint(section), func(t *testing.T) {
			m := testApp(t)
			m.section = section
			_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
			if cmd != nil {
				if _, ok := cmd().(tea.QuitMsg); ok {
					t.Fatal("esc quit from the root list")
				}
			}
		})
	}

	m := testApp(t)
	m.section, m.syncProvider = syncSection, "gcp"
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if cmd != nil || m.syncProvider != "" {
		t.Fatalf("Esc should return from provider: provider=%q cmd=%v", m.syncProvider, cmd)
	}

	m = testApp(t)
	m.palette = true
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.palette {
		t.Fatal("Esc did not close the action sheet")
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

func TestBackspaceDoesNotQuitAtRootAndReturnsFromSyncProvider(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}),
		tea.KeyPressMsg(tea.Key{Code: 'h', Mod: tea.ModCtrl}),
	} {
		for _, section := range []section{hostsSection, keysSection, syncSection} {
			m := testApp(t)
			m.section = section
			_, cmd := m.Update(key)
			if cmd != nil {
				if _, ok := cmd().(tea.QuitMsg); ok {
					t.Fatal("backspace quit from the root list")
				}
			}
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

func TestVercelAndHetznerSyncErrorsShowIcon(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{Alias: "sandbox"}, {Alias: "web"}}
	if err := m.metadata.SetHost("sandbox", metadata.Host{Group: "Vercel"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("web", metadata.Host{Group: "Hetzner Cloud/default/fsn1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetVercel(metadata.VercelIntegration{Enabled: true, LastSyncError: "team required"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	if rendered := m.renderHosts(m.styles()); strings.Count(rendered, "⚠") != 1 {
		t.Fatalf("expected Vercel error icon:\n%s", rendered)
	}

	vercel := m.metadata.Vercel()
	vercel.LastSyncError = ""
	if err := m.metadata.SetVercel(vercel); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHetzner(metadata.HetznerIntegration{Enabled: true, LastSyncError: "401"}); err != nil {
		t.Fatal(err)
	}
	if rendered := m.renderHosts(m.styles()); strings.Count(rendered, "⚠") != 1 {
		t.Fatalf("expected Hetzner error icon:\n%s", rendered)
	}
}

func TestProviderConnectClearsStaleSyncErrorImmediately(t *testing.T) {
	m := testApp(t)
	m.syncProvider = "vercel"
	m.syncStatus.Vercel.Error = "team required"
	if err := m.metadata.SetVercel(metadata.VercelIntegration{Enabled: true, TeamID: "team_1", ProjectID: "prj_1", LastSyncError: "old list error"}); err != nil {
		t.Fatal(err)
	}
	m.beginProviderOp("vercel")
	if m.syncStatus.Vercel.Error != "" {
		t.Fatalf("probe error survived begin: %q", m.syncStatus.Vercel.Error)
	}
	identity := m.renderProviderIdentity(m.styles(), "vercel")
	if strings.Contains(identity, "team required") || strings.Contains(identity, "old list error") {
		t.Fatalf("stale error still visible while connecting:\n%s", identity)
	}
	if err := m.metadata.SetVercel(metadata.VercelIntegration{Enabled: true, TeamID: "team_1", ProjectID: "prj_1"}); err != nil {
		t.Fatal(err)
	}
	m.Update(syncDoneMsg{provider: "vercel", opGen: m.providerOpGen("vercel")})
	if m.syncStatus.Vercel.Error != "" {
		t.Fatalf("probe error survived success: %q", m.syncStatus.Vercel.Error)
	}
}

func TestCloudSyncRootShowsCurrentCLIError(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Amazon EC2/default/eu-west-2"}); err != nil {
		t.Fatal(err)
	}
	m.syncStatus.AWS.AWSCLIError = "aws executable not found"
	m.sortHosts()

	if rendered := m.renderHosts(m.styles()); strings.Contains(rendered, "⚠") {
		t.Fatalf("a missing CLI should not mark the group as failed:\n%s", rendered)
	}

	m.syncStatus.AWS.AWSCLIError = "access denied"
	if rendered := m.renderHosts(m.styles()); strings.Count(rendered, "⚠") != 1 {
		t.Fatalf("expected one error icon for a real CLI failure:\n%s", rendered)
	}
}

func TestFooterHidesConnectWhenGroupSelected(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = 0

	footer := sheet(m)
	if !strings.Contains(footer, "␣") || !strings.Contains(footer, "collapse") && !strings.Contains(footer, "expand") {
		t.Fatalf("group sheet should show collapse: %q", footer)
	}
	if strings.Contains(footer, "connect") {
		t.Fatalf("group sheet should not advertise connect: %q", footer)
	}

	m.cursor = 1
	footer = sheet(m)
	if !strings.Contains(footer, "connect") || !strings.Contains(footer, "edit") || !strings.Contains(footer, "files") {
		t.Fatalf("host sheet = %q", footer)
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

func TestGroupAndHostAppearance(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{
		{Alias: "alpha", Managed: true},
		{
			Alias: "gcp_demo_web", Synced: true, SyncSource: "gcp",
			Resolved: sshconfig.Resolved{HostName: "web", User: "ubuntu"},
		},
	}
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work", Icon: "🇩🇪", Color: "red"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("gcp_demo_web", metadata.Host{Label: "web", Group: "Google Cloud/Demo"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetGroups(map[string]metadata.GroupAppearance{
		"Work":         {Icon: "🇩🇪", Color: "red"},
		"Google Cloud": {Icon: "🇩🇪", Color: "red"},
	}); err != nil {
		t.Fatal(err)
	}
	m.refreshHostMetadata()
	m.sortHosts()
	m.cursor = len(m.hostListRows()) - 1

	body := m.renderHosts(m.styles())
	groupName := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Render("Work")
	if !strings.Contains(body, groupName) {
		t.Fatalf("group name missing ANSI red:\n%s", body)
	}
	if strings.Contains(groupName, "38;2;") || strings.Contains(groupName, "38;5;") {
		t.Fatalf("theme colour rendered as truecolor or indexed: %q", groupName)
	}
	if !strings.Contains(groupName, "31") {
		t.Fatalf("theme colour missing SGR 31: %q", groupName)
	}
	hostName := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("alpha")
	if !strings.Contains(body, hostName) || !strings.Contains(body, "🇩🇪") {
		t.Fatalf("host row missing icon or ANSI colour:\n%s", body)
	}
	blue := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4285F4"))
	if !strings.Contains(body, blue.Render("▾ ")) {
		t.Fatalf("provider row should keep its brand colour:\n%s", body)
	}
	if strings.Contains(body, "🇩🇪 ▾") || strings.Count(body, "🇩🇪") < 2 {
		t.Fatalf("provider row should not gain a user icon, and both user rows should:\n%s", body)
	}
	for i, row := range m.hostRows() {
		if row.header && row.group == "Work" {
			m.cursor = i
			break
		}
	}
	selected := m.renderHosts(m.styles())
	selectedName := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Background(lipgloss.Color("#1F2937")).Render("Work")
	selectedIcon := lipgloss.NewStyle().Background(lipgloss.Color("#1F2937")).Render("🇩🇪 ")
	if !strings.Contains(selected, selectedName) || !strings.Contains(selected, selectedIcon) || !strings.Contains(selected, m.styles().selected.Render("▾ ")) {
		t.Fatalf("selected group should keep the ANSI name, an uncoloured icon, and a theme chevron:\n%s", selected)
	}

	if err := m.metadata.SetGroups(map[string]metadata.GroupAppearance{
		"Work": {Color: "#abc"},
	}); err != nil {
		t.Fatal(err)
	}
	m.refreshHostMetadata()
	body = m.renderHosts(m.styles())
	hexName := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AABBCC")).Render("Work")
	if !strings.Contains(body, hexName) || !strings.Contains(hexName, "38;2;") {
		t.Fatalf("hex group name missing truecolor:\n%s\nname=%q", body, hexName)
	}

	for i, row := range m.hostRows() {
		if row.header && row.group == "Work" {
			m.cursor = i
			break
		}
	}
	detail := m.renderGroupDetail(m.styles(), m.hostListRows()[m.cursor], 60)
	if !strings.Contains(detail, hexName) || !strings.Contains(detail, "e edit") {
		t.Fatalf("group detail = %q", detail)
	}
	hostDetail := m.renderHostDetail(m.styles(), m.hosts[0], 60)
	hostTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Render("alpha")
	if !strings.Contains(hostDetail, "🇩🇪") || !strings.Contains(hostDetail, hostTitle) || strings.Contains(hostTitle, "48;") {
		t.Fatalf("host detail should prefix the icon and colour the name:\n%s", hostDetail)
	}

	m.openEditGroupForm()
	if m.form == nil || m.form.title != "Edit group: Work" || m.form.revealed != len(m.form.fields)-1 {
		t.Fatalf("group form = %+v", m.form)
	}
	color := formFieldByLabel(m, "Color")
	labels := make([]string, 0, len(color.options))
	for _, option := range color.options {
		labels = append(labels, option.label)
	}
	if strings.Join(labels, ",") != "Default,Red,Green,Yellow,Blue,Magenta,Cyan,White,Gray,Custom hex" {
		t.Fatalf("palette = %v", labels)
	}
	if !color.options[color.selected].custom || color.value != "#AABBCC" {
		t.Fatalf("hex should open on custom: %+v", color)
	}

	for i, row := range m.hostRows() {
		if row.header && row.group == "Google Cloud" {
			m.cursor = i
			break
		}
	}
	m.form = nil
	m.status, m.statusError = "", false
	m.openEditGroupForm()
	if m.form != nil || !strings.Contains(m.status, "cannot be edited") {
		t.Fatalf("synced group status = %q form=%v", m.status, m.form != nil)
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

func TestHostGroupArrowsFollowTheTree(t *testing.T) {
	m := testApp(t)
	m.hosts = append(m.hosts, sshconfig.Host{Alias: "gamma"}, sshconfig.Host{Alias: "delta"}, sshconfig.Host{Alias: "loose"})
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

	index := func(match func(hostRow) bool) int {
		t.Helper()
		for i, row := range m.hostListRows() {
			if match(row) {
				return i
			}
		}
		t.Fatal("row not found")
		return -1
	}
	left := tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft})
	right := tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})
	up := tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	down := tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})

	alpha := index(func(row hostRow) bool { return row.host.Alias == "alpha" })
	m.cursor = alpha
	m.Update(up)
	if m.cursor != alpha-1 {
		t.Fatalf("up moved to %d, want %d", m.cursor, alpha-1)
	}
	m.Update(down)
	if m.cursor != alpha {
		t.Fatalf("down moved to %d, want %d", m.cursor, alpha)
	}

	m.Update(left)
	if m.cursor != index(func(row hostRow) bool { return row.header && row.group == "test/abc" }) {
		t.Fatalf("left on a host did not focus test/abc: cursor=%d rows=%+v", m.cursor, m.hostRows())
	}
	m.Update(left)
	if !m.collapsedGroups["test/abc"] || m.hostRows()[m.cursor].group != "test/abc" || !m.hostRows()[m.cursor].header {
		t.Fatalf("left on an expanded group did not collapse it: collapsed=%v cursor=%d", m.collapsedGroups, m.cursor)
	}
	m.Update(right)
	if m.collapsedGroups["test/abc"] {
		t.Fatal("right did not expand the collapsed subgroup")
	}
	expanded := m.cursor
	m.Update(right)
	if m.cursor != expanded || m.collapsedGroups["test/abc"] {
		t.Fatal("right on an expanded group changed the selection")
	}
	m.Update(left)
	m.Update(left)
	if m.cursor != index(func(row hostRow) bool { return row.header && row.group == "test" }) {
		t.Fatalf("left on a collapsed group did not focus its parent: cursor=%d", m.cursor)
	}

	m.cursor = index(func(row hostRow) bool { return row.header && row.group == "test" })
	m.Update(press("h"))
	if m.collapsedGroups["test"] {
		t.Fatal("h collapsed a group")
	}
	m.Update(left)
	if !m.collapsedGroups["test"] {
		t.Fatal("left did not collapse the expanded parent")
	}
	stayed := m.cursor
	m.Update(left)
	if m.cursor != stayed || !m.collapsedGroups["test"] {
		t.Fatalf("left on a top-level collapsed group moved: cursor=%d collapsed=%v", m.cursor, m.collapsedGroups)
	}
	m.Update(right)
	if m.collapsedGroups["test"] {
		t.Fatal("right did not expand the parent")
	}

	m.cursor = index(func(row hostRow) bool { return row.host.Alias == "delta" })
	m.Update(left)
	flat := index(func(row hostRow) bool { return row.header && row.group == "one/two/three/four/five" })
	if m.cursor != flat {
		t.Fatalf("left on a host in a flattened chain did not focus the visible header: cursor=%d", m.cursor)
	}
	m.Update(left)
	if !m.collapsedGroups["one/two/three/four/five"] {
		t.Fatal("left did not collapse the flattened header")
	}
	m.Update(left)
	if m.cursor != flat || !m.collapsedGroups["one/two/three/four/five"] {
		t.Fatalf("left looked for a parent row that flattening removed: cursor=%d collapsed=%v", m.cursor, m.collapsedGroups)
	}

	m.cursor = index(func(row hostRow) bool { return row.host.Alias == "loose" })
	loose := m.cursor
	m.Update(left)
	m.Update(right)
	if m.cursor != loose || m.collapsedGroups["loose"] {
		t.Fatalf("arrows changed an ungrouped host: cursor=%d collapsed=%v", m.cursor, m.collapsedGroups)
	}

	m.search = "a"
	m.cursor = index(func(row hostRow) bool { return row.header && row.group == "test" })
	m.Update(left)
	if m.collapsedGroups["test"] || !strings.Contains(m.status, "Clear the search filter") {
		t.Fatalf("search left collapsed or stayed quiet: collapsed=%v status=%q", m.collapsedGroups, m.status)
	}
	m.cursor = index(func(row hostRow) bool { return row.host.Alias == "alpha" })
	m.Update(left)
	if m.cursor != index(func(row hostRow) bool { return row.header && row.group == "test/abc" }) {
		t.Fatal("left during search did not move from a host to its parent")
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
	if m.form != nil || m.statusError {
		t.Fatalf("synced host should not offer move: form=%#v status=%q", m.form, m.status)
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

	t.Run("Ctrl+S", func(t *testing.T) {
		m := testApp(t)
		if err := m.metadata.SetHost("alpha", metadata.Host{Favorite: true, ConnectionCount: 4, Notes: "keep"}); err != nil {
			t.Fatal(err)
		}
		m.openEditHostForm()
		enterHostFormSection(t, m, formSectionMetadata)
		focused := false
		for i := range m.form.fields {
			if m.form.fields[i].label == rdpFieldLabel {
				m.form.index = i
				m.form.fields[i].selected = 1
				focused = true
				break
			}
		}
		if !focused {
			t.Fatal("metadata section has no Open RDP field")
		}
		m.updateForm(ctrlS())
		if m.form != nil {
			t.Fatal("Ctrl+S did not save from a host form section")
		}
		got := m.metadata.Host("alpha")
		if !got.RDP || !got.Favorite || got.ConnectionCount != 4 || got.Notes != "keep" {
			t.Fatalf("metadata = %+v", got)
		}
	})

	t.Run("Cmd+S", func(t *testing.T) {
		m := testApp(t)
		m.openEditHostForm()
		m.form.input.SetValue("Saved with command")
		m.updateForm(superS())
		if m.form != nil {
			t.Fatal("Cmd+S did not save the host form")
		}
		if got := m.metadata.Host("alpha").Label; got != "Saved with command" {
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

func TestHostEditorSheetOffersSave(t *testing.T) {
	m := testApp(t)
	m.Update(press("a"))
	if m.form == nil || !isHostForm(m.form) {
		t.Fatal("a should open the host editor")
	}
	if got := sheet(m); !strings.Contains(got, "save") || strings.Contains(got, "Ctrl+J") || strings.Contains(got, "connect") {
		t.Fatalf("host editor sheet = %q", got)
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
	if !strings.Contains(rendered, "Label") || !strings.Contains(rendered, "Customisation") || strings.Contains(rendered, "Authentication") || strings.Contains(rendered, "Hostname") {
		t.Fatalf("metadata editor layout is incorrect:\n%s", rendered)
	}
	enterHostFormSection(t, m, formSectionMetadata)
	rendered = m.renderForm(m.styles())
	if strings.Contains(rendered, "Group") || !strings.Contains(rendered, "Tags") {
		t.Fatalf("metadata section was not opened:\n%s", rendered)
	}
	footer := sheet(m)
	if !strings.Contains(footer, "save") || strings.Contains(footer, "Ctrl+J") || strings.Contains(footer, "connect") {
		t.Fatalf("metadata editor sheet is incorrect: %q", footer)
	}
}

func TestFooterShowsControlsForTheActiveFormState(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	if footer := sheet(m); !strings.Contains(footer, "next") || !strings.Contains(footer, "save") || strings.Contains(footer, "Ctrl+J") || !strings.Contains(footer, "cancel") || strings.Contains(footer, "connect") {
		t.Fatalf("create sheet = %q", footer)
	}

	m.openEditHostForm()
	if footer := sheet(m); !strings.Contains(footer, "save") || strings.Contains(footer, "Ctrl+J") || strings.Contains(footer, "connect") {
		t.Fatalf("edit sheet = %q", footer)
	}
	enterHostFormSection(t, m, formSectionMetadata)
	if footer := sheet(m); strings.Contains(footer, "quit") {
		t.Fatalf("plain-text host field advertises quit: %q", footer)
	}

	m.hosts[0].Managed = true
	m.openEditHostForm()
	enterHostFormSection(t, m, formSectionAuth)
	if footer := sheet(m); !strings.Contains(footer, "quit") {
		t.Fatalf("selectable host field does not advertise quit: %q", footer)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if footer := sheet(m); !strings.Contains(footer, "choose") || !strings.Contains(footer, "select") || !strings.Contains(footer, "cancel") {
		t.Fatalf("choice sheet = %q", footer)
	}
}

func TestEditFormUsesArrowsToMoveAndEnterToSave(t *testing.T) {
	m := testApp(t)
	m.openEditHostForm()
	focusHostFormField(t, m, "Tags")
	if m.form == nil || m.form.fields[m.form.index].label != "Tags" {
		t.Fatal("tags field was not focused")
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
	m.form.hubIndex = hostHubIndex(t, m, "advanced")
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

func TestColorGridMovesAndRendersSwatches(t *testing.T) {
	if got, _ := moveColorGrid(0, 10, 5, "right"); got != 1 {
		t.Fatalf("right = %d", got)
	}
	if got, _ := moveColorGrid(0, 10, 5, "left"); got != 0 {
		t.Fatalf("left edge = %d", got)
	}
	if got, _ := moveColorGrid(1, 10, 5, "down"); got != 6 {
		t.Fatalf("down = %d", got)
	}
	if got, _ := moveColorGrid(6, 10, 5, "up"); got != 1 {
		t.Fatalf("up = %d", got)
	}
	if got, _ := moveColorGrid(4, 10, 5, "right"); got != 4 {
		t.Fatalf("right edge = %d", got)
	}
	if got, _ := moveColorGrid(9, 10, 5, "down"); got != 9 {
		t.Fatalf("down edge = %d", got)
	}

	m := testApp(t)
	m.openAddHostForm()
	focusHostFormField(t, m, "Color")
	m.form.selecting = true
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	item := m.form.fields[m.form.index]
	if item.options[item.selected].value != "red" {
		t.Fatalf("highlight = %q", item.options[item.selected].value)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	item = m.form.fields[m.form.index]
	if item.options[item.selected].value != "cyan" {
		t.Fatalf("down highlight = %q", item.options[item.selected].value)
	}
	body := m.renderHostForm(m.styles())
	red := lipgloss.NewStyle().Background(lipgloss.Color("1")).Render(strings.Repeat(" ", colorSwatchInner))
	if !strings.Contains(body, red) || !strings.Contains(body, "Default") || !strings.Contains(body, "Cyan") {
		t.Fatalf("colour grid:\n%s", body)
	}
	footer := sheet(m)
	if !strings.Contains(footer, "highlight") {
		t.Fatalf("sheet = %q", footer)
	}
}

func TestEmptyColorHexReturnsToPalette(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	focusHostFormField(t, m, "Color")
	item := &m.form.fields[m.form.index]
	for i, option := range item.options {
		if option.custom {
			item.selected = i
			break
		}
	}
	item.customValue = "#AABBCC"
	m.form.selecting = false
	m.focusFormField()

	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form.selecting || m.form.input.Value() == "" {
		t.Fatal("backspace should delete a hex character before leaving the field")
	}

	m.form.input.SetValue("")
	body := m.renderHostForm(m.styles())
	if !strings.Contains(body, "⌫/Esc colours") {
		t.Fatalf("empty hex field should show how to return:\n%s", body)
	}
	footer := sheet(m)
	if !strings.Contains(footer, "colours") {
		t.Fatalf("sheet = %q", footer)
	}

	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if !m.form.selecting {
		t.Fatal("backspace on an empty hex field should reopen the colours")
	}
	body = m.renderHostForm(m.styles())
	if !strings.Contains(body, "Gray") || !strings.Contains(body, "Custom") {
		t.Fatalf("palette should be visible:\n%s", body)
	}

	m = testApp(t)
	m.openForm("Edit group: Work", "group_edit", []field{colorFormField("#ABC", "")})
	m.form.selecting = false
	m.focusFormField()
	m.form.input.SetValue("")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form == nil || !m.form.selecting {
		t.Fatal("group colour hex should return to the palette")
	}
}

func TestHostFormBackspaceNavigatesSubmenus(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	focusHostFormField(t, m, "Tags")
	m.form.input.SetValue("")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form == nil || m.form.fields[m.form.index].label != "Tags" {
		t.Fatal("backspace in an empty text field navigated away")
	}

	focusHostFormField(t, m, "Hostname")
	m.form.input.SetValue("prod")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form.fields[m.form.index].label != "Hostname" || m.form.input.Value() != "pro" {
		t.Fatalf("backspace should edit text before navigating: label=%q value=%q", m.form.fields[m.form.index].label, m.form.input.Value())
	}
	m.form.input.SetValue("")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form == nil || m.form.fields[m.form.index].label != "Hostname" {
		t.Fatal("backspace in an empty hostname field should remain in the field")
	}

	m.form.hubIndex = hostHubIndex(t, m, "advanced")
	m.focusHostHubItem()
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if m.form != nil {
		t.Fatal("backspace on Advanced should close the form")
	}

	m.openAddHostForm()
	enterHostFormSection(t, m, formSectionAdvanced)
	if footer := sheet(m); !strings.Contains(footer, "open") || !strings.Contains(footer, "back") {
		t.Fatalf("advanced hub sheet = %q", footer)
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

func TestHostFormScrollKeepsTheFocusedFieldVisible(t *testing.T) {
	m := testApp(t)
	m.height = 14
	m.openAddHostForm()
	focusHostFormField(t, m, "Notes")
	rendered := m.renderForm(m.styles())
	if !strings.Contains(rendered, "› Notes") {
		t.Fatalf("focused Notes field is not in the form window:\n%s", rendered)
	}
	if strings.Contains(rendered, "Hostname") {
		t.Fatalf("scroll window still includes Hostname:\n%s", rendered)
	}
}

func TestFormRevealsFieldsProgressivelyAndRevisitsThem(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	initial := m.renderForm(m.styles())
	if !strings.Contains(initial, "Basics") || !strings.Contains(initial, "Hostname") || !strings.Contains(initial, "Customisation") || !strings.Contains(initial, "Advanced") || strings.Contains(initial, "Authentication") {
		t.Fatalf("initial hub form layout is incorrect:\n%s", initial)
	}
	m.form.input.SetValue("prod.example")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	second := m.renderForm(m.styles())
	if !strings.Contains(second, "Hostname  prod.example") || !strings.Contains(second, "› User") {
		t.Fatalf("Enter did not advance to user:\n%s", second)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.form.hubIndex != 0 || m.form.fields[m.form.index].label != "Hostname" {
		t.Fatalf("up did not revisit hostname: hubIndex=%d", m.form.hubIndex)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.form.fields[m.form.index].label != "User" {
		t.Fatalf("down did not return to user: %q", m.form.fields[m.form.index].label)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	third := m.renderForm(m.styles())
	if m.form.fields[m.form.index].label != methodFieldLabel || !strings.Contains(third, "› Method") {
		t.Fatal("down did not move to method")
	}
}

func TestHostFormExplainsOptionalConnectionFields(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	focusHostFormField(t, m, "Label")

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

	m.Update(press("a"))
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
	help := strings.Join(m.paletteContent(m.styles(), 80), "\n")
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
	footer := sheet(m)
	if !strings.Contains(footer, "delete") || strings.Contains(footer, "save") {
		t.Fatalf("delete sheet is incorrect: %q", footer)
	}
	m.windowsTerminal = true
	footer = sheet(m)
	if !strings.Contains(footer, "delete") || strings.Contains(footer, nerdEnterGlyph) {
		t.Fatalf("Windows Terminal delete sheet is incorrect: %q", footer)
	}
	if chord := formatChord(Binding{Chord: nerdEnterGlyph + " then ~."}, true, true); chord != "↵ then ~." {
		t.Fatalf("Windows Terminal chord = %q", chord)
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
	if rendered := m.render(); !strings.Contains(rendered, "Name to type") {
		t.Fatalf("copy affordance is not visible:\n%s", rendered)
	}
	if !strings.Contains(sheet(m), "copy name") {
		t.Fatalf("sheet = %q", sheet(m))
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

func TestSyncListHasNoTiles(t *testing.T) {
	m := testApp(t)
	m.width = 80
	m.section = syncSection
	raw := m.renderSync(m.styles())
	body := stripANSI(raw)
	if strings.Contains(body, "┌") || strings.Contains(body, "└") {
		t.Fatalf("sync list should not draw tiles:\n%s", body)
	}
	for _, name := range []string{"Google Cloud", "Amazon EC2", "Boat", "boxd"} {
		if !strings.Contains(body, name) {
			t.Fatalf("missing %s\n%s", name, raw)
		}
	}
}

func TestPaletteUsesTerminalWidthWithoutWrapping(t *testing.T) {
	m := testApp(t)
	m.width = 100
	if got := m.contentWidth(); got < 80 {
		t.Fatalf("contentWidth = %d", got)
	}
	for _, line := range m.paletteContent(m.styles(), m.terminalWidth()) {
		if strings.Contains(line, "\n") {
			t.Fatalf("sheet line wrapped internally: %q", line)
		}
		if lipgloss.Width(line) > m.terminalWidth() {
			t.Fatalf("sheet line wider than the terminal: %q", line)
		}
	}
}

func TestSyncListStacksOnMobile(t *testing.T) {
	m := testApp(t)
	m.width = 50
	m.height = 40
	m.section = syncSection
	if !m.isMobileLayout() {
		t.Fatal("expected mobile layout")
	}
	raw := m.renderSync(m.styles())
	body := stripANSI(raw)
	if strings.Contains(raw, "┌") || strings.Contains(body, "│") {
		t.Fatalf("mobile sync should stack, not tile:\n%s", raw)
	}
	if !strings.Contains(body, "─") {
		t.Fatalf("mobile sync should separate list and settings:\n%s", body)
	}
	if !strings.Contains(body, "Google Cloud") || !strings.Contains(body, "Boat") || !strings.Contains(body, "Upstash") || !strings.Contains(body, "Vercel") || !strings.Contains(body, "Hetzner") || !strings.Contains(body, "BitQate") {
		t.Fatalf("mobile sync body:\n%s", body)
	}
}

func TestSyncGridShowsNerdIconsWhenEnabled(t *testing.T) {
	m := testApp(t)
	m.width = 80
	m.height = 40
	m.section = syncSection
	m.nerdFont = true
	body := m.renderSync(m.styles())
	for _, icon := range []string{"\ue7f1", "\ue7ad", "\ue754", "\U000f0ec8", "\uf1b2", "\u25b2", "\uf233"} {
		if !strings.Contains(body, icon) {
			t.Fatalf("missing icon %q in:\n%s", icon, body)
		}
	}
}

func TestProviderChooserTitleKeepsIconWhenNarrow(t *testing.T) {
	m := testApp(t)
	m.nerdFont = true
	title := m.providerChooserTitle(cloud.Azure, 12)
	if !strings.Contains(title, "\ue754") {
		t.Fatalf("narrow title dropped icon: %q", title)
	}
}

func TestSyncGridScrollsWhenProvidersOverflow(t *testing.T) {
	m := testApp(t)
	m.width = 50
	m.height = 16
	m.section = syncSection
	m.syncCursor = 0
	if !m.isMobileLayout() {
		t.Fatal("expected mobile layout")
	}
	body := m.renderSync(m.styles())
	if strings.Contains(body, "BitQate") {
		t.Fatalf("short mobile view should clip later providers:\n%s", body)
	}
	if !strings.Contains(body, "Amazon EC2") {
		t.Fatalf("first providers should be visible:\n%s", body)
	}

	m.updateSyncKeys("G")
	body = m.renderSync(m.styles())
	if !strings.Contains(body, "boxd") {
		t.Fatalf("jumping to the last provider should reveal it:\n%s", body)
	}
	if strings.Contains(body, "Amazon EC2") {
		t.Fatalf("first providers should scroll away:\n%s", body)
	}

	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 4, Y: 4, Button: tea.MouseWheelUp}))
	body = m.renderSync(m.styles())
	if m.syncCursor != len(m.syncMenuItems())-2 {
		t.Fatalf("wheel up should move one provider, cursor=%d", m.syncCursor)
	}
	if !strings.Contains(body, "DigitalOcean") {
		t.Fatalf("wheel should keep the focused provider in view:\n%s", body)
	}

	layout := m.panelLayout()
	start := scrollStart(m.syncCursor, len(cloud.Descriptors()), layout.listHeight)
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: layout.listTop, Button: tea.MouseLeft}))
	if m.syncCursor != start {
		t.Fatalf("click on first visible row should select %d, got %d", start, m.syncCursor)
	}

	m.width = 100
	m.height = 7
	m.syncCursor = 0
	if m.isMobileLayout() {
		t.Fatal("expected desktop layout")
	}
	body = m.renderSync(m.styles())
	if strings.Contains(body, "Hetzner") {
		t.Fatalf("short desktop view should clip later provider rows:\n%s", body)
	}
	m.updateSyncKeys("G")
	if view := m.render(); !strings.Contains(view, "boxd") {
		t.Fatalf("full view should show the focused provider after scroll:\n%s", view)
	}
}

func TestSyncSettingsScroll(t *testing.T) {
	m := testApp(t)
	m.width = 100
	m.height = 10
	m.section = syncSection
	m.updateSyncKeys("l")
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "Login") {
		t.Fatalf("top of settings should stay visible:\n%s", body)
	}
	if strings.Contains(body, "Add service account") {
		t.Fatalf("short settings pane should clip later rows:\n%s", body)
	}
	m.updateSyncKeys("G")
	body = m.renderSync(m.styles())
	if !strings.Contains(body, "Add service account") {
		t.Fatalf("last setting should scroll into view:\n%s", body)
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
	footer := stripANSI(m.renderFooter(m.styles()))
	if strings.Contains(footer, "connect") || !strings.HasPrefix(strings.TrimLeft(footer, " "), "/ for shortcuts") {
		t.Fatalf("mobile status row = %q", footer)
	}
	if strings.Contains(sheet(m), "enter/click Connect") {
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

func TestOpenRDPUsesTheHostAddress(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 100, 30
	m.hosts[0].Resolved = sshconfig.Resolved{HostName: "rdp.example", User: "alice", Port: "2222"}
	m.hosts[0].Synced = true
	var got rdp.Session
	calls := 0
	prev := launchRDP
	launchRDP = func(session rdp.Session) error {
		if _, _, err := rdp.Command("linux", session); err != nil {
			return err
		}
		calls++
		got = session
		return nil
	}
	t.Cleanup(func() { launchRDP = prev })

	selectHostAlias(t, m, "alpha")
	if err := m.metadata.SetHost("alpha", metadata.Host{RDP: true}); err != nil {
		t.Fatal(err)
	}
	m.refreshHostMetadata()
	m.Update(ctrlO())
	if calls != 1 || got.Address != "rdp.example" || got.User != "" || got.Domain != "" {
		t.Fatalf("session = %+v calls=%d", got, calls)
	}
	if m.statusError || !strings.Contains(m.status, "rdp.example") {
		t.Fatalf("status = %q error=%v", m.status, m.statusError)
	}
	if !strings.Contains(strings.Join(m.paletteContent(m.styles(), 120), "\n"), "rdp") {
		t.Fatal("sheet should list Remote Desktop once the host enables it")
	}

	m.hosts[0].Resolved.HostName = ""
	m.Update(ctrlO())
	if got.Address != "alpha" {
		t.Fatalf("alias fallback = %q", got.Address)
	}

	m.hosts[0].Resolved.HostName = "bad;rm -rf /"
	m.Update(ctrlO())
	if !m.statusError || strings.Contains(m.status, "rm") || strings.Contains(m.status, "executable file not found") {
		t.Fatalf("invalid address status = %q", m.status)
	}

	if err := m.metadata.SetHost("alpha", metadata.Host{RDP: true}); err != nil {
		t.Fatal(err)
	}
	m.refreshHostMetadata()
	m.hosts[0].Resolved.HostName = "rdp.example"
	m.status, m.statusError = "", false
	body := m.renderHosts(m.styles())
	if !strings.Contains(body, rdpAction) || !strings.Contains(body, connectAction) {
		t.Fatalf("detail is missing the RDP chip:\n%s", body)
	}
	sheet := strings.Join(m.paletteContent(m.styles(), 120), "\n")
	if !strings.Contains(sheet, "Ctrl+O") || !strings.Contains(sheet, "rdp") {
		t.Fatalf("sheet = %q", sheet)
	}

	calls = 0
	m.statusError = false
	layout := m.panelLayout()
	x, y, width, ok := m.rdpButtonBounds(layout)
	if !ok || x <= layout.listWidth {
		t.Fatalf("rdp bounds = %d,%d,%d ok=%v", x, y, width, ok)
	}
	connectX, connectY, _ := m.connectButtonBounds(layout)
	if x < connectX+1 || y != connectY {
		t.Fatalf("rdp chip is not on the connect row: connect=%d,%d rdp=%d,%d", connectX, connectY, x, y)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x + width/2, Y: y, Button: tea.MouseLeft}))
	if calls != 1 {
		t.Fatalf("RDP chip calls = %d", calls)
	}
	calls = 0
	_, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: connectX, Y: connectY, Button: tea.MouseLeft}))
	if calls != 0 || cmd == nil {
		t.Fatalf("connect chip launched RDP or did not connect: calls=%d cmd=%v", calls, cmd != nil)
	}

	if err := m.metadata.SetHost("beta", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = -1
	for i, row := range m.hostListRows() {
		if row.header && row.group == "Work" {
			m.cursor = i
			break
		}
	}
	if m.cursor < 0 {
		t.Fatal("Work header was not listed")
	}
	calls = 0
	m.Update(ctrlO())
	if calls != 0 || !strings.Contains(m.status, "Select a host") {
		t.Fatalf("group row launched RDP: calls=%d status=%q", calls, m.status)
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
	if !strings.Contains(lines[len(lines)-1], "/") {
		t.Fatalf("footer was not on the bottom row: %q", lines[len(lines)-1])
	}
}

func TestSyncAndFilesRulesMeetTheColumnSplit(t *testing.T) {
	for _, sec := range []section{syncSection, filesSection} {
		m := testApp(t)
		m.width, m.height = 80, 24
		m.section = sec
		rendered := stripANSI(m.render())
		lines := strings.Split(rendered, "\n")
		listWidth, _, bodyHeight := m.columnDimensions()
		if strings.Count(rendered, "┬") != 1 {
			t.Fatalf("%v header rule junctions = %d\n%s", sec, strings.Count(rendered, "┬"), rendered)
		}
		if len(lines) < 2+bodyHeight {
			t.Fatalf("%v render has %d lines", sec, len(lines))
		}
		if got := runeColumn(lines[1], '┬'); got != listWidth {
			t.Fatalf("%v junction column = %d, want %d: %q", sec, got, listWidth, lines[1])
		}
		for i := 2; i < 2+bodyHeight; i++ {
			if strings.Count(lines[i], "│") != 1 || runeColumn(lines[i], '│') != listWidth {
				t.Fatalf("%v line %d divider = %q", sec, i, lines[i])
			}
		}
	}

	m := testApp(t)
	m.width, m.height = 80, 24
	m.section = vaultSection
	if strings.Contains(m.render(), "┬") {
		t.Fatal("vault should keep a plain header rule")
	}
	m.section = filesSection
	m.files.preview.active = true
	if strings.Contains(stripANSI(m.render()), "┬") {
		t.Fatal("file preview should keep a plain header rule")
	}
	m.files.preview.active = false
	m.width = 40
	if strings.Contains(stripANSI(m.render()), "┬") || strings.Contains(stripANSI(m.renderFiles(m.styles())), "│") {
		t.Fatal("mobile files should stack without a column junction")
	}
}

func runeColumn(s string, r rune) int {
	column := 0
	for _, c := range s {
		if c == r {
			return column
		}
		column++
	}
	return -1
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
		"discord.gg/bast",
		"github.com/ellipse-software/bast",
		"MIT License",
		"v1.2.3",
		"Sponsor",
	} {
		if !strings.Contains(rendered, text) {
			t.Fatalf("credits screen does not contain %q:\n%s", text, rendered)
		}
	}
	for _, label := range []string{"Created by", "Website", "Discord", "Repository", "License", "Version"} {
		for _, line := range strings.Split(rendered, "\n") {
			if strings.Contains(line, "Sponsor") {
				continue
			}
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

func TestCreditsSponsorOpensBastSponsorPage(t *testing.T) {
	var opened string
	prev := openBrowser
	openBrowser = func(raw string) error {
		opened = raw
		return nil
	}
	t.Cleanup(func() { openBrowser = prev })

	m := testApp(t)
	m.Update(press("v"))
	if !m.credits {
		t.Fatal("v should open about")
	}
	m.Update(press("s"))
	if opened != sponsorURL || !m.credits {
		t.Fatalf("s should open %s from about, opened=%q credits=%v", sponsorURL, opened, m.credits)
	}

	opened = ""
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if opened != sponsorURL {
		t.Fatalf("enter should open sponsor, opened=%q", opened)
	}

	opened = ""
	m.Update(press("l"))
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if opened != discordURL {
		t.Fatalf("enter on Discord should open %s, opened=%q", discordURL, opened)
	}
	m.Update(press("l"))
	opened = ""
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if opened != websiteURL {
		t.Fatalf("enter on Website should open %s, opened=%q", websiteURL, opened)
	}
	m.Update(press("h"))
	m.Update(press("h"))
	opened = ""
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if opened != sponsorURL || m.creditsChoice != 0 {
		t.Fatalf("h should return to Sponsor, choice=%d opened=%q", m.creditsChoice, opened)
	}

	opened = ""
	x, y, width := m.creditsSponsorBounds()
	if width == 0 {
		t.Fatal("sponsor chip bounds missing")
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	if opened != sponsorURL {
		t.Fatalf("click should open sponsor at (%d,%d), opened=%q", x, y, opened)
	}
}

func TestActionSheetOpensAndCloses(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 24
	m.Update(press("?"))
	if !m.palette {
		t.Fatal("? did not open the action sheet")
	}
	rendered := stripANSI(m.render())
	for _, text := range []string{"Selection", "Move", "add", "connect"} {
		if !strings.Contains(rendered, text) {
			t.Fatalf("sheet does not contain %q:\n%s", text, rendered)
		}
	}
	if strings.Contains(rendered, "jump") && strings.Contains(rendered, "directory") {
		t.Fatalf("hosts sheet should not include Files actions:\n%s", rendered)
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) != m.height {
		t.Fatalf("render height = %d, want %d", len(lines), m.height)
	}
	if strings.Contains(lines[len(lines)-1], "/ for shortcuts") {
		t.Fatalf("open sheet should hide the shortcut prompt: %q", lines[len(lines)-1])
	}

	m.Update(press("j"))
	if m.palette {
		t.Fatal("a listed key should run and close the sheet")
	}

	m.Update(press("/"))
	if !m.palette {
		t.Fatal("/ did not open the action sheet")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.palette || m.paletteOffset != 0 {
		t.Fatalf("Esc should close the sheet: palette=%v offset=%d", m.palette, m.paletteOffset)
	}
	closed := stripANSI(strings.Split(m.render(), "\n")[m.height-1])
	if !strings.HasPrefix(strings.TrimLeft(closed, " "), "/ for shortcuts") {
		t.Fatalf("status row = %q", closed)
	}
}

func TestDoctorOpensOutsideFilesAndCloses(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 24
	_, cmd := m.Update(press("D"))
	if !m.doctor || m.palette || cmd == nil {
		t.Fatalf("D should open doctor: doctor=%v palette=%v cmd=%v", m.doctor, m.palette, cmd)
	}
	m.Update(doctorDoneMsg{report: doctor.Report{
		Healthy: true,
		Findings: []doctor.Finding{{
			ID: "env.openssh_ok", Severity: doctor.SeverityOK, Category: doctor.CatEnv, Title: "ssh ok",
		}},
	}})
	if m.doctorLoading {
		t.Fatal("doctor should finish loading")
	}
	rendered := m.render()
	if !strings.Contains(rendered, "Doctor") || !strings.Contains(rendered, "ssh ok") {
		t.Fatalf("doctor overlay missing content:\n%s", rendered)
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.doctor {
		t.Fatal("Esc did not close doctor")
	}
}

func TestFilesDDoesNotOpenDoctor(t *testing.T) {
	m := testApp(t)
	m.section = filesSection
	m.initFilesState()
	m.Update(press("D"))
	if m.doctor {
		t.Fatal("D on Files opened doctor")
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
	if !strings.Contains(view, "No hosts yet") || !strings.Contains(view, "Your SSH map is empty.") {
		t.Fatalf("empty host state is not helpful:\n%s", view)
	}
	if strings.Contains(view, "Press") {
		t.Fatalf("empty hosts should not narrate the key:\n%s", view)
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
	if !strings.Contains(host, "Promote to Bast managed") {
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
	if !strings.Contains(key, m.keyInstallChip()) {
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
	if m.form != nil || !strings.Contains(m.status, "cannot be edited") {
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

	footer := sheet(m)
	if !strings.Contains(footer, "␣") || !strings.Contains(footer, "collapse") {
		t.Fatalf("sheet = %q", footer)
	}
	m.updateKeys(enter)
	footer = sheet(m)
	if !strings.Contains(footer, "expand") {
		t.Fatalf("collapsed sheet = %q", footer)
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
	rendered := renderManagedGroupName("Google Cloud", lipgloss.NewStyle(), false, false)
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
	if got := renderManagedGroupName("Work", lipgloss.NewStyle(), false, false); got != "Work" {
		t.Fatalf("ordinary group changed: %q", got)
	}
}

func TestAmazonEC2GroupNameUsesOneBrandColor(t *testing.T) {
	restStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8B5CF6"))
	rendered := renderManagedGroupName("Amazon EC2/default/eu-west-2", restStyle, false, false)
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
	rendered := renderManagedGroupName("Microsoft Azure/Production/apps", restStyle, false, false)
	provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0078D4")).Render("Microsoft Azure")
	if !strings.HasPrefix(rendered, provider) {
		t.Fatalf("Microsoft Azure brand colour was not applied uniformly: %q", rendered)
	}
	if !strings.Contains(rendered, restStyle.Render("/Production/apps")) {
		t.Fatalf("Microsoft Azure group path did not retain the normal style: %q", rendered)
	}
}

func TestBoatGroupNameUsesBrandColor(t *testing.T) {
	rendered := renderManagedGroupName("Boat", lipgloss.NewStyle(), false, false)
	provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#263FA9")).Render("Boat")
	if rendered != provider {
		t.Fatalf("Boat brand colour was not applied: %q", rendered)
	}
}

func TestVercelGroupNameIsWhiteTriangle(t *testing.T) {
	rendered := renderManagedGroupName("Vercel", lipgloss.NewStyle(), false, false)
	provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render("Vercel")
	if rendered != provider {
		t.Fatalf("Vercel brand colour was not applied: %q", rendered)
	}
	withIcon := renderManagedGroupName("Vercel", lipgloss.NewStyle(), true, false)
	if !strings.Contains(withIcon, "\u25b2") {
		t.Fatalf("Vercel triangle icon missing: %q", withIcon)
	}
}

func TestUpstashGroupNameUsesBrandColor(t *testing.T) {
	restStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	rendered := renderManagedGroupName("Upstash/dev", restStyle, false, false)
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
	rendered := renderManagedGroupName("Upstash", rest, false, false)
	want := brandText("#00E9A3", "Upstash", rest)
	if rendered != want {
		t.Fatalf("got %q want %q", rendered, want)
	}
	if !strings.Contains(rendered, "48;2;31;41;55") {
		t.Fatalf("expected rest background under Upstash brand text: %q", rendered)
	}
	icon := managedGroupIcon("Upstash", true, false)
	withIcon := renderManagedGroupName("Upstash", rest, true, false)
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
	icon := managedGroupIcon("Upstash", true, m.windowsTerminal)
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

	vercelSel := m.renderSyncTile(m.styles(), 0, syncMenuItem{provider: "vercel"}, 24)
	if !strings.Contains(vercelSel, "38;2;255;255;255") {
		t.Fatalf("selected Vercel tile should use white border:\n%q", vercelSel)
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

func TestBoatCreateBusyThenSelectsHost(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "boat"
	m.beginSyncBusy("Creating sandbox…")
	m.syncingProviders = map[string]bool{"boat": true}
	if !m.vaultBusyBlocksBody() {
		t.Fatal("expected create busy to replace the sync body")
	}
	busy := m.renderVaultBusy(m.styles())
	if !strings.Contains(busy, "boat.dev") || !strings.Contains(busy, "Creating sandbox…") {
		t.Fatalf("create busy body:\n%s", busy)
	}

	if err := m.metadata.SetHost("boat_sunny", metadata.Host{Label: "sunny", Group: "Boat"}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{"Boat": true}

	_, _ = m.Update(syncDoneMsg{
		provider:   "boat",
		result:     cloudsync.Result{Provider: "boat", Count: 1},
		focusAlias: "boat_sunny",
	})
	if m.syncBusy != "" {
		t.Fatalf("busy should clear after create, got %q", m.syncBusy)
	}
	if m.section != hostsSection || m.syncProvider != "" {
		t.Fatalf("expected Hosts after create, section=%v provider=%q", m.section, m.syncProvider)
	}
	if m.selectAfterLoadName != "boat_sunny" {
		t.Fatalf("selectAfterLoadName = %q", m.selectAfterLoadName)
	}

	_, _ = m.Update(loadedMsg{hosts: []sshconfig.Host{{
		Alias: "boat_sunny", Synced: true, SyncSource: "boat",
		Resolved: sshconfig.Resolved{HostName: "1.2.3.4", User: "user"},
	}}})
	host, ok := m.selectedHost()
	if !ok || host.Alias != "boat_sunny" {
		t.Fatalf("new sandbox was not selected: ok=%v host=%+v cursor=%d", ok, host, m.cursor)
	}
	if m.collapsedGroups["Boat"] {
		t.Fatalf("Boat group should be expanded for selection: %v", m.collapsedGroups)
	}
}

func TestStoppedBoatHiddenUntilToggled(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{
		{
			Alias: "boat_live", Synced: true, SyncSource: "boat",
			Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"},
		},
		{
			Alias: "boat_idle", Synced: true, SyncSource: "boat",
			Resolved: sshconfig.Resolved{HostName: "boat.stopped.invalid", User: "user"},
		},
	}
	if err := m.metadata.SetHost("boat_live", metadata.Host{Label: "live", Group: "Boat", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("boat_idle", metadata.Host{Label: "idle", Group: "Boat", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}

	var sawStopped, sawRunning bool
	for _, row := range m.hostRows() {
		if row.header {
			continue
		}
		switch row.host.Alias {
		case "boat_live":
			sawRunning = true
		case "boat_idle":
			sawStopped = true
		}
	}
	if !sawRunning {
		t.Fatal("running boat should stay visible")
	}
	if sawStopped {
		t.Fatal("stopped boat should be hidden by default")
	}

	m.showHidden = true
	sawStopped, sawRunning = false, false
	for _, row := range m.hostRows() {
		if row.header {
			continue
		}
		switch row.host.Alias {
		case "boat_live":
			sawRunning = true
		case "boat_idle":
			sawStopped = true
			if !m.hostLooksStopped(row.host) {
				t.Fatal("stopped boat should look stopped")
			}
		}
	}
	if !sawRunning || !sawStopped {
		t.Fatalf("expected both sandboxes after ., running=%v stopped=%v", sawRunning, sawStopped)
	}

	m.showHidden = false
	m.search = "idle"
	sawStopped = false
	for _, row := range m.hostRows() {
		if !row.header && row.host.Alias == "boat_idle" {
			sawStopped = true
		}
	}
	if !sawStopped {
		t.Fatal("search should reveal matching stopped sandboxes")
	}

	detail := m.renderHostDetail(m.styles(), m.hosts[1], 60)
	if !strings.Contains(detail, "stopped") || !strings.Contains(detail, "Boat stopped") {
		t.Fatalf("stopped detail:\n%s", detail)
	}
	if !strings.Contains(detail, resumeAction) || strings.Contains(detail, connectAction) {
		t.Fatalf("stopped detail should offer Resume, not Connect:\n%s", detail)
	}
}

func TestHetznerGroupHiddenWhenEveryServerIsOff(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "hetzner_off", Synced: true, SyncSource: "hetzner", SyncID: "hetzner/1",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "root"},
	}}
	if err := m.metadata.SetHetzner(metadata.HetznerIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("hetzner_off", metadata.Host{Label: "vpn", Group: "Hetzner Cloud/ted.ac/nbg1", Tags: []string{"state:off"}}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}

	for _, row := range m.hostRows() {
		if strings.HasPrefix(row.group, "Hetzner Cloud") || row.label == "Hetzner Cloud" || (!row.header && row.host.Alias == "hetzner_off") {
			t.Fatalf("stopped-only Hetzner inventory should hide the group, row=%+v", row)
		}
	}

	m.showHidden = true
	var sawGroup, sawHost bool
	for _, row := range m.hostRows() {
		if row.header && strings.HasPrefix(row.group, "Hetzner Cloud") {
			sawGroup = true
		}
		if !row.header && row.host.Alias == "hetzner_off" {
			sawHost = true
		}
	}
	if !sawGroup || !sawHost {
		t.Fatalf(". should reveal the Hetzner group and off server, group=%v host=%v", sawGroup, sawHost)
	}

	m.showHidden = false
	m.hosts = append(m.hosts, sshconfig.Host{
		Alias: "hetzner_on", Synced: true, SyncSource: "hetzner", SyncID: "hetzner/2",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.11", User: "root"},
	})
	if err := m.metadata.SetHost("hetzner_on", metadata.Host{Label: "web", Group: "Hetzner Cloud/ted.ac/nbg1", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	sawGroup, sawHost, sawOff := false, false, false
	for _, row := range m.hostRows() {
		if row.header && strings.HasPrefix(row.group, "Hetzner Cloud") {
			sawGroup = true
		}
		if row.header {
			continue
		}
		switch row.host.Alias {
		case "hetzner_on":
			sawHost = true
		case "hetzner_off":
			sawOff = true
		}
	}
	if !sawGroup || !sawHost {
		t.Fatal("a running server should show the Hetzner group")
	}
	if sawOff {
		t.Fatal("off servers stay hidden while a running one is visible")
	}
}

func TestBoatResumeActionsAreStateAware(t *testing.T) {
	m := testApp(t)
	running := sshconfig.Host{
		Alias: "boat_live", Synced: true, SyncSource: "boat", SyncID: "bx_live01",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"},
	}
	stopped := sshconfig.Host{
		Alias: "boat_idle", Synced: true, SyncSource: "boat", SyncID: "bx_idle01",
		Resolved: sshconfig.Resolved{HostName: "boat.stopped.invalid", User: "user"},
	}
	m.hosts = []sshconfig.Host{running, stopped}
	if err := m.metadata.SetHost("boat_live", metadata.Host{Label: "live", Group: "Boat", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("boat_idle", metadata.Host{Label: "idle", Group: "Boat", Tags: []string{"state:stopped"}}); err != nil {
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

	selectHost("boat_live")
	footer := sheet(m)
	if strings.Contains(footer, "resume") || !strings.Contains(footer, "stop") {
		t.Fatalf("running sheet = %q", footer)
	}
	if m.hostPrimaryAction(running) != connectAction {
		t.Fatalf("running primary action = %q", m.hostPrimaryAction(running))
	}
	if cmd := m.resumeSelectedBoat(running, false); cmd == nil {
		t.Fatal("expected notice cmd when resuming a running boat")
	}
	// r on a running boat should reload, not resume
	m.syncingProviders = map[string]bool{}
	_, _ = m.updateKeys(press("r"))
	if m.syncingProviders["boat"] {
		t.Fatal("r on running boat should not start resume")
	}

	selectHost("boat_idle")
	footer = sheet(m)
	if !strings.Contains(footer, "connect") || !strings.Contains(footer, "resume") || strings.Contains(footer, "stop") {
		t.Fatalf("stopped sheet = %q", footer)
	}
	if m.hostPrimaryAction(stopped) != resumeAction {
		t.Fatalf("stopped primary action = %q", m.hostPrimaryAction(stopped))
	}
	_, cmd := m.connectSelected()
	if !m.syncingProviders["boat"] || m.syncActivity != "resuming…" {
		t.Fatalf("enter should resume with activity label, syncing=%v activity=%q", m.syncingProviders["boat"], m.syncActivity)
	}
	if m.sandboxConnectAfter != "boat_idle" {
		t.Fatalf("enter should queue SSH after resume, got %q", m.sandboxConnectAfter)
	}
	if cmd == nil {
		t.Fatal("expected resume command")
	}
	footerLeft := m.renderFooter(m.styles())
	if !strings.Contains(footerLeft, "resuming…") || strings.Contains(footerLeft, "syncing…") {
		t.Fatalf("footer during resume = %q", footerLeft)
	}

	// After a successful resume reload, connect into the woken boat.
	m.syncingProviders = map[string]bool{}
	m.syncActivity = ""
	m.hosts = []sshconfig.Host{{
		Alias: "boat_idle", Synced: true, SyncSource: "boat", SyncID: "bx_idle01",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.20", User: "user"},
	}}
	if err := m.metadata.SetHost("boat_idle", metadata.Host{Label: "idle", Group: "Boat", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	m.selectAfterLoadSection, m.selectAfterLoadName = hostsSection, "boat_idle"
	_, cmd = m.Update(loadedMsg{hosts: m.hosts})
	if m.sandboxConnectAfter != "" {
		t.Fatal("sandboxConnectAfter should clear once connect is queued")
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
		!strings.Contains(body, "Microsoft Azure") || !strings.Contains(body, "Boat") {
		t.Fatalf("sync grid body:\n%s", body)
	}
	if !strings.Contains(body, "Sync now") {
		t.Fatalf("settings should offer Sync now:\n%s", body)
	}
	if !strings.Contains(body, "Enabled") || !strings.Contains(body, "38;2;66;133;244") {
		t.Fatalf("selected GCP row should keep the brand color:\n%s", body)
	}

	m.updateSyncKeys("j")
	if m.syncCursor != 1 || m.syncProvider != "" {
		t.Fatalf("j should move to AWS, cursor=%d provider=%q", m.syncCursor, m.syncProvider)
	}
	m.updateSyncKeys("j")
	m.updateSyncKeys("l")
	if m.syncProvider != "azure" {
		t.Fatalf("expected Azure provider, got %q", m.syncProvider)
	}
	body = m.renderSync(m.styles())
	if !strings.Contains(body, "Subscriptions") || !strings.Contains(body, "Resource groups") {
		t.Fatalf("azure settings:\n%s", body)
	}
	if !strings.Contains(body, "Login") || !strings.Contains(body, "Enabled") {
		t.Fatalf("azure settings should offer Login and Enabled:\n%s", body)
	}
	m.updateSyncKeys("esc")
	if m.syncProvider != "" {
		t.Fatalf("expected esc to return to the list, got %q", m.syncProvider)
	}
	m.syncCursor = 0
	m.updateSyncKeys("enter")
	if m.syncProvider != "gcp" {
		t.Fatalf("expected GCP provider, got %q", m.syncProvider)
	}
	body = m.renderSync(m.styles())
	if !strings.Contains(body, "Login") || !strings.Contains(body, "Projects") || !strings.Contains(body, "Sync now") {
		t.Fatalf("gcp settings:\n%s", body)
	}
	tabs := m.renderTabs(m.styles())
	if !strings.Contains(tabs, "[3] Vault") || !strings.Contains(tabs, "[4] Sync") || !strings.Contains(tabs, "[5] Files") {
		t.Fatalf("tabs = %q", tabs)
	}
}

func TestBoatProviderLifecycleRow(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "boat"
	m.syncStatus.Boat.Authenticated = true
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "New sandbox") || !strings.Contains(body, "Sync now") || !strings.Contains(body, "off") {
		t.Fatalf("boat settings:\n%s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "New sandbox") && strings.Contains(line, "Sync now") {
			t.Fatalf("New sandbox should be its own row:\n%s", body)
		}
	}
	activateSyncRow(t, m, cloud.Boat, "create")
	if m.form == nil || m.form.action != "boat_new" {
		t.Fatalf("enter on New sandbox should open form, got %#v", m.form)
	}
}

func TestUpstashProviderLifecycleRow(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "upstash"
	m.syncStatus.Upstash.HasKey = true
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "New box") || !strings.Contains(body, "New token") {
		t.Fatalf("upstash settings:\n%s", body)
	}
	activateSyncRow(t, m, cloud.Upstash, "create")
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

func TestBoatDeleteUsesRemoteConfirm(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "boat_dev", Synced: true, SyncSource: "boat", SyncID: "bx_dev0001",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"},
	}}
	if err := m.metadata.SetHost("boat_dev", metadata.Host{Label: "dev", Group: "Boat", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	m.section = hostsSection
	selectHostAlias(t, m, "boat_dev")
	_, _ = m.updateKeys(press("d"))
	if m.form == nil || m.form.action != "boat_delete" {
		t.Fatalf("d on boat host should confirm remote delete, got %#v", m.form)
	}
}

func TestVercelProviderLifecycleRow(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vercel"
	m.syncCursor = 0
	m.syncStatus.Vercel.HasToken = true
	if err := m.metadata.SetVercel(metadata.VercelIntegration{TeamID: "team_1", ProjectID: "prj_1"}); err != nil {
		t.Fatal(err)
	}
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "New sandbox") || !strings.Contains(body, "New token") {
		t.Fatalf("vercel settings:\n%s", body)
	}
	activateSyncRow(t, m, cloud.Vercel, "create")
	if m.form == nil || m.form.action != "vercel_new" {
		t.Fatalf("enter on New sandbox should open form, got %#v", m.form)
	}
}

func TestVercelCleanupAction(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vercel"
	m.syncCursor = 0
	m.syncStatus.Vercel.HasToken = true
	if err := m.metadata.SetVercel(metadata.VercelIntegration{
		Enabled: true, TeamID: "team_1", ProjectID: "prj_1", Unrestorable: []string{"idle", "temp"},
	}); err != nil {
		t.Fatal(err)
	}
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "Cleanup") {
		t.Fatalf("vercel page should offer Cleanup:\n%s", body)
	}
	if !strings.Contains(body, "2 unrestorable") {
		t.Fatalf("vercel page should show unrestorable count:\n%s", body)
	}
	activateSyncRow(t, m, cloud.Vercel, "setting:"+cloud.KeyCleanup)
	if m.form == nil || m.form.action != "vercel_cleanup" {
		t.Fatalf("enter on Cleanup should open form, got %#v", m.form)
	}
	if m.form.fields[0].placeholder != "cleanup" {
		t.Fatalf("cleanup confirm placeholder = %q", m.form.fields[0].placeholder)
	}
	if !strings.Contains(m.form.fields[0].description, "idle") || !strings.Contains(m.form.fields[0].description, "temp") {
		t.Fatalf("cleanup form should list sandboxes, got %q", m.form.fields[0].description)
	}
}

func TestUpstashAndHetznerTokenFormsLinkToConsole(t *testing.T) {
	m := testApp(t)
	m.openUpstashKeyForm()
	if m.form == nil || m.form.action != "upstash_key" || len(m.form.fields) == 0 {
		t.Fatalf("upstash form=%#v", m.form)
	}
	if got := m.form.fields[0].description; got != "https://console.upstash.com" {
		t.Fatalf("upstash key description = %q", got)
	}

	m.form = nil
	m.openHetznerKeyForm()
	if m.form == nil || m.form.action != "hetzner_key" {
		t.Fatalf("hetzner form=%#v", m.form)
	}
	found := false
	for _, field := range m.form.fields {
		if strings.Contains(field.description, "~/.config") || strings.Contains(field.description, "stored") {
			t.Fatalf("token form should not describe local storage: %+v", field)
		}
		if field.label == "API token" {
			found = true
			if field.description != "https://console.hetzner.cloud" {
				t.Fatalf("hetzner token description = %q", field.description)
			}
		}
	}
	if !found {
		t.Fatal("expected API token field")
	}
}

func TestVercelCleanupProgressUpdatesBusy(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "vercel"
	m.syncOpGen = map[string]uint64{"vercel": 1}
	m.syncingProviders = map[string]bool{"vercel": true}
	m.beginSyncBusy("Cleaning up Vercel…")
	if !m.vaultBusyBlocksBody() {
		t.Fatal("cleanup should replace the sync body")
	}
	_, cmd := m.Update(syncProgressMsg{provider: "vercel", opGen: 1, label: "Cleaning up Vercel 12/340"})
	if m.syncBusy != "Cleaning up Vercel 12/340" {
		t.Fatalf("busy = %q", m.syncBusy)
	}
	body := m.renderVaultBusy(m.styles())
	if !strings.Contains(body, "12/340") {
		t.Fatalf("busy body:\n%s", body)
	}
	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "12/340") {
		t.Fatalf("footer = %q", footer)
	}
	_ = cmd
	_, _ = m.Update(syncDoneMsg{provider: "vercel", opGen: 1, notice: "Deleted 340 unrestorable sandboxes"})
	if m.syncBusy != "" {
		t.Fatalf("busy should clear after cleanup, got %q", m.syncBusy)
	}
}

func TestVercelFilesUnavailable(t *testing.T) {
	m := testApp(t)
	host := sshconfig.Host{Alias: "vercel_dev", Synced: true, SyncSource: "vercel", SyncID: "prj_1/dev"}
	cmd := m.openFilesForHost(host)
	if m.section == filesSection {
		t.Fatal("Vercel Files should stay on Hosts")
	}
	if cmd == nil {
		t.Fatal("expected SFTP unavailable notice")
	}
}

func TestVercelDeleteUsesRemoteConfirm(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "vercel_dev", Synced: true, SyncSource: "vercel", SyncID: "prj_1/dev",
		Resolved: sshconfig.Resolved{HostName: "vercel.sandbox.invalid"},
	}}
	if err := m.metadata.SetHost("vercel_dev", metadata.Host{Label: "dev", Group: "Vercel", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	m.section = hostsSection
	selectHostAlias(t, m, "vercel_dev")
	_, _ = m.updateKeys(press("d"))
	if m.form == nil || m.form.action != "vercel_delete" {
		t.Fatalf("d on vercel host should confirm remote delete, got %#v", m.form)
	}
}

func TestProviderPageJMovesToConfig(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "gcp"
	m.syncCursor = 0
	body := m.renderSync(m.styles())
	if !strings.Contains(body, "off") {
		t.Fatalf("expected disabled status:\n%s", body)
	}
	if !strings.Contains(body, "Login") || !strings.Contains(body, "Enabled") {
		t.Fatalf("expected Login and Enabled:\n%s", body)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 1 {
		t.Fatalf("j should move to the next setting, cursor=%d", m.syncCursor)
	}
	rows := m.syncSettingRows(cloud.GCP)
	if len(rows) < 2 || rows[1].label != "Enabled" {
		t.Fatalf("second row = %+v", rows)
	}
	m.updateSyncKeys("k")
	if m.syncCursor != 0 {
		t.Fatalf("k should return to Login, cursor=%d", m.syncCursor)
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
	m.syncProvider = "boat"
	m.hosts = []sshconfig.Host{
		{Alias: "boat_run", Synced: true, SyncSource: "boat", SyncID: "bx_run001", Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"}},
		{Alias: "boat_stop", Synced: true, SyncSource: "boat", SyncID: "bx_stop01", Resolved: sshconfig.Resolved{HostName: "boat.stopped.invalid", User: "user"}},
	}
	if err := m.metadata.SetHost("boat_run", metadata.Host{Label: "alpha-boat", Group: "Boat", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("boat_stop", metadata.Host{Label: "idle-boat", Group: "Boat", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	body := m.renderSync(m.styles())
	if strings.Contains(body, "alpha-boat") || strings.Contains(body, "idle-boat") {
		t.Fatalf("sync settings should not list hosts:\n%s", body)
	}
	if !strings.Contains(body, "Login") || !strings.Contains(body, "Enabled") {
		t.Fatalf("boat settings:\n%s", body)
	}

	m.section = hostsSection
	m.collapsedGroups = map[string]bool{}
	var sawRun, sawStop bool
	for _, row := range m.hostRows() {
		if row.header {
			continue
		}
		switch row.host.Alias {
		case "boat_run":
			sawRun = true
		case "boat_stop":
			sawStop = true
		}
	}
	if !sawRun || sawStop {
		t.Fatalf("hosts should show the running sandbox and hide the stopped one, running=%v stopped=%v", sawRun, sawStop)
	}
	m.showHidden = true
	sawRun, sawStop = false, false
	for _, row := range m.hostRows() {
		if row.header {
			continue
		}
		switch row.host.Alias {
		case "boat_run":
			sawRun = true
		case "boat_stop":
			sawStop = true
		}
	}
	if !sawRun || !sawStop {
		t.Fatalf("showing hidden hosts should reveal both sandboxes, running=%v stopped=%v", sawRun, sawStop)
	}
}

func TestProviderInventorySandboxLifecycle(t *testing.T) {
	m := testApp(t)
	m.section = hostsSection
	m.showHidden = true
	m.collapsedGroups = map[string]bool{}
	m.hosts = []sshconfig.Host{
		{Alias: "boat_run", Synced: true, SyncSource: "boat", SyncID: "bx_run001", Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"}},
		{Alias: "boat_stop", Synced: true, SyncSource: "boat", SyncID: "bx_stop01", Resolved: sshconfig.Resolved{HostName: "boat.stopped.invalid", User: "user"}},
	}
	if err := m.metadata.SetHost("boat_run", metadata.Host{Label: "alpha-boat", Group: "Boat", Tags: []string{"state:running", "snapshot"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("boat_stop", metadata.Host{Label: "idle-boat", Group: "Boat", Tags: []string{"state:stopped", "snapshot"}}); err != nil {
		t.Fatal(err)
	}

	selectHostAlias(t, m, "boat_run")
	_, _ = m.updateKeys(press("o"))
	if m.form == nil || m.form.action != "boat_stop" {
		t.Fatalf("o on running boat should stop, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "boat_fork" {
		t.Fatalf("n on running boat should fork, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("d"))
	if m.form == nil || m.form.action != "boat_delete" {
		t.Fatalf("d on running boat should delete, got %#v", m.form)
	}

	m.form = nil
	selectHostAlias(t, m, "boat_stop")
	footer := sheet(m)
	if !strings.Contains(footer, "resume") || strings.Contains(footer, "stop") {
		t.Fatalf("stopped boat sheet = %q", footer)
	}
	_, cmd := m.updateKeys(press("r"))
	if cmd == nil {
		t.Fatal("r on stopped boat should resume")
	}
	if m.sandboxConnectAfter != "" {
		t.Fatalf("r should resume without connecting, after=%q", m.sandboxConnectAfter)
	}
	if m.syncActivity != "resuming…" {
		t.Fatalf("resume activity = %q", m.syncActivity)
	}

	m.form = nil
	m.syncingProviders = map[string]bool{}
	m.syncActivity = ""
	m.section = syncSection
	m.syncStatus.Boat.Authenticated = true
	m.syncProvider = ""
	for i, d := range cloud.Descriptors() {
		if d.Kind == cloud.Boat {
			m.syncCursor = i
			break
		}
	}
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "boat_new" {
		t.Fatalf("n on Boat in the sync list should create a sandbox, got %#v", m.form)
	}

	m.form = nil
	m.section = hostsSection
	m.syncingProviders = map[string]bool{}
	m.syncActivity = ""
	m.hosts = []sshconfig.Host{{
		Alias: "upstash_dev", Synced: true, SyncSource: "upstash", SyncID: "current-wasp-05510",
		Resolved: sshconfig.Resolved{HostName: "us-east-1.box.upstash.com", User: "root"},
	}}
	if err := m.metadata.SetHost("upstash_dev", metadata.Host{Label: "dev", Group: "Upstash", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	selectHostAlias(t, m, "upstash_dev")
	_, _ = m.updateKeys(press("o"))
	if m.form == nil || m.form.action != "upstash_stop" {
		t.Fatalf("o on upstash host should pause, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "upstash_fork" {
		t.Fatalf("n on upstash host should fork, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("d"))
	if m.form == nil || m.form.action != "upstash_delete" {
		t.Fatalf("d on upstash host should delete, got %#v", m.form)
	}

	m.form = nil
	m.section = syncSection
	m.syncProvider = ""
	m.syncStatus.Upstash.HasKey = true
	for i, d := range cloud.Descriptors() {
		if d.Kind == cloud.Upstash {
			m.syncCursor = i
			break
		}
	}
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "upstash_new" {
		t.Fatalf("n on Upstash in the sync list should create a box, got %#v", m.form)
	}

	m.form = nil
	m.section = hostsSection
	m.syncingProviders = map[string]bool{}
	m.syncActivity = ""
	if err := m.metadata.SetVercel(metadata.VercelIntegration{TeamID: "team_1", ProjectID: "prj_1"}); err != nil {
		t.Fatal(err)
	}
	m.hosts = []sshconfig.Host{{
		Alias: "vercel_dev", Synced: true, SyncSource: "vercel", SyncID: "prj_1/dev",
		Resolved: sshconfig.Resolved{HostName: "vercel.sandbox.invalid"},
	}}
	if err := m.metadata.SetHost("vercel_dev", metadata.Host{Label: "dev", Group: "Vercel", Tags: []string{"state:running"}}); err != nil {
		t.Fatal(err)
	}
	selectHostAlias(t, m, "vercel_dev")
	_, _ = m.updateKeys(press("o"))
	if m.form == nil || m.form.action != "vercel_stop" {
		t.Fatalf("o on vercel host should stop, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("n"))
	if m.form == nil || m.form.action != "vercel_fork" {
		t.Fatalf("n on vercel host should fork, got %#v", m.form)
	}
	m.form = nil
	_, _ = m.updateKeys(press("d"))
	if m.form == nil || m.form.action != "vercel_delete" {
		t.Fatalf("d on vercel host should delete, got %#v", m.form)
	}
}

func TestProviderInstancesGroup(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "gcp"
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
	if strings.Contains(body, "\nweb") || strings.Contains(body, " web") || strings.Contains(body, "\napi") || strings.Contains(body, " api") {
		t.Fatalf("sync settings should not list instances:\n%s", body)
	}
	m.section = hostsSection
	m.collapsedGroups = map[string]bool{}
	var sawWeb, sawAPI bool
	for _, row := range m.hostRows() {
		switch row.host.Alias {
		case "gcp_web":
			sawWeb = true
		case "gcp_api":
			sawAPI = true
		}
	}
	if !sawWeb || !sawAPI {
		t.Fatalf("hosts should list both instances, web=%v api=%v", sawWeb, sawAPI)
	}
}

func TestProviderNavChipThenInventory(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "boat"
	m.syncCursor = 0
	m.syncStatus.Boat.Authenticated = true
	m.syncStatus.Boat.Login = "hi@ted.ac"
	m.updateSyncKeys("j")
	rows := m.syncSettingRows(cloud.Boat)
	if rowHas(rows, "Login") || m.syncCursor != 1 || len(rows) < 2 || rows[0].id != "enabled" || rows[1].id != "autosync" {
		t.Fatalf("j should move to Auto-sync, cursor=%d rows=%+v", m.syncCursor, rows)
	}
	var create int
	for i, row := range rows {
		if row.id == "create" {
			create = i
		}
	}
	if create == 0 {
		t.Fatalf("expected New sandbox row: %+v", rows)
	}
	m.syncCursor = create
	m.updateSyncKeys("l")
	if m.form == nil || m.form.action != "boat_new" {
		t.Fatalf("l on New sandbox should open the form, got %#v", m.form)
	}
}

func TestProviderPageMouse(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	m.section = syncSection
	m.syncProvider = "gcp"
	m.syncCursor = -1
	x, y := syncDetailClick(t, m, cloud.GCP, "setting:"+cloud.KeySSHUser)
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	rows := m.syncSettingRows(cloud.GCP)
	want := -1
	for i, row := range rows {
		if row.id == "setting:"+cloud.KeySSHUser {
			want = i
		}
	}
	if m.syncCursor != want {
		t.Fatalf("first click should select SSH user, cursor=%d want=%d", m.syncCursor, want)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	if m.form == nil || m.form.action != "provider_setting:gcp:ssh_user" {
		t.Fatalf("second click should open SSH user, form=%#v", m.form)
	}
}

func TestBoatLifecycleChipMouse(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	m.section = syncSection
	m.syncProvider = "boat"
	m.syncCursor = -1
	m.syncStatus.Boat.Authenticated = true
	x, y := syncDetailClick(t, m, cloud.Boat, "create")
	rows := m.syncSettingRows(cloud.Boat)
	want := -1
	for i, row := range rows {
		if row.id == "create" {
			want = i
		}
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	if m.syncCursor != want {
		t.Fatalf("first click should select New sandbox, cursor=%d want=%d", m.syncCursor, want)
	}
	x, y = syncDetailClick(t, m, cloud.Boat, "create")
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	if m.form == nil || m.form.action != "boat_new" {
		t.Fatalf("second click should open New sandbox, form=%#v", m.form)
	}
}

func TestProviderInventoryMouseToggle(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	m.section = syncSection
	m.syncCursor = 0
	m.hosts = []sshconfig.Host{
		{Alias: "boat_stop", Synced: true, SyncSource: "boat", SyncID: "bx_stop01", Resolved: sshconfig.Resolved{HostName: "boat.stopped.invalid", User: "user"}},
	}
	if err := m.metadata.SetHost("boat_stop", metadata.Host{Label: "idle-boat", Group: "Boat", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.renderSync(m.styles()), "idle-boat") {
		t.Fatal("sync settings should not list hosts")
	}
	layout := m.panelLayout()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: layout.listTop + 1, Button: tea.MouseLeft}))
	if m.syncCursor != 1 || m.syncProvider != "" {
		t.Fatalf("first click should select the second provider, cursor=%d provider=%q", m.syncCursor, m.syncProvider)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 4, Y: layout.listTop + 1, Button: tea.MouseLeft}))
	if m.syncProvider != string(cloud.Descriptors()[1].Kind) {
		t.Fatalf("second click should open settings, provider=%q", m.syncProvider)
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
	m.syncingProviders = map[string]bool{"gcp": true}
	for i, row := range m.syncSettingRows(cloud.GCP) {
		if row.id == "sync" {
			m.syncCursor = i
			break
		}
	}

	_, cmd := m.updateSyncKeys("enter")
	if cmd != nil {
		t.Fatal("sync action should be disabled while a sync is running")
	}
}

func TestSyncActionIgnoredWhileOtherProviderSyncing(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncProvider = "aws"
	m.syncingProviders = map[string]bool{"gcp": true}
	for i, row := range m.syncSettingRows(cloud.AWS) {
		if row.id == "sync" {
			m.syncCursor = i
			break
		}
	}

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
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Disabled: true}); err != nil {
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

func TestBoatAutoConnectSkipsExplicitAutoSyncOff(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Enabled: true, AutoSync: false}); err != nil {
		t.Fatal(err)
	}
	if cmd := m.autoSyncCmds(); cmd != nil {
		t.Fatal("enabled Boat with auto-sync off should not auto-connect or sync")
	}
	if m.syncingProviders["boat"] {
		t.Fatal("Boat should not be marked syncing")
	}
}

func TestBoatAutoConnectRunsWhenNotEnabled(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetBoat(metadata.BoatIntegration{}); err != nil {
		t.Fatal(err)
	}
	if cmd := m.autoSyncCmds(); cmd == nil {
		t.Fatal("unconfigured Boat should schedule auto-connect")
	}
	if !m.syncingProviders["boat"] {
		t.Fatal("Boat should be marked syncing for auto-connect")
	}
}

func TestStaleBoatSyncDoneDoesNotClearNewerOp(t *testing.T) {
	m := testApp(t)
	old := m.beginProviderOp("boat")
	m.syncActivity = "stopping…"
	newer := m.beginProviderOp("boat")
	if newer == old {
		t.Fatal("expected a new op generation")
	}
	m.Update(syncDoneMsg{provider: "boat", opGen: old})
	if !m.syncingProviders["boat"] {
		t.Fatal("stale completion cleared an in-flight boat op")
	}
	if m.syncActivity != "stopping…" {
		t.Fatalf("stale completion cleared activity: %q", m.syncActivity)
	}
	m.Update(syncDoneMsg{provider: "boat", opGen: newer})
	if m.syncingProviders["boat"] {
		t.Fatal("current completion should clear the sandbox op")
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
	if got := prepareTimeoutForHost(sshconfig.Host{Synced: true, SyncSource: "boat"}); got != boatPrepareTimeout {
		t.Fatalf("boat timeout = %v, want %v", got, boatPrepareTimeout)
	}
	if got := prepareTimeoutForHost(sshconfig.Host{Synced: true, SyncSource: "gcp"}); got != cloudPrepareTimeout {
		t.Fatalf("gcp timeout = %v, want %v", got, cloudPrepareTimeout)
	}
	if got := prepareTimeoutForHost(sshconfig.Host{}); got != cloudPrepareTimeout {
		t.Fatalf("local timeout = %v, want %v", got, cloudPrepareTimeout)
	}
	if boatPrepareTimeout <= 3*time.Minute {
		t.Fatalf("boat prepare timeout %v must exceed Resume WaitReady (3m)", boatPrepareTimeout)
	}
}

func TestFavoriteAndHiddenAllowedForNonBoatSyncedHosts(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{
		Alias: "gcp_demo_web", Synced: true, SyncSource: "gcp",
		SyncID:   "projects/demo/zones/us-central1-a/instances/web",
		Resolved: sshconfig.Resolved{HostName: "web", User: "ubuntu"},
	}, {
		Alias: "boat_sunny", Synced: true, SyncSource: "boat", SyncID: "bx_sunny01",
		Resolved: sshconfig.Resolved{HostName: "1.2.3.4", User: "user"},
	}}
	_ = m.metadata.SetHost("gcp_demo_web", metadata.Host{Label: "web"})
	_ = m.metadata.SetHost("boat_sunny", metadata.Host{Label: "sunny"})

	selectHostAlias(t, m, "gcp_demo_web")
	m.updateKeys(press("f"))
	if !m.metadata.Host("gcp_demo_web").Favorite {
		t.Fatal("GCP synced host should allow favorite toggle")
	}
	m.updateKeys(press("h"))
	if !m.metadata.Host("gcp_demo_web").Hidden {
		t.Fatal("GCP synced host should allow hidden toggle")
	}

	selectHostAlias(t, m, "boat_sunny")
	m.status = ""
	m.updateKeys(press("f"))
	if m.metadata.Host("boat_sunny").Favorite {
		t.Fatal("Boat host should not toggle favorite")
	}
	if strings.Contains(sheet(m), "favorite") {
		t.Fatal("boat sheet should not list favorite")
	}
	m.status = ""
	m.updateKeys(press("h"))
	if m.metadata.Host("boat_sunny").Hidden {
		t.Fatal("Boat host should not toggle hidden")
	}
	if strings.Contains(sheet(m), "h  hide") {
		t.Fatal("boat sheet should not list hide")
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
	m.hosts = []sshconfig.Host{
		{
			Alias: "boat_live", Synced: true, SyncSource: "boat",
			Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"},
		},
	}
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("boat_live", metadata.Host{Label: "live", Group: "Boat", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}
	rows := m.hostRows()
	if len(rows) == 0 || !rows[0].header || rows[0].group != "Boat" {
		t.Fatalf("expected Boat group, rows=%+v", rows)
	}
	m.cursor = 0
	detail := m.renderGroupDetail(m.styles(), rows[0], 60)
	if !strings.Contains(detail, "New sandbox") {
		t.Fatalf("Boat group should offer New sandbox:\n%s", detail)
	}
	lines := strings.Split(detail, "\n")
	if len(lines) < 3 || !strings.Contains(lines[0], "Boat") || strings.Contains(lines[0], "New sandbox") {
		t.Fatalf("New sandbox should sit under the Boat title:\n%s", detail)
	}
	if !strings.Contains(lines[2], "New sandbox") {
		t.Fatalf("New sandbox chip should be on the action row:\n%s", detail)
	}
	_, cmd := m.updateKeys(press("n"))
	_ = cmd
	if m.form == nil || m.form.action != "boat_new" {
		t.Fatalf("n on Boat group should open new form, got %#v", m.form)
	}
	if len(m.form.fields) < 3 || len(m.form.fields[0].options) != 3 || m.form.fields[0].selected != 1 {
		t.Fatalf("new sandbox form should offer constrained type options, got %#v", m.form.fields)
	}
}

func TestProviderGroupHiddenWhenNoActiveHosts(t *testing.T) {
	m := testApp(t)
	m.hosts = nil
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetVercel(metadata.VercelIntegration{Enabled: true, TeamID: "team_1", ProjectID: "prj_1"}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}
	if rows := m.hostRows(); len(rows) != 0 {
		t.Fatalf("empty enabled providers should not inject groups, rows=%+v", rows)
	}

	m.hosts = []sshconfig.Host{
		{
			Alias: "boat_idle", Synced: true, SyncSource: "boat",
			Resolved: sshconfig.Resolved{HostName: "boat.stopped.invalid", User: "user"},
		},
		{
			Alias: "vercel_idle", Synced: true, SyncSource: "vercel",
			Resolved: sshconfig.Resolved{HostName: "vercel.sandbox.invalid"},
		},
		{Alias: "alpha"},
	}
	if err := m.metadata.SetHost("boat_idle", metadata.Host{Label: "idle", Group: "Boat", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("vercel_idle", metadata.Host{Label: "idle", Group: "Vercel", Tags: []string{"state:stopped"}}); err != nil {
		t.Fatal(err)
	}
	groups := map[string]bool{}
	for _, row := range m.hostRows() {
		if row.header {
			groups[row.group] = true
		}
	}
	if groups["Boat"] || groups["Vercel"] {
		t.Fatalf("stopped-only provider groups should stay hidden, groups=%v rows=%+v", groups, m.hostRows())
	}

	m.showHidden = true
	groups = map[string]bool{}
	var sawBoat, sawVercel bool
	for _, row := range m.hostRows() {
		if row.header {
			groups[row.group] = true
		}
		if row.host.Alias == "boat_idle" {
			sawBoat = true
		}
		if row.host.Alias == "vercel_idle" {
			sawVercel = true
		}
	}
	if !groups["Boat"] || !groups["Vercel"] {
		t.Fatalf(". should reveal stopped-only groups, groups=%v", groups)
	}
	if !sawBoat || !sawVercel {
		t.Fatalf(". should reveal stopped hosts, boat=%v vercel=%v", sawBoat, sawVercel)
	}
}

func TestBitqateSyncActionsStayOnBitqate(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetGCP(metadata.GCPIntegration{AutoSync: false, DefaultSSHUser: "gcp-user"}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetAWS(metadata.AWSIntegration{RegionFilter: []string{"us-east-1"}}); err != nil {
		t.Fatal(err)
	}
	m.syncProvider = "bitqate"

	if _, cmd := m.runSyncAction("enable"); cmd != nil || m.form == nil || m.form.action != "bitqate_key" {
		t.Fatalf("enable without a token started a sync, form=%#v cmd=%v", m.form, cmd)
	}
	if m.syncingProviders["bitqate"] {
		t.Fatal("token form left BitQate marked syncing")
	}

	m.form = nil
	m.runSyncAction("auto_on")
	if !m.metadata.Bitqate().AutoSync {
		t.Fatal("auto-sync was not enabled")
	}
	if m.metadata.GCP().AutoSync {
		t.Fatal("BitQate auto-sync enabled GCP auto-sync")
	}

	m.runSyncAction("auto_off")
	if m.metadata.Bitqate().AutoSync {
		t.Fatal("auto-sync stayed enabled")
	}
	if m.metadata.GCP().AutoSync {
		t.Fatal("BitQate auto-sync disabled GCP auto-sync")
	}

	m.runSyncAction("user")
	if m.form == nil || m.form.action != "sync_bitqate_user" {
		t.Fatalf("SSH user form = %#v", m.form)
	}
	m.runSyncAction("regions")
	if m.form == nil || m.form.action != "sync_bitqate_regions" {
		t.Fatalf("region form = %#v", m.form)
	}
	if got := m.metadata.AWS().RegionFilter; len(got) != 1 || got[0] != "us-east-1" {
		t.Fatalf("AWS region filter changed: %v", got)
	}
	if m.metadata.GCP().DefaultSSHUser != "gcp-user" {
		t.Fatalf("GCP SSH user changed: %q", m.metadata.GCP().DefaultSSHUser)
	}
}

func TestLongErrorScrollsAndKeepsActions(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 16
	var body strings.Builder
	body.WriteString("ERROR_START\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, "detail line %02d\n", i)
	}
	body.WriteString("ERROR_END")
	m.showError(body.String())

	view := m.render()
	if h := lipgloss.Height(view); h > m.height {
		t.Fatalf("error view height %d exceeds terminal %d", h, m.height)
	}
	if !strings.Contains(view, "ERROR_START") || strings.Contains(view, "ERROR_END") || strings.Contains(view, "ERROR_START detail") {
		t.Fatalf("expected the start of a long error on its own line:\n%s", view)
	}
	for _, text := range []string{"Action failed", "Enter/Esc return", "↑/↓ scroll"} {
		if !strings.Contains(view, text) {
			t.Fatalf("error overlay missing %q:\n%s", text, view)
		}
	}
	if end, action := errorLine(view, "ERROR_START"), errorLine(view, "Enter/Esc return"); end < 0 || action <= end {
		t.Fatalf("actions should sit under the message: message=%d action=%d", end, action)
	}

	m.Update(press("j"))
	if m.errorOffset != 1 {
		t.Fatalf("j offset = %d", m.errorOffset)
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.errorOffset != 2 {
		t.Fatalf("down offset = %d", m.errorOffset)
	}
	m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	if m.errorOffset != 5 {
		t.Fatalf("wheel offset = %d", m.errorOffset)
	}
	m.form = &form{title: "Add host"}
	m.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	m.form = nil
	if m.errorOffset != 2 {
		t.Fatalf("wheel over a form offset = %d", m.errorOffset)
	}

	page := m.errorPage()
	m.Update(press("g"))
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if m.errorOffset != page {
		t.Fatalf("pgdown offset = %d want %d", m.errorOffset, page)
	}

	m.Update(press("G"))
	if m.errorOffset != m.maxErrorOffset() {
		t.Fatalf("G offset = %d want %d", m.errorOffset, m.maxErrorOffset())
	}
	bottom := m.render()
	if h := lipgloss.Height(bottom); h > m.height {
		t.Fatalf("scrolled error height %d exceeds terminal %d", h, m.height)
	}
	if !strings.Contains(bottom, "ERROR_END") || strings.Contains(bottom, "ERROR_START") {
		t.Fatalf("expected the end of a long error:\n%s", bottom)
	}
	if !strings.Contains(bottom, "Enter/Esc return") {
		t.Fatalf("actions missing after scrolling:\n%s", bottom)
	}
	if end, action := errorLine(bottom, "ERROR_END"), errorLine(bottom, "Enter/Esc return"); end < 0 || action <= end {
		t.Fatalf("actions should stay under the scrolled message: message=%d action=%d", end, action)
	}

	m.Update(press("g"))
	if m.errorOffset != 0 {
		t.Fatalf("g offset = %d", m.errorOffset)
	}
	top := m.render()
	if !strings.Contains(top, "ERROR_START") || strings.Contains(top, "ERROR_END") {
		t.Fatalf("g should return to the start:\n%s", top)
	}

	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.statusError || m.status != "" || m.errorOffset != 0 {
		t.Fatalf("enter should dismiss, status=%q error=%v offset=%d", m.status, m.statusError, m.errorOffset)
	}

	m.showError("failed")
	short := m.render()
	if !strings.Contains(short, "Action failed") || !strings.Contains(short, "Enter/Esc return") {
		t.Fatalf("short error:\n%s", short)
	}
	if strings.Contains(short, "↑/↓ scroll") {
		t.Fatalf("short error offered scrolling:\n%s", short)
	}

	m.errorOffset = 12
	m.setError(errors.New("next failure"))
	if m.errorOffset != 0 || m.status != "next failure" {
		t.Fatalf("new error did not reset scroll: offset=%d status=%q", m.errorOffset, m.status)
	}
}

func errorLine(view, needle string) int {
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}
