package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/sshconfig"
)

func (m *App) hetznerHasToken() bool {
	if m.syncStatus.Hetzner.HasToken {
		return true
	}
	if m.syncer != nil && m.syncer.Hetzner != nil {
		return m.syncer.Hetzner.HasToken()
	}
	return false
}

func (m *App) startSelectedHetzner(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["hetzner"] {
		return m.setNotice("Hetzner operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Hetzner server is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("Hetzner sync id missing; sync first")
	}
	opGen := m.beginProviderOp("hetzner")
	m.syncActivity = "starting…"
	if thenConnect {
		m.boxConnectAfter = host.Alias
	} else {
		m.boxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.StartHetzner(ctx, host.SyncID)
		return syncDoneMsg{provider: "hetzner", result: result, err: err, opGen: opGen}
	}
}

func (m *App) openHetznerKeyForm() {
	m.openForm("Add Hetzner API token", "hetzner_key", []field{
		{label: "Name", description: "Project name for this token; one token per Hetzner Cloud project", placeholder: "prod"},
		{label: "API token", description: "From Hetzner Console → Security → API Tokens", secret: true, placeholder: "token"},
	})
}

func (m *App) openHetznerKeyRemoveForm() {
	names := []string{}
	if m.syncer != nil && m.syncer.Hetzner != nil {
		names = m.syncer.Hetzner.StoredTokenNames()
	}
	if len(names) == 0 {
		m.setError(errString("no stored Hetzner tokens to remove"))
		return
	}
	options := make([]fieldOption, 0, len(names))
	for _, name := range names {
		options = append(options, fieldOption{label: name, value: name})
	}
	m.openForm("Remove Hetzner API token", "hetzner_key_remove", []field{
		{label: "Token", description: "Removes the stored token for this project", options: options},
	})
}

func (m *App) openHetznerStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("Hetzner server is already off"))
		return
	}
	m.openForm("Stop Hetzner server", "hetzner_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "ACPI shutdown. The server still bills while off", placeholder: "stop"},
		{
			label: "Force poweroff", description: "Hard poweroff if the guest ignores ACPI", optional: true,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
	})
}

func (m *App) openHetznerRestartForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("start the server before restarting"))
		return
	}
	m.openForm("Restart Hetzner server", "hetzner_restart", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type restart to confirm", description: "ACPI reboot. Type force for a hard reset", placeholder: "restart"},
		{
			label: "Force reset", description: "Hard reset if the guest ignores ACPI", optional: true,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
	})
}
