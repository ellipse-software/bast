package box

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type EnsureConfig struct {
	Home      string
	SSHKeygen string
	Status    func(string)
}

type EnsureResult struct {
	User           string
	HostName       string
	IdentityFile   string
	IdentitiesOnly bool
	KeyAdded       bool
}

func reportStatus(status func(string), message string) {
	if status != nil && strings.TrimSpace(message) != "" {
		status(message)
	}
}

func (c *Client) EnsureAccess(ctx context.Context, syncID string, cfg EnsureConfig) (EnsureResult, error) {
	id, err := ParseSyncID(syncID)
	if err != nil {
		return EnsureResult{}, err
	}
	if err := c.CheckAvailable(ctx); err != nil {
		return EnsureResult{}, err
	}
	reportStatus(cfg.Status, "Checking Box access…")
	info, err := c.Info(ctx, id)
	if err != nil {
		return EnsureResult{}, err
	}
	if !info.Running {
		if IsStoppedState(info.State) {
			return EnsureResult{}, fmt.Errorf("box %s is stopped; resume it first with bast box resume %s", id, id)
		}
		return EnsureResult{}, fmt.Errorf("box %s is not ready for SSH (state %s)", id, info.State)
	}
	if info.HostName == "" || info.HostName == stoppedHostName {
		return EnsureResult{}, fmt.Errorf("box %s has no IP yet; wait a moment and retry", id)
	}
	home := cfg.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if err := ensureBoxIdentity(ctx, home, cfg.SSHKeygen, cfg.Status); err != nil {
		return EnsureResult{}, err
	}
	reportStatus(cfg.Status, "Authorizing SSH key on Box…")
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return EnsureResult{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 800 * time.Millisecond):
			}
			info, err = c.Info(ctx, id)
			if err != nil {
				lastErr = err
				continue
			}
			if info.HostName == "" || info.HostName == stoppedHostName {
				lastErr = fmt.Errorf("box %s has no IP yet", id)
				continue
			}
		}
		if err := c.authorizeKey(ctx, id, home); err != nil {
			lastErr = err
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "box_restoring") || strings.Contains(lower, "restoring") {
				reportStatus(cfg.Status, "Box is restoring; retrying…")
				continue
			}
			return EnsureResult{}, err
		}
		return EnsureResult{
			User:           SSHUser,
			HostName:       info.HostName,
			IdentityFile:   IdentityFile,
			IdentitiesOnly: true,
			KeyAdded:       true,
		}, nil
	}
	if lastErr != nil {
		return EnsureResult{}, lastErr
	}
	return EnsureResult{}, fmt.Errorf("could not authorize SSH access to box %s", id)
}

func (c *Client) Info(ctx context.Context, id string) (Instance, error) {
	out, err := c.run(ctx, "info", id)
	if err != nil {
		return Instance{}, err
	}
	var raw struct {
		Box boxRecord `json:"box"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		// Some CLI versions may return the box object at the top level.
		var direct boxRecord
		if err2 := json.Unmarshal(out, &direct); err2 != nil || direct.ID == "" {
			return Instance{}, fmt.Errorf("parse box info: %w", err)
		}
		raw.Box = direct
	}
	inst, ok := instanceFromRecord(raw.Box)
	if !ok {
		return Instance{}, fmt.Errorf("box %s info was incomplete", id)
	}
	return inst, nil
}

func (c *Client) authorizeKey(ctx context.Context, id, home string) error {
	// Prefer a no-op SSH session so the CLI creates/registers ~/.ssh/ascii_box_ed25519.
	_, err := c.run(ctx, "ssh", id, "--", "true")
	if err == nil {
		return nil
	}
	pubPath := filepath.Join(home, ".ssh", "ascii_box_ed25519.pub")
	pub, readErr := os.ReadFile(pubPath)
	if readErr != nil {
		return err
	}
	key := strings.TrimSpace(string(pub))
	if key == "" {
		return err
	}
	// Fallback: the CLI has no key-upload command; retry ssh once more now that the key exists.
	_, retryErr := c.run(ctx, "ssh", id, "--", "true")
	if retryErr != nil {
		return fmt.Errorf("%v (public key ready at %s)", err, pubPath)
	}
	return nil
}

func ensureBoxIdentity(ctx context.Context, home, sshKeygen string, status func(string)) error {
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	priv := filepath.Join(dir, "ascii_box_ed25519")
	pub := priv + ".pub"
	if _, err := os.Stat(priv); err == nil {
		if _, err := os.Stat(pub); err == nil {
			return nil
		}
		bin := sshKeygen
		if bin == "" {
			bin = "ssh-keygen"
		}
		out, err := exec.CommandContext(ctx, bin, "-y", "-f", priv, "-P", "").Output()
		if err != nil {
			return fmt.Errorf("derive Box public key: %w", err)
		}
		return os.WriteFile(pub, out, 0644)
	}
	reportStatus(status, "Generating Box SSH key (~/.ssh/ascii_box_ed25519)…")
	bin := sshKeygen
	if bin == "" {
		bin = "ssh-keygen"
	}
	cmd := exec.CommandContext(ctx, bin, "-t", "ed25519", "-f", priv, "-N", "", "-C", "bast-box")
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("generate Box SSH key: %s", msg)
	}
	return nil
}
