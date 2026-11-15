package metadata

import (
	"fmt"
	"sort"
)

// Tunnel is a named forward through an SSH config host. Host, user, port and
// identity always come from OpenSSH config for Host; nothing is duplicated here.
type Tunnel struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Mode      string `json:"mode"`
	Local     string `json:"local,omitempty"`
	Remote    string `json:"remote,omitempty"`
	Group     string `json:"group,omitempty"`
	KeepAlive int    `json:"keepAlive,omitempty"`
}

func (s *Store) Tunnels() []Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tunnel, 0, len(s.state.Tunnels))
	for _, t := range s.state.Tunnels {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) Tunnel(name string) (Tunnel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.state.Tunnels[name]
	return t, ok
}

// PutTunnel stores t. A non-empty replace renames: the old record is removed.
func (s *Store) PutTunnel(t Tunnel, replace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.state.Tunnels[t.Name]; taken && replace != t.Name {
		return fmt.Errorf("tunnel %q already exists", t.Name)
	}
	prev := s.state.Tunnels
	next := make(map[string]Tunnel, len(prev)+1)
	for k, v := range prev {
		if k != replace {
			next[k] = v
		}
	}
	next[t.Name] = t
	s.state.Tunnels = next
	if err := s.save(); err != nil {
		s.state.Tunnels = prev
		return err
	}
	return nil
}

func (s *Store) DeleteTunnel(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Tunnels[name]; !ok {
		return fmt.Errorf("unknown tunnel %q", name)
	}
	prev := s.state.Tunnels
	next := make(map[string]Tunnel, len(prev))
	for k, v := range prev {
		if k != name {
			next[k] = v
		}
	}
	s.state.Tunnels = next
	if err := s.save(); err != nil {
		s.state.Tunnels = prev
		return err
	}
	return nil
}
