package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	boatcloud "bast/internal/cloud/boat"
	"bast/internal/sshconfig"
)

func (m *App) resumeSelectedBoat(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["boat"] {
		return m.setNotice("Boat operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Sandbox is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("Boat sync id missing; sync Boat first")
	}
	opGen := m.beginProviderOp("boat")
	m.syncActivity = "resuming…"
	if thenConnect {
		m.sandboxConnectAfter = host.Alias
	} else {
		m.sandboxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.ResumeBoat(ctx, host.SyncID, boatcloud.ResumeOpts{})
		return syncDoneMsg{provider: "boat", result: result, err: err, opGen: opGen}
	}
}

// connectAfterSandboxResume SSHes into a sandbox that was resumed via Enter/Resume.
// Called after hosts reload so the refreshed IP/auth are used.
func (m *App) connectAfterSandboxResume() tea.Cmd {
	alias := m.sandboxConnectAfter
	if alias == "" {
		return nil
	}
	m.sandboxConnectAfter = ""
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
		return m.setNotice("Resumed sandbox not found in hosts")
	}
	if m.hostLooksStopped(host) {
		return m.setNotice("Sandbox is still stopped after resume")
	}
	_, cmd := m.connectSelected()
	return cmd
}

func (m *App) openBoatNewForm() {
	m.openForm("New sandbox", "boat_new", []field{
		{
			label:       "Type",
			description: "small, default, or large",
			value:       "default",
			selected:    1,
			options: []fieldOption{
				{label: "small", value: "small"},
				{label: "default", value: "default"},
				{label: "large", value: "large"},
			},
		},
		{
			label:       "No auto-stop",
			description: "Keep running until stopped",
			optional:    true,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
		{
			label:       "No env",
			description: "Isolated no-env sandbox",
			optional:    true,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
	})
}

func (m *App) openBoatStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("sandbox is already stopped"))
		return
	}
	m.openForm("Stop sandbox", "boat_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Snapshots the sandbox and pauses billing", value: "", optional: false, placeholder: "stop"},
	})
}

func (m *App) openBoatDeleteForm(host sshconfig.Host) {
	label := m.hostLabel(host)
	m.openForm("Delete sandbox: "+label, "boat_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently deletes the sandbox and its snapshots", placeholder: "delete"},
	})
}

func (m *App) openBoatForkForm(host sshconfig.Host) {
	meta := m.metadata.Host(host.Alias)
	if !boatcloud.SnapshotAvailableFromTags(meta.Tags) {
		m.setError(errString("sandbox has no snapshot yet; stop it once before forking"))
		return
	}
	m.openForm("Fork sandbox", "boat_fork", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type fork to confirm", description: "Clones from the latest snapshot into a new sandbox", value: "", optional: false, placeholder: "fork"},
	})
}

type errString string

func (e errString) Error() string { return string(e) }
