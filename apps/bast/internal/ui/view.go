package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/cloud"
	"bast/internal/cloud/sync"
	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
)

const (
	headerTitle          = " BAST "
	headerTabSpacing     = "   "
	connectAction        = " Connect "
	resumeAction         = " Resume "
	addAction            = " Add "
	connectActionRow     = 2
	mobileScrollbarWidth = 3

	keyInstallAction    = "[u] Add to server"
	keyInstallActionRow = 4
)

var headerTabLabels = [...]string{"[1] Hosts", "[2] Keys", "[3] Vault", "[4] Sync", "[5] Files"}

var headerTabSections = [...]section{hostsSection, keysSection, vaultSection, syncSection, filesSection}

func (m *App) connectButtonBounds(layout panelLayout) (x, y, width int) {
	action := connectAction
	if host, ok := m.selectedHost(); ok {
		action = m.hostPrimaryAction(host)
	}
	return m.hostActionButtonBounds(layout, action)
}

func (m *App) hostPrimaryAction(host sshconfig.Host) string {
	if m.hostLooksStopped(host) {
		return resumeAction
	}
	return connectAction
}

func (m *App) hostActionButtonBounds(layout panelLayout, action string) (x, y, width int) {
	btn := m.styles().title.Render(action)
	width = lipgloss.Width(btn)
	y = layout.detailTop + connectActionRow
	x = 2
	if !layout.mobile {
		x = layout.listWidth + 1 + 2
	}
	return x, y, width
}

func (m *App) renderDetailActionChip(s styleSet, action string) string {
	return "\n  " + s.title.Render(action) + "\n\n"
}

func (m *App) render() string {
	styles := m.styles()
	width := m.terminalWidth()
	bodyHeight := max(1, m.terminalHeight()-3)
	header := m.renderHeader(styles)
	var body string
	if m.statusError && m.status != "" {
		body = m.renderError(styles)
	} else if m.credits {
		body = m.renderCredits(styles)
	} else if m.help {
		body = m.renderHelp(styles)
	} else if m.form != nil {
		body = m.renderForm(styles)
	} else if m.vaultBusyBlocksBody() {
		body = m.renderVaultBusy(styles)
	} else if m.section == hostsSection {
		body = m.renderHosts(styles)
	} else if m.section == keysSection {
		body = m.renderKeys(styles)
	} else if m.section == vaultSection {
		body = m.renderVault(styles)
	} else if m.section == filesSection {
		body = m.renderFiles(styles)
	} else {
		body = m.renderSync(styles)
	}
	bodyStyle, frameStyle := m.frameStyles(width, bodyHeight)
	body = bodyStyle.Render(body)
	footer := m.renderFooter(styles)
	return frameStyle.Render(header + "\n" + m.renderHeaderRule(styles) + "\n" + body + "\n" + footer)
}

func (m *App) renderHeader(s styleSet) string {
	header := s.title.Render(headerTitle) + "  " + m.renderTabs(s)
	if m.version != "" && m.version != "dev" {
		space := m.terminalWidth() - lipgloss.Width(header) - lipgloss.Width(m.version)
		if space > 0 {
			header += strings.Repeat(" ", space) + s.muted.Render(m.version)
		}
	}
	return header
}

func (m *App) renderError(s styleSet) string {
	contentWidth := max(16, min(66, m.terminalWidth()-10))
	explanation := "The action did not complete."
	if m.form != nil {
		explanation += " Your entries are still available when you return."
	}
	footer := "Enter/Esc return"
	if telemetry.Enabled() {
		footer = "Space send report · Enter/Esc return"
	}
	content := s.error.Bold(true).Render("✕  Action failed") +
		"\n\n" + s.muted.Render("What happened") +
		"\n" + s.value.Width(contentWidth).Render(m.status) +
		"\n\n" + s.muted.Width(contentWidth).Render(explanation) +
		"\n\n" + s.muted.Render(footer)
	panel := lipgloss.NewStyle().
		Width(contentWidth).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#EF4444")).
		Render(content)
	return lipgloss.Place(m.terminalWidth(), max(1, m.terminalHeight()-3), lipgloss.Center, lipgloss.Center, panel)
}

type styleSet struct {
	title, active, inactive, selected, muted, label, value, error, success, rule, plain lipgloss.Style
}

type frameStyleCache struct {
	width      int
	bodyHeight int
	body       lipgloss.Style
	frame      lipgloss.Style
}

var (
	frameStylesCached frameStyleCache
	managedLabelCache struct {
		ready    bool
		nerdFont bool
		google   string
		amazon   string
		azure    string
		box      string
	}
)

func (m *App) styles() styleSet {
	if m.styleCache.ready && m.styleCache.dark == m.dark {
		return m.styleCache.styles
	}
	primary := lipgloss.Color("#8B5CF6")
	text := lipgloss.Color("#E5E7EB")
	muted := lipgloss.Color("#6B7280")
	surface := lipgloss.Color("#1F2937")
	if !m.dark {
		text = lipgloss.Color("#111827")
		muted = lipgloss.Color("#6B7280")
		surface = lipgloss.Color("#EDE9FE")
	}
	styles := styleSet{
		title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(primary),
		active:   lipgloss.NewStyle().Bold(true).Foreground(primary),
		inactive: lipgloss.NewStyle().Foreground(muted),
		selected: lipgloss.NewStyle().Bold(true).Foreground(text).Background(surface),
		muted:    lipgloss.NewStyle().Foreground(muted),
		label:    lipgloss.NewStyle().Foreground(muted).Width(16),
		value:    lipgloss.NewStyle().Foreground(text),
		error:    lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")),
		success:  lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")),
		rule:     lipgloss.NewStyle().Foreground(surface),
		plain:    lipgloss.NewStyle(),
	}
	m.styleCache = styleCache{ready: true, dark: m.dark, styles: styles}
	return styles
}

