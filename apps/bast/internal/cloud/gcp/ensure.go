package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bast/internal/cloud"
)

// EnsureConfig controls credential and key preparation for a GCP connection.
type EnsureConfig struct {
	Home            string
	ManagedKeys     string
	DefaultSSHUser  string
	ServiceAccounts []string
	SSHKeygen       string
	PropagationWait time.Duration // delay after publishing a new key (guest agent)
	SkipKeyPublish  bool          // test helper: resolve auth only
	Status          func(string)
}

// EnsureResult contains the SSH settings prepared for a GCP connection.
type EnsureResult struct {
	User           string
	IdentityFile   string
	IdentitiesOnly bool
	KeyAdded       bool
}

// ParseSyncID extracts the project, zone, and instance name from a GCP sync ID.
func ParseSyncID(syncID string) (projectID, zone, name string, err error) {
	syncID = normalizeSelfLink(strings.TrimSpace(syncID))
	parts := strings.Split(syncID, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "zones" || parts[4] != "instances" {
		return "", "", "", fmt.Errorf("invalid GCP sync id %q", syncID)
	}
	projectID, zone, name = parts[1], parts[3], parts[5]
	if projectID == "" || zone == "" || name == "" {
		return "", "", "", fmt.Errorf("invalid GCP sync id %q", syncID)
	}
	return projectID, zone, name, nil
}

func reportStatus(status func(string), message string) {
	if status == nil || strings.TrimSpace(message) == "" {
		return
	}
	status(message)
}

// EnsureAccess prepares local SSH credentials for a synced GCP instance.
func (c *Client) EnsureAccess(ctx context.Context, syncID string, cfg EnsureConfig) (EnsureResult, error) {
	projectID, zone, name, err := ParseSyncID(syncID)
	if err != nil {
		return EnsureResult{}, err
	}
	if err := c.CheckAvailable(ctx); err != nil {
		return EnsureResult{}, err
	}
	creds, err := c.credentials(ctx, cfg.ServiceAccounts, cfg.Home)
	if err != nil {
		return EnsureResult{}, err
	}

	home := cfg.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	var (
		inst gceInstance
		cred credential
		last error
	)
	reportStatus(cfg.Status, "Checking GCP instance access…")
	for _, candidate := range creds {
		described, descErr := c.describeInstance(ctx, candidate, projectID, zone, name)
		if descErr != nil {
			last = descErr
			continue
		}
		inst = described
		cred = candidate
		break
	}
	if inst.Name == "" {
		if last != nil {
			return EnsureResult{}, last
		}
		return EnsureResult{}, fmt.Errorf("could not describe instance %s", name)
	}

	proj := project{ID: projectID, Name: projectID}
	proj.SSHKeys, proj.SSHUser, proj.EnableOSLogin, err = c.loadProjectSSHMetadata(ctx, cred, projectID)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("load project metadata: %w", err)
	}

	var osLogin osLoginProfile
	osLoginUser := ""
	if effectiveMetadataEnabled(inst, "enable-oslogin", proj.EnableOSLogin) {
		if profile, profileErr := c.describeOSLoginProfile(ctx, cred, projectID); profileErr == nil {
			osLogin = profile
			if cfg.DefaultSSHUser == "" {
				osLoginUser = profile.Username
			}
		}
	}

	mapped := mapInstance(inst, proj, cfg.DefaultSSHUser)
	ResolveAuth(&mapped, home, cfg.ManagedKeys, osLoginUser)

	// Confident local key match already present in metadata — nothing to publish.
	if mapped.IdentitiesOnly && mapped.IdentityFile != "" && mapped.User != "" {
		reportStatus(cfg.Status, "Using local SSH key…")
		return EnsureResult{
			User:           mapped.User,
			IdentityFile:   mapped.IdentityFile,
			IdentitiesOnly: true,
		}, nil
	}

	if mapped.OSLogin {
		return c.ensureOSLoginAccess(ctx, cred, projectID, home, cfg, mapped, osLogin)
	}
	return c.ensureMetadataAccess(ctx, cred, projectID, zone, name, home, cfg, mapped, inst, proj)
}

