package digitalocean

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type NewOpts struct {
	Name    string
	Region  string
	Size    string
	Image   string
	Context string
}

func DefaultNewOpts() NewOpts {
	return NewOpts{
		Region: "nyc3",
		Size:   "s-1vcpu-1gb",
		Image:  "ubuntu-24-04-x64",
	}
}

func HostLooksStopped(tags []string, status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "off" || status == "archive" {
		return true
	}
	for _, tag := range tags {
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "state:off", "state:archive":
			return true
		}
	}
	return false
}

func (c *Client) pollEvery() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return time.Second
}

func (c *Client) locate(ctx context.Context, syncID string, contextFilter []string) (string, droplet, error) {
	uuid, dropletID, err := ParseSyncID(syncID)
	if err != nil {
		return "", droplet{}, err
	}
	if err := c.CheckAvailable(ctx); err != nil {
		return "", droplet{}, err
	}
	contexts, err := c.ListContexts(ctx)
	if err != nil {
		return "", droplet{}, err
	}
	contexts = filterValues(contexts, contextFilter)
	var lastErr error
	for _, contextName := range contexts {
		acct, accountErr := c.account(ctx, contextName)
		if accountErr != nil {
			lastErr = fmt.Errorf("authenticate DigitalOcean context %q: %w; run doctl auth init --context %s", contextName, accountErr, contextName)
			continue
		}
		if scopeUUID(acct) != uuid {
			continue
		}
		item, describeErr := c.getDroplet(ctx, contextName, dropletID)
		if describeErr != nil {
			lastErr = describeErr
			continue
		}
		return contextName, item, nil
	}
	if lastErr != nil {
		return "", droplet{}, lastErr
	}
	return "", droplet{}, fmt.Errorf("no selected DigitalOcean context can access droplet %s", dropletID)
}

func (c *Client) New(ctx context.Context, opts NewOpts) (string, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return "", err
	}
	opts = applyNewDefaults(opts)
	if opts.Name == "" {
		return "", fmt.Errorf("droplet name is required")
	}
	if strings.Contains(strings.ToLower(opts.Image), "windows") {
		return "", fmt.Errorf("Windows Droplets are not supported")
	}
	contextName, err := c.resolveCreateContext(ctx, opts.Context)
	if err != nil {
		return "", err
	}
	acct, err := c.account(ctx, contextName)
	if err != nil {
		return "", err
	}
	uuid := scopeUUID(acct)
	if uuid == "" {
		return "", fmt.Errorf("account for context %s did not include a UUID", contextName)
	}
	keys, err := c.listSSHKeys(ctx, contextName)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no SSH keys on this DigitalOcean account; add one with doctl compute ssh-key import")
	}
	args := []string{
		"compute", "droplet", "create", opts.Name,
		"--region", opts.Region,
		"--size", opts.Size,
		"--image", opts.Image,
		"--wait",
		"--context", contextName,
		"--output", "json",
	}
	for _, key := range keys {
		if key.ID > 0 {
			args = append(args, "--ssh-keys", strconv.Itoa(key.ID))
		}
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("create droplet: %w", err)
	}
	created, err := parseDroplets(out)
	if err != nil {
		return "", fmt.Errorf("parse created droplet: %w", err)
	}
	if len(created) == 0 || created[0].ID == 0 {
		return "", fmt.Errorf("create droplet: no droplet id in CLI output")
	}
	return fmt.Sprintf("do:%s:%d", uuid, created[0].ID), nil
}

func applyNewDefaults(opts NewOpts) NewOpts {
	defaults := DefaultNewOpts()
	opts.Name = strings.TrimSpace(opts.Name)
	opts.Region = strings.TrimSpace(opts.Region)
	opts.Size = strings.TrimSpace(opts.Size)
	opts.Image = strings.TrimSpace(opts.Image)
	opts.Context = strings.TrimSpace(opts.Context)
	if opts.Region == "" {
		opts.Region = defaults.Region
	}
	if opts.Size == "" {
		opts.Size = defaults.Size
	}
	if opts.Image == "" {
		opts.Image = defaults.Image
	}
	return opts
}

func (c *Client) resolveCreateContext(ctx context.Context, wanted string) (string, error) {
	contexts, err := c.ListContexts(ctx)
	if err != nil {
		return "", err
	}
	if wanted != "" {
		selected := filterValues(contexts, []string{wanted})
		if len(selected) == 0 {
			return "", fmt.Errorf("no DigitalOcean context matched %q", wanted)
		}
		return selected[0], nil
	}
	if len(contexts) == 0 {
		return "", fmt.Errorf("no DigitalOcean auth contexts; run doctl auth init")
	}
	return contexts[0], nil
}

