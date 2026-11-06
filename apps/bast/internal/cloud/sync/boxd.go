package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	boxdcloud "bast/internal/cloud/boxd"
	"bast/internal/sshconfig"
)

func (e *Engine) SyncBoxd(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.boxdMu); err != nil {
		return Result{}, err
	}
	defer e.boxdMu.Unlock()
	return e.syncBoxdLocked(ctx)
}

func (e *Engine) syncBoxdLocked(ctx context.Context) (Result, error) {
	integration := e.Store.Boxd()
	discovery, err := e.Boxd.Discover(ctx, boxdcloud.DiscoverConfig{OrgFilter: integration.OrgFilter})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Boxd()
		latest.Enabled = true
		latest.Disabled = false
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetBoxd(latest)
		return Result{Provider: boxdcloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: boxdcloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncBoxdConfig); loadErr == nil {
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
		if host.Synced && host.SyncSource == boxdcloud.ProviderName && host.SyncID != "" {
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
			alias = boxdcloud.UniqueAlias(boxdcloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		block := boxdcloud.ToSyncHost(inst, alias)
		// A computer keeps its SSH port, so the last known entry is still good
		// while the boxd CLI rewrites ~/.ssh/config in the background.
		if prev, ok := previousBlocks[inst.SyncID]; ok && block.Port == "" && prev.HostName == block.HostName {
			block.Port = prev.Port
			block.IdentityFile = prev.IdentityFile
			block.IdentitiesOnly = prev.IdentitiesOnly
			block.ExtraOptions = prev.ExtraOptions
		}
		blocks = append(blocks, block)
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: boxdcloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}

	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		if shouldPruneBoxdHost(syncID, discovery) {
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

	if err := e.Config.EnsureSyncInclude(e.Paths.SyncBoxdConfig); err != nil {
		return Result{Provider: boxdcloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncBoxdConfig, blocks); err != nil {
		return Result{Provider: boxdcloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: boxdcloud.ProviderName}, err
	}
	latest := e.Store.Boxd()
	latest.Enabled = true
	latest.Disabled = false
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetBoxd(latest); err != nil {
		return Result{Provider: boxdcloud.ProviderName}, err
	}
	result := Result{Provider: boxdcloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

// shouldPruneBoxdHost drops a host when its org was listed and the computer
// was not in it, or when the org is no longer selected. A host of an org whose
// list failed, or of a region the list could not read, is kept.
func shouldPruneBoxdHost(syncID string, discovery boxdcloud.Discovery) bool {
	orgID, region, _, err := boxdcloud.ParseSyncID(syncID)
	if err != nil {
		return true
	}
	if !discovery.ListedOrgs[orgID] {
		return true
	}
	return discovery.ConfirmedOrgs[orgID] && !discovery.RegionUnavailable(orgID, region)
}

func (e *Engine) MaybeAutoConnectBoxd(ctx context.Context) (Result, bool, error) {
	if err := lockCtx(ctx, &e.boxdMu); err != nil {
		return Result{}, false, err
	}
	defer e.boxdMu.Unlock()
	integration := e.Store.Boxd()
	if integration.Disabled {
		return Result{}, false, nil
	}
	account, err := e.Boxd.Account(ctx)
	if err != nil || !account.Authenticated {
		return Result{}, false, nil
	}
	if integration.Enabled && integration.AutoSync {
		result, syncErr := e.syncBoxdLocked(ctx)
		return result, true, syncErr
	}
	integration.Enabled = true
	integration.AutoSync = true
	integration.Disabled = false
	if err := e.Store.SetBoxd(integration); err != nil {
		return Result{}, false, err
	}
	result, syncErr := e.syncBoxdLocked(ctx)
	return result, true, syncErr
}

// NewBoxd creates a computer. An empty org means the org being synced.
func (e *Engine) NewBoxd(ctx context.Context, opts boxdcloud.CreateOpts) (Result, string, error) {
	if err := lockCtx(ctx, &e.boxdMu); err != nil {
		return Result{}, "", err
	}
	defer e.boxdMu.Unlock()
	orgID, err := e.boxdOrgID(ctx, opts.OrgID)
	if err != nil {
		return Result{}, "", err
	}
	opts.OrgID = orgID
	created, err := e.Boxd.Create(ctx, opts)
	if err != nil && created.Name == "" {
		return Result{}, "", err
	}
	return e.finishBoxdMutation(ctx, created, err)
}

// boxdOrgID resolves an org name (or nothing, for the synced org) to its id.
func (e *Engine) boxdOrgID(ctx context.Context, org string) (string, error) {
	account, err := e.Boxd.Account(ctx)
	if err != nil {
		return "", err
	}
	if !account.Authenticated {
		return "", fmt.Errorf("%s", account.Error)
	}
	synced, err := boxdcloud.SelectOrgs(account, e.Store.Boxd().OrgFilter)
	if err != nil {
		return "", err
	}
	want := strings.TrimSpace(org)
	if want == "" {
		if len(synced) != 1 {
			return "", fmt.Errorf("pick one boxd org with --org; syncing %d", len(synced))
		}
		return synced[0].ID, nil
	}
	chosen, err := boxdcloud.SelectOrgs(account, []string{want})
	if err != nil {
		return "", err
	}
	if len(chosen) != 1 {
		return "", fmt.Errorf("pick one boxd org with --org")
	}
	for _, item := range synced {
		if item.ID == chosen[0].ID {
			return item.ID, nil
		}
	}
	name := chosen[0].Name
	if name == "" {
		name = chosen[0].ID
	}
	return "", fmt.Errorf("boxd org %s is outside the sync filter; add it before creating a computer there", name)
}

func (e *Engine) ForkBoxd(ctx context.Context, syncID string, opts boxdcloud.ForkOpts) (Result, string, error) {
	if err := lockCtx(ctx, &e.boxdMu); err != nil {
		return Result{}, "", err
	}
	defer e.boxdMu.Unlock()
	created, err := e.Boxd.Fork(ctx, syncID, opts)
	if err != nil && created.Name == "" {
		return Result{}, "", err
	}
	return e.finishBoxdMutation(ctx, created, err)
}

// finishBoxdMutation syncs after a create or fork. A readiness failure still
// syncs, so the computer shows up, but it returns no alias: callers treat an
// alias as success. A sync failure after a successful mutation keeps the alias
// and returns the sync error.
func (e *Engine) finishBoxdMutation(ctx context.Context, created boxdcloud.Created, opErr error) (Result, string, error) {
	result, syncErr := e.syncBoxdLocked(ctx)
	if opErr != nil {
		if syncErr != nil {
			opErr = fmt.Errorf("%w; sync: %v", opErr, syncErr)
		}
		return result, "", opErr
	}
	return result, e.AliasForBoxdHost(ctx, created.HostName), syncErr
}

func (e *Engine) StopBoxd(ctx context.Context, syncID string) (Result, error) {
	return e.boxdLifecycle(ctx, syncID, e.Boxd.Stop)
}

func (e *Engine) StartBoxd(ctx context.Context, syncID string) (Result, error) {
	return e.boxdLifecycle(ctx, syncID, e.Boxd.Start)
}

func (e *Engine) RestartBoxd(ctx context.Context, syncID string) (Result, error) {
	return e.boxdLifecycle(ctx, syncID, e.Boxd.Restart)
}

func (e *Engine) DeleteBoxd(ctx context.Context, syncID string) (Result, error) {
	return e.boxdLifecycle(ctx, syncID, e.Boxd.Delete)
}

func (e *Engine) boxdLifecycle(ctx context.Context, syncID string, action func(context.Context, string) error) (Result, error) {
	if err := lockCtx(ctx, &e.boxdMu); err != nil {
		return Result{}, err
	}
	defer e.boxdMu.Unlock()
	if _, _, _, err := boxdcloud.ParseSyncID(syncID); err != nil {
		return Result{}, err
	}
	actionErr := action(ctx, syncID)
	result, syncErr := e.syncBoxdLocked(ctx)
	if actionErr != nil {
		return result, actionErr
	}
	return result, syncErr
}

func (e *Engine) ResolveBoxdSyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if _, _, _, err := boxdcloud.ParseSyncID(hostOrID); err == nil {
		return hostOrID, nil
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == boxdcloud.ProviderName &&
			(strings.EqualFold(host.Alias, hostOrID) || host.SyncID == hostOrID ||
				strings.EqualFold(host.Resolved.HostName, hostOrID)) {
			return host.SyncID, nil
		}
	}
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != boxdcloud.ProviderName {
			continue
		}
		_, _, name, parseErr := boxdcloud.ParseSyncID(host.SyncID)
		meta := e.Store.Host(host.Alias)
		if (parseErr == nil && name == hostOrID) || (meta.Label != "" && strings.EqualFold(meta.Label, hostOrID)) {
			matches = append(matches, host.SyncID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("boxd name %q matches %d hosts; pass an alias or org/region/name", hostOrID, len(matches))
	}
	return "", fmt.Errorf("boxd host %q not found; sync with bast sync boxd", hostOrID)
}

// AliasForBoxdHost finds a synced host by the computer's domain, which is
// unique across orgs and regions.
func (e *Engine) AliasForBoxdHost(ctx context.Context, hostName string) string {
	hostName = strings.TrimSpace(hostName)
	if hostName == "" {
		return ""
	}
	blocks, err := sshconfig.LoadSyncHosts(e.Paths.SyncBoxdConfig)
	if err != nil {
		return ""
	}
	for _, block := range blocks {
		if strings.EqualFold(block.HostName, hostName) {
			return block.Alias
		}
	}
	return ""
}

func (e *Engine) EnsureBoxdAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.boxdMu); err != nil {
		return err
	}
	defer e.boxdMu.Unlock()
	if !host.Synced || host.SyncSource != boxdcloud.ProviderName || host.SyncID == "" {
		return nil
	}
	result, err := e.Boxd.EnsureAccess(ctx, host.SyncID, boxdcloud.EnsureConfig{Status: status, Alias: host.Alias})
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostDetails(
		e.Paths.SyncBoxdConfig, host.Alias, result.HostName, result.User,
		result.IdentityFile, "", result.Port, result.IdentitiesOnly,
	)
}

func (e *Engine) DisableBoxd(ctx context.Context) error {
	if err := lockCtx(ctx, &e.boxdMu); err != nil {
		return err
	}
	defer e.boxdMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	if err := e.deleteSyncedHostMetadata(existing, boxdcloud.ProviderName); err != nil {
		return err
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncBoxdConfig); err != nil {
		return err
	}
	integration := e.Store.Boxd()
	integration.Enabled = false
	integration.AutoSync = false
	integration.Disabled = true
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetBoxd(integration)
}
