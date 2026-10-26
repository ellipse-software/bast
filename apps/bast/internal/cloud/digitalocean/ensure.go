package digitalocean

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type EnsureConfig struct {
	Home           string
	ManagedKeys    string
	ContextFilter  []string
	DefaultSSHUser string
	Status         func(string)
}

type EnsureResult struct {
	HostName       string
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
	reportStatus(cfg.Status, "Checking DigitalOcean droplet access…")
	selected, item, err := c.locate(ctx, syncID, cfg.ContextFilter)
	if err != nil {
		return EnsureResult{}, err
	}
	status := strings.TrimSpace(item.Status)
	if !strings.EqualFold(status, "active") {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strconv.Itoa(item.ID)
		}
		return EnsureResult{}, fmt.Errorf("droplet %s is %s; resume it with bast digitalocean resume", name, status)
	}
	hostName := publicHost(item)
	if hostName == "" {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strconv.Itoa(item.ID)
		}
		return EnsureResult{}, fmt.Errorf("droplet %s has no public IP; Bast does not tunnel private DigitalOcean Droplets", name)
	}
	home := cfg.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	userName := strings.TrimSpace(cfg.DefaultSSHUser)
	if userName == "" {
		userName = imageSSHUser(item)
	}
	keys, keyErr := c.listSSHKeys(ctx, selected)
	if keyErr != nil {
		return EnsureResult{}, keyErr
	}
	identityFile := matchLocalKey(sshKeyBlobs(keys), home, cfg.ManagedKeys)
	if identityFile != "" {
		reportStatus(cfg.Status, "Using local DigitalOcean SSH key…")
	}
	return EnsureResult{
		HostName: hostName, User: userName, IdentityFile: identityFile,
		IdentitiesOnly: identityFile != "",
	}, nil
}
