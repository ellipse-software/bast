package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	railwaycloud "bast/internal/cloud/railway"
	"bast/internal/sshconfig"
)

func (m *App) railwayHasToken() bool {
	if m.syncStatus.Railway.HasToken {
		return true
	}
	if m.syncer != nil && m.syncer.Railway != nil {
		return m.syncer.Railway.HasToken()
	}
	return false
}

func (m *App) resumeSelectedRailway(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["railway"] {
		return m.setNotice("Railway operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Railway service is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("Railway sync id missing; sync first")
	}
	opGen := m.beginProviderOp("railway")
	m.syncActivity = "resuming…"
	if thenConnect {
		m.boxConnectAfter = host.Alias
	} else {
		m.boxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		result, err := m.syncer.ResumeRailway(ctx, host.SyncID)
		return syncDoneMsg{provider: "railway", result: result, err: err, opGen: opGen}
	}
}

func (m *App) openRailwayNewForm() {
	m.openForm("New Railway service", "railway_new", []field{
		{label: "Name", placeholder: "shell"},
		{label: "Project", description: "Existing project id or name", optional: true, placeholder: "my-app"},
		{
			label: "New project", description: "Create a project instead of using an existing one", optional: true,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
		{label: "New project name", optional: true, placeholder: "bast-sandbox"},
		{label: "Image", value: railwaycloud.DefaultImage, placeholder: railwaycloud.DefaultImage},
		{label: "Start command", value: railwaycloud.DefaultStart, optional: true, placeholder: railwaycloud.DefaultStart},
	})
}

func (m *App) openRailwayKeyForm() {
	m.openForm("Railway API token", "railway_key", []field{
		{label: "API token", description: "Account or workspace token from railway.com/account/tokens", secret: true, placeholder: "…"},
	})
}

func (m *App) openRailwayStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("railway service is already stopped"))
		return
	}
	m.openForm("Stop Railway service", "railway_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Stops the current deployment", placeholder: "stop"},
	})
}

func (m *App) openRailwayDeleteForm(host sshconfig.Host) {
	label := m.hostLabel(host)
	m.openForm("Delete Railway service: "+label, "railway_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently destroys the service and its deployments", placeholder: "delete"},
	})
}
