package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const CurrentVersion = 2

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

type State struct {
	Version     int             `json:"version"`
	Hosts       map[string]Host `json:"hosts"`
	Preferences Preferences     `json:"preferences,omitempty"`
}

type Store struct {
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
	s.state.Version = CurrentVersion
	return s, nil
}

func (s *Store) Host(alias string) Host { return s.state.Hosts[alias] }

func (s *Store) SetHost(alias string, host Host) error {
	host.Tags = cleanTags(host.Tags)
	s.state.Hosts[alias] = host
	return s.save()
}

func (s *Store) DeleteHost(alias string) error {
	delete(s.state.Hosts, alias)
	return s.save()
}

func (s *Store) RenameHost(from, to string) error {
	host, ok := s.state.Hosts[from]
	if ok {
		delete(s.state.Hosts, from)
		s.state.Hosts[to] = host
	}
	return s.save()
}

func (s *Store) ToggleFavorite(alias string) (bool, error) {
	host := s.state.Hosts[alias]
	host.Favorite = !host.Favorite
	s.state.Hosts[alias] = host
	return host.Favorite, s.save()
}

func (s *Store) ToggleHidden(alias string) (bool, error) {
	host := s.state.Hosts[alias]
	host.Hidden = !host.Hidden
	s.state.Hosts[alias] = host
	return host.Hidden, s.save()
}

func (s *Store) RecordUse(alias string) error {
	host := s.state.Hosts[alias]
	now := time.Now().UTC()
	host.LastUsedAt = &now
	host.ConnectionCount++
	s.state.Hosts[alias] = host
	return s.save()
}

func (s *Store) Preferences() Preferences { return s.state.Preferences }

func (s *Store) SetSort(sortOrder string) error {
	s.state.Preferences.Sort = sortOrder
	return s.save()
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
