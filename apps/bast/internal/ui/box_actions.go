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
	opGen := m.beginProviderOp("box")
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
		return syncDoneMsg{provider: "box", result: result, err: err, opGen: opGen}
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

func (m *App) openBoxNewForm() {
	m.openForm("New Box", "box_new", []field{
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
			description: "Isolated no-env box",
			optional:    true,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
	})
}

func (m *App) openBoxStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("box is already stopped"))
		return
	}
	m.openForm("Stop Box", "box_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Snapshots the box and pauses billing", value: "", optional: false, placeholder: "stop"},
	})
}

func (m *App) openBoxDeleteForm(host sshconfig.Host) {
	label := m.hostLabel(host)
	m.openForm("Delete Box: "+label, "box_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently deletes the box and its snapshots", placeholder: "delete"},
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
