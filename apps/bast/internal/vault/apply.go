package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

type Applier struct {
	Paths  paths.Paths
	Config sshconfig.Manager
	Store  *metadata.Store
}

// Apply writes a merged document to managed config, keys, and metadata.
func (a Applier) Apply(doc Document) error {
	if err := a.Config.EnsureManaged(); err != nil {
		return err
	}
	if err := os.MkdirAll(a.Paths.ManagedKeys, 0700); err != nil {
		return err
	}

	// Rewrite managed keys first so IdentityFile paths resolve.
	existing, _ := filepath.Glob(filepath.Join(a.Paths.ManagedKeys, "*"))
	keep := map[string]bool{}
	for _, key := range doc.Keys {
		if key.DeletedAt != 0 || key.PrivatePEM == "" {
			continue
		}
		if err := validateKeyName(key.Name); err != nil {
			return err
		}
		dest := filepath.Join(a.Paths.ManagedKeys, key.Name)
		if err := os.WriteFile(dest, []byte(key.PrivatePEM), 0600); err != nil {
			return err
		}
		keep[dest] = true
		if key.PublicOpenSSH != "" {
			pub := dest + ".pub"
			if err := os.WriteFile(pub, []byte(ensureNewline(key.PublicOpenSSH)), 0644); err != nil {
				return err
			}
			keep[pub] = true
		}
	}
	for _, path := range existing {
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			continue
		}
		if !keep[path] {
			_ = os.Remove(path)
		}
	}

	// Rebuild managed host config from document hosts.
	var body strings.Builder
	for _, host := range doc.Hosts {
		if host.DeletedAt != 0 {
			continue
		}
		identity := host.IdentityFile
		if identity != "" && !strings.HasPrefix(identity, "~/") && !filepath.IsAbs(identity) {
			identity = "~/.ssh/bast/keys/" + filepath.Base(identity)
		}
		input := sshconfig.HostInput{
			Alias:        host.Alias,
			HostName:     host.HostName,
			User:         host.User,
			Port:         host.Port,
			IdentityFile: identity,
			ExtraOptions: host.ExtraOptions,
			PasswordOnly: host.PasswordOnly,
			ProxyJump:    host.ProxyJump,
		}
		if err := sshconfig.Validate(input); err != nil {
			return fmt.Errorf("host %s: %w", host.Alias, err)
		}
		body.Write(sshconfig.RenderManagedBlock(host.ManagedID, input))
	}
	if err := sshconfig.WriteManagedConfig(a.Paths.ManagedConfig, []byte(body.String())); err != nil {
		return err
	}

	if a.Store == nil {
		return nil
	}
	if err := a.Store.UpdateHosts(func(hosts map[string]metadata.Host) {
		for alias := range hosts {
			delete(hosts, alias)
		}
		for alias, meta := range doc.Metadata {
			hosts[alias] = meta
		}
	}); err != nil {
		return err
	}
	if doc.Preferences.Sort != "" {
		if err := a.Store.SetSort(doc.Preferences.Sort); err != nil {
			return err
		}
	}
	if err := a.Store.SetCollapsedGroups(doc.Preferences.CollapsedGroups); err != nil {
		return err
	}
	return applyIntegrations(a.Store, doc.Integrations)
}

func applyIntegrations(store *metadata.Store, in VaultIntegrations) error {
	if in.GCP != nil {
		cur := store.GCP()
		cur.Enabled = in.GCP.Enabled
		cur.ProjectFilter = append([]string(nil), in.GCP.ProjectFilter...)
		cur.DefaultSSHUser = in.GCP.DefaultSSHUser
		cur.AutoSync = in.GCP.AutoSync
		if err := store.SetGCP(cur); err != nil {
			return err
		}
	}
	if in.AWS != nil {
		cur := store.AWS()
		cur.Enabled = in.AWS.Enabled
		cur.ProfileFilter = append([]string(nil), in.AWS.ProfileFilter...)
		cur.RegionFilter = append([]string(nil), in.AWS.RegionFilter...)
		cur.DefaultSSHUser = in.AWS.DefaultSSHUser
		cur.AutoSync = in.AWS.AutoSync
		if err := store.SetAWS(cur); err != nil {
			return err
		}
	}
	if in.Azure != nil {
		cur := store.Azure()
		cur.Enabled = in.Azure.Enabled
		cur.SubscriptionFilter = append([]string(nil), in.Azure.SubscriptionFilter...)
		cur.ResourceGroupFilter = append([]string(nil), in.Azure.ResourceGroupFilter...)
		cur.DefaultSSHUser = in.Azure.DefaultSSHUser
		cur.AutoSync = in.Azure.AutoSync
		if err := store.SetAzure(cur); err != nil {
			return err
		}
	}
	return nil
}

func validateKeyName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\x00\r\n/\\") {
		return fmt.Errorf("invalid key name %q", name)
	}
	return nil
}

func ensureNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
