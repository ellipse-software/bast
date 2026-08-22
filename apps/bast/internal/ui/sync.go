package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/cloud"
	boxcloud "bast/internal/cloud/box"
	"bast/internal/cloud/sync"
	"bast/internal/sshconfig"
)

type syncMenuItem struct {
	label       string
	detail      string
	description string
	action      string
	provider    string
	disabled    bool
}

const (
	invGroupRunning   = "running"
	invGroupStopped   = "stopped"
	invGroupInstances = "instances"
)

type providerDetail struct {
	enabled           bool
	autoSync          bool
	lastSyncAt        *time.Time
	lastInstanceCount int
	lastSyncError     string
	sshUser           string
	status            []providerRow
	filters           []providerRow
}

type providerRow struct {
	label string
	value string
}

func (m *App) syncProviderTitle() string {
	if m.syncProvider == "" {
		return "Sync"
	}
	if d, ok := cloud.DescriptorForKind(cloud.Kind(m.syncProvider)); ok {
		return d.FullTitle
	}
	return strings.ToUpper(m.syncProvider)
}

func (m *App) syncProviders() []syncMenuItem {
	items := make([]syncMenuItem, 0, len(cloud.Descriptors()))
	for _, d := range cloud.Descriptors() {
		detail := m.providerDetail(string(d.Kind))
		text := "disabled"
		if d.Kind == cloud.Box {
			running, stopped := m.providerGroupStats(d.GroupRoot)
			if !detail.enabled && running+stopped == 0 {
				text = "disabled"
			} else {
				text = fmt.Sprintf("%d running", running)
				if stopped > 0 {
					text += fmt.Sprintf(" · %d stopped", stopped)
				}
			}
		} else if detail.enabled {
			text = fmt.Sprintf("%d instances", detail.lastInstanceCount)
			if detail.lastSyncAt != nil {
				text = fmt.Sprintf("%d · %s", detail.lastInstanceCount, detail.lastSyncAt.Local().Format("2006-01-02 15:04"))
			}
		}
		if detail.lastSyncError != "" {
			text = "error"
		}
		items = append(items, syncMenuItem{
			label: d.GroupRoot, detail: text,
			description: d.Description, provider: string(d.Kind),
		})
	}
	return items
}

func (m *App) providerDetail(provider string) providerDetail {
	switch provider {
	case "gcp":
		integration := m.metadata.GCP()
		status := m.syncStatus.GCP
		accountValue := "none"
		accountLabel := "Accounts"
		if status.GCloudError != "" {
			accountLabel, accountValue = "gcloud", status.GCloudError
		} else if len(status.Accounts) > 0 {
			accountValue = strings.Join(status.Accounts, ", ")
		}
		projects := "all"
		if len(integration.ProjectFilter) > 0 {
			projects = strings.Join(integration.ProjectFilter, ", ")
		}
		filters := []providerRow{{"Projects", projects}}
		if len(integration.ServiceAccounts) > 0 {
			filters = append(filters, providerRow{"SA keys", strings.Join(integration.ServiceAccounts, ", ")})
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, integration.DefaultSSHUser, []providerRow{{accountLabel, accountValue}}, filters}
	case "aws":
		integration := m.metadata.AWS()
		status := m.syncStatus.AWS
		profileValue := "none"
		profileLabel := "Profiles"
		if status.AWSCLIError != "" {
			profileLabel, profileValue = "aws", status.AWSCLIError
		} else if len(status.Profiles) > 0 {
			profileValue = strings.Join(status.Profiles, ", ")
		}
		profiles := "all"
		if len(integration.ProfileFilter) > 0 {
			profiles = strings.Join(integration.ProfileFilter, ", ")
		}
		regions := "all enabled"
		if len(integration.RegionFilter) > 0 {
			regions = strings.Join(integration.RegionFilter, ", ")
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, integration.DefaultSSHUser, []providerRow{{profileLabel, profileValue}}, []providerRow{{"Profile filter", profiles}, {"Regions", regions}}}
	case "azure":
		integration := m.metadata.Azure()
		status := m.syncStatus.Azure
		subscriptionValue := "none"
		subscriptionLabel := "Subscriptions"
		if status.AzureCLIError != "" {
			subscriptionLabel, subscriptionValue = "az", status.AzureCLIError
		} else if len(status.Subscriptions) > 0 {
			subscriptionValue = strings.Join(status.Subscriptions, ", ")
		}
		statusRows := []providerRow{{subscriptionLabel, subscriptionValue}}
		if status.SSHExtensionError != "" {
			statusRows = append(statusRows, providerRow{"ssh extension", status.SSHExtensionError})
		}
		if status.BastionExtensionError != "" {
			statusRows = append(statusRows, providerRow{"bastion extension", status.BastionExtensionError})
		}
		subscriptions := "all enabled"
		if len(integration.SubscriptionFilter) > 0 {
			subscriptions = strings.Join(integration.SubscriptionFilter, ", ")
		}
		resourceGroups := "all"
		if len(integration.ResourceGroupFilter) > 0 {
			resourceGroups = strings.Join(integration.ResourceGroupFilter, ", ")
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, integration.DefaultSSHUser, statusRows, []providerRow{{"Subscription filter", subscriptions}, {"Resource groups", resourceGroups}}}
	case "box":
		integration := m.metadata.Box()
		status := m.syncStatus.Box
		accountLabel, accountValue := "Account", "not logged in"
		if status.BoxCLIError != "" {
			accountLabel, accountValue = "box", status.BoxCLIError
		} else if status.Authenticated {
			accountValue = status.Login
			if accountValue == "" {
				accountValue = "authenticated"
			}
		} else if integration.Disabled {
			accountValue = "disabled"
		}
		statusRows := []providerRow{{accountLabel, accountValue}}
		if status.Plan != "" {
			statusRows = append(statusRows, providerRow{"Plan", status.Plan})
		}
		if integration.Disabled {
			statusRows = append(statusRows, providerRow{"Opt-out", "sticky disable (no auto-connect)"})
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, "", statusRows, nil}
	default:
		return providerDetail{}
	}
}

