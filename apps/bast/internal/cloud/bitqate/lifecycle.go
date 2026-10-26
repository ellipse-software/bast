package bitqate

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (c *Client) Start(ctx context.Context, syncID string) error {
	vm, err := c.lookup(ctx, syncID)
	if err != nil {
		return err
	}
	if normalizeState(vm.Status) == "running" {
		return nil
	}
	if IsSuspendedState(vm.Status) {
		return fmt.Errorf("VM %s is suspended (billing-locked); renew it in the BitQate dashboard first", vmName(vm))
	}
	if err := c.vmAction(ctx, vm.ID, "start"); err != nil {
		return err
	}
	return c.waitVM(ctx, vm.ID, "running")
}

func (c *Client) Stop(ctx context.Context, syncID string) error {
	vm, err := c.lookup(ctx, syncID)
	if err != nil {
		return err
	}
	if IsStoppedState(vm.Status) {
		return nil
	}
	if err := c.vmAction(ctx, vm.ID, "stop"); err != nil {
		return err
	}
	return c.waitVM(ctx, vm.ID, "stopped")
}

func (c *Client) Restart(ctx context.Context, syncID string) error {
	vm, err := c.lookup(ctx, syncID)
	if err != nil {
		return err
	}
	if IsSuspendedState(vm.Status) {
		return fmt.Errorf("VM %s is suspended (billing-locked); renew it in the BitQate dashboard first", vmName(vm))
	}
	if IsStoppedState(vm.Status) {
		return fmt.Errorf("VM %s is stopped; start it first", vmName(vm))
	}
	if err := c.vmAction(ctx, vm.ID, "restart"); err != nil {
		return err
	}
	return c.waitVM(ctx, vm.ID, "running")
}

func (c *Client) lookup(ctx context.Context, syncID string) (apiVM, error) {
	id, err := ParseSyncID(syncID)
	if err != nil {
		return apiVM{}, err
	}
	if !c.HasToken() {
		return apiVM{}, fmt.Errorf("no API token; connect on the Sync tab, set %s, or run bast bitqate key", APIKeyEnv)
	}
	return c.getVM(ctx, id)
}

func vmName(vm apiVM) string {
	if name := strings.TrimSpace(vm.Name); name != "" {
		return name
	}
	return vm.ID
}

func (c *Client) waitVM(ctx context.Context, id, want string) error {
	want = normalizeState(want)
	for {
		vm, err := c.getVM(ctx, id)
		if err != nil {
			return err
		}
		status := normalizeState(vm.Status)
		if status == want {
			return nil
		}
		if IsSuspendedState(status) {
			return fmt.Errorf("VM %s is suspended (billing-locked)", vmName(vm))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for VM %s to become %s (last status %s)", id, want, status)
		case <-time.After(c.pollEvery()):
		}
	}
}
