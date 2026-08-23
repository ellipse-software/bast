package upstash

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CreateOpts struct {
	Name      string
	Runtime   string
	Size      string
	KeepAlive bool
}

func (c *Client) Create(ctx context.Context, opts CreateOpts) (BoxData, error) {
	body := map[string]any{}
	if name := strings.TrimSpace(opts.Name); name != "" {
		body["name"] = name
	}
	runtime := strings.TrimSpace(opts.Runtime)
	if runtime == "" {
		runtime = "node"
	}
	body["runtime"] = runtime
	size := strings.TrimSpace(opts.Size)
	if size == "" {
		size = "small"
	}
	body["size"] = size
	if opts.KeepAlive {
		body["keep_alive"] = true
	}
	var box BoxData
	if err := c.doJSON(ctx, http.MethodPost, "/v2/box", body, &box); err != nil {
		return BoxData{}, err
	}
	if strings.TrimSpace(box.ID) == "" {
		return BoxData{}, fmt.Errorf("upstash box create: no id in response")
	}
	if err := c.WaitReady(ctx, box.ID, 5*time.Minute); err != nil {
		return box, err
	}
	return c.Get(ctx, box.ID)
}

func (c *Client) Pause(ctx context.Context, id string) error {
	id, err := ParseSyncID(id)
	if err != nil {
		return err
	}
	info, err := c.Get(ctx, id)
	if err != nil {
		return err
	}
	if info.KeepAlive {
		return fmt.Errorf("upstash box %s is keep-alive and cannot be paused", id)
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v2/box/"+url.PathEscape(id)+"/pause", map[string]any{}, nil); err != nil {
		return err
	}
	return c.WaitPaused(ctx, id, 3*time.Minute)
}

func (c *Client) Resume(ctx context.Context, id string) error {
	id, err := ParseSyncID(id)
	if err != nil {
		return err
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v2/box/"+url.PathEscape(id)+"/resume", map[string]any{}, nil); err != nil {
		return err
	}
	return c.WaitReady(ctx, id, 5*time.Minute)
}

func (c *Client) Delete(ctx context.Context, id string) error {
	id, err := ParseSyncID(id)
	if err != nil {
		return err
	}
	return c.doJSON(ctx, http.MethodDelete, "/v2/box/"+url.PathEscape(id), nil, nil)
}

func (c *Client) Snapshot(ctx context.Context, id, name string) (Snapshot, error) {
	id, err := ParseSyncID(id)
	if err != nil {
		return Snapshot{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = "bast"
	}
	var snap Snapshot
	if err := c.doJSON(ctx, http.MethodPost, "/v2/box/"+url.PathEscape(id)+"/snapshots", map[string]any{"name": name}, &snap); err != nil {
		return Snapshot{}, err
	}
	if strings.TrimSpace(snap.ID) == "" {
		return Snapshot{}, fmt.Errorf("upstash snapshot: no id in response")
	}
	if err := c.waitSnapshot(ctx, id, snap.ID, 5*time.Minute); err != nil {
		return snap, err
	}
	return snap, nil
}

func (c *Client) ListSnapshots(ctx context.Context, id string) ([]Snapshot, error) {
	id, err := ParseSyncID(id)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Snapshots []Snapshot `json:"snapshots"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v2/box/"+url.PathEscape(id)+"/snapshots", nil, &raw); err != nil {
		return nil, err
	}
	return raw.Snapshots, nil
}

func (c *Client) FromSnapshot(ctx context.Context, snapshotID string, opts CreateOpts) (BoxData, error) {
	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return BoxData{}, fmt.Errorf("snapshot id is required")
	}
	body := map[string]any{"snapshot_id": snapshotID}
	if name := strings.TrimSpace(opts.Name); name != "" {
		body["name"] = name
	}
	if runtime := strings.TrimSpace(opts.Runtime); runtime != "" {
		body["runtime"] = runtime
	}
	if size := strings.TrimSpace(opts.Size); size != "" {
		body["size"] = size
	}
	if opts.KeepAlive {
		body["keep_alive"] = true
	}
	var box BoxData
	if err := c.doJSON(ctx, http.MethodPost, "/v2/box/from-snapshot", body, &box); err != nil {
		return BoxData{}, err
	}
	if strings.TrimSpace(box.ID) == "" {
		return BoxData{}, fmt.Errorf("upstash from-snapshot: no id in response")
	}
	if err := c.WaitReady(ctx, box.ID, 5*time.Minute); err != nil {
		return box, err
	}
	return c.Get(ctx, box.ID)
}

func (c *Client) Fork(ctx context.Context, id string) (string, error) {
	src, err := c.Get(ctx, id)
	if err != nil {
		return "", err
	}
	snap, err := c.Snapshot(ctx, src.ID, "bast-fork")
	if err != nil {
		return "", err
	}
	box, err := c.FromSnapshot(ctx, snap.ID, CreateOpts{
		Size:      src.Size,
		Runtime:   src.Runtime,
		KeepAlive: src.KeepAlive,
	})
	if err != nil {
		return box.ID, err
	}
	return box.ID, nil
}

func (c *Client) WaitReady(ctx context.Context, id string, timeout time.Duration) error {
	return c.waitStatus(ctx, id, "ready", timeout, isReadyState, []string{"error", "deleted"})
}

func (c *Client) WaitPaused(ctx context.Context, id string, timeout time.Duration) error {
	return c.waitStatus(ctx, id, "paused", timeout, func(status string) bool {
		return normalizeState(status) == "paused"
	}, []string{"error", "deleted"})
}

func (c *Client) waitStatus(ctx context.Context, id, timeoutLabel string, timeout time.Duration, ready func(string) bool, fail []string) error {
	deadline := time.Now().Add(timeout)
	var last string
	var lastErr error
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for upstash box %s to become %s (last state %s): %w", id, timeoutLabel, last, lastErr)
			}
			if last == "" {
				return fmt.Errorf("timed out waiting for upstash box %s to become %s", id, timeoutLabel)
			}
			return fmt.Errorf("timed out waiting for upstash box %s to become %s (last state %s)", id, timeoutLabel, last)
		}
		box, err := c.Get(ctx, id)
		if err == nil {
			lastErr = nil
			last = normalizeState(box.Status)
			if ready(box.Status) {
				return nil
			}
			for _, bad := range fail {
				if last == bad {
					return fmt.Errorf("upstash box %s entered %s state", id, last)
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

func (c *Client) waitSnapshot(ctx context.Context, boxID, snapshotID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			if last == "" {
				return fmt.Errorf("timed out waiting for snapshot %s", snapshotID)
			}
			return fmt.Errorf("timed out waiting for snapshot %s (last state %s)", snapshotID, last)
		}
		snaps, err := c.ListSnapshots(ctx, boxID)
		if err == nil {
			for _, snap := range snaps {
				if snap.ID != snapshotID {
					continue
				}
				last = strings.ToLower(strings.TrimSpace(snap.Status))
				if last == "ready" {
					return nil
				}
				if last == "error" || last == "deleted" {
					return fmt.Errorf("snapshot %s entered %s state", snapshotID, last)
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pollEvery()):
		}
	}
}