func (c *Client) Stop(ctx context.Context, syncID string) error {
	contextName, item, err := c.locate(ctx, syncID, nil)
	if err != nil {
		return err
	}
	if HostLooksStopped(nil, item.Status) {
		return nil
	}
	if _, err := c.run(ctx, "compute", "droplet-action", "power-off", strconv.Itoa(item.ID),
		"--wait", "--context", contextName, "--output", "json"); err != nil {
		return fmt.Errorf("power off droplet %s: %w", item.Name, err)
	}
	return c.waitStatus(ctx, contextName, strconv.Itoa(item.ID), "off", 2*time.Minute)
}

func (c *Client) Start(ctx context.Context, syncID string) error {
	contextName, item, err := c.locate(ctx, syncID, nil)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(item.Status), "active") {
		return nil
	}
	if _, err := c.run(ctx, "compute", "droplet-action", "power-on", strconv.Itoa(item.ID),
		"--wait", "--context", contextName, "--output", "json"); err != nil {
		return fmt.Errorf("power on droplet %s: %w", item.Name, err)
	}
	return c.waitStatus(ctx, contextName, strconv.Itoa(item.ID), "active", 3*time.Minute)
}

func (c *Client) Delete(ctx context.Context, syncID string) error {
	contextName, item, err := c.locate(ctx, syncID, nil)
	if err != nil {
		return err
	}
	if _, err := c.run(ctx, "compute", "droplet", "delete", strconv.Itoa(item.ID),
		"--force", "--context", contextName); err != nil {
		return fmt.Errorf("delete droplet %s: %w", item.Name, err)
	}
	return nil
}

func (c *Client) Fork(ctx context.Context, syncID string) (string, error) {
	contextName, item, err := c.locate(ctx, syncID, nil)
	if err != nil {
		return "", err
	}
	acct, err := c.account(ctx, contextName)
	if err != nil {
		return "", err
	}
	uuid := scopeUUID(acct)
	if uuid == "" {
		return "", fmt.Errorf("account for context %s did not include a UUID", contextName)
	}
	region := strings.TrimSpace(item.Region.Slug)
	size := strings.TrimSpace(item.SizeSlug)
	if region == "" || size == "" {
		return "", fmt.Errorf("droplet %s is missing region or size; re-sync and try again", item.Name)
	}
	snapName := fmt.Sprintf("bast-fork-%d-%d", item.ID, time.Now().Unix())
	if _, err := c.run(ctx, "compute", "droplet-action", "snapshot", strconv.Itoa(item.ID),
		"--snapshot-name", snapName, "--wait", "--context", contextName, "--output", "json"); err != nil {
		return "", fmt.Errorf("snapshot droplet %s: %w", item.Name, err)
	}
	snapID, err := c.snapshotIDByName(ctx, contextName, strconv.Itoa(item.ID), snapName)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = "droplet"
	}
	name = name + "-fork"
	out, err := c.run(ctx, "compute", "droplet", "create", name,
		"--region", region, "--size", size, "--image", snapID,
		"--wait", "--context", contextName, "--output", "json")
	if err != nil {
		return "", fmt.Errorf("create fork of %s: %w", item.Name, err)
	}
	created, err := parseDroplets(out)
	if err != nil {
		return "", fmt.Errorf("parse forked droplet: %w", err)
	}
	if len(created) == 0 || created[0].ID == 0 {
		return "", fmt.Errorf("fork droplet: no droplet id in CLI output")
	}
	return fmt.Sprintf("do:%s:%d", uuid, created[0].ID), nil
}

func (c *Client) snapshotIDByName(ctx context.Context, contextName, dropletID, name string) (string, error) {
	out, err := c.run(ctx, "compute", "droplet", "snapshots", dropletID, "--context", contextName, "--output", "json")
	if err != nil {
		return "", fmt.Errorf("list droplet snapshots: %w", err)
	}
	var snaps []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &snaps); err != nil {
		return "", fmt.Errorf("parse droplet snapshots: %w", err)
	}
	for _, snap := range snaps {
		if snap.Name == name && snap.ID > 0 {
			return strconv.Itoa(snap.ID), nil
		}
	}
	return "", fmt.Errorf("snapshot %q not found after create", name)
}

func (c *Client) waitStatus(ctx context.Context, contextName, dropletID, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for droplet %s to become %s (last state %s): %w", dropletID, want, last, lastErr)
			}
			if last == "" {
				return fmt.Errorf("timed out waiting for droplet %s to become %s", dropletID, want)
			}
			return fmt.Errorf("timed out waiting for droplet %s to become %s (last state %s)", dropletID, want, last)
		}
		item, err := c.getDroplet(ctx, contextName, dropletID)
		if err == nil {
			lastErr = nil
			last = strings.ToLower(strings.TrimSpace(item.Status))
			if last == want {
				return nil
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