func (m *App) frameStyles(width, bodyHeight int) (body, frame lipgloss.Style) {
	if frameStylesCached.width == width && frameStylesCached.bodyHeight == bodyHeight {
		return frameStylesCached.body, frameStylesCached.frame
	}
	body = lipgloss.NewStyle().Width(width).Height(bodyHeight)
	frame = lipgloss.NewStyle().Width(width)
	frameStylesCached = frameStyleCache{width: width, bodyHeight: bodyHeight, body: body, frame: frame}
	return body, frame
}

func (m *App) renderTabs(s styleSet) string {
	parts := make([]string, 0, len(headerTabLabels))
	for i, label := range headerTabLabels {
		if m.section == headerTabSections[i] {
			parts = append(parts, s.active.Render(label))
		} else {
			parts = append(parts, s.inactive.Render(label))
		}
	}
	return strings.Join(parts, headerTabSpacing)
}

func tabAtX(x int) (section, bool) {
	cursor := lipgloss.Width(headerTitle) + 2
	gap := lipgloss.Width(headerTabSpacing)
	for i, label := range headerTabLabels {
		width := lipgloss.Width(label)
		if x >= cursor && x < cursor+width {
			return headerTabSections[i], true
		}
		cursor += width + gap
	}
	return 0, false
}

