package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bast/internal/platform"
)

type Session struct {
	Email     string `json:"email"`
	Token     string `json:"token"`
	DeviceID  string `json:"deviceId,omitempty"`
	UserID    string `json:"userId,omitempty"`
	Revision  string `json:"revision,omitempty"`
	APIBase   string `json:"apiBase,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

func SessionPath(stateFile string) string {
	return filepath.Join(filepath.Dir(stateFile), "vault-session.json")
}

func PassphrasePath(stateFile string) string {
	return filepath.Join(filepath.Dir(stateFile), "vault-passphrase")
}

func LoadSession(path string) (Session, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, os.ErrNotExist
	}
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, fmt.Errorf("parse vault session: %w", err)
	}
	return s, nil
}

func SaveSession(path string, s Session) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return err
	}
	s.UpdatedAt = time.Now().UTC().Unix()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	if err := platform.ReplaceFile(tmp, path); err != nil {
		return err
	}
	return platform.SecurePath(path, 0600)
}

func ClearSession(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func LoadPassphrase(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func SavePassphrase(path, passphrase string) error {
	if passphrase == "" {
		return ClearPassphrase(path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(passphrase+"\n"), 0600); err != nil {
		return err
	}
	if err := platform.ReplaceFile(tmp, path); err != nil {
		return err
	}
	return platform.SecurePath(path, 0600)
}

func ClearPassphrase(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// VerifyPassphrase decrypts remote ciphertext when present. Empty remote
// vaults accept any passphrase (initial link / empty account).
func VerifyPassphrase(ciphertext []byte, passphrase string) error {
	if passphrase == "" {
		return errors.New("vault passphrase is required")
	}
	if len(ciphertext) == 0 {
		return nil
	}
	_, err := Decrypt(ciphertext, passphrase)
	return err
}
