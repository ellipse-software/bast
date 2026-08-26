package vercel

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
		Alias:      alias,
		SyncSource: ProviderName,
		SyncID:     inst.SyncID,
		HostName:   StoppedHost,
	}
}

func GroupPath(_ Instance) string {
	return "Vercel"
}

func AliasFor(inst Instance) string {
	name := sshutil.SanitizeAliasPart(inst.Name)
	if name == "" || strings.EqualFold(name, "vercel") {
		name = sshutil.SanitizeAliasPart(inst.SyncID)
	}
	if name == "" {
		name = "sandbox"
	}
	return "vercel_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

func ShellCommand(bastExecutable, name, projectID, teamID string) *exec.Cmd {
	exe := strings.TrimSpace(bastExecutable)
	if exe == "" {
		exe = "bast"
	}
	args := []string{"__vercel-sandbox-shell", "--name", name, "--project", projectID}
	if strings.TrimSpace(teamID) != "" {
		args = append(args, "--team", teamID)
	}
	return exec.Command(exe, args...)
}
