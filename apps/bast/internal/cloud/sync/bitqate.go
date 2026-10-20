package sync

import (
	"context"
	"strings"
	"time"

	bitqatecloud "bast/internal/cloud/bitqate"
	"bast/internal/sshconfig"
)

func (e *Engine) SyncBitqate(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.bitqateMu); err != nil {
		return Result{}, err
	}
	defer e.bitqateMu.Unlock()
	return e.syncBitqateLocked(ctx)
}

func (e *Engine) syncBitqateLocked(ctx context.Context) (Result, error) {
	integration := e.Store.Bitqate()
	discovery, err := e.Bitqate.Discover(ctx, bitqatecloud.DiscoverConfig{
		RegionFilter:   integration.RegionFilter,
		DefaultSSHUser: integration.DefaultSSHUser,
		DefaultSSHPort: integration.DefaultSSHPort,
		Home:           e.Paths.Home,
		ManagedKeys:    e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Bitqate()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetBitqate(latest)
		return Result{Provider: bitqatecloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: bitqatecloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncBitqateConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == bitqatecloud.ProviderName && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
			continue
		}
		usedAliases[host.Alias] = true
	}
	defaultUserSet := strings.TrimSpace(integration.DefaultSSHUser) != ""

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
			alias = bitqatecloud.UniqueAlias(bitqatecloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		block := bitqatecloud.ToSyncHost(inst, alias)
		applyBitqateSSHHint(&block, localSSHHint(inst.HostName, existing), previousBlocks[inst.SyncID], defaultUserSet)
		blocks = append(blocks, block)
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: bitqatecloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}
	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		meta := e.Store.Host(host.Alias)
		region := bitqatecloud.RegionFromGroup(meta.Group)
		if region != "" && bitqateRegionExcluded(region, integration.RegionFilter) {
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
			block.Alias = bitqatecloud.UniqueAlias(block.Alias, usedAliases)
		}
		usedAliases[block.Alias] = true
		blocks = append(blocks, block)
		aliases = append(aliases, block.Alias)
	}
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncBitqateConfig); err != nil {
		return Result{Provider: bitqatecloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncBitqateConfig, blocks); err != nil {
		return Result{Provider: bitqatecloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: bitqatecloud.ProviderName}, err
	}
	latest := e.Store.Bitqate()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetBitqate(latest); err != nil {
		return Result{Provider: bitqatecloud.ProviderName}, err
	}
	result := Result{Provider: bitqatecloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func bitqateRegionExcluded(region string, filter []string) bool {
	if len(filter) == 0 {
		return false
	}
	for _, value := range filter {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(region)) {
			return false
		}
	}
	return true
}

func (e *Engine) EnsureBitqateAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.bitqateMu); err != nil {
		return err
	}
	defer e.bitqateMu.Unlock()
	if !host.Synced || host.SyncSource != bitqatecloud.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.Bitqate()
	currentIdentity := ""
	if len(host.Resolved.IdentityFiles) > 0 {
		currentIdentity = host.Resolved.IdentityFiles[0]
	}
	result, err := e.Bitqate.EnsureAccess(ctx, host.SyncID, bitqatecloud.EnsureConfig{
		Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys, DefaultSSHUser: integration.DefaultSSHUser,
		DefaultSSHPort: integration.DefaultSSHPort,
		CurrentUser:    host.Resolved.User, CurrentPort: host.Resolved.Port, CurrentIdentity: currentIdentity,
		CurrentIdentitiesOnly: strings.EqualFold(host.Resolved.IdentitiesOnly, "yes"), Status: status,
	})
	if err != nil {
		return err
	}
	if existing, discoverErr := e.Discover(ctx); discoverErr == nil {
		applyBitqateEnsureSSHHint(&result, localSSHHint(result.HostName, existing), strings.TrimSpace(integration.DefaultSSHUser) != "")
	}
	return sshconfig.UpdateSyncHostDetails(
		e.Paths.SyncBitqateConfig, host.Alias, result.HostName, result.User, result.IdentityFile, "", result.Port, result.IdentitiesOnly,
	)
}

