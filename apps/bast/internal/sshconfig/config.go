package sshconfig

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	markerPrefix     = "# bast:id="
	markerEnd        = "# bast:end"
	syncMarkerPrefix = "# bast:sync:"
	syncMarkerEnd    = "# bast:sync:end"
)

type Host struct {
	Alias      string
	Source     string
	Line       int
	Managed    bool
	ManagedID  string
	Synced     bool
	SyncSource string
	SyncID     string
	KnownHost  bool
	Resolved   Resolved
}

type Resolved struct {
	HostName                 string
	User                     string
	Port                     string
	IdentityFiles            []string
	IdentitiesOnly           string
	PubkeyAuthentication     string
	PasswordAuthentication   string
	PreferredAuthentications string
	ProxyJump                string
}

type HostInput struct {
	Alias        string
	HostName     string
	User         string
	Port         string
	IdentityFile string
	ExtraOptions []string
	PasswordOnly bool
	ProxyJump    string
}

type Manager struct {
	Home            string
	MainConfig      string
	ManagedDir      string
	ManagedConfig   string
	ManagedKeys     string
	SyncGCPConfig   string
	SyncAWSConfig   string
	SyncAzureConfig string
}

type SyncHostInput struct {
	Alias           string
	SyncSource      string
	SyncID          string
	HostName        string
	User            string
	Port            string
	IdentityFile    string
	CertificateFile string
	IdentitiesOnly  bool
	ProxyCommand    string
	ExtraOptions    []string
}

func (m Manager) Discover() ([]Host, error) {
	if _, err := os.Stat(m.MainConfig); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var found []Host
	stack := map[string]bool{}
	if err := m.scanFile(m.MainConfig, 0, stack, &found); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]Host, 0, len(found))
	for _, host := range found {
		if !seen[host.Alias] {
			seen[host.Alias] = true
			out = append(out, host)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Alias) < strings.ToLower(out[j].Alias)
	})
	return out, nil
}

