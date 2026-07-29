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
	gcpDetail := "disabled"
	if gcp.Enabled {
		gcpDetail = fmt.Sprintf("%d instances", gcp.LastInstanceCount)
		if gcp.LastSyncAt != nil {
			gcpDetail = fmt.Sprintf("%d · %s", gcp.LastInstanceCount, gcp.LastSyncAt.Local().Format("2006-01-02 15:04"))
		}
	}
	aws := m.metadata.AWS()
	awsDetail := "disabled"
	if aws.Enabled {
		awsDetail = fmt.Sprintf("%d instances", aws.LastInstanceCount)
		if aws.LastSyncAt != nil {
			awsDetail = fmt.Sprintf("%d · %s", aws.LastInstanceCount, aws.LastSyncAt.Local().Format("2006-01-02 15:04"))
		}
	}
	azure := m.metadata.Azure()
	azureDetail := "disabled"
	if azure.Enabled {
		azureDetail = fmt.Sprintf("%d instances", azure.LastInstanceCount)
		if azure.LastSyncAt != nil {
			azureDetail = fmt.Sprintf("%d · %s", azure.LastInstanceCount, azure.LastSyncAt.Local().Format("2006-01-02 15:04"))
		}
	}
	return []syncMenuItem{
		{label: "GCP", detail: gcpDetail, provider: "gcp"},
		{label: "AWS", detail: awsDetail, provider: "aws"},
		{label: "Azure", detail: azureDetail, provider: "azure"},
	}
}

func (m *App) syncProviderActions(provider string) []syncMenuItem {
	switch provider {
	case "gcp":
		return m.gcpSyncActions()
	case "aws":
		return m.awsSyncActions()
	case "azure":
		return m.azureSyncActions()
	default:
		return nil
	}
}

func (m *App) azureSyncActions() []syncMenuItem {
	azure := m.metadata.Azure()
	actions := []syncMenuItem{{label: "Sync now", action: "sync"}}
	if azure.Enabled {
		actions = append(actions, syncMenuItem{label: "Disconnect", action: "disable"})
	} else {
		actions = append(actions, syncMenuItem{label: "Connect", action: "enable"})
	}
	if azure.AutoSync {
		actions = append(actions, syncMenuItem{label: "Disable auto-sync", action: "auto_off"})
	} else {
		actions = append(actions, syncMenuItem{label: "Enable auto-sync", action: "auto_on"})
	}
	return append(actions,
		syncMenuItem{label: "Default SSH user", action: "user"},
		syncMenuItem{label: "Subscription filter", action: "subscriptions"},
		syncMenuItem{label: "Resource group filter", action: "resource_groups"},
		syncMenuItem{label: "Refresh status", action: "refresh"},
	)
}

func (m *App) awsSyncActions() []syncMenuItem {
	aws := m.metadata.AWS()
	actions := []syncMenuItem{{label: "Sync now", action: "sync"}}
	if aws.Enabled {
		actions = append(actions, syncMenuItem{label: "Disconnect", action: "disable"})
	} else {
		actions = append(actions, syncMenuItem{label: "Connect", action: "enable"})
	}
	if aws.AutoSync {
		actions = append(actions, syncMenuItem{label: "Disable auto-sync", action: "auto_off"})
	} else {
		actions = append(actions, syncMenuItem{label: "Enable auto-sync", action: "auto_on"})
	}
	return append(actions,
		syncMenuItem{label: "Default SSH user", action: "user"},
		syncMenuItem{label: "Profile filter", action: "profiles"},
		syncMenuItem{label: "Region filter", action: "regions"},
		syncMenuItem{label: "Refresh status", action: "refresh"},
	)
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
		return syncDoneMsg{provider: "gcp", result: result, err: err}
	}
}

func (m *App) syncAWSCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncAWS(ctx)
		return syncDoneMsg{provider: "aws", result: result, err: err}
	}
}

