package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	hetznercloud "bast/internal/cloud/hetzner"
	"bast/internal/sshconfig"
)

func (e *Engine) SyncHetzner(ctx context.Context) (Result, error) {
	if err := lockCtx(ctx, &e.hetznerMu); err != nil {
		return Result{}, err
	}
	defer e.hetznerMu.Unlock()
	return e.syncHetznerLocked(ctx)
}

func (e *Engine) syncHetznerLocked(ctx context.Context) (Result, error) {
	integration := e.Store.Hetzner()
	configured := map[string]bool{}
	if tokens, tokenErr := e.Hetzner.TokenContexts(); tokenErr == nil {
		for _, token := range tokens {
			configured[token.Name] = true
		}
	}
	discovery, err := e.Hetzner.Discover(ctx, hetznercloud.DiscoverConfig{
		ContextFilter:   integration.ContextFilter,
		LocationFilter:  integration.LocationFilter,
		DefaultSSHUser:  integration.DefaultSSHUser,
		DefaultSSHPort:  integration.DefaultSSHPort,
		PreferPrivateIP: integration.PreferPrivateIP,
		Home:            e.Paths.Home,
		ManagedKeys:     e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.Hetzner()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetHetzner(latest)
		return Result{Provider: hetznercloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: hetznercloud.ProviderName}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncHetznerConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == hetznercloud.ProviderName && host.SyncID != "" {
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
			alias = hetznercloud.UniqueAlias(hetznercloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		block := hetznercloud.ToSyncHost(inst, alias)
		applyHetznerSSHHint(&block, localSSHHint(inst.HostName, existing), previousBlocks[inst.SyncID], defaultUserSet)
		blocks = append(blocks, block)
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: hetznercloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}
	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		meta := e.Store.Host(host.Alias)
		contextName := hetznercloud.ContextFromGroup(meta.Group)
		if contextName != "" && discovery.ConfirmedContexts[contextName] {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		if contextName != "" && !configured[contextName] {
			metadataDeletes = append(metadataDeletes, host.Alias)
			continue
		}
		if contextName != "" && hetznerContextExcluded(contextName, integration.ContextFilter) {
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
			block.Alias = hetznercloud.UniqueAlias(block.Alias, usedAliases)
		}
		usedAliases[block.Alias] = true
		blocks = append(blocks, block)
		aliases = append(aliases, block.Alias)
	}
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncHetznerConfig); err != nil {
		return Result{Provider: hetznercloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncHetznerConfig, blocks); err != nil {
		return Result{Provider: hetznercloud.ProviderName}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: hetznercloud.ProviderName}, err
	}
	latest := e.Store.Hetzner()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetHetzner(latest); err != nil {
		return Result{Provider: hetznercloud.ProviderName}, err
	}
	result := Result{Provider: hetznercloud.ProviderName, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func hetznerContextExcluded(contextName string, filter []string) bool {
	if len(filter) == 0 {
		return false
	}
	for _, value := range filter {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(contextName)) {
			return false
		}
	}
	return true
}

func (e *Engine) EnsureHetznerAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if err := lockCtx(ctx, &e.hetznerMu); err != nil {
		return err
	}
	defer e.hetznerMu.Unlock()
	if !host.Synced || host.SyncSource != hetznercloud.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.Hetzner()
	currentIdentity := ""
	if len(host.Resolved.IdentityFiles) > 0 {
		currentIdentity = host.Resolved.IdentityFiles[0]
	}
	result, err := e.Hetzner.EnsureAccess(ctx, host.SyncID, hetznercloud.EnsureConfig{
		Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys, DefaultSSHUser: integration.DefaultSSHUser,
		DefaultSSHPort: integration.DefaultSSHPort, PreferPrivateIP: integration.PreferPrivateIP,
		CurrentUser: host.Resolved.User, CurrentPort: host.Resolved.Port, CurrentIdentity: currentIdentity,
		CurrentIdentitiesOnly: strings.EqualFold(host.Resolved.IdentitiesOnly, "yes"), Status: status,
	})
	if err != nil {
		return err
	}
	if existing, discoverErr := e.Discover(ctx); discoverErr == nil {
		applyEnsureSSHHint(&result, localSSHHint(result.HostName, existing), strings.TrimSpace(integration.DefaultSSHUser) != "")
	}
	return sshconfig.UpdateSyncHostDetails(
		e.Paths.SyncHetznerConfig, host.Alias, result.HostName, result.User, result.IdentityFile, "", result.Port, result.IdentitiesOnly,
	)
}

type sshHint struct {
	user, port, identity string
	identitiesOnly       bool
}

func localSSHHint(hostname string, existing []sshconfig.Host) sshHint {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return sshHint{}
	}
	var hint sshHint
	for _, host := range existing {
		if host.Synced && host.SyncSource == hetznercloud.ProviderName {
			continue
		}
		if strings.TrimSpace(host.Resolved.HostName) != hostname {
			continue
		}
		identity := ""
		if len(host.Resolved.IdentityFiles) > 0 {
			identity = host.Resolved.IdentityFiles[0]
		}
		hint = sshHint{
			user:           host.Resolved.User,
			port:           host.Resolved.Port,
			identity:       identity,
			identitiesOnly: strings.EqualFold(host.Resolved.IdentitiesOnly, "yes"),
		}
		if host.Managed {
			return hint
		}
	}
	return hint
}

func applyHetznerSSHHint(block *sshconfig.SyncHostInput, hint sshHint, prev sshconfig.SyncHostInput, defaultUserSet bool) {
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

func applyEnsureSSHHint(result *hetznercloud.EnsureResult, hint sshHint, defaultUserSet bool) {
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

func configuredNonDefaultPort(port string) bool {
	port = strings.TrimSpace(port)
	return port != "" && port != "22"
}

func (e *Engine) StartHetzner(ctx context.Context, syncID string) (Result, error) {
	if err := lockCtx(ctx, &e.hetznerMu); err != nil {
		return Result{}, err
	}
	defer e.hetznerMu.Unlock()
	if err := e.Hetzner.Start(ctx, syncID); err != nil {
		return Result{}, err
	}
	return e.syncHetznerLocked(ctx)
}

func (e *Engine) StopHetzner(ctx context.Context, syncID string, force bool) (Result, error) {
	if err := lockCtx(ctx, &e.hetznerMu); err != nil {
		return Result{}, err
	}
	defer e.hetznerMu.Unlock()
	if err := e.Hetzner.Stop(ctx, syncID, force); err != nil {
		return Result{}, err
	}
	return e.syncHetznerLocked(ctx)
}

func (e *Engine) RestartHetzner(ctx context.Context, syncID string, force bool) (Result, error) {
	if err := lockCtx(ctx, &e.hetznerMu); err != nil {
		return Result{}, err
	}
	defer e.hetznerMu.Unlock()
	if err := e.Hetzner.Restart(ctx, syncID, force); err != nil {
		return Result{}, err
	}
	return e.syncHetznerLocked(ctx)
}

func (e *Engine) SaveHetznerKey(ctx context.Context, name, key string) (Result, error) {
	if err := e.Hetzner.SaveNamedToken(name, key); err != nil {
		return Result{}, err
	}
	return e.SyncHetzner(ctx)
}

func (e *Engine) DeleteHetznerToken(ctx context.Context, name string) (Result, error) {
	if err := e.Hetzner.DeleteNamedToken(name); err != nil {
		return Result{}, err
	}
	return e.SyncHetzner(ctx)
}

func (e *Engine) DisableHetzner(ctx context.Context) error {
	if err := lockCtx(ctx, &e.hetznerMu); err != nil {
		return err
	}
	defer e.hetznerMu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == hetznercloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncHetznerConfig); err != nil {
		return err
	}
	integration := e.Store.Hetzner()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetHetzner(integration)
}

func (e *Engine) ResolveHetznerSyncID(ctx context.Context, hostOrID string) (string, error) {
	hostOrID = strings.TrimSpace(hostOrID)
	if id, err := hetznercloud.ParseSyncID(hostOrID); err == nil {
		return hetznercloud.FormatSyncID(id), nil
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return "", err
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == hetznercloud.ProviderName &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID)) {
			return host.SyncID, nil
		}
	}
	var matches []string
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != hetznercloud.ProviderName {
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
		return "", fmt.Errorf("Hetzner label %q matches %d hosts; pass an alias or hetzner/<id>", hostOrID, len(matches))
	}
	return "", fmt.Errorf("Hetzner host %q not found; sync with bast sync hetzner", hostOrID)
}
