package ui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/cloud"
	"bast/internal/cloud/sync"
)

type providerLoginDoneMsg struct {
	kind string
	err  error
}

type syncSettingRow struct {
	id      string
	label   string
	value   string
	section string
	setting cloud.Setting
}

const (
	syncSectionSettings  = "Settings"
	syncSectionLifecycle = "Lifecycle"
)

func (m *App) providerService(kind cloud.Kind) (sync.Service, bool) {
	if m.syncer == nil {
		return nil, false
	}
	return m.syncer.Service(kind)
}

func (m *App) providerSnapshot(kind cloud.Kind) cloud.Snapshot {
	svc, ok := m.providerService(kind)
	if !ok {
		return cloud.Snapshot{}
	}
	snap := svc.Snapshot(m.syncStatus)
	if kind == cloud.Boat && m.boatMigrationConflict != nil {
		snap.Error = m.boatMigrationConflict.Error()
	}
	return snap
}

func (m *App) syncFocusKind() cloud.Kind {
	if m.syncProvider != "" {
		return cloud.Kind(m.syncProvider)
	}
	items := cloud.Descriptors()
	if len(items) == 0 {
		return ""
	}
	i := m.syncCursor
	if i < 0 {
		i = 0
	}
	if i >= len(items) {
		i = len(items) - 1
	}
	return items[i].Kind
}

func (m *App) syncListCursor() int {
	if m.syncProvider != "" {
		return m.syncListIndex
	}
	if m.syncCursor < 0 {
		return 0
	}
	return m.syncCursor
}

func providerListStatus(snap cloud.Snapshot, syncing bool) (word, count string) {
	if syncing {
		return "syncing", ""
	}
	if shownSyncError(snap, false) != "" {
		return "error", ""
	}
	if !snap.Enabled || cloud.Unset(snap.Error) {
		return "off", ""
	}
	if snap.InstanceCount > 0 {
		return "on", fmt.Sprintf("(%d)", snap.InstanceCount)
	}
	return "on", ""
}

func shownSyncError(snap cloud.Snapshot, syncing bool) string {
	return syncErrorText(snap, syncing)
}

func providerSignedIn(snap cloud.Snapshot) bool {
	if strings.TrimSpace(snap.Account) == "" {
		return false
	}
	return cloud.Fact(snap.Error) != "not logged in"
}

func syncErrorText(snap cloud.Snapshot, syncing bool) string {
	if syncing || !snap.Enabled {
		return ""
	}
	errText := strings.TrimSpace(snap.Error)
	if errText == "" || cloud.Unset(errText) {
		return ""
	}
	return errText
}

func syncStatusStyle(s styleSet, status string) lipgloss.Style {
	switch status {
	case "on":
		return s.success
	case "off", "syncing", "all":
		return s.muted
	case "error":
		return s.error
	default:
		return s.value
	}
}

func (m *App) syncSettingRows(kind cloud.Kind) []syncSettingRow {
	svc, ok := m.providerService(kind)
	if !ok {
		return nil
	}
	snap := m.providerSnapshot(kind)
	auth := svc.Auth()
	var settings, lifecycle []syncSettingRow
	if len(auth.Login) > 0 && !providerSignedIn(snap) {
		settings = append(settings, syncSettingRow{id: "login", section: syncSectionSettings, label: "Login"})
	} else if auth.Kind == cloud.AuthToken {
		settings = append(settings, syncSettingRow{id: "token", section: syncSectionSettings, label: "New token"})
	}
	enabled := "off"
	if snap.Enabled {
		enabled = "on"
	}
	auto := "off"
	if snap.AutoSync {
		auto = "on"
	}
	settings = append(settings,
		syncSettingRow{id: "enabled", section: syncSectionSettings, label: "Enabled", value: enabled},
		syncSettingRow{id: "autosync", section: syncSectionSettings, label: "Auto-sync", value: auto},
	)
	lifecycle = append(lifecycle, syncSettingRow{id: "sync", section: syncSectionLifecycle, label: "Sync now"})
	for _, setting := range snap.Settings {
		row := syncSettingRow{
			id:      "setting:" + setting.Key,
			section: syncSectionSettings,
			label:   setting.Label,
			value:   settingDisplay(setting),
			setting: setting,
		}
		if setting.Key == cloud.KeyCleanup {
			row.section = syncSectionLifecycle
			lifecycle = append(lifecycle, row)
			continue
		}
		settings = append(settings, row)
	}
	if snap.Enabled {
		settings = append(settings, syncSettingRow{id: "disconnect", section: syncSectionSettings, label: "Disconnect"})
	}
	if svc.CanCreate(m.syncStatus) {
		lifecycle = append(lifecycle, syncSettingRow{id: "create", section: syncSectionLifecycle, label: svc.CreateLabel()})
	}
	return append(settings, lifecycle...)
}

