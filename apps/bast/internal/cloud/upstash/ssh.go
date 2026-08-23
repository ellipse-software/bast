package upstash

import (
	"os/exec"
	"strings"

	"bast/internal/cloud/sshutil"
	"bast/internal/sshconfig"
)

func ToSyncHost(inst Instance, alias string) sshconfig.SyncHostInput {
	if alias == "" {
		alias = AliasFor(inst)
	}
	return sshconfig.SyncHostInput{
		Alias:        alias,
		SyncSource:   ProviderName,
		SyncID:       inst.SyncID,
		HostName:     inst.HostName,
		User:         inst.User,
		PasswordOnly: true,
		ExtraOptions: []string{"StrictHostKeyChecking accept-new"},
	}
}

func GroupPath(_ Instance) string {
	return "Upstash"
}

func AliasFor(inst Instance) string {
	name := sshutil.SanitizeAliasPart(inst.Name)
	if name == "" || strings.EqualFold(name, "upstash") {
		name = sshutil.SanitizeAliasPart(inst.SyncID)
	}
	if name == "" {
		name = "box"
	}
	return "upstash_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

func PrepareSSH(cmd *exec.Cmd, bastExecutable string) {
	ApplyAskPass(cmd, bastExecutable)
}