func (m *App) providerActionLayout() (life, config []syncMenuItem) {
	provider := m.syncProvider
	detail := m.providerDetail(provider)
	if detail.enabled {
		life = append(life, syncMenuItem{label: "Sync", action: "sync"})
	} else {
		life = append(life, syncMenuItem{label: "Connect", action: "enable"})
	}
	caps := cloud.CapabilitiesFor(cloud.Kind(provider))
	if caps.Create && provider == "box" && m.syncStatus.Box.Authenticated {
		life = append(life, syncMenuItem{label: "New box", action: "box_new"})
	}
	if detail.enabled {
		config = append(config, syncMenuItem{label: "Disconnect", action: "disable"})
	}
	if detail.autoSync {
		config = append(config, syncMenuItem{label: "Disable auto-sync", action: "auto_off"})
	} else {
		config = append(config, syncMenuItem{label: "Enable auto-sync", action: "auto_on"})
	}
	switch provider {
	case "gcp":
		config = append(config,
			syncMenuItem{label: "Default SSH user", action: "user"},
			syncMenuItem{label: "Project filter", action: "projects"},
			syncMenuItem{label: "Add service account key", action: "sa_add"},
		)
		if len(m.metadata.GCP().ServiceAccounts) > 0 {
			config = append(config, syncMenuItem{label: "Remove service account key", action: "sa_remove"})
		}
	case "aws":
		config = append(config,
			syncMenuItem{label: "Default SSH user", action: "user"},
			syncMenuItem{label: "Profile filter", action: "profiles"},
			syncMenuItem{label: "Region filter", action: "regions"},
		)
	case "azure":
		config = append(config,
			syncMenuItem{label: "Default SSH user", action: "user"},
			syncMenuItem{label: "Subscription filter", action: "subscriptions"},
			syncMenuItem{label: "Resource group filter", action: "resource_groups"},
		)
	}
	config = append(config, syncMenuItem{label: "Refresh status", action: "refresh"})
	return life, config
}

func (m *App) syncMenuItems() []syncMenuItem {
	if m.syncProvider == "" {
		return m.syncProviders()
	}
	life, config := m.providerActionLayout()
	return append(append([]syncMenuItem{}, life...), config...)
}

func (m *App) syncGCPCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncGCP(ctx)
		return syncDoneMsg{provider: "gcp", result: result, err: err}
	}
}

func (m *App) syncAWSCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncAWS(ctx)
		return syncDoneMsg{provider: "aws", result: result, err: err}
	}
}

func (m *App) syncAzureCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncAzure(ctx)
		return syncDoneMsg{provider: "azure", result: result, err: err}
	}
}

func (m *App) syncBoxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncBox(ctx)
		return syncDoneMsg{provider: "box", result: result, err: err}
	}
}

func (m *App) autoConnectBoxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, ran, err := m.syncer.MaybeAutoConnectBox(ctx)
		if !ran {
			return syncDoneMsg{provider: "box", result: result, err: nil, skipped: true}
		}
		return syncDoneMsg{provider: "box", result: result, err: err}
	}
}

func (m *App) syncStatusCmd() tea.Cmd {
	if m.syncStatusProbing {
		return nil
	}
	m.syncStatusProbing = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		status, err := m.syncer.Status(ctx)
		return syncStatusMsg{status: status, err: err}
	}
}

// enterSyncSection switches to Sync. Cloud CLI probes are reused for a short TTL
// so tab switches stay instant instead of shelling out on every visit.
func (m *App) enterSyncSection() tea.Cmd {
	m.clearFilesOverlays()
	m.section, m.syncProvider, m.syncCursor, m.search = syncSection, "", 0, ""
	if m.syncStatusProbing {
		return nil
	}
	if !m.syncStatusAt.IsZero() && time.Since(m.syncStatusAt) < 30*time.Second {
		return nil
	}
	return m.syncStatusCmd()
}

