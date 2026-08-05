package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	boxcloud "bast/internal/cloud/box"
	"bast/internal/cloud/sync"
)

type syncMenuItem struct {
	label       string
	detail      string
	description string
	action      string
	provider    string
	disabled    bool
}

type providerDetail struct {
	enabled           bool
	autoSync          bool
	lastSyncAt        *time.Time
	lastInstanceCount int
	lastSyncError     string
	sshUser           string
	status            []providerRow
	filters           []providerRow
}

type providerRow struct {
	label string
	value string
}

func (m *App) syncProviderTitle() string {
	switch m.syncProvider {
	case "vault":
		return "Vault"
	case "gcp":
		return "GCP"
	case "aws":
		return "AWS"
	case "azure":
		return "Azure"
	case "box":
		return "box.ascii.dev"
	default:
		if m.syncProvider == "" {
			return "Sync"
		}
		return strings.ToUpper(m.syncProvider)
	}
}

func (m *App) syncProviders() []syncMenuItem {
	vaultDetail := m.vaultStatusDetail()
	providers := []struct {
		label, name, description string
	}{
		{"Vault", "vault", "Bast-managed hosts and keys, encrypted between machines"},
		{"GCP", "gcp", "Import Compute Engine VMs into Bast"},
		{"AWS", "aws", "Import EC2 instances into Bast"},
		{"Azure", "azure", "Import Azure VMs into Bast"},
		{"box.ascii.dev", "box", "Import ASCII Box sandboxes into Bast"},
	}
	items := make([]syncMenuItem, 0, len(providers))
	for _, provider := range providers {
		if provider.name == "vault" {
			items = append(items, syncMenuItem{
				label: provider.label, detail: vaultDetail,
				description: provider.description, provider: provider.name,
			})
			continue
		}
		detail := m.providerDetail(provider.name)
		text := "disabled"
		if detail.enabled {
			text = fmt.Sprintf("%d instances", detail.lastInstanceCount)
			if detail.lastSyncAt != nil {
				text = fmt.Sprintf("%d · %s", detail.lastInstanceCount, detail.lastSyncAt.Local().Format("2006-01-02 15:04"))
			}
		}
		items = append(items, syncMenuItem{
			label: provider.label, detail: text,
			description: provider.description, provider: provider.name,
		})
	}
	return items
}

func (m *App) providerDetail(provider string) providerDetail {
	switch provider {
	case "gcp":
		integration := m.metadata.GCP()
		status := m.syncStatus.GCP
		accountValue := "none"
		accountLabel := "Accounts"
		if status.GCloudError != "" {
			accountLabel, accountValue = "gcloud", status.GCloudError
		} else if len(status.Accounts) > 0 {
			accountValue = strings.Join(status.Accounts, ", ")
		}
		projects := "all"
		if len(integration.ProjectFilter) > 0 {
			projects = strings.Join(integration.ProjectFilter, ", ")
		}
		filters := []providerRow{{"Projects", projects}}
		if len(integration.ServiceAccounts) > 0 {
			filters = append(filters, providerRow{"SA keys", strings.Join(integration.ServiceAccounts, ", ")})
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, integration.DefaultSSHUser, []providerRow{{accountLabel, accountValue}}, filters}
	case "aws":
		integration := m.metadata.AWS()
		status := m.syncStatus.AWS
		profileValue := "none"
		profileLabel := "Profiles"
		if status.AWSCLIError != "" {
			profileLabel, profileValue = "aws", status.AWSCLIError
		} else if len(status.Profiles) > 0 {
			profileValue = strings.Join(status.Profiles, ", ")
		}
		profiles := "all"
		if len(integration.ProfileFilter) > 0 {
			profiles = strings.Join(integration.ProfileFilter, ", ")
		}
		regions := "all enabled"
		if len(integration.RegionFilter) > 0 {
			regions = strings.Join(integration.RegionFilter, ", ")
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, integration.DefaultSSHUser, []providerRow{{profileLabel, profileValue}}, []providerRow{{"Profile filter", profiles}, {"Regions", regions}}}
	case "azure":
		integration := m.metadata.Azure()
		status := m.syncStatus.Azure
		subscriptionValue := "none"
		subscriptionLabel := "Subscriptions"
		if status.AzureCLIError != "" {
			subscriptionLabel, subscriptionValue = "az", status.AzureCLIError
		} else if len(status.Subscriptions) > 0 {
			subscriptionValue = strings.Join(status.Subscriptions, ", ")
		}
		statusRows := []providerRow{{subscriptionLabel, subscriptionValue}}
		if status.SSHExtensionError != "" {
			statusRows = append(statusRows, providerRow{"ssh extension", status.SSHExtensionError})
		}
		if status.BastionExtensionError != "" {
			statusRows = append(statusRows, providerRow{"bastion extension", status.BastionExtensionError})
		}
		subscriptions := "all enabled"
		if len(integration.SubscriptionFilter) > 0 {
			subscriptions = strings.Join(integration.SubscriptionFilter, ", ")
		}
		resourceGroups := "all"
		if len(integration.ResourceGroupFilter) > 0 {
			resourceGroups = strings.Join(integration.ResourceGroupFilter, ", ")
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, integration.DefaultSSHUser, statusRows, []providerRow{{"Subscription filter", subscriptions}, {"Resource groups", resourceGroups}}}
	case "box":
		integration := m.metadata.Box()
		status := m.syncStatus.Box
		accountLabel, accountValue := "Account", "not logged in"
		if status.BoxCLIError != "" {
			accountLabel, accountValue = "box", status.BoxCLIError
		} else if status.Authenticated {
			accountValue = status.Login
			if accountValue == "" {
				accountValue = "authenticated"
			}
		} else if integration.Disabled {
			accountValue = "disabled"
		}
		statusRows := []providerRow{{accountLabel, accountValue}}
		if status.Plan != "" {
			statusRows = append(statusRows, providerRow{"Plan", status.Plan})
		}
		if integration.Disabled {
			statusRows = append(statusRows, providerRow{"Opt-out", "sticky disable (no auto-connect)"})
		}
		return providerDetail{integration.Enabled, integration.AutoSync, integration.LastSyncAt, integration.LastInstanceCount, integration.LastSyncError, "", statusRows, nil}
	default:
		return providerDetail{}
	}
}

