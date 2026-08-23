package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bast/internal/platform"
)

const CurrentVersion = 7

type Host struct {
	Label           string     `json:"label,omitempty"`
	Favorite        bool       `json:"favorite,omitempty"`
	Hidden          bool       `json:"hidden,omitempty"`
	LastUsedAt      *time.Time `json:"lastUsedAt,omitempty"`
	ConnectionCount int        `json:"connectionCount,omitempty"`
	Group           string     `json:"group,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
	Environment     string     `json:"environment,omitempty"`
	Color           string     `json:"color,omitempty"`
	Notes           string     `json:"notes,omitempty"`
}

type Preferences struct {
	Sort            string   `json:"sort,omitempty"`
	CollapsedGroups []string `json:"collapsedGroups,omitempty"`
}

type HistorySource struct {
	Offset   int64    `json:"offset,omitempty"`
	TailHash string   `json:"tailHash,omitempty"`
	Anchors  []string `json:"anchors,omitempty"`
}

type HistorySuggestion struct {
	ID           string `json:"id"`
	Alias        string `json:"alias"`
	Target       string `json:"target"`
	HostName     string `json:"hostname"`
	User         string `json:"user,omitempty"`
	Port         string `json:"port,omitempty"`
	IdentityFile string `json:"identityFile,omitempty"`
	ProxyJump    string `json:"proxyJump,omitempty"`
	Source       string `json:"source"`
	SeenAt       int64  `json:"seenAt"`
}

type HistoryImport struct {
	Sources map[string]HistorySource `json:"sources,omitempty"`
	Pending []HistorySuggestion      `json:"pending,omitempty"`
}

type GCPIntegration struct {
	Enabled           bool       `json:"enabled"`
	ServiceAccounts   []string   `json:"serviceAccounts,omitempty"`
	ProjectFilter     []string   `json:"projectFilter,omitempty"`
	DefaultSSHUser    string     `json:"defaultSshUser,omitempty"`
	AutoSync          bool       `json:"autoSync,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
}

type AWSIntegration struct {
	Enabled           bool       `json:"enabled"`
	ProfileFilter     []string   `json:"profileFilter,omitempty"`
	RegionFilter      []string   `json:"regionFilter,omitempty"`
	DefaultSSHUser    string     `json:"defaultSshUser,omitempty"`
	AutoSync          bool       `json:"autoSync,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
}

type AzureIntegration struct {
	Enabled             bool       `json:"enabled"`
	SubscriptionFilter  []string   `json:"subscriptionFilter,omitempty"`
	ResourceGroupFilter []string   `json:"resourceGroupFilter,omitempty"`
	DefaultSSHUser      string     `json:"defaultSshUser,omitempty"`
	AutoSync            bool       `json:"autoSync,omitempty"`
	LastSyncAt          *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError       string     `json:"lastSyncError,omitempty"`
	LastInstanceCount   int        `json:"lastInstanceCount,omitempty"`
}

type BoxIntegration struct {
	Enabled           bool       `json:"enabled"`
	AutoSync          bool       `json:"autoSync,omitempty"`
	Disabled          bool       `json:"disabled,omitempty"` // sticky user opt-out; blocks auto-connect
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
}

type UpstashIntegration struct {
	Enabled           bool       `json:"enabled"`
	AutoSync          bool       `json:"autoSync,omitempty"`
	Disabled          bool       `json:"disabled,omitempty"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncError     string     `json:"lastSyncError,omitempty"`
	LastInstanceCount int        `json:"lastInstanceCount,omitempty"`
}

type Integrations struct {
	GCP     *GCPIntegration     `json:"gcp,omitempty"`
	AWS     *AWSIntegration     `json:"aws,omitempty"`
	Azure   *AzureIntegration   `json:"azure,omitempty"`
	Box     *BoxIntegration     `json:"box,omitempty"`
	Upstash *UpstashIntegration `json:"upstash,omitempty"`
}

type State struct {
	Version      int             `json:"version"`
	Hosts        map[string]Host `json:"hosts"`
	Preferences  Preferences     `json:"preferences,omitempty"`
	Integrations Integrations    `json:"integrations,omitempty"`
	History      HistoryImport   `json:"history,omitempty"`
}

