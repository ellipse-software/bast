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

func IAPProxyCommand(inst cloud.Instance) string {
	args := []string{
		"gcloud", "compute", "start-iap-tunnel", proxyLiteral(inst.Name), "%p",
		"--listen-on-stdin",
		"--project=" + proxyLiteral(inst.ProjectID),
		"--zone=" + proxyLiteral(inst.Zone),
		"--verbosity=warning",
	}
	if inst.CredentialAccount != "" {
		args = append(args, "--account="+proxyLiteral(inst.CredentialAccount))
	}
	for i := range args {
		args[i] = shellQuote(args[i])
	}
	command := strings.Join(args, " ")
	if inst.CredentialFile != "" {
		command = "env CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=" + shellQuote(proxyLiteral(inst.CredentialFile)) + " " + command
	}
	return command
}

func GroupPath(inst cloud.Instance) string {
	name := strings.TrimSpace(inst.ProjectName)
	if name == "" {
		name = strings.TrimSpace(inst.ProjectID)
	}
	if name == "" {
		return "Google Cloud"
	}
	name = strings.ReplaceAll(name, "/", "-")
	return "Google Cloud/" + name
}

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

func proxyLiteral(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("-_=./:@%+", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
