package ui

import (
	"context"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/askpass"
	"bast/internal/hostlife"
	"bast/internal/sshconfig"
)

const forwardAction = " Forward "

func (m *App) activeForwardAliases() []string {
	if m.tunnelSup == nil {
		return nil
	}
	return m.tunnelSup.ConfigAliases()
}

func (m *App) forwardOpen(alias string) bool {
	return m.tunnelSup != nil && m.tunnelSup.ConfigOpen(alias)
}

func (m *App) hostForwardSettings(host sshconfig.Host) sshconfig.AdvancedSettings {
	if host.ManagedID == "" {
		return sshconfig.AdvancedSettings{}
	}
	extras, err := m.config.ManagedExtras(host.ManagedID)
	if err != nil {
		return sshconfig.AdvancedSettings{}
	}
	return sshconfig.ParseAdvanced(extras, host.Resolved.ProxyJump)
}

func (m *App) hostHasForwards(host sshconfig.Host) bool {
	return forwardsConfigured(m.hostForwardSettings(host))
}

func forwardsConfigured(settings sshconfig.AdvancedSettings) bool {
	return len(settings.LocalForwards) > 0 || len(settings.RemoteForwards) > 0 || strings.TrimSpace(settings.DynamicForward) != ""
}

func forwardLines(settings sshconfig.AdvancedSettings) []string {
	lines := make([]string, 0, len(settings.LocalForwards)+len(settings.RemoteForwards)+1)
	for _, forward := range settings.LocalForwards {
		local, target, _ := strings.Cut(strings.TrimSpace(forward), " ")
		if target == "" {
			lines = append(lines, forward)
			continue
		}
		lines = append(lines, local+" → "+target)
	}
	for _, forward := range settings.RemoteForwards {
		remote, target, _ := strings.Cut(strings.TrimSpace(forward), " ")
		if target == "" {
			lines = append(lines, "remote "+forward)
			continue
		}
		lines = append(lines, "remote "+remote+" → "+target)
	}
	if port := strings.TrimSpace(settings.DynamicForward); port != "" {
		lines = append(lines, "socks "+port)
	}
	return lines
}

func (m *App) forwardSummary(alias string) string {
	host, ok := m.findHost(alias)
	if !ok {
		return alias
	}
	lines := forwardLines(m.hostForwardSettings(host))
	if len(lines) == 0 {
		return m.hostLabel(host)
	}
	return m.hostLabel(host) + "  " + strings.Join(lines, " · ")
}

func (m *App) forwardSection() []hostRow {
	aliases := m.activeForwardAliases()
	if len(aliases) == 0 {
		return nil
	}
	rows := make([]hostRow, 0, len(aliases)+1)
	rows = append(rows, hostRow{forwardHeader: true, label: "Forwards"})
	for _, alias := range aliases {
		rows = append(rows, hostRow{forwardAlias: alias, label: m.forwardSummary(alias)})
	}
	return rows
}

func (m *App) forwardHeaderSelected() bool {
	rows := m.hostListRows()
	return m.cursor >= 0 && m.cursor < len(rows) && rows[m.cursor].forwardHeader
}

func (m *App) selectedForward() (string, bool) {
	rows := m.hostListRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return "", false
	}
	alias := rows[m.cursor].forwardAlias
	return alias, alias != ""
}

func (m *App) startSelectedForward() (tea.Model, tea.Cmd) {
	host, ok := m.selectedHost()
	if !ok {
		return m, nil
	}
	if host.SyncSource == "vercel" {
		m.setError(errVercelForward)
		return m, nil
	}
	if hostlife.LooksStopped(host, m.hostMetadata()[host.Alias]) {
		return m, m.setNotice("Start " + m.hostLabel(host) + " first")
	}
	if !m.hostHasForwards(host) {
		return m, m.setNotice(m.hostLabel(host) + " has no forwards")
	}
	if _, err := m.tunnelSupervisor(); err != nil {
		m.setError(err)
		return m, nil
	}
	label := m.hostLabel(host)
	m.status, m.statusError = "Opening forwards for "+label, false
	return m, m.startForwardCmd(host, label)
}

var errVercelForward = vercelForwardError{}

type vercelForwardError struct{}

func (vercelForwardError) Error() string {
	return "Vercel sandboxes can't tunnel forwards"
}

