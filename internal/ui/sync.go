package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/cloud/sync"
)

type syncMenuItem struct {
	label    string
	detail   string
	action   string
	provider string
	disabled bool
}

func (m *App) syncProviders() []syncMenuItem {
	gcp := m.metadata.GCP()
	detail := "disabled"
	if gcp.Enabled {
		detail = fmt.Sprintf("%d instances", gcp.LastInstanceCount)
		if gcp.LastSyncAt != nil {
			detail = fmt.Sprintf("%d · %s", gcp.LastInstanceCount, gcp.LastSyncAt.Local().Format("2006-01-02 15:04"))
		}
	}
	return []syncMenuItem{
		{label: "GCP", detail: detail, provider: "gcp"},
		{label: "AWS", detail: "coming soon", disabled: true},
		{label: "Azure", detail: "coming soon", disabled: true},
	}
}

func (m *App) syncProviderActions(provider string) []syncMenuItem {
	switch provider {
	case "gcp":
		return m.gcpSyncActions()
	default:
		return nil
	}
}

func (m *App) gcpSyncActions() []syncMenuItem {
	gcp := m.metadata.GCP()
	actions := []syncMenuItem{
		{label: "Sync now", action: "sync"},
	}
	if gcp.Enabled {
		actions = append(actions, syncMenuItem{label: "Disconnect", action: "disable"})
	} else {
		actions = append(actions, syncMenuItem{label: "Connect", action: "enable"})
	}
	if gcp.AutoSync {
		actions = append(actions, syncMenuItem{label: "Disable auto-sync", action: "auto_off"})
	} else {
		actions = append(actions, syncMenuItem{label: "Enable auto-sync", action: "auto_on"})
	}
	actions = append(actions,
		syncMenuItem{label: "Default SSH user", action: "user"},
		syncMenuItem{label: "Project filter", action: "projects"},
		syncMenuItem{label: "Add service account key", action: "sa_add"},
	)
	if len(gcp.ServiceAccounts) > 0 {
		actions = append(actions, syncMenuItem{label: "Remove service account key", action: "sa_remove"})
	}
	actions = append(actions, syncMenuItem{label: "Refresh status", action: "refresh"})
	return actions
}

func (m *App) syncMenuItems() []syncMenuItem {
	if m.syncProvider == "" {
		return m.syncProviders()
	}
	return m.syncProviderActions(m.syncProvider)
}

func (m *App) syncGCPCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncGCP(ctx)
		return syncDoneMsg{result: result, err: err}
	}
}

func (m *App) syncStatusCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		status, err := m.syncer.Status(ctx)
		return syncStatusMsg{status: status, err: err}
	}
}

