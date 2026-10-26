package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/sshconfig"
)

func (m *App) bitqateHasToken() bool {
	if m.syncStatus.Bitqate.HasToken {
		return true
	}
	if m.syncer != nil && m.syncer.Bitqate != nil {
		return m.syncer.Bitqate.HasToken()
	}
	return false
}

func (m *App) startSelectedBitqate(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["bitqate"] {
		return m.setNotice("BitQate operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("BitQate VM is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("BitQate sync id missing; sync first")
	}
	opGen := m.beginProviderOp("bitqate")
	m.syncActivity = "starting…"
	if thenConnect {
		m.sandboxConnectAfter = host.Alias
	} else {
		m.sandboxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.StartBitqate(ctx, host.SyncID)
		return syncDoneMsg{provider: "bitqate", result: result, err: err, opGen: opGen}
	}
}

func (m *App) openBitqateKeyForm() {
	m.openForm("Add BitQate API token", "bitqate_key", []field{
		{label: "API token", description: "BitQate dashboard → Settings → API; scopes vms:read (plus vms:power for start/stop/restart)", secret: true, placeholder: "bqat_…"},
	})
}

func (m *App) openBitqateStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("BitQate VM is already stopped"))
		return
	}
	m.openForm("Stop BitQate VM", "bitqate_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Powers the VM off; billing continues while stopped", placeholder: "stop"},
	})
}

func (m *App) openBitqateRestartForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("start the VM before restarting"))
		return
	}
	m.openForm("Restart BitQate VM", "bitqate_restart", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type restart to confirm", description: "Restarts the VM", placeholder: "restart"},
	})
}
