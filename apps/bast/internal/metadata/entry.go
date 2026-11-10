package metadata

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"bast/internal/page"
	"bast/internal/rdp"
)

const (
	EntryRDP = "rdp"
	EntryWeb = "web"
)

// Entry is a Remote Desktop target or an internal web page. It is not an SSH
// host and is never written to SSH config. Via, when set, is an SSH alias.
type Entry struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	Label           string     `json:"label"`
	Favorite        bool       `json:"favorite,omitempty"`
	Hidden          bool       `json:"hidden,omitempty"`
	LastUsedAt      *time.Time `json:"lastUsedAt,omitempty"`
	ConnectionCount int        `json:"connectionCount,omitempty"`
	Group           string     `json:"group,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
	Environment     string     `json:"environment,omitempty"`
	Color           string     `json:"color,omitempty"`
	Icon            string     `json:"icon,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	Via             string     `json:"via,omitempty"`
	Address         string     `json:"address,omitempty"`
	Port            string     `json:"port,omitempty"`
	User            string     `json:"user,omitempty"`
	Domain          string     `json:"domain,omitempty"`
	Fullscreen      bool       `json:"fullscreen,omitempty"`
	Admin           bool       `json:"admin,omitempty"`
	URL             string     `json:"url,omitempty"`
	UpdatedAt       int64      `json:"updatedAt,omitempty"`
}

func (s *Store) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneEntryList(s.state.Entries)
}

func (s *Store) Entry(id string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.state.Entries[id]
	if !ok {
		return Entry{}, false
	}
	return cloneEntry(entry), true
}

// EntryByLabel returns the single entry of kind whose label matches, ignoring case.
func (s *Store) EntryByLabel(kind, label string) (Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return entryByLabelLocked(s.state.Entries, kind, label)
}

