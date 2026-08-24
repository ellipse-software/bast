package fly

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type CreateOpts struct {
	App    string
	Image  string
	Region string
	Size   string
	Name   string
	Org    string
}

type ForkOpts struct {
	Region string
}

var machineIDPattern = regexp.MustCompile(`(?i)\b([a-f0-9]{14,24})\b`)

func (c *Client) Create(ctx context.Context, opts CreateOpts) (string, error) {
	app := strings.TrimSpace(opts.App)
	image := strings.TrimSpace(opts.Image)
	if app == "" {
		return "", fmt.Errorf("fly app is required")
	}
	if image == "" {
		return "", fmt.Errorf("fly image is required")
	}
	args := []string{"machine", "run", image, "--app", app}
	if region := strings.TrimSpace(opts.Region); region != "" {
		args = append(args, "--region", region)
	}
	if size := strings.TrimSpace(opts.Size); size != "" {
		args = append(args, "--vm-size", size)
	}
	if name := strings.TrimSpace(opts.Name); name != "" {
		args = append(args, "--name", name)
	}
	if org := strings.TrimSpace(opts.Org); org != "" {
		args = append(args, "--org", org)
	}
	out, err := c.runRaw(ctx, args...)
	id := lastMachineID(string(out), "")
	if err != nil && id == "" {
		return "", err
	}
	if id == "" {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("fly machine run: no machine id in CLI output")
	}
	if waitErr := c.WaitStarted(ctx, app, id, 3*time.Minute); waitErr != nil {
		return id, waitErr
	}
	return id, err
}

func (c *Client) Start(ctx context.Context, org, app, id string) error {
	app, id, err := c.resolveMachine(org, app, id)
	if err != nil {
		return err
	}
	if _, err := c.runRaw(ctx, "machine", "start", id, "--app", app); err != nil {
		return err
	}
	return c.WaitStarted(ctx, app, id, 3*time.Minute)
}

func (c *Client) Stop(ctx context.Context, org, app, id string) error {
	app, id, err := c.resolveMachine(org, app, id)
	if err != nil {
		return err
	}
	if _, err := c.runRaw(ctx, "machine", "stop", id, "--app", app); err != nil {
		return err
	}
	return c.WaitStopped(ctx, app, id, 90*time.Second)
}

func (c *Client) Destroy(ctx context.Context, org, app, id string) error {
	app, id, err := c.resolveMachine(org, app, id)
	if err != nil {
		return err
	}
	_, err = c.runRaw(ctx, "machine", "destroy", id, "--app", app, "--force")
	return err
}

func (c *Client) Clone(ctx context.Context, org, app, id string, opts ForkOpts) (string, error) {
	app, id, err := c.resolveMachine(org, app, id)
	if err != nil {
		return "", err
	}
	args := []string{"machine", "clone", id, "--app", app}
	if region := strings.TrimSpace(opts.Region); region != "" {
		args = append(args, "--region", region)
	}
	out, err := c.runRaw(ctx, args...)
	newID := lastMachineID(string(out), id)
	if err != nil && newID == "" {
		return "", err
	}
	if newID == "" {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("fly machine clone: no machine id in CLI output")
	}
	if waitErr := c.WaitStarted(ctx, app, newID, 3*time.Minute); waitErr != nil {
		return newID, waitErr
	}
	return newID, err
}

func (c *Client) resolveMachine(_, app, id string) (string, string, error) {
	if strings.Contains(id, "/") {
		_, parsedApp, parsedID, err := ParseSyncID(id)
		if err != nil {
			return "", "", err
		}
		app, id = parsedApp, parsedID
	}
	app = strings.TrimSpace(app)
	id = strings.TrimSpace(id)
	if app == "" || id == "" {
		return "", "", fmt.Errorf("fly machine %q is missing app or id", id)
	}
	return app, id, nil
}

func (c *Client) WaitStarted(ctx context.Context, app, id string, timeout time.Duration) error {
	return c.waitState(ctx, app, id, timeout, "started", func(state string) bool {
		return normalizeState(state) == "running"
	}, []string{"failed", "destroyed"})
}

func (c *Client) WaitStopped(ctx context.Context, app, id string, timeout time.Duration) error {
	return c.waitState(ctx, app, id, timeout, "stopped", func(state string) bool {
		return IsStoppedState(state)
	}, []string{"failed", "destroyed"})
}

func (c *Client) waitState(ctx context.Context, app, id string, timeout time.Duration, label string, ready func(string) bool, fail []string) error {
	deadline := time.Now().Add(timeout)
	var last string
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for fly machine %s to become %s (last state %s): %w", id, label, last, lastErr)
			}
			if last == "" {
				return fmt.Errorf("timed out waiting for fly machine %s to become %s", id, label)
			}
			return fmt.Errorf("timed out waiting for fly machine %s to become %s (last state %s)", id, label, last)
		}
		rec, err := c.machineInfo(ctx, app, id)
		if err == nil {
			lastErr = nil
			last = normalizeState(rec.State)
			if ready(rec.State) {
				return nil
			}
			for _, bad := range fail {
				if last == bad {
					return fmt.Errorf("fly machine %s entered %s state", id, last)
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

func (c *Client) machineInfo(ctx context.Context, app, id string) (machineRecord, error) {
	recs, err := c.listMachines(ctx, app)
	if err != nil {
		return machineRecord{}, err
	}
	for _, rec := range recs {
		if rec.ID == id {
			return rec, nil
		}
	}
	return machineRecord{}, fmt.Errorf("fly machine %s not found in app %s", id, app)
}

func lastMachineID(out, exclude string) string {
	matches := machineIDPattern.FindAllString(out, -1)
	exclude = strings.ToLower(strings.TrimSpace(exclude))
	for i := len(matches) - 1; i >= 0; i-- {
		id := strings.ToLower(matches[i])
		if exclude != "" && id == exclude {
			continue
		}
		return matches[i]
	}
	return ""
}