func settingDisplay(setting cloud.Setting) string {
	switch setting.Kind {
	case cloud.SettingBool:
		if setting.Value == "true" {
			return "on"
		}
		return "off"
	case cloud.SettingAction:
		return setting.Value
	case cloud.SettingChoice:
		return ""
	case cloud.SettingCSV:
		if strings.TrimSpace(setting.Value) == "" {
			return "all"
		}
		return "on"
	default:
		value := strings.TrimSpace(setting.Value)
		if value == "" {
			return "off"
		}
		if lipgloss.Width(value) <= 12 {
			return value
		}
		return "on"
	}
}

func (m *App) renderSync(s styleSet) string {
	return m.renderSyncPanes(s)
}

func (m *App) renderSyncPanes(s styleSet) string {
	layout := m.panelLayout()
	descriptors := cloud.Descriptors()
	list := m.renderSyncList(s, descriptors, layout.listWidth, layout.listHeight)
	detail := m.renderSyncDetail(s, m.syncFocusKind(), layout.detailWidth, layout.detailHeight)
	listPanel := lipgloss.NewStyle().Width(layout.listWidth).Height(layout.listHeight).Render(list)
	detailPanel := lipgloss.NewStyle().Width(layout.detailWidth).Height(layout.detailHeight).Render(detail)
	if layout.mobile {
		divider := s.rule.Render(strings.Repeat("─", layout.listWidth))
		return lipgloss.JoinVertical(lipgloss.Left, listPanel, divider, detailPanel)
	}
	divider := s.rule.Render(strings.TrimSuffix(strings.Repeat("│\n", layout.bodyHeight), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, divider, detailPanel)
}

func (m *App) renderSyncList(s styleSet, descriptors []cloud.Descriptor, width, height int) string {
	if len(descriptors) == 0 || height <= 0 {
		return ""
	}
	cursor := m.syncListCursor()
	if cursor >= len(descriptors) {
		cursor = len(descriptors) - 1
	}
	start := scrollStart(cursor, len(descriptors), height)
	end := min(len(descriptors), start+height)
	var b strings.Builder
	for i := start; i < end; i++ {
		d := descriptors[i]
		snap := m.providerSnapshot(d.Kind)
		status, count := providerListStatus(snap, m.providerSyncing(string(d.Kind)))
		if i > start {
			b.WriteByte('\n')
		}
		b.WriteString(m.renderSyncListRow(s, d, status, count, width, i == cursor))
	}
	return b.String()
}

func (m *App) renderSyncDetail(s styleSet, kind cloud.Kind, width, height int) string {
	d, ok := cloud.DescriptorForKind(kind)
	if !ok || height <= 0 {
		return ""
	}
	snap := m.providerSnapshot(kind)
	rows := m.syncSettingRows(kind)
	var lines []string
	var rowLine []int
	lines = append(lines, m.renderSyncTitle(d, width))
	if errText := syncErrorText(snap, m.providerSyncing(string(kind))); errText != "" {
		lines = append(lines, "")
		for _, line := range wrapWords(errText, max(4, width-2)) {
			lines = append(lines, "  "+s.error.Render(line))
		}
	}
	if len(rows) > 0 {
		lines = append(lines, "")
	}
	selected := -1
	if m.syncProvider != "" {
		selected = m.syncCursor
	}
	walkSyncRows(rows, func(header string, i int) {
		switch {
		case i >= 0:
			rowLine = append(rowLine, len(lines))
			lines = append(lines, renderSyncSettingLine(s, rows[i], width, i == selected))
		case header != "":
			lines = append(lines, renderSyncSection(s, header, width))
		default:
			lines = append(lines, "")
		}
	})
	focus := 0
	if selected >= 0 && selected < len(rowLine) {
		focus = rowLine[selected]
	}
	start := scrollStart(focus, len(lines), height)
	end := min(len(lines), start+height)
	return strings.Join(lines[start:end], "\n")
}

func (m *App) renderSyncListRow(s styleSet, d cloud.Descriptor, status, count string, width int, selected bool) string {
	rest := lipgloss.Style{}
	if selected {
		rest = s.selected
	}
	plain := syncListStatusText(status, count)
	statusRendered := renderSyncListStatus(s, status, count, selected)
	room := width - lipgloss.Width(plain) - 2
	if room < 1 {
		room = 1
	}
	name := renderManagedGroupName(d.GroupRoot, rest, m.nerdFont, m.windowsTerminal)
	if lipgloss.Width(name) > room {
		icon := managedGroupIcon(d.GroupRoot, m.nerdFont, m.windowsTerminal)
		color, ok := providerBrandColor(d.Kind)
		if !ok {
			color = "#E5E7EB"
		}
		label := truncate(d.GroupRoot, max(1, room-lipgloss.Width(icon)))
		name = brandText(color, icon+label, rest)
	}
	gapN := width - 1 - lipgloss.Width(name) - lipgloss.Width(plain)
	if gapN < 1 {
		gapN = 1
	}
	gap := strings.Repeat(" ", gapN)
	gutter := " "
	if selected {
		bg := lipgloss.NewStyle().Background(s.selected.GetBackground())
		gap = bg.Render(gap)
		gutter = bg.Render(gutter)
	}
	return gutter + name + gap + statusRendered
}

func syncListStatusText(word, count string) string {
	if count == "" {
		return word
	}
	return word + " " + count
}

func renderSyncListStatus(s styleSet, word, count string, selected bool) string {
	style := syncStatusStyle(s, word)
	extra := s.value
	if selected {
		bg := s.selected.GetBackground()
		style = style.Background(bg)
		extra = extra.Background(bg)
	}
	if count == "" {
		return style.Render(word)
	}
	return style.Render(word) + extra.Render(" "+count)
}

func (m *App) renderSyncTitle(d cloud.Descriptor, width int) string {
	room := max(4, width-2)
	name := renderManagedGroupName(d.GroupRoot, lipgloss.Style{}, m.nerdFont, m.windowsTerminal)
	if lipgloss.Width(name) > room {
		icon := managedGroupIcon(d.GroupRoot, m.nerdFont, m.windowsTerminal)
		color, ok := providerBrandColor(d.Kind)
		if !ok {
			color = "#E5E7EB"
		}
		label := truncate(d.GroupRoot, max(1, room-lipgloss.Width(icon)))
		name = brandText(color, icon+label, lipgloss.Style{})
	}
	return "  " + name
}

func walkSyncRows(rows []syncSettingRow, visit func(header string, row int)) {
	prev := ""
	for i, row := range rows {
		if row.section != prev {
			if prev != "" {
				visit("", -1)
			}
			visit(row.section, -1)
			prev = row.section
		}
		visit("", i)
	}
}

func renderSyncSection(s styleSet, title string, width int) string {
	prefix := "  " + title + " "
	room := width - lipgloss.Width(prefix)
	if room < 1 {
		return s.muted.Render(truncate(strings.TrimRight(prefix, " "), max(1, width)))
	}
	return s.muted.Render(prefix + strings.Repeat("─", room))
}

func renderSyncSettingLine(s styleSet, row syncSettingRow, width int, selected bool) string {
	inner := max(1, width-2)
	if selected || row.value == "" {
		text := truncate(row.label, inner)
		if row.value != "" {
			text = fitRow(row.label, row.value, inner)
		}
		text = "  " + text
		if selected {
			return s.selected.Width(width).Render(text)
		}
		return text
	}
	val := syncStatusStyle(s, row.value).Render(row.value)
	room := inner - lipgloss.Width(row.value) - 1
	if room < 1 {
		return "  " + val
	}
	label := truncate(row.label, room)
	gap := inner - lipgloss.Width(label) - lipgloss.Width(row.value)
	if gap < 1 {
		gap = 1
	}
	return "  " + label + strings.Repeat(" ", gap) + val
}

func fitRow(left, right string, width int) string {
	rightW := lipgloss.Width(right)
	if rightW >= width {
		return truncate(right, width)
	}
	room := width - rightW - 1
	if room < 1 {
		return truncate(left, width)
	}
	if lipgloss.Width(left) > room {
		left = truncate(left, room)
	}
	gap := width - lipgloss.Width(left) - rightW
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *App) openSyncProvider(provider string) tea.Cmd {
	m.syncListIndex = m.syncListCursor()
	for i, item := range m.syncProviders() {
		if item.provider == provider {
			m.syncListIndex = i
			break
		}
	}
	m.syncProvider = provider
	m.syncCursor = 0
	return m.syncStatusCmd()
}

func (m *App) closeSyncProvider() {
	kind := m.syncProvider
	m.syncProvider = ""
	m.syncCursor = m.syncListIndex
	for i, item := range m.syncProviders() {
		if item.provider == kind {
			m.syncCursor = i
			break
		}
	}
}

func (m *App) updateSyncKeys(key string) (tea.Model, tea.Cmd) {
	if m.syncBusy != "" {
		return m, nil
	}
	if key == "tab" {
		if m.syncProvider != "" {
			m.closeSyncProvider()
			return m, nil
		}
		items := cloud.Descriptors()
		m.clampSyncCursor(m.syncMenuItems())
		if len(items) == 0 || m.syncCursor < 0 || m.syncCursor >= len(items) {
			return m, nil
		}
		return m, m.openSyncProvider(string(items[m.syncCursor].Kind))
	}
	if m.syncProvider != "" {
		return m.updateSyncDetailKeys(key)
	}
	items := cloud.Descriptors()
	m.clampSyncCursor(m.syncMenuItems())
	switch key {
	case "up", "k":
		if m.syncCursor > 0 {
			m.syncCursor--
		}
	case "down", "j":
		if m.syncCursor+1 < len(items) {
			m.syncCursor++
		}
	case "home", "g":
		m.syncCursor = 0
	case "end", "G":
		if len(items) > 0 {
			m.syncCursor = len(items) - 1
		}
	case "right", "l", "enter":
		if len(items) == 0 || m.syncCursor < 0 || m.syncCursor >= len(items) {
			return m, nil
		}
		return m, m.openSyncProvider(string(items[m.syncCursor].Kind))
	case "r":
		return m, m.syncStatusCmd()
	case "s":
		return m.startProviderSync(m.syncFocusKind())
	case "n":
		return m.openProviderCreate(m.syncFocusKind())
	}
	return m, nil
}

func (m *App) updateSyncDetailKeys(key string) (tea.Model, tea.Cmd) {
	rows := m.syncSettingRows(cloud.Kind(m.syncProvider))
	switch key {
	case "up", "k":
		if m.syncCursor > 0 {
			m.syncCursor--
		}
	case "down", "j":
		if m.syncCursor+1 < len(rows) {
			m.syncCursor++
		}
	case "home", "g":
		m.syncCursor = 0
	case "end", "G":
		if len(rows) > 0 {
			m.syncCursor = len(rows) - 1
		}
	case "left", "h", "esc", "backspace", "ctrl+h":
		m.closeSyncProvider()
	case "r":
		return m, m.syncStatusCmd()
	case "s":
		return m.startProviderSync(cloud.Kind(m.syncProvider))
	case "n":
		return m.openProviderCreate(cloud.Kind(m.syncProvider))
	case "enter", "right", "l":
		if m.syncCursor < 0 || m.syncCursor >= len(rows) {
			return m, nil
		}
		return m.activateSyncRow(cloud.Kind(m.syncProvider), rows[m.syncCursor])
	}
	return m, nil
}

func (m *App) updateSyncMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || m.syncBusy != "" {
		return m, nil
	}
	layout := m.panelLayout()
	inList := mouse.Y >= layout.listTop && mouse.Y < layout.listTop+layout.listHeight &&
		(layout.mobile || mouse.X < layout.listWidth)
	if inList {
		return m.clickSyncList(mouse.Y, layout)
	}
	inDetail := mouse.Y >= layout.detailTop && mouse.Y < layout.detailTop+layout.detailHeight &&
		(layout.mobile || mouse.X > layout.listWidth)
	if inDetail {
		return m.clickSyncDetail(mouse.Y, layout)
	}
	return m, nil
}