// EntryByAnyLabel returns one entry when the label matches exactly one entry of either kind.
func (s *Store) EntryByAnyLabel(label string) (Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var found []Entry
	for _, entry := range s.state.Entries {
		if sameEntryLabel(entry.Label, label) {
			found = append(found, cloneEntry(entry))
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) == 0 {
		return Entry{}, fmt.Errorf("unknown entry %q", strings.TrimSpace(label))
	}
	return Entry{}, fmt.Errorf("entry label %q is ambiguous; use bast rdp open or bast web open", strings.TrimSpace(label))
}

func (s *Store) PutEntry(entry Entry) (Entry, error) {
	normalized, err := NormalizeEntry(entry)
	if err != nil {
		return Entry{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Entries == nil {
		s.state.Entries = map[string]Entry{}
	}
	if normalized.ID == "" {
		id, err := newEntryID()
		if err != nil {
			return Entry{}, err
		}
		for _, taken := s.state.Entries[id]; taken; _, taken = s.state.Entries[id] {
			id, err = newEntryID()
			if err != nil {
				return Entry{}, err
			}
		}
		normalized.ID = id
	} else if _, ok := s.state.Entries[normalized.ID]; !ok {
		return Entry{}, fmt.Errorf("unknown entry %q", normalized.ID)
	}
	if err := labelAvailableLocked(s.state.Entries, normalized); err != nil {
		return Entry{}, err
	}
	previous, existed := s.state.Entries[normalized.ID]
	if existed {
		normalized.Favorite = previous.Favorite
		normalized.Hidden = previous.Hidden
		normalized.LastUsedAt = previous.LastUsedAt
		normalized.ConnectionCount = previous.ConnectionCount
	}
	normalized.UpdatedAt = time.Now().UTC().Unix()
	previousTombs := cloneVaultTombstones(s.state.VaultTombstones)
	s.state.Entries[normalized.ID] = normalized
	if s.state.VaultTombstones.Entries != nil {
		delete(s.state.VaultTombstones.Entries, normalized.ID)
	}
	if err := s.save(); err != nil {
		if existed {
			s.state.Entries[normalized.ID] = previous
		} else {
			delete(s.state.Entries, normalized.ID)
		}
		s.state.VaultTombstones = previousTombs
		return Entry{}, err
	}
	s.hostRevision.Add(1)
	return cloneEntry(normalized), nil
}

func (s *Store) MoveEntry(id, group string, collapsedGroups []string) error {
	group, err := NormalizeGroupPath(group)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.state.Entries[id]
	if !ok {
		return fmt.Errorf("unknown entry %q", id)
	}
	previous := cloneEntry(entry)
	previousPreferences := s.state.Preferences
	entry.Group = group
	entry.UpdatedAt = time.Now().UTC().Unix()
	s.state.Entries[id] = entry
	s.state.Preferences.CollapsedGroups = cleanCollapsedGroups(collapsedGroups)
	if err := s.save(); err != nil {
		s.state.Entries[id] = previous
		s.state.Preferences = previousPreferences
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

func (s *Store) DeleteEntry(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, ok := s.state.Entries[id]
	if !ok {
		return fmt.Errorf("unknown entry %q", id)
	}
	previousTombs := cloneVaultTombstones(s.state.VaultTombstones)
	delete(s.state.Entries, id)
	if s.state.VaultTombstones.Entries == nil {
		s.state.VaultTombstones.Entries = map[string]int64{}
	}
	s.state.VaultTombstones.Entries[id] = time.Now().UTC().Unix()
	if err := s.save(); err != nil {
		s.state.Entries[id] = previous
		s.state.VaultTombstones = previousTombs
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

func (s *Store) ToggleEntryFavorite(id string) (bool, error) {
	return s.toggleEntryFlag(id, true)
}

func (s *Store) ToggleEntryHidden(id string) (bool, error) {
	return s.toggleEntryFlag(id, false)
}

func (s *Store) toggleEntryFlag(id string, favorite bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.state.Entries[id]
	if !ok {
		return false, fmt.Errorf("unknown entry %q", id)
	}
	previous := cloneEntry(entry)
	if favorite {
		entry.Favorite = !entry.Favorite
	} else {
		entry.Hidden = !entry.Hidden
	}
	entry.UpdatedAt = time.Now().UTC().Unix()
	s.state.Entries[id] = entry
	if err := s.saveQuick(); err != nil {
		s.state.Entries[id] = previous
		if favorite {
			return previous.Favorite, err
		}
		return previous.Hidden, err
	}
	s.hostRevision.Add(1)
	if favorite {
		return entry.Favorite, nil
	}
	return entry.Hidden, nil
}

func (s *Store) RecordEntryUse(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.state.Entries[id]
	if !ok {
		return fmt.Errorf("unknown entry %q", id)
	}
	previous := cloneEntry(entry)
	now := time.Now().UTC()
	entry.LastUsedAt = &now
	entry.ConnectionCount++
	entry.UpdatedAt = now.Unix()
	s.state.Entries[id] = entry
	if err := s.saveQuick(); err != nil {
		s.state.Entries[id] = previous
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

// ReplaceEntries is the vault apply path. entries replaces the live set.
// Tombstones are the caller's already-merged set, including entry ids.
func (s *Store) ReplaceEntries(entries []Entry, tombs VaultTombstones) error {
	next := map[string]Entry{}
	for _, entry := range entries {
		normalized, err := NormalizeEntry(entry)
		if err != nil {
			return fmt.Errorf("entry %s: %w", entry.Label, err)
		}
		if normalized.ID == "" {
			return fmt.Errorf("entry %s has no id", entry.Label)
		}
		if _, taken := next[normalized.ID]; taken {
			return fmt.Errorf("entry id %s is duplicated", normalized.ID)
		}
		normalized.Favorite = entry.Favorite
		normalized.Hidden = entry.Hidden
		normalized.LastUsedAt = entry.LastUsedAt
		normalized.ConnectionCount = entry.ConnectionCount
		normalized.UpdatedAt = entry.UpdatedAt
		next[normalized.ID] = normalized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previousEntries := cloneEntryMap(s.state.Entries)
	previousTombs := cloneVaultTombstones(s.state.VaultTombstones)
	s.state.Entries = next
	s.state.VaultTombstones = tombs
	if err := s.save(); err != nil {
		s.state.Entries = previousEntries
		s.state.VaultTombstones = previousTombs
		return err
	}
	s.hostRevision.Add(1)
	return nil
}

func (s *Store) rewriteEntryGroupsLocked(oldPath, newPath string) {
	if oldPath == newPath || s.state.Entries == nil {
		return
	}
	for id, entry := range s.state.Entries {
		existing, err := NormalizeGroupPath(entry.Group)
		if err != nil || existing == "" || (existing != oldPath && !strings.HasPrefix(existing, oldPath+"/")) {
			continue
		}
		entry.Group = replaceGroupPathPrefix(existing, oldPath, newPath)
		s.state.Entries[id] = entry
	}
}

// NormalizeEntry checks an entry and returns the stored form.
// Usage fields are left as they were passed. UpdatedAt is left unchanged.
func NormalizeEntry(entry Entry) (Entry, error) {
	entry.Kind = strings.TrimSpace(entry.Kind)
	entry.Label = strings.TrimSpace(entry.Label)
	if entry.Label == "" {
		return Entry{}, errors.New("label is required")
	}
	if len(entry.Label) > 256 || strings.ContainsAny(entry.Label, "\r\n\x00") {
		return Entry{}, errors.New("that label can't be used")
	}
	group, err := NormalizeGroupPath(entry.Group)
	if err != nil {
		return Entry{}, err
	}
	icon, err := NormalizeIcon(entry.Icon)
	if err != nil {
		return Entry{}, err
	}
	color, err := NormalizeColor(entry.Color)
	if err != nil {
		return Entry{}, err
	}
	via, err := normalizeVia(entry.Via)
	if err != nil {
		return Entry{}, err
	}
	notes := strings.TrimSpace(entry.Notes)
	if len(notes) > 4000 || strings.ContainsAny(notes, "\r\n\x00") {
		return Entry{}, errors.New("that note can't be used")
	}
	environment := strings.TrimSpace(entry.Environment)
	if len(environment) > 64 || strings.ContainsAny(environment, "\r\n\x00") {
		return Entry{}, errors.New("that environment can't be used")
	}
	out := Entry{
		ID:              strings.TrimSpace(entry.ID),
		Kind:            entry.Kind,
		Label:           entry.Label,
		Favorite:        entry.Favorite,
		Hidden:          entry.Hidden,
		LastUsedAt:      entry.LastUsedAt,
		ConnectionCount: entry.ConnectionCount,
		Group:           group,
		Tags:            cleanTags(entry.Tags),
		Environment:     environment,
		Color:           color,
		Icon:            icon,
		Notes:           notes,
		Via:             via,
		UpdatedAt:       entry.UpdatedAt,
	}
	switch entry.Kind {
	case EntryRDP:
		target, err := rdp.DialTarget(entry.Address, entry.Port)
		if err != nil {
			return Entry{}, err
		}
		host, port := splitStoredTarget(target)
		user, domain, err := rdp.SplitSignIn(entry.User, entry.Domain)
		if err != nil {
			return Entry{}, err
		}
		out.Address = host
		out.Port = port
		out.User = user
		out.Domain = domain
		out.Fullscreen = entry.Fullscreen
		out.Admin = entry.Admin
		if out.Port == "3389" {
			out.Port = ""
		}
	case EntryWeb:
		parsed, err := page.ParseURL(entry.URL)
		if err != nil {
			return Entry{}, err
		}
		out.URL = parsed
	default:
		return Entry{}, errors.New("entry kind must be Remote Desktop or a web page")
	}
	return out, nil
}

func normalizeVia(via string) (string, error) {
	via = strings.TrimSpace(via)
	if via == "" {
		return "", nil
	}
	if strings.HasPrefix(via, "-") || strings.ContainsAny(via, " \t\r\n\x00") {
		return "", errors.New("that jump host can't be used")
	}
	return via, nil
}

func splitStoredTarget(target string) (string, string) {
	if strings.HasPrefix(target, "[") {
		end := strings.IndexByte(target, ']')
		if end > 1 {
			host := target[1:end]
			rest := target[end+1:]
			if strings.HasPrefix(rest, ":") {
				return host, rest[1:]
			}
			return host, ""
		}
	}
	host, port, found := strings.Cut(target, ":")
	if !found {
		return target, ""
	}
	return host, port
}

func entryByLabelLocked(entries map[string]Entry, kind, label string) (Entry, error) {
	kind = strings.TrimSpace(kind)
	var found []Entry
	for _, entry := range entries {
		if entry.Kind == kind && sameEntryLabel(entry.Label, label) {
			found = append(found, cloneEntry(entry))
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) == 0 {
		return Entry{}, fmt.Errorf("unknown entry %q", strings.TrimSpace(label))
	}
	return Entry{}, fmt.Errorf("entry label %q is ambiguous", strings.TrimSpace(label))
}

func labelAvailableLocked(entries map[string]Entry, entry Entry) error {
	for id, existing := range entries {
		if id == entry.ID || existing.Kind != entry.Kind {
			continue
		}
		if sameEntryLabel(existing.Label, entry.Label) {
			return duplicateLabelError(entry.Kind, entry.Label)
		}
	}
	return nil
}

func labelsUnique(entries map[string]Entry) error {
	for _, entry := range entries {
		if err := labelAvailableLocked(entries, entry); err != nil {
			return err
		}
	}
	return nil
}

func duplicateLabelError(kind, label string) error {
	if kind == EntryWeb {
		return fmt.Errorf("a web page named %q already exists", label)
	}
	return fmt.Errorf("a Remote Desktop entry named %q already exists", label)
}

func sameEntryLabel(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func newEntryID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func cloneEntry(entry Entry) Entry {
	entry.Tags = append([]string(nil), entry.Tags...)
	if entry.LastUsedAt != nil {
		at := *entry.LastUsedAt
		entry.LastUsedAt = &at
	}
	return entry
}

func cloneEntryMap(in map[string]Entry) map[string]Entry {
	if in == nil {
		return nil
	}
	out := make(map[string]Entry, len(in))
	for id, entry := range in {
		out[id] = cloneEntry(entry)
	}
	return out
}

func cloneEntryList(in map[string]Entry) []Entry {
	out := make([]Entry, 0, len(in))
	for _, entry := range in {
		out = append(out, cloneEntry(entry))
	}
	return out
}
