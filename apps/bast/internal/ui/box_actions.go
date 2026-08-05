package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	boxcloud "bast/internal/cloud/box"
	"bast/internal/sshconfig"
)

func (m *App) resumeSelectedBox(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["box"] {
		return m.setNotice("Box operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Box is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("Box sync id missing; sync Box first")
	}
	m.syncingProviders["box"] = true
	m.syncActivity = "resuming…"
	if thenConnect {
		m.boxConnectAfter = host.Alias
	} else {
		m.boxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.ResumeBox(ctx, host.SyncID, boxcloud.ResumeOpts{})
		return syncDoneMsg{provider: "box", result: result, err: err}
	}
}

// connectAfterBoxResume SSHes into a box that was resumed via Enter/Resume.
// Called after hosts reload so the refreshed IP/auth are used.
func (m *App) connectAfterBoxResume() tea.Cmd {
	alias := m.boxConnectAfter
	if alias == "" {
		return nil
	}
	m.boxConnectAfter = ""
	host, ok := m.selectedHost()
	if !ok || host.Alias != alias {
		for i, row := range m.hostRows() {
			if !row.header && row.host.Alias == alias {
				m.cursor = i
				host, ok = row.host, true
				break
			}
		}
	}
	if !ok {
		return m.setNotice("Resumed box not found in hosts")
	}
	if m.hostLooksStopped(host) {
		return m.setNotice("Box is still stopped after resume")
	}
	_, cmd := m.connectSelected()
	return cmd
}

func (m *App) openBoxStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.status, m.statusError = "Box is already stopped", true
		return
	}
	m.openForm("Stop Box", "box_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Snapshots the box and pauses billing", value: "", optional: false, placeholder: "stop"},
	})
}

func (m *App) openBoxForkForm(host sshconfig.Host) {
	meta := m.metadata.Host(host.Alias)
	if !boxcloud.SnapshotAvailableFromTags(meta.Tags) {
		m.setError(errString("box has no snapshot yet; stop it once before forking"))
		return
	}
	m.openForm("Fork Box", "box_fork", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type fork to confirm", description: "Clones from the latest snapshot into a new box", value: "", optional: false, placeholder: "fork"},
	})
}

type errString string

func (e errString) Error() string { return string(e) }
