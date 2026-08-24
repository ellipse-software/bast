package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	railwaycloud "bast/internal/cloud/railway"
	"bast/internal/sshconfig"
)

func (e *Engine) SyncRailway(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.railwayMu); err != nil {
		return Result{}, err
	}
	defer e.railwayMu.Unlock()

	_ = e.Railway.PersistResolvedToken()
	e.configureRailwayIdentity()
	if _, err := e.Railway.EnsureIdentity(ctx, nil); err != nil {
		now := time.Now().UTC()
		latest := e.Store.Railway()
		latest.Enabled = true
		latest.Disabled = false
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetRailway(latest)
		return Result{Provider: railwaycloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}
	discovery, err := e.Railway.Discover(ctx)
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Railway()
		latest.Enabled = true
		latest.Disabled = false
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetRailway(latest)
		return Result{Provider: railwaycloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: railwaycloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncRailwayConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == railwaycloud.ProviderName && host.SyncID != "" {
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
			alias = railwaycloud.UniqueAlias(railwaycloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, railwaycloud.ToSyncHost(inst, alias))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: railwaycloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
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
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncRailwayConfig); err != nil {
		return Result{Provider: railwaycloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncRailwayConfig, blocks); err != nil {
		return Result{Provider: railwaycloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: railwaycloud.ProviderName}, err
	}
	latest := e.Store.Railway()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetRailway(latest); err != nil {
		return Result{Provider: railwaycloud.ProviderName}, err
	}
	result := Result{Provider: railwaycloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func (e *Engine) MaybeAutoConnectRailway(ctx context.Context) (Result, bool, error) {
	integration := e.Store.Railway()
	if integration.Disabled {
		return Result{}, false, nil
	}
	if !e.Railway.HasToken() {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.SyncRailway(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetRailway(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.SyncRailway(ctx)
	return result, true, syncErr
}

func (e *Engine) SaveRailwayToken(ctx context.Context, token string) (Result, error) {
	if err := e.Railway.SaveToken(token); err != nil {
		return Result{}, err
	}
	return e.SyncRailway(ctx)
}

func (e *Engine) NewRailway(ctx context.Context, opts railwaycloud.CreateOpts) (Result, string, error) {
	e.configureRailwayIdentity()
	inst, err := e.Railway.Create(ctx, opts)
	if err != nil && inst.SyncID == "" {
		return Result{}, "", err
	}
	result, syncErr := e.SyncRailway(ctx)
	alias := e.AliasForRailwaySyncID(ctx, inst.SyncID)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopRailway(ctx context.Context, syncID string) (Result, error) {
	if err := e.Railway.Stop(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncRailway(ctx)
}

func (e *Engine) ResumeRailway(ctx context.Context, syncID string) (Result, error) {
	if err := e.Railway.Resume(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncRailway(ctx)
}

func (e *Engine) DeleteRailway(ctx context.Context, syncID string) (Result, error) {
	if err := e.Railway.Delete(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncRailway(ctx)
}

func (e *Engine) ResolveRailwaySyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if _, _, _, err := railwaycloud.ParseSyncID(hostOrID); err == nil {
		if _, getErr := e.Railway.GetInstance(ctx, hostOrID); getErr == nil {
			return hostOrID, nil
		}
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == railwaycloud.ProviderName &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID)) {
			return host.SyncID, nil
		}
	}
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != railwaycloud.ProviderName {
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
		return "", fmt.Errorf("railway label %q matches %d hosts; pass an alias or sync id", hostOrID, len(matches))
	}
	return "", fmt.Errorf("railway host %q not found; sync with bast sync railway", hostOrID)
}

func (e *Engine) AliasForRailwaySyncID(ctx context.Context, syncID string) string {
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == railwaycloud.ProviderName && host.SyncID == syncID {
			return host.Alias
		}
	}
	return ""
}

func (e *Engine) EnsureRailwayAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.railwayMu); err != nil {
		return err
	}
	defer e.railwayMu.Unlock()
	if !host.Synced || host.SyncSource != railwaycloud.ProviderName || host.SyncID == "" {
		return nil
	}
	if status != nil {
		status("Checking Railway access…")
	}
	if err := e.Railway.PersistResolvedToken(); err != nil {
		return err
	}
	e.configureRailwayIdentity()
	identity, err := e.Railway.EnsureIdentity(ctx, status)
	if err != nil {
		return err
	}
	info, err := e.Railway.GetInstance(ctx, host.SyncID)
	if err != nil {
		return err
	}
	if railwaycloud.IsStoppedState(info.State) {
		return fmt.Errorf("railway service %s is stopped; resume it first with bast railway resume %s", info.Name, info.SyncID)
	}
	if info.State == "starting" {
		if status != nil {
			status("Waiting for Railway service…")
		}
		if err := e.Railway.WaitReady(ctx, info.SyncID, 5*time.Minute); err != nil {
			return err
		}
		info, err = e.Railway.GetInstance(ctx, host.SyncID)
		if err != nil {
			return err
		}
	}
	if info.User == "" {
		return fmt.Errorf("railway service %s has no SSH target yet", info.Name)
	}
	return sshconfig.UpdateSyncHostAuthAndHost(
		e.Paths.SyncRailwayConfig, host.Alias, railwaycloud.SSHHost, info.User, identity, "", true,
	)
}

func (e *Engine) DisableRailway(ctx context.Context) error {
	if err := lockCtx(ctx, &e.railwayMu); err != nil {
		return err
	}
	defer e.railwayMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == railwaycloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncRailwayConfig); err != nil {
		return err
	}
	integration := e.Store.Railway()
	integration.Enabled = false
	integration.AutoSync = false
	integration.Disabled = true
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetRailway(integration)
}

func (e *Engine) configureRailwayIdentity() {
	if e.Railway == nil {
		return
	}
	if strings.TrimSpace(e.Railway.ManagedKeys) == "" {
		e.Railway.ManagedKeys = e.Paths.ManagedKeys
	}
	if strings.TrimSpace(e.Railway.IdentityFile) == "" && e.Paths.ManagedKeys != "" {
		e.Railway.IdentityFile = filepath.Join(e.Paths.ManagedKeys, railwaycloud.IdentityName)
	}
}