func (m Manager) scanFile(path string, depth int, stack map[string]bool, found *[]Host) error {
	if depth > 8 {
		return fmt.Errorf("SSH config include depth exceeded at %s", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if stack[abs] {
		return fmt.Errorf("cyclic SSH config include at %s", path)
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	stack[abs] = true
	defer delete(stack, abs)

	managedID := ""
	syncSource, syncID := "", ""
	var active []int
	baseDir := filepath.Dir(path)
	scanner := bufio.NewScanner(bytes.NewReader(b))
	// SSH configs can contain long ProxyCommand lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(raw, markerPrefix) {
			managedID = strings.TrimSpace(strings.TrimPrefix(raw, markerPrefix))
			continue
		}
		if raw == markerEnd {
			managedID = ""
			continue
		}
		if strings.HasPrefix(raw, syncMarkerPrefix) {
			if raw == syncMarkerEnd {
				syncSource, syncID = "", ""
			} else {
				rest := strings.TrimPrefix(raw, syncMarkerPrefix)
				if source, id, ok := strings.Cut(rest, "="); ok {
					syncSource = strings.TrimSpace(source)
					syncID = strings.TrimSpace(id)
				}
			}
			continue
		}
		fields, err := fields(raw)
		if err != nil || len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "include":
			active = nil
			for _, pattern := range fields[1:] {
				pattern = expandPath(pattern, m.Home, filepath.Dir(m.MainConfig))
				matches, globErr := filepath.Glob(pattern)
				if globErr != nil {
					return fmt.Errorf("invalid Include pattern %q in %s:%d", pattern, path, lineNo)
				}
				sort.Strings(matches)
				for _, match := range matches {
					if err := m.scanFile(match, depth+1, stack, found); err != nil {
						return err
					}
				}
			}
		case "host":
			active = active[:0]
			for _, alias := range fields[1:] {
				if selectableAlias(alias) {
					*found = append(*found, Host{
						Alias: alias, Source: path, Line: lineNo,
						Managed: path == m.ManagedConfig && managedID != "", ManagedID: managedID,
						Synced: syncID != "", SyncSource: syncSource, SyncID: syncID,
					})
					active = append(active, len(*found)-1)
				}
			}
		default:
			if len(active) == 0 || len(fields) < 2 {
				continue
			}
			applyHostDirective((*found), active, fields, m.Home, baseDir)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}
	return nil
}

// Parse common directives here so the UI can connect without waiting for `ssh -G`.
func applyHostDirective(hosts []Host, active []int, fields []string, home, baseDir string) {
	key := strings.ToLower(fields[0])
	value := fields[1]
	for _, idx := range active {
		resolved := &hosts[idx].Resolved
		switch key {
		case "hostname":
			resolved.HostName = value
		case "user":
			resolved.User = value
		case "port":
			resolved.Port = value
		case "identityfile":
			resolved.IdentityFiles = append(resolved.IdentityFiles, expandPath(value, home, baseDir))
		case "identitiesonly":
			resolved.IdentitiesOnly = value
		case "pubkeyauthentication":
			resolved.PubkeyAuthentication = value
		case "passwordauthentication":
			resolved.PasswordAuthentication = value
		case "preferredauthentications":
			resolved.PreferredAuthentications = strings.Join(fields[1:], " ")
		case "proxyjump":
			resolved.ProxyJump = value
		}
	}
}

func (m Manager) EnsureManaged() error {
	if err := os.MkdirAll(m.ManagedKeys, 0700); err != nil {
		return fmt.Errorf("create Bast SSH directory: %w", err)
	}
	if err := os.Chmod(m.ManagedDir, 0700); err != nil {
		return err
	}
	if _, err := os.Stat(m.ManagedConfig); errors.Is(err, os.ErrNotExist) {
		if err := atomicWrite(m.ManagedConfig, nil, 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	b, err := os.ReadFile(m.MainConfig)
	if errors.Is(err, os.ErrNotExist) {
		content := []byte("# Added by Bast\nInclude ~/.ssh/bast/config\n")
		return atomicWrite(m.MainConfig, content, 0600)
	}
	if err != nil {
		return err
	}
	if hasInclude(b, m.ManagedConfig, m.Home, filepath.Dir(m.MainConfig)) {
		return nil
	}
	prefix := []byte("# Added by Bast\nInclude ~/.ssh/bast/config\n\n")
	return atomicWriteChecked(m.MainConfig, b, append(prefix, b...), fileMode(m.MainConfig, 0600))
}

func (m Manager) Add(input HostInput) (Host, error) {
	if err := Validate(input); err != nil {
		return Host{}, err
	}
	hosts, err := m.Discover()
	if err != nil {
		return Host{}, err
	}
	for _, host := range hosts {
		if host.Alias == input.Alias {
			return Host{}, fmt.Errorf("host label %q already exists in %s", input.Alias, host.Source)
		}
	}
	if err := m.EnsureManaged(); err != nil {
		return Host{}, err
	}
	id, err := newID()
	if err != nil {
		return Host{}, err
	}
	b, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		return Host{}, err
	}
	original := append([]byte(nil), b...)
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	if len(bytes.TrimSpace(b)) > 0 {
		b = append(b, '\n')
	}
	b = append(b, renderBlock(id, input)...)
	if err := atomicWriteChecked(m.ManagedConfig, original, b, 0600); err != nil {
		return Host{}, err
	}
	return Host{Alias: input.Alias, Source: m.ManagedConfig, Managed: true, ManagedID: id}, nil
}

func (m Manager) Update(id string, input HostInput) error {
	if id == "" {
		return errors.New("cannot edit an externally managed host")
	}
	if err := Validate(input); err != nil {
		return err
	}
	hosts, err := m.Discover()
	if err != nil {
		return err
	}
	for _, host := range hosts {
		if host.Alias == input.Alias && host.ManagedID != id {
			return fmt.Errorf("host label %q already exists in %s", input.Alias, host.Source)
		}
	}
	b, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		return err
	}
	updated, ok := replaceBlock(b, id, renderBlock(id, input))
	if !ok {
		return fmt.Errorf("managed host block %q no longer exists; reload and try again", id)
	}
	return atomicWriteChecked(m.ManagedConfig, b, updated, 0600)
}

func (m Manager) ManagedExtras(id string) ([]string, error) {
	if id == "" {
		return nil, nil
	}
	b, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		return nil, err
	}
	return extractBlockExtras(b, id), nil
}

func (m Manager) Delete(id string) error {
	if id == "" {
		return errors.New("cannot delete an externally managed host")
	}
	b, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		return err
	}
	updated, ok := replaceBlock(b, id, nil)
	if !ok {
		return fmt.Errorf("managed host block %q no longer exists; reload and try again", id)
	}
	updated = bytes.TrimSpace(updated)
	if len(updated) > 0 {
		updated = append(updated, '\n')
	}
	return atomicWriteChecked(m.ManagedConfig, b, updated, 0600)
}

func Validate(input HostInput) error {
	if !selectableAlias(input.Alias) || strings.HasPrefix(input.Alias, "-") {
		return errors.New("label must be a literal SSH name without spaces, wildcards, or a leading dash")
	}
	if strings.TrimSpace(input.HostName) == "" {
		return errors.New("hostname is required")
	}
	for name, value := range map[string]string{"hostname": input.HostName, "user": input.User, "identity file": input.IdentityFile, "proxy jump": input.ProxyJump} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s cannot contain a newline or null byte", name)
		}
	}
	for name, value := range map[string]string{"hostname": input.HostName, "user": input.User, "proxy jump": input.ProxyJump} {
		if strings.ContainsAny(value, " \t") {
			return fmt.Errorf("%s cannot contain whitespace", name)
		}
	}
	if input.Port != "" {
		port, err := strconv.Atoi(input.Port)
		if err != nil || port < 1 || port > 65535 {
			return errors.New("port must be between 1 and 65535")
		}
	}
	return validateHostExtraOptions(input.ExtraOptions)
}

func validateHostExtraOptions(options []string) error {
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
		if coreManagedDirectives[name] {
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

func NormalizeAlias(label string) string {
	return strings.Join(strings.Fields(label), "_")
}