func (c *Client) ensureOSLoginAccess(
	ctx context.Context,
	cred credential,
	projectID, home string,
	cfg EnsureConfig,
	mapped cloud.Instance,
	profile osLoginProfile,
) (EnsureResult, error) {
	userName := strings.TrimSpace(mapped.User)
	if userName == "" {
		userName = strings.TrimSpace(profile.Username)
	}
	if userName == "" {
		userName = strings.TrimSpace(cfg.DefaultSSHUser)
	}
	if userName == "" {
		return EnsureResult{}, fmt.Errorf("could not determine OS Login SSH user; set a default SSH user in Sync")
	}

	// Prefer any local private key already registered on the OS Login profile.
	if _, identity := matchSSHKeyEntry(profile.SSHKeys, home, cfg.ManagedKeys); identity != "" {
		reportStatus(cfg.Status, "Using local SSH key…")
		return EnsureResult{
			User:           userName,
			IdentityFile:   identity,
			IdentitiesOnly: true,
		}, nil
	}

	if err := ensureGCloudIdentity(ctx, home, cfg.SSHKeygen, cfg.Status); err != nil {
		return EnsureResult{}, err
	}
	pubPath := filepath.Join(home, ".ssh", "google_compute_engine.pub")
	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("read gcloud public key: %w", err)
	}
	pubBlob := publicKeyBlob(string(pubBytes))
	if pubBlob == "" {
		return EnsureResult{}, fmt.Errorf("invalid gcloud public key at %s", pubPath)
	}

	result := EnsureResult{
		User:           userName,
		IdentityFile:   gcloudIdentityFile,
		IdentitiesOnly: true,
	}
	if hasUsablePublicKey(profile.SSHKeys, pubBlob) {
		reportStatus(cfg.Status, "Using Google SSH key…")
		return result, nil
	}
	if cfg.SkipKeyPublish {
		return result, nil
	}

	reportStatus(cfg.Status, "Registering SSH key with Google OS Login — this can take a few seconds…")
	if err := c.addOSLoginSSHKey(ctx, cred, projectID, pubPath); err != nil {
		return EnsureResult{}, fmt.Errorf("publish OS Login SSH key: %w", err)
	}
	result.KeyAdded = true
	reportStatus(cfg.Status, "Waiting for OS Login SSH key to become active…")
	return waitForSSHKeyPropagation(ctx, result, cfg.PropagationWait)
}

func (c *Client) ensureMetadataAccess(
	ctx context.Context,
	cred credential,
	projectID, zone, name, home string,
	cfg EnsureConfig,
	mapped cloud.Instance,
	inst gceInstance,
	proj project,
) (EnsureResult, error) {
	userName := strings.TrimSpace(mapped.User)
	if userName == "" {
		userName = localUsername()
	}
	if userName == "" {
		return EnsureResult{}, fmt.Errorf("could not determine SSH user; set a default SSH user in Sync")
	}

	if err := ensureGCloudIdentity(ctx, home, cfg.SSHKeygen, cfg.Status); err != nil {
		return EnsureResult{}, err
	}
	pubPath := filepath.Join(home, ".ssh", "google_compute_engine.pub")
	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("read gcloud public key: %w", err)
	}
	pubLine := strings.TrimSpace(string(pubBytes))
	pubBlob := publicKeyBlob(pubLine)
	if pubBlob == "" {
		return EnsureResult{}, fmt.Errorf("invalid gcloud public key at %s", pubPath)
	}

	result := EnsureResult{
		User:           userName,
		IdentityFile:   gcloudIdentityFile,
		IdentitiesOnly: true,
	}
	if cfg.SkipKeyPublish {
		return result, nil
	}

	instanceKeysRaw := metadataValue(inst, "ssh-keys")
	keys := mergeSSHKeys(instanceKeysRaw, proj.SSHKeys)
	if hasUsablePublicKey(keys, pubBlob) {
		reportStatus(cfg.Status, "Using Google SSH key…")
		return result, nil
	}

	reportStatus(cfg.Status, "Publishing Google SSH key to the VM — this can take a few seconds…")
	// Prefer instance metadata (typical VM creator IAM). Fall back to project-wide
	// keys like classic gcloud when instance updates are denied.
	if err := c.addInstanceSSHKey(ctx, cred, projectID, zone, name, instanceKeysRaw, userName, pubLine); err != nil {
		if mapped.BlockProjectSSHKeys {
			return EnsureResult{}, fmt.Errorf("publish instance SSH key: %w", err)
		}
		if projErr := c.addProjectSSHKey(ctx, cred, projectID, proj.SSHKeys, userName, pubLine); projErr != nil {
			return EnsureResult{}, fmt.Errorf("publish SSH key: instance: %v; project: %w", err, projErr)
		}
	}
	result.KeyAdded = true
	reportStatus(cfg.Status, "Waiting for the guest agent to pick up the new key…")
	return waitForSSHKeyPropagation(ctx, result, cfg.PropagationWait)
}

func waitForSSHKeyPropagation(ctx context.Context, result EnsureResult, wait time.Duration) (EnsureResult, error) {
	if wait < 0 {
		wait = 0
	} else if wait == 0 {
		wait = 3 * time.Second
	}
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-timer.C:
		}
	}
	return result, nil
}

func (c *Client) credentials(ctx context.Context, serviceAccounts []string, home string) ([]credential, error) {
	creds := []credential{}
	accounts, err := c.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	// Prefer ACTIVE account first for ensure operations.
	for _, account := range accounts {
		if account.Active {
			creds = append(creds, credential{
				label:   account.Account,
				account: account.Account,
				args:    []string{"--account=" + account.Account},
			})
		}
	}
	for _, account := range accounts {
		if account.Active {
			continue
		}
		creds = append(creds, credential{
			label:   account.Account,
			account: account.Account,
			args:    []string{"--account=" + account.Account},
		})
	}
	for _, keyPath := range serviceAccounts {
		expanded := expandHomeAt(keyPath, home)
		if _, err := os.Stat(expanded); err != nil {
			return nil, fmt.Errorf("service account key %q: %w", keyPath, err)
		}
		creds = append(creds, credential{
			label:          "sa:" + filepath.Base(expanded),
			credentialFile: expanded,
			env:            []string{"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=" + expanded},
		})
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf("no GCP credentials; run gcloud auth login or add a service account key")
	}
	return creds, nil
}

