package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	flycloud "bast/internal/cloud/fly"
	"bast/internal/sshconfig"
)

func (e *Engine) SyncFly(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return Result{}, err
	}
	defer e.flyMu.Unlock()
	return e.syncFlyLocked(ctx)
}

func (e *Engine) syncFlyLocked(ctx context.Context) (Result, error) {
	integration := e.Store.Fly()
	discovery, err := e.Fly.Discover(ctx, flycloud.DiscoverConfig{
		OrgFilter:      integration.OrgFilter,
		AppFilter:      integration.AppFilter,
		DefaultSSHUser: integration.DefaultSSHUser,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Fly()
		latest.Enabled = true
		latest.Disabled = false
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetFly(latest)
		return Result{Provider: flycloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: flycloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncFlyConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == flycloud.ProviderName && host.SyncID != "" {
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
			alias = flycloud.UniqueAlias(flycloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		block := flycloud.ToSyncHost(inst, alias, e.BastExecutable)
		if prev, ok := previousBlocks[inst.SyncID]; ok {
			if block.IdentityFile == "" {
				block.IdentityFile = prev.IdentityFile
				block.CertificateFile = prev.CertificateFile
				block.IdentitiesOnly = prev.IdentitiesOnly || block.IdentitiesOnly
			}
		}
		blocks = append(blocks, block)
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: flycloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}

	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		if shouldPruneFlyHost(syncID, discovery) {
			metadataDeletes = append(metadataDeletes, host.Alias)
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

	if err := e.Config.EnsureSyncInclude(e.Paths.SyncFlyConfig); err != nil {
		return Result{Provider: flycloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncFlyConfig, blocks); err != nil {
		return Result{Provider: flycloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: flycloud.ProviderName}, err
	}
	latest := e.Store.Fly()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetFly(latest); err != nil {
		return Result{Provider: flycloud.ProviderName}, err
	}
	result := Result{Provider: flycloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func shouldPruneFlyHost(syncID string, discovery flycloud.Discovery) bool {
	org, app, _, err := flycloud.ParseSyncID(syncID)
	if err != nil {
		return true
	}
	appKey := org + "/" + app
	if discovery.ExcludedOrgs[org] || discovery.ExcludedApps[appKey] {
		return true
	}
	if discovery.ConfirmedApps[appKey] {
		return true
	}
	if discovery.ConfirmedOrgs[org] && !discovery.ListedApps[appKey] {
		return true
	}
	return false
}

func (e *Engine) MaybeAutoConnectFly(ctx context.Context) (Result, bool, error) {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return Result{}, false, err
	}
	defer e.flyMu.Unlock()
	integration := e.Store.Fly()
	if integration.Disabled {
		return Result{}, false, nil
	}
	account, err := e.Fly.Account(ctx)
	if err != nil || !account.Authenticated {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.syncFlyLocked(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetFly(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.syncFlyLocked(ctx)
	return result, true, syncErr
}

func (e *Engine) NewFly(ctx context.Context, opts flycloud.CreateOpts) (Result, string, error) {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return Result{}, "", err
	}
	defer e.flyMu.Unlock()
	id, err := e.Fly.Create(ctx, opts)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncFlyLocked(ctx)
	syncID := flycloud.FormatSyncID(opts.Org, opts.App, id)
	if opts.Org == "" {
		syncID = id
	}
	alias := e.AliasForFlySyncID(ctx, syncID)
	if alias == "" {
		alias = e.AliasForFlyMachine(ctx, opts.App, id)
	}
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) ForkFly(ctx context.Context, syncID string, opts flycloud.ForkOpts) (Result, string, error) {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return Result{}, "", err
	}
	defer e.flyMu.Unlock()
	org, app, id, err := flycloud.ParseSyncID(syncID)
	if err != nil {
		return Result{}, "", err
	}
	newID, err := e.Fly.Clone(ctx, org, app, id, opts)
	if err != nil && newID == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncFlyLocked(ctx)
	alias := e.AliasForFlyMachine(ctx, app, newID)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopFly(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return Result{}, err
	}
	defer e.flyMu.Unlock()
	org, app, id, err := flycloud.ParseSyncID(syncID)
	if err != nil {
		return Result{}, err
	}
	if err := e.Fly.Stop(ctx, org, app, id); err != nil {
		return Result{}, err
	}
	return e.syncFlyLocked(ctx)
}

func (e *Engine) ResumeFly(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return Result{}, err
	}
	defer e.flyMu.Unlock()
	org, app, id, err := flycloud.ParseSyncID(syncID)
	if err != nil {
		return Result{}, err
	}
	if err := e.Fly.Start(ctx, org, app, id); err != nil {
		return Result{}, err
	}
	return e.syncFlyLocked(ctx)
}

func (e *Engine) DeleteFly(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return Result{}, err
	}
	defer e.flyMu.Unlock()
	org, app, id, err := flycloud.ParseSyncID(syncID)
	if err != nil {
		return Result{}, err
	}
	if err := e.Fly.Destroy(ctx, org, app, id); err != nil {
		return Result{}, err
	}
	return e.syncFlyLocked(ctx)
}

func (e *Engine) ResolveFlySyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if _, _, _, err := flycloud.ParseSyncID(hostOrID); err == nil {
		return hostOrID, nil
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == flycloud.ProviderName &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID) || host.SyncID == hostOrID) {
			return host.SyncID, nil
		}
		if host.Synced && host.SyncSource == flycloud.ProviderName {
			if _, _, machine, parseErr := flycloud.ParseSyncID(host.SyncID); parseErr == nil && machine == hostOrID {
				return host.SyncID, nil
			}
		}
	}
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != flycloud.ProviderName {
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
		return "", fmt.Errorf("fly label %q matches %d hosts; pass an alias or org/app/machine id", hostOrID, len(matches))
	}
	return "", fmt.Errorf("fly host %q not found; sync with bast sync fly", hostOrID)
}

func (e *Engine) AliasForFlySyncID(ctx context.Context, syncID string) string {
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == flycloud.ProviderName && host.SyncID == syncID {
			return host.Alias
		}
	}
	return ""
}

func (e *Engine) AliasForFlyMachine(ctx context.Context, app, id string) string {
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != flycloud.ProviderName {
			continue
		}
		_, hostApp, machine, err := flycloud.ParseSyncID(host.SyncID)
		if err == nil && hostApp == app && machine == id {
			return host.Alias
		}
	}
	return ""
}

func (e *Engine) EnsureFlyAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return err
	}
	defer e.flyMu.Unlock()
	if !host.Synced || host.SyncSource != flycloud.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.Fly()
	result, err := e.Fly.EnsureAccess(ctx, host.SyncID, flycloud.EnsureConfig{
		Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys,
		DefaultSSHUser: integration.DefaultSSHUser, Status: status,
	})
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuthAndHost(
		e.Paths.SyncFlyConfig, host.Alias, result.HostName, result.User,
		result.IdentityFile, result.CertificateFile, result.IdentitiesOnly,
	)
}

func (e *Engine) DisableFly(ctx context.Context) error {
	if err := lockCtx(ctx, &e.flyMu); err != nil {
		return err
	}
	defer e.flyMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == flycloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncFlyConfig); err != nil {
		return err
	}
	integration := e.Store.Fly()
	integration.Enabled = false
	integration.AutoSync = false
	integration.Disabled = true
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetFly(integration)
}
