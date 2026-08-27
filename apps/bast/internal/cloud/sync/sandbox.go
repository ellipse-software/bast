package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bast/internal/cloud/sshutil"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

// sandboxRow is one discovered sandbox-style instance ready to write to SSH config.
type sandboxRow struct {
	Name  string
	Group string
	Tags  []string
	Block sshconfig.SyncHostInput
}

func (e *Engine) reconcileSyncedHosts(ctx context.Context, provider, configPath string, rows []sandboxRow, complete bool, warnings []string) (Result, error) {
	now := time.Now().UTC()
	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: provider}, err
	}
	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(configPath); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == provider && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
			continue
		}
		usedAliases[host.Alias] = true
	}

	blocks := make([]sshconfig.SyncHostInput, 0, len(rows))
	aliases := make([]string, 0, len(rows))
	activeSyncIDs := map[string]bool{}
	metadataUpdates := make([]hostMetadataUpdate, 0, len(rows))
	var metadataDeletes []string
	for _, row := range rows {
		inst := row.Block
		activeSyncIDs[inst.SyncID] = true
		alias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok && !usedAliases[prev.Alias] {
			alias = prev.Alias
			usedAliases[alias] = true
		} else {
			base := inst.Alias
			if base == "" {
				base = row.Name
			}
			alias = sshutil.UniqueAlias(base, usedAliases)
			usedAliases[alias] = true
		}
		inst.Alias = alias
		blocks = append(blocks, inst)
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: row.Name,
			group: row.Group, tags: append([]string(nil), row.Tags...),
		})
	}
	if complete {
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
	if err := e.Config.EnsureSyncInclude(configPath); err != nil {
		return Result{Provider: provider}, err
	}
	if err := sshconfig.WriteSyncConfig(configPath, blocks); err != nil {
		return Result{Provider: provider}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: provider}, err
	}
	result := Result{Provider: provider, Count: len(blocks), SyncedAt: now, Aliases: aliases}
	if len(warnings) > 0 {
		result.Error = strings.Join(warnings, "; ")
	}
	return result, nil
}

func aliasForSyncID(configPath, syncID string) string {
	syncID = strings.TrimSpace(syncID)
	if syncID == "" {
		return ""
	}
	blocks, err := sshconfig.LoadSyncHosts(configPath)
	if err != nil {
		return ""
	}
	for _, block := range blocks {
		if block.SyncID == syncID {
			return block.Alias
		}
	}
	return ""
}

func (e *Engine) aliasFromHosts(ctx context.Context, provider, syncID string) string {
	if alias := aliasForSyncID(e.syncConfigPath(provider), syncID); alias != "" {
		return alias
	}
	hosts, err := e.Discover(ctx)
	if err != nil {
		return ""
	}
	for _, host := range hosts {
		if host.Synced && host.SyncSource == provider && host.SyncID == syncID {
			return host.Alias
		}
	}
	return ""
}

func (e *Engine) syncConfigPath(provider string) string {
	switch provider {
	case "box":
		return e.Paths.SyncBoxConfig
	case "upstash":
		return e.Paths.SyncUpstashConfig
	case "vercel":
		return e.Paths.SyncVercelConfig
	case "hetzner":
		return e.Paths.SyncHetznerConfig
	default:
		return ""
	}
}

func (e *Engine) matchSyncedID(hosts []sshconfig.Host, provider, hostOrID string) (aliasID string, labelIDs []string) {
	for _, host := range hosts {
		if host.Synced && host.SyncSource == provider &&
			(host.Alias == hostOrID || strings.EqualFold(host.Alias, hostOrID)) {
			return host.SyncID, nil
		}
	}
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != provider {
			continue
		}
		meta := e.Store.Host(host.Alias)
		if meta.Label != "" && strings.EqualFold(meta.Label, hostOrID) {
			labelIDs = append(labelIDs, host.SyncID)
		}
	}
	return "", labelIDs
}

func resolveMatchError(noun, hostOrID, hint, syncHint string, labelIDs []string) error {
	if len(labelIDs) == 1 {
		return nil
	}
	if len(labelIDs) > 1 {
		return fmt.Errorf("%s label %q matches %d hosts; %s", noun, hostOrID, len(labelIDs), hint)
	}
	return fmt.Errorf("%s host %q not found; %s", noun, hostOrID, syncHint)
}

func (e *Engine) deleteSyncedHostMetadata(existing []sshconfig.Host, provider string) error {
	var aliases []string
	for _, host := range existing {
		if host.Synced && host.SyncSource == provider {
			aliases = append(aliases, host.Alias)
		}
	}
	if len(aliases) == 0 {
		return nil
	}
	return e.Store.UpdateHosts(func(hosts map[string]metadata.Host) {
		for _, alias := range aliases {
			delete(hosts, alias)
		}
	})
}
