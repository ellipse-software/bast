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

func GroupPath(inst Instance) string {
	if !inst.SplitByProject {
		return "Vercel"
	}
	project := strings.ReplaceAll(strings.TrimSpace(inst.ProjectID), "/", "-")
	if project == "" {
		return "Vercel"
	}
	return "Vercel/" + project
}

func AliasFor(inst Instance) string {
	name := sshutil.SanitizeAliasPart(inst.Name)
	if name == "" || strings.EqualFold(name, "vercel") {
		name = sshutil.SanitizeAliasPart(inst.SyncID)
	}
	if name == "" {
		name = "sandbox"
	}
	if inst.SplitByProject {
		if project := sshutil.SanitizeAliasPart(inst.ProjectID); project != "" && !strings.EqualFold(project, name) {
			return "vercel_" + project + "_" + name
		}
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