func (m *App) disableGCPCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.syncer.DisableGCP(ctx)
		if err != nil {
			return syncDoneMsg{err: err}
		}
		return syncDoneMsg{result: sync.Result{Provider: "gcp", Count: 0, SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) renderSync(s styleSet) string {
	items := m.syncMenuItems()
	if m.syncCursor >= len(items) {
		m.syncCursor = max(0, len(items)-1)
	}

	var b strings.Builder
	if m.syncProvider == "" {
		b.WriteString("\n  " + s.active.Render("Providers") + "\n")
		b.WriteString("  " + s.muted.Render("Browse and connect to VMs from your cloud accounts.") + "\n\n")
		for i, item := range items {
			b.WriteString(m.renderSyncMenuLine(s, i, item) + "\n")
		}
		return b.String()
	}

	title := strings.ToUpper(m.syncProvider)
	switch m.syncProvider {
	case "gcp":
		title = "GCP"
	}
	b.WriteString("\n  " + s.active.Render(title) + "\n")
	b.WriteString(m.renderProviderStatus(s, m.syncProvider))
	b.WriteString("\n")
	for i, item := range items {
		b.WriteString(m.renderSyncMenuLine(s, i, item) + "\n")
	}
	if m.syncing {
		b.WriteString("\n  " + s.muted.Render("Syncing…") + "\n")
	}
	return b.String()
}

func (m *App) renderSyncMenuLine(s styleSet, index int, item syncMenuItem) string {
	width := min(56, max(24, m.terminalWidth()-4))
	label := item.label
	if item.detail != "" {
		gap := max(1, width-lipgloss.Width(label)-lipgloss.Width(item.detail)-2)
		label = label + strings.Repeat(" ", gap) + item.detail
	}
	line := "  " + label
	switch {
	case index == m.syncCursor && !item.disabled:
		return s.selected.Width(width + 2).Render(line)
	case item.disabled:
		return s.muted.Width(width + 2).Render(line)
	default:
		return s.value.Width(width + 2).Render(line)
	}
}

func (m *App) renderProviderStatus(s styleSet, provider string) string {
	width := m.terminalWidth()
	var b strings.Builder
	switch provider {
	case "gcp":
		gcp := m.metadata.GCP()
		status := m.syncStatus.GCP
		enabled := "disabled"
		if gcp.Enabled {
			enabled = "enabled"
		}
		b.WriteString(compactRow(s, "Status", enabled, width))
		if status.GCloudError != "" {
			b.WriteString(compactRow(s, "gcloud", status.GCloudError, width))
		} else if len(status.Accounts) > 0 {
			b.WriteString(compactRow(s, "Accounts", strings.Join(status.Accounts, ", "), width))
		} else {
			b.WriteString(compactRow(s, "Accounts", "none", width))
		}
		if gcp.LastSyncAt != nil {
			b.WriteString(compactRow(s, "Last sync", gcp.LastSyncAt.Local().Format("2006-01-02 15:04")+" · "+fmt.Sprintf("%d", gcp.LastInstanceCount), width))
		} else {
			b.WriteString(compactRow(s, "Last sync", "never", width))
		}
		if gcp.LastSyncError != "" {
			b.WriteString(compactRow(s, "Error", gcp.LastSyncError, width))
		}
		auto := "off"
		if gcp.AutoSync {
			auto = "on"
		}
		b.WriteString(compactRow(s, "Auto-sync", auto, width))
		b.WriteString(compactRow(s, "SSH user", noneValue(gcp.DefaultSSHUser), width))
		if len(gcp.ProjectFilter) > 0 {
			b.WriteString(compactRow(s, "Projects", strings.Join(gcp.ProjectFilter, ", "), width))
		} else {
			b.WriteString(compactRow(s, "Projects", "all", width))
		}
		if len(gcp.ServiceAccounts) > 0 {
			b.WriteString(compactRow(s, "SA keys", strings.Join(gcp.ServiceAccounts, ", "), width))
		}
	}
	return b.String()
}

func (m *App) updateSyncKeys(key string) (tea.Model, tea.Cmd) {
	items := m.syncMenuItems()
	switch key {
	case "up", "k":
		m.syncCursor = prevEnabledSyncItem(items, m.syncCursor)
	case "down", "j":
		m.syncCursor = nextEnabledSyncItem(items, m.syncCursor)
	case "home", "g":
		m.syncCursor = firstEnabledSyncItem(items)
	case "end", "G":
		m.syncCursor = lastEnabledSyncItem(items)
	case "esc":
		if m.syncProvider != "" {
			m.syncProvider = ""
			m.syncCursor = 0
		}
	case "r":
		if m.syncProvider != "" {
			return m, m.syncStatusCmd()
		}
	case "enter":
		if len(items) == 0 || m.syncCursor < 0 || m.syncCursor >= len(items) {
			return m, nil
		}
		item := items[m.syncCursor]
		if item.disabled {
			return m, nil
		}
		if m.syncProvider == "" {
			m.syncProvider = item.provider
			m.syncCursor = 0
			return m, m.syncStatusCmd()
		}
		if m.syncing {
			return m, nil
		}
		return m.runSyncAction(item.action)
	}
	return m, nil
}

func firstEnabledSyncItem(items []syncMenuItem) int {
	for i, item := range items {
		if !item.disabled {
			return i
		}
	}
	return 0
}

func lastEnabledSyncItem(items []syncMenuItem) int {
	for i := len(items) - 1; i >= 0; i-- {
		if !items[i].disabled {
			return i
		}
	}
	return max(0, len(items)-1)
}

func nextEnabledSyncItem(items []syncMenuItem, current int) int {
	for i := current + 1; i < len(items); i++ {
		if !items[i].disabled {
			return i
		}
	}
	return current
}

func prevEnabledSyncItem(items []syncMenuItem, current int) int {
	for i := current - 1; i >= 0; i-- {
		if !items[i].disabled {
			return i
		}
	}
	return current
}

func (m *App) runSyncAction(action string) (tea.Model, tea.Cmd) {
	if m.syncing {
		return m, nil
	}
	switch action {
	case "sync", "enable":
		m.syncing = true
		return m, tea.Batch(m.syncGCPCmd(), m.setNotice("Syncing GCP…"))
	case "disable":
		m.syncing = true
		return m, m.disableGCPCmd()
	case "auto_on":
		gcp := m.metadata.GCP()
		gcp.AutoSync = true
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return m, nil
		}
		return m, m.setNotice("Auto-sync enabled")
	case "auto_off":
		gcp := m.metadata.GCP()
		gcp.AutoSync = false
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return m, nil
		}
		return m, m.setNotice("Auto-sync disabled")
	case "user":
		m.openForm("Default GCP SSH user", "sync_gcp_user", []field{
			{label: "SSH user", description: "Blank uses OS Login or instance metadata when available", value: m.metadata.GCP().DefaultSSHUser, optional: true, placeholder: "ubuntu"},
		})
	case "projects":
		m.openForm("GCP project filter", "sync_gcp_projects", []field{
			{label: "Projects", description: "Comma-separated project IDs; blank = all accessible", value: strings.Join(m.metadata.GCP().ProjectFilter, ", "), optional: true, placeholder: "my-prod, my-staging"},
		})
	case "sa_add":
		m.openForm("Add service account key", "sync_gcp_sa_add", []field{
			{label: "Key path", description: "Path to a GCP service account JSON key", placeholder: "~/keys/gcp-sa.json"},
		})
	case "sa_remove":
		options := m.metadata.GCP().ServiceAccounts
		if len(options) == 0 {
			return m, m.setNotice("No service account keys configured")
		}
		m.openForm("Remove service account key", "sync_gcp_sa_remove", []field{
			{label: "Key path", description: "Exact path to remove", value: options[0], placeholder: options[0]},
		})
	case "refresh":
		return m, tea.Batch(m.syncStatusCmd(), m.setNotice("Refreshing sync status…"))
	}
	return m, nil
}

