package vercel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CreateOpts struct {
	Name       string
	VCPUs      int
	Timeout    time.Duration
	Persistent bool
}

func (c *Client) Create(ctx context.Context, opts CreateOpts) (Sandbox, error) {
	project, err := c.requireProject()
	if err != nil {
		return Sandbox{}, err
	}
	vcpus := opts.VCPUs
	if vcpus <= 0 {
		vcpus = 2
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = time.Hour
	}
	body := map[string]any{
		"projectId":  project,
		"runtime":    "node24",
		"persistent": opts.Persistent,
		"timeout":    timeout.Milliseconds(),
		"resources":  map[string]any{"vcpus": vcpus},
	}
	if name := strings.TrimSpace(opts.Name); name != "" {
		if _, _, err := ParseSyncID(name); err != nil || strings.Contains(name, "/") {
			return Sandbox{}, fmt.Errorf("sandbox name must be URL-safe (letters, numbers, hyphens, underscores)")
		}
		body["name"] = name
	}
	var raw sandboxSessionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v2/sandboxes", nil, body, &raw); err != nil {
		return Sandbox{}, err
	}
	if strings.TrimSpace(raw.Sandbox.Name) == "" {
		return Sandbox{}, fmt.Errorf("vercel sandbox create: no name in response")
	}
	if err := c.WaitReady(ctx, SyncID(project, raw.Sandbox.Name), 5*time.Minute); err != nil {
		return raw.Sandbox, err
	}
	got, err := c.Get(ctx, SyncID(project, raw.Sandbox.Name), false)
	if err != nil {
		return raw.Sandbox, err
	}
	return got.Sandbox, nil
}

func (c *Client) Stop(ctx context.Context, syncID string) error {
	info, err := c.Get(ctx, syncID, false)
	if err != nil {
		return err
	}
	if IsStoppedState(info.Sandbox.Status) {
		return nil
	}
	sessionID := strings.TrimSpace(info.Sandbox.CurrentSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(info.Session.ID)
	}
	if sessionID == "" {
		return fmt.Errorf("vercel sandbox %s has no session to stop", info.Sandbox.Name)
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v2/sandboxes/sessions/"+url.PathEscape(sessionID)+"/stop", nil, map[string]any{}, nil); err != nil {
		return err
	}
	return c.WaitStopped(ctx, syncID, 3*time.Minute)
}

func (c *Client) Resume(ctx context.Context, syncID string) error {
	if _, err := c.Get(ctx, syncID, true); err != nil {
		return err
	}
	return c.WaitReady(ctx, syncID, 5*time.Minute)
}

func (c *Client) Delete(ctx context.Context, syncID string) error {
	project, name, err := c.parseScopedID(syncID)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("projectId", project)
	return c.doJSON(ctx, http.MethodDelete, "/v2/sandboxes/"+url.PathEscape(name), query, nil, nil)
}

func (c *Client) Fork(ctx context.Context, syncID string, name string) (string, error) {
	project, source, err := c.parseScopedID(syncID)
	if err != nil {
		return "", err
	}
	body := map[string]any{}
	if n := strings.TrimSpace(name); n != "" {
		if _, _, parseErr := ParseSyncID(n); parseErr != nil || strings.Contains(n, "/") {
			return "", fmt.Errorf("sandbox name must be URL-safe (letters, numbers, hyphens, underscores)")
		}
		body["name"] = n
	}
	query := url.Values{}
	query.Set("projectId", project)
	var raw sandboxSessionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v2/sandboxes/"+url.PathEscape(source)+"/fork", query, body, &raw); err != nil {
		return "", err
	}
	if strings.TrimSpace(raw.Sandbox.Name) == "" {
		return "", fmt.Errorf("vercel sandbox fork: no name in response")
	}
	id := SyncID(project, raw.Sandbox.Name)
	if err := c.WaitReady(ctx, id, 5*time.Minute); err != nil {
		return id, err
	}
	return id, nil
}

func (c *Client) WaitReady(ctx context.Context, syncID string, timeout time.Duration) error {
	return c.waitStatus(ctx, syncID, "ready", timeout, isReadyState, []string{"failed", "aborted"})
}

func (c *Client) WaitStopped(ctx context.Context, syncID string, timeout time.Duration) error {
	return c.waitStatus(ctx, syncID, "stopped", timeout, IsStoppedState, []string{"failed", "aborted"})
}

func (c *Client) waitStatus(ctx context.Context, syncID, timeoutLabel string, timeout time.Duration, ready func(string) bool, fail []string) error {
	deadline := time.Now().Add(timeout)
	var last string
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for vercel sandbox %s to become %s (last state %s): %w", syncID, timeoutLabel, last, lastErr)
			}
			if last == "" {
				return fmt.Errorf("timed out waiting for vercel sandbox %s to become %s", syncID, timeoutLabel)
			}
			return fmt.Errorf("timed out waiting for vercel sandbox %s to become %s (last state %s)", syncID, timeoutLabel, last)
		}
		info, err := c.Get(ctx, syncID, false)
		if err == nil {
			lastErr = nil
			last = normalizeState(info.Sandbox.Status)
			if ready(info.Sandbox.Status) {
				return nil
			}
			for _, bad := range fail {
				if last == bad {
					return fmt.Errorf("vercel sandbox %s entered %s state", syncID, last)
				}
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

func ParseTimeoutFlag(value string) (time.Duration, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1h", "60m":
		return time.Hour, nil
	case "15m":
		return 15 * time.Minute, nil
	case "5h":
		return 5 * time.Hour, nil
	default:
		return 0, fmt.Errorf("timeout must be 15m, 1h, or 5h")
	}
}

func ValidateVCPUs(n int) error {
	switch n {
	case 1, 2, 4:
		return nil
	default:
		return fmt.Errorf("vcpus must be 1, 2, or 4")
	}
}
