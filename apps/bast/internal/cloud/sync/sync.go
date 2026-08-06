package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	stdsync "sync"
	"time"

	"bast/internal/cloud"
	awscloud "bast/internal/cloud/aws"
	azurecloud "bast/internal/cloud/azure"
	boxcloud "bast/internal/cloud/box"
	"bast/internal/cloud/gcp"
	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

type Result struct {
	Provider string    `json:"provider"`
	Count    int       `json:"count"`
	SyncedAt time.Time `json:"syncedAt"`
	Aliases  []string  `json:"aliases,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type Engine struct {
	gcpMu          stdsync.Mutex
	awsMu          stdsync.Mutex
	azureMu        stdsync.Mutex
	boxMu          stdsync.Mutex
	Paths          paths.Paths
	Config         sshconfig.Manager
	Store          *metadata.Store
	GCP            *gcp.Client
	AWS            *awscloud.Client
	Azure          *azurecloud.Client
	Box            *boxcloud.Client
	BastExecutable string
	Discover       func(ctx context.Context) ([]sshconfig.Host, error)

	// EnsureAccessWait overrides the guest-agent propagation pause after publishing a key.
	// Zero keeps the GCP client default; negative skips the wait (tests).
	EnsureAccessWait time.Duration
}

type hostMetadataUpdate struct {
	alias         string
	previousAlias string
	label         string
	group         string
	tags          []string
}

func New(p paths.Paths, store *metadata.Store) *Engine {
	cfg := sshconfig.Manager{
		Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir,
		ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys,
		SyncGCPConfig: p.SyncGCPConfig, SyncAWSConfig: p.SyncAWSConfig, SyncAzureConfig: p.SyncAzureConfig,
		SyncBoxConfig: p.SyncBoxConfig,
	}
	return &Engine{
		Paths:          p,
		Config:         cfg,
		Store:          store,
		GCP:            gcp.New(),
		AWS:            awscloud.New(),
		Azure:          azurecloud.New(),
		Box:            boxcloud.New(),
		BastExecutable: stableExecutablePath(),
		Discover: func(ctx context.Context) ([]sshconfig.Host, error) {
			return cfg.Discover()
		},
	}
}

func stableExecutablePath() string {
	name := os.Args[0]
	if found, err := exec.LookPath(name); err == nil {
		if absolute, absErr := filepath.Abs(found); absErr == nil {
			return absolute
		}
		return found
	}
	return "bast"
}

func lockCtx(ctx context.Context, mu *stdsync.Mutex) error {
	for {
		if mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (e *Engine) SyncGCP(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.gcpMu); err != nil {
		return Result{}, err
	}
	defer e.gcpMu.Unlock()

	integration := e.Store.GCP()
	discovery, err := e.GCP.DiscoverAll(ctx, cloud.DiscoverConfig{
		ProjectFilter:   integration.ProjectFilter,
		DefaultSSHUser:  integration.DefaultSSHUser,
		ServiceAccounts: integration.ServiceAccounts,
		Home:            e.Paths.Home,
		ManagedKeys:     e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.GCP()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetGCP(latest)
		return Result{Provider: gcp.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{}, err
	}

	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncGCPConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}

	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == gcp.ProviderName && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
			continue
		}
		usedAliases[host.Alias] = true
	}

	blocks := make([]sshconfig.SyncHostInput, 0, len(discovery.Instances)+len(previousBySyncID))
	aliases := make([]string, 0, len(discovery.Instances)+len(previousBySyncID))
	activeAliases := map[string]bool{}
	activeSyncIDs := map[string]bool{}
	metadataUpdates := make([]hostMetadataUpdate, 0, len(discovery.Instances))
	var metadataDeletes []string

	for _, inst := range discovery.Instances {
		activeSyncIDs[inst.SyncID] = true
		alias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok && !usedAliases[prev.Alias] {
			alias = prev.Alias
			usedAliases[alias] = true
		} else {
			alias = gcp.UniqueAlias(gcp.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}

		block := gcp.ToSyncHost(inst, alias)
		blocks = append(blocks, block)
		aliases = append(aliases, alias)
		activeAliases[alias] = true

		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: gcp.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}

	// Keep hosts from projects we could not re-scan (expired account, permission
	// denied, transient API errors). Prune when the project inventory was
	// confirmed empty/changed, or when the project was intentionally filtered out.
	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		projectID, _, _, parseErr := gcp.ParseSyncID(syncID)
		if parseErr == nil && (discovery.ConfirmedProjects[projectID] || discovery.ExcludedProjects[projectID]) {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		block, ok := previousBlocks[syncID]
		if !ok {
			// Fall back to a minimal block so the host stays selectable.
			block = sshconfig.SyncHostInput{
				Alias:      host.Alias,
				SyncSource: host.SyncSource,
				SyncID:     host.SyncID,
				HostName:   host.Alias,
			}
		}
		if usedAliases[block.Alias] && block.Alias != host.Alias {
			block.Alias = gcp.UniqueAlias(block.Alias, usedAliases)
		}
		usedAliases[block.Alias] = true
		activeSyncIDs[syncID] = true
		blocks = append(blocks, block)
		aliases = append(aliases, block.Alias)
		activeAliases[block.Alias] = true
	}

	if err := e.Config.EnsureSyncInclude(e.Paths.SyncGCPConfig); err != nil {
		return Result{}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncGCPConfig, blocks); err != nil {
		return Result{}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{}, err
	}

	latest := e.Store.GCP()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetGCP(latest); err != nil {
		return Result{}, err
	}
	result := Result{
		Provider: gcp.ProviderName,
		Count:    len(blocks),
		SyncedAt: now,
		Aliases:  aliases,
	}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func (e *Engine) SyncAWS(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.awsMu); err != nil {
		return Result{}, err
	}
	defer e.awsMu.Unlock()

	integration := e.Store.AWS()
	discovery, err := e.AWS.Discover(ctx, awscloud.DiscoverConfig{
		ProfileFilter: integration.ProfileFilter, RegionFilter: integration.RegionFilter,
		DefaultSSHUser: integration.DefaultSSHUser, Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.AWS()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetAWS(latest)
		return Result{Provider: awscloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncAWSConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == awscloud.ProviderName && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
			continue
		}
		usedAliases[host.Alias] = true
	}

	blocks := make([]sshconfig.SyncHostInput, 0, len(discovery.Instances)+len(previousBySyncID))
	aliases := make([]string, 0, len(discovery.Instances)+len(previousBySyncID))
	activeSyncIDs := map[string]bool{}
	metadataUpdates := make([]hostMetadataUpdate, 0, len(discovery.Instances))
	var metadataDeletes []string
	for _, inst := range discovery.Instances {
		activeSyncIDs[inst.SyncID] = true
		alias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok && !usedAliases[prev.Alias] {
			alias = prev.Alias
			usedAliases[alias] = true
		} else {
			alias = awscloud.UniqueAlias(awscloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, awscloud.ToSyncHost(inst, alias))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: awscloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}
	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		meta := e.Store.Host(host.Alias)
		scope := awsScopeFromHost(host, meta.Group)
		if scope != "" && discovery.ConfirmedScopes[scope] {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		if scope != "" && awsScopeExcludedByFilter(scope, integration.ProfileFilter, integration.RegionFilter) {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		block, ok := previousBlocks[syncID]
		if !ok {
			block = sshconfig.SyncHostInput{
				Alias: host.Alias, SyncSource: host.SyncSource, SyncID: host.SyncID, HostName: host.Alias,
			}
		}
		if usedAliases[block.Alias] && block.Alias != host.Alias {
			block.Alias = awscloud.UniqueAlias(block.Alias, usedAliases)
		}
		usedAliases[block.Alias] = true
		blocks = append(blocks, block)
		aliases = append(aliases, block.Alias)
	}
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncAWSConfig); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncAWSConfig, blocks); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	latest := e.Store.AWS()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetAWS(latest); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	result := Result{Provider: awscloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func awsScopeFromHost(host sshconfig.Host, group string) string {
	_, region, _, _, err := awscloud.ParseSyncID(host.SyncID)
	if err != nil || region == "" {
		return ""
	}
	profile := ""
	parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(group, "Amazon EC2/"), "AWS/"), "/")
	if len(parts) >= 1 && parts[0] != "" && parts[0] != "Amazon EC2" && parts[0] != "AWS" {
		profile = parts[0]
	}
	if profile == "" {
		return ""
	}
	return awscloud.ScopeKey(profile, region)
}

func awsScopeExcludedByFilter(scope string, profiles, regions []string) bool {
	profile, region, ok := strings.Cut(scope, "/")
	if !ok {
		return false
	}
	if len(profiles) > 0 && !stringInFold(profiles, profile) {
		return true
	}
	if len(regions) > 0 && !stringInFold(regions, region) {
		return true
	}
	return false
}

func stringInFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func (e *Engine) applyMetadataUpdates(updates []hostMetadataUpdate, deletes []string) error {
	return e.Store.UpdateHosts(func(hosts map[string]metadata.Host) {
		for _, update := range updates {
			if update.previousAlias != "" && update.previousAlias != update.alias {
				if previous, ok := hosts[update.previousAlias]; ok {
					delete(hosts, update.previousAlias)
					hosts[update.alias] = previous
				}
			}
			meta := hosts[update.alias]
			meta.Label = update.label
			meta.Group = update.group
			meta.Tags = append([]string(nil), update.tags...)
			hosts[update.alias] = meta
		}
		for _, alias := range deletes {
			delete(hosts, alias)
		}
	})
}

func (e *Engine) SyncAzure(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.azureMu); err != nil {
		return Result{}, err
	}
	defer e.azureMu.Unlock()

	integration := e.Store.Azure()
	discovery, err := e.Azure.Discover(ctx, azurecloud.DiscoverConfig{
		SubscriptionFilter: integration.SubscriptionFilter, ResourceGroupFilter: integration.ResourceGroupFilter,
		DefaultSSHUser: integration.DefaultSSHUser, Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys,
		BastExecutable: e.BastExecutable,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Azure()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetAzure(latest)
		return Result{Provider: azurecloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}
	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: azurecloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncAzureConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[strings.ToLower(block.SyncID)] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == azurecloud.ProviderName && host.SyncID != "" {
			previousBySyncID[strings.ToLower(host.SyncID)] = host
			continue
		}
		usedAliases[host.Alias] = true
	}
	blocks := make([]sshconfig.SyncHostInput, 0, len(discovery.Instances)+len(previousBySyncID))
	aliases := make([]string, 0, cap(blocks))
	activeSyncIDs := map[string]bool{}
	metadataUpdates := make([]hostMetadataUpdate, 0, len(discovery.Instances))
	var metadataDeletes []string
	for _, inst := range discovery.Instances {
		syncID := strings.ToLower(inst.SyncID)
		activeSyncIDs[syncID] = true
		alias := ""
		if prev, ok := previousBySyncID[syncID]; ok && !usedAliases[prev.Alias] {
			alias = prev.Alias
			usedAliases[alias] = true
		} else {
			alias = azurecloud.UniqueAlias(azurecloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, azurecloud.ToSyncHost(inst, alias, e.BastExecutable))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[syncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: azurecloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}
	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		subscriptionID, _, _, parseErr := azurecloud.ParseSyncID(syncID)
		if parseErr == nil && discovery.ConfirmedSubscriptions[strings.ToLower(subscriptionID)] {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		block, ok := previousBlocks[syncID]
		if !ok {
			block = sshconfig.SyncHostInput{Alias: host.Alias, SyncSource: host.SyncSource, SyncID: host.SyncID, HostName: host.Alias}
		}
		if usedAliases[block.Alias] {
			previousAlias := host.Alias
			block.Alias = azurecloud.UniqueAlias(block.Alias, usedAliases)
			meta := e.Store.Host(previousAlias)
			metadataUpdates = append([]hostMetadataUpdate{{
				alias: block.Alias, previousAlias: previousAlias, label: meta.Label,
				group: meta.Group, tags: append([]string(nil), meta.Tags...),
			}}, metadataUpdates...)
		}
		usedAliases[block.Alias] = true
		blocks = append(blocks, block)
		aliases = append(aliases, block.Alias)
	}
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncAzureConfig); err != nil {
		return Result{Provider: azurecloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncAzureConfig, blocks); err != nil {
		return Result{Provider: azurecloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: azurecloud.ProviderName}, err
	}
	latest := e.Store.Azure()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetAzure(latest); err != nil {
		return Result{Provider: azurecloud.ProviderName}, err
	}
	result := Result{Provider: azurecloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func (e *Engine) SyncBox(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.boxMu); err != nil {
		return Result{}, err
	}
	defer e.boxMu.Unlock()

	discovery, err := e.Box.Discover(ctx, boxcloud.DiscoverConfig{})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Box()
		latest.Enabled = true
		latest.Disabled = false
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetBox(latest)
		return Result{Provider: boxcloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: boxcloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncBoxConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == boxcloud.ProviderName && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
			continue
		}
		usedAliases[host.Alias] = true
	}

	blocks := make([]sshconfig.SyncHostInput, 0, len(discovery.Instances))
	aliases := make([]string, 0, len(discovery.Instances))
	activeSyncIDs := map[string]bool{}
	metadataUpdates := make([]hostMetadataUpdate, 0, len(discovery.Instances))
	var metadataDeletes []string
	for _, inst := range discovery.Instances {
		activeSyncIDs[inst.SyncID] = true
		alias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok && !usedAliases[prev.Alias] {
			alias = prev.Alias
			usedAliases[alias] = true
		} else {
			alias = boxcloud.UniqueAlias(boxcloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, boxcloud.ToSyncHost(inst, alias))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: boxcloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}
	if discovery.Complete {
		for syncID, host := range previousBySyncID {
			if activeSyncIDs[syncID] {
				continue
			}
			metadataDeletes = append(metadataDeletes, host.Alias)
		}
	} else {
		for syncID, host := range previousBySyncID {
			if activeSyncIDs[syncID] {
				continue
			}
			block, ok := previousBlocks[syncID]
			if !ok {
				block = sshconfig.SyncHostInput{
					Alias: host.Alias, SyncSource: host.SyncSource, SyncID: host.SyncID, HostName: host.Alias,
				}
			}
			usedAliases[block.Alias] = true
			blocks = append(blocks, block)
			aliases = append(aliases, block.Alias)
		}
	}
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncBoxConfig); err != nil {
		return Result{Provider: boxcloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncBoxConfig, blocks); err != nil {
		return Result{Provider: boxcloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: boxcloud.ProviderName}, err
	}
	latest := e.Store.Box()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetBox(latest); err != nil {
		return Result{Provider: boxcloud.ProviderName}, err
	}
	result := Result{Provider: boxcloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func (e *Engine) MaybeAutoConnectBox(ctx context.Context) (Result, bool, error) {
	integration := e.Store.Box()
	if integration.Disabled {
		return Result{}, false, nil
	}
	account, err := e.Box.Account(ctx)
	if err != nil || !account.Authenticated {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.SyncBox(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetBox(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.SyncBox(ctx)
	return result, true, syncErr
}

func (e *Engine) NewBox(ctx context.Context, opts boxcloud.NewOpts) (Result, string, error) {
	id, err := e.Box.New(ctx, opts)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.SyncBox(ctx)
	alias := e.AliasForBoxSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) ForkBox(ctx context.Context, syncID string, opts boxcloud.ForkOpts) (Result, string, error) {
	id, err := e.Box.Fork(ctx, syncID, opts)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.SyncBox(ctx)
	alias := e.AliasForBoxSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopBox(ctx context.Context, syncID string) (Result, error) {
	if err := e.Box.Stop(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncBox(ctx)
}

func (e *Engine) ResumeBox(ctx context.Context, syncID string, opts boxcloud.ResumeOpts) (Result, error) {
	if err := e.Box.Resume(ctx, syncID, opts); err != nil {
		return Result{}, err
	}
	return e.SyncBox(ctx)
}

func (e *Engine) ResolveBoxSyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if id, err := boxcloud.ParseSyncID(hostOrID); err == nil {
		return id, nil
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == boxcloud.ProviderName &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID)) {
			return host.SyncID, nil
		}
	}
	// Also match metadata labels.
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != boxcloud.ProviderName {
			continue
		}
		meta := e.Store.Host(host.Alias)
		if meta.Label != "" && strings.EqualFold(meta.Label, hostOrID) {
			matches = append(matches, host.SyncID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("box label %q matches %d hosts; pass an alias or a bx_ id", hostOrID, len(matches))
	}
	return "", fmt.Errorf("box host %q not found; sync with bast sync box or pass a bx_ id", hostOrID)
}

func (e *Engine) AliasForBoxSyncID(ctx context.Context, syncID string) string {
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == boxcloud.ProviderName && host.SyncID == syncID {
			return host.Alias
		}
	}
	return ""
}

func (e *Engine) EnsureGCPAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.gcpMu); err != nil {
		return err
	}
	defer e.gcpMu.Unlock()

	if !host.Synced || host.SyncSource != gcp.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.GCP()
	cfg := gcp.EnsureConfig{
		Home:            e.Paths.Home,
		ManagedKeys:     e.Paths.ManagedKeys,
		DefaultSSHUser:  integration.DefaultSSHUser,
		ServiceAccounts: integration.ServiceAccounts,
		Status:          status,
	}
	if e.EnsureAccessWait != 0 {
		cfg.PropagationWait = e.EnsureAccessWait
	}
	result, err := e.GCP.EnsureAccess(ctx, host.SyncID, cfg)
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuth(
		e.Paths.SyncGCPConfig,
		host.Alias,
		result.User,
		result.IdentityFile,
		"",
		result.IdentitiesOnly,
	)
}

func (e *Engine) EnsureAWSAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.awsMu); err != nil {
		return err
	}
	defer e.awsMu.Unlock()
	if !host.Synced || host.SyncSource != awscloud.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.AWS()
	result, err := e.AWS.EnsureAccess(ctx, host.SyncID, awscloud.EnsureConfig{
		Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys, ProfileFilter: integration.ProfileFilter,
		DefaultSSHUser: integration.DefaultSSHUser, Status: status,
	})
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuth(e.Paths.SyncAWSConfig, host.Alias, result.User, result.IdentityFile, "", result.IdentitiesOnly)
}

func (e *Engine) EnsureAzureAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.azureMu); err != nil {
		return err
	}
	defer e.azureMu.Unlock()
	if !host.Synced || host.SyncSource != azurecloud.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.Azure()
	result, err := e.Azure.EnsureAccess(ctx, host.SyncID, azurecloud.EnsureConfig{
		Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys, AzureDir: e.Paths.AzureDir,
		DefaultSSHUser: integration.DefaultSSHUser, Status: status,
	})
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuth(e.Paths.SyncAzureConfig, host.Alias, result.User, result.IdentityFile, result.CertificateFile, result.IdentitiesOnly)
}

func (e *Engine) EnsureBoxAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.boxMu); err != nil {
		return err
	}
	defer e.boxMu.Unlock()
	if !host.Synced || host.SyncSource != boxcloud.ProviderName || host.SyncID == "" {
		return nil
	}
	result, err := e.Box.EnsureAccess(ctx, host.SyncID, boxcloud.EnsureConfig{
		Home: e.Paths.Home, Status: status,
	})
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuthAndHost(
		e.Paths.SyncBoxConfig, host.Alias, result.HostName, result.User, result.IdentityFile, "", result.IdentitiesOnly,
	)
}

func (e *Engine) DisableGCP(ctx context.Context) error {
	if err := lockCtx(ctx, &e.gcpMu); err != nil {
		return err
	}
	defer e.gcpMu.Unlock()

	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == gcp.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncGCPConfig); err != nil {
		return err
	}
	integration := e.Store.GCP()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetGCP(integration)
}

func (e *Engine) DisableAWS(ctx context.Context) error {
	if err := lockCtx(ctx, &e.awsMu); err != nil {
		return err
	}
	defer e.awsMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == awscloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncAWSConfig); err != nil {
		return err
	}
	integration := e.Store.AWS()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetAWS(integration)
}

func (e *Engine) DisableAzure(ctx context.Context) error {
	if err := lockCtx(ctx, &e.azureMu); err != nil {
		return err
	}
	defer e.azureMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == azurecloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncAzureConfig); err != nil {
		return err
	}
	integration := e.Store.Azure()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetAzure(integration)
}

func (e *Engine) DisableBox(ctx context.Context) error {
	if err := lockCtx(ctx, &e.boxMu); err != nil {
		return err
	}
	defer e.boxMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == boxcloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncBoxConfig); err != nil {
		return err
	}
	integration := e.Store.Box()
	integration.Enabled = false
	integration.AutoSync = false
	integration.Disabled = true
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetBox(integration)
}

func (e *Engine) Status(ctx context.Context) (Status, error) {
	integration := e.Store.GCP()
	awsIntegration := e.Store.AWS()
	azureIntegration := e.Store.Azure()
	boxIntegration := e.Store.Box()
	status := Status{
		GCP: GCPStatus{
			Enabled:           integration.Enabled,
			AutoSync:          integration.AutoSync,
			DefaultSSHUser:    integration.DefaultSSHUser,
			ProjectFilter:     append([]string(nil), integration.ProjectFilter...),
			ServiceAccounts:   append([]string(nil), integration.ServiceAccounts...),
			LastSyncAt:        integration.LastSyncAt,
			LastSyncError:     integration.LastSyncError,
			LastInstanceCount: integration.LastInstanceCount,
		},
		AWS: AWSStatus{
			Enabled: awsIntegration.Enabled, AutoSync: awsIntegration.AutoSync,
			ProfileFilter:  append([]string(nil), awsIntegration.ProfileFilter...),
			RegionFilter:   append([]string(nil), awsIntegration.RegionFilter...),
			DefaultSSHUser: awsIntegration.DefaultSSHUser, LastSyncAt: awsIntegration.LastSyncAt,
			LastSyncError: awsIntegration.LastSyncError, LastInstanceCount: awsIntegration.LastInstanceCount,
		},
		Azure: AzureStatus{
			Enabled: azureIntegration.Enabled, AutoSync: azureIntegration.AutoSync,
			SubscriptionFilter:  append([]string(nil), azureIntegration.SubscriptionFilter...),
			ResourceGroupFilter: append([]string(nil), azureIntegration.ResourceGroupFilter...),
			DefaultSSHUser:      azureIntegration.DefaultSSHUser, LastSyncAt: azureIntegration.LastSyncAt,
			LastSyncError: azureIntegration.LastSyncError, LastInstanceCount: azureIntegration.LastInstanceCount,
		},
		Box: BoxStatus{
			Enabled: boxIntegration.Enabled, AutoSync: boxIntegration.AutoSync, Disabled: boxIntegration.Disabled,
			LastSyncAt: boxIntegration.LastSyncAt, LastSyncError: boxIntegration.LastSyncError,
			LastInstanceCount: boxIntegration.LastInstanceCount,
		},
	}

	var probes stdsync.WaitGroup
	probes.Add(4)
	go func() {
		defer probes.Done()
		if err := e.GCP.CheckAvailable(ctx); err != nil {
			status.GCP.GCloudError = err.Error()
		} else {
			accounts, listErr := e.GCP.ListAccounts(ctx)
			if listErr != nil {
				status.GCP.GCloudError = listErr.Error()
			} else {
				for _, account := range accounts {
					status.GCP.Accounts = append(status.GCP.Accounts, account.Account)
				}
			}
		}
	}()
	go func() {
		defer probes.Done()
		if err := e.AWS.CheckAvailable(ctx); err != nil {
			status.AWS.AWSCLIError = err.Error()
		} else {
			profiles, listErr := e.AWS.ListProfiles(ctx)
			if listErr != nil {
				status.AWS.AWSCLIError = listErr.Error()
			} else {
				status.AWS.Profiles = profiles
			}
		}
	}()
	go func() {
		defer probes.Done()
		if err := e.Azure.CheckAvailable(ctx); err != nil {
			status.Azure.AzureCLIError = err.Error()
		} else {
			subscriptions, listErr := e.Azure.ListSubscriptions(ctx)
			if listErr != nil {
				status.Azure.AzureCLIError = listErr.Error()
			} else {
				for _, subscription := range subscriptions {
					status.Azure.Subscriptions = append(status.Azure.Subscriptions, subscription.Name)
				}
			}
			if err := e.Azure.CheckExtension(ctx, "ssh"); err != nil {
				status.Azure.SSHExtensionError = err.Error()
			}
			if err := e.Azure.CheckExtension(ctx, "bastion"); err != nil {
				status.Azure.BastionExtensionError = err.Error()
			}
		}
	}()
	go func() {
		defer probes.Done()
		account, err := e.Box.Account(ctx)
		if err != nil {
			status.Box.BoxCLIError = err.Error()
			return
		}
		if account.Error != "" && !account.Authenticated {
			status.Box.BoxCLIError = account.Error
		}
		status.Box.Authenticated = account.Authenticated
		status.Box.Login = account.Login
		if status.Box.Login == "" {
			status.Box.Login = account.Email
		}
		status.Box.Plan = account.Plan
	}()
	probes.Wait()
	return status, nil
}

type Status struct {
	GCP   GCPStatus   `json:"gcp"`
	AWS   AWSStatus   `json:"aws"`
	Azure AzureStatus `json:"azure"`
	Box   BoxStatus   `json:"box"`
}

type GCPStatus struct {
	Enabled           bool       `json:"enabled"`
	AutoSync          bool       `json:"autoSync"`
	Accounts          []string   `json:"accounts,omitempty"`
	ServiceAccounts   []string   `json:"serviceAccounts,omitempty"`
	ProjectFilter     []string   `json:"projectFilter,omitempty"`
	DefaultSSHUser    string     `json:"defaultSshUser,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
	GCloudError       string     `json:"gcloudError,omitempty"`
}

