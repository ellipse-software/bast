package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

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
	addAction            = " Add "
	connectActionRow     = 0
	mobileScrollbarWidth = 3

	keyInstallAction    = "[u] Add to server"
	keyInstallActionRow = 4
)

var headerTabLabels = [...]string{"[1] Hosts", "[2] Keys", "[3] Sync"}

func (m *App) connectButtonBounds(layout panelLayout) (x, y, width int) {
	return m.hostActionButtonBounds(layout, connectAction)
}

func (m *App) hostActionButtonBounds(layout panelLayout, action string) (x, y, width int) {
	btn := m.styles().title.Render(action)
	width = lipgloss.Width(btn)
	y = layout.detailTop + connectActionRow
	x = max(0, m.terminalWidth()-width-2)
	return x, y, width
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
	} else if m.vaultBusy != "" {
		body = m.renderVaultBusy(styles)
	} else if m.form != nil {
		body = m.renderForm(styles)
	} else if m.section == hostsSection {
		body = m.renderHosts(styles)
	} else if m.section == keysSection {
		body = m.renderKeys(styles)
	} else {
		body = m.renderSync(styles)
	}
	body = lipgloss.NewStyle().Width(width).Height(bodyHeight).Render(body)
	footer := m.renderFooter(styles)
	return lipgloss.NewStyle().Width(width).Render(header + "\n" + m.renderHeaderRule(styles) + "\n" + body + "\n" + footer)
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

func (m *App) renderVaultBusy(s styleSet) string {
	return "\n  " + s.active.Render(m.vaultBusy) + "\n"
}

type styleSet struct{ title, active, inactive, selected, muted, label, value, error, success, rule lipgloss.Style }

func (m *App) styles() styleSet {
	primary := lipgloss.Color("#8B5CF6")
	text := lipgloss.Color("#E5E7EB")
	muted := lipgloss.Color("#6B7280")
	surface := lipgloss.Color("#1F2937")
	if !m.dark {
		text = lipgloss.Color("#111827")
		muted = lipgloss.Color("#6B7280")
		surface = lipgloss.Color("#EDE9FE")
	}
	return styleSet{
		title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(primary),
		active: lipgloss.NewStyle().Bold(true).Foreground(primary), inactive: lipgloss.NewStyle().Foreground(muted),
		selected: lipgloss.NewStyle().Bold(true).Foreground(text).Background(surface), muted: lipgloss.NewStyle().Foreground(muted),
		label: lipgloss.NewStyle().Foreground(muted).Width(16), value: lipgloss.NewStyle().Foreground(text),
		error: lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")), success: lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")),
		rule: lipgloss.NewStyle().Foreground(surface),
	}
}

