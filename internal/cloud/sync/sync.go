package sync

import (
	"context"
	"fmt"
	"strings"
	stdsync "sync"
	"time"

	"bast/internal/cloud"
	"bast/internal/cloud/gcp"
	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

// Result summarizes a sync operation.
type Result struct {
	Provider string    `json:"provider"`
	Count    int       `json:"count"`
	SyncedAt time.Time `json:"syncedAt"`
	Aliases  []string  `json:"aliases,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Engine reconciles cloud instances into Bast SSH config + metadata.
type Engine struct {
	mu       stdsync.Mutex
	Paths    paths.Paths
	Config   sshconfig.Manager
	Store    *metadata.Store
	GCP      *gcp.Client
	Discover func(ctx context.Context) ([]sshconfig.Host, error)

	// EnsureAccessWait overrides the guest-agent propagation pause after publishing a key.
	// Zero keeps the GCP client default; negative skips the wait (tests).
	EnsureAccessWait time.Duration
}

func New(p paths.Paths, store *metadata.Store) *Engine {
	cfg := sshconfig.Manager{
		Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir,
		ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys,
		SyncGCPConfig: p.SyncGCPConfig,
	}
	return &Engine{
		Paths:  p,
		Config: cfg,
		Store:  store,
		GCP:    gcp.New(),
		Discover: func(ctx context.Context) ([]sshconfig.Host, error) {
			return cfg.Discover()
		},
	}
}

// SyncGCP discovers GCP instances and writes the sync SSH config.
func (e *Engine) SyncGCP(ctx context.Context) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	integration := e.Store.GCP()
	discovery, err := e.GCP.DiscoverAll(ctx, cloud.DiscoverConfig{
		ProjectFilter:   integration.ProjectFilter,
		DefaultSSHUser:  integration.DefaultSSHUser,
		ServiceAccounts: integration.ServiceAccounts,
		Home:            e.Paths.Home,
		ManagedKeys:     e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.GCP()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetGCP(latest)
		return Result{Provider: gcp.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{}, err
	}

	previousBlocks := map[string]sshconfig.SyncHostInput{}
	if loaded, loadErr := sshconfig.LoadSyncHosts(e.Paths.SyncGCPConfig); loadErr == nil {
		for _, block := range loaded {
			if block.SyncID != "" {
				previousBlocks[block.SyncID] = block
			}
		}
	}

	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == gcp.ProviderName && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
			continue
		}
		usedAliases[host.Alias] = true
	}

	blocks := make([]sshconfig.SyncHostInput, 0, len(discovery.Instances)+len(previousBySyncID))
	aliases := make([]string, 0, len(discovery.Instances)+len(previousBySyncID))
	activeAliases := map[string]bool{}
	activeSyncIDs := map[string]bool{}

	for _, inst := range discovery.Instances {
		activeSyncIDs[inst.SyncID] = true
		alias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok && !usedAliases[prev.Alias] {
			alias = prev.Alias
			usedAliases[alias] = true
		} else {
			alias = gcp.UniqueAlias(gcp.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}

		block := gcp.ToSyncHost(inst, alias)
		blocks = append(blocks, block)
		aliases = append(aliases, alias)
		activeAliases[alias] = true

		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		if err := e.applyMetadata(alias, inst, prevAlias); err != nil {
			return Result{}, err
		}
	}

	// Keep hosts from projects we could not re-scan (expired account, permission
	// denied, transient API errors). Only prune when the project inventory was
	// confirmed in this sync.
	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		projectID, _, _, parseErr := gcp.ParseSyncID(syncID)
		if parseErr == nil && discovery.ConfirmedProjects[projectID] {
			_ = e.Store.DeleteHost(host.Alias)
			continue
		}
		block, ok := previousBlocks[syncID]
		if !ok {
			// Fall back to a minimal block so the host stays selectable.
			block = sshconfig.SyncHostInput{
				Alias:      host.Alias,
				SyncSource: host.SyncSource,
				SyncID:     host.SyncID,
				HostName:   host.Alias,
			}
		}
		if usedAliases[block.Alias] && block.Alias != host.Alias {
			block.Alias = gcp.UniqueAlias(block.Alias, usedAliases)
		}
		usedAliases[block.Alias] = true
		activeSyncIDs[syncID] = true
		blocks = append(blocks, block)
		aliases = append(aliases, block.Alias)
		activeAliases[block.Alias] = true
	}

	if err := e.Config.EnsureSyncInclude(); err != nil {
		return Result{}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncGCPConfig, blocks); err != nil {
		return Result{}, err
	}

	latest := e.Store.GCP()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = strings.Join(discovery.Warnings, "; ")
	latest.LastInstanceCount = len(blocks)
	if err := e.Store.SetGCP(latest); err != nil {
		return Result{}, err
	}
	result := Result{
		Provider: gcp.ProviderName,
		Count:    len(blocks),
		SyncedAt: now,
		Aliases:  aliases,
	}
	if len(discovery.Warnings) > 0 {
		result.Error = strings.Join(discovery.Warnings, "; ")
	}
	return result, nil
}

func (e *Engine) applyMetadata(alias string, inst cloud.Instance, previousAlias string) error {
	if previousAlias != "" && previousAlias != alias {
		_ = e.Store.RenameHost(previousAlias, alias)
	}
	meta := e.Store.Host(alias)
	meta.Label = inst.Name
	meta.Group = gcp.GroupPath(inst)
	meta.Tags = append([]string(nil), inst.Tags...)
	return e.Store.SetHost(alias, meta)
}

// EnsureGCPAccess prepares SSH auth for a GCP-synced host (gcloud-style key ensure)
// and updates the sync SSH config block so the subsequent ssh uses the right User/key.
// status, when non-nil, receives short progress messages for interactive connects.
func (e *Engine) EnsureGCPAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !host.Synced || host.SyncSource != gcp.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.GCP()
	cfg := gcp.EnsureConfig{
		Home:            e.Paths.Home,
		ManagedKeys:     e.Paths.ManagedKeys,
		DefaultSSHUser:  integration.DefaultSSHUser,
		ServiceAccounts: integration.ServiceAccounts,
		Status:          status,
	}
	if e.EnsureAccessWait != 0 {
		cfg.PropagationWait = e.EnsureAccessWait
	}
	result, err := e.GCP.EnsureAccess(ctx, host.SyncID, cfg)
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuth(
		e.Paths.SyncGCPConfig,
		host.Alias,
		result.User,
		result.IdentityFile,
		result.IdentitiesOnly,
	)
}

// DisableGCP turns off GCP sync and removes generated SSH config.
func (e *Engine) DisableGCP(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == gcp.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(); err != nil {
		return err
	}
	integration := e.Store.GCP()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetGCP(integration)
}

// Status returns a human-readable sync status snapshot.
func (e *Engine) Status(ctx context.Context) (Status, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	integration := e.Store.GCP()
	status := Status{
		GCP: GCPStatus{
			Enabled:           integration.Enabled,
			AutoSync:          integration.AutoSync,
			DefaultSSHUser:    integration.DefaultSSHUser,
			ProjectFilter:     append([]string(nil), integration.ProjectFilter...),
			ServiceAccounts:   append([]string(nil), integration.ServiceAccounts...),
			LastSyncAt:        integration.LastSyncAt,
			LastSyncError:     integration.LastSyncError,
			LastInstanceCount: integration.LastInstanceCount,
		},
	}
	if err := e.GCP.CheckAvailable(ctx); err != nil {
		status.GCP.GCloudError = err.Error()
		return status, nil
	}
	accounts, err := e.GCP.ListAccounts(ctx)
	if err != nil {
		status.GCP.GCloudError = err.Error()
		return status, nil
	}
	for _, account := range accounts {
		status.GCP.Accounts = append(status.GCP.Accounts, account.Account)
	}
	return status, nil
}

// Status is the CLI/TUI sync status payload.
type Status struct {
	GCP GCPStatus `json:"gcp"`
}

// GCPStatus describes GCP integration state.
type GCPStatus struct {
	Enabled           bool       `json:"enabled"`
	AutoSync          bool       `json:"autoSync"`
	Accounts          []string   `json:"accounts,omitempty"`
	ServiceAccounts   []string   `json:"serviceAccounts,omitempty"`
	ProjectFilter     []string   `json:"projectFilter,omitempty"`
	DefaultSSHUser    string     `json:"defaultSshUser,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
	GCloudError       string     `json:"gcloudError,omitempty"`
}

// IsSyncedGroup reports whether a group path is owned by cloud sync.
func IsSyncedGroup(group string) bool {
	group = strings.TrimSpace(group)
	return group == "Google Cloud" || strings.HasPrefix(group, "Google Cloud/") ||
		group == "GCP" || strings.HasPrefix(group, "GCP/")
}

// ValidateServiceAccountPath checks a key file path can be stored.
func ValidateServiceAccountPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("service account path is required")
	}
	return nil
}