func (m *App) clickSyncList(y int, layout panelLayout) (tea.Model, tea.Cmd) {
	descriptors := cloud.Descriptors()
	if len(descriptors) == 0 {
		return m, nil
	}
	cursor := m.syncListCursor()
	start := scrollStart(cursor, len(descriptors), layout.listHeight)
	idx := start + (y - layout.listTop)
	if idx < 0 || idx >= len(descriptors) {
		return m, nil
	}
	kind := string(descriptors[idx].Kind)
	if m.syncProvider != "" {
		m.syncListIndex = idx
		m.syncProvider = kind
		m.syncCursor = 0
		return m, nil
	}
	if idx == m.syncCursor {
		return m, m.openSyncProvider(kind)
	}
	m.syncCursor = idx
	return m, nil
}

func (m *App) clickSyncDetail(y int, layout panelLayout) (tea.Model, tea.Cmd) {
	kind := m.syncFocusKind()
	lines, rowLine := m.syncDetailLines(kind, layout.detailWidth)
	if len(lines) == 0 {
		return m, nil
	}
	focus := 0
	if m.syncProvider != "" && m.syncCursor >= 0 && m.syncCursor < len(rowLine) {
		focus = rowLine[m.syncCursor]
	}
	start := scrollStart(focus, len(lines), layout.detailHeight)
	line := start + (y - layout.detailTop)
	hit := -1
	for i, at := range rowLine {
		if at == line {
			hit = i
			break
		}
	}
	if hit < 0 {
		return m, nil
	}
	if m.syncProvider == "" {
		m.syncListIndex = m.syncListCursor()
		m.syncProvider = string(kind)
		m.syncCursor = hit
		return m, nil
	}
	if hit == m.syncCursor {
		rows := m.syncSettingRows(kind)
		if hit >= 0 && hit < len(rows) {
			return m.activateSyncRow(kind, rows[hit])
		}
		return m, nil
	}
	m.syncCursor = hit
	return m, nil
}

