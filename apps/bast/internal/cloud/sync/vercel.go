package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	vercelcloud "bast/internal/cloud/vercel"
	"bast/internal/sshconfig"
)

func (e *Engine) applyVercelScope() {
	integration := e.Store.Vercel()
	if team := strings.TrimSpace(integration.TeamID); team != "" {
		e.Vercel.TeamID = team
	}
	if project := strings.TrimSpace(integration.ProjectID); project != "" {
		e.Vercel.ProjectID = project
	}
}

func (e *Engine) SyncVercel(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, err
	}
	defer e.vercelMu.Unlock()

	e.applyVercelScope()
	_ = e.Vercel.PersistResolvedToken()
	discovery, err := e.Vercel.Discover(ctx, struct{}{})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Vercel()
		latest.Enabled = true
		latest.Disabled = false
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetVercel(latest)
		return Result{Provider: vercelcloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: vercelcloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncVercelConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == vercelcloud.ProviderName && host.SyncID != "" {
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
			alias = vercelcloud.UniqueAlias(vercelcloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, vercelcloud.ToSyncHost(inst, alias))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: vercelcloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
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
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncVercelConfig); err != nil {
		return Result{Provider: vercelcloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncVercelConfig, blocks); err != nil {
		return Result{Provider: vercelcloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: vercelcloud.ProviderName}, err
	}
	latest := e.Store.Vercel()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetVercel(latest); err != nil {
		return Result{Provider: vercelcloud.ProviderName}, err
	}
	result := Result{Provider: vercelcloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func (e *Engine) MaybeAutoConnectVercel(ctx context.Context) (Result, bool, error) {
	integration := e.Store.Vercel()
	if integration.Disabled {
		return Result{}, false, nil
	}
	e.applyVercelScope()
	if !e.Vercel.HasToken() {
		return Result{}, false, nil
	}
	if strings.TrimSpace(e.Vercel.ResolveTeam()) == "" || strings.TrimSpace(e.Vercel.ResolveProject()) == "" {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.SyncVercel(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetVercel(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.SyncVercel(ctx)
	return result, true, syncErr
}

func (e *Engine) SaveVercelToken(ctx context.Context, token, teamID, projectID string) (Result, error) {
	if err := e.Vercel.SaveToken(token); err != nil {
		return Result{}, err
	}
	integration := e.Store.Vercel()
	if team := strings.TrimSpace(teamID); team != "" {
		integration.TeamID = team
	}
	if project := strings.TrimSpace(projectID); project != "" {
		integration.ProjectID = project
	}
	if err := e.Store.SetVercel(integration); err != nil {
		return Result{}, err
	}
	return e.SyncVercel(ctx)
}

func (e *Engine) NewVercel(ctx context.Context, opts vercelcloud.CreateOpts) (Result, string, error) {
	e.applyVercelScope()
	box, err := e.Vercel.Create(ctx, opts)
	if err != nil && box.Name == "" {
		return Result{}, "", err
	}
	result, syncErr := e.SyncVercel(ctx)
	alias := e.AliasForVercelSyncID(ctx, vercelcloud.SyncID(e.Vercel.ResolveProject(), box.Name))
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) ForkVercel(ctx context.Context, syncID, name string) (Result, string, error) {
	e.applyVercelScope()
	id, err := e.Vercel.Fork(ctx, syncID, name)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.SyncVercel(ctx)
	alias := e.AliasForVercelSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopVercel(ctx context.Context, syncID string) (Result, error) {
	e.applyVercelScope()
	if err := e.Vercel.Stop(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncVercel(ctx)
}

func (e *Engine) ResumeVercel(ctx context.Context, syncID string) (Result, error) {
	e.applyVercelScope()
	if err := e.Vercel.Resume(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncVercel(ctx)
}

func (e *Engine) DeleteVercel(ctx context.Context, syncID string) (Result, error) {
	e.applyVercelScope()
	if err := e.Vercel.Delete(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.SyncVercel(ctx)
}

func (e *Engine) ResolveVercelSyncID(ctx context.Context, hostOrID string) (string, error) {
	e.applyVercelScope()
	hostOrID = strings.TrimSpace(hostOrID)
	if project, name, err := vercelcloud.ParseSyncID(hostOrID); err == nil {
		id := vercelcloud.SyncID(project, name)
		if project == "" {
			id = vercelcloud.SyncID(e.Vercel.ResolveProject(), name)
		}
		if _, getErr := e.Vercel.Get(ctx, id, false); getErr == nil {
			return id, nil
		}
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == vercelcloud.ProviderName &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID)) {
			return host.SyncID, nil
		}
	}
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != vercelcloud.ProviderName {
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
		return "", fmt.Errorf("vercel label %q matches %d hosts; pass an alias or sandbox name", hostOrID, len(matches))
	}
	return "", fmt.Errorf("vercel host %q not found; sync with bast sync vercel", hostOrID)
}

func (e *Engine) AliasForVercelSyncID(ctx context.Context, syncID string) string {
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == vercelcloud.ProviderName && host.SyncID == syncID {
			return host.Alias
		}
	}
	return ""
}

func (e *Engine) EnsureVercelAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return err
	}
	defer e.vercelMu.Unlock()
	if !host.Synced || host.SyncSource != vercelcloud.ProviderName || host.SyncID == "" {
		return nil
	}
	e.applyVercelScope()
	if status != nil {
		status("Checking Vercel Sandbox access…")
	}
	if err := e.Vercel.PersistResolvedToken(); err != nil {
		return err
	}
	info, err := e.Vercel.Get(ctx, host.SyncID, false)
	if err != nil {
		return err
	}
	state := strings.ToLower(strings.TrimSpace(info.Sandbox.Status))
	if vercelcloud.IsStoppedState(state) {
		if status != nil {
			status("Resuming Vercel Sandbox…")
		}
		if err := e.Vercel.Resume(ctx, host.SyncID); err != nil {
			return err
		}
	}
	if state == "pending" {
		if status != nil {
			status("Waiting for Vercel Sandbox…")
		}
		if err := e.Vercel.WaitReady(ctx, host.SyncID, 5*time.Minute); err != nil {
			return err
		}
	}
	if state == "failed" || state == "aborted" {
		return fmt.Errorf("vercel sandbox %s is not ready (%s)", info.Sandbox.Name, state)
	}
	return nil
}

func (e *Engine) DisableVercel(ctx context.Context) error {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return err
	}
	defer e.vercelMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == vercelcloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncVercelConfig); err != nil {
		return err
	}
	integration := e.Store.Vercel()
	integration.Enabled = false
	integration.AutoSync = false
	integration.Disabled = true
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetVercel(integration)
}
