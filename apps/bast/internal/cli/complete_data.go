package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bast/internal/metadata"
)

type completeHost struct {
	Alias      string
	Label      string
	SyncSource string
	SyncID     string
	Hidden     bool
}

func (r *Runner) completeHosts() []completeHost {
	hosts, err := r.config.Discover()
	if err != nil {
		return nil
	}
	store := r.store
	if store == nil {
		opened, openErr := metadata.Open(r.Paths.StateFile)
		if openErr != nil {
			store = nil
		} else {
			store = opened
		}
	}
	out := make([]completeHost, 0, len(hosts))
	for _, host := range hosts {
		item := completeHost{Alias: host.Alias, Label: host.Alias, SyncSource: host.SyncSource, SyncID: host.SyncID}
		if store != nil {
			meta := store.Host(host.Alias)
			item.Hidden = meta.Hidden
			if label := strings.TrimSpace(meta.Label); label != "" {
				item.Label = label
			}
		}
		out = append(out, item)
	}
	return out
}

func (r *Runner) completeKeyNames() []string {
	seen := map[string]bool{}
	var names []string
	add := func(dir string, requirePub bool) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".pub") {
				continue
			}
			switch name {
			case "config", "known_hosts", "known_hosts.old", "authorized_keys", "authorized_keys2", "environment":
				continue
			}
			if requirePub {
				if _, err := os.Stat(filepath.Join(dir, name+".pub")); err != nil {
					continue
				}
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	add(r.Paths.ManagedKeys, false)
	add(r.Paths.SSHDir, true)
	sort.Strings(names)
	return names
}
