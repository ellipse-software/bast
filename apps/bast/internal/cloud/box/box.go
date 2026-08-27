package box

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const ProviderName = "box"

const (
	SSHUser         = "user"
	IdentityFile    = "~/.ssh/ascii_box_ed25519"
	StoppedHostName = "box.stopped.invalid"
)

// stoppedHostName keeps internal call sites on the exported constant.
const stoppedHostName = StoppedHostName

type Runner func(ctx context.Context, args []string, env []string) ([]byte, error)

type Client struct {
	Box string
	Run Runner
	// PollInterval overrides WaitReady/WaitStopped sleep; zero uses 1s.
	PollInterval time.Duration
}

type AccountStatus struct {
	Authenticated bool
	Login         string
	Email         string
	Plan          string
	Error         string
}

type DiscoverConfig struct{}

type Discovery struct {
	Instances []Instance
	Warnings  []string
	Complete  bool
}

type Instance struct {
	SyncID            string
	Name              string
	State             string
	HostName          string
	User              string
	IdentityFile      string
	IdentitiesOnly    bool
	Running           bool
	SnapshotAvailable bool
	BoxType           string
	Tags              []string
}

type boxRecord struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	State             string  `json:"state"`
	Type              string  `json:"type"`
	IP                *string `json:"ip"`
	SnapshotAvailable bool    `json:"snapshotAvailable"`
}

func New() *Client { return &Client{Box: resolveBoxBin(), Run: defaultRunner} }

// resolveBoxBin finds the Box CLI. The installer puts it at ~/.ascii/bin/box and
// exposes a shell function named box, so LookPath("box") often fails for GUI/TUI launches.
func resolveBoxBin() string {
	if env := strings.TrimSpace(os.Getenv("BOX_CLI")); env != "" {
		return env
	}
	if found, err := exec.LookPath("box"); err == nil {
		return found
	}
	home, err := os.UserHomeDir()
	if err == nil {
		candidates := []string{
			filepath.Join(home, ".ascii", "bin", "box"),
			filepath.Join(home, ".local", "bin", "box"),
		}
		for _, candidate := range candidates {
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	for _, candidate := range []string{"/opt/homebrew/bin/box", "/usr/local/bin/box"} {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate
		}
	}
	return "box"
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
		return nil, fmt.Errorf("box: %s", msg)
	}
	return out, nil
}

func (c *Client) bin() string {
	if c.Box != "" {
		return c.Box
	}
	return "box"
}

const boxCLIProbeTimeout = 20 * time.Second

func boundBoxCmd(ctx context.Context, args []string) (context.Context, context.CancelFunc) {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "info", "list", "status", "--version":
		return context.WithTimeout(ctx, boxCLIProbeTimeout)
	default:
		return ctx, func() {}
	}
}

func (c *Client) runRaw(ctx context.Context, args ...string) ([]byte, error) {
	run := c.Run
	if run == nil {
		run = defaultRunner
	}
	ctx, cancel := boundBoxCmd(ctx, args)
	defer cancel()
	full := append([]string{c.bin()}, args...)
	return run(ctx, full, nil)
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	args = append(append([]string{}, args...), "--json", "--no-update")
	return c.runRaw(ctx, args...)
}

func (c *Client) CheckAvailable(ctx context.Context) error {
	_, err := c.runRaw(ctx, "--version")
	if err == nil {
		return nil
	}
	msg := err.Error()
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "command not found") || strings.Contains(msg, "not found in $PATH") {
		return fmt.Errorf("Box CLI not found; install from https://box.ascii.dev/ and run box login")
	}
	if _, lookErr := exec.LookPath(c.bin()); lookErr == nil {
		return nil
	}
	return fmt.Errorf("Box CLI is not usable: %w", err)
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return AccountStatus{Error: err.Error()}, err
	}
	out, err := c.run(ctx, "status")
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "unauthor") || strings.Contains(lower, "not logged") ||
			strings.Contains(lower, "login") || strings.Contains(lower, "401") {
			return AccountStatus{Authenticated: false, Error: "not logged in; run box login"}, nil
		}
		return AccountStatus{Error: msg}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return AccountStatus{Authenticated: false, Error: "could not parse box status"}, nil
	}
	login, email, plan := parseStatusIdentity(raw)
	if !statusLooksAuthenticated(raw) && login == "" && email == "" {
		return AccountStatus{Authenticated: false, Error: "not logged in; run box login"}, nil
	}
	if login == "" {
		login = email
	}
	return AccountStatus{Authenticated: true, Login: login, Email: email, Plan: plan}, nil
}

func parseStatusIdentity(raw map[string]any) (login, email, plan string) {
	if user, ok := raw["user"].(map[string]any); ok {
		login, _ = user["login"].(string)
		email, _ = user["email"].(string)
		if login == "" {
			login, _ = user["identifier"].(string)
		}
	}
	if account, ok := raw["account"].(map[string]any); ok {
		if login == "" {
			login, _ = account["login"].(string)
		}
		if login == "" {
			login, _ = account["identifier"].(string)
		}
		if email == "" {
			email, _ = account["email"].(string)
		}
		// Real box status uses identifier for email-style accounts.
		if email == "" && strings.Contains(login, "@") {
			email = login
		}
		plan, _ = account["plan"].(string)
		if plan == "no plan" {
			plan = ""
		}
	}
	if data, ok := raw["data"].(map[string]any); ok {
		l, e, p := parseStatusIdentity(data)
		if login == "" {
			login = l
		}
		if email == "" {
			email = e
		}
		if plan == "" {
			plan = p
		}
	}
	if plan == "" {
		plan, _ = raw["plan"].(string)
	}
	return strings.TrimSpace(login), strings.TrimSpace(email), strings.TrimSpace(plan)
}

