package hetzner

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
	return sshconfig.SyncHostInput{
		Alias:          alias,
		SyncSource:     ProviderName,
		SyncID:         inst.SyncID,
		HostName:       inst.HostName,
		Port:           inst.Port,
		User:           inst.User,
		IdentityFile:   inst.IdentityFile,
		IdentitiesOnly: inst.IdentitiesOnly,
	}
}

func GroupPath(inst Instance) string {
	contextName := strings.ReplaceAll(strings.TrimSpace(inst.Context), "/", "-")
	location := strings.ReplaceAll(strings.TrimSpace(inst.Location), "/", "-")
	if contextName == "" {
		return "Hetzner Cloud"
	}
	if location == "" {
		return "Hetzner Cloud/" + contextName
	}
	return "Hetzner Cloud/" + contextName + "/" + location
}

func AliasFor(inst Instance) string {
	contextName := sshutil.SanitizeAliasPart(inst.Context)
	location := sshutil.SanitizeAliasPart(inst.Location)
	name := sshutil.SanitizeAliasPart(inst.Name)
	if contextName == "" {
		contextName = "default"
	}
	if name == "" {
		name = "server"
	}
	if location == "" {
		return "hetzner_" + contextName + "_" + name
	}
	return "hetzner_" + contextName + "_" + location + "_" + name
}

func UniqueAlias(base string, used map[string]bool) string {
	return sshutil.UniqueAlias(base, used)
}

func matchLocalIdentity(publicKeys []string, home, managedKeys string) string {
	if len(publicKeys) == 0 {
		return ""
	}
	wanted := map[string]bool{}
	for _, key := range publicKeys {
		if blob := publicKeyBlob(key); blob != "" {
			wanted[blob] = true
		}
	}
	if len(wanted) == 0 {
		return ""
	}
	dirs := []string{}
	if managedKeys != "" {
		dirs = append(dirs, managedKeys)
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".ssh"))
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pub") {
				continue
			}
			pubPath := filepath.Join(dir, entry.Name())
			b, err := os.ReadFile(pubPath)
			if err != nil {
				continue
			}
			blob := publicKeyBlob(string(b))
			if blob == "" || !wanted[blob] {
				continue
			}
			privatePath := strings.TrimSuffix(pubPath, ".pub")
			if _, err := os.Stat(privatePath); err != nil {
				continue
			}
			return platform.HomeRelative(privatePath, home)
		}
	}
	return ""
}

func publicKeyBlob(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 2 {
		return ""
	}
	return fields[0] + " " + fields[1]
}