type Store struct {
	mu               sync.RWMutex
	path             string
	state            State
	directorySecured bool
	hostRevision     atomic.Uint64
	historyRevision  atomic.Uint64
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, state: State{Version: CurrentVersion, Hosts: map[string]Host{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	if err := json.Unmarshal(b, &s.state); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}
	if s.state.Version > CurrentVersion {
		return nil, fmt.Errorf("metadata schema %d is newer than this Bast supports", s.state.Version)
	}
	if s.state.Hosts == nil {
		s.state.Hosts = map[string]Host{}
	}
	for alias, host := range s.state.Hosts {
		if host.Group == "GCP" {
			host.Group = "Google Cloud"
		} else if strings.HasPrefix(host.Group, "GCP/") {
			host.Group = "Google Cloud/" + strings.TrimPrefix(host.Group, "GCP/")
		} else if host.Group == "AWS" {
			host.Group = "Amazon EC2"
		} else if strings.HasPrefix(host.Group, "AWS/") {
			host.Group = "Amazon EC2/" + strings.TrimPrefix(host.Group, "AWS/")
		}
		s.state.Hosts[alias] = host
	}
	s.state.Version = CurrentVersion
	return s, nil
}

func (s *Store) Host(alias string) Host {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneHost(s.state.Hosts[alias])
}

func (s *Store) Hosts() map[string]Host {
	hosts, _ := s.HostsSnapshot()
	return hosts
}

func (s *Store) HostsSnapshot() (map[string]Host, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Host, len(s.state.Hosts))
	for k, v := range s.state.Hosts {
		out[k] = cloneHost(v)
	}
	return out, s.hostRevision.Load()
}

func (s *Store) HostRevision() uint64 {
	return s.hostRevision.Load()
}

