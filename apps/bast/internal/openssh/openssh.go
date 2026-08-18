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
			return missingToolError(name)
		}
	}
	return nil
}

func (c Client) Resolve(ctx context.Context, alias string) (sshconfig.Resolved, error) {
	if err := validateAlias(alias); err != nil {
		return sshconfig.Resolved{}, err
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
		case "pubkeyauthentication":
			resolved.PubkeyAuthentication = value
		case "passwordauthentication":
			resolved.PasswordAuthentication = value
		case "preferredauthentications":
			resolved.PreferredAuthentications = value
		case "proxyjump":
			resolved.ProxyJump = value
		}
	}
	return resolved, scanner.Err()
}

func (c Client) InstallPublicKeyCommand(alias, publicKey string) (*exec.Cmd, error) {
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" || strings.ContainsAny(publicKey, "\r\n\x00") {
		return nil, errors.New("public key must contain exactly one non-empty line")
	}
	remote := `umask 077
mkdir -p "$HOME/.ssh" &&
touch "$HOME/.ssh/authorized_keys" &&
chmod 700 "$HOME/.ssh" &&
chmod 600 "$HOME/.ssh/authorized_keys" &&
IFS= read -r key &&
if grep -qxF "$key" "$HOME/.ssh/authorized_keys"; then
    printf '%s\n' 'Public key is already installed.'
else
    printf '\n%s\n' "$key" >> "$HOME/.ssh/authorized_keys" &&
    printf '%s\n' 'Public key installed.'
fi`
	cmd := exec.Command(c.SSH, "--", alias, remote)
	cmd.Stdin = strings.NewReader(publicKey + "\n")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func (c Client) SSHCommand(alias string) (*exec.Cmd, error) {
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	cmd := exec.Command(c.SSH, "--", alias)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

// SFTPCommand starts an OpenSSH SFTP subsystem session for alias.
// Stdin and stdout are left unset so the caller can attach pipes for the SFTP protocol.
func (c Client) SFTPCommand(alias string) (*exec.Cmd, error) {
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	// -s treats the remote command as a subsystem name (sftp). BatchMode avoids
	// passphrase/host-key prompts that cannot interrupt the Bast TUI.
	cmd := exec.Command(c.SSH, "-o", "BatchMode=yes", "-s", "--", alias, "sftp")
	return cmd, nil
}

// SSHCommandInDir opens an interactive shell on alias with cwd set to remoteDir.
// RemoteCommand is cleared so host startup commands (e.g. tmux attach) do not
// override the cd + shell invocation.
func (c Client) SSHCommandInDir(alias, remoteDir string) (*exec.Cmd, error) {
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	remoteDir = strings.TrimSpace(remoteDir)
	if remoteDir == "" || strings.ContainsAny(remoteDir, "\r\n\x00") {
		return nil, errors.New("invalid remote directory")
	}
	remote := "cd " + shellQuote(remoteDir) + " && exec \"${SHELL:-/bin/sh}\" -l"
	cmd := exec.Command(c.SSH, "-t", "-o", "RemoteCommand=none", "--", alias, remote)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("-_=./:@%+", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
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
	if strings.TrimSpace(host) == "" {
		return errors.New("host is required")
	}
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

func validateAlias(alias string) error {
	if alias == "" || strings.HasPrefix(alias, "-") || strings.ContainsAny(alias, "\r\n\x00") {
		return errors.New("invalid host label")
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

// FormatError returns a user-facing description for OpenSSH process failures.
// ssh(1) uses exit 255 for its own errors (connection, auth, or interrupt);
// other non-zero codes are typically the remote command's exit status, including
// the conventional 128+signal form used by shells.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return err.Error()
	}
	code := exitErr.ExitCode()
	if code <= 0 {
		return "interrupted before completion"
	}
	if msg, ok := exitMessages[code]; ok {
		return msg
	}
	if code > 128 && code < 255 {
		sig := code - 128
		if name := signalName(sig); name != "" {
			return fmt.Sprintf("terminated by %s", name)
		}
		return fmt.Sprintf("terminated by signal %d", sig)
	}
	return fmt.Sprintf("exited with status %d", code)
}

var exitMessages = map[int]string{
	1:   "command failed",
	2:   "invalid arguments or misuse",
	126: "command not executable",
	127: "command not found",
	128: "invalid exit status",
	129: "terminated by SIGHUP (hangup)",
	130: "interrupted (Ctrl-C)",
	131: "terminated by SIGQUIT",
	137: "killed",
	143: "terminated",
	255: "connection failed, refused, or interrupted",
}

func signalName(sig int) string {
	switch sig {
	case 1:
		return "SIGHUP"
	case 2:
		return "SIGINT"
	case 3:
		return "SIGQUIT"
	case 9:
		return "SIGKILL"
	case 15:
		return "SIGTERM"
	default:
		return ""
	}
}
