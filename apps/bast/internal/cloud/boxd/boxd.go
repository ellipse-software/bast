package boxd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	stdsync "sync"
	"time"
)

const (
	ProviderName = "boxd"
	GroupRoot    = "boxd"
	SSHUser      = "boxd"
)

// ErrNotLoggedIn is returned when the boxd CLI has no session. The CLI would
// otherwise start a browser sign-in, so the runner stops it first.
var ErrNotLoggedIn = errors.New("not logged in; run boxd auth login")

// ErrNotFound is returned when the CLI reports that a computer does not exist.
var ErrNotFound = errors.New("boxd computer not found")

// Runner runs the boxd CLI. It returns stderr even when the command succeeds:
// a `--region all` list names a region it could not read there and exits 0.
type Runner func(ctx context.Context, args []string, env []string) (stdout, stderr []byte, err error)

// SSHResolver returns the SSH settings OpenSSH resolves for host, which come
// from the entries the boxd CLI keeps in ~/.ssh/config.
type SSHResolver func(ctx context.Context, host string) (SSHDetails, error)

type SSHDetails struct {
	HostName       string
	Port           string
	User           string
	IdentityFile   string
	IdentitiesOnly bool
	DeviceID       string
}

// Found reports whether the boxd CLI has written an entry for the host. Port
// 22 on a computer's domain reaches the boxd proxy, not the computer.
func (d SSHDetails) Found() bool {
	port := strings.TrimSpace(d.Port)
	return port != "" && port != "22"
}

type Client struct {
	Boxd         string
	Run          Runner
	ResolveSSH   SSHResolver
	PollInterval time.Duration
	Home         string
}

type Org struct {
	ID     string
	Name   string
	Active bool
}

type AccountStatus struct {
	Authenticated bool
	Login         string
	Orgs          []Org
	ActiveOrg     string
	Error         string
}

func New() *Client {
	home, _ := os.UserHomeDir()
	return &Client{Boxd: resolveBoxdBin(), Run: defaultRunner, ResolveSSH: resolveWithOpenSSH, Home: home}
}

// resolveBoxdBin finds the boxd CLI. The installer puts it in ~/.local/bin,
// which GUI and TUI launches often leave off PATH.
func resolveBoxdBin() string {
	if env := strings.TrimSpace(os.Getenv("BOXD_CLI")); env != "" {
		return env
	}
	if found, err := exec.LookPath("boxd"); err == nil {
		return found
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".local", "bin", "boxd")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	for _, candidate := range []string{"/opt/homebrew/bin/boxd", "/usr/local/bin/boxd"} {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	return "boxd"
}

// loginWatch collects stderr and cancels the command as soon as the CLI says
// it is not logged in, before it opens a browser sign-in nobody is watching.
type loginWatch struct {
	mu       stdsync.Mutex
	buf      bytes.Buffer
	cancel   context.CancelFunc
	prompted bool
}

func (w *loginWatch) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if !w.prompted && strings.Contains(w.buf.String(), "Not logged in") {
		w.prompted = true
		w.cancel()
	}
	return len(p), nil
}

func (w *loginWatch) result() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String(), w.prompted
}

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, []byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = nil
	watch := &loginWatch{cancel: cancel}
	cmd.Stderr = watch
	out, err := cmd.Output()
	stderr, prompted := watch.result()
	if prompted {
		return nil, nil, ErrNotLoggedIn
	}
	if err != nil {
		return out, []byte(stderr), cliError(stderr, err)
	}
	return out, []byte(stderr), nil
}

func cliError(stderr string, err error) error {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("boxd: %w", err)
	}
	msg := cleanCLIOutput(stderr)
	if msg == "" && err != nil {
		msg = err.Error()
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "no machine"), strings.Contains(lower, "not found") && strings.Contains(lower, "machine"):
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case strings.Contains(lower, "not logged in"), strings.Contains(lower, "invalidtoken"),
		strings.Contains(lower, "unauthenticated"), strings.Contains(lower, "token expired"):
		return ErrNotLoggedIn
	}
	return fmt.Errorf("boxd: %s", msg)
}

