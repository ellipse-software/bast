package upstash

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	AskPassEnv   = "BAST_SSH_ASKPASS"
	AskPassValue = "1"
)

func IsAskPassRequest() bool {
	return os.Getenv(AskPassEnv) == AskPassValue
}

func PrintAPIKey(out io.Writer, keyFile string) error {
	key, err := ReadKeyFile(keyFile)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, key)
	return err
}

func ApplyAskPass(cmd *exec.Cmd, bastExecutable string) {
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
	filtered := make([]string, 0, len(env)+3)
	for _, item := range env {
		name, _, _ := strings.Cut(item, "=")
		switch name {
		case AskPassEnv, "SSH_ASKPASS", "SSH_ASKPASS_REQUIRE":
			continue
		}
		filtered = append(filtered, item)
	}
	cmd.Env = append(filtered,
		AskPassEnv+"="+AskPassValue,
		"SSH_ASKPASS="+exe,
		"SSH_ASKPASS_REQUIRE=force",
	)
}
