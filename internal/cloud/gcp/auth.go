package gcp

import (
	"os"
	"path/filepath"
	"strings"

	"bast/internal/cloud"
)

const gcloudIdentityFile = "~/.ssh/google_compute_engine"

// ResolveAuth finalizes User / IdentityFile for an instance using local keys.
// Call after discovery, once Home and managed key paths are known.
func ResolveAuth(inst *cloud.Instance, home, managedKeys, osLoginUser string) {
	if inst == nil {
		return
	}
	settingsUser := strings.TrimSpace(inst.User)

	if user, identity := matchSSHKeyEntry(inst.SSHKeys, home, managedKeys); identity != "" {
		if user == "" {
			user = settingsUser
		}
		inst.User = user
		inst.IdentityFile = identity
		inst.IdentitiesOnly = true
		return
	}

	user := settingsUser
	if user == "" {
		user = osLoginUser
	}
	if user == "" {
		user = imageSSHUser(inst.Image)
	}
	inst.User = user
	inst.IdentityFile = ""
	inst.IdentitiesOnly = false
	if home != "" {
		if _, err := os.Stat(filepath.Join(home, ".ssh", "google_compute_engine")); err == nil {
			inst.IdentityFile = gcloudIdentityFile
		}
	}
}

func matchSSHKeyEntry(keys []cloud.SSHKey, home, managedKeys string) (user, identity string) {
	if len(keys) == 0 {
		return "", ""
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
			if blob == "" {
				continue
			}
			for _, key := range keys {
				if publicKeyBlob(key.PublicKey) != blob {
					continue
				}
				privatePath := strings.TrimSuffix(pubPath, ".pub")
				if _, err := os.Stat(privatePath); err != nil {
					continue
				}
				return key.User, shortenHomePath(privatePath, home)
			}
		}
	}
	return "", ""
}

func imageSSHUser(image string) string {
	image = strings.ToLower(image)
	switch {
	case image == "":
		return ""
	case strings.Contains(image, "ubuntu"):
		return "ubuntu"
	case strings.Contains(image, "debian"):
		return "debian"
	case strings.Contains(image, "centos"):
		return "centos"
	case strings.Contains(image, "rhel"), strings.Contains(image, "redhat"),
		strings.Contains(image, "rocky"), strings.Contains(image, "alma"):
		return "cloud-user"
	case strings.Contains(image, "container-optimized"), strings.Contains(image, "/cos-"),
		strings.Contains(image, "cos-"):
		return "cloud-user"
	case strings.Contains(image, "fedora"):
		return "fedora"
	default:
		return ""
	}
}

// mergeSSHKeys merges instance and project ssh-keys. Instance entries win on username.
func mergeSSHKeys(instanceRaw, projectRaw string) []cloud.SSHKey {
	projectKeys := parseSSHKeys(projectRaw)
	instanceKeys := parseSSHKeys(instanceRaw)
	if len(instanceKeys) == 0 {
		return projectKeys
	}
	if len(projectKeys) == 0 {
		return instanceKeys
	}
	seen := map[string]bool{}
	out := make([]cloud.SSHKey, 0, len(instanceKeys)+len(projectKeys))
	for _, key := range instanceKeys {
		out = append(out, key)
		if key.User != "" {
			seen[key.User] = true
		}
	}
	for _, key := range projectKeys {
		if key.User != "" && seen[key.User] {
			continue
		}
		out = append(out, key)
	}
	return out
}

func parseSSHKeys(raw string) []cloud.SSHKey {
	var keys []cloud.SSHKey
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		user, key, ok := strings.Cut(line, ":")
		if !ok {
			key = line
			user = ""
		}
		user = strings.TrimSpace(user)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if user != "" && strings.Contains(user, " ") {
			// malformed; treat whole line as key
			user = ""
			key = line
		}
		keys = append(keys, cloud.SSHKey{User: user, PublicKey: key})
	}
	return keys
}