func (m *App) syncProviderActions(provider string) []syncMenuItem {
	switch provider {
	case "vault":
		return m.vaultMenuItems()
	case "gcp":
		return m.gcpSyncActions()
	case "aws":
		return m.awsSyncActions()
	case "azure":
		return m.azureSyncActions()
	case "box":
		return m.boxSyncActions()
	default:
		return nil
	}
}

func (m *App) boxSyncActions() []syncMenuItem {
	box := m.metadata.Box()
	actions := []syncMenuItem{{label: "Sync now", action: "sync"}}
	if m.syncStatus.Box.Authenticated {
		actions = append(actions, syncMenuItem{label: "New box", action: "box_new"})
	}
	if box.Enabled {
		actions = append(actions, syncMenuItem{label: "Disconnect", action: "disable"})
	} else {
		actions = append(actions, syncMenuItem{label: "Connect", action: "enable"})
	}
	if box.AutoSync {
		actions = append(actions, syncMenuItem{label: "Disable auto-sync", action: "auto_off"})
	} else {
		actions = append(actions, syncMenuItem{label: "Enable auto-sync", action: "auto_on"})
	}
	return append(actions, syncMenuItem{label: "Refresh status", action: "refresh"})
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

func (m *App) syncBoxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := m.syncer.SyncBox(ctx)
		return syncDoneMsg{provider: "box", result: result, err: err}
	}
}

func (m *App) autoConnectBoxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, ran, err := m.syncer.MaybeAutoConnectBox(ctx)
		if !ran {
			return syncDoneMsg{provider: "box", result: result, err: nil, skipped: true}
		}
		return syncDoneMsg{provider: "box", result: result, err: err}
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

func (m *App) disableBoxCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.syncer.DisableBox(ctx); err != nil {
			return syncDoneMsg{provider: "box", err: err}
		}
		return syncDoneMsg{provider: "box", result: sync.Result{Provider: "box", SyncedAt: time.Now().UTC(), Error: "disabled"}}
	}
}

func (m *App) renderSync(s styleSet) string {
	items := m.syncMenuItems()

	var b strings.Builder
	if m.syncProvider == "" {
		for i, item := range items {
			b.WriteString(m.renderSyncMenuLine(s, i, item) + "\n")
		}
		return b.String()
	}

	b.WriteString("\n  " + s.active.Render(m.syncProviderTitle()) + "\n")
	if m.syncProvider == "vault" {
		b.WriteString(m.renderVaultStatus(s))
	} else {
		b.WriteString(m.renderProviderStatus(s, m.syncProvider))
	}
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
	var rendered string
	switch {
	case index == m.syncCursor && !item.disabled:
		rendered = s.selected.Width(width + 2).Render(line)
	case item.disabled:
		rendered = s.muted.Width(width + 2).Render(line)
	default:
		rendered = s.value.Width(width + 2).Render(line)
	}
	if index == m.syncCursor && item.description != "" {
		desc := truncate(item.description, max(20, m.terminalWidth()-8))
		rendered += "\n    " + s.muted.Render(desc)
	}
	return rendered
}

func (m *App) renderProviderStatus(s styleSet, provider string) string {
	width := m.terminalWidth()
	var b strings.Builder
	detail := m.providerDetail(provider)
	enabled := "disabled"
	if detail.enabled {
		enabled = "enabled"
	}
	b.WriteString(compactRow(s, "Status", enabled, width))
	for _, row := range detail.status {
		b.WriteString(compactRow(s, row.label, row.value, width))
	}
	lastSync := "never"
	if detail.lastSyncAt != nil {
		lastSync = detail.lastSyncAt.Local().Format("2006-01-02 15:04") + " · " + fmt.Sprintf("%d", detail.lastInstanceCount)
	}
	b.WriteString(compactRow(s, "Last sync", lastSync, width))
	if detail.lastSyncError != "" {
		b.WriteString(compactRow(s, "Error", detail.lastSyncError, width))
	}
	autoSync := "off"
	if detail.autoSync {
		autoSync = "on"
	}
	b.WriteString(compactRow(s, "Auto-sync", autoSync, width))
	b.WriteString(compactRow(s, "SSH user", noneValue(detail.sshUser), width))
	for _, row := range detail.filters {
		b.WriteString(compactRow(s, row.label, row.value, width))
	}
	return b.String()
}

