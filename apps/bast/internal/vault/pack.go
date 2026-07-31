package vault

import (
	"context"
	"os"
	"strings"
	"time"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

type Packer struct {
	Paths   paths.Paths
	Config  sshconfig.Manager
	Keyring keys.Manager
	Store   *metadata.Store
}

func (p Packer) Pack() (Document, error) {
	now := time.Now().UTC().Unix()
	hosts, err := p.Config.Discover()
	if err != nil {
		return Document{}, err
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
		entries = append(entries, HostEntry{
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
		})
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
		keyEntries = append(keyEntries, KeyEntry{
			Name:          key.Name,
			Fingerprint:   key.Fingerprint,
			PrivatePEM:    string(private),
			PublicOpenSSH: public,
			Comment:       key.Comment,
			Algorithm:     key.Algorithm,
			UpdatedAt:     now,
		})
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
	return out
}
