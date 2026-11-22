package tunnel

import (
	"fmt"
	"strings"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

// ImportResult is a one-time move of managed SSH-config forwards into tunnels.
type ImportResult struct {
	Migrated []string
	Created  int
	Updated  int
	Closed   []string
	Problems []string
}

// Notice is empty when nothing changed. It is safe to print once.
func (r ImportResult) Notice() string {
	var parts []string
	n := r.Created + r.Updated
	switch {
	case n == 1:
		parts = append(parts, "Moved 1 forward to a tunnel")
	case n > 1:
		parts = append(parts, fmt.Sprintf("Moved %d forwards to tunnels", n))
	}
	if n > 0 {
		parts = append(parts, "A shell that is already connected keeps those ports until it exits")
	}
	switch len(r.Closed) {
	case 0:
	case 1:
		parts = append(parts, "Stopped the old background forward for "+r.Closed[0])
	default:
		parts = append(parts, "Stopped the old background forwards for "+strings.Join(r.Closed, ", "))
	}
	parts = append(parts, r.Problems...)
	return strings.Join(parts, ". ")
}

// ImportConfigForwards copies LocalForward, RemoteForward, and DynamicForward
// off Bast-managed hosts into named tunnels with OpenWithHost set, then removes
// those directives. A host with a forward that does not validate is left as it is.
// Unmanaged and synced blocks are not edited. Tunnels are not started.
func ImportConfigForwards(mgr sshconfig.Manager, store *metadata.Store) (ImportResult, error) {
	var result ImportResult
	if store == nil {
		return result, nil
	}
	hosts, err := mgr.Discover()
	if err != nil {
		return result, err
	}
	for _, host := range hosts {
		if !host.Managed || host.ManagedID == "" {
			continue
		}
		input, err := mgr.ManagedInput(host.ManagedID)
		if err != nil {
			result.Problems = append(result.Problems, host.Alias+": "+err.Error())
			continue
		}
		forwards, rest, bad := sshconfig.TakeManagedForwards(input.ExtraOptions)
		if len(bad) > 0 {
			result.Problems = append(result.Problems, host.Alias+": left SSH config forwards in place ("+strings.Join(bad, "; ")+")")
			continue
		}
		if len(forwards) == 0 {
			continue
		}
		built := make([]metadata.Tunnel, 0, len(forwards))
		var failed []string
		for _, forward := range forwards {
			t, err := tunnelFromManagedForward(host.Alias, forward)
			if err != nil {
				failed = append(failed, err.Error())
				continue
			}
			built = append(built, t)
		}
		if len(failed) > 0 {
			result.Problems = append(result.Problems, host.Alias+": left SSH config forwards in place ("+strings.Join(failed, "; ")+")")
			continue
		}
		created, updated, err := storeImportedTunnels(store, built)
		if err != nil {
			result.Problems = append(result.Problems, host.Alias+": "+err.Error())
			continue
		}
		input.ExtraOptions = rest
		if err := mgr.Update(host.ManagedID, input); err != nil {
			result.Problems = append(result.Problems, host.Alias+": "+err.Error())
			continue
		}
		result.Created += created
		result.Updated += updated
		result.Migrated = append(result.Migrated, host.Alias)
	}
	return result, nil
}

func storeImportedTunnels(store *metadata.Store, built []metadata.Tunnel) (created, updated int, err error) {
	for _, t := range built {
		current := store.Tunnels()
		if existing, ok := equivalentTunnel(current, t); ok {
			if existing.OpenWithHost {
				continue
			}
			existing.OpenWithHost = true
			if err = store.PutTunnel(existing, existing.Name); err != nil {
				return created, updated, err
			}
			updated++
			continue
		}
		t.Name = tunnelImportName(t, current)
		if err = Validate(specFrom(t)); err != nil {
			return created, updated, err
		}
		if err = store.PutTunnel(t, ""); err != nil {
			return created, updated, err
		}
		created++
	}
	return created, updated, nil
}

func tunnelFromManagedForward(alias string, forward sshconfig.ManagedForward) (metadata.Tunnel, error) {
	t := metadata.Tunnel{Name: "pending", Host: alias, OpenWithHost: true}
	switch forward.Kind {
	case "local":
		t.Mode = ModeLocal
		t.Local = forward.Listen
		t.Remote = forward.Target
	case "remote":
		t.Mode = ModeRemote
		t.Local = forward.Target
		t.Remote = forward.Listen
	case "dynamic":
		t.Mode = ModeSOCKS
		t.Local = forward.Listen
	default:
		return metadata.Tunnel{}, fmt.Errorf("unknown forward %q", forward.Kind)
	}
	if err := Validate(specFrom(t)); err != nil {
		return metadata.Tunnel{}, err
	}
	return t, nil
}

func equivalentTunnel(list []metadata.Tunnel, want metadata.Tunnel) (metadata.Tunnel, bool) {
	for _, t := range list {
		if t.Host == want.Host && t.Mode == want.Mode && t.Local == want.Local && t.Remote == want.Remote {
			return t, true
		}
	}
	return metadata.Tunnel{}, false
}

func tunnelImportName(t metadata.Tunnel, existing []metadata.Tunnel) string {
	listen := t.Local
	if t.Mode == ModeRemote {
		listen = t.Remote
	}
	used := map[string]bool{}
	for _, item := range existing {
		used[item.Name] = true
	}
	base := sanitizeTunnelName(t.Host + "-" + listen)
	if !used[base] {
		return base
	}
	withMode := sanitizeTunnelName(t.Host + "-" + t.Mode + "-" + listen)
	if !used[withMode] {
		return withMode
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", withMode, n)
		if !used[candidate] {
			return candidate
		}
	}
}

func sanitizeTunnelName(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "tunnel"
	}
	return out
}

func specFrom(t metadata.Tunnel) Spec {
	return Spec{Name: t.Name, Host: t.Host, Mode: t.Mode, Local: t.Local, Remote: t.Remote, Group: t.Group, KeepAlive: t.KeepAlive}
}
