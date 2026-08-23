package askpass

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"bast/internal/hostpass"
	"bast/internal/sshconfig"
)

const (
	Env         = "BAST_SSH_ASKPASS"
	Value       = "1"
	KindEnv     = "BAST_SSH_ASKPASS_KIND"
	IDEnv       = "BAST_SSH_ASKPASS_ID"
	KindUpstash = "upstash"
	KindHost    = "host"
)

func IsRequest() bool {
	return os.Getenv(Env) == Value
}

func Kind() string {
	kind := strings.TrimSpace(os.Getenv(KindEnv))
	if kind == "" {
		return KindUpstash
	}
	return kind
}

func HostID() string {
	return strings.TrimSpace(os.Getenv(IDEnv))
}

func Apply(cmd *exec.Cmd, bastExecutable, kind, id string) {
	if cmd == nil {
		return
	}
	exe := strings.TrimSpace(bastExecutable)
	if exe == "" {
		if self, err := os.Executable(); err == nil {
			exe = self
		} else {
			exe = os.Args[0]
		}
	}
	if !filepath.IsAbs(exe) {
		if found, err := exec.LookPath(exe); err == nil {
			exe = found
		}
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	filtered := make([]string, 0, len(env)+5)
	for _, item := range env {
		name, _, _ := strings.Cut(item, "=")
		switch name {
		case Env, KindEnv, IDEnv, "SSH_ASKPASS", "SSH_ASKPASS_REQUIRE":
			continue
		}
		filtered = append(filtered, item)
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = KindUpstash
	}
	filtered = append(filtered,
		Env+"="+Value,
		KindEnv+"="+kind,
		"SSH_ASKPASS="+exe,
		"SSH_ASKPASS_REQUIRE=force",
	)
	if id = strings.TrimSpace(id); id != "" {
		filtered = append(filtered, IDEnv+"="+id)
	}
	cmd.Env = filtered
}

func ApplyUpstash(cmd *exec.Cmd, bastExecutable string) {
	Apply(cmd, bastExecutable, KindUpstash, "")
}

func ApplyHost(cmd *exec.Cmd, bastExecutable, managedID string) {
	Apply(cmd, bastExecutable, KindHost, managedID)
}

func Needed(host sshconfig.Host, passwordsDir string) bool {
	if host.Synced && host.SyncSource == "upstash" {
		return true
	}
	return host.Managed && host.ManagedID != "" && hostpass.Exists(passwordsDir, host.ManagedID)
}

func Prepare(cmd *exec.Cmd, bastExecutable string, host sshconfig.Host, passwordsDir string) {
	if cmd == nil {
		return
	}
	if host.Synced && host.SyncSource == "upstash" {
		ApplyUpstash(cmd, bastExecutable)
		return
	}
	if host.Managed && host.ManagedID != "" && hostpass.Exists(passwordsDir, host.ManagedID) {
		ApplyHost(cmd, bastExecutable, host.ManagedID)
	}
}
