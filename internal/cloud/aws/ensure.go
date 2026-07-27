package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const awsIdentityFile = "~/.ssh/bast/aws_compute"

type EnsureConfig struct {
	Home           string
	ManagedKeys    string
	ProfileFilter  []string
	DefaultSSHUser string
	SSHKeygen      string
	Status         func(string)
}

type EnsureResult struct {
	User           string
	IdentityFile   string
	IdentitiesOnly bool
	KeyAdded       bool
}

func ParseSyncID(syncID string) (partition, region, accountID, instanceID string, err error) {
	parts := strings.Split(strings.TrimSpace(syncID), ":")
	if len(parts) != 6 || parts[0] != "arn" || parts[2] != "ec2" || !strings.HasPrefix(parts[5], "instance/") {
		return "", "", "", "", fmt.Errorf("invalid AWS sync id %q", syncID)
	}
	partition, region, accountID = parts[1], parts[3], parts[4]
	instanceID = strings.TrimPrefix(parts[5], "instance/")
	if partition == "" || region == "" || accountID == "" || instanceID == "" {
		return "", "", "", "", fmt.Errorf("invalid AWS sync id %q", syncID)
	}
	return partition, region, accountID, instanceID, nil
}

func reportStatus(status func(string), message string) {
	if status != nil && strings.TrimSpace(message) != "" {
		status(message)
	}
}

func (c *Client) EnsureAccess(ctx context.Context, syncID string, cfg EnsureConfig) (EnsureResult, error) {
	_, region, accountID, instanceID, err := ParseSyncID(syncID)
	if err != nil {
		return EnsureResult{}, err
	}
	if err := c.CheckAvailable(ctx); err != nil {
		return EnsureResult{}, err
	}
	profiles, err := c.ListProfiles(ctx)
	if err != nil {
		return EnsureResult{}, err
	}
	profiles = filterValues(profiles, cfg.ProfileFilter)
	reportStatus(cfg.Status, "Checking AWS instance access…")
	var selected identity
	var inst ec2Instance
	var lastErr error
	for _, profile := range profiles {
		id, identityErr := c.callerIdentity(ctx, profile)
		if identityErr != nil || id.AccountID != accountID {
			if identityErr != nil {
				lastErr = identityErr
			}
			continue
		}
		described, describeErr := c.describeInstance(ctx, profile, region, instanceID)
		if describeErr != nil {
			lastErr = describeErr
			continue
		}
		selected, inst = id, described
		break
	}
	if inst.InstanceID == "" {
		if lastErr != nil {
			return EnsureResult{}, lastErr
		}
		return EnsureResult{}, fmt.Errorf("no selected AWS profile can access instance %s", instanceID)
	}
	home := cfg.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	userName := strings.TrimSpace(cfg.DefaultSSHUser)
	if userName == "" {
		images, imageErr := c.describeImages(ctx, selected.Profile, region, map[string]bool{inst.ImageID: true})
		if imageErr != nil {
			return EnsureResult{}, imageErr
		}
		userName = imageSSHUser(images[inst.ImageID])
	}
	if userName == "" {
		return EnsureResult{}, fmt.Errorf("could not determine the EC2 SSH user; set a default SSH user in Sync")
	}
	if identityFile := findLaunchKey(home, cfg.ManagedKeys, inst.KeyName); identityFile != "" {
		reportStatus(cfg.Status, "Using local EC2 key…")
		return EnsureResult{User: userName, IdentityFile: identityFile, IdentitiesOnly: true}, nil
	}
	if err := ensureAWSIdentity(ctx, home, cfg.SSHKeygen, cfg.Status); err != nil {
		return EnsureResult{}, err
	}
	pubPath := filepath.Join(home, ".ssh", "bast", "aws_compute.pub")
	reportStatus(cfg.Status, "Publishing a short-lived SSH key with EC2 Instance Connect…")
	out, err := c.run(ctx, "ec2-instance-connect", "send-ssh-public-key",
		"--instance-id", inst.InstanceID,
		"--instance-os-user", userName,
		"--availability-zone", inst.Placement.AvailabilityZone,
		"--ssh-public-key", "file://"+pubPath,
		"--profile", selected.Profile,
		"--region", region,
		"--output", "json", "--no-cli-pager")
	if err != nil {
		return EnsureResult{}, fmt.Errorf("publish EC2 Instance Connect SSH key: %w", err)
	}
	var response struct {
		Success bool `json:"Success"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return EnsureResult{}, fmt.Errorf("parse EC2 Instance Connect response: %w", err)
	}
	if !response.Success {
		return EnsureResult{}, fmt.Errorf("EC2 Instance Connect did not accept the SSH key")
	}
	return EnsureResult{User: userName, IdentityFile: awsIdentityFile, IdentitiesOnly: true, KeyAdded: true}, nil
}

func (c *Client) describeInstance(ctx context.Context, profile, region, instanceID string) (ec2Instance, error) {
	out, err := c.run(ctx, "ec2", "describe-instances", "--instance-ids", instanceID,
		"--profile", profile, "--region", region, "--output", "json", "--no-cli-pager")
	if err != nil {
		return ec2Instance{}, err
	}
	var raw struct {
		Reservations []struct {
			Instances []ec2Instance `json:"Instances"`
		} `json:"Reservations"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return ec2Instance{}, fmt.Errorf("parse instance describe: %w", err)
	}
	for _, reservation := range raw.Reservations {
		for _, inst := range reservation.Instances {
			if inst.InstanceID == instanceID {
				return inst, nil
			}
		}
	}
	return ec2Instance{}, fmt.Errorf("instance %s was not found", instanceID)
}

func ensureAWSIdentity(ctx context.Context, home, sshKeygen string, status func(string)) error {
	dir := filepath.Join(home, ".ssh", "bast")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	priv := filepath.Join(dir, "aws_compute")
	pub := priv + ".pub"
	if _, err := os.Stat(priv); err == nil {
		if _, err := os.Stat(pub); err == nil {
			return nil
		}
		bin := sshKeygen
		if bin == "" {
			bin = "ssh-keygen"
		}
		out, err := exec.CommandContext(ctx, bin, "-y", "-f", priv, "-P", "").Output()
		if err != nil {
			return fmt.Errorf("derive AWS public key: %w", err)
		}
		return os.WriteFile(pub, out, 0644)
	}
	reportStatus(status, "Generating an AWS SSH key (~/.ssh/bast/aws_compute)…")
	bin := sshKeygen
	if bin == "" {
		bin = "ssh-keygen"
	}
	cmd := exec.CommandContext(ctx, bin, "-t", "rsa", "-b", "3072", "-f", priv, "-N", "", "-C", "bast-aws")
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("generate AWS SSH key: %s", msg)
	}
	return nil
}
