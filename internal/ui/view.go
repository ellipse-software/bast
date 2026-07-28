package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/cloud/sync"
	"bast/internal/keys"
	"bast/internal/sshconfig"
)

const (
	connectAction        = " Connect "
	connectActionRow     = 0
	mobileScrollbarWidth = 3

	keyInstallAction    = "[u] Add to server"
	keyInstallActionRow = 4
)

func (m *App) connectButtonBounds(layout panelLayout) (x, y, width int) {
	btn := m.styles().title.Render(connectAction)
	width = lipgloss.Width(btn)
	y = layout.detailTop + connectActionRow
	x = max(0, m.terminalWidth()-width-2)
	return x, y, width
}

func (m *App) render() string {
	styles := m.styles()
	width := m.terminalWidth()
	bodyHeight := max(1, m.terminalHeight()-3)
	header := styles.title.Render(" BAST ") + "  " + m.renderTabs(styles)
	if m.version != "" && m.version != "dev" {
		space := width - lipgloss.Width(header) - lipgloss.Width(m.version)
		if space > 0 {
			header += strings.Repeat(" ", space) + styles.muted.Render(m.version)
		}
	}
	var body string
	if m.statusError && m.status != "" {
		body = m.renderError(styles)
	} else if m.credits {
		body = m.renderCredits(styles)
	} else if m.help {
		body = m.renderHelp(styles)
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

func (m *App) renderError(s styleSet) string {
	contentWidth := max(16, min(66, m.terminalWidth()-10))
	explanation := "The action did not complete."
	if m.form != nil {
		explanation += " Your entries are still available when you return."
	}
	content := s.error.Bold(true).Render("✕  Action failed") +
		"\n\n" + s.muted.Render("What happened") +
		"\n" + s.value.Width(contentWidth).Render(m.status) +
		"\n\n" + s.muted.Width(contentWidth).Render(explanation) +
		"\n\n" + s.muted.Render("Press Enter or Esc to return")
	panel := lipgloss.NewStyle().
		Width(contentWidth).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#EF4444")).
		Render(content)
	return lipgloss.Place(m.terminalWidth(), max(1, m.terminalHeight()-3), lipgloss.Center, lipgloss.Center, panel)
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
	hosts, keyTab, syncTab := "[1] Hosts", "[2] Keys", "[3] Sync"
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
	return hosts + "   " + keyTab + "   " + syncTab
}

func (m *App) renderHosts(s styleSet) string {
	filtered := m.filteredHosts()
	if len(filtered) == 0 {
		if m.loading {
			return "\n  " + s.muted.Render("Loading OpenSSH configuration…")
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
	rowsData := m.hostRows()
	rowWidth := listWidth
	if layout.mobile && len(rowsData) > listHeight {
		rowWidth -= mobileScrollbarWidth
	}
	start := scrollStart(m.cursor, len(rowsData), listHeight)
	var list strings.Builder
	for i := start; i < min(len(rowsData), start+listHeight); i++ {
		row := rowsData[i]
		if row.header {
			indent := strings.Repeat("  ", row.depth)
			indicator := "▾"
			if m.collapsedGroups[row.group] && m.searchText() == "" {
				indicator = "▸"
			}
			name := row.group
			if slash := strings.LastIndex(name, "/"); row.depth > 0 && slash >= 0 {
				name = name[slash+1:]
			}
			name = truncate(name, max(2, rowWidth-lipgloss.Width(indent)-8))
			line := indent + indicator + " " + renderManagedGroupName(name, s.active) + " " + s.muted.Render(fmt.Sprintf("(%d)", row.count))
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
		if rowsData[m.cursor].header {
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

func (m *App) renderGroupDetail(s styleSet, row hostRow, width int) string {
	state := "expanded"
	if m.collapsedGroups[row.group] {
		state = "collapsed"
		if m.searchText() != "" {
			state = "expanded for search"
		}
	}
	hint := "␣ collapse/expand · e rename"
	if sync.IsSyncedGroup(row.group) {
		hint = "␣ collapse/expand · cloud sync (read-only)"
	}
	return "  " + renderManagedGroupName(truncate(row.group, max(4, width-3)), s.active) + "\n" +
		"  " + s.muted.Render(fmt.Sprintf("%d servers · %s", row.count, state)) + "\n\n" +
		"  " + s.muted.Render(truncate(hint, max(4, width-3)))
}

func renderManagedGroupName(name string, restStyle lipgloss.Style) string {
	if name == "Amazon EC2" || strings.HasPrefix(name, "Amazon EC2/") {
		amazon := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render("Amazon")
		ec2 := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF9900")).Render(" EC2")
		return amazon + ec2 + restStyle.Render(strings.TrimPrefix(name, "Amazon EC2"))
	}
	if name == "Google Cloud" || strings.HasPrefix(name, "Google Cloud/") {
		colors := []string{"#4285F4", "#EA4335", "#FBBC05", "#4285F4", "#34A853", "#EA4335"}
		var rendered strings.Builder
		for i, letter := range "Google" {
			rendered.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colors[i])).Render(string(letter)))
		}
		remainder := name[len("Google"):]
		cloud := " Cloud"
		rendered.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(cloud))
		rendered.WriteString(restStyle.Render(strings.TrimPrefix(remainder, cloud)))
		return rendered.String()
	}
	return name
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
		if m.loading {
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
	if isHostForm(m.form) {
		return m.renderHostForm(s)
	}
	f := m.form
	var b strings.Builder
	current, total := formProgress(f)
	b.WriteString("\n  " + s.active.Render(f.title) + "  " + s.muted.Render(fmt.Sprintf("%d/%d", current, total)) + "\n\n")
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
		if value == "" {
			value = "—"
		}
		label := item.label
		b.WriteString("  " + s.muted.Render("  "+label+"  "+value) + "\n")
	}
	return b.String()
}

func (m *App) formHint() string {
	if isHostForm(m.form) {
		return hostFormHint(m.form)
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

func (m *App) renderHelp(s styleSet) string {
	lines := []string{"Navigation", "  ↑/↓ or j/k  move       /  search       r  reload", "  1  hosts       2  keys   3  sync   ?  help         v  about       q  quit", "", "Hosts", "  󰌑 connect      a add     e edit         d delete", "  ␣ collapse/expand                        s sort", "  f favorite      h hide/show selected     . toggle hidden hosts", "  K remove known-host entry", "", "Keys", "  a generate      i import  e edit comment d delete", "  u add to server x export  p change passphrase       c copy public key", "", "Sync", "  󰌑 open provider / run action   Esc back   r refresh", "", "During SSH", "  exit returns to Bast; press 󰌑 then ~. to force-close a stuck session"}
	return "\n  " + s.active.Render("Keyboard help") + "\n\n" + strings.Join(lines, "\n") + "\n\n  " + s.muted.Render("Press ?, Esc, or ⌫ to close")
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
	query := m.searchText()
	left := ""
	if m.anySyncing() {
		left = s.muted.Render("syncing…")
	} else if strings.HasPrefix(m.search, "\x00") {
		left = "/ " + query + "█"
	} else if query != "" {
		left = "filter: " + query
	}
	if !m.anySyncing() && m.status != "" {
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
		if _, groupSelected := m.selectedGroupHeader(); groupSelected {
			if m.isMobileLayout() {
				hint = "↑/↓ or j/k move • e rename • ␣ collapse/expand • a add • v about • ? help"
			} else {
				hint = "␣ collapse/expand • e rename • a add • v about • ? help"
			}
		} else if m.isMobileLayout() {
			hint = "↑/↓ or j/k move • click Connect • a add • v about • ? help"
		} else {
			hint = "󰌑 connect • ␣ collapse/expand • a add • h hide • v about • ? help"
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
	if m.statusError || m.credits || m.help || m.form != nil || m.loading || m.section == syncSection || m.itemCount() == 0 {
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
