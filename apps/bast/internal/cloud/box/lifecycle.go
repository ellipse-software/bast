package box

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// errMissingActionID means the CLI response did not include an action/box id.
// Stop/Resume may still have started, so callers can continue waiting.
var errMissingActionID = errors.New("missing action id")

type NewOpts struct {
	Type       string // small|default|large
	TTLSeconds int    // 0 = CLI default; ignored when NoAutoStop
	NoAutoStop bool
	NoEnv      bool
}

type ForkOpts struct {
	Type  string
	NoEnv bool
}

type ResumeOpts struct {
	Type  string
	NoEnv bool
}

type ActionResult struct {
	ID     string
	Status string
}

func (c *Client) pollEvery() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return time.Second
}

func (c *Client) New(ctx context.Context, opts NewOpts) (string, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return "", err
	}
	args := []string{"new"}
	if t := strings.TrimSpace(opts.Type); t != "" && t != "default" {
		args = append(args, "--type", t)
	}
	if opts.NoAutoStop {
		args = append(args, "--no-auto-stop")
	} else if opts.TTLSeconds > 0 {
		args = append(args, "--ttl", fmt.Sprintf("%d", opts.TTLSeconds))
	}
	if opts.NoEnv {
		args = append(args, "--no-env")
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	id, err := parseNewJSONL(out)
	if err != nil {
		return id, err
	}
	if err := c.WaitReady(ctx, id, 3*time.Minute); err != nil {
		return id, err
	}
	return id, nil
}

func parseNewJSONL(out []byte) (string, error) {
	var id string
	var lastErr string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event struct {
			Event string `json:"event"`
			ID    string `json:"id"`
			Error string `json:"error"`
			Ok    *bool  `json:"ok"`
			Box   *struct {
				ID string `json:"id"`
			} `json:"box"`
			Type   string `json:"type"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.ID != "" {
			id = event.ID
		}
		if event.Box != nil && event.Box.ID != "" {
			id = event.Box.ID
		}
		switch event.Event {
		case "ready":
			if id != "" {
				return id, nil
			}
		case "error":
			if event.Error != "" {
				lastErr = event.Error
			} else {
				lastErr = line
			}
		}
		if event.Type == "box.created" && id != "" {
			// Async create accepted; caller will WaitReady.
			continue
		}
		if event.Ok != nil && !*event.Ok && event.Error != "" {
			lastErr = event.Error
		}
	}
	if id != "" && lastErr == "" {
		return id, nil
	}
	if lastErr != "" {
		return id, fmt.Errorf("box new: %s", lastErr)
	}
	// Single JSON object fallback (non-JSONL).
	var single struct {
		ID  string `json:"id"`
		Box *struct {
			ID string `json:"id"`
		} `json:"box"`
	}
	if err := json.Unmarshal(out, &single); err == nil {
		if single.ID != "" {
			return single.ID, nil
		}
		if single.Box != nil && single.Box.ID != "" {
			return single.Box.ID, nil
		}
	}
	return "", fmt.Errorf("box new: no box id in CLI output")
}

func (c *Client) Fork(ctx context.Context, id string, opts ForkOpts) (string, error) {
	id, err := ParseSyncID(id)
	if err != nil {
		return "", err
	}
	info, err := c.Info(ctx, id)
	if err != nil {
		return "", err
	}
	if !info.SnapshotAvailable {
		return "", fmt.Errorf("box %s has no snapshot yet; stop it once to create one before forking", id)
	}
	args := []string{"fork", id}
	if t := strings.TrimSpace(opts.Type); t != "" {
		args = append(args, "--type", t)
	}
	if opts.NoEnv {
		args = append(args, "--no-env")
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	newID, err := parseActionID(out, "fork")
	if err != nil {
		return "", err
	}
	if err := c.WaitReady(ctx, newID, 3*time.Minute); err != nil {
		return newID, err
	}
	return newID, nil
}

func (c *Client) Stop(ctx context.Context, id string) error {
	id, err := ParseSyncID(id)
	if err != nil {
		return err
	}
	out, err := c.run(ctx, "stop", id)
	if err != nil {
		return err
	}
	if _, err := parseActionID(out, "stop"); err != nil && !errors.Is(err, errMissingActionID) {
		return err
	}
	return c.WaitStopped(ctx, id, 5*time.Minute)
}

func (c *Client) Resume(ctx context.Context, id string, opts ResumeOpts) error {
	id, err := ParseSyncID(id)
	if err != nil {
		return err
	}
	args := []string{"resume", id}
	if t := strings.TrimSpace(opts.Type); t != "" {
		args = append(args, "--type", t)
	}
	if opts.NoEnv {
		args = append(args, "--no-env")
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return err
	}
	if _, err := parseActionID(out, "resume"); err != nil && !errors.Is(err, errMissingActionID) {
		return err
	}
	return c.WaitReady(ctx, id, 3*time.Minute)
}

func parseActionID(out []byte, action string) (string, error) {
	var raw struct {
		ID  string `json:"id"`
		Box *struct {
			ID string `json:"id"`
		} `json:"box"`
		Ok    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", fmt.Errorf("parse box %s: %w", action, err)
	}
	if raw.Ok != nil && !*raw.Ok {
		msg := raw.Error
		if msg == "" {
			msg = string(out)
		}
		return "", fmt.Errorf("box %s: %s", action, msg)
	}
	if raw.ID != "" {
		return raw.ID, nil
	}
	if raw.Box != nil && raw.Box.ID != "" {
		return raw.Box.ID, nil
	}
	return "", fmt.Errorf("box %s: %w", action, errMissingActionID)
}

func (c *Client) WaitReady(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if lastState == "" && lastErr != nil {
				return fmt.Errorf("timed out waiting for box %s to become ready: %w", id, lastErr)
			}
			if lastState == "" {
				return fmt.Errorf("timed out waiting for box %s to become ready", id)
			}
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for box %s to become ready (last state %s): %w", id, lastState, lastErr)
			}
			return fmt.Errorf("timed out waiting for box %s to become ready (last state %s)", id, lastState)
		}
		info, err := c.Info(ctx, id)
		if err == nil {
			lastErr = nil
			lastState = info.State
			if info.Running && info.HostName != "" && info.HostName != stoppedHostName {
				return nil
			}
			if info.State == "error" {
				return fmt.Errorf("box %s entered error state", id)
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pollEvery()):
		}
	}
}

func (c *Client) WaitStopped(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if lastState == "" && lastErr != nil {
				return fmt.Errorf("timed out waiting for box %s to stop: %w", id, lastErr)
			}
			if lastState == "" {
				return fmt.Errorf("timed out waiting for box %s to stop", id)
			}
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for box %s to stop (last state %s): %w", id, lastState, lastErr)
			}
			return fmt.Errorf("timed out waiting for box %s to stop (last state %s)", id, lastState)
		}
		info, err := c.Info(ctx, id)
		if err == nil {
			lastErr = nil
			lastState = info.State
			if IsTerminalStoppedState(info.State) {
				return nil
			}
			if info.State == "error" {
				return fmt.Errorf("box %s entered error state while stopping", id)
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pollEvery()):
		}
	}
}
