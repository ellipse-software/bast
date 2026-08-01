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

// RenderManagedBlock renders a Bast-managed host block for vault apply/rebuild.
func RenderManagedBlock(id string, input HostInput) []byte {
	return renderBlock(id, input)
}

// WriteManagedConfig atomically replaces the managed SSH config file.
func WriteManagedConfig(path string, content []byte) error {
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	return atomicWrite(path, content, 0600)
}

func RenderSyncBlock(input SyncHostInput) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s=%s\nHost %s\n", syncMarkerPrefix, input.SyncSource, input.SyncID, input.Alias)
	fmt.Fprintf(&b, "    HostName %s\n", configValue(input.HostName))
	if input.User != "" {
		fmt.Fprintf(&b, "    User %s\n", configValue(input.User))
	}
	if input.Port != "" {
		fmt.Fprintf(&b, "    Port %s\n", configValue(input.Port))
	}
	if input.IdentityFile != "" {
		fmt.Fprintf(&b, "    IdentityFile %s\n", configValue(input.IdentityFile))
		if input.IdentitiesOnly && !HasDirective(input.ExtraOptions, "IdentitiesOnly") {
			b.WriteString("    IdentitiesOnly yes\n")
		}
	}
	if input.CertificateFile != "" {
		fmt.Fprintf(&b, "    CertificateFile %s\n", configValue(input.CertificateFile))
	}
	if input.ProxyCommand != "" {
		fmt.Fprintf(&b, "    ProxyCommand %s\n", input.ProxyCommand)
	}
	for _, option := range input.ExtraOptions {
		fmt.Fprintf(&b, "    %s\n", option)
	}
	b.WriteString(syncMarkerEnd + "\n")
	return []byte(b.String())
}

func WriteSyncConfig(path string, blocks []SyncHostInput) error {
	var body bytes.Buffer
	body.WriteString("# Managed by Bast cloud sync: do not edit by hand\n")
	for _, block := range blocks {
		body.Write(RenderSyncBlock(block))
	}
	return atomicWrite(path, body.Bytes(), 0600)
}

func LoadSyncHosts(path string) ([]SyncHostInput, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	var hosts []SyncHostInput
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, syncMarkerPrefix) || trimmed == syncMarkerEnd {
			continue
		}
		rest := strings.TrimPrefix(trimmed, syncMarkerPrefix)
		source, syncID, ok := strings.Cut(rest, "=")
		if !ok {
			continue
		}
		hostIdx := -1
		for j := i + 1; j < len(lines); j++ {
			parts, err := fields(strings.TrimSpace(lines[j]))
			if err != nil || len(parts) == 0 {
				continue
			}
			if strings.EqualFold(parts[0], "host") && len(parts) > 1 {
				hostIdx = j
				break
			}
			if strings.HasPrefix(strings.TrimSpace(lines[j]), syncMarkerPrefix) {
				break
			}
		}
		if hostIdx < 0 {
			continue
		}
		endIdx := len(lines)
		for j := hostIdx + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == syncMarkerEnd {
				endIdx = j
				break
			}
			parts, err := fields(strings.TrimSpace(lines[j]))
			if err == nil && len(parts) > 0 && strings.EqualFold(parts[0], "host") {
				endIdx = j
				break
			}
			if strings.HasPrefix(strings.TrimSpace(lines[j]), syncMarkerPrefix) {
				endIdx = j
				break
			}
		}
		aliasParts, err := fields(strings.TrimSpace(lines[hostIdx]))
		if err != nil || len(aliasParts) < 2 {
			continue
		}
		input := SyncHostInput{
			Alias:      aliasParts[1],
			SyncSource: strings.TrimSpace(source),
			SyncID:     strings.TrimSpace(syncID),
		}
		for _, line := range lines[hostIdx+1 : endIdx] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			parts, err := fields(trimmed)
			if err != nil || len(parts) == 0 {
				continue
			}
			switch strings.ToLower(parts[0]) {
			case "hostname":
				if len(parts) > 1 {
					input.HostName = parts[1]
				}
			case "user":
				if len(parts) > 1 {
					input.User = parts[1]
				}
			case "port":
				if len(parts) > 1 {
					input.Port = parts[1]
				}
			case "identityfile":
				if len(parts) > 1 {
					input.IdentityFile = parts[1]
				}
			case "certificatefile":
				if len(parts) > 1 {
					input.CertificateFile = parts[1]
				}
			case "identitiesonly":
				input.IdentitiesOnly = len(parts) > 1 && strings.EqualFold(parts[1], "yes")
			case "proxycommand":
				if idx := strings.IndexFunc(trimmed, func(r rune) bool { return r == ' ' || r == '\t' }); idx >= 0 {
					input.ProxyCommand = strings.TrimSpace(trimmed[idx+1:])
				}
			default:
				input.ExtraOptions = append(input.ExtraOptions, trimmed)
			}
		}
		hosts = append(hosts, input)
		i = endIdx
	}
	return hosts, nil
}

