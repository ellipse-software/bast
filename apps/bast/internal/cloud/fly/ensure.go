package fly

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type EnsureConfig struct {
	Home           string
	ManagedKeys    string
	DefaultSSHUser string
	Status         func(string)
}

type EnsureResult struct {
	User            string
	HostName        string
	IdentityFile    string
	CertificateFile string
	IdentitiesOnly  bool
}

const certReuseAge = 20 * time.Hour

func reportStatus(status func(string), message string) {
	if status != nil && strings.TrimSpace(message) != "" {
		status(message)
	}
}

func (c *Client) EnsureAccess(ctx context.Context, syncID string, cfg EnsureConfig) (EnsureResult, error) {
	org, app, id, err := ParseSyncID(syncID)
	if err != nil {
		return EnsureResult{}, err
	}
	if err := c.CheckAvailable(ctx); err != nil {
		return EnsureResult{}, err
	}
	reportStatus(cfg.Status, "Checking Fly Machine…")
	info, err := c.machineInfo(ctx, app, id)
	if err != nil {
		return EnsureResult{}, err
	}
	if !isRunningState(info.State) {
		if IsStoppedState(info.State) {
			return EnsureResult{}, fmt.Errorf("fly machine %s is stopped; resume it first with bast fly resume %s", id, id)
		}
		return EnsureResult{}, fmt.Errorf("fly machine %s is not ready for SSH (state %s)", id, normalizeState(info.State))
	}
	home := cfg.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	managedKeys := strings.TrimSpace(cfg.ManagedKeys)
	if managedKeys == "" {
		managedKeys = filepath.Join(home, ".ssh", "bast", "keys")
	}
	identity, cert, err := c.ensureCertificate(ctx, org, managedKeys, cfg.DefaultSSHUser, cfg.Status)
	if err != nil {
		return EnsureResult{}, err
	}
	user := strings.TrimSpace(cfg.DefaultSSHUser)
	if user == "" {
		user = SSHUser
	}
	return EnsureResult{
		User:            user,
		HostName:        id,
		IdentityFile:    shortenHomePath(identity, home),
		CertificateFile: shortenHomePath(cert, home),
		IdentitiesOnly:  true,
	}, nil
}

func (c *Client) ensureCertificate(ctx context.Context, org, managedKeys, user string, status func(string)) (identity, cert string, err error) {
	if err := os.MkdirAll(managedKeys, 0o700); err != nil {
		return "", "", fmt.Errorf("create Fly key dir: %w", err)
	}
	identity = IdentityPath(managedKeys, org)
	cert = CertificatePath(identity)
	if certFresh(cert) && fileExists(identity) {
		return identity, cert, nil
	}
	reportStatus(status, "Issuing Fly SSH certificate…")
	args := []string{"ssh", "issue", org, identity, "--org", org, "--hours", "24", "--overwrite"}
	if trimmed := strings.TrimSpace(user); trimmed != "" && trimmed != SSHUser && trimmed != "fly" {
		args = append(args, "--username", SSHUser+","+trimmed)
	}
	if _, err := c.runRaw(ctx, args...); err != nil {
		return "", "", err
	}
	if !fileExists(identity) {
		return "", "", fmt.Errorf("fly ssh issue did not write %s", identity)
	}
	if !fileExists(cert) {
		if alt := identity + "-cert.pub"; fileExists(alt) {
			cert = alt
		} else {
			return "", "", fmt.Errorf("fly ssh issue did not write %s", cert)
		}
	}
	return identity, cert, nil
}

func certFresh(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < certReuseAge
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func shortenHomePath(path, home string) string {
	if home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	sep := string(os.PathSeparator)
	prefix := strings.TrimRight(home, "\\/") + sep
	if strings.HasPrefix(path, prefix) {
		return "~/" + strings.TrimPrefix(path, prefix)
	}
	return path
}
