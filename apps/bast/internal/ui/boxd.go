package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	boxdcloud "bast/internal/cloud/boxd"
	"bast/internal/sshconfig"
)

func (m *App) startSelectedBoxd(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["boxd"] {
		return m.setNotice("boxd operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("boxd computer is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("boxd sync id missing; sync boxd first")
	}
	opGen := m.beginProviderOp("boxd")
	m.syncActivity = "starting…"
	if thenConnect {
		m.sandboxConnectAfter = host.Alias
	} else {
		m.sandboxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.StartBoxd(ctx, host.SyncID)
		return syncDoneMsg{provider: "boxd", result: result, err: err, opGen: opGen}
	}
}

func (m *App) openBoxdNewForm() {
	org := ""
	if orgs := m.metadata.Boxd().OrgFilter; len(orgs) == 1 {
		org = orgs[0]
	}
	m.openForm("New boxd computer", "boxd_new", []field{
		{label: "Name", description: "Blank picks a name", optional: true, placeholder: "web"},
		{label: "Org", description: "Blank uses the synced org", optional: true, placeholder: "my-org", value: org},
		{label: "Region", description: "Blank uses the org's default region", optional: true, placeholder: "fra"},
		{
			label: "Size", description: "A snapshot keeps its own size", optional: true,
			options: []fieldOption{
				{label: "Org default", value: ""},
				{label: "1 vCPU · 4 GB", value: "1"},
				{label: "2 vCPU · 8 GB", value: "2"},
				{label: "4 vCPU · 16 GB", value: "4"},
				{label: "8 vCPU · 32 GB", value: "8"},
			},
		},
		{label: "Snapshot", description: "Optional snapshot to start from", optional: true},
	})
}

func (m *App) openBoxdStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("boxd computer is already stopped"))
		return
	}
	m.openForm("Stop boxd computer", "boxd_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Keeps the disk; running processes stop", placeholder: "stop"},
	})
}

func (m *App) openBoxdRestartForm(host sshconfig.Host) {
	m.openForm("Restart boxd computer", "boxd_restart", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type restart to confirm", description: "Reboots the computer; the disk is kept", placeholder: "restart"},
	})
}

func (m *App) openBoxdForkForm(host sshconfig.Host) {
	m.openForm("Fork boxd computer", "boxd_fork", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Name", description: "Blank picks a name", optional: true},
		{label: "Type fork to confirm", description: "Copies the disk, memory, and running processes", placeholder: "fork"},
	})
}

func (m *App) openBoxdDeleteForm(host sshconfig.Host) {
	m.openForm("Delete boxd computer: "+m.hostLabel(host), "boxd_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently deletes the computer and its disk", placeholder: "delete"},
	})
}

func (m *App) submitBoxdForm(action string, values map[string]string) tea.Cmd {
	confirm := map[string]string{"boxd_stop": "stop", "boxd_restart": "restart", "boxd_fork": "fork", "boxd_delete": "delete"}
	if word, ok := confirm[action]; ok && strings.TrimSpace(values["Type "+word+" to confirm"]) != word {
		if m.form != nil {
			m.form.validationError = "type " + word + " to confirm"
		}
		return nil
	}
	if action == "sync_boxd_orgs" {
		boxd := m.metadata.Boxd()
		boxd.OrgFilter = splitCSV(values["Orgs"])
		if err := m.metadata.SetBoxd(boxd); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("boxd org filter updated")
	}
	syncID := strings.TrimSpace(values["SyncID"])
	m.form = nil
	if m.syncingProviders["boxd"] {
		return m.setNotice("boxd operation already in progress")
	}
	opGen := m.beginProviderOp("boxd")
	busy := func(sync, hosts string) {
		if m.section == syncSection {
			m.beginSyncBusy(sync)
		} else {
			m.syncActivity = hosts
		}
	}
	switch action {
	case "boxd_new":
		vcpu, _ := strconv.Atoi(strings.TrimSpace(values["Size"]))
		opts := boxdcloud.CreateOpts{
			OrgID: strings.TrimSpace(values["Org"]), Region: strings.TrimSpace(values["Region"]),
			Name: strings.TrimSpace(values["Name"]), VCPU: vcpu, Snapshot: strings.TrimSpace(values["Snapshot"]),
		}
		busy("Creating boxd computer…", "creating…")
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()
			result, alias, err := m.syncer.NewBoxd(ctx, opts)
			msg := syncDoneMsg{provider: "boxd", result: result, err: err, opGen: opGen}
			if err == nil {
				msg.focusAlias = alias
			}
			return msg
		}
	case "boxd_fork":
		name := strings.TrimSpace(values["Name"])
		busy("Forking boxd computer…", "forking…")
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()
			result, alias, err := m.syncer.ForkBoxd(ctx, syncID, boxdcloud.ForkOpts{Name: name})
			msg := syncDoneMsg{provider: "boxd", result: result, err: err, opGen: opGen}
			if err == nil {
				msg.focusAlias = alias
			}
			return msg
		}
	case "boxd_stop", "boxd_restart", "boxd_delete":
		activity := map[string]string{"boxd_stop": "stopping…", "boxd_restart": "restarting…", "boxd_delete": "deleting…"}[action]
		m.syncActivity = activity
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			msg := syncDoneMsg{provider: "boxd", opGen: opGen}
			switch action {
			case "boxd_stop":
				msg.result, msg.err = m.syncer.StopBoxd(ctx, syncID)
			case "boxd_restart":
				msg.result, msg.err = m.syncer.RestartBoxd(ctx, syncID)
			case "boxd_delete":
				msg.result, msg.err = m.syncer.DeleteBoxd(ctx, syncID)
			}
			return msg
		}
	}
	return nil
}

func (m *App) openBoxdOrgsForm() {
	m.openForm("boxd org filter", "sync_boxd_orgs", []field{
		{label: "Orgs", description: "Comma-separated org names or ids; blank = the boxd CLI's active org", value: strings.Join(m.metadata.Boxd().OrgFilter, ", "), optional: true, placeholder: "my-org"},
	})
}
