package gcp

import (
	"strings"

	"bast/internal/cloud"
	"bast/internal/cloud/sshutil"
	"bast/internal/platform"
	"bast/internal/sshconfig"
)

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
	if path == home {
		return "~"
	}
	return platform.HomeRelative(path, home)
}

func IAPProxyCommand(inst cloud.Instance) string {
	args := []string{
		"gcloud", "compute", "start-iap-tunnel", sshutil.ProxyLiteral(inst.Name), "%p",
		"--listen-on-stdin",
		"--project=" + sshutil.ProxyLiteral(inst.ProjectID),
		"--zone=" + sshutil.ProxyLiteral(inst.Zone),
		"--verbosity=warning",
	}
	if inst.CredentialAccount != "" {
		args = append(args, "--account="+sshutil.ProxyLiteral(inst.CredentialAccount))
	}
	for i := range args {
		args[i] = sshutil.ShellQuote(args[i])
	}
	command := strings.Join(args, " ")
	if inst.CredentialFile != "" {
		command = sshutil.WithEnvironment(command, "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", sshutil.ProxyLiteral(inst.CredentialFile))
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
	project := sshutil.SanitizeAliasPart(inst.ProjectID)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if project == "" {
		project = "gcp"
	}
	if name == "" {
		name = "instance"
	}
	return "gcp_" + project + "_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}
