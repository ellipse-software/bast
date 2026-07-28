package keys

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"bast/internal/paths"
)

type Key struct {
	Name        string
	PrivatePath string
	PublicPath  string
	Fingerprint string
	Algorithm   string
	Comment     string
	Managed     bool
	InAgent     bool
	References  []string
}

type Manager struct {
	Paths     paths.Paths
	SSHKeygen string
	SSHAdd    string
}

func (m Manager) Discover(ctx context.Context, referenced map[string][]string) ([]Key, error) {
	candidates := map[string]*Key{}
	for _, dir := range []string{m.Paths.SSHDir, m.Paths.ManagedKeys} {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if strings.HasSuffix(path, ".pub") {
				private := strings.TrimSuffix(path, ".pub")
				key := ensure(candidates, private)
				key.PublicPath = path
				if regular(private) {
					key.PrivatePath = private
				}
				continue
			}
			if looksPrivate(path) {
				key := ensure(candidates, path)
				key.PrivatePath = path
				if regular(path + ".pub") {
					key.PublicPath = path + ".pub"
				}
			}
		}
	}
	for path, aliases := range referenced {
		path = expandHome(path, m.Paths.Home)
		if !regular(path) && !regular(path+".pub") {
			continue
		}
		private := strings.TrimSuffix(path, ".pub")
		key := ensure(candidates, private)
		if regular(private) {
			key.PrivatePath = private
		}
		if regular(private + ".pub") {
			key.PublicPath = private + ".pub"
		}
		key.References = appendUnique(key.References, aliases...)
	}

	agentPrints, agentOnly := m.agentKeys(ctx)
	candidateKeys := make([]*Key, 0, len(candidates))
	for _, key := range candidates {
		key.Managed = within(key.PrivatePath, m.Paths.ManagedKeys) || within(key.PublicPath, m.Paths.ManagedKeys)
		candidateKeys = append(candidateKeys, key)
	}
	jobs := make(chan *Key)
	var workers sync.WaitGroup
	for range min(8, len(candidateKeys)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for key := range jobs {
				inspect := key.PublicPath
				if inspect == "" {
					inspect = key.PrivatePath
				}
				if inspect != "" {
					key.Fingerprint, key.Comment, key.Algorithm = m.inspect(ctx, inspect)
					key.InAgent = agentPrints[key.Fingerprint]
				}
			}
		}()
	}
	for _, key := range candidateKeys {
		jobs <- key
	}
	close(jobs)
	workers.Wait()

	keys := make([]Key, 0, len(candidateKeys))
	for _, key := range candidateKeys {
		sort.Strings(key.References)
		keys = append(keys, *key)
	}
	for fingerprint, agentKey := range agentOnly {
		if _, represented := agentPrints[fingerprint]; represented {
			found := false
			for _, key := range keys {
				if key.Fingerprint == fingerprint {
					found = true
					break
				}
			}
			if !found {
				keys = append(keys, agentKey)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Managed != keys[j].Managed {
			return keys[i].Managed
		}
		return strings.ToLower(keys[i].Name) < strings.ToLower(keys[j].Name)
	})
	return keys, nil
}

func (m Manager) inspect(ctx context.Context, path string) (fingerprint, comment, algorithm string) {
	cmd := exec.CommandContext(ctx, m.SSHKeygen, "-lf", path)
	out, err := cmd.Output()
	if err != nil {
		return "", "", "unknown"
	}
	parts := strings.Fields(string(out))
	if len(parts) >= 2 {
		fingerprint = parts[1]
	}
	if len(parts) >= 4 {
		algorithm = strings.Trim(parts[len(parts)-1], "()")
		comment = strings.Join(parts[2:len(parts)-1], " ")
	}
	return
}

func (m Manager) agentKeys(ctx context.Context) (map[string]bool, map[string]Key) {
	result := map[string]bool{}
	keys := map[string]Key{}
	cmd := exec.CommandContext(ctx, m.SSHAdd, "-l")
	out, err := cmd.Output()
	if err != nil {
		return result, keys
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 2 {
			result[parts[1]] = true
			comment := "agent key"
			algorithm := "unknown"
			if len(parts) >= 4 {
				comment = strings.Join(parts[2:len(parts)-1], " ")
				algorithm = strings.Trim(parts[len(parts)-1], "()")
			}
			name := comment
			if name == "" || name == "agent key" {
				name = parts[1]
			}
			keys[parts[1]] = Key{Name: name, Fingerprint: parts[1], Algorithm: algorithm, Comment: comment, InAgent: true}
		}
	}
	return result, keys
}

func ensure(keys map[string]*Key, path string) *Key {
	path = filepath.Clean(path)
	if key, ok := keys[path]; ok {
		return key
	}
	key := &Key{Name: filepath.Base(path)}
	keys[path] = key
	return key
}

func looksPrivate(path string) bool {
	base := filepath.Base(path)
	switch base {
	case "config", "known_hosts", "known_hosts.old", "authorized_keys", "authorized_keys2", "environment":
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	r := bufio.NewReader(io.LimitReader(f, 256))
	line, _ := r.ReadString('\n')
	return strings.Contains(line, "PRIVATE KEY")
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.HasPrefix(name, "-") || strings.ContainsAny(name, "\x00\r\n/\\") {
		return errors.New("key name must be a simple file name without slashes or a leading dash")
	}
	return nil
}

func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func within(path, dir string) bool {
	if path == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func expandHome(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func appendUnique(items []string, values ...string) []string {
	seen := map[string]bool{}
	for _, item := range items {
		seen[item] = true
	}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			items = append(items, value)
		}
	}
	return items
}

func copyFile(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(destination)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
