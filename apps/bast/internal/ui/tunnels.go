package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/metadata"
	"bast/internal/tunnel"
)

const (
	tunnelSectionBasics = "Basics"
	tunnelSectionMore   = "Options"
)

type tunnelTickMsg struct{}

type tunnelResultMsg struct {
	notice string
	err    error
}

func (m *App) tunnelList() []metadata.Tunnel { return m.metadata.Tunnels() }

func (m *App) selectedTunnel() (metadata.Tunnel, bool) {
	list := m.tunnelList()
	if m.section != tunnelsSection || m.cursor < 0 || m.cursor >= len(list) {
		return metadata.Tunnel{}, false
	}
	return list[m.cursor], true
}

func tunnelSelected(m *App) bool {
	_, ok := m.selectedTunnel()
	return ok
}

func tunnelToggleLabel(m *App) string {
	if t, ok := m.selectedTunnel(); ok && m.tunnelStatus(t.Name).State != tunnel.StateConfigured && m.tunnelMgr != nil && m.tunnelMgr.Running(t.Name) {
		return "Close"
	}
	return "Open"
}

func (m *App) tunnelStatus(name string) tunnel.Status {
	if m.tunnelMgr == nil {
		return tunnel.Status{Name: name, State: tunnel.StateConfigured}
	}
	return m.tunnelMgr.Status(name)
}

func (m *App) enterTunnelsSection() tea.Cmd {
	m.section, m.cursor, m.search = tunnelsSection, 0, ""
	if m.tunnelTicking {
		return nil
	}
	m.tunnelTicking = true
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tunnelTickMsg{} })
}

// tunnelTick redraws state while the tab is open; the runners change state
// on their own, so the screen would otherwise go stale.
func (m *App) tunnelTick() tea.Cmd {
	if m.section != tunnelsSection {
		m.tunnelTicking = false
		return nil
	}
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tunnelTickMsg{} })
}

func tunnelSpec(t metadata.Tunnel) tunnel.Spec {
	return tunnel.Spec{Name: t.Name, Host: t.Host, Mode: t.Mode, Local: t.Local, Remote: t.Remote, Group: t.Group, KeepAlive: t.KeepAlive}
}

func (m *App) toggleSelectedTunnel() tea.Cmd {
	t, ok := m.selectedTunnel()
	if !ok {
		return nil
	}
	if m.tunnelMgr == nil {
		err := m.tunnelMgrErr
		if err == nil {
			err = fmt.Errorf("tunnel runner is unavailable")
		}
		return func() tea.Msg { return tunnelResultMsg{err: err} }
	}
	mgr := m.tunnelMgr
	return func() tea.Msg {
		if mgr.Running(t.Name) {
			mgr.Stop(t.Name)
			return tunnelResultMsg{notice: "Closed " + t.Name}
		}
		if err := mgr.Start(tunnelSpec(t)); err != nil && err != tunnel.ErrRunning {
			return tunnelResultMsg{err: err}
		}
		return tunnelResultMsg{notice: "Opened " + t.Name}
	}
}

func (m *App) showTunnelLog() tea.Cmd {
	t, ok := m.selectedTunnel()
	if !ok {
		return nil
	}
	if m.tunnelMgr == nil {
		msg := "tunnel runner is unavailable"
		if m.tunnelMgrErr != nil {
			msg = m.tunnelMgrErr.Error()
		}
		return m.setNotice(msg)
	}
	body, err := os.ReadFile(m.tunnelMgr.LogPath(t.Name))
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if err != nil || len(lines) == 0 || lines[len(lines)-1] == "" {
		return m.setNotice("No log yet for " + t.Name)
	}
	return m.setNotice(lines[len(lines)-1])
}

func tunnelGlyph(state string) string {
	switch state {
	case tunnel.StateConnected:
		return "●"
	case tunnel.StateConnecting, tunnel.StateReconnecting:
		return "◐"
	case tunnel.StateError:
		return "✕"
	default:
		return "○"
	}
}