func (m *App) disableGCPCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.syncer.DisableGCP(ctx)
		if err != nil {
			return syncDoneMsg{provider: "gcp", err: err}
		}
		return syncDoneMsg{provider: "gcp", result: sync.Result{Provider: "gcp", Count: 0, SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) disableAWSCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.syncer.DisableAWS(ctx)
		if err != nil {
			return syncDoneMsg{provider: "aws", err: err}
		}
		return syncDoneMsg{provider: "aws", result: sync.Result{Provider: "aws", SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) disableAzureCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.syncer.DisableAzure(ctx)
		if err != nil {
			return syncDoneMsg{provider: "azure", err: err}
		}
		return syncDoneMsg{provider: "azure", result: sync.Result{Provider: "azure", SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) disableBoxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.syncer.DisableBox(ctx); err != nil {
			return syncDoneMsg{provider: "box", err: err}
		}
		return syncDoneMsg{provider: "box", result: sync.Result{Provider: "box", SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) syncGridCols() int {
	if m.isMobileLayout() {
		return 1
	}
	return 2
}

func (m *App) syncTileWidth() int {
	cols := m.syncGridCols()
	if cols <= 1 {
		return max(18, m.terminalWidth()-4)
	}
	const gap = 2
	return max(18, (m.terminalWidth()-4-gap)/cols)
}

func (m *App) renderSync(s styleSet) string {
	if m.syncProvider != "" {
		return m.renderProviderPage(s)
	}
	return m.renderSyncGrid(s, m.syncMenuItems())
}

const syncTileHeight = 4

func (m *App) renderSyncGrid(s styleSet, items []syncMenuItem) string {
	cols := m.syncGridCols()
	gap := 2
	tileWidth := m.syncTileWidth()
	var b strings.Builder
	b.WriteString("\n")
	for i := 0; i < len(items); i += cols {
		tiles := make([]string, 0, cols)
		for c := 0; c < cols; c++ {
			idx := i + c
			if idx >= len(items) {
				tiles = append(tiles, strings.Repeat(" ", tileWidth))
				continue
			}
			tiles = append(tiles, m.renderSyncTile(s, idx, items[idx], tileWidth))
		}
		row := tiles[0]
		for _, tile := range tiles[1:] {
			row = lipgloss.JoinHorizontal(lipgloss.Top, row, strings.Repeat(" ", gap), tile)
		}
		b.WriteString(indentLines(row, "  ") + "\n")
	}
	if m.syncCursor >= 0 && m.syncCursor < len(items) && items[m.syncCursor].description != "" {
		desc := truncate(items[m.syncCursor].description, max(20, m.terminalWidth()-8))
		b.WriteString("\n    " + s.muted.Render(desc) + "\n")
	}
	return b.String()
}

func (m *App) renderSyncTile(s styleSet, index int, item syncMenuItem, width int) string {
	inner := max(8, width-2)
	content := max(4, inner-2)
	title := m.providerChooserTitle(cloud.Kind(item.provider), content)
	detailText := item.detail
	if lipgloss.Width(detailText) > content {
		detailText = truncate(detailText, content)
	}
	detail := s.muted.Render(detailText)
	border := s.muted
	if index == m.syncCursor {
		border = s.active
	}
	top := border.Render("┌" + strings.Repeat("─", inner) + "┐")
	bot := border.Render("└" + strings.Repeat("─", inner) + "┘")
	side := border.Render("│")
	line := func(contentLine string) string {
		return side + " " + padVisual(contentLine, content) + " " + side
	}
	return top + "\n" + line(title) + "\n" + line(detail) + "\n" + bot
}

func (m *App) providerChooserTitle(kind cloud.Kind, width int) string {
	d, ok := cloud.DescriptorForKind(kind)
	if !ok {
		return truncate(string(kind), width)
	}
	title := renderManagedGroupName(d.GroupRoot, lipgloss.NewStyle(), m.nerdFont)
	if lipgloss.Width(title) <= width {
		return title
	}
	return truncate(d.GroupRoot, width)
}

func indentLines(s, pad string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

func padVisual(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func (m *App) renderSyncMenuLine(s styleSet, index int, item syncMenuItem) string {
	width := min(56, max(24, m.terminalWidth()-4))
	label := item.label
	if item.detail != "" {
		gap := max(1, width-lipgloss.Width(label)-lipgloss.Width(item.detail)-2)
		label = label + strings.Repeat(" ", gap) + item.detail
	}
	line := "  " + label
	var rendered string
	switch {
	case index == m.syncCursor && !item.disabled:
		rendered = s.selected.Width(width + 2).Render(line)
	case item.disabled:
		rendered = s.muted.Width(width + 2).Render(line)
	default:
		rendered = s.value.Width(width + 2).Render(line)
	}
	if index == m.syncCursor && item.description != "" {
		desc := truncate(item.description, max(20, m.terminalWidth()-8))
		rendered += "\n    " + s.muted.Render(desc)
	}
	return rendered
}

func (m *App) renderProviderPage(s styleSet) string {
	kind := cloud.Kind(m.syncProvider)
	d, _ := cloud.DescriptorForKind(kind)
	life, config := m.providerActionLayout()
	inv := m.providerInventoryRows()
	m.clampProviderCursor()

	title := renderManagedGroupName(d.GroupRoot, lipgloss.NewStyle(), m.nerdFont)
	var b strings.Builder
	b.WriteString("  " + title + "\n\n")
	b.WriteString(m.renderProviderIdentity(s, m.syncProvider))
	if len(life) > 0 {
		sel := -1
		if m.syncCursor >= 0 && m.syncCursor < len(life) {
			sel = m.syncCursor
		}
		b.WriteString("\n" + m.renderActionChips(s, life, sel) + "\n")
	}
	if len(inv) > 0 {
		b.WriteString("\n" + m.renderProviderInventory(s, len(life), inv))
	}
	if len(config) > 0 {
		b.WriteString("\n")
		for i, item := range config {
			b.WriteString(m.renderSyncMenuLine(s, len(life)+len(inv)+i, item) + "\n")
		}
	}
	if m.providerSyncing(m.syncProvider) {
		b.WriteString("\n  " + s.muted.Render("Syncing…") + "\n")
	}
	return b.String()
}

func (m *App) renderProviderInventory(s styleSet, lifeCount int, rows []hostRow) string {
	width := min(56, max(24, m.terminalWidth()-4))
	hostMeta := m.hostMetadata()
	var b strings.Builder
	for i, row := range rows {
		index := lifeCount + i
		var line string
		if row.header {
			indicator := "▾"
			if m.providerInvCollapsed(row.group) {
				indicator = "▸"
			}
			count := s.muted.Render(fmt.Sprintf("(%d)", row.count))
			line = "  " + indicator + " " + row.label + " " + count
		} else {
			label := hostLabel(row.host, hostMeta[row.host.Alias])
			line = "    " + truncate(label, max(2, width-4))
		}
		switch {
		case index == m.syncCursor:
			line = s.selected.Width(width + 2).Render(line)
		case !row.header && hostLooksStopped(row.host, hostMeta[row.host.Alias]):
			line = s.muted.Width(width + 2).Render(line)
		case row.header:
			line = s.active.Width(width + 2).Render(line)
		default:
			line = s.value.Width(width + 2).Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m *App) providerInventoryRows() []hostRow {
	if m.syncProvider == "" {
		return nil
	}
	hostMeta := m.hostMetadata()
	var running, stopped []sshconfig.Host
	for _, host := range m.hosts {
		if !host.Synced || host.SyncSource != m.syncProvider {
			continue
		}
		if hostLooksStopped(host, hostMeta[host.Alias]) {
			stopped = append(stopped, host)
		} else {
			running = append(running, host)
		}
	}
	sortInventoryHosts := func(hosts []sshconfig.Host) {
		sort.SliceStable(hosts, func(i, j int) bool {
			a := strings.ToLower(hostLabel(hosts[i], hostMeta[hosts[i].Alias]))
			b := strings.ToLower(hostLabel(hosts[j], hostMeta[hosts[j].Alias]))
			return a < b
		})
	}
	sortInventoryHosts(running)
	sortInventoryHosts(stopped)

	appendGroup := func(rows []hostRow, key, label string, hosts []sshconfig.Host) []hostRow {
		if len(hosts) == 0 {
			return rows
		}
		rows = append(rows, hostRow{group: key, label: label, header: true, count: len(hosts)})
		if m.providerInvCollapsed(key) {
			return rows
		}
		for _, host := range hosts {
			rows = append(rows, hostRow{group: key, host: host, depth: 1})
		}
		return rows
	}

	var rows []hostRow
	if cloud.CapabilitiesFor(cloud.Kind(m.syncProvider)).Stop {
		rows = appendGroup(rows, invGroupRunning, "Running", running)
		rows = appendGroup(rows, invGroupStopped, "Stopped", stopped)
	} else if n := len(running) + len(stopped); n > 0 {
		all := make([]sshconfig.Host, 0, n)
		all = append(all, running...)
		all = append(all, stopped...)
		sortInventoryHosts(all)
		rows = appendGroup(rows, invGroupInstances, "Instances", all)
	}
	return rows
}

func (m *App) invCollapseKey(group string) string {
	if m.syncProvider == "" {
		return group
	}
	return m.syncProvider + "/" + group
}

func (m *App) providerInvCollapsed(key string) bool {
	if m.syncInvCollapsed != nil {
		if collapsed, ok := m.syncInvCollapsed[m.invCollapseKey(key)]; ok {
			return collapsed
		}
	}
	return key == invGroupStopped
}

func (m *App) toggleProviderInv(key string) {
	if m.syncInvCollapsed == nil {
		m.syncInvCollapsed = map[string]bool{}
	}
	collapsed := !m.providerInvCollapsed(key)
	m.syncInvCollapsed[m.invCollapseKey(key)] = collapsed
	if !collapsed {
		return
	}
	life, _ := m.providerActionLayout()
	for i, row := range m.providerInventoryRows() {
		if row.header && row.group == key {
			m.syncCursor = len(life) + i
			return
		}
	}
}

func (m *App) providerNavCounts() (life, inv, config int) {
	l, c := m.providerActionLayout()
	return len(l), len(m.providerInventoryRows()), len(c)
}

func (m *App) renderProviderIdentity(s styleSet, provider string) string {
	detail := m.providerDetail(provider)
	kind := cloud.Kind(provider)
	var b strings.Builder

	state := "disabled"
	stateStyle := s.muted
	if detail.enabled {
		state = "enabled"
		stateStyle = s.success
	}
	facts := make([]string, 0, 4)
	if kind == cloud.Box {
		running, stopped := m.providerGroupStats("Box")
		facts = append(facts, fmt.Sprintf("%d running", running))
		if stopped > 0 {
			facts = append(facts, fmt.Sprintf("%d stopped", stopped))
		}
	} else if detail.enabled || detail.lastInstanceCount > 0 {
		facts = append(facts, fmt.Sprintf("%d instances", detail.lastInstanceCount))
	}
	if detail.lastSyncAt != nil {
		facts = append(facts, detail.lastSyncAt.Local().Format("2006-01-02 15:04"))
	}
	line := stateStyle.Render(state)
	if len(facts) > 0 {
		line += s.muted.Render(" · " + strings.Join(facts, " · "))
	}
	b.WriteString("  " + line + "\n")

	var bits []string
	var errBit string
	for _, row := range detail.status {
		switch row.label {
		case "gcloud", "aws", "az", "box":
			errBit = row.value
		default:
			if row.value != "" && row.value != "none" && row.value != "not logged in" {
				bits = append(bits, row.value)
			} else if row.value == "not logged in" {
				bits = append(bits, "not logged in")
			}
		}
	}
	if detail.enabled {
		if detail.autoSync {
			bits = append(bits, "auto-sync on")
		} else {
			bits = append(bits, "auto-sync off")
		}
	}
	if len(bits) > 0 {
		b.WriteString("  " + s.muted.Render(strings.Join(bits, " · ")) + "\n")
	}
	if errBit != "" {
		b.WriteString("  " + s.error.Render(errBit) + "\n")
	}
	if detail.lastSyncError != "" {
		b.WriteString("  " + s.error.Render(detail.lastSyncError) + "\n")
	}
	return b.String()
}

func (m *App) renderActionChips(s styleSet, items []syncMenuItem, selected int) string {
	parts := make([]string, 0, len(items))
	for i, item := range items {
		label := " " + item.label + " "
		if i == selected {
			parts = append(parts, s.title.Render(label))
		} else {
			parts = append(parts, s.value.Render(label))
		}
	}
	return "  " + strings.Join(parts, "  ")
}

func (m *App) clampProviderCursor() {
	L, I, C := m.providerNavCounts()
	maxIdx := L + I + C - 1
	if maxIdx < 0 {
		m.syncCursor = 0
		return
	}
	if m.syncCursor < 0 {
		m.syncCursor = 0
	}
	if m.syncCursor > maxIdx {
		m.syncCursor = maxIdx
	}
}

func visualLineCount(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

type providerPageHit struct {
	y0, y1 int
	x0, x1 int
	kind   string // chip, inv, config
	index  int
}

func (m *App) actionChipBounds(items []syncMenuItem) [][2]int {
	x := 2
	out := make([][2]int, len(items))
	for i, item := range items {
		w := lipgloss.Width(" " + item.label + " ")
		out[i] = [2]int{x, x + w}
		x += w + 2
	}
	return out
}

func (m *App) providerPageHits() []providerPageHit {
	life, config := m.providerActionLayout()
	inv := m.providerInventoryRows()
	hits := make([]providerPageHit, 0, len(life)+len(inv)+len(config))
	width := max(1, m.terminalWidth())
	y := 2
	y += 2
	y += visualLineCount(m.renderProviderIdentity(m.styles(), m.syncProvider))
	if len(life) > 0 {
		y++
		for i, bounds := range m.actionChipBounds(life) {
			hits = append(hits, providerPageHit{y0: y, y1: y + 1, x0: bounds[0], x1: bounds[1], kind: "chip", index: i})
		}
		y++
	}
	if len(inv) > 0 {
		y++
		for i := range inv {
			hits = append(hits, providerPageHit{y0: y, y1: y + 1, x0: 0, x1: width, kind: "inv", index: i})
			y++
		}
	}
	if len(config) > 0 {
		y++
		for i, item := range config {
			h := 1
			if m.syncCursor == len(life)+len(inv)+i && item.description != "" {
				h = 2
			}
			hits = append(hits, providerPageHit{y0: y, y1: y + h, x0: 0, x1: width, kind: "config", index: i})
			y += h
		}
	}
	return hits
}

func (m *App) openSyncProvider(provider string) tea.Cmd {
	m.syncProvider = provider
	m.syncCursor = 0
	return m.syncStatusCmd()
}

func (m *App) updateSyncKeys(key string) (tea.Model, tea.Cmd) {
	if m.syncBusy != "" {
		return m, nil
	}
	if m.syncProvider != "" {
		return m.updateProviderKeys(key)
	}
	items := m.syncMenuItems()
	m.clampSyncCursor(items)
	switch key {
	case "up", "k":
		m.moveSyncGrid(0, -1, items)
	case "down", "j":
		m.moveSyncGrid(0, 1, items)
	case "left", "h":
		m.moveSyncGrid(-1, 0, items)
	case "right", "l":
		m.moveSyncGrid(1, 0, items)
	case "home", "g":
		m.syncCursor = firstEnabledSyncItem(items)
	case "end", "G":
		m.syncCursor = lastEnabledSyncItem(items)
	case "r":
		return m, m.syncStatusCmd()
	case "s":
		if len(items) == 0 || m.syncCursor < 0 || m.syncCursor >= len(items) {
			return m, nil
		}
		item := items[m.syncCursor]
		if item.disabled || item.provider == "" {
			return m, nil
		}
		previous := m.syncProvider
		m.syncProvider = item.provider
		model, cmd := m.runSyncAction("sync")
		m.syncProvider = previous
		return model, cmd
	case "enter":
		if len(items) == 0 || m.syncCursor < 0 || m.syncCursor >= len(items) {
			return m, nil
		}
		item := items[m.syncCursor]
		if item.disabled {
			return m, nil
		}
		return m, m.openSyncProvider(item.provider)
	}
	return m, nil
}

func (m *App) updateProviderKeys(key string) (tea.Model, tea.Cmd) {
	life, config := m.providerActionLayout()
	inv := m.providerInventoryRows()
	m.clampProviderCursor()
	L, I, C := len(life), len(inv), len(config)
	inv0, config0, end := L, L+I, L+I+C
	switch key {
	case "up", "k":
		switch {
		case m.syncCursor < L:
		case m.syncCursor == inv0:
			if L > 0 {
				m.syncCursor = 0
			}
		case m.syncCursor == config0:
			switch {
			case I > 0:
				m.syncCursor = config0 - 1
			case L > 0:
				m.syncCursor = 0
			}
		default:
			m.syncCursor--
		}
	case "down", "j":
		switch {
		case m.syncCursor < L:
			switch {
			case I > 0:
				m.syncCursor = inv0
			case C > 0:
				m.syncCursor = config0
			}
		case m.syncCursor < end-1:
			m.syncCursor++
		}
	case "left", "h":
		if m.syncCursor < L && m.syncCursor > 0 {
			m.syncCursor--
		}
	case "right", "l":
		if m.syncCursor < L-1 {
			m.syncCursor++
		}
	case "home", "g":
		m.syncCursor = 0
	case "end", "G":
		if end > 0 {
			m.syncCursor = end - 1
		}
	case "esc", "backspace", "ctrl+h":
		kind := m.syncProvider
		m.syncProvider = ""
		m.syncCursor = 0
		for i, item := range m.syncProviders() {
			if item.provider == kind {
				m.syncCursor = i
				break
			}
		}
	case "r":
		return m, m.syncStatusCmd()
	case "s":
		if m.anySyncing() || len(life) == 0 {
			return m, nil
		}
		return m.runSyncAction(life[0].action)
	case "n":
		if m.anySyncing() {
			return m, nil
		}
		for _, item := range life {
			if item.action == "box_new" {
				return m.runSyncAction(item.action)
			}
		}
	case "space":
		if m.syncCursor >= inv0 && m.syncCursor < config0 {
			row := inv[m.syncCursor-inv0]
			if row.header {
				m.toggleProviderInv(row.group)
			}
		}
	case "enter":
		if m.syncCursor < L {
			item := life[m.syncCursor]
			if item.disabled || m.anySyncing() {
				return m, nil
			}
			return m.runSyncAction(item.action)
		}
		if m.syncCursor < config0 {
			return m.activateProviderInventory(inv[m.syncCursor-inv0])
		}
		cfgIdx := m.syncCursor - config0
		if cfgIdx < 0 || cfgIdx >= len(config) {
			return m, nil
		}
		item := config[cfgIdx]
		if item.disabled || (m.anySyncing() && item.action != "refresh") {
			return m, nil
		}
		return m.runSyncAction(item.action)
	}
	return m, nil
}

func (m *App) activateProviderInventory(row hostRow) (tea.Model, tea.Cmd) {
	if row.header {
		m.toggleProviderInv(row.group)
		return m, nil
	}
	if m.anySyncing() {
		return m, m.setNotice("Operation already in progress")
	}
	return m.connectHost(row.host)
}

func (m *App) updateSyncMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if m.syncProvider != "" {
		return m.updateProviderMouse(mouse)
	}
	items := m.syncMenuItems()
	cols := m.syncGridCols()
	gap := 2
	tileWidth := m.syncTileWidth()
	idx := -1
	x := mouse.X - 2
	y := mouse.Y - 3
	if y >= 0 && x >= 0 {
		col := 0
		if cols > 1 && x >= tileWidth+gap {
			col = 1
			x -= tileWidth + gap
		}
		if x < tileWidth {
			idx = (y / syncTileHeight * cols) + col
		}
	}
	if idx < 0 || idx >= len(items) {
		return m, nil
	}
	if idx == m.syncCursor {
		item := items[idx]
		if item.disabled {
			return m, nil
		}
		return m, m.openSyncProvider(item.provider)
	}
	m.syncCursor = idx
	return m, nil
}

func (m *App) updateProviderMouse(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if m.syncBusy != "" {
		return m, nil
	}
	life, config := m.providerActionLayout()
	inv := m.providerInventoryRows()
	L, I := len(life), len(inv)
	hits := m.providerPageHits()
	var hit *providerPageHit
	for i := range hits {
		h := hits[i]
		if mouse.Y < h.y0 || mouse.Y >= h.y1 || mouse.X < h.x0 || mouse.X >= h.x1 {
			continue
		}
		hit = &hits[i]
		break
	}
	if hit == nil {
		return m, nil
	}
	switch hit.kind {
	case "chip":
		if hit.index == m.syncCursor {
			item := life[hit.index]
			if item.disabled || m.anySyncing() {
				return m, nil
			}
			return m.runSyncAction(item.action)
		}
		m.syncCursor = hit.index
		return m, nil
	case "inv":
		cursor := L + hit.index
		row := inv[hit.index]
		if row.header {
			m.syncCursor = cursor
			m.toggleProviderInv(row.group)
			return m, nil
		}
		if cursor == m.syncCursor {
			return m.activateProviderInventory(row)
		}
		m.syncCursor = cursor
		return m, nil
	case "config":
		cursor := L + I + hit.index
		if cursor == m.syncCursor {
			item := config[hit.index]
			if item.disabled || (m.anySyncing() && item.action != "refresh") {
				return m, nil
			}
			return m.runSyncAction(item.action)
		}
		m.syncCursor = cursor
		return m, nil
	}
	return m, nil
}

func (m *App) moveSyncGrid(dx, dy int, items []syncMenuItem) {
	n := len(items)
	if n == 0 {
		return
	}
	cols := m.syncGridCols()
	row := m.syncCursor / cols
	col := m.syncCursor % cols
	rows := (n + cols - 1) / cols
	col += dx
	row += dy
	if col < 0 {
		col = 0
	}
	if col >= cols {
		col = cols - 1
	}
	if row < 0 {
		row = 0
	}
	if row >= rows {
		row = rows - 1
	}
	idx := row*cols + col
	if idx >= n {
		idx = n - 1
	}
	m.syncCursor = idx
}

func (m *App) clampSyncCursor(items []syncMenuItem) {
	if m.syncCursor >= len(items) {
		m.syncCursor = max(0, len(items)-1)
	}
}

func firstEnabledSyncItem(items []syncMenuItem) int {
	for i, item := range items {
		if !item.disabled {
			return i
		}
	}
	return 0
}

func lastEnabledSyncItem(items []syncMenuItem) int {
	for i := len(items) - 1; i >= 0; i-- {
		if !items[i].disabled {
			return i
		}
	}
	return max(0, len(items)-1)
}

func nextEnabledSyncItem(items []syncMenuItem, current int) int {
	for i := current + 1; i < len(items); i++ {
		if !items[i].disabled {
			return i
		}
	}
	return current
}

func prevEnabledSyncItem(items []syncMenuItem, current int) int {
	for i := current - 1; i >= 0; i-- {
		if !items[i].disabled {
			return i
		}
	}
	return current
}

func (m *App) runSyncAction(action string) (tea.Model, tea.Cmd) {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.anySyncing() {
		return m, nil
	}
	switch action {
	case "sync", "enable":
		m.syncingProviders[m.syncProvider] = true
		switch m.syncProvider {
		case "aws":
			return m, tea.Batch(m.syncAWSCmd(), m.setNotice("Syncing AWS…"))
		case "azure":
			return m, tea.Batch(m.syncAzureCmd(), m.setNotice("Syncing Azure…"))
		case "box":
			box := m.metadata.Box()
			box.Disabled = false
			box.Enabled = true
			if action == "enable" {
				box.AutoSync = true
			}
			if err := m.metadata.SetBox(box); err != nil {
				delete(m.syncingProviders, "box")
				m.setError(err)
				return m, nil
			}
			return m, tea.Batch(m.syncBoxCmd(), m.setNotice("Syncing Box…"))
		}
		return m, tea.Batch(m.syncGCPCmd(), m.setNotice("Syncing GCP…"))
	case "disable":
		m.syncingProviders[m.syncProvider] = true
		if m.syncProvider == "aws" {
			return m, m.disableAWSCmd()
		}
		if m.syncProvider == "azure" {
			return m, m.disableAzureCmd()
		}
		if m.syncProvider == "box" {
			return m, m.disableBoxCmd()
		}
		return m, m.disableGCPCmd()
	case "box_new":
		m.openBoxNewForm()
		return m, nil
	case "auto_on":
		if m.syncProvider == "aws" {
			aws := m.metadata.AWS()
			aws.AutoSync = true
			if err := m.metadata.SetAWS(aws); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync enabled")
		}
		if m.syncProvider == "azure" {
			azure := m.metadata.Azure()
			azure.AutoSync = true
			if err := m.metadata.SetAzure(azure); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync enabled")
		}
		if m.syncProvider == "box" {
			box := m.metadata.Box()
			box.AutoSync = true
			box.Disabled = false
			if err := m.metadata.SetBox(box); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync enabled")
		}
		gcp := m.metadata.GCP()
		gcp.AutoSync = true
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return m, nil
		}
		return m, m.setNotice("Auto-sync enabled")
	case "auto_off":
		if m.syncProvider == "aws" {
			aws := m.metadata.AWS()
			aws.AutoSync = false
			if err := m.metadata.SetAWS(aws); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync disabled")
		}
		if m.syncProvider == "azure" {
			azure := m.metadata.Azure()
			azure.AutoSync = false
			if err := m.metadata.SetAzure(azure); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync disabled")
		}
		if m.syncProvider == "box" {
			box := m.metadata.Box()
			box.AutoSync = false
			if err := m.metadata.SetBox(box); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync disabled")
		}
		gcp := m.metadata.GCP()
		gcp.AutoSync = false
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return m, nil
		}
		return m, m.setNotice("Auto-sync disabled")
	case "user":
		if m.syncProvider == "aws" {
			m.openForm("Default AWS SSH user", "sync_aws_user", []field{
				{label: "SSH user", description: "Blank uses the AMI default when known", value: m.metadata.AWS().DefaultSSHUser, optional: true, placeholder: "ec2-user"},
			})
			break
		}
		if m.syncProvider == "azure" {
			m.openForm("Default Azure SSH user", "sync_azure_user", []field{
				{label: "SSH user", description: "Blank uses the VM admin user or Microsoft Entra login", value: m.metadata.Azure().DefaultSSHUser, optional: true, placeholder: "azureuser"},
			})
			break
		}
		if m.syncProvider == "box" {
			return m, m.setNotice("Box SSH user is always user")
		}
		m.openForm("Default GCP SSH user", "sync_gcp_user", []field{
			{label: "SSH user", description: "Blank uses OS Login or instance metadata when available", value: m.metadata.GCP().DefaultSSHUser, optional: true, placeholder: "ubuntu"},
		})
	case "projects":
		m.openForm("GCP project filter", "sync_gcp_projects", []field{
			{label: "Projects", description: "Comma-separated IDs or names; blank = all. Narrowing removes previously synced hosts outside the filter", value: strings.Join(m.metadata.GCP().ProjectFilter, ", "), optional: true, placeholder: "my-prod, my-staging"},
		})
	case "profiles":
		m.openForm("AWS profile filter", "sync_aws_profiles", []field{
			{label: "Profiles", description: "Comma-separated AWS CLI profiles; blank = all configured", value: strings.Join(m.metadata.AWS().ProfileFilter, ", "), optional: true, placeholder: "default, production"},
		})
	case "regions":
		m.openForm("AWS region filter", "sync_aws_regions", []field{
			{label: "Regions", description: "Comma-separated AWS regions; blank = all enabled", value: strings.Join(m.metadata.AWS().RegionFilter, ", "), optional: true, placeholder: "eu-west-1, us-east-1"},
		})
	case "subscriptions":
		m.openForm("Azure subscription filter", "sync_azure_subscriptions", []field{
			{label: "Subscriptions", description: "Comma-separated names or IDs; blank = all enabled", value: strings.Join(m.metadata.Azure().SubscriptionFilter, ", "), optional: true, placeholder: "Production, Staging"},
		})
	case "resource_groups":
		m.openForm("Azure resource group filter", "sync_azure_resource_groups", []field{
			{label: "Resource groups", description: "Comma-separated names; blank = all", value: strings.Join(m.metadata.Azure().ResourceGroupFilter, ", "), optional: true, placeholder: "production, staging"},
		})
	case "sa_add":
		m.openForm("Add service account key", "sync_gcp_sa_add", []field{
			{label: "Key path", description: "Path to a GCP service account JSON key", placeholder: "~/keys/gcp-sa.json"},
		})
	case "sa_remove":
		options := m.metadata.GCP().ServiceAccounts
		if len(options) == 0 {
			return m, m.setNotice("No service account keys configured")
		}
		fieldOptions := make([]fieldOption, 0, len(options))
		for _, path := range options {
			fieldOptions = append(fieldOptions, fieldOption{label: path, value: path})
		}
		m.openForm("Remove service account key", "sync_gcp_sa_remove", []field{
			{label: "Key path", description: "Choose the key to remove", options: fieldOptions},
		})
	case "refresh":
		return m, tea.Batch(m.syncStatusCmd(), m.setNotice("Refreshing sync status…"))
	}
	return m, nil
}

func (m *App) submitSyncForm(action string, values map[string]string) tea.Cmd {
	gcp := m.metadata.GCP()
	switch action {
	case "box_new":
		boxType := strings.ToLower(strings.TrimSpace(values["Type"]))
		if boxType == "" {
			boxType = "default"
		}
		if boxType != "small" && boxType != "default" && boxType != "large" {
			if m.form != nil {
				m.form.validationError = "type must be small, default, or large"
			}
			return nil
		}
		noAutoStop := truthyForm(values["No auto-stop"])
		noEnv := truthyForm(values["No env"])
		m.form = nil
		if m.syncingProviders == nil {
			m.syncingProviders = map[string]bool{}
		}
		m.syncingProviders["box"] = true
		if m.section == syncSection {
			m.beginSyncBusy("Creating box…")
		} else {
			m.syncActivity = "creating…"
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			result, alias, err := m.syncer.NewBox(ctx, boxcloud.NewOpts{
				Type: boxType, NoAutoStop: noAutoStop, NoEnv: noEnv,
			})
			if err != nil {
				return syncDoneMsg{provider: "box", result: result, err: err}
			}
			return syncDoneMsg{provider: "box", result: result, err: nil, focusAlias: alias}
		}
	case "sync_azure_user":
		azure := m.metadata.Azure()
		azure.DefaultSSHUser = strings.TrimSpace(values["SSH user"])
		if err := m.metadata.SetAzure(azure); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Default Azure SSH user updated")
	case "sync_azure_subscriptions":
		azure := m.metadata.Azure()
		azure.SubscriptionFilter = splitCSV(values["Subscriptions"])
		if err := m.metadata.SetAzure(azure); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Azure subscription filter updated")
	case "sync_azure_resource_groups":
		azure := m.metadata.Azure()
		azure.ResourceGroupFilter = splitCSV(values["Resource groups"])
		if err := m.metadata.SetAzure(azure); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Azure resource group filter updated")
	case "sync_aws_user":
		aws := m.metadata.AWS()
		aws.DefaultSSHUser = strings.TrimSpace(values["SSH user"])
		if err := m.metadata.SetAWS(aws); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Default AWS SSH user updated")
	case "sync_aws_profiles":
		aws := m.metadata.AWS()
		aws.ProfileFilter = splitCSV(values["Profiles"])
		if err := m.metadata.SetAWS(aws); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("AWS profile filter updated")
	case "sync_aws_regions":
		aws := m.metadata.AWS()
		aws.RegionFilter = splitCSV(values["Regions"])
		if err := m.metadata.SetAWS(aws); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("AWS region filter updated")
	case "sync_gcp_user":
		gcp.DefaultSSHUser = strings.TrimSpace(values["SSH user"])
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Default GCP SSH user updated")
	case "sync_gcp_projects":
		gcp.ProjectFilter = splitCSV(values["Projects"])
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("GCP project filter updated")
	case "sync_gcp_sa_add":
		path := strings.TrimSpace(values["Key path"])
		if err := sync.ValidateServiceAccountPath(path); err != nil {
			m.setError(err)
			return nil
		}
		for _, existing := range gcp.ServiceAccounts {
			if existing == path {
				m.form = nil
				return m.setNotice("Service account already configured")
			}
		}
		gcp.ServiceAccounts = append(gcp.ServiceAccounts, path)
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Service account key added")
	case "sync_gcp_sa_remove":
		path := strings.TrimSpace(values["Key path"])
		kept := make([]string, 0, len(gcp.ServiceAccounts))
		removed := false
		for _, existing := range gcp.ServiceAccounts {
			if existing == path {
				removed = true
				continue
			}
			kept = append(kept, existing)
		}
		if !removed {
			m.setError(fmt.Errorf("service account key %q not found", path))
			return nil
		}
		gcp.ServiceAccounts = kept
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Service account key removed")
	case "box_stop":
		if strings.TrimSpace(values["Type stop to confirm"]) != "stop" {
			if m.form != nil {
				m.form.validationError = "type stop to confirm"
			}
			return nil
		}
		syncID := strings.TrimSpace(values["SyncID"])
		m.form = nil
		if m.syncingProviders == nil {
			m.syncingProviders = map[string]bool{}
		}
		m.syncingProviders["box"] = true
		m.syncActivity = "stopping…"
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()
			result, err := m.syncer.StopBox(ctx, syncID)
			return syncDoneMsg{provider: "box", result: result, err: err}
		}
	case "box_fork":
		if strings.TrimSpace(values["Type fork to confirm"]) != "fork" {
			if m.form != nil {
				m.form.validationError = "type fork to confirm"
			}
			return nil
		}
		syncID := strings.TrimSpace(values["SyncID"])
		m.form = nil
		if m.syncingProviders == nil {
			m.syncingProviders = map[string]bool{}
		}
		m.syncingProviders["box"] = true
		return tea.Batch(m.setNotice("Forking box…"), func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			result, alias, err := m.syncer.ForkBox(ctx, syncID, boxcloud.ForkOpts{})
			if err != nil {
				return syncDoneMsg{provider: "box", result: result, err: err}
			}
			return syncDoneMsg{provider: "box", result: result, focusAlias: alias}
		})
	}
	return nil
}

func truthyForm(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "y", "yes", "true", "on":
		return true
	default:
		return false
	}
}