// cleanCLIOutput drops the CLI's update notice and feedback hint from stderr.
func cleanCLIOutput(s string) string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "a new version of boxd") || strings.HasPrefix(lower, "update with:") ||
			strings.HasPrefix(lower, "curl -fssl") || strings.Contains(lower, "`boxd feedback`") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "error: ")
		trimmed = strings.TrimPrefix(trimmed, "Error: ")
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "\n")
}

func isNotFound(err error) bool {
	return err != nil && (errors.Is(err, ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "no machine"))
}

func isNotLoggedIn(err error) bool {
	return err != nil && (errors.Is(err, ErrNotLoggedIn) || strings.Contains(strings.ToLower(err.Error()), "not logged in"))
}

func (c *Client) bin() string {
	if c.Boxd != "" {
		return c.Boxd
	}
	return "boxd"
}

func (c *Client) pollEvery() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return time.Second
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	out, _, err := c.runWithStderr(ctx, args...)
	return out, err
}

func (c *Client) runWithStderr(ctx context.Context, args ...string) ([]byte, []byte, error) {
	run := c.Run
	if run == nil {
		run = defaultRunner
	}
	return run(ctx, append([]string{c.bin()}, args...), nil)
}

func (c *Client) CheckAvailable(ctx context.Context) error {
	_, err := c.run(ctx, "version")
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("boxd CLI not found; install it with curl -fsSL https://boxd.sh/downloads/install.sh | sh and run boxd auth login")
	}
	return fmt.Errorf("boxd CLI is not usable: %w", err)
}

type authRecord struct {
	ActiveOrg   *string `json:"active_org"`
	DisplayName string  `json:"display_name"`
	UserID      string  `json:"user_id"`
	Orgs        []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	} `json:"orgs"`
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return AccountStatus{Error: err.Error()}, err
	}
	out, err := c.run(ctx, "auth", "--json")
	if err != nil {
		if isNotLoggedIn(err) {
			return AccountStatus{Error: ErrNotLoggedIn.Error()}, nil
		}
		return AccountStatus{Error: err.Error()}, err
	}
	var rec authRecord
	if err := decodeFirstJSON(out, &rec); err != nil {
		return AccountStatus{Error: err.Error()}, fmt.Errorf("parse boxd auth: %w", err)
	}
	status := AccountStatus{Authenticated: true, Login: strings.TrimSpace(rec.DisplayName)}
	if status.Login == "" {
		status.Login = strings.TrimSpace(rec.UserID)
	}
	active := ""
	if rec.ActiveOrg != nil {
		active = strings.TrimSpace(*rec.ActiveOrg)
	}
	for _, raw := range rec.Orgs {
		org := Org{ID: strings.TrimSpace(raw.ID), Name: strings.TrimSpace(raw.Name), Active: raw.Active}
		if org.ID == "" {
			continue
		}
		if org.Name == "" {
			org.Name = org.ID
		}
		if active != "" && (strings.EqualFold(org.Name, active) || org.ID == active) {
			org.Active = true
		}
		if org.Active {
			status.ActiveOrg = org.Name
		}
		status.Orgs = append(status.Orgs, org)
	}
	return status, nil
}

// SelectOrgs returns the orgs to sync: the filter's entries (names or ids,
// case-insensitive), or the CLI's active org, or the only org an API key has.
func SelectOrgs(account AccountStatus, filter []string) ([]Org, error) {
	filter = normalizeFilter(filter)
	if len(filter) > 0 {
		var out []Org
		var missing []string
		for _, want := range filter {
			found := false
			for _, org := range account.Orgs {
				if strings.EqualFold(org.Name, want) || org.ID == want {
					out = append(out, org)
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			return out, fmt.Errorf("boxd org %s not found for this login", strings.Join(missing, ", "))
		}
		return out, nil
	}
	for _, org := range account.Orgs {
		if org.Active {
			return []Org{org}, nil
		}
	}
	if len(account.Orgs) == 1 {
		return []Org{account.Orgs[0]}, nil
	}
	return nil, fmt.Errorf("no active boxd org; run boxd auth switch, or set the boxd org filter")
}

var valuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validateValue(label, value string) error {
	if !valuePattern.MatchString(value) {
		return fmt.Errorf("boxd %s %q contains unsupported characters", label, value)
	}
	return nil
}

func decodeFirstJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(v)
}

func normalizeFilter(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
