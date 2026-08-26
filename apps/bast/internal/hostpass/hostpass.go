package hostpass

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"bast/internal/platform"
)

func validID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return false
	}
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			continue
		case r == '-' || r == '_' || r == '.':
			if i == 0 {
				return false
			}
			continue
		default:
			return false
		}
	}
	return true
}

func Path(dir, managedID string) (string, error) {
	if !validID(managedID) {
		return "", errors.New("invalid host password id")
	}
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("host password directory is required")
	}
	return filepath.Join(dir, strings.TrimSpace(managedID)), nil
}

func Exists(dir, managedID string) bool {
	path, err := Path(dir, managedID)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func Read(dir, managedID string) (string, error) {
	path, err := Path(dir, managedID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("no stored host password")
		}
		return "", fmt.Errorf("read host password: %w", err)
	}
	secret := strings.TrimRight(string(data), "\r\n")
	if secret == "" {
		return "", errors.New("stored host password is empty")
	}
	return secret, nil
}

func Save(dir, managedID, password string) error {
	password = strings.TrimRight(password, "\r\n")
	if password == "" {
		return errors.New("host password is required")
	}
	if strings.ContainsAny(password, "\r\n") {
		return errors.New("host password must be a single line")
	}
	path, err := Path(dir, managedID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".bast-*")
	if err != nil {
		return fmt.Errorf("write host password: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write([]byte(password + "\n")); err != nil {
		tmp.Close()
		return fmt.Errorf("write host password: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := platform.ReplaceFile(tmpName, path); err != nil {
		return err
	}
	return platform.SecurePath(path, 0600)
}

func Delete(dir, managedID string) error {
	path, err := Path(dir, managedID)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func LooksLikePassword(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}
	denied := []string{
		"passphrase", "passcode", "verification", "otp", "totp", "2fa",
		"token", "challenge", "authenticator", "pin",
	}
	for _, word := range denied {
		if strings.Contains(p, word) {
			return false
		}
	}
	return strings.Contains(p, "password")
}

func PromptHost(prompt string) (string, bool) {
	p := strings.TrimSpace(prompt)
	lower := strings.ToLower(p)
	if i := strings.Index(lower, "'s password"); i >= 0 {
		prefix := strings.TrimSpace(p[:i])
		if _, host, ok := strings.Cut(prefix, "@"); ok {
			host = strings.TrimSpace(host)
			return host, host != ""
		}
		return "", false
	}
	at := strings.LastIndex(p, "@")
	if at < 0 {
		return "", false
	}
	rest := strings.TrimSpace(p[at+1:])
	rest = strings.Trim(rest, ".:")
	host, _, _ := strings.Cut(rest, " ")
	host = strings.Trim(host, `"'`)
	if host == "" || strings.ContainsAny(host, `/\`) {
		return "", false
	}
	return host, true
}

func MatchesDestination(prompt string, expected ...string) bool {
	host, ok := PromptHost(prompt)
	if !ok {
		return true
	}
	for _, candidate := range expected {
		if strings.TrimSpace(candidate) != "" && strings.EqualFold(host, strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func Print(out io.Writer, dir, managedID, prompt string, expectedHosts ...string) error {
	if !LooksLikePassword(prompt) {
		return errors.New("askpass refused a non-password prompt")
	}
	if !MatchesDestination(prompt, expectedHosts...) {
		return errors.New("askpass refused a password prompt for another host")
	}
	secret, err := Read(dir, managedID)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, secret)
	return err
}