func (s *Store) SetHost(alias string, host Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.state.Hosts[alias]
	host.Tags = cleanTags(host.Tags)
	s.state.Hosts[alias] = host
	if err := s.save(); err != nil {
		if existed {
			s.state.Hosts[alias] = previous
		} else {
			delete(s.state.Hosts, alias)
		}
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

// MoveHost updates a host's group and the collapsed-group preferences in one
// state-file replacement so the UI cannot report a failed move after the host
// change has already been persisted.
func (s *Store) MoveHost(alias, group string, collapsedGroups []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	host, existed := s.state.Hosts[alias]
	previousHost := host
	previousPreferences := s.state.Preferences
	host.Group = group
	s.state.Hosts[alias] = host
	s.state.Preferences.CollapsedGroups = cleanCollapsedGroups(collapsedGroups)
	if err := s.save(); err != nil {
		if existed {
			s.state.Hosts[alias] = previousHost
		} else {
			delete(s.state.Hosts, alias)
		}
		s.state.Preferences = previousPreferences
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

// UpdateHosts applies a related set of host metadata changes with one atomic
// persistence operation. The callback must not call other Store methods.
func (s *Store) UpdateHosts(update func(map[string]Host)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneHosts(s.state.Hosts)
	update(next)
	for alias, host := range next {
		host.Tags = cleanTags(host.Tags)
		next[alias] = host
	}
	previous := s.state.Hosts
	s.state.Hosts = next
	if err := s.save(); err != nil {
		s.state.Hosts = previous
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

func (s *Store) DeleteHost(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.state.Hosts[alias]
	delete(s.state.Hosts, alias)
	if err := s.save(); err != nil {
		if existed {
			s.state.Hosts[alias] = previous
		}
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

func (s *Store) RenameHost(from, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	host, ok := s.state.Hosts[from]
	previousDestination, destinationExisted := s.state.Hosts[to]
	if ok {
		delete(s.state.Hosts, from)
		s.state.Hosts[to] = host
	}
	if err := s.save(); err != nil {
		if ok {
			s.state.Hosts[from] = host
			if destinationExisted {
				s.state.Hosts[to] = previousDestination
			} else if to != from {
				delete(s.state.Hosts, to)
			}
		}
		return err
	}
	if ok {
		s.hostRevision.Add(1)
	}
	return nil
}

func (s *Store) ToggleFavorite(alias string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	host, existed := s.state.Hosts[alias]
	previous := host
	host.Favorite = !host.Favorite
	s.state.Hosts[alias] = host
	if err := s.saveQuick(); err != nil {
		if existed {
			s.state.Hosts[alias] = previous
		} else {
			delete(s.state.Hosts, alias)
		}
		return host.Favorite, err
	}
	s.hostRevision.Add(1)
	return host.Favorite, nil
}

func (s *Store) ToggleHidden(alias string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	host, existed := s.state.Hosts[alias]
	previous := host
	host.Hidden = !host.Hidden
	s.state.Hosts[alias] = host
	if err := s.saveQuick(); err != nil {
		if existed {
			s.state.Hosts[alias] = previous
		} else {
			delete(s.state.Hosts, alias)
		}
		return host.Hidden, err
	}
	s.hostRevision.Add(1)
	return host.Hidden, nil
}

func (s *Store) RecordUse(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	host, existed := s.state.Hosts[alias]
	previous := host
	now := time.Now().UTC()
	host.LastUsedAt = &now
	host.ConnectionCount++
	s.state.Hosts[alias] = host
	if err := s.saveQuick(); err != nil {
		if existed {
			s.state.Hosts[alias] = previous
		} else {
			delete(s.state.Hosts, alias)
		}
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

func (s *Store) Preferences() Preferences {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefs := s.state.Preferences
	if prefs.CollapsedGroups != nil {
		prefs.CollapsedGroups = append([]string(nil), prefs.CollapsedGroups...)
	}
	return prefs
}

func (s *Store) SetSort(sortOrder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Preferences
	s.state.Preferences.Sort = sortOrder
	if err := s.saveQuick(); err != nil {
		s.state.Preferences = previous
		return err
	}
	return nil
}

func (s *Store) SetCollapsedGroups(groups []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Preferences
	s.state.Preferences.CollapsedGroups = cleanCollapsedGroups(groups)
	if err := s.saveQuick(); err != nil {
		s.state.Preferences = previous
		return err
	}
	return nil
}

func (s *Store) HistoryImport() HistoryImport {
	history, _ := s.HistoryImportSnapshot()
	return history
}

func (s *Store) HistoryImportSnapshot() (HistoryImport, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneHistoryImport(s.state.History), s.historyRevision.Load()
}

func (s *Store) SetHistoryImport(history HistoryImport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.History
	s.state.History = cloneHistoryImport(history)
	if err := s.save(); err != nil {
		s.state.History = previous
		return err
	}
	s.historyRevision.Add(1)
	return nil
}

// CommitHistoryImport applies a scan only if the pending state has not changed
// since the scan began. This prevents a background scan from resurrecting a
// suggestion that the user dismissed while it was running.
func (s *Store) CommitHistoryImport(expectedRevision uint64, history HistoryImport) (HistoryImport, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.historyRevision.Load() != expectedRevision {
		return cloneHistoryImport(s.state.History), false, nil
	}
	previous := s.state.History
	s.state.History = cloneHistoryImport(history)
	if err := s.save(); err != nil {
		s.state.History = previous
		return cloneHistoryImport(previous), false, err
	}
	s.historyRevision.Add(1)
	return cloneHistoryImport(s.state.History), true, nil
}

func (s *Store) DismissHistorySuggestion(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.History
	next := cloneHistoryImport(previous)
	next.Pending = removeHistorySuggestion(next.Pending, id)
	s.state.History = next
	if err := s.save(); err != nil {
		s.state.History = previous
		return err
	}
	s.historyRevision.Add(1)
	return nil
}

// AcceptHistorySuggestion records host metadata and removes its history
// suggestion in one state-file replacement. The SSH config write is managed by
// the caller because it belongs to a separate file.
func (s *Store) AcceptHistorySuggestion(alias string, host Host, suggestionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previousHost, hostExisted := s.state.Hosts[alias]
	previousHistory := s.state.History
	host.Tags = cleanTags(host.Tags)
	s.state.Hosts[alias] = host
	nextHistory := cloneHistoryImport(previousHistory)
	nextHistory.Pending = removeHistorySuggestion(nextHistory.Pending, suggestionID)
	s.state.History = nextHistory
	if err := s.save(); err != nil {
		if hostExisted {
			s.state.Hosts[alias] = previousHost
		} else {
			delete(s.state.Hosts, alias)
		}
		s.state.History = previousHistory
		return err
	}
	s.hostRevision.Add(1)
	s.historyRevision.Add(1)
	return nil
}

func (s *Store) Integrations() Integrations {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneIntegrations(s.state.Integrations)
}

func (s *Store) GCP() GCPIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Integrations.GCP == nil {
		return GCPIntegration{}
	}
	return cloneGCP(*s.state.Integrations.GCP)
}

func (s *Store) SetGCP(gcp GCPIntegration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Integrations.GCP
	if !gcp.Enabled && len(gcp.ServiceAccounts) == 0 && len(gcp.ProjectFilter) == 0 &&
		gcp.DefaultSSHUser == "" && !gcp.AutoSync && gcp.LastSyncAt == nil &&
		gcp.LastSyncError == "" && gcp.LastInstanceCount == 0 {
		s.state.Integrations.GCP = nil
	} else {
		copy := cloneGCP(gcp)
		s.state.Integrations.GCP = &copy
	}
	if err := s.save(); err != nil {
		s.state.Integrations.GCP = previous
		return err
	}
	return nil
}

func (s *Store) AWS() AWSIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Integrations.AWS == nil {
		return AWSIntegration{}
	}
	return cloneAWS(*s.state.Integrations.AWS)
}

func (s *Store) SetAWS(aws AWSIntegration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Integrations.AWS
	if !aws.Enabled && len(aws.ProfileFilter) == 0 && len(aws.RegionFilter) == 0 &&
		aws.DefaultSSHUser == "" && !aws.AutoSync && aws.LastSyncAt == nil &&
		aws.LastSyncError == "" && aws.LastInstanceCount == 0 {
		s.state.Integrations.AWS = nil
	} else {
		copy := cloneAWS(aws)
		s.state.Integrations.AWS = &copy
	}
	if err := s.save(); err != nil {
		s.state.Integrations.AWS = previous
		return err
	}
	return nil
}

func (s *Store) Azure() AzureIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Integrations.Azure == nil {
		return AzureIntegration{}
	}
	return cloneAzure(*s.state.Integrations.Azure)
}

func (s *Store) SetAzure(azure AzureIntegration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Integrations.Azure
	if !azure.Enabled && len(azure.SubscriptionFilter) == 0 && len(azure.ResourceGroupFilter) == 0 &&
		azure.DefaultSSHUser == "" && !azure.AutoSync && azure.LastSyncAt == nil &&
		azure.LastSyncError == "" && azure.LastInstanceCount == 0 {
		s.state.Integrations.Azure = nil
	} else {
		copy := cloneAzure(azure)
		s.state.Integrations.Azure = &copy
	}
	if err := s.save(); err != nil {
		s.state.Integrations.Azure = previous
		return err
	}
	return nil
}

func (s *Store) Box() BoxIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Integrations.Box == nil {
		return BoxIntegration{}
	}
	return cloneBox(*s.state.Integrations.Box)
}

func (s *Store) SetBox(box BoxIntegration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Integrations.Box
	if !box.Enabled && !box.AutoSync && !box.Disabled && box.LastSyncAt == nil &&
		box.LastSyncError == "" && box.LastInstanceCount == 0 {
		s.state.Integrations.Box = nil
	} else {
		copy := cloneBox(box)
		s.state.Integrations.Box = &copy
	}
	if err := s.save(); err != nil {
		s.state.Integrations.Box = previous
		return err
	}
	return nil
}

func (s *Store) Upstash() UpstashIntegration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Integrations.Upstash == nil {
		return UpstashIntegration{}
	}
	return cloneUpstash(*s.state.Integrations.Upstash)
}

func (s *Store) SetUpstash(upstash UpstashIntegration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Integrations.Upstash
	if !upstash.Enabled && !upstash.AutoSync && !upstash.Disabled && upstash.LastSyncAt == nil &&
		upstash.LastSyncError == "" && upstash.LastInstanceCount == 0 {
		s.state.Integrations.Upstash = nil
	} else {
		copy := cloneUpstash(upstash)
		s.state.Integrations.Upstash = &copy
	}
	if err := s.save(); err != nil {
		s.state.Integrations.Upstash = previous
		return err
	}
	return nil
}

func cloneHost(host Host) Host {
	host.Tags = append([]string(nil), host.Tags...)
	if host.LastUsedAt != nil {
		lastUsedAt := *host.LastUsedAt
		host.LastUsedAt = &lastUsedAt
	}
	return host
}

func cloneHistoryImport(history HistoryImport) HistoryImport {
	out := HistoryImport{Pending: append([]HistorySuggestion(nil), history.Pending...)}
	if history.Sources != nil {
		out.Sources = make(map[string]HistorySource, len(history.Sources))
		for path, source := range history.Sources {
			source.Anchors = append([]string(nil), source.Anchors...)
			out.Sources[path] = source
		}
	}
	return out
}

func removeHistorySuggestion(suggestions []HistorySuggestion, id string) []HistorySuggestion {
	out := make([]HistorySuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if suggestion.ID != id {
			out = append(out, suggestion)
		}
	}
	return out
}

func cloneHosts(hosts map[string]Host) map[string]Host {
	out := make(map[string]Host, len(hosts))
	for alias, host := range hosts {
		out[alias] = cloneHost(host)
	}
	return out
}

func cloneGCP(gcp GCPIntegration) GCPIntegration {
	gcp.ServiceAccounts = append([]string(nil), gcp.ServiceAccounts...)
	gcp.ProjectFilter = append([]string(nil), gcp.ProjectFilter...)
	if gcp.LastSyncAt != nil {
		lastSyncAt := *gcp.LastSyncAt
		gcp.LastSyncAt = &lastSyncAt
	}
	return gcp
}

func cloneAWS(aws AWSIntegration) AWSIntegration {
	aws.ProfileFilter = append([]string(nil), aws.ProfileFilter...)
	aws.RegionFilter = append([]string(nil), aws.RegionFilter...)
	if aws.LastSyncAt != nil {
		lastSyncAt := *aws.LastSyncAt
		aws.LastSyncAt = &lastSyncAt
	}
	return aws
}

func cloneAzure(azure AzureIntegration) AzureIntegration {
	azure.SubscriptionFilter = append([]string(nil), azure.SubscriptionFilter...)
	azure.ResourceGroupFilter = append([]string(nil), azure.ResourceGroupFilter...)
	if azure.LastSyncAt != nil {
		lastSyncAt := *azure.LastSyncAt
		azure.LastSyncAt = &lastSyncAt
	}
	return azure
}

func cloneBox(box BoxIntegration) BoxIntegration {
	if box.LastSyncAt != nil {
		lastSyncAt := *box.LastSyncAt
		box.LastSyncAt = &lastSyncAt
	}
	return box
}

func cloneUpstash(upstash UpstashIntegration) UpstashIntegration {
	if upstash.LastSyncAt != nil {
		lastSyncAt := *upstash.LastSyncAt
		upstash.LastSyncAt = &lastSyncAt
	}
	return upstash
}

func cloneIntegrations(integrations Integrations) Integrations {
	out := Integrations{}
	if integrations.GCP != nil {
		gcp := cloneGCP(*integrations.GCP)
		out.GCP = &gcp
	}
	if integrations.AWS != nil {
		aws := cloneAWS(*integrations.AWS)
		out.AWS = &aws
	}
	if integrations.Azure != nil {
		azure := cloneAzure(*integrations.Azure)
		out.Azure = &azure
	}
	if integrations.Box != nil {
		box := cloneBox(*integrations.Box)
		out.Box = &box
	}
	if integrations.Upstash != nil {
		upstash := cloneUpstash(*integrations.Upstash)
		out.Upstash = &upstash
	}
	return out
}

func (s *Store) save() error {
	return s.writeState(true)
}

// saveQuick persists without fsync. Used for high-frequency preference and usage
// writes where durability can wait for the next durable save.
func (s *Store) saveQuick() error {
	return s.writeState(false)
}

func (s *Store) writeState(syncDisk bool) error {
	dir := filepath.Dir(s.path)
	if err := s.ensureDirectory(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if syncDisk {
		if err := tmp.Sync(); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := platform.ReplaceFile(tmpName, s.path); err != nil {
		return err
	}
	if err := platform.SecurePath(s.path, 0600); err != nil {
		return err
	}
	if !syncDisk {
		return nil
	}
	return platform.SyncDirectory(dir)
}

func (s *Store) ensureDirectory(dir string) error {
	created := false
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create metadata directory: %w", err)
		}
		created = true
	} else if err != nil {
		return fmt.Errorf("inspect metadata directory: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("metadata directory path is not a directory: %s", dir)
	}
	if s.directorySecured && !created {
		return nil
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return fmt.Errorf("secure metadata directory: %w", err)
	}
	s.directorySecured = true
	return nil
}

func cleanTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

func cleanCollapsedGroups(groups []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		out = append(out, group)
	}
	sort.Strings(out)
	return out
}
