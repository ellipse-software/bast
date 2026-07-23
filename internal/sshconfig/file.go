package sshconfig

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func renderBlock(id string, input HostInput) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s\nHost %s\n", markerPrefix, id, input.Alias)
	fmt.Fprintf(&b, "    HostName %s\n", configValue(input.HostName))
	if input.User != "" {
		fmt.Fprintf(&b, "    User %s\n", configValue(input.User))
	}
	if input.Port != "" {
		fmt.Fprintf(&b, "    Port %s\n", configValue(input.Port))
	}
	if input.IdentityFile != "" {
		fmt.Fprintf(&b, "    IdentityFile %s\n", configValue(input.IdentityFile))
	}
	for _, option := range input.ExtraOptions {
		fmt.Fprintf(&b, "    %s\n", option)
	}
	if input.PasswordOnly {
		b.WriteString("    PubkeyAuthentication no\n")
		b.WriteString("    PasswordAuthentication yes\n")
		b.WriteString("    PreferredAuthentications keyboard-interactive,password\n")
	}
	if input.ProxyJump != "" {
		fmt.Fprintf(&b, "    ProxyJump %s\n", configValue(input.ProxyJump))
	}
	b.WriteString(markerEnd + "\n")
	return []byte(b.String())
}

func configValue(value string) string {
	if !strings.ContainsAny(value, " \t#\\\"") {
		return value
	}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + escaped + `"`
}

func replaceBlock(data []byte, id string, replacement []byte) ([]byte, bool) {
	startMarker := []byte(markerPrefix + id)
	start := bytes.Index(data, startMarker)
	if start < 0 || (start > 0 && data[start-1] != '\n') {
		return data, false
	}
	endRel := bytes.Index(data[start:], []byte(markerEnd))
	if endRel < 0 {
		return data, false
	}
	end := start + endRel + len(markerEnd)
	if end < len(data) && data[end] == '\r' {
		end++
	}
	if end < len(data) && data[end] == '\n' {
		end++
	}
	out := make([]byte, 0, len(data)-end+start+len(replacement))
	out = append(out, data[:start]...)
	out = append(out, replacement...)
	out = append(out, data[end:]...)
	return out, true
}

func hasInclude(data []byte, target, home, base string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	topLevel := true
	for scanner.Scan() {
		parts, _ := fields(strings.TrimSpace(scanner.Text()))
		if len(parts) == 0 {
			continue
		}
		if strings.EqualFold(parts[0], "host") || strings.EqualFold(parts[0], "match") {
			topLevel = false
		}
		if topLevel && len(parts) > 1 && strings.EqualFold(parts[0], "include") {
			for _, part := range parts[1:] {
				if cleanPath(expandPath(part, home, base)) == cleanPath(target) {
					return true
				}
			}
		}
	}
	return false
}

func expandPath(path, home, base string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(base, path)
	}
	return path
}

func cleanPath(path string) string {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func selectableAlias(alias string) bool {
	return alias != "" && !strings.HasPrefix(alias, "!") && !strings.ContainsAny(alias, "*?%#\\\"' \t\r\n\x00")
}

func fields(line string) ([]string, error) {
	var result []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			result = append(result, current.String())
			current.Reset()
		}
	}
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' {
			break
		}
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		current.WriteRune(r)
	}
	if escaped || quote != 0 {
		return nil, errors.New("unterminated escape or quote")
	}
	flush()
	return result, nil
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".bast-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode.Perm()); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func atomicWriteChecked(path string, original, updated []byte, mode os.FileMode) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("%s changed while Bast was editing it; reload and try again", path)
	}
	return atomicWrite(path, updated, mode)
}

func fileMode(path string, fallback os.FileMode) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode().Perm()
}

var (
	managedDirectives = map[string]bool{
		"hostname": true, "user": true, "port": true,
		"identityfile": true, "proxyjump": true,
		"pubkeyauthentication": true, "passwordauthentication": true,
		"preferredauthentications": true,
	}
	forbiddenDirectives = map[string]bool{
		"host": true, "match": true, "include": true,
		"proxycommand": true, "remotecommand": true, "localcommand": true,
	}
)

func extractBlockExtras(data []byte, id string) []string {
	block := findManagedBlock(data, id)
	if block == nil {
		return nil
	}
	var extras []string
	for _, line := range strings.Split(string(block), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts, err := fields(line)
		if err != nil || len(parts) == 0 {
			continue
		}
		if strings.EqualFold(parts[0], "Host") || managedDirectives[strings.ToLower(parts[0])] {
			continue
		}
		extras = append(extras, line)
	}
	return extras
}

func findManagedBlock(data []byte, id string) []byte {
	startMarker := []byte(markerPrefix + id)
	start := bytes.Index(data, startMarker)
	if start < 0 || (start > 0 && data[start-1] != '\n') {
		return nil
	}
	endRel := bytes.Index(data[start:], []byte(markerEnd))
	if endRel < 0 {
		return nil
	}
	end := start + endRel + len(markerEnd)
	return data[start:end]
}

func ParseSSHFlags(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var options []string
	for _, chunk := range strings.FieldsFunc(input, func(r rune) bool { return r == '\n' || r == ';' }) {
		if line := strings.TrimSpace(chunk); line != "" {
			options = append(options, line)
		}
	}
	return options
}

func FormatSSHFlags(options []string) string {
	return strings.Join(options, "; ")
}

func validateExtraOptions(options []string) error {
	for _, option := range options {
		parts, err := fields(strings.TrimSpace(option))
		if err != nil {
			return fmt.Errorf("invalid SSH flag %q: %w", option, err)
		}
		if len(parts) == 0 {
			return errors.New("SSH flags cannot be empty")
		}
		name := strings.ToLower(parts[0])
		if forbiddenDirectives[name] {
			return fmt.Errorf("SSH flag %q is not allowed", parts[0])
		}
		if managedDirectives[name] {
			return fmt.Errorf("use the dedicated field for %s instead of SSH flags", parts[0])
		}
		for _, value := range parts[1:] {
			if strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("SSH flag %q cannot contain a newline or null byte", option)
			}
		}
	}
	return nil
}

func HasDirective(options []string, name string) bool {
	lower := strings.ToLower(name)
	for _, option := range options {
		parts, err := fields(strings.TrimSpace(option))
		if err != nil || len(parts) == 0 {
			continue
		}
		if strings.EqualFold(parts[0], lower) {
			return true
		}
	}
	return false
}
