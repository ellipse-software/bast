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
)

const CurrentVersion = 6

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
	Sort string `json:"sort,omitempty"`
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

type Integrations struct {
	GCP   *GCPIntegration   `json:"gcp,omitempty"`
	AWS   *AWSIntegration   `json:"aws,omitempty"`
	Azure *AzureIntegration `json:"azure,omitempty"`
}

type State struct {
	Version      int             `json:"version"`
	Hosts        map[string]Host `json:"hosts"`
	Preferences  Preferences     `json:"preferences,omitempty"`
	Integrations Integrations    `json:"integrations,omitempty"`
}

type Store struct {
	mu           sync.RWMutex
	path         string
	state        State
	hostRevision atomic.Uint64
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
	if err := s.save(); err != nil {
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
	if err := s.save(); err != nil {
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

func (s *Store) Preferences() Preferences {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Preferences
}

func (s *Store) SetSort(sortOrder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Preferences.Sort = sortOrder
	return s.save()
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
	if !gcp.Enabled && len(gcp.ServiceAccounts) == 0 && len(gcp.ProjectFilter) == 0 &&
		gcp.DefaultSSHUser == "" && !gcp.AutoSync && gcp.LastSyncAt == nil &&
		gcp.LastSyncError == "" && gcp.LastInstanceCount == 0 {
		s.state.Integrations.GCP = nil
	} else {
		copy := cloneGCP(gcp)
		s.state.Integrations.GCP = &copy
	}
	return s.save()
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
	if !aws.Enabled && len(aws.ProfileFilter) == 0 && len(aws.RegionFilter) == 0 &&
		aws.DefaultSSHUser == "" && !aws.AutoSync && aws.LastSyncAt == nil &&
		aws.LastSyncError == "" && aws.LastInstanceCount == 0 {
		s.state.Integrations.AWS = nil
	} else {
		copy := cloneAWS(aws)
		s.state.Integrations.AWS = &copy
	}
	return s.save()
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
	if !azure.Enabled && len(azure.SubscriptionFilter) == 0 && len(azure.ResourceGroupFilter) == 0 &&
		azure.DefaultSSHUser == "" && !azure.AutoSync && azure.LastSyncAt == nil &&
		azure.LastSyncError == "" && azure.LastInstanceCount == 0 {
		s.state.Integrations.Azure = nil
	} else {
		copy := cloneAzure(azure)
		s.state.Integrations.Azure = &copy
	}
	return s.save()
}

func cloneHost(host Host) Host {
	host.Tags = append([]string(nil), host.Tags...)
	if host.LastUsedAt != nil {
		lastUsedAt := *host.LastUsedAt
		host.LastUsedAt = &lastUsedAt
	}
	return host
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
	return out
}

func (s *Store) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
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
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func cleanTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}