func (m *App) syncDetailLines(kind cloud.Kind, width int) (lines []string, rowLine []int) {
	d, ok := cloud.DescriptorForKind(kind)
	if !ok {
		return nil, nil
	}
	snap := m.providerSnapshot(kind)
	rows := m.syncSettingRows(kind)
	lines = append(lines, "  "+d.GroupRoot)
	if errText := syncErrorText(snap, m.providerSyncing(string(kind))); errText != "" {
		lines = append(lines, "")
		for _, line := range wrapWords(errText, max(4, width-2)) {
			lines = append(lines, "  "+line)
		}
	}
	if len(rows) > 0 {
		lines = append(lines, "")
	}
	walkSyncRows(rows, func(header string, i int) {
		switch {
		case i >= 0:
			rowLine = append(rowLine, len(lines))
			lines = append(lines, rows[i].label)
		case header != "":
			lines = append(lines, header)
		default:
			lines = append(lines, "")
		}
	})
	return lines, rowLine
}

func (m *App) activateSyncRow(kind cloud.Kind, row syncSettingRow) (tea.Model, tea.Cmd) {
	switch row.id {
	case "login":
		return m, m.providerLoginCmd(kind)
	case "token":
		m.openProviderToken(kind)
		return m, nil
	case "enabled":
		snap := m.providerSnapshot(kind)
		if snap.Enabled {
			return m.disableProvider(kind)
		}
		return m.connectProvider(kind)
	case "autosync":
		return m, m.toggleProviderAutoSync(kind)
	case "sync":
		return m.startProviderSync(kind)
	case "disconnect":
		return m.disableProvider(kind)
	case "create":
		return m.openProviderCreate(kind)
	default:
		return m.activateProviderSetting(kind, row.setting)
	}
}

