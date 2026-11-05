package boxd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CreateOpts struct {
	OrgID    string
	Region   string
	Name     string
	VCPU     int
	Snapshot string
}

type ForkOpts struct {
	Name string
}

// Created is a new computer. HostName is its domain, which is unique, so a
// caller can find the synced host even when it did not pick the region.
type Created struct {
	Name     string
	HostName string
}

type Machine struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	URL         string `json:"url"`
	SSHDisabled bool   `json:"ssh_disabled"`
}

type createRecord struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

const (
	startTimeout  = 3 * time.Minute
	settleTimeout = 2 * time.Minute
)

// Create makes a computer. A zero VCPU takes the org's default size. A computer
// from a snapshot keeps the snapshot's size, so it takes no VCPU.
func (c *Client) Create(ctx context.Context, opts CreateOpts) (Created, error) {
	orgID := strings.TrimSpace(opts.OrgID)
	if orgID == "" {
		return Created{}, fmt.Errorf("boxd org is required")
	}
	if err := validateValue("org", orgID); err != nil {
		return Created{}, err
	}
	snapshot := strings.TrimSpace(opts.Snapshot)
	if snapshot != "" && opts.VCPU != 0 {
		return Created{}, fmt.Errorf("a boxd computer from a snapshot keeps the snapshot's size; leave the size blank")
	}
	args := []string{"machine", "new"}
	if name := strings.TrimSpace(opts.Name); name != "" {
		if err := validateValue("name", name); err != nil {
			return Created{}, err
		}
		args = append(args, name)
	}
	args = append(args, "--org", orgID)
	if region := strings.TrimSpace(opts.Region); region != "" {
		if err := validateValue("region", region); err != nil {
			return Created{}, err
		}
		args = append(args, "--region", region)
	}
	if opts.VCPU != 0 {
		switch opts.VCPU {
		case 1, 2, 4, 8:
		default:
			return Created{}, fmt.Errorf("boxd vCPU must be 1, 2, 4, or 8")
		}
		args = append(args, "--vcpu", strconv.Itoa(opts.VCPU))
	}
	if snapshot != "" {
		if err := validateValue("snapshot", snapshot); err != nil {
			return Created{}, err
		}
		args = append(args, "--from-snapshot", snapshot)
	}
	out, err := c.run(ctx, append(args, "--json")...)
	if err != nil {
		return Created{}, err
	}
	var rec createRecord
	if err := decodeFirstJSON(out, &rec); err != nil || strings.TrimSpace(rec.Name) == "" {
		return Created{}, fmt.Errorf("boxd machine new: no computer in CLI output")
	}
	created := Created{Name: strings.TrimSpace(rec.Name), HostName: hostFromURL(rec.URL)}
	if region := strings.TrimSpace(opts.Region); region != "" {
		if err := c.WaitRunning(ctx, orgID, region, created.Name, startTimeout); err != nil {
			return created, err
		}
	}
	return created, nil
}

// Fork copies a computer, its disk, memory, and running processes, in its own
// region. The source must not be mid-reboot: a fork taken then comes up failed.
func (c *Client) Fork(ctx context.Context, syncID string, opts ForkOpts) (Created, error) {
	orgID, region, name, err := ParseSyncID(syncID)
	if err != nil {
		return Created{}, err
	}
	if err := c.waitSettled(ctx, orgID, region, name, settleTimeout); err != nil {
		return Created{}, err
	}
	args := []string{"machine", "fork", name}
	if forkName := strings.TrimSpace(opts.Name); forkName != "" {
		if err := validateValue("name", forkName); err != nil {
			return Created{}, err
		}
		args = append(args, forkName)
	}
	args = append(args, "--org", orgID, "--region", region, "--json")
	out, err := c.run(ctx, args...)
	if err != nil {
		return Created{}, err
	}
	var rec createRecord
	if err := decodeFirstJSON(out, &rec); err != nil || strings.TrimSpace(rec.Name) == "" {
		return Created{}, fmt.Errorf("boxd machine fork: no computer in CLI output")
	}
	created := Created{Name: strings.TrimSpace(rec.Name), HostName: hostFromURL(rec.URL)}
	if err := c.WaitRunning(ctx, orgID, region, created.Name, startTimeout); err != nil {
		return created, err
	}
	return created, nil
}

