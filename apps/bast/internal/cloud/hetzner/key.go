package hetzner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"bast/internal/platform"
)

func (c *Client) tokenDir() string {
	return strings.TrimSpace(c.TokenDir)
}

func ValidateTokenName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("token name is required")
	}
	if name == "." || name == ".." || filepath.Base(name) != name || strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid token name %q", name)
	}
	if strings.ContainsAny(name, "/\\ \t\r\n") {
		return fmt.Errorf("invalid token name %q", name)
	}
	if len(name) > 48 {
		return fmt.Errorf("token name is too long")
	}
	for _, r := range name {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.') {
			return fmt.Errorf("invalid token name %q", name)
		}
	}
	return nil
}

func (c *Client) storedTokenPath(name string) (string, error) {
	if err := ValidateTokenName(name); err != nil {
		return "", err
	}
	dir := c.tokenDir()
	if dir == "" {
		return "", fmt.Errorf("Hetzner token directory is not configured")
	}
	return filepath.Join(dir, name), nil
}

func (c *Client) ListStoredTokens() ([]TokenContext, error) {
	c.migrateLegacyToken()
	dir := c.tokenDir()
	if dir == "" {
		if token, err := ReadKeyFile(c.KeyFile); err == nil {
			return []TokenContext{{Name: "default", Token: token, Source: "file"}}, nil
		}
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			if token, err := ReadKeyFile(c.KeyFile); err == nil {
				return []TokenContext{{Name: "default", Token: token, Source: "file"}}, nil
			}
			return nil, nil
		}
		return nil, fmt.Errorf("read Hetzner tokens: %w", err)
	}
	var out []TokenContext
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".tmp") || strings.HasPrefix(name, ".") {
			continue
		}
		if err := ValidateTokenName(name); err != nil {
			continue
		}
		token, err := ReadKeyFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		out = append(out, TokenContext{Name: name, Token: token, Source: "file"})
	}
	if len(out) == 0 {
		if token, err := ReadKeyFile(c.KeyFile); err == nil {
			out = append(out, TokenContext{Name: "default", Token: token, Source: "file"})
		}
	}
	return out, nil
}

func (c *Client) StoredTokenNames() []string {
	tokens, err := c.ListStoredTokens()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(tokens))
	for _, token := range tokens {
		names = append(names, token.Name)
	}
	return names
}

func (c *Client) SaveNamedToken(name, token string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	if err := ValidateTokenName(name); err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("Hetzner API token is required")
	}
	if strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("Hetzner API token must be a single line")
	}
	dir := c.tokenDir()
	if dir == "" {
		return WriteKeyFile(c.KeyFile, token)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := WriteKeyFile(path, token); err != nil {
		return err
	}
	c.APIKey = ""
	return nil
}

func (c *Client) DeleteNamedToken(name string) error {
	name = strings.TrimSpace(name)
	if err := ValidateTokenName(name); err != nil {
		return err
	}
	dir := c.tokenDir()
	if dir != "" {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove Hetzner token %q: %w", name, err)
		}
	}
	if name == "default" && c.KeyFile != "" {
		_ = os.Remove(c.KeyFile)
	}
	c.APIKey = ""
	return nil
}

func (c *Client) SaveKey(key string) error {
	return c.SaveNamedToken("default", key)
}

func (c *Client) migrateLegacyToken() {
	dir := c.tokenDir()
	if dir == "" || strings.TrimSpace(c.KeyFile) == "" {
		return
	}
	legacy, err := ReadKeyFile(c.KeyFile)
	if err != nil {
		return
	}
	dest := filepath.Join(dir, "default")
	if _, err := os.Stat(dest); err == nil {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	_ = platform.SecurePath(dir, 0700)
	if err := WriteKeyFile(dest, legacy); err == nil {
		_ = os.Remove(c.KeyFile)
	}
}

func ReadKeyFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("no Hetzner API token; connect on the Sync tab, set %s, or run bast hetzner key", APIKeyEnv)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no Hetzner API token; connect on the Sync tab, set %s, or run bast hetzner key", APIKeyEnv)
		}
		return "", fmt.Errorf("read Hetzner API token: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("Hetzner API token file is empty")
	}
	return key, nil
}

func WriteKeyFile(path, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("Hetzner API token is required")
	}
	if strings.ContainsAny(key, "\r\n") {
		return fmt.Errorf("Hetzner API token must be a single line")
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
		return fmt.Errorf("write Hetzner API token: %w", err)
	}
	if err := platform.ReplaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return platform.SecurePath(path, 0600)
}