func (m *App) startForwardCmd(host sshconfig.Host, label string) tea.Cmd {
	exe := m.bastExecutable()
	passwords := m.paths.PasswordsDir
	timeout := prepareTimeoutForHost(host)
	alias := host.Alias
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := m.ensureSyncedAccess(ctx, host, func(string) {}); err != nil {
			return entryOpenedMsg{err: err}
		}
		_, err := m.tunnelSup.Config(alias, func(cmd *exec.Cmd) {
			askpass.Prepare(cmd, exe, host, passwords)
		})
		if err != nil {
			return entryOpenedMsg{err: err}
		}
		return entryOpenedMsg{notice: "Forwarding " + label}
	}
}

func (m *App) closeSelectedForward() (tea.Model, tea.Cmd) {
	alias := ""
	if selected, ok := m.selectedForward(); ok {
		alias = selected
	} else if host, ok := m.selectedHost(); ok && m.forwardOpen(host.Alias) {
		alias = host.Alias
	}
	if alias == "" || m.tunnelSup == nil {
		return m, nil
	}
	m.tunnelSup.CloseConfig(alias)
	label := alias
	if host, ok := m.findHost(alias); ok {
		label = m.hostLabel(host)
	}
	return m, m.setNotice("Closed forwards for " + label)
}

func (m *App) renderForwardHeader(s styleSet, rowWidth int, selected bool) string {
	line := " " + s.muted.Render("── Forwards")
	if selected {
		return s.selected.Width(rowWidth).Render(line)
	}
	return s.muted.Width(rowWidth).Render(line)
}

func (m *App) renderForwardRow(s styleSet, row hostRow, selected bool, rowWidth int) string {
	line := "  " + truncate(row.label, max(2, rowWidth-3))
	if selected {
		return s.selected.Width(rowWidth).Render(line)
	}
	return s.plain.Width(rowWidth).Render(line)
}

func (m *App) renderForwardDetail(s styleSet, alias string, width int) string {
	host, ok := m.findHost(alias)
	label := alias
	var lines []string
	if ok {
		label = m.hostLabel(host)
		lines = forwardLines(m.hostForwardSettings(host))
	}
	var b strings.Builder
	b.WriteString("  " + s.active.Render(truncate(label, max(4, width-3))) + "\n")
	b.WriteString("\n  " + s.title.Render(closeTunnelAction) + "\n\n")
	b.WriteString("  " + s.muted.Render("Forwarding") + "\n")
	if len(lines) == 0 {
		b.WriteString(compactRow(s, "Tunnel", "ssh -N", width))
		return b.String()
	}
	for _, line := range lines {
		b.WriteString("  " + s.value.Render(truncate(line, max(4, width-3))) + "\n")
	}
	return b.String()
}

func (m *App) forwardButtonBounds(layout panelLayout) (x, y, width int, ok bool) {
	host, selected := m.selectedHost()
	if !selected || m.forwardOpen(host.Alias) || !m.hostHasForwards(host) {
		return 0, 0, 0, false
	}
	primaryX, primaryY, primaryWidth := m.hostActionButtonBounds(layout, m.hostPrimaryAction(host))
	extra := 0
	if m.hostMetadata()[host.Alias].RDP {
		extra = lipgloss.Width(m.styles().title.Render(rdpAction)) + 1
	}
	width = lipgloss.Width(m.styles().title.Render(forwardAction))
	return primaryX + primaryWidth + 1 + extra, primaryY, width, true
}

func (m *App) forwardCloseBounds(layout panelLayout) (x, y, width int, ok bool) {
	if _, selected := m.selectedForward(); selected {
		x, y, width = m.hostActionButtonBounds(layout, closeTunnelAction)
		return x, y, width, true
	}
	host, selected := m.selectedHost()
	if !selected || !m.forwardOpen(host.Alias) {
		return 0, 0, 0, false
	}
	primaryX, primaryY, primaryWidth := m.hostActionButtonBounds(layout, m.hostPrimaryAction(host))
	extra := 0
	if m.hostMetadata()[host.Alias].RDP {
		extra = lipgloss.Width(m.styles().title.Render(rdpAction)) + 1
	}
	width = lipgloss.Width(m.styles().title.Render(closeTunnelAction))
	return primaryX + primaryWidth + 1 + extra, primaryY, width, true
}
