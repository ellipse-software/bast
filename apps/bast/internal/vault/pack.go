package vault

import (
	"context"
	"os"
	"reflect"
	"strings"
	"time"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

type Packer struct {
	Paths    paths.Paths
	Config   sshconfig.Manager
	Keyring  keys.Manager
	Store    *metadata.Store
	Previous Document // optional prior document for stable UpdatedAt
}

func (p Packer) Pack() (Document, error) {
	now := time.Now().UTC().Unix()
	hosts, err := p.Config.Discover()
	if err != nil {
		return Document{}, err
	}
	prevHosts := map[string]HostEntry{}
	for _, h := range p.Previous.Hosts {
		prevHosts[h.ManagedID] = h
	}
	prevKeys := map[string]KeyEntry{}
	for _, k := range p.Previous.Keys {
		prevKeys[keyID(k)] = k
	}

	entries := make([]HostEntry, 0)
	metaOut := map[string]metadata.Host{}
	allMeta := map[string]metadata.Host{}
	if p.Store != nil {
		allMeta = p.Store.Hosts()
	}
	for _, host := range hosts {
		if !host.Managed || host.ManagedID == "" {
			continue
		}
		identity := ""
		passwordOnly := isPasswordOnlyResolved(host.Resolved)
		if !passwordOnly && len(host.Resolved.IdentityFiles) > 0 {
			identity = p.Config.HomeRelative(host.Resolved.IdentityFiles[0])
		}
		proxy := host.Resolved.ProxyJump
		if strings.EqualFold(proxy, "none") {
			proxy = ""
		}
		extras, _ := p.Config.ManagedExtras(host.ManagedID)
		entry := HostEntry{
			ManagedID:    host.ManagedID,
			Alias:        host.Alias,
			HostName:     host.Resolved.HostName,
			User:         host.Resolved.User,
			Port:         host.Resolved.Port,
			IdentityFile: identity,
			ExtraOptions: extras,
			PasswordOnly: passwordOnly,
			ProxyJump:    proxy,
			UpdatedAt:    now,
		}
		if prev, ok := prevHosts[host.ManagedID]; ok && hostEntryEqual(prev, entry) {
			entry.UpdatedAt = prev.UpdatedAt
		}
		entries = append(entries, entry)
		if meta, ok := allMeta[host.Alias]; ok {
			metaOut[host.Alias] = meta
		}
	}

	discoveredKeys, err := p.Keyring.Discover(context.Background(), map[string][]string{})
	if err != nil {
		return Document{}, err
	}
	keyEntries := make([]KeyEntry, 0)
	for _, key := range discoveredKeys {
		if !key.Managed || key.PrivatePath == "" {
			continue
		}
		private, err := os.ReadFile(key.PrivatePath)
		if err != nil {
			return Document{}, err
		}
		public := ""
		if key.PublicPath != "" {
			b, readErr := os.ReadFile(key.PublicPath)
			if readErr != nil {
				return Document{}, readErr
			}
			public = string(b)
		}
		entry := KeyEntry{
			Name:          key.Name,
			Fingerprint:   key.Fingerprint,
			PrivatePEM:    string(private),
			PublicOpenSSH: public,
			Comment:       key.Comment,
			Algorithm:     key.Algorithm,
			UpdatedAt:     now,
		}
		if prev, ok := prevKeys[keyID(entry)]; ok && keyEntryEqual(prev, entry) {
			entry.UpdatedAt = prev.UpdatedAt
		}
		keyEntries = append(keyEntries, entry)
	}

	doc := Document{
		Version:   DocumentVersion,
		UpdatedAt: now,
		Hosts:     entries,
		Keys:      keyEntries,
		Metadata:  metaOut,
	}
	if p.Store != nil {
		doc.Preferences = p.Store.Preferences()
		doc.Integrations = packIntegrations(p.Store.Integrations())
	}
	return doc, nil
}

func hostEntryEqual(a, b HostEntry) bool {
	a.UpdatedAt, b.UpdatedAt = 0, 0
	a.DeletedAt, b.DeletedAt = 0, 0
	return reflect.DeepEqual(a, b)
}

func keyEntryEqual(a, b KeyEntry) bool {
	a.UpdatedAt, b.UpdatedAt = 0, 0
	a.DeletedAt, b.DeletedAt = 0, 0
	return reflect.DeepEqual(a, b)
}

func isPasswordOnlyResolved(resolved sshconfig.Resolved) bool {
	return strings.EqualFold(resolved.PubkeyAuthentication, "no") &&
		(strings.EqualFold(resolved.PasswordAuthentication, "yes") ||
			strings.Contains(strings.ToLower(resolved.PreferredAuthentications), "password"))
}

func packIntegrations(in metadata.Integrations) VaultIntegrations {
	out := VaultIntegrations{}
	if in.GCP != nil {
		out.GCP = &VaultGCPIntegration{
			Enabled:        in.GCP.Enabled,
			ProjectFilter:  append([]string(nil), in.GCP.ProjectFilter...),
			DefaultSSHUser: in.GCP.DefaultSSHUser,
			AutoSync:       in.GCP.AutoSync,
		}
	}
	if in.AWS != nil {
		out.AWS = &VaultAWSIntegration{
			Enabled:        in.AWS.Enabled,
			ProfileFilter:  append([]string(nil), in.AWS.ProfileFilter...),
			RegionFilter:   append([]string(nil), in.AWS.RegionFilter...),
			DefaultSSHUser: in.AWS.DefaultSSHUser,
			AutoSync:       in.AWS.AutoSync,
		}
	}
	if in.Azure != nil {
		out.Azure = &VaultAzureIntegration{
			Enabled:             in.Azure.Enabled,
			SubscriptionFilter:  append([]string(nil), in.Azure.SubscriptionFilter...),
			ResourceGroupFilter: append([]string(nil), in.Azure.ResourceGroupFilter...),
			DefaultSSHUser:      in.Azure.DefaultSSHUser,
			AutoSync:            in.Azure.AutoSync,
		}
	}
	if in.Upstash != nil {
		out.Upstash = &VaultUpstashIntegration{
			Enabled:  in.Upstash.Enabled,
			AutoSync: in.Upstash.AutoSync,
		}
	}
	if in.Vercel != nil {
		out.Vercel = &VaultVercelIntegration{
			Enabled:  in.Vercel.Enabled,
			AutoSync: in.Vercel.AutoSync,
		}
	}
	if in.Hetzner != nil {
		out.Hetzner = &VaultHetznerIntegration{
			Enabled:         in.Hetzner.Enabled,
			AutoSync:        in.Hetzner.AutoSync,
			DefaultSSHUser:  in.Hetzner.DefaultSSHUser,
			DefaultSSHPort:  in.Hetzner.DefaultSSHPort,
			PreferPrivateIP: in.Hetzner.PreferPrivateIP,
			ContextFilter:   append([]string(nil), in.Hetzner.ContextFilter...),
			LocationFilter:  append([]string(nil), in.Hetzner.LocationFilter...),
		}
	}
	return out
}
