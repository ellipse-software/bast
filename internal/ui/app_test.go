package ui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	keymodel "bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
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
func press(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: []rune(text)[0], Text: text})
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

func TestFooterHidesConnectWhenGroupSelected(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = 0

	footer := m.renderFooter(m.styles())
	if strings.Contains(footer, "connect") || strings.Contains(footer, "Connect") {
		t.Fatalf("group footer should not mention connect: %q", footer)
	}
	if !strings.Contains(footer, "rename") {
		t.Fatalf("group footer should mention rename: %q", footer)
	}

	m.cursor = 1
	footer = m.renderFooter(m.styles())
	if !strings.Contains(footer, "connect") && !strings.Contains(footer, "Connect") {
		t.Fatalf("host footer should mention connect: %q", footer)
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
		{group: "one", header: true, depth: 0, count: 1},
		{group: "one/two", header: true, depth: 1, count: 1},
		{group: "one/two/three", header: true, depth: 2, count: 1},
		{group: "one/two/three/four", header: true, depth: 3, count: 1},
		{group: "one/two/three/four/five", header: true, depth: 4, count: 1},
		{group: "one/two/three/four/five", alias: "delta", depth: 5},
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

func TestGroupPathsAreNormalizedAndLimitedToFiveLevels(t *testing.T) {
	m := testApp(t)
	m.openEditHostForm()
	for i := range m.form.fields {
		if m.form.fields[i].label == "Group" {
			m.form.fields[i].value = "one/two/three/four/five/six"
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
		if m.form.fields[i].label == "Group" {
			m.form.fields[i].value = " one / two / three / four / five "
		}
	}
	m.submitForm()
	if m.form != nil || m.statusError {
		t.Fatalf("five-level group was rejected: %q", m.status)
	}
	if got := m.metadata.Host("alpha").Group; got != "one/two/three/four/five" {
		t.Fatalf("normalized group = %q", got)
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
	if !strings.Contains(rendered, "Label") || !strings.Contains(rendered, "Group") || !strings.Contains(rendered, "Tags") || strings.Contains(rendered, "Alias") {
		t.Fatalf("metadata editor was not fully revealed:\n%s", rendered)
	}
	footer := m.renderFooter(m.styles())
	if !strings.Contains(footer, "󰌑 save") || strings.Contains(footer, "󰌑 next") {
		t.Fatalf("metadata editor does not offer immediate save: %q", footer)
	}
}

func TestFooterShowsControlsForTheActiveFormState(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "󰌑 next") || !strings.Contains(footer, "Esc cancel") || strings.Contains(footer, "connect") {
		t.Fatalf("create footer = %q", footer)
	}

	m.openEditHostForm()
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "󰌑 save") || !strings.Contains(footer, "↑/↓ move") || strings.Contains(footer, "connect") {
		t.Fatalf("edit footer = %q", footer)
	}

	m.hosts[0].Managed = true
	m.openEditHostForm()
	for range 4 {
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "choose") || !strings.Contains(footer, "󰌑 select") || !strings.Contains(footer, "Esc close") {
		t.Fatalf("choice footer = %q", footer)
	}
}

func TestEditFormUsesArrowsToMoveAndEnterToSave(t *testing.T) {
	m := testApp(t)
	m.openEditHostForm()
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.form == nil || m.form.fields[m.form.index].label != "Group" {
		t.Fatal("Down did not move to the next edit field")
	}
	m.form.input.SetValue("operations")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.form == nil || m.form.fields[m.form.index].label != "Group" {
		t.Fatal("Up did not return to the previous edit field")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form != nil {
		t.Fatal("Enter did not save and close the edit form")
	}
	if got := m.metadata.Host("alpha").Group; got != "operations" {
		t.Fatalf("saved group = %q", got)
	}
}

func TestEditFormUsesSpaceToChangeAChoice(t *testing.T) {
	m := testApp(t)
	m.hosts = []sshconfig.Host{{Alias: "alpha", Managed: true, ManagedID: "alpha"}}
	m.openEditHostForm()
	for range 4 {
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	if m.form.fields[m.form.index].label != "Identity file" || m.form.selecting {
		t.Fatal("arrow navigation did not focus the identity field")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	if !m.form.selecting {
		t.Fatal("Space did not open the identity choices")
	}
	m.updateForm(press("j"))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form.selecting || m.form.fields[m.form.index].label != "Identity file" {
		t.Fatal("Enter did not confirm the identity choice in place")
	}
	if got := m.form.fields[m.form.index].value; got != passwordOnlyIdentity {
		t.Fatalf("selected identity = %q", got)
	}
}

func TestFormRevealsFieldsProgressivelyAndRevisitsThem(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()
	initial := m.renderForm(m.styles())
	if !strings.Contains(initial, "Label") || strings.Contains(initial, "Hostname") {
		t.Fatalf("initial form exposed future fields:\n%s", initial)
	}
	m.form.input.SetValue("prod")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	second := m.renderForm(m.styles())
	if !strings.Contains(second, "Label  prod") || !strings.Contains(second, "Hostname") || strings.Contains(second, "User") {
		t.Fatalf("Enter did not reveal exactly one new field:\n%s", second)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.form.index != 0 {
		t.Fatalf("up did not revisit the previous field: index=%d", m.form.index)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.form.index != 1 {
		t.Fatalf("down did not return to the revealed field: index=%d", m.form.index)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.form.index != 1 || strings.Contains(m.renderForm(m.styles()), "User") {
		t.Fatal("down revealed a future field without Enter")
	}
}

func TestHostFormExplainsOptionalConnectionFields(t *testing.T) {
	m := testApp(t)
	m.openAddHostForm()

	initial := m.renderForm(m.styles())
	if !strings.Contains(initial, "spaces become SSH underscores") {
		t.Fatalf("label description is missing:\n%s", initial)
	}

	for index := range m.form.fields {
		if m.form.fields[index].label != "Proxy jump" {
			continue
		}
		m.form.index = index
		m.form.revealed = index
		m.focusFormField()
		proxy := m.renderForm(m.styles())
		if !strings.Contains(proxy, "Optional - Route connection through a jump host") {
			t.Fatalf("proxy jump is not clearly explained as optional:\n%s", proxy)
		}
		return
	}
	t.Fatal("host form has no proxy jump field")
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
	if got := m.form.fields[1].value; got != "Production web" {
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
	for range 4 {
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	}
	if m.form.fields[m.form.index].label != "Identity file" || !m.form.selecting {
		t.Fatalf("identity picker was not opened: %+v", m.form)
	}
	view := m.renderForm(m.styles())
	if !strings.Contains(view, "OpenSSH defaults / agent") || !strings.Contains(view, "work · ~/.ssh/bast/keys/work") || !strings.Contains(view, "Manual path…") {
		t.Fatalf("identity picker is missing expected choices:\n%s", view)
	}
	if strings.Contains(view, "agent-only") {
		t.Fatalf("agent-only key was offered as an IdentityFile:\n%s", view)
	}
	if !m.form.selecting {
		t.Fatal("identity picker closed while rendering")
	}

	m.updateForm(press("j"))
	if option := m.form.fields[m.form.index].options[m.form.fields[m.form.index].selected]; option.value != passwordOnlyIdentity {
		t.Fatalf("j did not select password-only authentication: %+v", option)
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form.fields[m.form.index].label != "SSH flags" {
		t.Fatal("password-only selection did not advance to SSH flags")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form.fields[m.form.index].label != "Proxy jump" {
		t.Fatal("SSH flags did not advance to proxy jump")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if m.form.fields[m.form.index].label != "Identity file" {
		t.Fatal("up did not return to the password-only identity choice")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m.updateForm(press("j"))
	if option := m.form.fields[m.form.index].options[m.form.fields[m.form.index].selected]; option.value != "~/.ssh/bast/keys/work" {
		t.Fatalf("second j selected wrong identity option: %+v", option)
	}
	m.updateForm(press("k"))
	if option := m.form.fields[m.form.index].options[m.form.fields[m.form.index].selected]; option.value != passwordOnlyIdentity {
		t.Fatalf("k selected wrong identity option: %+v", option)
	}
	m.updateForm(press("j"))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := m.form.fields[4].value; got != "~/.ssh/bast/keys/work" {
		t.Fatalf("selected identity = %q", got)
	}
	if m.form.fields[m.form.index].label != "SSH flags" {
		t.Fatalf("selecting a key did not advance: index=%d label=%q", m.form.index, m.form.fields[m.form.index].label)
	}

	m = testApp(t)
	manualTestPath := filepath.Join(m.paths.ManagedKeys, "work")
	m.keys = []keymodel.Key{{Name: "work", PrivatePath: manualTestPath}}
	m.openAddHostForm()
	for range 4 {
		m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.form.selecting || m.form.fields[m.form.index].label != "Identity file" {
		t.Fatal("manual choice did not switch the picker to path entry")
	}
	m.form.input.SetValue("~/.ssh/special_key")
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if !m.form.selecting || m.form.fields[4].customValue != "~/.ssh/special_key" {
		t.Fatal("Esc did not return manual path entry to the key choices")
	}
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if got := m.form.fields[4].value; got != "~/.ssh/special_key" {
		t.Fatalf("manual identity = %q", got)
	}

	m = testApp(t)
	editPath := filepath.Join(m.paths.ManagedKeys, "work")
	m.keys = []keymodel.Key{{Name: "work", PrivatePath: editPath}}
	m.hosts = []sshconfig.Host{{Alias: "alpha", Managed: true, ManagedID: "alpha", Resolved: sshconfig.Resolved{IdentityFiles: []string{editPath}}}}
	m.openEditHostForm()
	identity := m.form.fields[5]
	if identity.options[identity.selected].value != "~/.ssh/bast/keys/work" {
		t.Fatalf("existing detected identity was not preselected: %+v", identity)
	}
	if m.form.revealed != len(m.form.fields)-1 || !strings.Contains(m.renderForm(m.styles()), "Notes") {
		t.Fatal("managed host editor did not reveal every field")
	}

	m.hosts[0].Resolved = sshconfig.Resolved{PubkeyAuthentication: "no", PasswordAuthentication: "yes"}
	m.openEditHostForm()
	identity = m.form.fields[5]
	if identity.options[identity.selected].value != passwordOnlyIdentity {
		t.Fatalf("password-only authentication was not preselected: %+v", identity)
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
	if !strings.Contains(connectionBanner, "Press Enter, then ~.") {
		t.Fatal("connection banner does not explain how to force-close a stuck session")
	}
	var output bytes.Buffer
	cmd := exec.Command("/bin/sh", "-c", "printf session-output")
	cmd.Stdout = &output
	process := &connectionProcess{cmd: cmd}
	if err := process.Run(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != connectionBanner+"session-output" {
		t.Fatalf("output = %q", got)
	}
}

func TestSuccessfulSSHSessionExitsBast(t *testing.T) {
	m := testApp(t)
	_, cmd := m.Update(processDoneMsg{name: "SSH session", exitBast: true})
	if cmd == nil {
		t.Fatal("successful SSH session did not request a quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("successful SSH session did not return tea.Quit")
	}

	_, cmd = m.Update(processDoneMsg{name: "SSH session", err: errors.New("connection lost"), exitBast: true})
	if cmd == nil || !m.statusError || !strings.Contains(m.status, "connection lost") {
		t.Fatal("failed SSH session did not return to Bast with its error")
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

func TestErrorsUseAProminentScreenAndPreserveTheForm(t *testing.T) {
	m := testApp(t)
	m.hosts[0].Managed = true
	m.openDeleteHostForm()
	m.form.input.SetValue("wrong-name")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.statusError || m.form == nil {
		t.Fatal("failed deletion did not retain its form and error state")
	}

	rendered := m.render()
	for _, expected := range []string{"Action failed", "What happened", "Delete host — alpha failed", "confirmation did not match", "Your entries are still"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("prominent error screen is missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "Type the name to confirm") {
		t.Fatalf("the form was rendered over the error screen:\n%s", rendered)
	}

	m.Update(press("x"))
	if !m.statusError {
		t.Fatal("an unrelated key dismissed the error screen")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.statusError || m.form == nil {
		t.Fatal("Enter did not return to the retained form")
	}
	if !strings.Contains(m.render(), "Type the name to confirm") {
		t.Fatal("the retained form was not restored")
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
	if cmd == nil || m.status != "Hidden hosts concealed" {
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
	if !strings.Contains(footer, "click Connect") {
		t.Fatalf("mobile footer does not mention connect: %q", footer)
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

func TestDesktopConnectButtonIsVisible(t *testing.T) {
	m := testApp(t)
	m.width, m.height = 80, 24
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
	if !strings.Contains(lines[len(lines)-1], "? help") {
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
		"Created by", "tedbrine",
		"https://bast.sh",
		"github.com/ellipse-software/bast",
		"MIT License",
		"v1.2.3",
		"v / Esc close",
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
	if strings.Contains(host, "Label") || strings.Contains(host, "Color") || strings.Contains(host, "Group") {
		t.Fatalf("host details contain redundant or empty fields:\n%s", host)
	}
	if lipgloss.Height(host) > 8 {
		t.Fatalf("host details are too tall: %d lines\n%s", lipgloss.Height(host), host)
	}
	key := m.renderKeyDetail(m.styles(), keymodel.Key{Name: "work", Algorithm: "ED25519", Fingerprint: "SHA256:test", PrivatePath: "/tmp/work"}, 50)
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