func statusLooksAuthenticated(raw map[string]any) bool {
	if account, ok := raw["account"].(map[string]any); ok {
		loginState, _ := account["loginState"].(string)
		status, _ := account["status"].(string)
		identifier, _ := account["identifier"].(string)
		login, _ := account["login"].(string)
		if strings.EqualFold(loginState, "active") || strings.EqualFold(status, "active") {
			return true
		}
		if strings.TrimSpace(identifier) != "" || strings.TrimSpace(login) != "" {
			return true
		}
	}
	if ok, _ := raw["ok"].(bool); ok {
		return true
	}
	if _, has := raw["user"]; has {
		return true
	}
	return false
}

func (c *Client) Discover(ctx context.Context, _ DiscoverConfig) (Discovery, error) {
	account, err := c.Account(ctx)
	if err != nil {
		return Discovery{}, err
	}
	if !account.Authenticated {
		msg := account.Error
		if msg == "" {
			msg = "not logged in; run box login"
		}
		return Discovery{}, fmt.Errorf("%s", msg)
	}
	// Include stopping/archiving (t): Box snapshotting can take minutes, and
	// omitting that group makes Bast delete the host until it becomes stopped.
	out, err := c.run(ctx, "list", "--filter", "srt")
	if err != nil {
		return Discovery{}, err
	}
	var raw struct {
		Boxes []boxRecord `json:"boxes"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Discovery{}, fmt.Errorf("parse box list: %w", err)
	}
	instances := make([]Instance, 0, len(raw.Boxes))
	for _, rec := range raw.Boxes {
		if inst, ok := instanceFromRecord(rec); ok {
			instances = append(instances, inst)
		}
	}
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Running != instances[j].Running {
			return instances[i].Running
		}
		return instances[i].Name < instances[j].Name
	})
	return Discovery{Instances: instances, Complete: true}, nil
}

func instanceFromRecord(rec boxRecord) (Instance, bool) {
	id := strings.TrimSpace(rec.ID)
	if id == "" {
		return Instance{}, false
	}
	state := normalizeState(rec.State)
	running := isRunningState(state)
	ip := ""
	if rec.IP != nil {
		ip = strings.TrimSpace(*rec.IP)
	}
	hostName := ip
	if hostName == "" {
		// Keep running boxes with no IP yet so sync does not delete metadata
		// during the brief post-start/clone window. EnsureAccess rejects the
		// placeholder until a real IP appears.
		hostName = stoppedHostName
	}
	name := strings.TrimSpace(rec.Name)
	if name == "" {
		name = id
	}
	tags := []string{"state:" + state}
	if rec.Type != "" {
		tags = append(tags, "type:"+rec.Type)
	}
	if rec.SnapshotAvailable {
		tags = append(tags, "snapshot")
	}
	return Instance{
		SyncID:            id,
		Name:              name,
		State:             state,
		HostName:          hostName,
		User:              SSHUser,
		IdentityFile:      IdentityFile,
		IdentitiesOnly:    true,
		Running:           running,
		SnapshotAvailable: rec.SnapshotAvailable,
		BoxType:           strings.TrimSpace(rec.Type),
		Tags:              tags,
	}, true
}

func normalizeState(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "provisioned":
		return "ready"
	case "archiving":
		return "stopping"
	case "archived":
		return "stopped"
	default:
		return strings.ToLower(strings.TrimSpace(state))
	}
}

func isRunningState(state string) bool {
	switch normalizeState(state) {
	case "ready", "idle", "running", "cloning":
		return true
	default:
		return false
	}
}

func IsStoppedState(state string) bool {
	switch normalizeState(state) {
	case "stopped", "stopping":
		return true
	default:
		return false
	}
}

// IsTerminalStoppedState is true only when stop/archive has finished.
func IsTerminalStoppedState(state string) bool {
	return normalizeState(state) == "stopped"
}

// HostLooksStopped reports whether a synced Box host should be treated as
// stopped (state tags from the last sync, or the placeholder hostname when
// state is unknown). A running box with no IP yet keeps the placeholder
// hostname but must not look stopped.
func HostLooksStopped(hostName string, tags []string) bool {
	if state := StateFromTags(tags); state != "" {
		return IsStoppedState(state)
	}
	return strings.TrimSpace(hostName) == StoppedHostName
}

func ParseSyncID(syncID string) (string, error) {
	id := strings.TrimSpace(syncID)
	if !strings.HasPrefix(id, "bx_") || len(id) < 5 {
		return "", fmt.Errorf("invalid Box sync id %q", syncID)
	}
	return id, nil
}

func SnapshotAvailableFromTags(tags []string) bool {
	for _, tag := range tags {
		if tag == "snapshot" {
			return true
		}
	}
	return false
}

func StateFromTags(tags []string) string {
	for _, tag := range tags {
		if state, ok := strings.CutPrefix(tag, "state:"); ok {
			return state
		}
	}
	return ""
}
