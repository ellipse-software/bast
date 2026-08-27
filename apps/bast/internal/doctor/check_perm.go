package doctor

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"bast/internal/platform"
)

func (e Engine) checkPerm(r *Report, st runState) {
	if !platform.SupportsPOSIXPermissions() {
		if dirExists(e.Paths.ManagedDir) {
			r.add(Finding{
				ID: "permissions.ok", Severity: SeverityOK, Category: CatPermissions,
				Title: "Bast-managed paths are present (Windows DACL applied when Bast writes them)",
			})
		}
		return
	}
	issues := 0
	issues += e.checkDirWritable(r, e.Paths.SSHDir, "permissions.ssh_dir", "~/.ssh")
	if fileExists(e.Paths.MainConfig) {
		issues += e.checkOtherWritable(r, e.Paths.MainConfig, "permissions.ssh_config", "~/.ssh/config")
	}
	if dirExists(e.Paths.ManagedDir) {
		issues += e.checkDirWritable(r, e.Paths.ManagedDir, "permissions.bast_dir", "~/.ssh/bast")
	}
	if dirExists(e.Paths.ManagedKeys) {
		issues += e.checkDirWritable(r, e.Paths.ManagedKeys, "permissions.bast_keys_dir", "~/.ssh/bast/keys")
	}
	configDir := filepath.Dir(e.Paths.StateFile)
	if dirExists(configDir) {
		issues += e.checkDirWritable(r, configDir, "permissions.config_dir", "~/.config/bast")
	}
	for _, item := range []struct{ path, id, title string }{
		{e.Paths.StateFile, "permissions.state", "~/.config/bast/state.json"},
		{e.Paths.UpstashAPIKey, "permissions.upstash_key", "~/.config/bast/upstash-box-api-key"},
	} {
		if fileExists(item.path) {
			issues += e.checkNotShared(r, item.path, item.id, item.title, 0o077)
		}
	}
	issues += e.checkPrivateKeyModes(r, st)
	if issues == 0 {
		r.add(Finding{
			ID: "permissions.ok", Severity: SeverityOK, Category: CatPermissions,
			Title: "~/.ssh and identity files are not group or world accessible",
		})
	}
}

func (e Engine) checkDirWritable(r *Report, path, id, title string) int {
	return e.checkNotShared(r, path, id, title, 0o022)
}

func (e Engine) checkOtherWritable(r *Report, path, id, title string) int {
	return e.checkNotShared(r, path, id, title, 0o002)
}

func (e Engine) checkNotShared(r *Report, path, id, title string, bad fs.FileMode) int {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	mode := info.Mode().Perm()
	if mode&bad == 0 {
		return 0
	}
	fixMode := "600"
	if info.IsDir() {
		fixMode = "700"
	}
	detail := "OpenSSH refuses paths that are accessible by others."
	if id == "permissions.ssh_config" {
		detail = "OpenSSH warns when ~/.ssh/config is world-accessible."
	}
	r.add(Finding{
		ID: id, Severity: SeverityFail, Category: CatPermissions,
		Title: title + " mode is too open", Path: e.display(path),
		Detail: detail, Fix: "chmod " + fixMode + " " + e.display(path), Fixable: true,
	})
	return 1
}

func (e Engine) checkPrivateKeyModes(r *Report, st runState) int {
	issues := 0
	seen := map[string]bool{}
	scan := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasSuffix(entry.Name(), ".pub") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] || !looksPrivateKey(path) {
				continue
			}
			seen[path] = true
			issues += e.checkNotShared(r, path, "permissions.private_key", e.display(path), 0o077)
		}
	}
	scan(e.Paths.SSHDir)
	scan(e.Paths.ManagedKeys)
	for _, h := range st.hosts {
		for _, id := range h.IdentityFiles {
			if id == "" || seen[id] || strings.HasSuffix(id, ".pub") || !fileExists(id) {
				continue
			}
			if !looksPrivateKey(id) {
				continue
			}
			seen[id] = true
			issues += e.checkNotShared(r, id, "permissions.private_key", e.display(id), 0o077)
		}
	}
	return issues
}

func looksPrivateKey(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 80)
	n, _ := f.Read(buf)
	return n > 0 && strings.Contains(string(buf[:n]), "PRIVATE KEY")
}