func (m *App) updateSyncKeys(key string) (tea.Model, tea.Cmd) {
	if m.syncBusy != "" {
		return m, nil
	}
	items := m.syncMenuItems()
	m.clampSyncCursor(items)
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
			if item.provider == "vault" {
				return m, nil
			}
			return m, m.syncStatusCmd()
		}
		if m.syncProvider == "vault" {
			return m.runVaultAction(item.action)
		}
		if m.anySyncing() {
			return m, nil
		}
		return m.runSyncAction(item.action)
	}
	return m, nil
}

func (m *App) clampSyncCursor(items []syncMenuItem) {
	if m.syncCursor >= len(items) {
		m.syncCursor = max(0, len(items)-1)
	}
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
		case "box":
			box := m.metadata.Box()
			box.Disabled = false
			box.Enabled = true
			box.AutoSync = true
			if err := m.metadata.SetBox(box); err != nil {
				delete(m.syncingProviders, "box")
				m.setError(err)
				return m, nil
			}
			return m, tea.Batch(m.syncBoxCmd(), m.setNotice("Syncing Box…"))
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
		if m.syncProvider == "box" {
			return m, m.disableBoxCmd()
		}
		return m, m.disableGCPCmd()
	case "box_new":
		m.openForm("New Box", "box_new", []field{
			{label: "Type", description: "small, default, or large", value: "default", optional: false, placeholder: "default"},
			{label: "No auto-stop", description: "yes to keep running until stopped", value: "", optional: true, placeholder: "yes"},
			{label: "No env", description: "yes for an isolated no-env box", value: "", optional: true, placeholder: "yes"},
		})
		return m, nil
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
		if m.syncProvider == "box" {
			box := m.metadata.Box()
			box.AutoSync = true
			box.Disabled = false
			if err := m.metadata.SetBox(box); err != nil {
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
		if m.syncProvider == "box" {
			box := m.metadata.Box()
			box.AutoSync = false
			if err := m.metadata.SetBox(box); err != nil {
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
		if m.syncProvider == "box" {
			return m, m.setNotice("Box SSH user is always user")
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
	case "box_new":
		boxType := strings.ToLower(strings.TrimSpace(values["Type"]))
		if boxType == "" {
			boxType = "default"
		}
		if boxType != "small" && boxType != "default" && boxType != "large" {
			m.setError(fmt.Errorf("type must be small, default, or large"))
			return nil
		}
		noAutoStop := truthyForm(values["No auto-stop"])
		noEnv := truthyForm(values["No env"])
		m.form = nil
		if m.syncingProviders == nil {
			m.syncingProviders = map[string]bool{}
		}
		m.syncingProviders["box"] = true
		m.beginSyncBusy("Creating box…")
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			result, alias, err := m.syncer.NewBox(ctx, boxcloud.NewOpts{
				Type: boxType, NoAutoStop: noAutoStop, NoEnv: noEnv,
			})
			if err != nil {
				return syncDoneMsg{provider: "box", result: result, err: err}
			}
			return syncDoneMsg{provider: "box", result: result, err: nil, focusAlias: alias}
		}
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
	case "box_stop":
		if strings.TrimSpace(values["Type stop to confirm"]) != "stop" {
			m.setError(fmt.Errorf("type stop to confirm"))
			return nil
		}
		syncID := strings.TrimSpace(values["SyncID"])
		m.form = nil
		if m.syncingProviders == nil {
			m.syncingProviders = map[string]bool{}
		}
		m.syncingProviders["box"] = true
		m.syncActivity = "stopping…"
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()
			result, err := m.syncer.StopBox(ctx, syncID)
			return syncDoneMsg{provider: "box", result: result, err: err}
		}
	case "box_fork":
		if strings.TrimSpace(values["Type fork to confirm"]) != "fork" {
			m.setError(fmt.Errorf("type fork to confirm"))
			return nil
		}
		syncID := strings.TrimSpace(values["SyncID"])
		m.form = nil
		if m.syncingProviders == nil {
			m.syncingProviders = map[string]bool{}
		}
		m.syncingProviders["box"] = true
		return tea.Batch(m.setNotice("Forking box…"), func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			result, _, err := m.syncer.ForkBox(ctx, syncID, boxcloud.ForkOpts{})
			return syncDoneMsg{provider: "box", result: result, err: err}
		})
	}
	return nil
}

func truthyForm(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "y", "yes", "true", "on":
		return true
	default:
		return false
	}
}
