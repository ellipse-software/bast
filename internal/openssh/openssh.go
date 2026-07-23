package openssh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"bast/internal/sshconfig"
)

type Client struct {
	SSH       string
	SSHKeygen string
	SSHAdd    string
}

func Default() Client {
	return Client{SSH: "ssh", SSHKeygen: "ssh-keygen", SSHAdd: "ssh-add"}
}

func (c Client) Check() error {
	for _, name := range []string{c.SSH, c.SSHKeygen, c.SSHAdd} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("required OpenSSH tool %q is not available on PATH", name)
		}
	}
	return nil
}

func (c Client) Resolve(ctx context.Context, alias string) (sshconfig.Resolved, error) {
	if alias == "" || strings.HasPrefix(alias, "-") {
		return sshconfig.Resolved{}, errors.New("invalid host label")
	}
	cmd := exec.CommandContext(ctx, c.SSH, "-G", "--", alias)
	out, err := cmd.Output()
	if err != nil {
		return sshconfig.Resolved{}, fmt.Errorf("resolve %s: %w", alias, err)
	}
	var resolved sshconfig.Resolved
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "hostname":
			resolved.HostName = value
		case "user":
			resolved.User = value
		case "port":
			resolved.Port = value
		case "identityfile":
			resolved.IdentityFiles = append(resolved.IdentityFiles, expandHome(value))
		case "identitiesonly":
			resolved.IdentitiesOnly = value
		case "proxyjump":
			resolved.ProxyJump = value
		}
	}
	return resolved, scanner.Err()
}

func (c Client) SSHCommand(alias string) (*exec.Cmd, error) {
	if alias == "" || strings.HasPrefix(alias, "-") || strings.ContainsAny(alias, "\r\n\x00") {
		return nil, errors.New("invalid host label")
	}
	cmd := exec.Command(c.SSH, "--", alias)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func (c Client) Fingerprints(ctx context.Context, host string, port string) (string, error) {
	if host == "" {
		return "", nil
	}
	lookup := host
	if port != "" && port != "22" {
		lookup = "[" + host + "]:" + port
	}
	cmd := exec.CommandContext(ctx, c.SSHKeygen, "-F", lookup)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("look up known host: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (c Client) RemoveKnownHost(ctx context.Context, host, port string) error {
	lookup := host
	if port != "" && port != "22" {
		lookup = "[" + host + "]:" + port
	}
	cmd := exec.CommandContext(ctx, c.SSHKeygen, "-R", lookup)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("remove known host: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
