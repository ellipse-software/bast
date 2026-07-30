package gcp

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"bast/internal/cloud"
)

const gcloudIdentityFile = "~/.ssh/google_compute_engine"

func ResolveAuth(inst *cloud.Instance, home, managedKeys, osLoginUser string) {
	if inst == nil {
		return
	}
	settingsUser := strings.TrimSpace(inst.User)
	if inst.OSLogin {
		inst.User = strings.TrimSpace(osLoginUser)
		if inst.User == "" {
			inst.User = settingsUser
		}
		inst.IdentityFile = ""
		inst.IdentitiesOnly = false
		if home != "" {
			if _, err := os.Stat(filepath.Join(home, ".ssh", "google_compute_engine")); err == nil {
				inst.IdentityFile = gcloudIdentityFile
			}
		}
		return
	}

	if user, identity := matchSSHKeyEntry(inst.SSHKeys, home, managedKeys); identity != "" {
		if user == "" {
			user = settingsUser
		}
		inst.User = user
		inst.IdentityFile = identity
		inst.IdentitiesOnly = true
		return
	}

	userName := settingsUser
	if userName == "" {
		userName = imageSSHUser(inst.Image)
	}
	if userName == "" {
		userName = firstSSHKeyUser(inst.SSHKeys)
	}
	inst.User = userName
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
				if key.Expired {
					continue
				}
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

func firstSSHKeyUser(keys []cloud.SSHKey) string {
	for _, key := range keys {
		if key.Expired || key.User == "" {
			continue
		}
		return key.User
	}
	return ""
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
		userName, key, ok := strings.Cut(line, ":")
		if !ok {
			key = line
			userName = ""
		}
		userName = strings.TrimSpace(userName)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if userName != "" && strings.Contains(userName, " ") {
			// malformed; treat whole line as key
			userName = ""
			key = line
		}
		keys = append(keys, cloud.SSHKey{
			User:      userName,
			PublicKey: key,
			Expired:   sshKeyExpired(key),
		})
	}
	return keys
}

func sshKeyExpired(publicKey string) bool {
	idx := strings.Index(publicKey, "google-ssh")
	if idx < 0 {
		return false
	}
	rest := strings.TrimSpace(publicKey[idx+len("google-ssh"):])
	if rest == "" || rest[0] != '{' {
		return false
	}
	var meta struct {
		ExpireOn string `json:"expireOn"`
	}
	if err := json.Unmarshal([]byte(rest), &meta); err != nil {
		return false
	}
	expireOn := strings.TrimSpace(meta.ExpireOn)
	if expireOn == "" {
		return false
	}
	// gcloud uses RFC3339-ish values, sometimes without colon in the zone offset.
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05+0000",
		"2006-01-02T15:04:05Z07:00",
	} {
		if ts, err := time.Parse(layout, expireOn); err == nil {
			return !ts.After(time.Now().UTC())
		}
	}
	return false
}

func localUsername() string {
	if u, err := user.Current(); err == nil {
		if name := strings.TrimSpace(u.Username); name != "" {
			if i := strings.LastIndex(name, `\`); i >= 0 {
				name = name[i+1:]
			}
			return name
		}
	}
	return ""
}
