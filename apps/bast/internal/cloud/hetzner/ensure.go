package hetzner

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
	PreferPrivateIP       bool
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
	Public         bool
}

func reportStatus(status func(string), message string) {
	if status != nil && strings.TrimSpace(message) != "" {
		status(message)
	}
}

func (c *Client) EnsureAccess(ctx context.Context, syncID string, cfg EnsureConfig) (EnsureResult, error) {
	reportStatus(cfg.Status, "Checking Hetzner server access…")
	token, srv, err := c.lookup(ctx, syncID)
	if err != nil {
		return EnsureResult{}, err
	}
	status := normalizeState(srv.Status)
	if IsStoppedState(status) {
		return EnsureResult{}, fmt.Errorf("server %s is off; start it first", displayName(srv))
	}
	if status != "running" {
		reportStatus(cfg.Status, "Waiting for Hetzner server…")
		if err := c.waitGuest(ctx, token, srv.ID, "running"); err != nil {
			return EnsureResult{}, err
		}
		srv, err = c.getServer(ctx, token, srv.ID)
		if err != nil {
			return EnsureResult{}, err
		}
	}
	host, public := hostForServer(srv, cfg.PreferPrivateIP)
	if host == "" {
		return EnsureResult{}, fmt.Errorf("server %s has no IP address", displayName(srv))
	}
	if !public {
		reportStatus(cfg.Status, "Using private Cloud Network address (VPN or private route required)…")
	}
	user := strings.TrimSpace(cfg.DefaultSSHUser)
	if user == "" {
		user = strings.TrimSpace(cfg.CurrentUser)
	}
	if user == "" {
		user = "root"
	}
	labeled := labeledSSHPort(srv.Labels)
	port := labeled
	if port == "" {
		port = configuredSSHPort(cfg.DefaultSSHPort)
	}
	if port == "" {
		port = configuredSSHPort(cfg.CurrentPort)
	}
	if labeled == "" {
		if detected := detectSSHPort(ctx, host, port); detected != port {
			if detected != "" && detected != "22" {
				reportStatus(cfg.Status, "SSH is on port "+detected+"…")
			}
			port = detected
		}
	}
	keys, _ := c.listSSHKeys(ctx, token)
	keyByID := map[int]apiSSHKey{}
	var projectPubs []string
	for _, key := range keys {
		keyByID[key.ID] = key
		if strings.TrimSpace(key.PublicKey) != "" {
			projectPubs = append(projectPubs, key.PublicKey)
		}
	}
	var attached []string
	for _, ref := range srv.SSHKeys {
		if key, ok := keyByID[ref.ID]; ok && strings.TrimSpace(key.PublicKey) != "" {
			attached = append(attached, key.PublicKey)
		}
	}
	pubs := attached
	identitiesOnly := len(attached) > 0
	if len(pubs) == 0 {
		pubs = projectPubs
	}
	identity := matchLocalIdentity(pubs, cfg.Home, cfg.ManagedKeys)
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
		Public:         public,
	}, nil
}
