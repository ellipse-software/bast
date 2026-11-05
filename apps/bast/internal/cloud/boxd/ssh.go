package boxd

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
	user := strings.TrimSpace(inst.User)
	if user == "" {
		user = SSHUser
	}
	input := sshconfig.SyncHostInput{
		Alias:          alias,
		SyncSource:     ProviderName,
		SyncID:         inst.SyncID,
		HostName:       inst.HostName,
		User:           user,
		Port:           inst.Port,
		IdentityFile:   inst.IdentityFile,
		IdentitiesOnly: inst.IdentitiesOnly,
		ExtraOptions:   []string{"ServerAliveInterval 30"},
	}
	if inst.DeviceID != "" && !strings.ContainsAny(inst.DeviceID, " \t\r\n\"") {
		input.ExtraOptions = append(input.ExtraOptions, "SetEnv BOXD_DEVICE_ID="+inst.DeviceID)
	}
	return input
}

func GroupPath(inst Instance) string {
	org := safeGroupPart(inst.OrgName)
	if org == "" {
		org = safeGroupPart(inst.OrgID)
	}
	region := safeGroupPart(inst.Region)
	if org == "" {
		return GroupRoot
	}
	if region == "" {
		return GroupRoot + "/" + org
	}
	return GroupRoot + "/" + org + "/" + region
}

// AliasFor names a computer boxd_<region>_<name>. The boxd CLI names it
// <name>.boxd in boxd's home region and <name>.<region>.boxd in the others;
// Bast always names the region, since it syncs every region.
func AliasFor(inst Instance) string {
	region := sshutil.SanitizeAliasPart(inst.Region)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if region == "" {
		region = "region"
	}
	if name == "" {
		name = "computer"
	}
	return "boxd_" + region + "_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

// FormatSyncID keys a computer by org id, region, and name: the three values
// every boxd CLI call about one computer needs.
func FormatSyncID(orgID, region, name string) string {
	return strings.TrimSpace(orgID) + "/" + strings.TrimSpace(region) + "/" + strings.TrimSpace(name)
}

func ParseSyncID(syncID string) (orgID, region, name string, err error) {
	parts := strings.Split(strings.TrimSpace(syncID), "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid boxd sync id %q", syncID)
	}
	for i, label := range []string{"org", "region", "computer"} {
		if err := validateValue(label, parts[i]); err != nil {
			return "", "", "", fmt.Errorf("invalid boxd sync id %q", syncID)
		}
	}
	return parts[0], parts[1], parts[2], nil
}

func safeGroupPart(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "/", "-")
}
