package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	upstashcloud "bast/internal/cloud/upstash"
	"bast/internal/sshconfig"
)

func (e *Engine) SyncUpstash(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return Result{}, err
	}
	defer e.upstashMu.Unlock()

	_ = e.Upstash.PersistResolvedKey()
	discovery, err := e.Upstash.Discover(ctx, struct{}{})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Upstash()
		latest.Enabled = true
		latest.Disabled = false
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetUpstash(latest)
		return Result{Provider: upstashcloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: upstashcloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncUpstashConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == upstashcloud.ProviderName && host.SyncID != "" {
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
			alias = upstashcloud.UniqueAlias(upstashcloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, upstashcloud.ToSyncHost(inst, alias))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: upstashcloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
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
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncUpstashConfig); err != nil {
		return Result{Provider: upstashcloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncUpstashConfig, blocks); err != nil {
		return Result{Provider: upstashcloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: upstashcloud.ProviderName}, err
	}
	latest := e.Store.Upstash()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetUpstash(latest); err != nil {
		return Result{Provider: upstashcloud.ProviderName}, err
	}
	result := Result{Provider: upstashcloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func (e *Engine) MaybeAutoConnectUpstash(ctx context.Context) (Result, bool, error) {
	integration := e.Store.Upstash()
	if integration.Disabled {
		return Result{}, false, nil
	}
	if !e.Upstash.HasKey() {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.SyncUpstash(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetUpstash(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.SyncUpstash(ctx)
	return result, true, syncErr
}

func (e *Engine) SaveUpstashKey(ctx context.Context, key string) (Result, error) {
	if err := e.Upstash.SaveKey(key); err != nil {
		return Result{}, err
	}
	return e.SyncUpstash(ctx)
}

func (e *Engine) NewUpstash(ctx context.Context, opts upstashcloud.CreateOpts) (Result, string, error) {
	box, err := e.Upstash.Create(ctx, opts)
	if err != nil && box.ID == "" {
		return Result{}, "", err
	}
	result, syncErr := e.SyncUpstash(ctx)
	alias := e.AliasForUpstashSyncID(ctx, box.ID)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) ForkUpstash(ctx context.Context, syncID string) (Result, string, error) {
	id, err := e.Upstash.Fork(ctx, syncID)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.SyncUpstash(ctx)
	alias := e.AliasForUpstashSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopUpstash(ctx context.Context, syncID string) (Result, error) {
	if err := e.Upstash.Pause(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncUpstash(ctx)
}

func (e *Engine) ResumeUpstash(ctx context.Context, syncID string) (Result, error) {
	if err := e.Upstash.Resume(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncUpstash(ctx)
}

func (e *Engine) DeleteUpstash(ctx context.Context, syncID string) (Result, error) {
	if err := e.Upstash.Delete(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncUpstash(ctx)
}

func (e *Engine) ResolveUpstashSyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if id, err := upstashcloud.ParseSyncID(hostOrID); err == nil {
		if _, getErr := e.Upstash.Get(ctx, id); getErr == nil {
			return id, nil
		}
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == upstashcloud.ProviderName &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID)) {
			return host.SyncID, nil
		}
	}
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != upstashcloud.ProviderName {
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
		return "", fmt.Errorf("upstash label %q matches %d hosts; pass an alias or box id", hostOrID, len(matches))
	}
	return "", fmt.Errorf("upstash host %q not found; sync with bast sync upstash", hostOrID)
}

func (e *Engine) AliasForUpstashSyncID(ctx context.Context, syncID string) string {
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == upstashcloud.ProviderName && host.SyncID == syncID {
			return host.Alias
		}
	}
	return ""
}

func (e *Engine) EnsureUpstashAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return err
	}
	defer e.upstashMu.Unlock()
	if !host.Synced || host.SyncSource != upstashcloud.ProviderName || host.SyncID == "" {
		return nil
	}
	if status != nil {
		status("Checking Upstash Box access…")
	}
	if err := e.Upstash.PersistResolvedKey(); err != nil {
		return err
	}
	info, err := e.Upstash.Get(ctx, host.SyncID)
	if err != nil {
		return err
	}
	state := strings.ToLower(strings.TrimSpace(info.Status))
	if state == "paused" {
		return fmt.Errorf("upstash box %s is paused; resume it first with bast upstash resume %s", info.ID, info.ID)
	}
	if state == "creating" {
		if status != nil {
			status("Waiting for Upstash Box…")
		}
		if err := e.Upstash.WaitReady(ctx, info.ID, 5*time.Minute); err != nil {
			return err
		}
	}
	if state == "error" || state == "deleted" {
		return fmt.Errorf("upstash box %s is not ready for SSH (state %s)", info.ID, state)
	}
	return sshconfig.UpdateSyncHostAuthAndHost(
		e.Paths.SyncUpstashConfig, host.Alias, e.Upstash.SSHHost(), info.ID, "", "", false,
	)
}

func (e *Engine) DisableUpstash(ctx context.Context) error {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return err
	}
	defer e.upstashMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == upstashcloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncUpstashConfig); err != nil {
		return err
	}
	integration := e.Store.Upstash()
	integration.Enabled = false
	integration.AutoSync = false
	integration.Disabled = true
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetUpstash(integration)
}
