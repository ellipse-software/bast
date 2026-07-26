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
	"time"
)

const CurrentVersion = 3

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

type Integrations struct {
	GCP *GCPIntegration `json:"gcp,omitempty"`
}

type State struct {
	Version      int             `json:"version"`
	Hosts        map[string]Host `json:"hosts"`
	Preferences  Preferences     `json:"preferences,omitempty"`
	Integrations Integrations    `json:"integrations,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	state State
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Host, len(s.state.Hosts))
	for k, v := range s.state.Hosts {
		out[k] = cloneHost(v)
	}
	return out
}

func (s *Store) SetHost(alias string, host Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	host.Tags = cleanTags(host.Tags)
	s.state.Hosts[alias] = host
	return s.save()
}

func (s *Store) DeleteHost(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Hosts, alias)
	return s.save()
}

func (s *Store) RenameHost(from, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	host, ok := s.state.Hosts[from]
	if ok {
		delete(s.state.Hosts, from)
		s.state.Hosts[to] = host
	}
	return s.save()
}

func (s *Store) ToggleFavorite(alias string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	host := s.state.Hosts[alias]
	host.Favorite = !host.Favorite
	s.state.Hosts[alias] = host
	return host.Favorite, s.save()
}

func (s *Store) ToggleHidden(alias string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	host := s.state.Hosts[alias]
	host.Hidden = !host.Hidden
	s.state.Hosts[alias] = host
	return host.Hidden, s.save()
}

func (s *Store) RecordUse(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	host := s.state.Hosts[alias]
	now := time.Now().UTC()
	host.LastUsedAt = &now
	host.ConnectionCount++
	s.state.Hosts[alias] = host
	return s.save()
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

func cloneHost(host Host) Host {
	host.Tags = append([]string(nil), host.Tags...)
	if host.LastUsedAt != nil {
		lastUsedAt := *host.LastUsedAt
		host.LastUsedAt = &lastUsedAt
	}
	return host
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

func cloneIntegrations(integrations Integrations) Integrations {
	if integrations.GCP == nil {
		return Integrations{}
	}
	gcp := cloneGCP(*integrations.GCP)
	return Integrations{GCP: &gcp}
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