func UpdateSyncHostAuth(path, alias, user, identityFile, certificateFile string, identitiesOnly bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, ok := patchSyncHostAuth(data, alias, user, identityFile, certificateFile, identitiesOnly)
	if !ok {
		return fmt.Errorf("synced host %q not found in %s", alias, path)
	}
	if bytes.Equal(data, updated) {
		return nil
	}
	return atomicWriteChecked(path, data, updated, 0600)
}

func patchSyncHostAuth(data []byte, alias, user, identityFile, certificateFile string, identitiesOnly bool) ([]byte, bool) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return data, false
	}
	lines := strings.Split(string(data), "\n")
	// Track trailing newline so we can restore file shape.
	trailingNL := len(data) > 0 && data[len(data)-1] == '\n'
	if trailingNL && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	hostIdx := -1
	for i, line := range lines {
		parts, err := fields(strings.TrimSpace(line))
		if err != nil || len(parts) < 2 || !strings.EqualFold(parts[0], "host") {
			continue
		}
		for _, candidate := range parts[1:] {
			if candidate == alias {
				hostIdx = i
				break
			}
		}
		if hostIdx >= 0 {
			break
		}
	}
	if hostIdx < 0 {
		return data, false
	}

	endIdx := len(lines)
	for i := hostIdx + 1; i < len(lines); i++ {
		raw := strings.TrimSpace(lines[i])
		if raw == syncMarkerEnd {
			endIdx = i + 1 // consume the old end marker; RenderSyncBlock writes a new one
			break
		}
		parts, err := fields(raw)
		if err == nil && len(parts) > 0 && strings.EqualFold(parts[0], "host") {
			endIdx = i
			break
		}
		if strings.HasPrefix(raw, syncMarkerPrefix) && raw != syncMarkerEnd {
			endIdx = i
			break
		}
	}

	var kept []string
	kept = append(kept, lines[hostIdx])
	var hostname, port, proxyCommand string
	var extras []string
	for _, line := range lines[hostIdx+1 : endIdx] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts, err := fields(trimmed)
		if err != nil || len(parts) == 0 {
			extras = append(extras, trimmed)
			continue
		}
		switch strings.ToLower(parts[0]) {
		case "hostname":
			if len(parts) > 1 {
				hostname = parts[1]
			}
		case "port":
			if len(parts) > 1 {
				port = parts[1]
			}
		case "proxycommand":
			if idx := strings.IndexFunc(trimmed, func(r rune) bool { return r == ' ' || r == '\t' }); idx >= 0 {
				proxyCommand = strings.TrimSpace(trimmed[idx+1:])
			}
		case "user", "identityfile", "certificatefile", "identitiesonly":
		default:
			extras = append(extras, trimmed)
		}
	}

	input := SyncHostInput{
		Alias:           alias,
		User:            user,
		HostName:        hostname,
		Port:            port,
		IdentityFile:    identityFile,
		CertificateFile: certificateFile,
		IdentitiesOnly:  identitiesOnly,
		ProxyCommand:    proxyCommand,
		ExtraOptions:    extras,
	}
	// Preserve sync marker from the line before Host when present.
	syncSource, syncID := "", ""
	if hostIdx > 0 {
		prev := strings.TrimSpace(lines[hostIdx-1])
		if strings.HasPrefix(prev, syncMarkerPrefix) && prev != syncMarkerEnd {
			rest := strings.TrimPrefix(prev, syncMarkerPrefix)
			if source, id, ok := strings.Cut(rest, "="); ok {
				syncSource = strings.TrimSpace(source)
				syncID = strings.TrimSpace(id)
			}
		}
	}
	input.SyncSource = syncSource
	input.SyncID = syncID

	block := string(RenderSyncBlock(input))
	block = strings.TrimSuffix(block, "\n")
	blockLines := strings.Split(block, "\n")

	startReplace := hostIdx
	if syncSource != "" && hostIdx > 0 && strings.HasPrefix(strings.TrimSpace(lines[hostIdx-1]), syncMarkerPrefix) {
		startReplace = hostIdx - 1
	}
	out := make([]string, 0, len(lines)-(endIdx-startReplace)+len(blockLines))
	out = append(out, lines[:startReplace]...)
	out = append(out, blockLines...)
	out = append(out, lines[endIdx:]...)
	result := strings.Join(out, "\n")
	if trailingNL {
		result += "\n"
	}
	return []byte(result), true
}

