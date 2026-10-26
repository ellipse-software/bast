package digitalocean

import (
	"fmt"
	"strings"

	"bast/internal/cloud/sshutil"
	"bast/internal/sshconfig"
)

func ToSyncHost(inst Instance, alias string) sshconfig.SyncHostInput {
	if alias == "" {
		alias = AliasFor(inst)
	}
	return sshconfig.SyncHostInput{
		Alias: alias, SyncSource: ProviderName, SyncID: inst.SyncID,
		HostName: inst.HostName, User: inst.User, IdentityFile: inst.IdentityFile,
		IdentitiesOnly: inst.IdentitiesOnly,
	}
}

func GroupPath(inst Instance) string {
	contextName := strings.ReplaceAll(strings.TrimSpace(inst.Context), "/", "-")
	region := strings.ReplaceAll(strings.TrimSpace(inst.Region), "/", "-")
	if contextName == "" {
		return "DigitalOcean"
	}
	if region == "" {
		return "DigitalOcean/" + contextName
	}
	return "DigitalOcean/" + contextName + "/" + region
}

func AliasFor(inst Instance) string {
	contextName := sshutil.SanitizeAliasPart(inst.Context)
	region := sshutil.SanitizeAliasPart(inst.Region)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if contextName == "" {
		contextName = "default"
	}
	if region == "" {
		region = "region"
	}
	if name == "" {
		name = "droplet"
	}
	return "do_" + contextName + "_" + region + "_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

func ParseSyncID(syncID string) (uuid, dropletID string, err error) {
	syncID = strings.TrimSpace(syncID)
	rest, ok := strings.CutPrefix(syncID, "do:")
	if !ok {
		return "", "", fmt.Errorf("invalid DigitalOcean sync id %q", syncID)
	}
	uuid, dropletID, ok = strings.Cut(rest, ":")
	if !ok || uuid == "" || dropletID == "" || strings.Contains(dropletID, ":") {
		return "", "", fmt.Errorf("invalid DigitalOcean sync id %q", syncID)
	}
	return uuid, dropletID, nil
}