func (m *App) syncAzureCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncAzure(ctx)
		return syncDoneMsg{provider: "azure", result: result, err: err}
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
			return syncDoneMsg{provider: "gcp", err: err}
		}
		return syncDoneMsg{provider: "gcp", result: sync.Result{Provider: "gcp", Count: 0, SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) disableAWSCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.syncer.DisableAWS(ctx)
		if err != nil {
			return syncDoneMsg{provider: "aws", err: err}
		}
		return syncDoneMsg{provider: "aws", result: sync.Result{Provider: "aws", SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) disableAzureCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := m.syncer.DisableAzure(ctx)
		if err != nil {
			return syncDoneMsg{provider: "azure", err: err}
		}
		return syncDoneMsg{provider: "azure", result: sync.Result{Provider: "azure", SyncedAt: time.Now().UTC(), Error: "disabled"}}
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
	case "azure":
		title = "Azure"
	}
	b.WriteString("\n  " + s.active.Render(title) + "\n")
	b.WriteString(m.renderProviderStatus(s, m.syncProvider))
	b.WriteString("\n")
	for i, item := range items {
		b.WriteString(m.renderSyncMenuLine(s, i, item) + "\n")
	}
	if m.providerSyncing(m.syncProvider) {
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
	case "aws":
		aws := m.metadata.AWS()
		status := m.syncStatus.AWS
		enabled := "disabled"
		if aws.Enabled {
			enabled = "enabled"
		}
		b.WriteString(compactRow(s, "Status", enabled, width))
		if status.AWSCLIError != "" {
			b.WriteString(compactRow(s, "aws", status.AWSCLIError, width))
		} else if len(status.Profiles) > 0 {
			b.WriteString(compactRow(s, "Profiles", strings.Join(status.Profiles, ", "), width))
		} else {
			b.WriteString(compactRow(s, "Profiles", "none", width))
		}
		if aws.LastSyncAt != nil {
			b.WriteString(compactRow(s, "Last sync", aws.LastSyncAt.Local().Format("2006-01-02 15:04")+" · "+fmt.Sprintf("%d", aws.LastInstanceCount), width))
		} else {
			b.WriteString(compactRow(s, "Last sync", "never", width))
		}
		if aws.LastSyncError != "" {
			b.WriteString(compactRow(s, "Error", aws.LastSyncError, width))
		}
		auto := "off"
		if aws.AutoSync {
			auto = "on"
		}
		b.WriteString(compactRow(s, "Auto-sync", auto, width))
		b.WriteString(compactRow(s, "SSH user", noneValue(aws.DefaultSSHUser), width))
		profiles := "all"
		if len(aws.ProfileFilter) > 0 {
			profiles = strings.Join(aws.ProfileFilter, ", ")
		}
		regions := "all enabled"
		if len(aws.RegionFilter) > 0 {
			regions = strings.Join(aws.RegionFilter, ", ")
		}
		b.WriteString(compactRow(s, "Profile filter", profiles, width))
		b.WriteString(compactRow(s, "Regions", regions, width))
	case "azure":
		azure := m.metadata.Azure()
		status := m.syncStatus.Azure
		enabled := "disabled"
		if azure.Enabled {
			enabled = "enabled"
		}
		b.WriteString(compactRow(s, "Status", enabled, width))
		if status.AzureCLIError != "" {
			b.WriteString(compactRow(s, "az", status.AzureCLIError, width))
		} else if len(status.Subscriptions) > 0 {
			b.WriteString(compactRow(s, "Subscriptions", strings.Join(status.Subscriptions, ", "), width))
		} else {
			b.WriteString(compactRow(s, "Subscriptions", "none", width))
		}
		if status.SSHExtensionError != "" {
			b.WriteString(compactRow(s, "ssh extension", status.SSHExtensionError, width))
		}
		if status.BastionExtensionError != "" {
			b.WriteString(compactRow(s, "bastion extension", status.BastionExtensionError, width))
		}
		if azure.LastSyncAt != nil {
			b.WriteString(compactRow(s, "Last sync", azure.LastSyncAt.Local().Format("2006-01-02 15:04")+" · "+fmt.Sprintf("%d", azure.LastInstanceCount), width))
		} else {
			b.WriteString(compactRow(s, "Last sync", "never", width))
		}
		if azure.LastSyncError != "" {
			b.WriteString(compactRow(s, "Error", azure.LastSyncError, width))
		}
		auto := "off"
		if azure.AutoSync {
			auto = "on"
		}
		b.WriteString(compactRow(s, "Auto-sync", auto, width))
		b.WriteString(compactRow(s, "SSH user", noneValue(azure.DefaultSSHUser), width))
		subscriptions := "all enabled"
		if len(azure.SubscriptionFilter) > 0 {
			subscriptions = strings.Join(azure.SubscriptionFilter, ", ")
		}
		resourceGroups := "all"
		if len(azure.ResourceGroupFilter) > 0 {
			resourceGroups = strings.Join(azure.ResourceGroupFilter, ", ")
		}
		b.WriteString(compactRow(s, "Subscription filter", subscriptions, width))
		b.WriteString(compactRow(s, "Resource groups", resourceGroups, width))
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
	case "esc", "backspace", "ctrl+h":
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
		if m.anySyncing() {
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
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.anySyncing() {
		return m, nil
	}
	switch action {
	case "sync", "enable":
		m.syncingProviders[m.syncProvider] = true
		switch m.syncProvider {
		case "aws":
			return m, tea.Batch(m.syncAWSCmd(), m.setNotice("Syncing AWS…"))
		case "azure":
			return m, tea.Batch(m.syncAzureCmd(), m.setNotice("Syncing Azure…"))
		}
		return m, tea.Batch(m.syncGCPCmd(), m.setNotice("Syncing GCP…"))
	case "disable":
		m.syncingProviders[m.syncProvider] = true
		if m.syncProvider == "aws" {
			return m, m.disableAWSCmd()
		}
		if m.syncProvider == "azure" {
			return m, m.disableAzureCmd()
		}
		return m, m.disableGCPCmd()
	case "auto_on":
		if m.syncProvider == "aws" {
			aws := m.metadata.AWS()
			aws.AutoSync = true
			if err := m.metadata.SetAWS(aws); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync enabled")
		}
		if m.syncProvider == "azure" {
			azure := m.metadata.Azure()
			azure.AutoSync = true
			if err := m.metadata.SetAzure(azure); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync enabled")
		}
		gcp := m.metadata.GCP()
		gcp.AutoSync = true
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return m, nil
		}
		return m, m.setNotice("Auto-sync enabled")
	case "auto_off":
		if m.syncProvider == "aws" {
			aws := m.metadata.AWS()
			aws.AutoSync = false
			if err := m.metadata.SetAWS(aws); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync disabled")
		}
		if m.syncProvider == "azure" {
			azure := m.metadata.Azure()
			azure.AutoSync = false
			if err := m.metadata.SetAzure(azure); err != nil {
				m.setError(err)
				return m, nil
			}
			return m, m.setNotice("Auto-sync disabled")
		}
		gcp := m.metadata.GCP()
		gcp.AutoSync = false
		if err := m.metadata.SetGCP(gcp); err != nil {
			m.setError(err)
			return m, nil
		}
		return m, m.setNotice("Auto-sync disabled")
	case "user":
		if m.syncProvider == "aws" {
			m.openForm("Default AWS SSH user", "sync_aws_user", []field{
				{label: "SSH user", description: "Blank uses the AMI default when known", value: m.metadata.AWS().DefaultSSHUser, optional: true, placeholder: "ec2-user"},
			})
			break
		}
		if m.syncProvider == "azure" {
			m.openForm("Default Azure SSH user", "sync_azure_user", []field{
				{label: "SSH user", description: "Blank uses the VM admin user or Microsoft Entra login", value: m.metadata.Azure().DefaultSSHUser, optional: true, placeholder: "azureuser"},
			})
			break
		}
		m.openForm("Default GCP SSH user", "sync_gcp_user", []field{
			{label: "SSH user", description: "Blank uses OS Login or instance metadata when available", value: m.metadata.GCP().DefaultSSHUser, optional: true, placeholder: "ubuntu"},
		})
	case "projects":
		m.openForm("GCP project filter", "sync_gcp_projects", []field{
			{label: "Projects", description: "Comma-separated IDs or names; blank = all. Narrowing removes previously synced hosts outside the filter", value: strings.Join(m.metadata.GCP().ProjectFilter, ", "), optional: true, placeholder: "my-prod, my-staging"},
		})
	case "profiles":
		m.openForm("AWS profile filter", "sync_aws_profiles", []field{
			{label: "Profiles", description: "Comma-separated AWS CLI profiles; blank = all configured", value: strings.Join(m.metadata.AWS().ProfileFilter, ", "), optional: true, placeholder: "default, production"},
		})
	case "regions":
		m.openForm("AWS region filter", "sync_aws_regions", []field{
			{label: "Regions", description: "Comma-separated AWS regions; blank = all enabled", value: strings.Join(m.metadata.AWS().RegionFilter, ", "), optional: true, placeholder: "eu-west-1, us-east-1"},
		})
	case "subscriptions":
		m.openForm("Azure subscription filter", "sync_azure_subscriptions", []field{
			{label: "Subscriptions", description: "Comma-separated names or IDs; blank = all enabled", value: strings.Join(m.metadata.Azure().SubscriptionFilter, ", "), optional: true, placeholder: "Production, Staging"},
		})
	case "resource_groups":
		m.openForm("Azure resource group filter", "sync_azure_resource_groups", []field{
			{label: "Resource groups", description: "Comma-separated names; blank = all", value: strings.Join(m.metadata.Azure().ResourceGroupFilter, ", "), optional: true, placeholder: "production, staging"},
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
		fieldOptions := make([]fieldOption, 0, len(options))
		for _, path := range options {
			fieldOptions = append(fieldOptions, fieldOption{label: path, value: path})
		}
		m.openForm("Remove service account key", "sync_gcp_sa_remove", []field{
			{label: "Key path", description: "Choose the key to remove", options: fieldOptions},
		})
	case "refresh":
		return m, tea.Batch(m.syncStatusCmd(), m.setNotice("Refreshing sync status…"))
	}
	return m, nil
}

func (m *App) submitSyncForm(action string, values map[string]string) tea.Cmd {
	gcp := m.metadata.GCP()
	switch action {
	case "sync_azure_user":
		azure := m.metadata.Azure()
		azure.DefaultSSHUser = strings.TrimSpace(values["SSH user"])
		if err := m.metadata.SetAzure(azure); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Default Azure SSH user updated")
	case "sync_azure_subscriptions":
		azure := m.metadata.Azure()
		azure.SubscriptionFilter = splitCSV(values["Subscriptions"])
		if err := m.metadata.SetAzure(azure); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Azure subscription filter updated")
	case "sync_azure_resource_groups":
		azure := m.metadata.Azure()
		azure.ResourceGroupFilter = splitCSV(values["Resource groups"])
		if err := m.metadata.SetAzure(azure); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Azure resource group filter updated")
	case "sync_aws_user":
		aws := m.metadata.AWS()
		aws.DefaultSSHUser = strings.TrimSpace(values["SSH user"])
		if err := m.metadata.SetAWS(aws); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("Default AWS SSH user updated")
	case "sync_aws_profiles":
		aws := m.metadata.AWS()
		aws.ProfileFilter = splitCSV(values["Profiles"])
		if err := m.metadata.SetAWS(aws); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("AWS profile filter updated")
	case "sync_aws_regions":
		aws := m.metadata.AWS()
		aws.RegionFilter = splitCSV(values["Regions"])
		if err := m.metadata.SetAWS(aws); err != nil {
			m.setError(err)
			return nil
		}
		m.form = nil
		return m.setNotice("AWS region filter updated")
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
