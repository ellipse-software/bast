package fly

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"bast/internal/cloud/sshutil"
	"bast/internal/sshconfig"
)

func ToSyncHost(inst Instance, alias, bastExecutable string) sshconfig.SyncHostInput {
	if alias == "" {
		alias = AliasFor(inst)
	}
	input := sshconfig.SyncHostInput{
		Alias:          alias,
		SyncSource:     ProviderName,
		SyncID:         inst.SyncID,
		HostName:       inst.HostName,
		User:           inst.User,
		IdentitiesOnly: true,
		ExtraOptions:   []string{"StrictHostKeyChecking accept-new"},
	}
	if org, app, machine, err := ParseSyncID(inst.SyncID); err == nil {
		input.ProxyCommand = ProxyCommand(ProxyOptions{
			Org: org, App: app, Machine: machine, ResourcePort: 22,
		}, bastExecutable)
	}
	return input
}

func GroupPath(inst Instance) string {
	org := safeGroupPart(inst.OrgName)
	if org == "" {
		org = safeGroupPart(inst.OrgSlug)
	}
	app := safeGroupPart(inst.App)
	if org == "" {
		return GroupRoot
	}
	if app == "" {
		return GroupRoot + "/" + org
	}
	return GroupRoot + "/" + org + "/" + app
}

func AliasFor(inst Instance) string {
	org := sshutil.SanitizeAliasPart(inst.OrgSlug)
	app := sshutil.SanitizeAliasPart(inst.App)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if org == "" {
		org = "org"
	}
	if app == "" {
		app = "app"
	}
	if name == "" || strings.EqualFold(name, app) {
		name = sshutil.SanitizeAliasPart(machineIDFromSync(inst.SyncID))
	}
	if name == "" {
		name = "machine"
	}
	return "fly_" + org + "_" + app + "_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

func FormatSyncID(org, app, machine string) string {
	return strings.TrimSpace(org) + "/" + strings.TrimSpace(app) + "/" + strings.TrimSpace(machine)
}

func ParseSyncID(syncID string) (org, app, machine string, err error) {
	syncID = strings.TrimSpace(syncID)
	parts := strings.Split(syncID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid Fly sync id %q", syncID)
	}
	for _, part := range parts {
		if strings.ContainsAny(part, "\\ \t\r\n") {
			return "", "", "", fmt.Errorf("invalid Fly sync id %q", syncID)
		}
		for _, r := range part {
			if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.') {
				return "", "", "", fmt.Errorf("invalid Fly sync id %q", syncID)
			}
		}
	}
	return parts[0], parts[1], parts[2], nil
}

func machineIDFromSync(syncID string) string {
	_, _, machine, err := ParseSyncID(syncID)
	if err != nil {
		return ""
	}
	return machine
}

func IdentityPath(managedKeys, org string) string {
	org = sshutil.SanitizeAliasPart(org)
	if org == "" {
		org = "org"
	}
	return filepath.Join(managedKeys, "fly_"+org)
}

func CertificatePath(identityFile string) string {
	return identityFile + "-cert.pub"
}

func InternalHost(app, machine string) string {
	return machine + ".vm." + app + ".internal"
}

func safeGroupPart(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "/", "-")
}