func (m *App) renderHosts(s styleSet) string {
	rowsData := m.hostListRows()
	if len(rowsData) == 0 {
		if m.loading {
			return "\n  " + s.muted.Render("Loading hosts…")
		}
		if m.searchText() != "" {
			return "\n  " + s.muted.Render("No hosts match “"+m.searchText()+"”")
		}
		if m.hasHiddenHosts() && !m.showHidden {
			return "\n  " + s.muted.Render("No visible hosts. Press . to show hidden and stopped hosts.")
		}
		return "\n\n  " + s.active.Render("◇  No hosts yet") +
			"\n\n  " + s.muted.Render("Your SSH map is empty.") +
			"\n  " + s.muted.Render("Press a to add your first destination.")
	}
	listWidth, detailWidth, bodyHeight := m.columnDimensions()
	layout := m.panelLayout()
	listHeight := layout.listHeight
	detailHeight := layout.detailHeight
	rowWidth := listWidth
	if layout.mobile && len(rowsData) > listHeight {
		rowWidth -= mobileScrollbarWidth
	}
	selectedRow := s.selected.Width(rowWidth)
	mutedRow := s.muted.Width(rowWidth)
	activeRow := s.active.Width(rowWidth)
	plainRow := s.plain.Width(rowWidth)
	hostMeta := m.hostMetadata()
	gcpErr := m.metadata.GCP().LastSyncError != "" || m.syncStatus.GCP.GCloudError != ""
	awsErr := m.metadata.AWS().LastSyncError != "" || m.syncStatus.AWS.AWSCLIError != ""
	azureErr := m.metadata.Azure().LastSyncError != "" || m.syncStatus.Azure.AzureCLIError != ""
	boxErr := m.metadata.Box().LastSyncError != "" || m.syncStatus.Box.BoxCLIError != ""
	start := scrollStart(m.cursor, len(rowsData), listHeight)
	var list strings.Builder
	list.Grow(listHeight * (rowWidth + 8))
	for i := start; i < min(len(rowsData), start+listHeight); i++ {
		row := rowsData[i]
		if row.historyHeader {
			indicator := "▾"
			if m.historySuggestionsCollapsed && m.searchText() == "" {
				indicator = "▸"
			}
			line := indicator + " " + row.label + " " + s.muted.Render(fmt.Sprintf("(%d)", row.count))
			if i == m.cursor {
				line = selectedRow.Render(line)
			} else {
				line = mutedRow.Render(line)
			}
			list.WriteString(line + "\n")
			continue
		}
		if row.suggestion != nil {
			indent := strings.Repeat("  ", row.depth)
			line := indent + "＋ " + truncate(row.suggestion.Alias, max(2, rowWidth-lipgloss.Width(indent)-4))
			if i == m.cursor {
				line = selectedRow.Render(line)
			} else {
				line = mutedRow.Render(line)
			}
			list.WriteString(line + "\n")
			continue
		}
		if row.header {
			indent := strings.Repeat("  ", row.depth)
			indicator := "▾"
			if m.collapsedGroups[row.group] && m.searchText() == "" {
				indicator = "▸"
			}
			name := row.label
			if name == "" {
				name = row.group
				if slash := strings.LastIndex(name, "/"); row.depth > 0 && slash >= 0 {
					name = name[slash+1:]
				}
			}
			count := s.muted.Render(fmt.Sprintf("(%d)", row.count))
			errorIcon := ""
			if row.depth == 0 && cloudSyncGroupHasErrorCached(row.group, gcpErr, awsErr, azureErr, boxErr) {
				errorIcon = s.error.Render("⚠")
			}
			prefix := indent + indicator + " "
			reservedWidth := lipgloss.Width(prefix) + 1 + lipgloss.Width(count) + lipgloss.Width(managedGroupIcon(name, m.nerdFont))
			if errorIcon != "" {
				reservedWidth += 1 + lipgloss.Width(errorIcon)
			}
			name = truncate(name, max(2, rowWidth-reservedWidth))
			line := prefix + renderManagedGroupName(name, s.active, m.nerdFont) + " " + count
			if errorIcon != "" {
				line += strings.Repeat(" ", max(1, rowWidth-lipgloss.Width(line)-lipgloss.Width(errorIcon))) + errorIcon
			}
			if i == m.cursor {
				line = selectedRow.Render(line)
			} else {
				line = activeRow.Render(line)
			}
			list.WriteString(line + "\n")
			continue
		}
		host := row.host
		meta := hostMeta[host.Alias]
		prefix := "  "
		if meta.Hidden {
			prefix = "◌ "
		} else if meta.Favorite {
			prefix = "◆ "
		}
		indent := strings.Repeat("  ", row.depth)
		line := indent + prefix + truncate(hostLabel(host, meta), max(2, rowWidth-lipgloss.Width(indent+prefix)-2))
		if i == m.cursor {
			line = selectedRow.Render(line)
		} else if hostLooksStopped(host, meta) {
			line = mutedRow.Render(line)
		} else {
			line = plainRow.Render(line)
		}
		list.WriteString(line + "\n")
	}
	listPanel := lipgloss.NewStyle().Width(rowWidth).Height(listHeight).Render(strings.TrimRight(list.String(), "\n"))
	if layout.mobile && len(rowsData) > listHeight {
		listPanel = lipgloss.JoinHorizontal(lipgloss.Top, listPanel, m.renderMobileScrollbar(s, len(rowsData), listHeight))
	}
	detailContent := ""
	if m.cursor >= 0 && m.cursor < len(rowsData) {
		if rowsData[m.cursor].historyHeader {
			detailContent = m.renderHistorySuggestionsDetail(s, rowsData[m.cursor].count, detailWidth)
		} else if rowsData[m.cursor].suggestion != nil {
			detailContent = m.renderHistorySuggestionDetail(s, *rowsData[m.cursor].suggestion, detailWidth)
		} else if rowsData[m.cursor].header {
			detailContent = m.renderGroupDetail(s, rowsData[m.cursor], detailWidth)
		} else {
			detailContent = m.renderHostDetail(s, rowsData[m.cursor].host, detailWidth)
		}
	}
	detail := lipgloss.NewStyle().Width(detailWidth).Height(detailHeight).Render(detailContent)
	if layout.mobile {
		divider := s.rule.Render(strings.Repeat("─", listWidth))
		return lipgloss.JoinVertical(lipgloss.Left, listPanel, divider, detail)
	}
	divider := s.rule.Render(strings.TrimSuffix(strings.Repeat("│\n", bodyHeight), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, divider, detail)
}

func (m *App) renderHistorySuggestionsDetail(s styleSet, count, width int) string {
	state := "expanded"
	if m.historySuggestionsCollapsed {
		state = "collapsed"
		if m.searchText() != "" {
			state = "expanded for search"
		}
	}
	return "  " + s.muted.Render("(Suggested)") + "\n" +
		"  " + s.muted.Render(fmt.Sprintf("%d hosts · %s", count, state)) + "\n\n" +
		"  " + s.muted.Render(truncate("␣ collapse or expand", max(4, width-3)))
}

func (m *App) renderHistorySuggestionDetail(s styleSet, suggestion metadata.HistorySuggestion, width int) string {
	var b strings.Builder
	title := truncate(suggestion.Alias, max(4, width-3))
	b.WriteString("  " + s.active.Render(title) + "\n")
	b.WriteString(m.renderDetailActionChip(s, addAction))
	destination := suggestion.HostName
	if suggestion.User != "" {
		destination = suggestion.User + "@" + destination
	}
	if suggestion.Port != "" && suggestion.Port != "22" {
		destination += ":" + suggestion.Port
	}
	b.WriteString("  " + s.value.Render(truncate(destination, max(4, width-3))) + "\n")
	b.WriteString("  " + s.muted.Render("From "+suggestion.Source+" history") + "\n\n")
	b.WriteString("  " + s.muted.Render("Access") + "\n")
	identity := suggestion.IdentityFile
	if identity == "" {
		identity = "agent/defaults"
	}
	b.WriteString(compactRow(s, "Auth", identity, width))
	if suggestion.ProxyJump != "" {
		b.WriteString(compactRow(s, "Jump", suggestion.ProxyJump, width))
	}
	return b.String()
}

func (m *App) cloudSyncGroupHasError(group string) bool {
	return cloudSyncGroupHasErrorCached(
		group,
		m.metadata.GCP().LastSyncError != "" || m.syncStatus.GCP.GCloudError != "",
		m.metadata.AWS().LastSyncError != "" || m.syncStatus.AWS.AWSCLIError != "",
		m.metadata.Azure().LastSyncError != "" || m.syncStatus.Azure.AzureCLIError != "",
		m.metadata.Box().LastSyncError != "" || m.syncStatus.Box.BoxCLIError != "",
	)
}

func cloudSyncGroupHasErrorCached(group string, gcpErr, awsErr, azureErr, boxErr bool) bool {
	kind, ok := cloud.KindForGroup(group)
	if !ok {
		return false
	}
	switch kind {
	case cloud.GCP:
		return gcpErr
	case cloud.AWS:
		return awsErr
	case cloud.Azure:
		return azureErr
	case cloud.Box:
		return boxErr
	default:
		return false
	}
}

func (m *App) renderGroupDetail(s styleSet, row hostRow, width int) string {
	if cloud.IsProviderRoot(row.group) {
		if kind, ok := cloud.KindForGroup(row.group); ok {
			return m.renderProviderGroupDetail(s, row, kind, width)
		}
	}
	state := "expanded"
	if m.collapsedGroups[row.group] {
		state = "collapsed"
		if m.searchText() != "" {
			state = "expanded for search"
		}
	}
	hint := "␣ " + m.collapseActionLabel() + " · e rename"
	if sync.IsSyncedGroup(row.group) {
		hint = "␣ " + m.collapseActionLabel() + " · cloud sync (read-only)"
	}
	iconWidth := lipgloss.Width(managedGroupIcon(row.group, m.nerdFont))
	name := truncate(row.group, max(2, width-3-iconWidth))
	return "  " + renderManagedGroupName(name, s.active, m.nerdFont) + "\n" +
		"  " + s.muted.Render(fmt.Sprintf("%d servers · %s", row.count, state)) + "\n\n" +
		"  " + s.muted.Render(truncate(hint, max(4, width-3)))
}

func (m *App) renderProviderGroupDetail(s styleSet, row hostRow, kind cloud.Kind, width int) string {
	var b strings.Builder
	iconWidth := lipgloss.Width(managedGroupIcon(row.group, m.nerdFont))
	name := truncate(row.group, max(2, width-3-iconWidth))
	titlePart := renderManagedGroupName(name, s.active, m.nerdFont)
	b.WriteString("  " + titlePart + "\n")
	b.WriteString(m.renderDetailActionChip(s, m.providerGroupPrimaryAction(kind)))

	running, stopped := m.providerGroupStats(row.group)
	summary := fmt.Sprintf("%d instances", running+stopped)
	if cloud.CapabilitiesFor(kind).Stop {
		summary = fmt.Sprintf("%d running", running)
		if stopped > 0 {
			summary += fmt.Sprintf(" · %d stopped", stopped)
			if !m.showHidden {
				summary += " · . to show"
			}
		}
	} else if row.count >= 0 {
		summary = fmt.Sprintf("%d instances", row.count)
	}
	b.WriteString("  " + s.muted.Render(truncate(summary, max(4, width-3))) + "\n")

	detail := m.providerDetail(string(kind))
	b.WriteString("\n")
	lastSync := "never"
	if detail.lastSyncAt != nil {
		lastSync = detail.lastSyncAt.Local().Format("2006-01-02 15:04")
	}
	b.WriteString(compactRow(s, "Last sync", lastSync, width))
	for _, statusRow := range detail.status {
		b.WriteString(compactRow(s, statusRow.label, statusRow.value, width))
	}
	autoSync := "off"
	if detail.autoSync {
		autoSync = "on"
	}
	b.WriteString(compactRow(s, "Auto-sync", autoSync, width))
	if detail.lastSyncError != "" {
		b.WriteString(compactRow(s, "Error", detail.lastSyncError, width))
	}
	return b.String()
}

func renderManagedGroupName(name string, restStyle lipgloss.Style, nerdFont bool) string {
	labels := managedProviderLabels(nerdFont)
	switch {
	case name == "Amazon EC2" || strings.HasPrefix(name, "Amazon EC2/"):
		return labels.amazon + restStyle.Render(strings.TrimPrefix(name, "Amazon EC2"))
	case name == "Google Cloud" || strings.HasPrefix(name, "Google Cloud/"):
		return labels.google + restStyle.Render(strings.TrimPrefix(name, "Google Cloud"))
	case name == "Microsoft Azure" || strings.HasPrefix(name, "Microsoft Azure/"):
		return labels.azure + restStyle.Render(strings.TrimPrefix(name, "Microsoft Azure"))
	case name == "Box" || strings.HasPrefix(name, "Box/"):
		return labels.box + restStyle.Render(strings.TrimPrefix(name, "Box"))
	default:
		return name
	}
}

func managedProviderLabels(nerdFont bool) (labels struct{ google, amazon, azure, box string }) {
	if managedLabelCache.ready && managedLabelCache.nerdFont == nerdFont {
		return struct{ google, amazon, azure, box string }{
			google: managedLabelCache.google,
			amazon: managedLabelCache.amazon,
			azure:  managedLabelCache.azure,
			box:    managedLabelCache.box,
		}
	}
	amazonName := managedGroupIcon("Amazon EC2", nerdFont) + "Amazon EC2"
	amazon := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF9900")).Render(amazonName)

	colors := []string{"#4285F4", "#EA4335", "#FBBC05", "#4285F4", "#34A853", "#EA4335"}
	var google strings.Builder
	if icon := managedGroupIcon("Google Cloud", nerdFont); icon != "" {
		google.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4285F4")).Render(icon))
	}
	for i, letter := range "Google" {
		google.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors[i])).Render(string(letter)))
	}
	google.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(" Cloud"))

	azureName := managedGroupIcon("Microsoft Azure", nerdFont) + "Microsoft Azure"
	azure := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0078D4")).Render(azureName)

	boxName := managedGroupIcon("Box", nerdFont) + "Box"
	box := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(boxName)

	managedLabelCache.ready = true
	managedLabelCache.nerdFont = nerdFont
	managedLabelCache.google = google.String()
	managedLabelCache.amazon = amazon
	managedLabelCache.azure = azure
	managedLabelCache.box = box
	return struct{ google, amazon, azure, box string }{
		google: managedLabelCache.google,
		amazon: managedLabelCache.amazon,
		azure:  managedLabelCache.azure,
		box:    managedLabelCache.box,
	}
}

