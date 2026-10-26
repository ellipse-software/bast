package sync

import (
	"context"
	"strings"
	"time"

	docloud "bast/internal/cloud/digitalocean"
	"bast/internal/sshconfig"
)

func (e *Engine) SyncDigitalOcean(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return Result{}, err
	}
	defer e.digitalOceanMu.Unlock()
	return e.syncDigitalOceanLocked(ctx)
}

func (e *Engine) syncDigitalOceanLocked(ctx context.Context) (Result, error) {
	integration := e.Store.DigitalOcean()
	discovery, err := e.DigitalOcean.Discover(ctx, docloud.DiscoverConfig{
		ContextFilter: integration.ContextFilter, RegionFilter: integration.RegionFilter,
		DefaultSSHUser: integration.DefaultSSHUser, Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.DigitalOcean()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetDigitalOcean(latest)
		return Result{Provider: docloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: docloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncDigitalOceanConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		usedAliases[host.Alias] = true
		if host.Synced && host.SyncSource == docloud.ProviderName && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
		}
	}

	blocks := make([]sshconfig.SyncHostInput, 0, len(discovery.Instances)+len(previousBySyncID))
	aliases := make([]string, 0, len(discovery.Instances)+len(previousBySyncID))
	activeSyncIDs := map[string]bool{}
	metadataUpdates := make([]hostMetadataUpdate, 0, len(discovery.Instances))
	var metadataDeletes []string
	for _, inst := range discovery.Instances {
		activeSyncIDs[inst.SyncID] = true
		alias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			alias = prev.Alias
		} else {
			alias = docloud.UniqueAlias(docloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, docloud.ToSyncHost(inst, alias))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: docloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}
	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		meta := e.Store.Host(host.Alias)
		contextName, region := digitalOceanContextRegion(meta.Group)
		if contextName != "" && discovery.ConfirmedContexts[contextName] {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		if contextName != "" && region != "" && discovery.ConfirmedScopes[docloud.ScopeKey(contextName, region)] {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		if digitalOceanScopeExcluded(contextName, region, integration.ContextFilter, integration.RegionFilter) {
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
			block.Alias = docloud.UniqueAlias(block.Alias, usedAliases)
		}
		usedAliases[block.Alias] = true
		blocks = append(blocks, block)
		aliases = append(aliases, block.Alias)
	}
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncDigitalOceanConfig); err != nil {
		return Result{Provider: docloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncDigitalOceanConfig, blocks); err != nil {
		return Result{Provider: docloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: docloud.ProviderName}, err
	}
	latest := e.Store.DigitalOcean()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetDigitalOcean(latest); err != nil {
		return Result{Provider: docloud.ProviderName}, err
	}
	result := Result{Provider: docloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func digitalOceanContextRegion(group string) (contextName, region string) {
	rest := strings.TrimPrefix(strings.TrimSpace(group), "DigitalOcean")
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return "", ""
	}
	contextName, region, _ = strings.Cut(rest, "/")
	return contextName, region
}

func digitalOceanScopeExcluded(contextName, region string, contexts, regions []string) bool {
	if contextName == "" {
		return false
	}
	if len(contexts) > 0 && !stringInFold(contexts, contextName) {
		return true
	}
	if len(regions) > 0 && region != "" && !stringInFold(regions, region) {
		return true
	}
	return false
}

func (e *Engine) EnsureDigitalOceanAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return err
	}
	defer e.digitalOceanMu.Unlock()
	if !host.Synced || host.SyncSource != docloud.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.DigitalOcean()
	result, err := e.DigitalOcean.EnsureAccess(ctx, host.SyncID, docloud.EnsureConfig{
		Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys, ContextFilter: integration.ContextFilter,
		DefaultSSHUser: integration.DefaultSSHUser, Status: status,
	})
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuthAndHost(
		e.Paths.SyncDigitalOceanConfig, host.Alias, result.HostName, result.User, result.IdentityFile, "", result.IdentitiesOnly,
	)
}

func (e *Engine) DisableDigitalOcean(ctx context.Context) error {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return err
	}
	defer e.digitalOceanMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == docloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncDigitalOceanConfig); err != nil {
		return err
	}
	integration := e.Store.DigitalOcean()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetDigitalOcean(integration)
}
