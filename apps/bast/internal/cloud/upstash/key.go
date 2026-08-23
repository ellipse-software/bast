package upstash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bast/internal/platform"
)

func (c *Client) ResolveKey() (string, error) {
	if key := strings.TrimSpace(c.APIKey); key != "" {
		return key, nil
	}
	if key := strings.TrimSpace(os.Getenv(APIKeyEnv)); key != "" {
		return key, nil
	}
	return ReadKeyFile(c.KeyFile)
}

func ReadKeyFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("no Upstash Box API key; connect on the Sync tab or set %s", APIKeyEnv)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no Upstash Box API key; connect on the Sync tab or set %s", APIKeyEnv)
		}
		return "", fmt.Errorf("read Upstash Box API key: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("Upstash Box API key file is empty")
	}
	return key, nil
}

func WriteKeyFile(path, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("Upstash Box API key is required")
	}
	if strings.ContainsAny(key, "\r\n") {
		return fmt.Errorf("Upstash Box API key must be a single line")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(key+"\n"), 0600); err != nil {
		return fmt.Errorf("write Upstash Box API key: %w", err)
	}
	if err := platform.ReplaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return platform.SecurePath(path, 0600)
}

func (c *Client) SaveKey(key string) error {
	if err := WriteKeyFile(c.KeyFile, key); err != nil {
		return err
	}
	c.APIKey = ""
	return nil
}

// PersistResolvedKey copies env/override keys into the local key file so SSH
// askpass can read them without putting the secret in the ssh process environment.
func (c *Client) PersistResolvedKey() error {
	key, err := c.ResolveKey()
	if err != nil {
		return err
	}
	if existing, err := ReadKeyFile(c.KeyFile); err == nil && existing == key {
		return nil
	}
	return WriteKeyFile(c.KeyFile, key)
}