func (m *App) providerBlocked(kind cloud.Kind) bool {
	if kind == cloud.Boat && m.boatMigrationConflict != nil {
		m.setError(m.boatMigrationConflict)
		return true
	}
	return false
}

func (m *App) providerTitle(kind cloud.Kind) string {
	if d, ok := cloud.DescriptorForKind(kind); ok {
		return d.Title
	}
	return string(kind)
}

func (m *App) startProviderSync(kind cloud.Kind) (tea.Model, tea.Cmd) {
	if kind == "" || m.providerBlocked(kind) {
		return m, nil
	}
	if m.anySyncing() {
		return m, nil
	}
	m.beginProviderOp(string(kind))
	return m, tea.Batch(m.providerServiceCmd(kind, "sync"), m.setNotice("Syncing "+m.providerTitle(kind)+"…"))
}

func (m *App) connectProvider(kind cloud.Kind) (tea.Model, tea.Cmd) {
	if m.providerBlocked(kind) {
		return m, nil
	}
	if m.anySyncing() {
		return m, nil
	}
	svc, ok := m.providerService(kind)
	if !ok {
		return m, nil
	}
	if err := svc.SetEnabled(context.Background(), true); err != nil {
		if errors.Is(err, sync.ErrCredentialRequired) {
			m.openProviderToken(kind)
			return m, nil
		}
		m.setError(err)
		return m, nil
	}
	m.beginProviderOp(string(kind))
	return m, tea.Batch(m.providerServiceCmd(kind, "sync"), m.setNotice("Syncing "+m.providerTitle(kind)+"…"))
}

