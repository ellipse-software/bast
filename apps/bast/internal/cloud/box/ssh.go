package box

import (
	"bast/internal/cloud/sshutil"
	"bast/internal/sshconfig"
)

func ToSyncHost(inst Instance, alias string) sshconfig.SyncHostInput {
	if alias == "" {
		alias = AliasFor(inst)
	}
	return sshconfig.SyncHostInput{
		Alias:          alias,
		SyncSource:     ProviderName,
		SyncID:         inst.SyncID,
		HostName:       inst.HostName,
		User:           inst.User,
		IdentityFile:   inst.IdentityFile,
		IdentitiesOnly: inst.IdentitiesOnly,
	}
}

func GroupPath(inst Instance) string {
	// Keep running and stopped boxes in one group so stopping a box does not
	// make it jump (or appear to vanish) into a separate subgroup.
	return "Box"
}

func AliasFor(inst Instance) string {
	name := sshutil.SanitizeAliasPart(inst.Name)
	if name == "" || name == "box" {
		name = sshutil.SanitizeAliasPart(inst.SyncID)
	}
	if name == "" {
		name = "box"
	}
	return "box_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}
