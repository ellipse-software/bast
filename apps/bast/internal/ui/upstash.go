package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	upstashcloud "bast/internal/cloud/upstash"
	"bast/internal/sshconfig"
)

func (m *App) upstashHasKey() bool {
	if m.syncStatus.Upstash.HasKey {
		return true
	}
	if m.syncer != nil && m.syncer.Upstash != nil {
		return m.syncer.Upstash.HasKey()
	}
	return false
}

func (m *App) resumeSelectedUpstash(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["upstash"] {
		return m.setNotice("Upstash operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Upstash box is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("Upstash sync id missing; sync first")
	}
	opGen := m.beginProviderOp("upstash")
	m.syncActivity = "resuming…"
	if thenConnect {
		m.boxConnectAfter = host.Alias
	} else {
		m.boxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		result, err := m.syncer.ResumeUpstash(ctx, host.SyncID)
		return syncDoneMsg{provider: "upstash", result: result, err: err, opGen: opGen}
	}
}

func (m *App) openUpstashNewForm() {
	m.openForm("New Upstash Box", "upstash_new", []field{
		{label: "Name", description: "Optional display name", optional: true, placeholder: "worker"},
		{
			label: "Runtime", value: "node", selected: 0,
			options: []fieldOption{
				{label: "node", value: "node"},
				{label: "python", value: "python"},
				{label: "golang", value: "golang"},
				{label: "ruby", value: "ruby"},
				{label: "rust", value: "rust"},
			},
		},
		{
			label: "Size", value: "small", selected: 0,
			options: []fieldOption{
				{label: "small (2 vCPU / 4 GB)", value: "small"},
				{label: "medium (4 vCPU / 8 GB)", value: "medium"},
				{label: "large (8 vCPU / 16 GB)", value: "large"},
			},
		},
		{
			label: "Keep alive", description: "Stay on between sessions; cannot pause", optional: true,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
	})
}

func (m *App) openUpstashKeyForm() {
	m.openForm("Upstash Box API key", "upstash_key", []field{
		{label: "API key", description: "https://console.upstash.com", secret: true, placeholder: "box_…"},
	})
}

func (m *App) openUpstashStopForm(host sshconfig.Host) {
	if upstashcloud.KeepAliveFromTags(m.metadata.Host(host.Alias).Tags) {
		m.setError(errString("keep-alive boxes cannot be paused"))
		return
	}
	if m.hostLooksStopped(host) {
		m.setError(errString("upstash box is already paused"))
		return
	}
	m.openForm("Pause Upstash Box", "upstash_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type pause to confirm", description: "Releases compute and preserves the filesystem", placeholder: "pause"},
	})
}

func (m *App) openUpstashForkForm(host sshconfig.Host) {
	m.openForm("Fork Upstash Box", "upstash_fork", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type fork to confirm", description: "Snapshots this box and creates a new one from it", placeholder: "fork"},
	})
}

func (m *App) openUpstashDeleteForm(host sshconfig.Host) {
	label := m.hostLabel(host)
	m.openForm("Delete Upstash Box: "+label, "upstash_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently destroys the live box; snapshots are kept", placeholder: "delete"},
	})
}

func (m *App) connectAfterUpstashResume() tea.Cmd {
	return m.connectAfterBoxResume()
}