func (m *App) disableProvider(kind cloud.Kind) (tea.Model, tea.Cmd) {
	if m.providerBlocked(kind) {
		return m, nil
	}
	if m.anySyncing() {
		return m, nil
	}
	m.beginProviderOp(string(kind))
	return m, m.providerServiceCmd(kind, "disable")
}

func (m *App) toggleProviderAutoSync(kind cloud.Kind) tea.Cmd {
	if m.providerBlocked(kind) {
		return nil
	}
	svc, ok := m.providerService(kind)
	if !ok {
		return nil
	}
	on := !m.providerSnapshot(kind).AutoSync
	if err := svc.SetAutoSync(context.Background(), on); err != nil {
		m.setError(err)
		return nil
	}
	state := "off"
	if on {
		state = "on"
	}
	return m.setNotice("Auto-sync " + state)
}

func (m *App) providerServiceCmd(kind cloud.Kind, op string) tea.Cmd {
	svc, ok := m.providerService(kind)
	if !ok {
		return nil
	}
	opGen := m.providerOpGen(string(kind))
	timeout := svc.SyncTimeout()
	if op == "disable" {
		timeout = 30 * time.Second
	}
	id := string(kind)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		switch op {
		case "sync":
			result, err := svc.Sync(ctx)
			return syncDoneMsg{provider: id, result: result, err: err, opGen: opGen}
		case "disable":
			if err := svc.SetEnabled(ctx, false); err != nil {
				return syncDoneMsg{provider: id, err: err, opGen: opGen}
			}
			return syncDoneMsg{provider: id, result: sync.Result{Provider: id, SyncedAt: time.Now().UTC(), Error: "disabled"}, opGen: opGen}
		default:
			return syncDoneMsg{provider: id, err: fmt.Errorf("unknown provider op %s", op), opGen: opGen}
		}
	}
}

func (m *App) providerLoginCmd(kind cloud.Kind) tea.Cmd {
	svc, ok := m.providerService(kind)
	if !ok {
		return nil
	}
	login := svc.Auth().Login
	if len(login) == 0 {
		return nil
	}
	cmd := exec.Command(login[0], login[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return providerLoginDoneMsg{kind: string(kind), err: err}
	})
}

func (m *App) openProviderToken(kind cloud.Kind) {
	svc, ok := m.providerService(kind)
	if !ok {
		return
	}
	auth := svc.Auth()
	if auth.TokenURL != "" {
		if err := openBrowser(auth.TokenURL); err != nil {
			m.setError(err)
		}
	}
	label := auth.TokenLabel
	if label == "" {
		label = "Token"
	}
	fields := []field{{
		label:       label,
		description: auth.TokenHint,
		secret:      true,
	}}
	for _, extra := range auth.TokenExtra {
		fields = append(fields, field{
			label:       extra.Label,
			placeholder: extra.Placeholder,
			optional:    extra.Optional,
			secret:      extra.Kind == cloud.SettingSecret,
		})
	}
	m.openForm(m.providerTitle(kind), "provider_token:"+string(kind), fields)
}