func managedGroupIcon(name string, nerdFont bool) string {
	if !nerdFont {
		return ""
	}
	kind, ok := cloud.KindForGroup(name)
	if !ok {
		return ""
	}
	d, ok := cloud.DescriptorForKind(kind)
	if !ok || d.NerdIcon == "" {
		return ""
	}
	return d.NerdIcon + " "
}

func (m *App) renderHostDetail(s styleSet, host sshconfig.Host, width int) string {
	meta := m.hostMetadata()[host.Alias]
	var b strings.Builder
	label := hostLabel(host, meta)
	primaryAction := resumeAction
	if !hostLooksStopped(host, meta) {
		primaryAction = connectAction
	}
	titleStyle := s.active
	titleMax := max(4, width-3)
	title := truncate(label, titleMax)
	if foreground, ok := contrastingTextColor(meta.Color); ok {
		titleStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(foreground)).
			Background(lipgloss.Color(meta.Color)).
			Padding(0, 1)
		titleMax = max(4, width-5)
		title = truncate(label, titleMax)
	}
	dest := destination(host)
	if dest == "" {
		dest = host.Alias
	}
	destStyle := s.value
	if hostLooksStopped(host, meta) {
		dest = "stopped"
		destStyle = s.muted
		if _, ok := contrastingTextColor(meta.Color); !ok {
			titleStyle = s.muted
		}
	}
	titlePart := titleStyle.Render(title)
	b.WriteString("  " + titlePart + "\n")
	b.WriteString(m.renderDetailActionChip(s, primaryAction))
	b.WriteString("  " + destStyle.Render(truncate(dest, max(4, width-3))) + "\n")
	b.WriteString("  " + s.muted.Render(truncate(hostStatusLine(host, meta), max(4, width-3))) + "\n")

	b.WriteString("\n")
	b.WriteString("  " + s.muted.Render("Access") + "\n")
	b.WriteString(compactRow(s, "Auth", hostAuthSummary(host), width))
	if host.Resolved.ProxyJump != "" && host.Resolved.ProxyJump != "none" {
		b.WriteString(compactRow(s, "Jump", host.Resolved.ProxyJump, width))
	}
	if label != host.Alias {
		b.WriteString(compactRow(s, "SSH name", host.Alias, width))
	}
	if !host.Managed && !host.Synced {
		b.WriteString("\n")
		b.WriteString("  " + s.active.Render("[p] Promote to Bast managed") + "\n")
	}

	var about strings.Builder
	if meta.Group != "" {
		about.WriteString(compactRow(s, "Group", meta.Group, width))
	}
	if meta.Environment != "" {
		about.WriteString(compactRow(s, "Env", meta.Environment, width))
	}
	if len(meta.Tags) > 0 {
		about.WriteString(compactRow(s, "Tags", strings.Join(meta.Tags, ", "), width))
	}
	if meta.LastUsedAt != nil {
		about.WriteString(compactRow(s, "Used", usage(meta), width))
	}
	if meta.Notes != "" {
		about.WriteString(compactRow(s, "Notes", meta.Notes, width))
	}
	if about.Len() > 0 {
		b.WriteString("\n")
		b.WriteString("  " + s.muted.Render("About") + "\n")
		b.WriteString(about.String())
	}
	return b.String()
}

