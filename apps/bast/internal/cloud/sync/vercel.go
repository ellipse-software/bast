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
	projects := integration.Projects()
	e.Vercel.ProjectIDs = append([]string(nil), projects...)
	if len(projects) > 0 {
		e.Vercel.ProjectID = projects[0]
	}
}

func (e *Engine) SyncVercel(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, err
	}
	defer e.vercelMu.Unlock()
	return e.syncVercelLocked(ctx)
}

func (e *Engine) syncVercelLocked(ctx context.Context) (Result, error) {
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

	rows := make([]sandboxRow, 0, len(discovery.Instances))
	for _, inst := range discovery.Instances {
		rows = append(rows, sandboxRow{
			Name:  inst.Name,
			Group: vercelcloud.GroupPath(inst),
			Tags:  append([]string(nil), inst.Tags...),
			Block: vercelcloud.ToSyncHost(inst, vercelcloud.AliasFor(inst)),
		})
	}
	result, err := e.reconcileSyncedHosts(ctx, vercelcloud.ProviderName, e.Paths.SyncVercelConfig, rows, discovery.Complete, discovery.Warnings)
	if err != nil {
		return result, err
	}
	latest := e.Store.Vercel()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &result.SyncedAt
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = result.Count
	latest.Unrestorable = append([]string(nil), discovery.Unrestorable...)
	if err := e.Store.SetVercel(latest); err != nil {
		return Result{Provider: vercelcloud.ProviderName}, err
	}
	return result, nil
}

func (e *Engine) MaybeAutoConnectVercel(ctx context.Context) (Result, bool, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, false, err
	}
	defer e.vercelMu.Unlock()
	integration := e.Store.Vercel()
	if integration.Disabled {
		return Result{}, false, nil
	}
	e.applyVercelScope()
	if !e.Vercel.HasToken() {
		return Result{}, false, nil
	}
	if strings.TrimSpace(e.Vercel.ResolveTeam()) == "" {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.syncVercelLocked(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetVercel(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.syncVercelLocked(ctx)
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
	if projects := vercelcloud.ParseProjectList(projectID); len(projects) > 0 {
		integration.ProjectID = projects[0]
		if len(projects) > 1 {
			integration.ProjectIDs = projects
		} else {
			integration.ProjectIDs = nil
		}
	}
	if err := e.Store.SetVercel(integration); err != nil {
		return Result{}, err
	}
	return e.SyncVercel(ctx)
}

func (e *Engine) NewVercel(ctx context.Context, opts vercelcloud.CreateOpts) (Result, string, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, "", err
	}
	defer e.vercelMu.Unlock()
	e.applyVercelScope()
	box, err := e.Vercel.Create(ctx, opts)
	if err != nil && box.Name == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncVercelLocked(ctx)
	alias := e.AliasForVercelSyncID(ctx, vercelcloud.SyncID(e.Vercel.ResolveProject(), box.Name))
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) ForkVercel(ctx context.Context, syncID, name string) (Result, string, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, "", err
	}
	defer e.vercelMu.Unlock()
	e.applyVercelScope()
	id, err := e.Vercel.Fork(ctx, syncID, name)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncVercelLocked(ctx)
	alias := e.AliasForVercelSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopVercel(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, err
	}
	defer e.vercelMu.Unlock()
	e.applyVercelScope()
	if err := e.Vercel.Stop(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncVercelLocked(ctx)
}

func (e *Engine) ResumeVercel(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, err
	}
	defer e.vercelMu.Unlock()
	e.applyVercelScope()
	if err := e.Vercel.Resume(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncVercelLocked(ctx)
}

func (e *Engine) DeleteVercel(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, err
	}
	defer e.vercelMu.Unlock()
	e.applyVercelScope()
	if err := e.Vercel.Delete(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncVercelLocked(ctx)
}

func (e *Engine) ListVercelUnrestorable(ctx context.Context) ([]string, error) {
	e.applyVercelScope()
	return e.Vercel.Unrestorable(ctx)
}

func (e *Engine) CleanupVercel(ctx context.Context) (Result, []string, error) {
	if err := lockCtx(ctx, &e.vercelMu); err != nil {
		return Result{}, nil, err
	}
	defer e.vercelMu.Unlock()
	e.applyVercelScope()
	deleted, err := e.Vercel.CleanupUnrestorable(ctx)
	result, syncErr := e.syncVercelLocked(ctx)
	if err != nil {
		return result, deleted, err
	}
	return result, deleted, syncErr
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
	if aliasID, labels := e.matchSyncedID(hosts, vercelcloud.ProviderName, hostOrID); aliasID != "" {
		return aliasID, nil
	} else if len(labels) == 1 {
		return labels[0], nil
	} else {
		return "", resolveMatchError("vercel", hostOrID, "pass an alias or sandbox name", "sync with bast sync vercel", labels)
	}
}

func (e *Engine) AliasForVercelSyncID(ctx context.Context, syncID string) string {
	return e.aliasFromHosts(ctx, vercelcloud.ProviderName, syncID)
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
	if err := e.deleteSyncedHostMetadata(existing, vercelcloud.ProviderName); err != nil {
		return err
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
	integration.Unrestorable = nil
	return e.Store.SetVercel(integration)
}
