package sync

import (
	"context"
	"fmt"
	"strings"
	stdsync "sync"
	"time"

	"bast/internal/cloud"
	awscloud "bast/internal/cloud/aws"
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
	AWS      *awscloud.Client
	Discover func(ctx context.Context) ([]sshconfig.Host, error)

	// EnsureAccessWait overrides the guest-agent propagation pause after publishing a key.
	// Zero keeps the GCP client default; negative skips the wait (tests).
	EnsureAccessWait time.Duration
}

type hostMetadataUpdate struct {
	alias         string
	previousAlias string
	label         string
	group         string
	tags          []string
}

func New(p paths.Paths, store *metadata.Store) *Engine {
	cfg := sshconfig.Manager{
		Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir,
		ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys,
		SyncGCPConfig: p.SyncGCPConfig, SyncAWSConfig: p.SyncAWSConfig,
	}
	return &Engine{
		Paths:  p,
		Config: cfg,
		Store:  store,
		GCP:    gcp.New(),
		AWS:    awscloud.New(),
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
	metadataUpdates := make([]hostMetadataUpdate, 0, len(discovery.Instances))
	var metadataDeletes []string

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
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: gcp.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
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
			metadataDeletes = append(metadataDeletes, host.Alias)
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

	if err := e.Config.EnsureSyncInclude(e.Paths.SyncGCPConfig); err != nil {
		return Result{}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncGCPConfig, blocks); err != nil {
		return Result{}, err
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
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

// SyncAWS discovers EC2 instances and writes the AWS sync SSH config.
func (e *Engine) SyncAWS(ctx context.Context) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	integration := e.Store.AWS()
	instances, err := e.AWS.Discover(ctx, awscloud.DiscoverConfig{
		ProfileFilter: integration.ProfileFilter, RegionFilter: integration.RegionFilter,
		DefaultSSHUser: integration.DefaultSSHUser, Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys,
	})
	now := time.Now().UTC()
	if err != nil {
		latest := e.Store.AWS()
		latest.Enabled = true
		latest.LastSyncAt = &now
		latest.LastSyncError = err.Error()
		_ = e.Store.SetAWS(latest)
		return Result{Provider: awscloud.ProviderName, SyncedAt: now, Error: err.Error()}, err
	}

	existing, err := e.Discover(ctx)
	if err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	usedAliases := map[string]bool{}
	previousBySyncID := map[string]sshconfig.Host{}
	for _, host := range existing {
		if host.Synced && host.SyncSource == awscloud.ProviderName && host.SyncID != "" {
			previousBySyncID[host.SyncID] = host
			continue
		}
		usedAliases[host.Alias] = true
	}

	blocks := make([]sshconfig.SyncHostInput, 0, len(instances))
	aliases := make([]string, 0, len(instances))
	activeSyncIDs := map[string]bool{}
	metadataUpdates := make([]hostMetadataUpdate, 0, len(instances))
	for _, inst := range instances {
		activeSyncIDs[inst.SyncID] = true
		alias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok && !usedAliases[prev.Alias] {
			alias = prev.Alias
			usedAliases[alias] = true
		} else {
			alias = awscloud.UniqueAlias(awscloud.AliasFor(inst), usedAliases)
			usedAliases[alias] = true
		}
		blocks = append(blocks, awscloud.ToSyncHost(inst, alias))
		aliases = append(aliases, alias)
		prevAlias := ""
		if prev, ok := previousBySyncID[inst.SyncID]; ok {
			prevAlias = prev.Alias
		}
		metadataUpdates = append(metadataUpdates, hostMetadataUpdate{
			alias: alias, previousAlias: prevAlias, label: inst.Name,
			group: awscloud.GroupPath(inst), tags: append([]string(nil), inst.Tags...),
		})
	}
	if err := e.Config.EnsureSyncInclude(e.Paths.SyncAWSConfig); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	if err := sshconfig.WriteSyncConfig(e.Paths.SyncAWSConfig, blocks); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	var metadataDeletes []string
	for syncID, host := range previousBySyncID {
		if !activeSyncIDs[syncID] {
			metadataDeletes = append(metadataDeletes, host.Alias)
		}
	}
	if err := e.applyMetadataUpdates(metadataUpdates, metadataDeletes); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	latest := e.Store.AWS()
	latest.Enabled = true
	latest.LastSyncAt = &now
	latest.LastSyncError = ""
	latest.LastInstanceCount = len(instances)
	if err := e.Store.SetAWS(latest); err != nil {
		return Result{Provider: awscloud.ProviderName}, err
	}
	return Result{Provider: awscloud.ProviderName, Count: len(instances), SyncedAt: now, Aliases: aliases}, nil
}

func (e *Engine) applyMetadataUpdates(updates []hostMetadataUpdate, deletes []string) error {
	return e.Store.UpdateHosts(func(hosts map[string]metadata.Host) {
		for _, update := range updates {
			if update.previousAlias != "" && update.previousAlias != update.alias {
				if previous, ok := hosts[update.previousAlias]; ok {
					delete(hosts, update.previousAlias)
					hosts[update.alias] = previous
				}
			}
			meta := hosts[update.alias]
			meta.Label = update.label
			meta.Group = update.group
			meta.Tags = append([]string(nil), update.tags...)
			hosts[update.alias] = meta
		}
		for _, alias := range deletes {
			delete(hosts, alias)
		}
	})
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

// EnsureAWSAccess prepares SSH auth for an AWS-synced host immediately before connect.
func (e *Engine) EnsureAWSAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !host.Synced || host.SyncSource != awscloud.ProviderName || host.SyncID == "" {
		return nil
	}
	integration := e.Store.AWS()
	result, err := e.AWS.EnsureAccess(ctx, host.SyncID, awscloud.EnsureConfig{
		Home: e.Paths.Home, ManagedKeys: e.Paths.ManagedKeys, ProfileFilter: integration.ProfileFilter,
		DefaultSSHUser: integration.DefaultSSHUser, Status: status,
	})
	if err != nil {
		return err
	}
	return sshconfig.UpdateSyncHostAuth(e.Paths.SyncAWSConfig, host.Alias, result.User, result.IdentityFile, result.IdentitiesOnly)
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
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncGCPConfig); err != nil {
		return err
	}
	integration := e.Store.GCP()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetGCP(integration)
}

// DisableAWS turns off AWS sync and removes only generated AWS state.
func (e *Engine) DisableAWS(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	existing, err := e.Discover(ctx)
	if err != nil {
		return err
	}
	for _, host := range existing {
		if host.Synced && host.SyncSource == awscloud.ProviderName {
			_ = e.Store.DeleteHost(host.Alias)
		}
	}
	if err := e.Config.RemoveSyncInclude(e.Paths.SyncAWSConfig); err != nil {
		return err
	}
	integration := e.Store.AWS()
	integration.Enabled = false
	integration.LastSyncError = ""
	integration.LastInstanceCount = 0
	return e.Store.SetAWS(integration)
}

// Status returns a human-readable sync status snapshot.
func (e *Engine) Status(ctx context.Context) (Status, error) {
	e.mu.Lock()
	integration := e.Store.GCP()
	awsIntegration := e.Store.AWS()
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
		AWS: AWSStatus{
			Enabled: awsIntegration.Enabled, AutoSync: awsIntegration.AutoSync,
			ProfileFilter:  append([]string(nil), awsIntegration.ProfileFilter...),
			RegionFilter:   append([]string(nil), awsIntegration.RegionFilter...),
			DefaultSSHUser: awsIntegration.DefaultSSHUser, LastSyncAt: awsIntegration.LastSyncAt,
			LastSyncError: awsIntegration.LastSyncError, LastInstanceCount: awsIntegration.LastInstanceCount,
		},
	}
	e.mu.Unlock()

	if err := e.GCP.CheckAvailable(ctx); err != nil {
		status.GCP.GCloudError = err.Error()
	} else {
		accounts, listErr := e.GCP.ListAccounts(ctx)
		if listErr != nil {
			status.GCP.GCloudError = listErr.Error()
		} else {
			for _, account := range accounts {
				status.GCP.Accounts = append(status.GCP.Accounts, account.Account)
			}
		}
	}
	if err := e.AWS.CheckAvailable(ctx); err != nil {
		status.AWS.AWSCLIError = err.Error()
	} else {
		profiles, listErr := e.AWS.ListProfiles(ctx)
		if listErr != nil {
			status.AWS.AWSCLIError = listErr.Error()
		} else {
			status.AWS.Profiles = profiles
		}
	}
	return status, nil
}

// Status is the CLI/TUI sync status payload.
type Status struct {
	GCP GCPStatus `json:"gcp"`
	AWS AWSStatus `json:"aws"`
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

type AWSStatus struct {
	Enabled           bool       `json:"enabled"`
	AutoSync          bool       `json:"autoSync"`
	Profiles          []string   `json:"profiles,omitempty"`
	ProfileFilter     []string   `json:"profileFilter,omitempty"`
	RegionFilter      []string   `json:"regionFilter,omitempty"`
	DefaultSSHUser    string     `json:"defaultSshUser,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
	AWSCLIError       string     `json:"awsCliError,omitempty"`
}

// IsSyncedGroup reports whether a group path is owned by cloud sync.
func IsSyncedGroup(group string) bool {
	group = strings.TrimSpace(group)
	return group == "Google Cloud" || strings.HasPrefix(group, "Google Cloud/") ||
		group == "GCP" || strings.HasPrefix(group, "GCP/") ||
		group == "Amazon EC2" || strings.HasPrefix(group, "Amazon EC2/") ||
		group == "AWS" || strings.HasPrefix(group, "AWS/")
}

// ValidateServiceAccountPath checks a key file path can be stored.
func ValidateServiceAccountPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("service account path is required")
	}
	return nil
}