// Include must come before any Host/Match blocks; otherwise OpenSSH treats it as part of the
// preceding host and synced hosts never apply.
func (m Manager) EnsureSyncInclude(path string) error {
	if path == "" {
		return errors.New("sync config path is not configured")
	}
	if err := m.EnsureManaged(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create sync directory: %w", err)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := atomicWrite(path, []byte("# Managed by Bast cloud sync: do not edit by hand\n"), 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	b, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		return err
	}
	original := append([]byte(nil), b...)
	base := filepath.Dir(m.ManagedConfig)
	cleaned := removeIncludeLines(b, path, m.Home, base)
	includeLine := []byte("Include " + m.syncIncludePath(path) + "\n")
	if hasLeadingInclude(cleaned, path, m.Home, base) {
		if bytes.Equal(cleaned, original) {
			return nil
		}
		return atomicWriteChecked(m.ManagedConfig, original, cleaned, 0600)
	}
	updated := append(includeLine, cleaned...)
	if bytes.Equal(updated, original) {
		return nil
	}
	return atomicWriteChecked(m.ManagedConfig, original, updated, 0600)
}

func (m Manager) syncIncludePath(path string) string {
	sshDir := filepath.Join(m.Home, ".ssh")
	if rel, err := filepath.Rel(sshDir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "~/.ssh/" + filepath.ToSlash(rel)
	}
	return path
}

func hasLeadingInclude(data []byte, target, home, base string) bool {
	target = cleanPath(target)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts, err := fields(raw)
		if err != nil || len(parts) == 0 {
			return false
		}
		if !strings.EqualFold(parts[0], "include") {
			return false
		}
		for _, part := range parts[1:] {
			if cleanPath(expandPath(part, home, base)) == target {
				return true
			}
		}
		return false
	}
	return false
}

func (m Manager) RemoveSyncInclude(path string) error {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	updated := removeIncludeLines(b, path, m.Home, filepath.Dir(m.ManagedConfig))
	if !bytes.Equal(b, updated) {
		if err := atomicWriteChecked(m.ManagedConfig, b, updated, 0600); err != nil {
			return err
		}
	}
	_ = os.Remove(path)
	return nil
}

func removeIncludeLines(data []byte, target, home, base string) []byte {
	target = cleanPath(target)
	var out bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		parts, err := fields(strings.TrimSpace(line))
		if err == nil && len(parts) > 1 && strings.EqualFold(parts[0], "include") {
			kept := []string{}
			for _, part := range parts[1:] {
				if cleanPath(expandPath(part, home, base)) != target {
					kept = append(kept, part)
				}
			}
			if len(kept) == 0 {
				continue
			}
			fmt.Fprintf(&out, "Include %s\n", strings.Join(kept, " "))
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes()
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
	coreManagedDirectives = map[string]bool{
		"hostname": true, "user": true, "port": true,
		"identityfile": true, "proxyjump": true,
		"pubkeyauthentication": true, "passwordauthentication": true,
		"preferredauthentications": true,
	}
	managedDirectives = map[string]bool{
		"forwardagent": true, "remotecommand": true, "requesttty": true,
		"setenv": true, "localforward": true, "remoteforward": true,
		"dynamicforward": true, "serveraliveinterval": true, "compression": true,
	}
	forbiddenDirectives = map[string]bool{
		"host": true, "match": true, "include": true,
		"proxycommand": true, "localcommand": true,
	}
)

func init() {
	for name := range coreManagedDirectives {
		managedDirectives[name] = true
	}
}

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
		if strings.EqualFold(parts[0], "Host") || coreManagedDirectives[strings.ToLower(parts[0])] {
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
