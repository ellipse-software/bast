package sync

import (
	"context"
	"fmt"
	"strings"

	docloud "bast/internal/cloud/digitalocean"
)

func (e *Engine) NewDigitalOcean(ctx context.Context, opts docloud.NewOpts) (Result, string, error) {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return Result{}, "", err
	}
	defer e.digitalOceanMu.Unlock()
	id, err := e.DigitalOcean.New(ctx, opts)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncDigitalOceanLocked(ctx)
	alias := e.AliasForDigitalOceanSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) ForkDigitalOcean(ctx context.Context, syncID string) (Result, string, error) {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return Result{}, "", err
	}
	defer e.digitalOceanMu.Unlock()
	id, err := e.DigitalOcean.Fork(ctx, syncID)
	if err != nil && id == "" {
		return Result{}, "", err
	}
	result, syncErr := e.syncDigitalOceanLocked(ctx)
	alias := e.AliasForDigitalOceanSyncID(ctx, id)
	if err != nil {
		return result, alias, err
	}
	return result, alias, syncErr
}

func (e *Engine) StopDigitalOcean(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return Result{}, err
	}
	defer e.digitalOceanMu.Unlock()
	if err := e.DigitalOcean.Stop(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncDigitalOceanLocked(ctx)
}

func (e *Engine) ResumeDigitalOcean(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return Result{}, err
	}
	defer e.digitalOceanMu.Unlock()
	if err := e.DigitalOcean.Start(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncDigitalOceanLocked(ctx)
}

func (e *Engine) DeleteDigitalOcean(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.digitalOceanMu); err != nil {
		return Result{}, err
	}
	defer e.digitalOceanMu.Unlock()
	if err := e.DigitalOcean.Delete(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncDigitalOceanLocked(ctx)
}

func (e *Engine) ResolveDigitalOceanSyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if _, _, err := docloud.ParseSyncID(hostOrID); err == nil {
		return hostOrID, nil
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == docloud.ProviderName &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID)) {
			return host.SyncID, nil
		}
	}
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != docloud.ProviderName {
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
		return "", fmt.Errorf("DigitalOcean label %q matches %d hosts; pass an alias or do:<account>:<id>", hostOrID, len(matches))
	}
	return "", fmt.Errorf("DigitalOcean host %q not found; sync with bast sync digitalocean", hostOrID)
}

func (e *Engine) AliasForDigitalOceanSyncID(ctx context.Context, syncID string) string {
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == docloud.ProviderName && host.SyncID == syncID {
			return host.Alias
		}
	}
	return ""
}
