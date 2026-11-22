package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/hostlife"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
	"bast/internal/tunnel"
)

const forwardAction = " Forward "

func (m *App) hostOpenWithHostTunnels(alias string) []metadata.Tunnel {
	if m.metadata == nil || alias == "" {
		return nil
	}
	var out []metadata.Tunnel
	for _, t := range m.metadata.Tunnels() {
		if t.Host == alias && t.OpenWithHost {
			out = append(out, t)
		}
	}
	return out
}

// hostHasForwards reports an open-with-host tunnel that is not running.
// A missing runner counts as not running, so Forward stays available.
func (m *App) hostHasForwards(host sshconfig.Host) bool {
	for _, t := range m.hostOpenWithHostTunnels(host.Alias) {
		if m.tunnelMgr == nil || !m.tunnelMgr.Running(t.Name) {
			return true
		}
	}
	return false
}

// forwardOpen reports an open-with-host tunnel that is running.
func (m *App) forwardOpen(alias string) bool {
	if m.tunnelMgr == nil {
		return false
	}
	for _, t := range m.hostOpenWithHostTunnels(alias) {
		if m.tunnelMgr.Running(t.Name) {
			return true
		}
	}
	return false
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
	if m.tunnelMgr == nil {
		err := m.tunnelMgrErr
		if err == nil {
			err = fmt.Errorf("tunnel runner is unavailable")
		}
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
	tunnels := m.hostOpenWithHostTunnels(host.Alias)
	mgr := m.tunnelMgr
	timeout := prepareTimeoutForHost(host)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := m.ensureSyncedAccess(ctx, host, func(string) {}); err != nil {
			return entryOpenedMsg{err: err}
		}
		if mgr == nil {
			return entryOpenedMsg{err: fmt.Errorf("tunnel runner is unavailable")}
		}
		var failed []string
		started := 0
		for _, t := range tunnels {
			if mgr.Running(t.Name) {
				continue
			}
			if err := mgr.Start(tunnelSpec(t)); err != nil && err != tunnel.ErrRunning {
				failed = append(failed, err.Error())
				continue
			}
			started++
		}
		if len(failed) > 0 {
			return entryOpenedMsg{err: fmt.Errorf("%s", strings.Join(failed, "\n"))}
		}
		if started == 0 {
			return entryOpenedMsg{notice: label + " forwards are already open"}
		}
		return entryOpenedMsg{notice: "Forwarding " + label}
	}
}

func (m *App) closeSelectedForward() (tea.Model, tea.Cmd) {
	host, ok := m.selectedHost()
	if !ok || m.tunnelMgr == nil {
		return m, nil
	}
	closed := 0
	for _, t := range m.hostOpenWithHostTunnels(host.Alias) {
		if m.tunnelMgr.Running(t.Name) {
			m.tunnelMgr.Stop(t.Name)
			closed++
		}
	}
	if closed == 0 {
		return m, nil
	}
	return m, m.setNotice("Closed forwards for " + m.hostLabel(host))
}

func (m *App) forwardButtonBounds(layout panelLayout) (x, y, width int, ok bool) {
	host, selected := m.selectedHost()
	if !selected || !m.hostHasForwards(host) {
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
	host, selected := m.selectedHost()
	if !selected || !m.forwardOpen(host.Alias) {
		return 0, 0, 0, false
	}
	primaryX, primaryY, primaryWidth := m.hostActionButtonBounds(layout, m.hostPrimaryAction(host))
	extra := 0
	if m.hostMetadata()[host.Alias].RDP {
		extra = lipgloss.Width(m.styles().title.Render(rdpAction)) + 1
	}
	if m.hostHasForwards(host) {
		extra += lipgloss.Width(m.styles().title.Render(forwardAction)) + 1
	}
	width = lipgloss.Width(m.styles().title.Render(closeTunnelAction))
	return primaryX + primaryWidth + 1 + extra, primaryY, width, true
}
