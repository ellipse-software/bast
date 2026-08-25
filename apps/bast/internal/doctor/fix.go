package doctor

import (
	"os"
	"path/filepath"
	"strings"

	"bast/internal/platform"
)

func (e Engine) applyFixes(r Report) []string {
	var applied []string
	needInclude := false
	for _, f := range r.Findings {
		if !f.Fixable {
			continue
		}
		switch f.ID {
		case "ssh_config.include_missing", "ssh_config.include_not_toplevel", "ssh_config.missing":
			needInclude = true
		case "permissions.ssh_dir", "permissions.bast_dir", "permissions.bast_keys_dir", "permissions.config_dir":
			if e.fixDirMode(f.Path) {
				applied = append(applied, "permissions on "+f.Path)
			}
		case "permissions.ssh_config", "permissions.state", "permissions.upstash_key", "permissions.private_key", "vault.passphrase_mode":
			if e.fixFileMode(f.Path) {
				applied = append(applied, "permissions on "+f.Path)
			}
		}
	}
	if needInclude {
		if err := e.Config.EnsureManaged(); err == nil {
			applied = append(applied, "Include ~/.ssh/bast/config at the top of ~/.ssh/config")
		}
	}
	if platform.SupportsPOSIXPermissions() {
		return uniqueStrings(applied)
	}
	for _, path := range []string{e.Paths.ManagedDir, e.Paths.ManagedKeys, e.Paths.ManagedConfig} {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			_ = platform.SecurePath(path, 0o700)
		}
	}
	return uniqueStrings(applied)
}

func (e Engine) fixDirMode(displayPath string) bool {
	path := e.expandDisplay(displayPath)
	if path == "" {
		return false
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return false
	}
	return true
}

func (e Engine) fixFileMode(displayPath string) bool {
	path := e.expandDisplay(displayPath)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return e.fixDirMode(displayPath)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return false
	}
	return true
}

func (e Engine) expandDisplay(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	if path == "~/.ssh" {
		return e.Paths.SSHDir
	}
	if path == "~/.ssh/config" {
		return e.Paths.MainConfig
	}
	if path == "~/.ssh/bast" {
		return e.Paths.ManagedDir
	}
	if path == "~/.ssh/bast/keys" {
		return e.Paths.ManagedKeys
	}
	if path == "~/.config/bast" {
		return filepath.Dir(e.Paths.StateFile)
	}
	if path == "~/.config/bast/state.json" {
		return e.Paths.StateFile
	}
	if path == "~/.config/bast/upstash-box-api-key" {
		return e.Paths.UpstashAPIKey
	}
	if path == "~/.config/bast/vault-passphrase" {
		return filepath.Join(filepath.Dir(e.Paths.StateFile), "vault-passphrase")
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(e.Paths.Home, path[2:])
	}
	return path
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
