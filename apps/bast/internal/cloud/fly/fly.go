package fly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ProviderName    = "fly"
	SSHUser         = "root"
	StoppedHostName = "fly.stopped.invalid"
	GroupRoot       = "Fly.io"
)

type Runner func(ctx context.Context, args []string, env []string) ([]byte, error)

type Client struct {
	Fly          string
	Run          Runner
	PollInterval time.Duration
}

type AccountStatus struct {
	Authenticated bool
	Login         string
	Error         string
}

type DiscoverConfig struct {
	OrgFilter      []string
	AppFilter      []string
	DefaultSSHUser string
}

type Org struct {
	Slug string
	Name string
}

func New() *Client { return &Client{Fly: resolveFlyBin(), Run: defaultRunner} }

func resolveFlyBin() string {
	if env := strings.TrimSpace(os.Getenv("FLY_CLI")); env != "" {
		return env
	}
	for _, name := range []string{"fly", "flyctl"} {
		if found, err := exec.LookPath(name); err == nil {
			return found
		}
	}
	home, err := os.UserHomeDir()
	if err == nil {
		candidates := []string{
			filepath.Join(home, ".fly", "bin", "fly"),
			filepath.Join(home, ".fly", "bin", "flyctl"),
			filepath.Join(home, ".local", "bin", "fly"),
			filepath.Join(home, ".local", "bin", "flyctl"),
		}
		for _, candidate := range candidates {
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	for _, candidate := range []string{
		"/opt/homebrew/bin/fly", "/opt/homebrew/bin/flyctl",
		"/usr/local/bin/fly", "/usr/local/bin/flyctl",
	} {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	return "fly"
}

// flyctlEnv keeps flyctl from prompting, auto-updating, or sending metrics
// while Bast captures stdout. FLY_APP is cleared so a caller shell cannot bind
// commands to an unrelated app.
var flyctlEnv = []string{
	"FLY_NO_UPDATE_CHECK=1",
	"FLY_SEND_METRICS=0",
	"FLY_APP=",
}

func isolatedWorkDir() string {
	dir := filepath.Join(os.TempDir(), "bast-fly")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	return dir
}

func configureFlyctlCmd(cmd *exec.Cmd, extraEnv []string) {
	if dir := isolatedWorkDir(); dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), flyctlEnv...)
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Env, extraEnv...)
	}
}

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.WaitDelay = 2 * time.Second
	configureFlyctlCmd(cmd, env)
	// Null stdin so flyctl does not inherit a TTY and try to prompt.
	cmd.Stdin = nil
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fly: %s", flyctlError(stderr.String(), err))
	}
	return out, nil
}

func flyctlError(stderr string, err error) string {
	msg := cleanFlyctlOutput(stderr)
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return rewriteFlyctlError(msg)
}

func cleanFlyctlOutput(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "warning: metrics token unavailable") {
			continue
		}
		if strings.Contains(lower, "error spawning metrics process") {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "\n")
}

func rewriteFlyctlError(msg string) string {
	lines := strings.Split(msg, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "Error: ")
		line = strings.TrimPrefix(line, "error: ")
		if line != "" {
			kept = append(kept, line)
		}
	}
	msg = strings.Join(kept, "\n")
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "org slug must be specified"):
		return "organization slug is required"
	case strings.Contains(lower, "region code must be specified"):
		return "region is required when flyctl cannot prompt; pass a region code such as iad"
	case strings.Contains(lower, "no access token"), strings.Contains(lower, "no auth token"):
		return "not logged in; run fly auth login"
	case strings.Contains(lower, "prompt: non interactive"):
		return "flyctl cannot prompt from Bast; run fly auth login in a terminal, and specify org and region when creating a machine"
	default:
		return msg
	}
}

func (c *Client) bin() string {
	if c.Fly != "" {
		return c.Fly
	}
	return "fly"
}

func (c *Client) pollEvery() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return time.Second
}

func (c *Client) runRaw(ctx context.Context, args ...string) ([]byte, error) {
	run := c.Run
	if run == nil {
		run = defaultRunner
	}
	full := append([]string{c.bin()}, args...)
	return run(ctx, full, nil)
}

func (c *Client) runJSON(ctx context.Context, args ...string) ([]byte, error) {
	return c.runRaw(ctx, append(args, "--json")...)
}

func (c *Client) CheckAvailable(ctx context.Context) error {
	_, err := c.runRaw(ctx, "version")
	if err == nil {
		return nil
	}
	msg := err.Error()
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "command not found") || strings.Contains(msg, "not found in $PATH") {
		return fmt.Errorf("Fly CLI not found; install flyctl from https://fly.io/docs/flyctl/install/ and run fly auth login")
	}
	if _, lookErr := exec.LookPath(c.bin()); lookErr == nil {
		return nil
	}
	return fmt.Errorf("Fly CLI is not usable: %w", err)
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return AccountStatus{Error: err.Error()}, err
	}
	out, err := c.runJSON(ctx, "auth", "whoami")
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "unauthor") || strings.Contains(lower, "not logged") ||
			strings.Contains(lower, "login") || strings.Contains(lower, "no access token") ||
			strings.Contains(lower, "no auth token") || strings.Contains(lower, "401") ||
			strings.Contains(lower, "prompt: non interactive") {
			return AccountStatus{Authenticated: false, Error: "not logged in; run fly auth login"}, nil
		}
		return AccountStatus{Error: msg}, err
	}
	login := parseWhoami(out)
	if login == "" {
		return AccountStatus{Authenticated: false, Error: "not logged in; run fly auth login"}, nil
	}
	return AccountStatus{Authenticated: true, Login: login}, nil
}

func parseWhoami(out []byte) string {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err == nil {
		for _, key := range []string{"email", "Email", "login", "Login", "id", "ID"} {
			if value, _ := raw[key].(string); strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	line := trimmed
	if idx := strings.IndexAny(line, "\r\n"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	lower := strings.ToLower(line)
	if lower == "" || strings.Contains(lower, "not logged") {
		return ""
	}
	return line
}
