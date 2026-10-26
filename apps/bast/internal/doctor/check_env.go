package doctor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (e Engine) checkEnv(ctx context.Context, r *Report) {
	type tool struct {
		name string
		bin  string
	}
	tools := []tool{
		{"ssh", e.OpenSSH.SSH},
		{"ssh-keygen", e.OpenSSH.SSHKeygen},
		{"ssh-add", e.OpenSSH.SSHAdd},
	}
	var present []string
	var missing []string
	var resolved string
	for _, t := range tools {
		name := t.bin
		if name == "" {
			name = t.name
		}
		path, err := exec.LookPath(name)
		if err != nil {
			missing = append(missing, t.name)
			continue
		}
		present = append(present, t.name)
		if t.name == "ssh" {
			resolved = path
		}
	}
	if len(missing) > 0 {
		detail := "Bast needs the system OpenSSH commands on PATH."
		if runtime.GOOS == "windows" {
			detail = "Install OpenSSH Client from Windows Optional Features."
		}
		r.add(Finding{
			ID: "env.openssh_missing", Severity: SeverityFail, Category: CatEnv,
			Title: "Missing " + strings.Join(missing, ", "), Detail: detail,
			Fix: "Install OpenSSH and ensure ssh, ssh-keygen, and ssh-add are on PATH.",
		})
		return
	}
	version := e.sshVersion(ctx)
	title := strings.Join(present, " · ")
	if version != "" {
		title = "ssh " + version + " · ssh-keygen · ssh-add"
	}
	r.add(Finding{ID: "env.openssh_ok", Severity: SeverityOK, Category: CatEnv, Title: title})
	if runtime.GOOS == "windows" && gitSSH(resolved) {
		r.add(Finding{
			ID: "env.git_ssh", Severity: SeverityWarn, Category: CatEnv,
			Title:  "PATH ssh is Git's copy, not Windows OpenSSH",
			Path:   resolved,
			Detail: "Git for Windows ships its own ssh.exe. Bast expects the Windows OpenSSH Client.",
			Fix:    "Put Windows OpenSSH ahead of Git on PATH, or install OpenSSH Client from Optional Features.",
		})
	}
}

func (e Engine) sshVersion(ctx context.Context) string {
	bin := e.OpenSSH.SSH
	if bin == "" {
		bin = "ssh"
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "-V")
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	_ = cmd.Run()
	line := strings.TrimSpace(stderr.String())
	if line == "" {
		return ""
	}
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimPrefix(line, "OpenSSH_")
	if fields := strings.Fields(line); len(fields) > 0 {
		return fields[0]
	}
	return line
}

func gitSSH(path string) bool {
	norm := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(norm, "/git/") && strings.Contains(norm, "/ssh")
}

func (e Engine) inspectAgent(ctx context.Context) agentState {
	st := agentState{checked: true}
	if runtime.GOOS != "windows" && os.Getenv("SSH_AUTH_SOCK") == "" {
		st.err = "SSH_AUTH_SOCK is unset"
		return st
	}
	bin := e.OpenSSH.SSHAdd
	if bin == "" {
		bin = "ssh-add"
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, "-l")
	out, err := cmd.Output()
	if err == nil {
		st.present = true
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) != "" {
				st.count++
			}
		}
		st.empty = st.count == 0
		return st
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case 1:
			st.present = true
			st.empty = true
			return st
		case 2:
			st.err = "cannot connect to the ssh-agent"
			return st
		}
	}
	st.err = err.Error()
	return st
}
