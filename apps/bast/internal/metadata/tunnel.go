package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Tunnel is a named forward through an SSH config host. Host, user, port and
// identity always come from OpenSSH config for Host; nothing is duplicated here.
type Tunnel struct {
	Name         string `json:"name"`
	Host         string `json:"host"`
	Mode         string `json:"mode"`
	Local        string `json:"local,omitempty"`
	Remote       string `json:"remote,omitempty"`
	Group        string `json:"group,omitempty"`
	KeepAlive    int    `json:"keepAlive,omitempty"`
	OpenWithHost bool   `json:"openWithHost,omitempty"`
}

func (s *Store) Tunnels() []Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	current := s.tunnelMap()
	out := make([]Tunnel, 0, len(current))
	for _, t := range current {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) Tunnel(name string) (Tunnel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tunnelMap()[name]
	return t, ok
}

// tunnelEdit is applied under the metadata file lock, against the tunnels
// currently on disk, so a stale in-memory snapshot cannot drop another
// process's tunnel.
type tunnelEdit struct {
	put     *Tunnel
	replace string
	del     string
}

// PutTunnel stores t. A non-empty replace renames: the old record is removed.
func (s *Store) PutTunnel(t Tunnel, replace string) error {
	return s.commitTunnels(tunnelEdit{put: &t, replace: replace})
}

func (s *Store) DeleteTunnel(name string) error {
	return s.commitTunnels(tunnelEdit{del: name})
}

func (s *Store) commitTunnels(edit tunnelEdit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.state.Tunnels
	s.pendingTunnel = &edit
	err := s.save()
	s.pendingTunnel = nil
	if err != nil {
		s.state.Tunnels = prev
		return err
	}
	return nil
}

// tunnelMap prefers the file so a long-lived process sees tunnels written by
// another Bast process. The in-memory map is the fallback before the first save.
func (s *Store) tunnelMap() map[string]Tunnel {
	if fresh, err := s.readTunnelMap(); err == nil {
		return fresh
	}
	return s.state.Tunnels
}

func (s *Store) readTunnelMap() (map[string]Tunnel, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Tunnels map[string]Tunnel `json:"tunnels"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return nil, err
	}
	return file.Tunnels, nil
}

// mergeTunnelsFromDisk runs inside the metadata file lock. Tunnel edits reload
// the on-disk map before changing it. Every other save keeps that map, so a
// host or preference write does not erase a tunnel just stored by the CLI.
func (s *Store) mergeTunnelsFromDisk() error {
	disk, err := s.readTunnelMap()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if s.pendingTunnel == nil {
		if err == nil {
			s.state.Tunnels = cloneTunnelMap(disk)
		}
		return nil
	}
	base := s.state.Tunnels
	if err == nil {
		base = disk
	}
	next, err := applyTunnelEdit(base, s.pendingTunnel)
	if err != nil {
		return err
	}
	s.state.Tunnels = next
	return nil
}

func applyTunnelEdit(prev map[string]Tunnel, edit *tunnelEdit) (map[string]Tunnel, error) {
	if edit.del != "" {
		if _, ok := prev[edit.del]; !ok {
			return nil, fmt.Errorf("unknown tunnel %q", edit.del)
		}
		next := make(map[string]Tunnel, len(prev))
		for k, v := range prev {
			if k != edit.del {
				next[k] = v
			}
		}
		return next, nil
	}
	t := *edit.put
	if _, taken := prev[t.Name]; taken && edit.replace != t.Name {
		return nil, fmt.Errorf("tunnel %q already exists", t.Name)
	}
	next := make(map[string]Tunnel, len(prev)+1)
	for k, v := range prev {
		if k != edit.replace {
			next[k] = v
		}
	}
	next[t.Name] = t
	return next, nil
}

func cloneTunnelMap(in map[string]Tunnel) map[string]Tunnel {
	if in == nil {
		return nil
	}
	out := make(map[string]Tunnel, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