func (m *App) renderTunnels(s styleSet) string {
	list := m.tunnelList()
	if len(list) == 0 {
		lines := []string{"Press a to add a local, remote, or SOCKS forward through a host."}
		if m.tunnelMgrErr != nil {
			lines = append(lines, m.tunnelMgrErr.Error())
		}
		return m.renderSeal(s, "◇  No tunnels yet", lines, nil)
	}
	m.cursor = min(m.cursor, len(list)-1)
	listWidth, detailWidth, bodyHeight := m.columnDimensions()
	layout := m.panelLayout()
	listHeight := layout.listHeight
	rowWidth := listWidth
	start := scrollStart(m.cursor, len(list), listHeight)
	selectedRow := s.selected.Width(rowWidth)
	plainRow := s.plain.Width(rowWidth)
	var rows strings.Builder
	for i := start; i < min(len(list), start+listHeight); i++ {
		t := list[i]
		line := tunnelGlyph(m.tunnelStatus(t.Name).State) + " " + truncate(t.Name, rowWidth-4)
		if i == m.cursor {
			line = selectedRow.Render(line)
		} else {
			line = plainRow.Render(line)
		}
		rows.WriteString(line + "\n")
	}
	listPanel := lipgloss.NewStyle().Width(rowWidth).Height(listHeight).Render(strings.TrimRight(rows.String(), "\n"))
	detail := lipgloss.NewStyle().Width(detailWidth).Height(layout.detailHeight).Render(m.renderTunnelDetail(s, list[m.cursor], detailWidth))
	if layout.mobile {
		divider := s.rule.Render(strings.Repeat("─", listWidth))
		return lipgloss.JoinVertical(lipgloss.Left, listPanel, divider, detail)
	}
	divider := s.rule.Render(strings.TrimSuffix(strings.Repeat("│\n", bodyHeight), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, divider, detail)
}

func (m *App) renderTunnelDetail(s styleSet, t metadata.Tunnel, width int) string {
	st := m.tunnelStatus(t.Name)
	var b strings.Builder
	b.WriteString("\n  " + s.title.Render(" "+t.Name+" ") + "\n\n")
	row := func(label, value string) {
		if value != "" {
			b.WriteString(compactRow(s, label, value, width))
		}
	}
	row("State", st.State)
	if m.tunnelMgrErr != nil {
		row("Runner", m.tunnelMgrErr.Error())
	}
	if st.State == tunnel.StateReconnecting || st.State == tunnel.StateError {
		row("Problem", string(st.Kind)+": "+st.Error)
	}
	if st.Attempts > 1 {
		row("Attempts", strconv.Itoa(st.Attempts))
	}
	row("Host", t.Host)
	row("Mode", t.Mode)
	row("Local", t.Local)
	row("Remote", t.Remote)
	row("Group", t.Group)
	if t.KeepAlive > 0 {
		row("Keep-alive", fmt.Sprintf("%ds", t.KeepAlive))
	}
	return b.String()
}

func (m *App) tunnelHostField(current string) field {
	item := field{label: "Host", section: tunnelSectionBasics, description: "SSH host to tunnel through; user, port and key come from its config"}
	for _, host := range m.hosts {
		label := m.hostLabel(host)
		if label != host.Alias {
			label += " · " + host.Alias
		}
		item.options = append(item.options, fieldOption{label: label, value: host.Alias})
		if host.Alias == current {
			item.selected = len(item.options) - 1
			item.value = host.Alias
		}
	}
	if current != "" && item.value == "" {
		item.options = append(item.options, fieldOption{label: current, value: current})
		item.selected = len(item.options) - 1
		item.value = current
	} else if item.value == "" && len(item.options) > 0 {
		item.selected = 0
		item.value = item.options[0].value
	}
	return item
}

func (m *App) openTunnelForm(t metadata.Tunnel) {
	title := "Add tunnel"
	if t.Name != "" {
		title = "Edit " + t.Name
	}
	mode := field{label: "Mode", section: tunnelSectionBasics, description: "local -L, remote -R, socks -D, socks-remote -R"}
	for i, name := range []string{tunnel.ModeLocal, tunnel.ModeRemote, tunnel.ModeSOCKS, tunnel.ModeSOCKSRemote} {
		mode.options = append(mode.options, fieldOption{label: name, value: name})
		if name == t.Mode {
			mode.selected = i
		}
	}
	mode.value = mode.options[mode.selected].value
	keep := ""
	if t.KeepAlive > 0 {
		keep = strconv.Itoa(t.KeepAlive)
	}
	m.openForm(title, "tunnel_save", []field{
		{label: "Original name", value: t.Name, hidden: true},
		{label: "Name", section: tunnelSectionBasics, description: "Unique name; globs like prod-* select several", value: t.Name},
		m.tunnelHostField(t.Host),
		mode,
		{label: "Local", section: tunnelSectionBasics, description: "Listen port (8080, 8000-8003) or socket; for remote: the local target", value: t.Local, optional: true},
		{label: "Remote", section: tunnelSectionBasics, description: "host:port or socket; for remote modes: the server's listen port", value: t.Remote, optional: true},
		{label: "Group", section: tunnelSectionMore, description: "Optional - group for open/close --group", value: t.Group, optional: true},
		{label: "Keep-alive", section: tunnelSectionMore, description: "Optional - ServerAliveInterval seconds; blank uses 30", value: keep, optional: true},
	})
	if m.form != nil {
		m.form.revealed = len(m.form.fields) - 1
	}
}

func (m *App) submitTunnelSave(values map[string]string) (tea.Model, tea.Cmd) {
	keep := 0
	if v := strings.TrimSpace(values["Keep-alive"]); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return m.formValidationError("keep-alive must be a number of seconds")
		}
		keep = n
	}
	t := metadata.Tunnel{
		Name: strings.TrimSpace(values["Name"]), Host: values["Host"], Mode: values["Mode"],
		Local: strings.TrimSpace(values["Local"]), Remote: strings.TrimSpace(values["Remote"]),
		Group: strings.TrimSpace(values["Group"]), KeepAlive: keep,
	}
	if err := tunnel.Validate(tunnelSpec(t)); err != nil {
		return m.formValidationError(err.Error())
	}
	orig := values["Original name"]
	if err := m.metadata.PutTunnel(t, orig); err != nil {
		return m.formValidationError(err.Error())
	}
	if orig != "" && orig != t.Name && m.tunnelMgr != nil {
		m.tunnelMgr.Stop(orig)
	}
	m.form = nil
	notice := "Tunnel saved"
	if m.tunnelMgr != nil && m.tunnelMgr.Running(t.Name) {
		notice += "; close and open it to apply"
	}
	return m, m.setNotice(notice)
}

func (m *App) openTunnelDeleteForm() {
	t, ok := m.selectedTunnel()
	if !ok {
		return
	}
	m.openForm("Delete "+t.Name, "tunnel_delete", []field{
		{label: "Name", value: t.Name, hidden: true},
		{label: "Confirmation", value: t.Name, hidden: true},
		{label: "Type the name to confirm", placeholder: t.Name},
	})
}

func (m *App) submitTunnelDelete(values map[string]string) (tea.Model, tea.Cmd) {
	if values["Type the name to confirm"] != values["Confirmation"] {
		return m.formValidationError("Name does not match")
	}
	if m.tunnelMgr != nil {
		m.tunnelMgr.Stop(values["Name"])
	}
	if err := m.metadata.DeleteTunnel(values["Name"]); err != nil {
		return m.formError(err.Error())
	}
	m.form = nil
	m.cursor = max(0, m.cursor-1)
	return m, m.setNotice("Tunnel deleted")
}

func (m *App) openAddTunnelForm() { m.openTunnelForm(metadata.Tunnel{}) }
