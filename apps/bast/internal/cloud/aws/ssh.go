package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"bast/internal/sshconfig"
)

var unsafeAliasChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

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
		"--instance-id=" + proxyLiteral(inst.HostName),
		"--instance-connect-endpoint-id=" + proxyLiteral(inst.EndpointID),
		"--remote-port=%p",
		"--profile=" + proxyLiteral(inst.Profile),
		"--region=" + proxyLiteral(inst.Region),
		"--no-cli-pager"}
	for i := range args {
		args[i] = shellQuote(args[i])
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
	profile := sanitizeAliasPart(inst.Profile)
	region := sanitizeAliasPart(inst.Region)
	name := sanitizeAliasPart(inst.Name)
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
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func sanitizeAliasPart(value string) string {
	value = unsafeAliasChars.ReplaceAllString(strings.TrimSpace(value), "_")
	value = strings.Trim(value, "_")
	if len(value) > 48 {
		value = value[:48]
	}
	return value
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
	if home != "" {
		prefix := strings.TrimRight(home, string(filepath.Separator)) + string(filepath.Separator)
		if strings.HasPrefix(path, prefix) {
			return "~/" + filepath.ToSlash(strings.TrimPrefix(path, prefix))
		}
	}
	return path
}

func proxyLiteral(value string) string { return strings.ReplaceAll(value, "%", "%%") }

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-_=./:@%+", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