func (c *Client) addOSLoginSSHKey(ctx context.Context, cred credential, projectID, pubPath string) error {
	args := append([]string{
		"compute", "os-login", "ssh-keys", "add",
		"--key-file=" + pubPath,
		"--project=" + projectID,
		"--quiet",
	}, cred.args...)
	_, err := c.run(ctx, args, cred.env)
	return err
}

func (c *Client) describeInstance(ctx context.Context, cred credential, projectID, zone, name string) (gceInstance, error) {
	args := append([]string{
		"compute", "instances", "describe", name,
		"--project=" + projectID,
		"--zone=" + zone,
		"--format=json",
	}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return gceInstance{}, err
	}
	var inst gceInstance
	if err := json.Unmarshal(out, &inst); err != nil {
		return gceInstance{}, fmt.Errorf("parse instance describe: %w", err)
	}
	return inst, nil
}

func hasUsablePublicKey(keys []cloud.SSHKey, pubBlob string) bool {
	for _, key := range keys {
		if key.Expired {
			continue
		}
		if publicKeyBlob(key.PublicKey) == pubBlob {
			return true
		}
	}
	return false
}

func (c *Client) addInstanceSSHKey(ctx context.Context, cred credential, projectID, zone, name, existing, userName, pubLine string) error {
	return c.writeSSHKeysMetadata(ctx, cred, existing, userName, pubLine, []string{
		"compute", "instances", "add-metadata", name,
		"--project=" + projectID,
		"--zone=" + zone,
	})
}

func (c *Client) addProjectSSHKey(ctx context.Context, cred credential, projectID, existing, userName, pubLine string) error {
	return c.writeSSHKeysMetadata(ctx, cred, existing, userName, pubLine, []string{
		"compute", "project-info", "add-metadata",
		"--project=" + projectID,
	})
}

func (c *Client) writeSSHKeysMetadata(ctx context.Context, cred credential, existing, userName, pubLine string, baseArgs []string) error {
	entry := userName + ":" + strings.TrimSpace(pubLine)
	merged := mergeSSHKeyMetadata(existing, entry)

	tmp, err := os.CreateTemp("", "bast-gcp-ssh-keys-*.txt")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(merged); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	args := append(append([]string{}, baseArgs...), "--metadata-from-file=ssh-keys="+tmpPath)
	args = append(args, cred.args...)
	_, err = c.run(ctx, args, cred.env)
	if err != nil {
		return err
	}
	return nil
}

func mergeSSHKeyMetadata(existing, entry string) string {
	entry = strings.TrimSpace(entry)
	_, entryKey, _ := strings.Cut(entry, ":")
	entryBlob := publicKeyBlob(entryKey)

	var lines []string
	for _, line := range strings.Split(existing, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, key, ok := strings.Cut(line, ":")
		if !ok {
			key = line
		}
		// Drop expired google-ssh temps and exact duplicates of the key we are adding.
		if sshKeyExpired(key) {
			continue
		}
		if entryBlob != "" && publicKeyBlob(key) == entryBlob {
			continue
		}
		lines = append(lines, line)
	}
	lines = append(lines, entry)
	return strings.Join(lines, "\n") + "\n"
}

func ensureGCloudIdentity(ctx context.Context, home, sshKeygen string, status func(string)) error {
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	priv := filepath.Join(dir, "google_compute_engine")
	pub := priv + ".pub"
	if _, err := os.Stat(priv); err == nil {
		if _, err := os.Stat(pub); err == nil {
			return nil
		}
		// Private key exists without pub — derive it.
		return runSSHKeygen(ctx, sshKeygen, "-y", "-f", priv, "-P", "")
	}
	reportStatus(status, "Generating Google SSH key (~/.ssh/google_compute_engine)…")
	if err := runSSHKeygen(ctx, sshKeygen,
		"-t", "rsa",
		"-b", "3072",
		"-f", priv,
		"-N", "",
		"-C", "bast-gcp",
	); err != nil {
		return fmt.Errorf("generate gcloud SSH key: %w", err)
	}
	return nil
}

func runSSHKeygen(ctx context.Context, sshKeygen string, args ...string) error {
	bin := sshKeygen
	if bin == "" {
		bin = "ssh-keygen"
	}
	// Special-case: -y writes pubkey to stdout; write it next to the private key.
	if len(args) >= 1 && args[0] == "-y" {
		cmd := exec.CommandContext(ctx, bin, args...)
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("ssh-keygen: %w", err)
		}
		priv := ""
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "-f" {
				priv = args[i+1]
				break
			}
		}
		if priv == "" {
			return fmt.Errorf("ssh-keygen -y: missing -f")
		}
		return os.WriteFile(priv+".pub", out, 0644)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ssh-keygen: %s", msg)
	}
	return nil
}