func (m *App) renderKeys(s styleSet) string {
	filtered := m.filteredKeys()
	if len(filtered) == 0 {
		if m.loading || m.enriching {
			return "\n  " + s.muted.Render("Loading OpenSSH keys and agent…")
		}
		return "\n  " + s.muted.Render("No keys found. Press a to generate or i to import one.")
	}
	listWidth, detailWidth, bodyHeight := m.columnDimensions()
	layout := m.panelLayout()
	listHeight := layout.listHeight
	detailHeight := layout.detailHeight
	rowWidth := listWidth
	if layout.mobile && len(filtered) > listHeight {
		rowWidth -= mobileScrollbarWidth
	}
	start := scrollStart(m.cursor, len(filtered), listHeight)
	selectedRow := s.selected.Width(rowWidth)
	plainRow := s.plain.Width(rowWidth)
	var list strings.Builder
	for i := start; i < min(len(filtered), start+listHeight); i++ {
		key := filtered[i]
		prefix := "  "
		if key.InAgent {
			prefix = "● "
		}
		line := prefix + truncate(key.Name, rowWidth-4)
		if i == m.cursor {
			line = selectedRow.Render(line)
		} else {
			line = plainRow.Render(line)
		}
		list.WriteString(line + "\n")
	}
	listPanel := lipgloss.NewStyle().Width(rowWidth).Height(listHeight).Render(strings.TrimRight(list.String(), "\n"))
	if layout.mobile && len(filtered) > listHeight {
		listPanel = lipgloss.JoinHorizontal(lipgloss.Top, listPanel, m.renderMobileScrollbar(s, len(filtered), listHeight))
	}
	detail := lipgloss.NewStyle().Width(detailWidth).Height(detailHeight).Render(m.renderKeyDetail(s, filtered[m.cursor], detailWidth))
	if layout.mobile {
		divider := s.rule.Render(strings.Repeat("─", listWidth))
		return lipgloss.JoinVertical(lipgloss.Left, listPanel, divider, detail)
	}
	divider := s.rule.Render(strings.TrimSuffix(strings.Repeat("│\n", bodyHeight), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, divider, detail)
}

func (m *App) renderMobileScrollbar(s styleSet, total, height int) string {
	thumb := 0
	if total > 1 {
		track := max(0, height-3)
		thumb = (m.cursor*track + (total-1)/2) / (total - 1)
	}
	var bar strings.Builder
	bar.WriteString(s.active.Render(" ↑ ") + "\n")
	for row := range max(0, height-2) {
		glyph := " │ "
		if row == thumb {
			glyph = " ┃ "
		}
		bar.WriteString(s.muted.Render(glyph) + "\n")
	}
	bar.WriteString(s.active.Render(" ↓ "))
	return lipgloss.NewStyle().Width(mobileScrollbarWidth).Height(height).Render(bar.String())
}

func (m *App) renderKeyDetail(s styleSet, key keys.Key, width int) string {
	owner := "external"
	if key.Managed {
		owner = "Bast managed"
	}
	var b strings.Builder
	b.WriteString("  " + s.active.Render(truncate(key.Name, max(4, width-3))) + "\n")
	summaryParts := []string{noneValue(key.Algorithm), owner}
	if key.InAgent {
		summaryParts = append(summaryParts, "agent cached")
	}
	summary := strings.Join(summaryParts, " · ")
	b.WriteString("  " + s.muted.Render(truncate(summary, max(4, width-3))) + "\n")
	if key.Fingerprint != "" {
		b.WriteString("  " + s.value.Render(truncate(key.Fingerprint, max(4, width-3))) + "\n")
	} else {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if !key.Managed && key.PrivatePath != "" {
		b.WriteString("  " + s.active.Render("[p] Promote to Bast managed") + "\n\n")
	}
	if key.PublicPath != "" || key.PrivatePath != "" {
		b.WriteString("  " + s.active.Render(keyInstallAction) + "\n\n")
	}
	if key.PrivatePath != "" {
		b.WriteString(compactRow(s, "Private", shortPath(key.PrivatePath, m.paths.Home), width))
	}
	if key.PublicPath != "" {
		b.WriteString(compactRow(s, "Public", shortPath(key.PublicPath, m.paths.Home), width))
	}
	if key.Comment != "" {
		b.WriteString(compactRow(s, "Comment", key.Comment, width))
	}
	if len(key.References) > 0 {
		references := make([]string, 0, len(key.References))
		for _, alias := range key.References {
			if host, ok := m.findHost(alias); ok {
				references = append(references, m.hostLabel(host))
			} else {
				references = append(references, alias)
			}
		}
		b.WriteString(compactRow(s, "Used by", strings.Join(references, ", "), width))
	}
	return b.String()
}

func (m *App) renderForm(s styleSet) string {
	if isGroupAssignmentForm(m.form) {
		return m.renderGroupAssignmentForm(s)
	}
	if isHostForm(m.form) {
		return m.renderHostForm(s)
	}
	f := m.form
	var b strings.Builder
	current, total := formProgress(f)
	b.WriteString("\n  " + s.active.Render(f.title) + "  " + s.muted.Render(fmt.Sprintf("%d/%d", current, total)) + "\n\n")
	if target := destructiveConfirmationTarget(f); target != "" {
		b.WriteString("  " + s.label.Render("Name to type") + s.value.Render(target) + "\n\n")
	}
	for i, item := range f.fields {
		if item.hidden || i > f.revealed {
			continue
		}
		if i == f.index {
			label := item.label
			b.WriteString("  " + s.active.Render("› "+label) + "\n")
			if item.description != "" {
				b.WriteString("    " + s.muted.Render(truncate(item.description, max(20, m.terminalWidth()-8))) + "\n")
			}
			if len(item.options) > 0 {
				if f.selecting {
					rows := min(7, len(item.options))
					start := scrollStart(item.selected, len(item.options), rows)
					for optionIndex := start; optionIndex < min(len(item.options), start+rows); optionIndex++ {
						option := "  " + item.options[optionIndex].label
						if optionIndex == item.selected {
							option = s.selected.Render("› " + item.options[optionIndex].label)
						} else {
							option = s.muted.Render(option)
						}
						b.WriteString("    " + option + "\n")
					}
				} else if item.options[item.selected].custom {
					b.WriteString("    " + f.input.View() + "\n")
				} else {
					b.WriteString("    " + s.value.Render(item.options[item.selected].label) + "\n")
				}
			} else {
				b.WriteString("    " + f.input.View() + "\n")
			}
			if f.validationError != "" {
				b.WriteString("    " + s.error.Render("✕ "+f.validationError) + "\n")
			}
			if item.label == "Color" {
				colour := strings.TrimSpace(f.input.Value())
				if foreground, ok := contrastingTextColor(colour); ok {
					preview := lipgloss.NewStyle().Bold(true).
						Foreground(lipgloss.Color(foreground)).
						Background(lipgloss.Color(colour)).
						Padding(0, 1).
						Render("Host label preview")
					b.WriteString("    " + preview + "\n")
				}
			}
			continue
		}
		value := item.value
		if len(item.options) > 0 && !item.options[item.selected].custom {
			value = item.options[item.selected].label
		}
		if item.secret && value != "" {
			value = strings.Repeat("*", len([]rune(value)))
		}
		if value == "" {
			value = "-"
		}
		label := item.label
		b.WriteString("  " + s.muted.Render("  "+label+"  "+value) + "\n")
	}
	return b.String()
}

func (m *App) formHint() string {
	if isGroupAssignmentForm(m.form) {
		return groupAssignmentHint()
	}
	if isHostForm(m.form) {
		return hostFormHint(m.form, m.formTextInputActive(), m.hostSaveHintEnter)
	}
	f := m.form
	action := "󰌑 next"
	if isEditForm(f) && !f.selecting {
		action = "󰌑 save"
		if len(f.fields[f.index].options) > 0 && !f.fields[f.index].options[f.fields[f.index].selected].custom {
			action += " • ␣ change"
		}
	} else if f.selecting {
		action = "↑/↓ or j/k choose • 󰌑 select"
	} else if len(f.fields[f.index].options) > 0 && !f.fields[f.index].options[f.fields[f.index].selected].custom {
		action = "󰌑 change"
	}
	if !isEditForm(f) && !hasNextFormField(f) {
		action = "󰌑 save"
		if f.action == "host_delete" || f.action == "key_delete" || f.action == "known_delete" || f.action == "files_delete" {
			action = "󰌑 delete"
		}
		if f.action == "vault_reset_passphrase" {
			action = "󰌑 reset"
		}
		if f.action == "vault_rotate_passphrase" {
			action = "󰌑 rotate"
		}
	}
	if destructiveConfirmationTarget(f) != "" {
		action += " • Ctrl+Y copy name"
	}
	if f.selecting {
		return action + " • Esc/⌫ close"
	}
	escape := "Esc cancel"
	if !m.formTextInputActive() {
		escape = "Esc/⌫ cancel"
	}
	if len(f.fields[f.index].options) > 0 && f.fields[f.index].options[f.fields[f.index].selected].custom {
		escape = "Esc choices"
	}
	movement := "↑/↓ revisit"
	if isEditForm(f) {
		movement = "↑/↓ move"
	}
	return action + " • " + movement + " • " + escape
}

func contrastingTextColor(background string) (string, bool) {
	background = strings.TrimSpace(background)
	if !strings.HasPrefix(background, "#") {
		return "", false
	}
	hex := strings.TrimPrefix(background, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return "", false
	}
	value, err := strconv.ParseUint(hex, 16, 24)
	if err != nil {
		return "", false
	}
	channel := func(v uint64) float64 {
		srgb := float64(v) / 255
		if srgb <= 0.04045 {
			return srgb / 12.92
		}
		return math.Pow((srgb+0.055)/1.055, 2.4)
	}
	r := channel(value >> 16)
	g := channel((value >> 8) & 0xff)
	b := channel(value & 0xff)
	if 0.2126*r+0.7152*g+0.0722*b > 0.2 {
		return "#111827", true
	}
	return "#FFFFFF", true
}

type helpBinding struct {
	keys string
	desc string
}

type helpSection struct {
	title    string
	bindings []helpBinding
}

func helpSections() []helpSection {
	return []helpSection{
		{
			title: "Navigation",
			bindings: []helpBinding{
				{"↑ ↓  j k", "Move selection"},
				{"g  Home", "Jump to top"},
				{"G  End", "Jump to bottom"},
				{"/", "Search"},
				{"r", "Reload"},
				{"1", "Hosts"},
				{"2", "Keys"},
				{"3", "Vault"},
				{"4", "Sync"},
				{"5", "Files"},
				{"?", "Help"},
				{"v", "About"},
				{"q", "Quit"},
			},
		},
		{
			title: "Hosts",
			bindings: []helpBinding{
				{"󰌑", "Connect or add suggestion"},
				{"a", "Add host"},
				{"e", "Edit or review suggestion"},
				{"m", "Move host to group"},
				{"x", "Dismiss history suggestion"},
				{"d", "Delete host"},
				{"p", "Promote external host"},
				{"␣", "Collapse or expand group"},
				{"[", "Collapse all groups"},
				{"]", "Expand all groups"},
				{"s", "Cycle sort"},
				{"f", "Toggle favorite"},
				{"F", "Open Files for host"},
				{"h", "Hide or show selected"},
				{".", "Toggle hidden and stopped hosts"},
				{"n", "New VM on a provider group"},
				{"s", "Sync provider group, or cycle sort"},
				{"K", "Remove known-host entry"},
			},
		},
		{
			title: "Keys",
			bindings: []helpBinding{
				{"a", "Generate key"},
				{"i", "Import key"},
				{"e", "Edit comment"},
				{"d", "Delete key"},
				{"u", "Add to server"},
				{"x", "Export key"},
				{"p", "Promote external / change passphrase"},
				{"c", "Copy public key"},
			},
		},
		{
			title: "Vault",
			bindings: []helpBinding{
				{"󰌑", "Link, unlock, or sync"},
				{"j k", "Secondary actions"},
				{"r", "Sync now when unlocked"},
			},
		},
		{
			title: "Sync",
			bindings: []helpBinding{
				{"h j k l", "Grid move, or cycle actions"},
				{"󰌑", "Open provider, run action, or connect"},
				{"␣", "Collapse or expand status group"},
				{"s", "Sync"},
				{"Esc", "Back"},
				{"r", "Refresh status"},
			},
		},
		{
			title: "Files",
			bindings: []helpBinding{
				{"Tab", "Switch pane"},
				{"w", "Swap panes"},
				{"L  R", "Pane local / remote"},
				{"h  l", "Parent / enter"},
				{"󰌑", "Enter dir or connect host"},
				{"j k  g G", "Move / top / bottom"},
				{"f", "Fuzzy jump"},
				{"/", "Path jump or host search"},
				{"␣", "Toggle mark"},
				{"v", "Range mark"},
				{"c  m", "Copy / move to other pane"},
				{"d", "Delete"},
				{"a", "New directory"},
				{"r", "Rename"},
				{"i", "File info"},
				{"p", "Permissions (chmod)"},
				{"t", "Shell in directory"},
				{".", "Toggle hidden files"},
				{"D", "Disconnect remote"},
				{"Esc", "Clear marks / disconnect / Hosts"},
				{"Esc  x", "Cancel transfer or connect"},
			},
		},
		{
			title: "During SSH",
			bindings: []helpBinding{
				{"exit", "Return to Bast"},
				{"󰌑 then ~.", "Force-close a stuck session"},
			},
		},
	}
}

func (m *App) contextualHelpSections() []helpSection {
	all := helpSections()
	nav := all[0]
	byTitle := map[string]helpSection{}
	for _, section := range all[1:] {
		byTitle[section.title] = section
	}
	current := "Hosts"
	switch m.section {
	case keysSection:
		current = "Keys"
	case vaultSection:
		current = "Vault"
	case syncSection:
		current = "Sync"
	case filesSection:
		current = "Files"
	}
	out := []helpSection{nav, byTitle[current]}
	if current == "Hosts" {
		out = append(out, byTitle["During SSH"])
	}
	out = append(out, helpSection{
		title: "Docs",
		bindings: []helpBinding{
			{"bast.sh/docs", "Full shortcut reference"},
		},
	})
	return out
}

func (m *App) helpContentWidth() int {
	return max(36, m.terminalWidth()-6)
}

func (m *App) helpLines(s styleSet) []string {
	const keyCol = 18
	width := m.helpContentWidth()
	lines := []string{
		"",
		s.active.Render("Keyboard shortcuts"),
		"",
	}
	for i, section := range m.contextualHelpSections() {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, s.active.Render(section.title), "")
		for _, binding := range section.bindings {
			keys := s.value.Render(binding.keys)
			gap := max(1, keyCol-lipgloss.Width(binding.keys))
			descWidth := max(8, width-keyCol-2)
			desc := s.muted.Render(truncate(binding.desc, descWidth))
			lines = append(lines, keys+strings.Repeat(" ", gap)+desc)
		}
	}
	return lines
}

func (m *App) helpBodyHeight() int {
	return max(1, m.terminalHeight()-3)
}

func (m *App) maxHelpOffset() int {
	return max(0, len(m.helpLines(m.styles()))-m.helpBodyHeight())
}

func (m *App) clampHelpOffset() {
	if m.helpOffset < 0 {
		m.helpOffset = 0
	}
	if maxOffset := m.maxHelpOffset(); m.helpOffset > maxOffset {
		m.helpOffset = maxOffset
	}
}

func (m *App) scrollHelp(delta int) {
	m.helpOffset += delta
	m.clampHelpOffset()
}

func (m *App) helpCanScroll() bool {
	return m.maxHelpOffset() > 0
}

func (m *App) renderHelp(s styleSet) string {
	lines := m.helpLines(s)
	bodyHeight := m.helpBodyHeight()
	offset := min(max(0, m.helpOffset), max(0, len(lines)-bodyHeight))
	end := min(len(lines), offset+bodyHeight)
	content := lipgloss.NewStyle().
		MarginLeft(2).
		Render(strings.Join(lines[offset:end], "\n"))
	return lipgloss.Place(m.terminalWidth(), bodyHeight, lipgloss.Left, lipgloss.Top, content)
}

func (m *App) renderCredits(s styleSet) string {
	const infoWidth = 52
	banner := strings.Join([]string{
		"██████╗  █████╗ ███████╗████████╗",
		"██╔══██╗██╔══██╗██╔════╝╚══██╔══╝",
		"██████╔╝███████║███████╗   ██║   ",
		"██╔══██╗██╔══██║╚════██║   ██║   ",
		"██████╔╝██║  ██║███████║   ██║   ",
		"╚═════╝ ╚═╝  ╚═╝╚══════╝   ╚═╝   ",
	}, "\n")
	version := m.version
	if version == "" {
		version = "dev"
	}
	row := func(label, value string) string {
		gap := max(2, infoWidth-lipgloss.Width(label)-lipgloss.Width(value))
		return s.label.Width(lipgloss.Width(label)).Render(label) + strings.Repeat(" ", gap) + s.value.Render(value)
	}
	content := s.active.Width(infoWidth).Align(lipgloss.Center).Render(banner) +
		"\n\n" + s.muted.Width(infoWidth).Align(lipgloss.Center).Render("The fast way into the servers you use every day.") +
		"\n\n" + row("Created by", "@tedbrine") +
		"\n" + row("Website", "https://bast.sh") +
		"\n" + row("Repository", "github.com/ellipse-software/bast") +
		"\n" + row("License", "MIT License") +
		"\n" + row("Version", version)
	if m.latestVersion != "" {
		content += "\n" + row("Update", m.latestVersion+" · "+m.updateSuggestion)
	}
	return lipgloss.Place(m.terminalWidth(), max(1, m.terminalHeight()-3), lipgloss.Center, lipgloss.Center, content)
}

func (m *App) renderFooter(s styleSet) string {
	if m.statusError && m.status != "" {
		hint := "Enter / Esc / ⌫ return"
		return strings.Repeat(" ", max(1, m.terminalWidth()-lipgloss.Width(hint))) + s.muted.Render(hint)
	}
	if m.credits {
		hint := "v / Esc / ⌫ close"
		return strings.Repeat(" ", max(1, m.terminalWidth()-lipgloss.Width(hint))) + s.muted.Render(hint)
	}
	if m.help {
		hint := "? / Esc / ⌫ close"
		if m.helpCanScroll() {
			hint = "↑/↓ scroll · " + hint
		}
		return strings.Repeat(" ", max(1, m.terminalWidth()-lipgloss.Width(hint))) + s.muted.Render(hint)
	}
	query := m.searchText()
	left := ""
	switch {
	case m.vaultBusy != "":
		left = s.muted.Render(m.vaultBusy + " · esc cancel")
	case m.syncBusy != "":
		left = s.muted.Render(m.syncBusy)
	case m.syncActivity != "":
		left = s.muted.Render(m.syncActivity)
	case m.anySyncing():
		left = s.muted.Render("syncing…")
	case m.loading:
		left = s.muted.Render("loading…")
	case strings.HasPrefix(m.search, "\x00"):
		left = "/ " + query + "█"
	case query != "":
		left = "filter: " + query
	}
	if m.vaultBusy == "" && m.syncBusy == "" && m.syncActivity == "" && !m.anySyncing() && !m.loading && m.status != "" {
		if left != "" {
			left += "  •  "
		}
		if m.statusError {
			left += s.error.Render(m.status)
		} else {
			left += s.success.Render(m.status)
		}
	} else if left == "" && m.latestVersion != "" {
		left = s.active.Render("Update " + m.latestVersion + " · " + m.updateSuggestion)
	}
	hint := "?"
	if m.form != nil {
		hint = m.formHint()
	} else if m.vaultBusy != "" {
		hint = "esc"
	} else if m.syncBusy != "" {
		hint = ""
	} else {
		budget := max(8, m.terminalWidth()-lipgloss.Width(left)-1)
		hint = m.browseFooterHint(budget)
	}
	space := max(1, m.terminalWidth()-lipgloss.Width(left)-lipgloss.Width(hint)-1)
	return left + strings.Repeat(" ", space) + s.muted.Render(hint)
}

func (m *App) renderHeaderRule(s styleSet) string {
	width := m.terminalWidth()
	if m.statusError || m.credits || m.help || m.form != nil || m.loading || m.vaultBusyBlocksBody() || m.section == vaultSection || m.section == syncSection || m.section == filesSection || !m.hasListItems() {
		return s.rule.Render(strings.Repeat("─", width))
	}
	if m.isMobileLayout() {
		return s.rule.Render(strings.Repeat("─", width))
	}
	listWidth, detailWidth, _ := m.columnDimensions()
	return s.rule.Render(strings.Repeat("─", listWidth) + "┬" + strings.Repeat("─", detailWidth))
}

func (m *App) hasListItems() bool {
	switch m.section {
	case hostsSection:
		return len(m.hostListRows()) > 0
	case keysSection:
		return len(m.filteredKeys()) > 0
	case filesSection:
		return false
	default:
		return len(m.syncMenuItems()) > 0
	}
}

func compactRow(s styleSet, label, value string, width int) string {
	labelWidth := min(10, max(7, width/4))
	return "  " + s.label.Width(labelWidth).Render(label) + s.value.Render(truncate(value, max(4, width-labelWidth-3))) + "\n"
}
