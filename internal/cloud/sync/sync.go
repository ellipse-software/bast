package sync

import (
	"context"
	"fmt"
	"strings"
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
	Paths    paths.Paths
	Config   sshconfig.Manager
	Store    *metadata.Store
	GCP      *gcp.Client
	Discover func(ctx context.Context) ([]sshconfig.Host, error)
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
	integration := e.Store.GCP()
	instances, err := e.GCP.Discover(ctx, cloud.DiscoverConfig{
		ProjectFilter:   integration.ProjectFilter,
		DefaultSSHUser:  integration.DefaultSSHUser,
		ServiceAccounts: integration.ServiceAccounts,
		Home:            e.Paths.Home,
		ManagedKeys:     e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		integration.Enabled = true
		integration.LastSyncAt = &now
		integration.LastSyncError = err.Error()
		_ = e.Store.SetGCP(integration)
		return Result{Provider: gcp.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{}, err
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

	blocks := make([]sshconfig.SyncHostInput, 0, len(instances))
	aliases := make([]string, 0, len(instances))
	activeAliases := map[string]bool{}
	activeSyncIDs := map[string]bool{}

	for _, inst := range instances {
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

	if err := e.Config.EnsureSyncInclude(); err != nil {
		return Result{}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncGCPConfig, blocks); err != nil {
		return Result{}, err
	}

	for syncID, host := range previousBySyncID {
		if activeSyncIDs[syncID] {
			continue
		}
		_ = e.Store.DeleteHost(host.Alias)
	}

	integration.Enabled = true
	integration.LastSyncAt = &now
	integration.LastSyncError = ""
	integration.LastInstanceCount = len(instances)
	if err := e.Store.SetGCP(integration); err != nil {
		return Result{}, err
	}
	return Result{
		Provider: gcp.ProviderName,
		Count:    len(instances),
		SyncedAt: now,
		Aliases:  aliases,
	}, nil
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

// DisableGCP turns off GCP sync and removes generated SSH config.
func (e *Engine) DisableGCP(ctx context.Context) error {
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
	return group == "GCP" || strings.HasPrefix(group, "GCP/")
}

// ValidateServiceAccountPath checks a key file path can be stored.
func ValidateServiceAccountPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("service account path is required")
	}
	return nil
}