func (m *App) submitSyncForm(action string, values map[string]string) tea.Cmd {
	gcp := m.metadata.GCP()
	switch action {
	case "sync_gcp_user":
		gcp.DefaultSSHUser = strings.TrimSpace(values["SSH user"])
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Default GCP SSH user updated")
	case "sync_gcp_projects":
		gcp.ProjectFilter = splitCSV(values["Projects"])
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("GCP project filter updated")
	case "sync_gcp_sa_add":
		path := strings.TrimSpace(values["Key path"])
		if err := sync.ValidateServiceAccountPath(path); err != nil {
			m.setError(err)
			return nil
		}
		for _, existing := range gcp.ServiceAccounts {
			if existing == path {
				m.form = nil
				return m.setNotice("Service account already configured")
			}
		}
		gcp.ServiceAccounts = append(gcp.ServiceAccounts, path)
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Service account key added")
	case "sync_gcp_sa_remove":
		path := strings.TrimSpace(values["Key path"])
		kept := make([]string, 0, len(gcp.ServiceAccounts))
		removed := false
		for _, existing := range gcp.ServiceAccounts {
			if existing == path {
				removed = true
				continue
			}
			kept = append(kept, existing)
		}
		if !removed {
			m.setError(fmt.Errorf("service account key %q not found", path))
			return nil
		}
		gcp.ServiceAccounts = kept
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Service account key removed")
	}
	return nil
}