func (m *App) submitProviderToken(f *form) tea.Cmd {
	kind := cloud.Kind(strings.TrimPrefix(f.action, "provider_token:"))
	svc, ok := m.providerService(kind)
	if !ok {
		m.setError(fmt.Errorf("unknown provider %s", kind))
		return nil
	}
	auth := svc.Auth()
	values := map[string]string{}
	tokenLabel := auth.TokenLabel
	if tokenLabel == "" {
		tokenLabel = "Token"
	}
	for _, item := range f.fields {
		if item.label == tokenLabel {
			values["token"] = strings.TrimSpace(item.value)
			continue
		}
		for _, extra := range auth.TokenExtra {
			if item.label == extra.Label {
				values[extra.Key] = strings.TrimSpace(item.value)
			}
		}
	}
	if values["token"] == "" {
		f.validationError = tokenLabel + " is required"
		return nil
	}
	if kind == cloud.Vercel {
		if values["team"] == "" {
			f.validationError = "team is required"
			return nil
		}
		if values["project"] == "" && len(m.metadata.Vercel().Projects()) == 0 {
			f.validationError = "project is required"
			return nil
		}
	}
	if m.syncingProviders[string(kind)] {
		return m.setNotice(m.providerTitle(kind) + " operation already in progress")
	}
	m.form = nil
	opGen := m.beginProviderOp(string(kind))
	m.beginSyncBusy("Connecting " + m.providerTitle(kind) + "…")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), svc.SyncTimeout())
		defer cancel()
		result, err := svc.SaveCredential(ctx, values)
		return syncDoneMsg{provider: string(kind), result: result, err: err, opGen: opGen}
	}
}

func (m *App) activateProviderSetting(kind cloud.Kind, setting cloud.Setting) (tea.Model, tea.Cmd) {
	switch setting.Key {
	case cloud.KeyCleanup:
		m.openVercelCleanupForm()
		return m, nil
	case cloud.KeyRemoveToken:
		m.openHetznerKeyRemoveForm()
		return m, nil
	}
	if setting.Kind == cloud.SettingBool {
		next := "true"
		if setting.Value == "true" {
			next = "false"
		}
		return m, m.applyProviderSetting(kind, setting.Key, next, setting.Label)
	}
	if setting.Kind == cloud.SettingChoice {
		if len(setting.Choices) == 0 {
			return m, nil
		}
		options := make([]fieldOption, 0, len(setting.Choices))
		for _, choice := range setting.Choices {
			options = append(options, fieldOption{label: choice, value: choice})
		}
		m.openForm(setting.Label, "provider_setting:"+string(kind)+":"+setting.Key, []field{{
			label:   setting.Label,
			options: options,
		}})
		return m, nil
	}
	secret := setting.Kind == cloud.SettingSecret
	m.openForm(setting.Label, "provider_setting:"+string(kind)+":"+setting.Key, []field{{
		label:       setting.Label,
		value:       setting.Value,
		placeholder: setting.Placeholder,
		description: setting.Hint,
		optional:    setting.Optional,
		secret:      secret,
	}})
	return m, nil
}

func (m *App) applyProviderSetting(kind cloud.Kind, key, value, label string) tea.Cmd {
	svc, ok := m.providerService(kind)
	if !ok {
		return nil
	}
	if err := svc.Apply(context.Background(), key, value); err != nil {
		m.setError(err)
		return nil
	}
	m.form = nil
	if label == "" {
		label = key
	}
	return m.setNotice(label + " updated")
}

func (m *App) submitProviderSetting(f *form) tea.Cmd {
	rest := strings.TrimPrefix(f.action, "provider_setting:")
	kindName, key, ok := strings.Cut(rest, ":")
	if !ok || key == "" {
		m.setError(fmt.Errorf("unknown setting"))
		return nil
	}
	var value, label string
	if len(f.fields) > 0 {
		value = strings.TrimSpace(f.fields[0].value)
		label = f.fields[0].label
	}
	return m.applyProviderSetting(cloud.Kind(kindName), key, value, label)
}

func (m *App) openProviderCreate(kind cloud.Kind) (tea.Model, tea.Cmd) {
	svc, ok := m.providerService(kind)
	if !ok || !svc.CanCreate(m.syncStatus) {
		return m, nil
	}
	if m.syncingProviders[string(kind)] {
		return m, m.setNotice(m.providerTitle(kind) + " operation already in progress")
	}
	switch kind {
	case cloud.Boat:
		m.openBoatNewForm()
	case cloud.Upstash:
		m.openUpstashNewForm()
	case cloud.Vercel:
		m.openVercelNewForm()
	case cloud.Railway:
		m.openRailwayNewForm()
	case cloud.Fly:
		m.openFlyNewForm()
	case cloud.DigitalOcean:
		m.openDigitalOceanNewForm()
	case cloud.Boxd:
		m.openBoxdNewForm()
	}
	return m, nil
}
