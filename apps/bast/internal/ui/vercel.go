package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	vercelcloud "bast/internal/cloud/vercel"
	"bast/internal/sshconfig"
)

func (m *App) vercelHasToken() bool {
	if m.syncStatus.Vercel.HasToken {
		return true
	}
	if m.syncer != nil && m.syncer.Vercel != nil {
		return m.syncer.Vercel.HasToken()
	}
	return false
}

func (m *App) vercelReady() bool {
	if !m.vercelHasToken() {
		return false
	}
	integration := m.metadata.Vercel()
	return strings.TrimSpace(integration.TeamID) != "" && strings.TrimSpace(integration.ProjectID) != ""
}

func (m *App) resumeSelectedVercel(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["vercel"] {
		return m.setNotice("Vercel operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Vercel sandbox is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("Vercel sync id missing; sync first")
	}
	opGen := m.beginProviderOp("vercel")
	m.syncActivity = "resuming…"
	if thenConnect {
		m.boxConnectAfter = host.Alias
	} else {
		m.boxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		result, err := m.syncer.ResumeVercel(ctx, host.SyncID)
		return syncDoneMsg{provider: "vercel", result: result, err: err, opGen: opGen}
	}
}

func (m *App) openVercelNewForm() {
	m.openForm("New Vercel Sandbox", "vercel_new", []field{
		{label: "Name", description: "Optional URL-safe name", optional: true, placeholder: "dev"},
		{
			label: "vCPUs", value: "2", selected: 1,
			options: []fieldOption{
				{label: "1 vCPU / 2 GB", value: "1"},
				{label: "2 vCPU / 4 GB", value: "2"},
				{label: "4 vCPU / 8 GB", value: "4"},
			},
		},
		{
			label: "Timeout", value: "1h", selected: 1,
			options: []fieldOption{
				{label: "15 minutes", value: "15m"},
				{label: "1 hour", value: "1h"},
				{label: "5 hours", value: "5h"},
			},
		},
		{
			label: "Persistent", description: "Snapshot filesystem on stop", optional: true, value: "yes", selected: 1,
			options: []fieldOption{
				{label: "No", value: ""},
				{label: "Yes", value: "yes"},
			},
		},
	})
}

func (m *App) openVercelTokenForm() {
	integration := m.metadata.Vercel()
	m.openForm("Vercel access token", "vercel_token", []field{
		{label: "Access token", description: "https://vercel.com/account/settings/tokens", secret: true, placeholder: "token"},
		{label: "Team ID", value: integration.TeamID, placeholder: "team_…"},
		{label: "Project ID", value: integration.ProjectID, placeholder: "prj_…"},
	})
}

func (m *App) openVercelStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("vercel sandbox is already stopped"))
		return
	}
	m.openForm("Stop Vercel Sandbox", "vercel_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Snapshots the filesystem and stops compute", placeholder: "stop"},
	})
}

func (m *App) openVercelForkForm(host sshconfig.Host) {
	m.openForm("Fork Vercel Sandbox", "vercel_fork", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type fork to confirm", description: "Creates a new sandbox from this one's snapshot", placeholder: "fork"},
	})
}

func (m *App) openVercelDeleteForm(host sshconfig.Host) {
	label := m.hostLabel(host)
	m.openForm("Delete Vercel Sandbox: "+label, "vercel_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently destroys the sandbox; snapshots are kept", placeholder: "delete"},
	})
}

func (m *App) openVercelCleanupForm() {
	names := m.metadata.Vercel().Unrestorable
	desc := "Deletes offline sandboxes with no snapshot"
	if len(names) > 0 {
		list := strings.Join(names, ", ")
		if len(list) > 80 {
			list = fmt.Sprintf("%d sandboxes", len(names))
		}
		desc = list + " · no snapshot, cannot resume"
	}
	m.openForm("Cleanup Vercel sandboxes", "vercel_cleanup", []field{
		{label: "Type cleanup to confirm", description: desc, placeholder: "cleanup"},
	})
}

func (m *App) vercelShellCmd(host sshconfig.Host) (*exec.Cmd, error) {
	integration := m.metadata.Vercel()
	projectID, name, err := vercelcloud.ScopedName(host.SyncID, integration.ProjectID)
	if err != nil {
		return nil, err
	}
	exe := ""
	if m.syncer != nil {
		exe = m.syncer.BastExecutable
	}
	return vercelcloud.ShellCommand(exe, name, projectID, integration.TeamID), nil
}
