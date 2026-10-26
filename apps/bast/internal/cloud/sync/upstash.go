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
	return e.syncUpstashLocked(ctx)
}

func (e *Engine) syncUpstashLocked(ctx context.Context) (Result, error) {
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

	rows := make([]sandboxRow, 0, len(discovery.Instances))
	for _, inst := range discovery.Instances {
		rows = append(rows, sandboxRow{
			Name:  inst.Name,
			Group: upstashcloud.GroupPath(inst),
			Tags:  append([]string(nil), inst.Tags...),
			Block: upstashcloud.ToSyncHost(inst, upstashcloud.AliasFor(inst)),
		})
	}
	result, err := e.reconcileSyncedHosts(ctx, upstashcloud.ProviderName, e.Paths.SyncUpstashConfig, rows, discovery.Complete, discovery.Warnings)
	if err != nil {
		return result, err
	}
	latest := e.Store.Upstash()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &result.SyncedAt
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = result.Count
	if err := e.Store.SetUpstash(latest); err != nil {
		return Result{Provider: upstashcloud.ProviderName}, err
	}
	return result, nil
}

func (e *Engine) MaybeAutoConnectUpstash(ctx context.Context) (Result, bool, error) {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return Result{}, false, err
	}
	defer e.upstashMu.Unlock()
	integration := e.Store.Upstash()
	if integration.Disabled {
		return Result{}, false, nil
	}
	if !e.Upstash.HasKey() {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.syncUpstashLocked(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetUpstash(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.syncUpstashLocked(ctx)
	return result, true, syncErr
}

func (e *Engine) SaveUpstashKey(ctx context.Context, key string) (Result, error) {
	if err := e.Upstash.SaveKey(key); err != nil {
		return Result{}, err
	}
	return e.SyncUpstash(ctx)
}

func (e *Engine) NewUpstash(ctx context.Context, opts upstashcloud.CreateOpts) (Result, string, error) {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return Result{}, "", err
	}
	defer e.upstashMu.Unlock()
	box, err := e.Upstash.Create(ctx, opts)
	if err != nil && box.ID == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncUpstashLocked(ctx)
	alias := e.AliasForUpstashSyncID(ctx, box.ID)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) ForkUpstash(ctx context.Context, syncID string) (Result, string, error) {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return Result{}, "", err
	}
	defer e.upstashMu.Unlock()
	id, err := e.Upstash.Fork(ctx, syncID)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncUpstashLocked(ctx)
	alias := e.AliasForUpstashSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopUpstash(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return Result{}, err
	}
	defer e.upstashMu.Unlock()
	if err := e.Upstash.Pause(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncUpstashLocked(ctx)
}

func (e *Engine) ResumeUpstash(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return Result{}, err
	}
	defer e.upstashMu.Unlock()
	if err := e.Upstash.Resume(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncUpstashLocked(ctx)
}

func (e *Engine) DeleteUpstash(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.upstashMu); err != nil {
		return Result{}, err
	}
	defer e.upstashMu.Unlock()
	if err := e.Upstash.Delete(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncUpstashLocked(ctx)
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
	if aliasID, labels := e.matchSyncedID(hosts, upstashcloud.ProviderName, hostOrID); aliasID != "" {
		return aliasID, nil
	} else if len(labels) == 1 {
		return labels[0], nil
	} else {
		return "", resolveMatchError("upstash", hostOrID, "pass an alias or box id", "sync with bast sync upstash", labels)
	}
}

func (e *Engine) AliasForUpstashSyncID(ctx context.Context, syncID string) string {
	return e.aliasFromHosts(ctx, upstashcloud.ProviderName, syncID)
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
	if err := e.deleteSyncedHostMetadata(existing, upstashcloud.ProviderName); err != nil {
		return err
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
