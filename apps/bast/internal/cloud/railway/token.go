package railway

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bast/internal/platform"
)

func (c *Client) ResolveToken() (string, error) {
	if token := strings.TrimSpace(c.APIToken); token != "" {
		return token, nil
	}
	if token := strings.TrimSpace(os.Getenv(APIKeyEnv)); token != "" {
		return token, nil
	}
	return ReadTokenFile(c.TokenFile)
}

func ReadTokenFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("no Railway API token; connect on the Sync tab or set %s", APIKeyEnv)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no Railway API token; connect on the Sync tab or set %s", APIKeyEnv)
		}
		return "", fmt.Errorf("read Railway API token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("Railway API token file is empty")
	}
	return token, nil
}

func WriteTokenFile(path, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("Railway API token is required")
	}
	if strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("Railway API token must be a single line")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0600); err != nil {
		return fmt.Errorf("write Railway API token: %w", err)
	}
	if err := platform.ReplaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return platform.SecurePath(path, 0600)
}

func (c *Client) SaveToken(token string) error {
	if err := WriteTokenFile(c.TokenFile, token); err != nil {
		return err
	}
	c.APIToken = ""
	return nil
}

// PersistResolvedToken copies env/override tokens into the local token file.
func (c *Client) PersistResolvedToken() error {
	token, err := c.ResolveToken()
	if err != nil {
		return err
	}
	if existing, err := ReadTokenFile(c.TokenFile); err == nil && existing == token {
		return nil
	}
	return WriteTokenFile(c.TokenFile, token)
}
