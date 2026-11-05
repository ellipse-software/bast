package boxd

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type EnsureConfig struct {
	Status func(string)
	// Alias is the host's Bast alias, for the hint on a stopped computer.
	Alias string
	// EntryWait bounds the wait for the boxd CLI to write a missing SSH entry.
	EntryWait time.Duration
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

// EnsureAccess checks that SSH will reach the computer and returns the host
// details the boxd CLI keeps for it. Standby and hibernated computers are left
// alone: the SSH connection wakes them.
func (c *Client) EnsureAccess(ctx context.Context, syncID string, cfg EnsureConfig) (EnsureResult, error) {
	orgID, region, name, err := ParseSyncID(syncID)
	if err != nil {
		return EnsureResult{}, err
	}
	reportStatus(cfg.Status, "Checking boxd computer…")
	machine, err := c.Get(ctx, orgID, region, name)
	if err != nil {
		return EnsureResult{}, err
	}
	if machine.SSHDisabled {
		return EnsureResult{}, fmt.Errorf("SSH is off for boxd computer %s; turn it on with boxd machine config set %s ssh on --org %s --region %s", name, name, orgID, region)
	}
	state := normalizeState(machine.Status)
	switch {
	case IsStoppedState(state):
		target := strings.TrimSpace(cfg.Alias)
		if target == "" {
			target = syncID
		}
		return EnsureResult{}, fmt.Errorf("boxd computer %s is %s; start it first with bast boxd start %s", name, state, target)
	case isTransitionalState(state):
		reportStatus(cfg.Status, "Waiting for boxd computer…")
		if err := c.WaitRunning(ctx, orgID, region, name, startTimeout); err != nil {
			return EnsureResult{}, err
		}
	}
	host := hostFromURL(machine.URL)
	if host == "" {
		return EnsureResult{}, fmt.Errorf("boxd computer %s has no address", name)
	}
	details, err := c.sshEntry(ctx, orgID, host, cfg)
	if err != nil {
		return EnsureResult{}, err
	}
	user := strings.TrimSpace(details.User)
	if user == "" {
		user = SSHUser
	}
	identity := shortenHomePath(strings.TrimSpace(details.IdentityFile), c.Home)
	return EnsureResult{
		HostName: host, Port: strings.TrimSpace(details.Port), User: user,
		IdentityFile: identity, IdentitiesOnly: identity != "" && details.IdentitiesOnly,
	}, nil
}

// sshEntry reads the computer's entry from the SSH config. When it is missing,
// a full `boxd machine list` makes the CLI rewrite its entries in the
// background, so this asks once and waits for the entry to appear.
func (c *Client) sshEntry(ctx context.Context, orgID, host string, cfg EnsureConfig) (SSHDetails, error) {
	if c.ResolveSSH == nil {
		return SSHDetails{}, fmt.Errorf("no SSH resolver")
	}
	details, err := c.ResolveSSH(ctx, host)
	if err == nil && details.Found() {
		return details, nil
	}
	reportStatus(cfg.Status, "Refreshing boxd SSH entries…")
	if _, listErr := c.run(ctx, "machine", "list", "--org", orgID, "--json"); listErr != nil {
		return SSHDetails{}, listErr
	}
	wait := cfg.EntryWait
	if wait <= 0 {
		wait = 8 * time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		details, err = c.ResolveSSH(ctx, host)
		if err == nil && details.Found() {
			return details, nil
		}
		if time.Now().After(deadline) {
			return SSHDetails{}, fmt.Errorf("the boxd CLI has no SSH entry for %s; run boxd auth login (an API key in BOXD_TOKEN does not write SSH config), then connect again", host)
		}
		select {
		case <-ctx.Done():
			return SSHDetails{}, ctx.Err()
		case <-time.After(c.pollEvery() / 2):
		}
	}
}
