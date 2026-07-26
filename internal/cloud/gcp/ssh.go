package gcp

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"bast/internal/cloud"
	"bast/internal/sshconfig"
)

var unsafeAliasChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// ToSyncHost maps a GCP instance to an SSH sync host block.
// inst should already have ResolveAuth applied.
func ToSyncHost(inst cloud.Instance, alias string) sshconfig.SyncHostInput {
	if alias == "" {
		alias = AliasFor(inst)
	}
	input := sshconfig.SyncHostInput{
		Alias:          alias,
		SyncSource:     ProviderName,
		SyncID:         inst.SyncID,
		HostName:       inst.HostName,
		User:           inst.User,
		IdentityFile:   inst.IdentityFile,
		IdentitiesOnly: inst.IdentitiesOnly,
	}
	if inst.UseIAP {
		input.HostName = inst.Name
		if input.HostName == "" {
			input.HostName = inst.HostName
		}
		input.ProxyCommand = IAPProxyCommand(inst)
	}
	return input
}

func publicKeyBlob(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		return ""
	}
	return fields[0] + " " + fields[1]
}

func shortenHomePath(path, home string) string {
	if home == "" {
		return path
	}
	home = strings.TrimRight(home, string(filepath.Separator))
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~/" + filepath.ToSlash(path[len(prefix):])
	}
	return path
}

// IAPProxyCommand builds a gcloud IAP tunnel ProxyCommand.
func IAPProxyCommand(inst cloud.Instance) string {
	return fmt.Sprintf(
		"gcloud compute start-iap-tunnel %s %%p --listen-on-stdin --project=%s --zone=%s --verbosity=warning",
		shellSafe(inst.Name),
		shellSafe(inst.ProjectID),
		shellSafe(inst.Zone),
	)
}

// GroupPath returns the Bast group path for a GCP instance.
func GroupPath(inst cloud.Instance) string {
	name := strings.TrimSpace(inst.ProjectName)
	if name == "" {
		name = strings.TrimSpace(inst.ProjectID)
	}
	if name == "" {
		return "GCP"
	}
	name = strings.ReplaceAll(name, "/", "-")
	return "GCP/" + name
}

// AliasFor returns the preferred SSH alias for an instance.
func AliasFor(inst cloud.Instance) string {
	project := sanitizeAliasPart(inst.ProjectID)
	name := sanitizeAliasPart(inst.Name)
	if project == "" {
		project = "gcp"
	}
	if name == "" {
		name = "instance"
	}
	return "gcp_" + project + "_" + name
}

// UniqueAlias ensures the alias does not collide with existing ones.
func UniqueAlias(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func sanitizeAliasPart(value string) string {
	value = strings.TrimSpace(value)
	value = unsafeAliasChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if len(value) > 48 {
		value = value[:48]
	}
	return value
}

func shellSafe(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.' || r == ':':
			return r
		default:
			return -1
		}
	}, value)
}
