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

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("fly: %s", msg)
	}
	return out, nil
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
	out, err := c.runRaw(ctx, "auth", "whoami")
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "unauthor") || strings.Contains(lower, "not logged") ||
			strings.Contains(lower, "login") || strings.Contains(lower, "no access token") ||
			strings.Contains(lower, "401") {
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
