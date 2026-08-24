package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	flycloud "bast/internal/cloud/fly"
	"bast/internal/sshconfig"
)

func (m *App) resumeSelectedFly(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["fly"] {
		return m.setNotice("Fly operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Fly Machine is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("Fly sync id missing; sync Fly first")
	}
	opGen := m.beginProviderOp("fly")
	m.syncActivity = "starting…"
	if thenConnect {
		m.boxConnectAfter = host.Alias
	} else {
		m.boxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		result, err := m.syncer.ResumeFly(ctx, host.SyncID)
		return syncDoneMsg{provider: "fly", result: result, err: err, opGen: opGen}
	}
}

func (m *App) flyAppOptions() []fieldOption {
	seen := map[string]bool{}
	var options []fieldOption
	for _, host := range m.hosts {
		if !host.Synced || host.SyncSource != "fly" {
			continue
		}
		_, app, _, err := flycloud.ParseSyncID(host.SyncID)
		if err != nil || app == "" || seen[app] {
			continue
		}
		seen[app] = true
		options = append(options, fieldOption{label: app, value: app})
	}
	return options
}

func (m *App) openFlyNewForm() {
	appField := field{label: "App", description: "Existing Fly app", placeholder: "my-app"}
	if apps := m.flyAppOptions(); len(apps) > 0 {
		appField.options = apps
		appField.value = apps[0].value
		appField.selected = 0
	}
	m.openForm("New Fly Machine", "fly_new", []field{
		appField,
		{label: "Image", description: "Docker image", placeholder: "nginx", value: "nginx"},
		{label: "Org", description: "Organization slug", optional: true, placeholder: "personal", value: strings.Join(m.metadata.Fly().OrgFilter, ", ")},
		{label: "Region", description: "Optional region code", optional: true, placeholder: "iad"},
		{
			label: "Size", value: "shared-cpu-1x", selected: 0,
			options: []fieldOption{
				{label: "shared-cpu-1x", value: "shared-cpu-1x"},
				{label: "shared-cpu-2x", value: "shared-cpu-2x"},
				{label: "performance-1x", value: "performance-1x"},
				{label: "performance-2x", value: "performance-2x"},
			},
		},
		{label: "Name", description: "Optional machine name", optional: true},
	})
}

func (m *App) openFlyStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("fly machine is already stopped"))
		return
	}
	m.openForm("Stop Fly Machine", "fly_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Stops compute; ephemeral disks are lost unless a volume is mounted", placeholder: "stop"},
	})
}

func (m *App) openFlyForkForm(host sshconfig.Host) {
	m.openForm("Clone Fly Machine", "fly_fork", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Region", description: "Leave blank to clone in the same region", optional: true, placeholder: "iad"},
		{label: "Type fork to confirm", description: "Clones config and image, not volumes", placeholder: "fork"},
	})
}

func (m *App) openFlyDeleteForm(host sshconfig.Host) {
	label := m.hostLabel(host)
	m.openForm("Destroy Fly Machine: "+label, "fly_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently destroys this Machine, not the Fly app", placeholder: "delete"},
	})
}
