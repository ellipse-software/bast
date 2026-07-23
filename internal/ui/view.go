package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

const (
	keyInstallAction    = "[u] Add to server"
	keyInstallActionRow = 4
)

func (m *App) render() string {
	styles := m.styles()
	width := m.terminalWidth()
	bodyHeight := max(1, m.terminalHeight()-3)
	header := styles.title.Render(" BAST ") + "  " + m.renderTabs(styles)
	if m.loading {
		header += "  " + styles.muted.Render("syncing…")
	}
	var body string
	if m.statusError && m.status != "" {
		body = m.renderError(styles)
	} else if m.help {
		body = m.renderHelp(styles)
	} else if m.form != nil {
		body = m.renderForm(styles)
	} else if m.section == hostsSection {
		body = m.renderHosts(styles)
	} else {
		body = m.renderKeys(styles)
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
	hosts, keyTab := "[1] Hosts", "[2] Keys"
	if m.section == hostsSection {
		hosts = s.active.Render(hosts)
		keyTab = s.inactive.Render(keyTab)
	} else {
		hosts = s.inactive.Render(hosts)
		keyTab = s.active.Render(keyTab)
	}
	return hosts + "   " + keyTab
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
	rowsData := m.hostRows()
	rows := bodyHeight
	start := scrollStart(m.cursor, len(rowsData), rows)
	var list strings.Builder
	for i := start; i < min(len(rowsData), start+rows); i++ {
		row := rowsData[i]
		if row.header {
			indicator := "▾"
			if m.collapsedGroups[row.group] && m.searchText() == "" {
				indicator = "▸"
			}
			line := indicator + " " + truncate(row.group, max(2, listWidth-8)) + " " + s.muted.Render(fmt.Sprintf("(%d)", row.count))
			if i == m.cursor {
				line = s.selected.Width(listWidth).Render(line)
			} else {
				line = s.active.Width(listWidth).Render(line)
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
		indent := ""
		if row.group != "" {
			indent = "  "
		}
		line := indent + prefix + truncate(m.hostLabel(host), max(2, listWidth-lipgloss.Width(indent+prefix)-2))
		if i == m.cursor {
			line = s.selected.Width(listWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(listWidth).Render(line)
		}
		list.WriteString(line + "\n")
	}
	listPanel := lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Render(strings.TrimRight(list.String(), "\n"))
	detailContent := ""
	if m.cursor >= 0 && m.cursor < len(rowsData) {
		if rowsData[m.cursor].header {
			detailContent = m.renderGroupDetail(s, rowsData[m.cursor], detailWidth)
		} else {
			detailContent = m.renderHostDetail(s, rowsData[m.cursor].host, detailWidth)
		}
	}
	detail := lipgloss.NewStyle().Width(detailWidth).Height(bodyHeight).Render(detailContent)
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
	return "  " + s.active.Render(truncate(row.group, max(4, width-3))) + "\n" +
		"  " + s.muted.Render(fmt.Sprintf("%d servers · %s", row.count, state)) + "\n\n" +
		"  " + s.value.Render("Press ␣ to collapse or expand this group.")
}

func (m *App) renderHostDetail(s styleSet, host sshconfig.Host, width int) string {
	meta := m.metadata.Host(host.Alias)
	owner := "external"
	if host.Managed {
		owner = "Bast managed"
	}
	trust := "not in known_hosts"
	if host.KnownHost {
		trust = "known host"
	}
	var b strings.Builder
	label := m.hostLabel(host)
	titleStyle := s.active
	title := truncate(label, max(4, width-3))
	if foreground, ok := contrastingTextColor(meta.Color); ok {
		titleStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(foreground)).
			Background(lipgloss.Color(meta.Color)).
			Padding(0, 1)
		title = truncate(label, max(4, width-5))
	}
	destination := destination(host)
	if destination == "" {
		destination = host.Alias
	}
	b.WriteString("  " + titleStyle.Render(title) + "\n")
	b.WriteString("  " + s.value.Render(truncate(destination, max(4, width-3))) + "\n")
	b.WriteString("  " + s.muted.Render(truncate(owner+" · "+trust, max(4, width-3))) + "\n\n")
	b.WriteString(compactRow(s, "Source", shortPath(host.Source, m.paths.Home)+":"+strconv.Itoa(host.Line), width))
	if label != host.Alias {
		b.WriteString(compactRow(s, "SSH name", host.Alias, width))
	}
	b.WriteString(compactRow(s, "Key", hostIdentity(host), width))
	if host.Resolved.ProxyJump != "" && host.Resolved.ProxyJump != "none" {
		b.WriteString(compactRow(s, "Jump", host.Resolved.ProxyJump, width))
	}
	if inventory := inventorySummary(meta); inventory != "" {
		b.WriteString(compactRow(s, "Meta", inventory, width))
	}
	if meta.LastUsedAt != nil {
		b.WriteString(compactRow(s, "Used", usage(meta), width))
	}
	if meta.Notes != "" {
		b.WriteString(compactRow(s, "Notes", meta.Notes, width))
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
	rows := bodyHeight
	start := scrollStart(m.cursor, len(filtered), rows)
	var list strings.Builder
	for i := start; i < min(len(filtered), start+rows); i++ {
		key := filtered[i]
		prefix := "  "
		if key.InAgent {
			prefix = "● "
		}
		line := prefix + truncate(key.Name, listWidth-4)
		if i == m.cursor {
			line = s.selected.Width(listWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(listWidth).Render(line)
		}
		list.WriteString(line + "\n")
	}
	listPanel := lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Render(strings.TrimRight(list.String(), "\n"))
	detail := lipgloss.NewStyle().Width(detailWidth).Height(bodyHeight).Render(m.renderKeyDetail(s, filtered[m.cursor], detailWidth))
	divider := s.rule.Render(strings.TrimSuffix(strings.Repeat("│\n", bodyHeight), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, divider, detail)
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
		return action + " • Esc close"
	}
	escape := "Esc cancel"
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
	lines := []string{"Navigation", "  ↑/↓ or j/k  move       /  search       r  reload", "  1  hosts       2  keys   ?  help         q  quit", "", "Hosts", "  󰌑 connect      a add     e edit         d delete", "  ␣ collapse/expand group                  s sort", "  f favorite      h hide/show selected     . toggle hidden hosts", "  K remove known-host entry", "", "Keys", "  a generate      i import  e edit comment d delete", "  u add to server x export  p change passphrase       c copy public key", "", "During SSH", "  exit returns normally; press 󰌑 then ~. to force-close a stuck session"}
	return "\n  " + s.active.Render("Keyboard help") + "\n\n" + strings.Join(lines, "\n") + "\n\n  " + s.muted.Render("Press ? or Esc to close")
}

func (m *App) renderFooter(s styleSet) string {
	if m.statusError && m.status != "" {
		hint := "Enter / Esc return"
		return strings.Repeat(" ", max(1, m.terminalWidth()-lipgloss.Width(hint))) + s.muted.Render(hint)
	}
	query := m.searchText()
	left := ""
	if strings.HasPrefix(m.search, "\x00") {
		left = "/ " + query + "█"
	} else if query != "" {
		left = "filter: " + query
	}
	if m.status != "" {
		if left != "" {
			left += "  •  "
		}
		if m.statusError {
			left += s.error.Render(m.status)
		} else {
			left += s.success.Render(m.status)
		}
	}
	hint := "? help"
	if m.form != nil {
		hint = m.formHint()
	} else if m.section == hostsSection {
		hint = "󰌑 connect • ␣ group • a add • h hide • ? help"
	} else {
		hint = "a generate • i import • u add to server • x export • ? help"
	}
	space := max(1, m.terminalWidth()-lipgloss.Width(left)-lipgloss.Width(hint)-1)
	return left + strings.Repeat(" ", space) + s.muted.Render(hint)
}

func (m *App) renderHeaderRule(s styleSet) string {
	width := m.terminalWidth()
	if m.statusError || m.help || m.form != nil || m.loading || m.itemCount() == 0 {
		return s.rule.Render(strings.Repeat("─", width))
	}
	listWidth, detailWidth, _ := m.columnDimensions()
	return s.rule.Render(strings.Repeat("─", listWidth) + "┬" + strings.Repeat("─", detailWidth))
}

func compactRow(s styleSet, label, value string, width int) string {
	labelWidth := min(10, max(7, width/4))
	return "  " + s.label.Width(labelWidth).Render(label) + s.value.Render(truncate(value, max(4, width-labelWidth-3))) + "\n"
}

func inventorySummary(host metadata.Host) string {
	parts := make([]string, 0, 3)
	if host.Group != "" {
		parts = append(parts, host.Group)
	}
	if host.Environment != "" {
		parts = append(parts, host.Environment)
	}
	if len(host.Tags) > 0 {
		parts = append(parts, strings.Join(host.Tags, ", "))
	}
	return strings.Join(parts, " · ")
}
