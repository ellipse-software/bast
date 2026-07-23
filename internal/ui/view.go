package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
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
	if m.help {
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
	rows := bodyHeight
	start := scrollStart(m.cursor, len(filtered), rows)
	var list strings.Builder
	for i := start; i < min(len(filtered), start+rows); i++ {
		host := filtered[i]
		meta := m.metadata.Host(host.Alias)
		prefix := "  "
		if meta.Hidden {
			prefix = "◌ "
		} else if meta.Favorite {
			prefix = "◆ "
		}
		line := prefix + truncate(host.Alias, listWidth-4)
		if i == m.cursor {
			line = s.selected.Width(listWidth).Render(line)
		} else {
			line = lipgloss.NewStyle().Width(listWidth).Render(line)
		}
		list.WriteString(line + "\n")
	}
	listPanel := lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Render(strings.TrimRight(list.String(), "\n"))
	detail := lipgloss.NewStyle().Width(detailWidth).Height(bodyHeight).Render(m.renderHostDetail(s, filtered[m.cursor], detailWidth))
	divider := s.rule.Render(strings.TrimSuffix(strings.Repeat("│\n", bodyHeight), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, divider, detail)
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
	titleStyle := s.active
	if strings.HasPrefix(meta.Color, "#") {
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(meta.Color))
	}
	destination := destination(host)
	if destination == "" {
		destination = host.Alias
	}
	b.WriteString("  " + titleStyle.Render(truncate(host.Alias, max(4, width-3))) + "\n")
	b.WriteString("  " + s.value.Render(truncate(destination, max(4, width-3))) + "\n")
	b.WriteString("  " + s.muted.Render(truncate(owner+" · "+trust, max(4, width-3))) + "\n\n")
	b.WriteString(compactRow(s, "Source", shortPath(host.Source, m.paths.Home)+":"+strconv.Itoa(host.Line), width))
	b.WriteString(compactRow(s, "Key", joinOr(host.Resolved.IdentityFiles, "agent/defaults"), width))
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
	}
	b.WriteString("\n")
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
		b.WriteString(compactRow(s, "Used by", strings.Join(key.References, ", "), width))
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
			b.WriteString("  " + s.active.Render("› "+item.label) + "\n")
			b.WriteString("    " + f.input.View() + "\n")
			continue
		}
		value := item.value
		if value == "" {
			value = "—"
		}
		b.WriteString("  " + s.muted.Render("  "+item.label+"  "+value) + "\n")
	}
	action := "󰌑 next"
	if !hasNextFormField(f) {
		action = "󰌑 save"
		if f.action == "host_delete" || f.action == "key_delete" || f.action == "known_delete" {
			action = "󰌑 delete"
		}
	}
	b.WriteString("\n  " + s.muted.Render(action+" • ↑/↓ revisit • Esc cancel"))
	return b.String()
}

func (m *App) renderHelp(s styleSet) string {
	lines := []string{"Navigation", "  ↑/↓ or j/k  move       /  search       r  reload", "  1  hosts       2  keys   ?  help         q  quit", "", "Hosts", "  󰌑 connect      a add     e edit         d delete", "  f favorite      h hide/show selected     . toggle hidden hosts", "  s sort          K remove known-host entry", "", "Keys", "  a generate      i import  e edit comment d delete", "  x export        p change passphrase       c copy public key", "", "During SSH", "  exit returns normally; press 󰌑 then ~. to force-close a stuck session"}
	return "\n  " + s.active.Render("Keyboard help") + "\n\n" + strings.Join(lines, "\n") + "\n\n  " + s.muted.Render("Press ? or Esc to close")
}

func (m *App) renderFooter(s styleSet) string {
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
	if m.section == hostsSection {
		hint = "󰌑 connect • a add • h hide • . hidden • ? help"
	} else {
		hint = "a generate • i import • e comment • x export • ? help"
	}
	space := max(1, m.terminalWidth()-lipgloss.Width(left)-lipgloss.Width(hint)-1)
	return left + strings.Repeat(" ", space) + s.muted.Render(hint)
}

func (m *App) renderHeaderRule(s styleSet) string {
	width := m.terminalWidth()
	if m.help || m.form != nil || m.loading || m.itemCount() == 0 {
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