func applyBitqateSSHHint(block *sshconfig.SyncHostInput, hint sshHint, prev sshconfig.SyncHostInput, defaultUserSet bool) {
	if block.Port == "" && prev.Port != "" {
		block.Port = prev.Port
	}
	if !defaultUserSet && (block.User == "" || block.User == "root") && prev.User != "" && prev.User != "root" {
		block.User = prev.User
	}
	if block.IdentityFile == "" && prev.IdentityFile != "" {
		block.IdentityFile = prev.IdentityFile
		block.IdentitiesOnly = prev.IdentitiesOnly
	}
	if !defaultUserSet && (block.User == "" || block.User == "root") && strings.TrimSpace(hint.user) != "" {
		block.User = hint.user
	}
	if block.Port == "" && configuredNonDefaultPort(hint.port) {
		block.Port = strings.TrimSpace(hint.port)
	}
	if block.IdentityFile == "" && strings.TrimSpace(hint.identity) != "" {
		block.IdentityFile = hint.identity
		block.IdentitiesOnly = hint.identitiesOnly
	}
}

func applyBitqateEnsureSSHHint(result *bitqatecloud.EnsureResult, hint sshHint, defaultUserSet bool) {
	if !defaultUserSet && (result.User == "" || result.User == "root") && strings.TrimSpace(hint.user) != "" {
		result.User = hint.user
	}
	if result.Port == "" && configuredNonDefaultPort(hint.port) {
		result.Port = strings.TrimSpace(hint.port)
	}
	if result.IdentityFile == "" && strings.TrimSpace(hint.identity) != "" {
		result.IdentityFile = hint.identity
		result.IdentitiesOnly = hint.identitiesOnly
	}
}

func (e *Engine) StartBitqate(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.bitqateMu); err != nil {
		return Result{}, err
	}
	defer e.bitqateMu.Unlock()
	if err := e.Bitqate.Start(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncBitqateLocked(ctx)
}

func (e *Engine) StopBitqate(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.bitqateMu); err != nil {
		return Result{}, err
	}
	defer e.bitqateMu.Unlock()
	if err := e.Bitqate.Stop(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncBitqateLocked(ctx)
}

func (e *Engine) RestartBitqate(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.bitqateMu); err != nil {
		return Result{}, err
	}
	defer e.bitqateMu.Unlock()
	if err := e.Bitqate.Restart(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncBitqateLocked(ctx)
}

func (e *Engine) SaveBitqateKey(ctx context.Context, key string) (Result, error) {
	if err := e.Bitqate.SaveToken(key); err != nil {
		return Result{}, err
	}
	return e.SyncBitqate(ctx)
}

func (e *Engine) DeleteBitqateToken(ctx context.Context) (Result, error) {
	if err := e.Bitqate.DeleteToken(); err != nil {
		return Result{}, err
	}
	return e.SyncBitqate(ctx)
}

func (e *Engine) DisableBitqate(ctx context.Context) error {
	if err := lockCtx(ctx, &e.bitqateMu); err != nil {
		return err
	}
	defer e.bitqateMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	if err := e.deleteSyncedHostMetadata(existing, bitqatecloud.ProviderName); err != nil {
		return err
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncBitqateConfig); err != nil {
		return err
	}
	integration := e.Store.Bitqate()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetBitqate(integration)
}

func (e *Engine) ResolveBitqateSyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if id, err := bitqatecloud.ParseSyncID(hostOrID); err == nil {
		return bitqatecloud.FormatSyncID(id), nil
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	if aliasID, labels := e.matchSyncedID(hosts, bitqatecloud.ProviderName, hostOrID); aliasID != "" {
		return aliasID, nil
	} else if len(labels) == 1 {
		return labels[0], nil
	} else {
		return "", resolveMatchError("BitQate", hostOrID, "pass an alias or bitqate/<id>", "sync with bast sync bitqate", labels)
	}
}