// Stop shuts a computer down. Its disk is kept; memory and processes are not.
func (c *Client) Stop(ctx context.Context, syncID string) error {
	orgID, region, name, err := ParseSyncID(syncID)
	if err != nil {
		return err
	}
	machine, err := c.Get(ctx, orgID, region, name)
	if err != nil {
		return err
	}
	if normalizeState(machine.Status) == "stopped" {
		return nil
	}
	if _, err := c.run(ctx, "machine", "stop", name, "--org", orgID, "--region", region, "--json"); err != nil {
		return err
	}
	return c.waitState(ctx, orgID, region, name, settleTimeout, "stopped", func(state string) bool {
		return state == "stopped"
	})
}

// Start boots a stopped computer, wakes a hibernated one, and resumes one on
// standby, then waits until it runs.
func (c *Client) Start(ctx context.Context, syncID string) error {
	orgID, region, name, err := ParseSyncID(syncID)
	if err != nil {
		return err
	}
	machine, err := c.Get(ctx, orgID, region, name)
	if err != nil {
		return err
	}
	verb := ""
	switch normalizeState(machine.Status) {
	case "running":
		return nil
	case "hibernated":
		verb = "wake"
	case "standby":
		verb = "resume"
	case "starting", "rebooting", "waking", "resuming":
	default:
		verb = "start"
	}
	if verb != "" {
		if _, err := c.run(ctx, "machine", verb, name, "--org", orgID, "--region", region, "--json"); err != nil {
			return err
		}
	}
	return c.WaitRunning(ctx, orgID, region, name, startTimeout)
}

func (c *Client) Restart(ctx context.Context, syncID string) error {
	orgID, region, name, err := ParseSyncID(syncID)
	if err != nil {
		return err
	}
	if _, err := c.run(ctx, "machine", "reboot", name, "--org", orgID, "--region", region, "--json"); err != nil {
		return err
	}
	return c.WaitRunning(ctx, orgID, region, name, startTimeout)
}

func (c *Client) Delete(ctx context.Context, syncID string) error {
	orgID, region, name, err := ParseSyncID(syncID)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "machine", "remove", name, "--org", orgID, "--region", region, "-y", "--json")
	if isNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) Get(ctx context.Context, orgID, region, name string) (Machine, error) {
	out, err := c.run(ctx, "machine", "get", name, "--org", orgID, "--region", region, "--json")
	if err != nil {
		return Machine{}, err
	}
	var machine Machine
	if err := decodeFirstJSON(out, &machine); err != nil {
		return Machine{}, fmt.Errorf("parse boxd machine get: %w", err)
	}
	if strings.TrimSpace(machine.Name) == "" {
		return Machine{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return machine, nil
}

func (c *Client) WaitRunning(ctx context.Context, orgID, region, name string, timeout time.Duration) error {
	return c.waitState(ctx, orgID, region, name, timeout, "running", func(state string) bool {
		return state == "running"
	})
}

// waitSettled waits out a start or reboot, so a fork does not copy a
// computer that is booting.
func (c *Client) waitSettled(ctx context.Context, orgID, region, name string, timeout time.Duration) error {
	return c.waitState(ctx, orgID, region, name, timeout, "settled", func(state string) bool {
		return !isTransitionalState(state)
	})
}

func (c *Client) waitState(ctx context.Context, orgID, region, name string, timeout time.Duration, label string, ready func(string) bool) error {
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		machine, err := c.Get(ctx, orgID, region, name)
		if err == nil {
			last = normalizeState(machine.Status)
			if ready(last) {
				return nil
			}
			if last == "failed" || last == "destroyed" {
				return fmt.Errorf("boxd computer %s is %s", name, last)
			}
		} else if isNotLoggedIn(err) || isNotFound(err) {
			return err
		}
		if time.Now().After(deadline) {
			if last == "" && err != nil {
				return fmt.Errorf("timed out waiting for boxd computer %s to be %s: %w", name, label, err)
			}
			return fmt.Errorf("timed out waiting for boxd computer %s to be %s (last state %s)", name, label, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.pollEvery()):
		}
	}
}
