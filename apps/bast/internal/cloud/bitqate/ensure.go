package bitqate

import (
	"context"
	"fmt"
	"strings"
)

type EnsureConfig struct {
	Home                  string
	ManagedKeys           string
	DefaultSSHUser        string
	DefaultSSHPort        string
	CurrentUser           string
	CurrentPort           string
	CurrentIdentity       string
	CurrentIdentitiesOnly bool
	Status                func(string)
}

type EnsureResult struct {
	HostName       string
	Port           string
	User           string
	IdentityFile   string
	IdentitiesOnly bool
}

func reportStatus(status func(string), message string) {
	if status != nil && strings.TrimSpace(message) != "" {
		status(message)
	}
}

func (c *Client) EnsureAccess(ctx context.Context, syncID string, cfg EnsureConfig) (EnsureResult, error) {
	reportStatus(cfg.Status, "Checking BitQate VM access…")
	vm, err := c.lookup(ctx, syncID)
	if err != nil {
		return EnsureResult{}, err
	}
	status := normalizeState(vm.Status)
	switch {
	case IsSuspendedState(status):
		return EnsureResult{}, fmt.Errorf("VM %s is suspended (billing-locked); renew it in the BitQate dashboard first", vmName(vm))
	case IsStoppedState(status):
		return EnsureResult{}, fmt.Errorf("VM %s is stopped; start it first", vmName(vm))
	case status != "running":
		reportStatus(cfg.Status, "Waiting for BitQate VM…")
		if err := c.waitVM(ctx, vm.ID, "running"); err != nil {
			return EnsureResult{}, err
		}
		vm, err = c.getVM(ctx, vm.ID)
		if err != nil {
			return EnsureResult{}, err
		}
	}
	rawHost := strings.TrimSpace(vm.IPv4)
	host := normalizeIPv4(rawHost)
	if host == "" {
		if rawHost != "" {
			return EnsureResult{}, fmt.Errorf("VM %s has an invalid IPv4 address", vmName(vm))
		}
		return EnsureResult{}, fmt.Errorf("VM %s has no IPv4 address", vmName(vm))
	}
	user := strings.TrimSpace(cfg.DefaultSSHUser)
	if user == "" {
		user = strings.TrimSpace(cfg.CurrentUser)
	}
	if user == "" {
		user = "root"
	}
	port := configuredSSHPort(cfg.DefaultSSHPort)
	if port == "" {
		port = configuredSSHPort(cfg.CurrentPort)
	}
	var pubs []string
	if key := strings.TrimSpace(vm.SSH.AuthorizedKey); key != "" {
		pubs = append(pubs, key)
	}
	identity := matchLocalIdentity(pubs, cfg.Home, cfg.ManagedKeys)
	identitiesOnly := len(pubs) > 0
	if identity == "" && strings.TrimSpace(cfg.CurrentIdentity) != "" {
		identity = cfg.CurrentIdentity
		identitiesOnly = cfg.CurrentIdentitiesOnly
	}
	return EnsureResult{
		HostName:       host,
		Port:           port,
		User:           user,
		IdentityFile:   identity,
		IdentitiesOnly: identity != "" && identitiesOnly,
	}, nil
}
