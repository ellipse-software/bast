package railway

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"bast/internal/platform"
)

type sshPublicKey struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PublicKey   string `json:"publicKey"`
	Fingerprint string `json:"fingerprint"`
}

func reportStatus(status func(string), message string) {
	if status != nil && strings.TrimSpace(message) != "" {
		status(message)
	}
}

func (c *Client) EnsureIdentity(ctx context.Context, status func(string)) (string, error) {
	priv := c.identityPath()
	if priv == "" {
		return "", fmt.Errorf("railway identity path is not configured")
	}
	if err := ensureLocalKey(ctx, priv, c.SSHKeygen, status); err != nil {
		return "", err
	}
	pub, err := os.ReadFile(priv + ".pub")
	if err != nil {
		return "", fmt.Errorf("read Railway public key: %w", err)
	}
	publicKey := strings.TrimSpace(string(pub))
	if publicKey == "" {
		return "", fmt.Errorf("Railway public key is empty")
	}
	reportStatus(status, "Checking Railway SSH keys…")
	keys, err := c.listSSHKeys(ctx)
	if err != nil {
		return "", err
	}
	blob := keyBlob(publicKey)
	for _, key := range keys {
		if keyBlob(key.PublicKey) == blob {
			return priv, nil
		}
	}
	reportStatus(status, "Registering SSH key with Railway…")
	if err := c.registerSSHKey(ctx, "bast", publicKey); err != nil {
		return "", err
	}
	return priv, nil
}

func (c *Client) listSSHKeys(ctx context.Context) ([]sshPublicKey, error) {
	var data struct {
		SSHPublicKeys edgeList[sshPublicKey] `json:"sshPublicKeys"`
	}
	if err := c.graphql(ctx, `query {
  sshPublicKeys(first: 100) {
    edges { node { id name publicKey fingerprint } }
  }
}`, nil, &data); err != nil {
		return nil, err
	}
	return nodes(data.SSHPublicKeys), nil
}

func (c *Client) registerSSHKey(ctx context.Context, name, publicKey string) error {
	return c.graphql(ctx, `mutation sshPublicKeyCreate($input: SshPublicKeyCreateInput!) {
  sshPublicKeyCreate(input: $input) { id name fingerprint }
}`, map[string]any{
		"input": map[string]any{
			"name":      name,
			"publicKey": publicKey,
		},
	}, nil)
}

func ensureLocalKey(ctx context.Context, priv, sshKeygen string, status func(string)) error {
	if err := os.MkdirAll(filepath.Dir(priv), 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(filepath.Dir(priv), 0700); err != nil {
		return err
	}
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
			return fmt.Errorf("derive Railway public key: %w", err)
		}
		if err := os.WriteFile(pub, out, 0644); err != nil {
			return err
		}
		return nil
	}
	reportStatus(status, "Generating Railway SSH key…")
	bin := sshKeygen
	if bin == "" {
		bin = "ssh-keygen"
	}
	cmd := exec.CommandContext(ctx, bin, "-t", "ed25519", "-f", priv, "-N", "", "-C", "bast-railway")
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("generate Railway SSH key: %s", msg)
	}
	_ = platform.SecurePath(priv, 0600)
	return nil
}

func keyBlob(pub string) string {
	parts := strings.Fields(strings.TrimSpace(pub))
	if len(parts) >= 2 {
		return parts[0] + " " + parts[1]
	}
	return strings.TrimSpace(pub)
}
