package aws

import (
	"os"
	"path/filepath"
	"strings"

	"bast/internal/cloud/sshutil"
	"bast/internal/platform"
	"bast/internal/sshconfig"
)

func ToSyncHost(inst Instance, alias string) sshconfig.SyncHostInput {
	if alias == "" {
		alias = AliasFor(inst)
	}
	input := sshconfig.SyncHostInput{
		Alias: alias, SyncSource: ProviderName, SyncID: inst.SyncID,
		HostName: inst.HostName, User: inst.User, IdentityFile: inst.IdentityFile,
		IdentitiesOnly: inst.IdentitiesOnly,
	}
	if inst.UseEICE {
		input.ProxyCommand = EICEProxyCommand(inst)
	}
	return input
}

func EICEProxyCommand(inst Instance) string {
	args := []string{"aws", "ec2-instance-connect", "open-tunnel",
		"--instance-id=" + sshutil.ProxyLiteral(inst.HostName),
		"--instance-connect-endpoint-id=" + sshutil.ProxyLiteral(inst.EndpointID),
		"--remote-port=%p",
		"--profile=" + sshutil.ProxyLiteral(inst.Profile),
		"--region=" + sshutil.ProxyLiteral(inst.Region),
		"--no-cli-pager"}
	for i := range args {
		args[i] = sshutil.ShellQuote(args[i])
	}
	return strings.Join(args, " ")
}

func GroupPath(inst Instance) string {
	profile := strings.ReplaceAll(strings.TrimSpace(inst.Profile), "/", "-")
	region := strings.ReplaceAll(strings.TrimSpace(inst.Region), "/", "-")
	if profile == "" {
		return "Amazon EC2"
	}
	if region == "" {
		return "Amazon EC2/" + profile
	}
	return "Amazon EC2/" + profile + "/" + region
}

func AliasFor(inst Instance) string {
	profile := sshutil.SanitizeAliasPart(inst.Profile)
	region := sshutil.SanitizeAliasPart(inst.Region)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if profile == "" {
		profile = "default"
	}
	if region == "" {
		region = "region"
	}
	if name == "" {
		name = "instance"
	}
	return "aws_" + profile + "_" + region + "_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

func findLaunchKey(home, managedKeys, keyName string) string {
	keyName = strings.TrimSpace(keyName)
	if keyName == "" || filepath.Base(keyName) != keyName {
		return ""
	}
	for _, dir := range []string{managedKeys, filepath.Join(home, ".ssh")} {
		if dir == "" {
			continue
		}
		for _, name := range []string{keyName, keyName + ".pem", keyName + ".key"} {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && looksPrivateKey(path) {
				return shortenHomePath(path, home)
			}
		}
	}
	return ""
}

func looksPrivateKey(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	buf := make([]byte, 256)
	n, _ := file.Read(buf)
	return strings.Contains(string(buf[:n]), "PRIVATE KEY-----")
}

func shortenHomePath(path, home string) string {
	return platform.HomeRelative(path, home)
}
