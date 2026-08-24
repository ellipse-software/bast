package railway

import (
	"strings"

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
		IdentitiesOnly: inst.IdentityFile != "",
		ExtraOptions: []string{
			"ServerAliveInterval 30",
			"ServerAliveCountMax 3",
			"StrictHostKeyChecking accept-new",
		},
	}
}

func GroupPath(inst Instance) string {
	project := strings.TrimSpace(inst.ProjectName)
	if project == "" {
		project = inst.ProjectID
	}
	env := strings.TrimSpace(inst.EnvironmentName)
	if env == "" {
		env = inst.EnvironmentID
	}
	if project == "" {
		return "Railway"
	}
	if env == "" {
		return "Railway / " + project
	}
	return "Railway / " + project + " / " + env
}

func AliasFor(inst Instance) string {
	project := sshutil.SanitizeAliasPart(inst.ProjectName)
	env := sshutil.SanitizeAliasPart(inst.EnvironmentName)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if name == "" {
		name = sshutil.SanitizeAliasPart(inst.ServiceID)
	}
	if name == "" {
		name = "service"
	}
	parts := []string{"railway"}
	if project != "" && !strings.EqualFold(project, "railway") {
		parts = append(parts, project)
	}
	if env != "" && !strings.EqualFold(env, "production") {
		parts = append(parts, env)
	}
	parts = append(parts, name)
	return strings.Join(parts, "_")
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}