type AWSStatus struct {
	Enabled           bool       `json:"enabled"`
	AutoSync          bool       `json:"autoSync"`
	Profiles          []string   `json:"profiles,omitempty"`
	ProfileFilter     []string   `json:"profileFilter,omitempty"`
	RegionFilter      []string   `json:"regionFilter,omitempty"`
	DefaultSSHUser    string     `json:"defaultSshUser,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
	AWSCLIError       string     `json:"awsCliError,omitempty"`
}

type AzureStatus struct {
	Enabled               bool       `json:"enabled"`
	AutoSync              bool       `json:"autoSync"`
	Subscriptions         []string   `json:"subscriptions,omitempty"`
	SubscriptionFilter    []string   `json:"subscriptionFilter,omitempty"`
	ResourceGroupFilter   []string   `json:"resourceGroupFilter,omitempty"`
	DefaultSSHUser        string     `json:"defaultSshUser,omitempty"`
	LastSyncAt            *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError         string     `json:"lastSyncError,omitempty"`
	LastInstanceCount     int        `json:"lastInstanceCount,omitempty"`
	AzureCLIError         string     `json:"azureCliError,omitempty"`
	SSHExtensionError     string     `json:"sshExtensionError,omitempty"`
	BastionExtensionError string     `json:"bastionExtensionError,omitempty"`
}

type BoxStatus struct {
	Enabled           bool       `json:"enabled"`
	AutoSync          bool       `json:"autoSync"`
	Disabled          bool       `json:"disabled,omitempty"`
	Authenticated     bool       `json:"authenticated,omitempty"`
	Login             string     `json:"login,omitempty"`
	Plan              string     `json:"plan,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
	BoxCLIError       string     `json:"boxCliError,omitempty"`
}

func IsSyncedGroup(group string) bool {
	group = strings.TrimSpace(group)
	return group == "Google Cloud" || strings.HasPrefix(group, "Google Cloud/") ||
		group == "GCP" || strings.HasPrefix(group, "GCP/") ||
		group == "Amazon EC2" || strings.HasPrefix(group, "Amazon EC2/") ||
		group == "AWS" || strings.HasPrefix(group, "AWS/") ||
		group == "Microsoft Azure" || strings.HasPrefix(group, "Microsoft Azure/") ||
		group == "Box" || strings.HasPrefix(group, "Box/")
}

func ValidateServiceAccountPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("service account path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("service account key: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("service account key must be a file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read service account key: %w", err)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("service account key is not valid JSON")
	}
	return nil
}