func (m *App) renderTabs(s styleSet) string {
	hosts, keyTab, syncTab := headerTabLabels[0], headerTabLabels[1], headerTabLabels[2]
	switch m.section {
	case hostsSection:
		hosts = s.active.Render(hosts)
		keyTab = s.inactive.Render(keyTab)
		syncTab = s.inactive.Render(syncTab)
	case keysSection:
		hosts = s.inactive.Render(hosts)
		keyTab = s.active.Render(keyTab)
		syncTab = s.inactive.Render(syncTab)
	default:
		hosts = s.inactive.Render(hosts)
		keyTab = s.inactive.Render(keyTab)
		syncTab = s.active.Render(syncTab)
	}
	return hosts + headerTabSpacing + keyTab + headerTabSpacing + syncTab
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
			return "\n  " + s.muted.Render("No visible hosts. Press . to show hidden hosts.")
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
	start := scrollStart(m.cursor, len(rowsData), listHeight)
	var list strings.Builder
	for i := start; i < min(len(rowsData), start+listHeight); i++ {
		row := rowsData[i]
		if row.historyHeader {
			indicator := "▾"
			if m.historySuggestionsCollapsed && m.searchText() == "" {
				indicator = "▸"
			}
			line := indicator + " " + row.label + " " + s.muted.Render(fmt.Sprintf("(%d)", row.count))
			if i == m.cursor {
				line = s.selected.Width(rowWidth).Render(line)
			} else {
				line = s.muted.Width(rowWidth).Render(line)
			}
			list.WriteString(line + "\n")
			continue
		}
		if row.suggestion != nil {
			indent := strings.Repeat("  ", row.depth)
			line := indent + "＋ " + truncate(row.suggestion.Alias, max(2, rowWidth-lipgloss.Width(indent)-4))
			if i == m.cursor {
				line = s.selected.Width(rowWidth).Render(line)
			} else {
				line = s.muted.Width(rowWidth).Render(line)
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
			if row.depth == 0 && m.cloudSyncGroupHasError(row.group) {
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
				line = s.selected.Width(rowWidth).Render(line)
			} else {
				line = s.active.Width(rowWidth).Render(line)
			}
			list.WriteString(line + "\n")
			continue
		}
		host := row.host
		meta := m.metadata.Host(host.Alias)
		prefix := "  "
		if meta.Hidden {
			prefix = "◌ "
		} else if meta.Favorite {
			prefix = "◆ "
		}
		indent := strings.Repeat("  ", row.depth)
		line := indent + prefix + truncate(m.hostLabel(host), max(2, rowWidth-lipgloss.Width(indent+prefix)-2))
		if i == m.cursor {
			line = s.selected.Width(rowWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(rowWidth).Render(line)
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
	addButton := s.title.Render(addAction)
	title := truncate(suggestion.Alias, max(4, width-lipgloss.Width(addButton)-4))
	gap := max(1, width-2-lipgloss.Width(title)-lipgloss.Width(addButton))
	b.WriteString("  " + s.active.Render(title) + strings.Repeat(" ", gap) + addButton + "\n")
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
	switch group {
	case "Google Cloud", "GCP":
		return m.metadata.GCP().LastSyncError != "" || m.syncStatus.GCP.GCloudError != ""
	case "Amazon EC2", "AWS":
		return m.metadata.AWS().LastSyncError != "" || m.syncStatus.AWS.AWSCLIError != ""
	case "Microsoft Azure":
		return m.metadata.Azure().LastSyncError != "" || m.syncStatus.Azure.AzureCLIError != ""
	default:
		return false
	}
}

func (m *App) renderGroupDetail(s styleSet, row hostRow, width int) string {
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

func renderManagedGroupName(name string, restStyle lipgloss.Style, nerdFont bool) string {
	if name == "Amazon EC2" || strings.HasPrefix(name, "Amazon EC2/") {
		providerName := managedGroupIcon(name, nerdFont) + "Amazon EC2"
		provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF9900")).Render(providerName)
		return provider + restStyle.Render(strings.TrimPrefix(name, "Amazon EC2"))
	}
	if name == "Google Cloud" || strings.HasPrefix(name, "Google Cloud/") {
		colors := []string{"#4285F4", "#EA4335", "#FBBC05", "#4285F4", "#34A853", "#EA4335"}
		var rendered strings.Builder
		if icon := managedGroupIcon(name, nerdFont); icon != "" {
			rendered.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4285F4")).Render(icon))
		}
		for i, letter := range "Google" {
			rendered.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors[i])).Render(string(letter)))
		}
		remainder := name[len("Google"):]
		cloud := " Cloud"
		rendered.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(cloud))
		rendered.WriteString(restStyle.Render(strings.TrimPrefix(remainder, cloud)))
		return rendered.String()
	}
	if name == "Microsoft Azure" || strings.HasPrefix(name, "Microsoft Azure/") {
		providerName := managedGroupIcon(name, nerdFont) + "Microsoft Azure"
		provider := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0078D4")).Render(providerName)
		return provider + restStyle.Render(strings.TrimPrefix(name, "Microsoft Azure"))
	}
	return name
}

func managedGroupIcon(name string, nerdFont bool) string {
	if !nerdFont {
		return ""
	}
	switch {
	case name == "Amazon EC2" || strings.HasPrefix(name, "Amazon EC2/"):
		return "\ue7ad "
	case name == "Google Cloud" || strings.HasPrefix(name, "Google Cloud/"):
		return "\ue7f1 "
	case name == "Microsoft Azure" || strings.HasPrefix(name, "Microsoft Azure/"):
		return "\ue754 "
	default:
		return ""
	}
}

func (m *App) renderHostDetail(s styleSet, host sshconfig.Host, width int) string {
	meta := m.metadata.Host(host.Alias)
	var b strings.Builder
	label := m.hostLabel(host)
	connectBtn := s.title.Render(connectAction)
	connectBtnWidth := lipgloss.Width(connectBtn)
	titleStyle := s.active
	titleMax := max(4, width-connectBtnWidth-4)
	title := truncate(label, titleMax)
	if foreground, ok := contrastingTextColor(meta.Color); ok {
		titleStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(foreground)).
			Background(lipgloss.Color(meta.Color)).
			Padding(0, 1)
		titleMax = max(4, width-connectBtnWidth-6)
		title = truncate(label, titleMax)
	}
	dest := destination(host)
	if dest == "" {
		dest = host.Alias
	}
	titlePart := titleStyle.Render(title)
	gap := max(1, width-2-lipgloss.Width(titlePart)-connectBtnWidth)
	b.WriteString("  " + titlePart + strings.Repeat(" ", gap) + connectBtn + "\n")
	b.WriteString("  " + s.value.Render(truncate(dest, max(4, width-3))) + "\n")
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
	var list strings.Builder
	for i := start; i < min(len(filtered), start+listHeight); i++ {
		key := filtered[i]
		prefix := "  "
		if key.InAgent {
			prefix = "● "
		}
		line := prefix + truncate(key.Name, rowWidth-4)
		if i == m.cursor {
			line = s.selected.Width(rowWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(rowWidth).Render(line)
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
			value = "—"
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
		if f.action == "host_delete" || f.action == "key_delete" || f.action == "known_delete" {
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
				{"3", "Sync"},
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
				{"h", "Hide or show selected"},
				{".", "Toggle hidden hosts"},
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
			title: "Sync",
			bindings: []helpBinding{
				{"󰌑", "Open Vault/provider or run action"},
				{"Esc", "Back"},
				{"r", "Refresh"},
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

func (m *App) helpContentWidth() int {
	return min(48, max(36, m.terminalWidth()-8))
}

func (m *App) helpLines(s styleSet) []string {
	const keyWidth = 16
	width := m.helpContentWidth()
	lines := []string{
		"",
		s.active.Render("Keyboard shortcuts"),
		"",
	}
	for i, section := range helpSections() {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, s.active.Render(section.title), "")
		for _, binding := range section.bindings {
			keys := s.value.Width(keyWidth).Render(binding.keys)
			descWidth := max(8, width-keyWidth-2)
			desc := s.muted.Width(descWidth).Render(binding.desc)
			lines = append(lines, keys+"  "+desc)
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
		Width(m.helpContentWidth()).
		MarginLeft(3).
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
		"\n\n" + s.muted.Width(infoWidth).Align(lipgloss.Center).Render("Native SSH picker and key manager") +
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
	if m.vaultBusy != "" {
		left := s.muted.Render("please wait…")
		return left + strings.Repeat(" ", max(1, m.terminalWidth()-lipgloss.Width(left)))
	}
	query := m.searchText()
	left := ""
	switch {
	case m.anySyncing():
		left = s.muted.Render("syncing…")
	case m.loading:
		left = s.muted.Render("loading…")
	case strings.HasPrefix(m.search, "\x00"):
		left = "/ " + query + "█"
	case query != "":
		left = "filter: " + query
	}
	if !m.anySyncing() && !m.loading && m.status != "" {
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
	hint := "v about • ? help"
	if m.form != nil {
		hint = m.formHint()
	} else if m.section == hostsSection {
		if _, suggestionSelected := m.selectedHistorySuggestion(); suggestionSelected {
			hint = "󰌑 add • e review • x dismiss • v about • ? help"
		} else if m.historySuggestionsHeaderSelected() {
			hint = "␣ collapse/expand • v about • ? help"
		} else if _, groupSelected := m.selectedGroupHeader(); groupSelected {
			collapse := "␣ " + m.collapseActionLabel()
			if m.isMobileLayout() {
				hint = "↑/↓ or j/k move • e rename • " + collapse + " • a add • v about • ? help"
			} else {
				hint = collapse + " • e rename • a add • v about • ? help"
			}
		} else if m.isMobileLayout() {
			hint = "↑/↓ or j/k move • enter/click Connect • m group • a add • v about • ? help"
		} else {
			hint = "󰌑 connect • m group • ␣ " + m.collapseActionLabel() + " • a add • h hide • v about • ? help"
		}
	} else if m.section == keysSection {
		hint = "a generate • i import • u add to server • x export • v about • ? help"
	} else if m.syncProvider == "" {
		hint = "󰌑 open • j/k move • v about • ? help"
	} else {
		hint = "󰌑 run • j/k move • Esc back • r refresh • v about • ? help"
	}
	space := max(1, m.terminalWidth()-lipgloss.Width(left)-lipgloss.Width(hint)-1)
	return left + strings.Repeat(" ", space) + s.muted.Render(hint)
}

func (m *App) renderHeaderRule(s styleSet) string {
	width := m.terminalWidth()
	if m.statusError || m.credits || m.help || m.form != nil || m.vaultBusy != "" || m.loading || m.section == syncSection || m.itemCount() == 0 {
		return s.rule.Render(strings.Repeat("─", width))
	}
	if m.isMobileLayout() {
		return s.rule.Render(strings.Repeat("─", width))
	}
	listWidth, detailWidth, _ := m.columnDimensions()
	return s.rule.Render(strings.Repeat("─", listWidth) + "┬" + strings.Repeat("─", detailWidth))
}

func compactRow(s styleSet, label, value string, width int) string {
	labelWidth := min(10, max(7, width/4))
	return "  " + s.label.Width(labelWidth).Render(label) + s.value.Render(truncate(value, max(4, width-labelWidth-3))) + "\n"
}
