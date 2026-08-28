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
	return strings.TrimSpace(integration.TeamID) != "" && len(integration.Projects()) > 0
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
	projects := m.metadata.Vercel().Projects()
	defaultProject := ""
	if len(projects) > 0 {
		defaultProject = projects[0]
	}
	m.openForm("New Vercel Sandbox", "vercel_new", []field{
		{label: "Name", description: "Optional URL-safe name", optional: true, placeholder: "dev"},
		{label: "Project", description: "Required to create if team-wide list has no default", optional: true, value: defaultProject, placeholder: "prj_…"},
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
	fields := []field{
		{label: "Access token", description: "https://vercel.com/account/settings/tokens", secret: true, placeholder: "token"},
		{label: "Team ID", value: integration.TeamID, placeholder: "team_…"},
	}
	if len(integration.Projects()) == 0 {
		fields = append(fields, field{label: "Project ID", placeholder: "prj_…"})
	}
	m.openForm("Vercel access token", "vercel_token", fields)
}

func (m *App) openVercelProjectAddForm() {
	m.openForm("Add Vercel project", "vercel_project_add", []field{
		{label: "Project ID", description: "Project ID or name. Sandboxes in this project are imported on the next sync", placeholder: "prj_…"},
	})
}

func (m *App) openVercelProjectRemoveForm() {
	projects := m.metadata.Vercel().Projects()
	if len(projects) == 0 {
		m.setError(errString("no stored Vercel projects to remove"))
		return
	}
	options := make([]fieldOption, 0, len(projects))
	for _, id := range projects {
		options = append(options, fieldOption{label: id, value: id})
	}
	m.openForm("Remove Vercel project", "vercel_project_remove", []field{
		{label: "Project", description: "Stops importing sandboxes from this project", options: options},
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
	fallback := integration.ProjectID
	if projects := integration.Projects(); len(projects) > 0 {
		fallback = projects[0]
	}
	projectID, name, err := vercelcloud.ScopedName(host.SyncID, fallback)
	if err != nil {
		return nil, err
	}
	exe := ""
	if m.syncer != nil {
		exe = m.syncer.BastExecutable
	}
	return vercelcloud.ShellCommand(exe, name, projectID, integration.TeamID), nil
}
