package hetzner

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (c *Client) Start(ctx context.Context, syncID string) error {
	token, srv, err := c.lookup(ctx, syncID)
	if err != nil {
		return err
	}
	if normalizeState(srv.Status) == "running" {
		return nil
	}
	if err := c.runAction(ctx, token, srv.ID, "poweron"); err != nil {
		return err
	}
	return c.waitGuest(ctx, token, srv.ID, "running", false)
}

func (c *Client) Stop(ctx context.Context, syncID string, force bool) error {
	token, srv, err := c.lookup(ctx, syncID)
	if err != nil {
		return err
	}
	if normalizeState(srv.Status) == "off" {
		return nil
	}
	command := "shutdown"
	if force {
		command = "poweroff"
	}
	if err := c.runAction(ctx, token, srv.ID, command); err != nil {
		return err
	}
	if err := c.waitGuest(ctx, token, srv.ID, "off", false); err != nil {
		if !force {
			return fmt.Errorf("%w; ACPI shutdown did not power the server off (the guest may ignore it). Retry with --force for a hard poweroff", err)
		}
		return err
	}
	return nil
}

func (c *Client) Restart(ctx context.Context, syncID string, force bool) error {
	token, srv, err := c.lookup(ctx, syncID)
	if err != nil {
		return err
	}
	if IsStoppedState(srv.Status) {
		return fmt.Errorf("server %s is off; start it first", displayName(srv))
	}
	if force {
		if err := c.runAction(ctx, token, srv.ID, "reset"); err != nil {
			return err
		}
		return c.waitGuest(ctx, token, srv.ID, "running", false)
	}
	if err := c.runAction(ctx, token, srv.ID, "reboot"); err != nil {
		return err
	}
	if err := c.waitGuest(ctx, token, srv.ID, "running", true); err != nil {
		return fmt.Errorf("%w; ACPI reboot did not restart the server. Retry with --force for a hard reset", err)
	}
	return nil
}

func (c *Client) lookup(ctx context.Context, syncID string) (string, apiServer, error) {
	id, err := ParseSyncID(syncID)
	if err != nil {
		return "", apiServer{}, err
	}
	contexts, err := c.TokenContexts()
	if err != nil {
		return "", apiServer{}, err
	}
	if len(contexts) == 0 {
		return "", apiServer{}, fmt.Errorf("no API token; connect on the Sync tab, set %s, or run bast hetzner key", APIKeyEnv)
	}
	var lastErr error
	for _, tokenCtx := range contexts {
		srv, getErr := c.getServer(ctx, tokenCtx.Token, id)
		if getErr != nil {
			lastErr = getErr
			continue
		}
		return tokenCtx.Token, srv, nil
	}
	if lastErr != nil {
		return "", apiServer{}, lastErr
	}
	return "", apiServer{}, fmt.Errorf("no configured Hetzner token can access server %d", id)
}

func (c *Client) runAction(ctx context.Context, token string, serverID int, command string) error {
	var out struct {
		Action apiAction `json:"action"`
	}
	if err := c.doJSON(ctx, token, http.MethodPost, "/servers/"+strconv.Itoa(serverID)+"/actions/"+command, nil, nil, &out); err != nil {
		return err
	}
	if out.Action.ID == 0 {
		if strings.EqualFold(out.Action.Status, "success") {
			return nil
		}
		return fmt.Errorf("hetzner %s returned no action id", command)
	}
	return c.waitAction(ctx, token, out.Action.ID)
}

func (c *Client) waitAction(ctx context.Context, token string, actionID int) error {
	for {
		var out struct {
			Action apiAction `json:"action"`
		}
		if err := c.doJSON(ctx, token, http.MethodGet, "/actions/"+strconv.Itoa(actionID), nil, nil, &out); err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(out.Action.Status)) {
		case "success":
			return nil
		case "error":
			msg := "action failed"
			if out.Action.Error != nil && out.Action.Error.Message != "" {
				msg = out.Action.Error.Message
			}
			return fmt.Errorf("hetzner action %d: %s", actionID, msg)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pollEvery()):
		}
	}
}

func (c *Client) waitGuest(ctx context.Context, token string, serverID int, want string, leaveRunningFirst bool) error {
	want = normalizeState(want)
	leftRunning := !leaveRunningFirst
	for {
		srv, err := c.getServer(ctx, token, serverID)
		if err != nil {
			return err
		}
		status := normalizeState(srv.Status)
		if leaveRunningFirst && !leftRunning {
			if status != "running" {
				leftRunning = true
			}
		} else if status == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for server %d to become %s (last status %s)", serverID, want, status)
		case <-time.After(c.pollEvery()):
		}
	}
}
