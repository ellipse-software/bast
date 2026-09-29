package sshconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MigrateBoatSync copies the inventory before switching Includes, then removes
// the old file. Each step can be retried after interruption. Disabled inventories
// stay disabled, and conflicting inventories are never overwritten.
func (m Manager) MigrateBoatSync() error {
	if m.SyncBoatConfig == "" || m.ManagedDir == "" {
		return nil
	}
	legacy := filepath.Join(m.ManagedDir, "sync", "box", "config")
	original, err := os.ReadFile(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy Boat inventory: %w", err)
	}
	updated := migrateBoatInventory(original)
	current, err := os.ReadFile(m.SyncBoatConfig)
	if err == nil && !bytes.Equal(current, updated) {
		return fmt.Errorf("Boat migration found conflicting inventories at %s and %s; reconcile them before retrying", legacy, m.SyncBoatConfig)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read Boat inventory: %w", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		if err := atomicWrite(m.SyncBoatConfig, updated, 0600); err != nil {
			return fmt.Errorf("write Boat inventory: %w", err)
		}
	}
	for _, path := range []string{m.MainConfig, m.ManagedConfig} {
		if path == "" {
			continue
		}
		before, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read Boat sync Include: %w", err)
		}
		after := migrateBoatInclude(before, legacy, m.syncIncludePath(m.SyncBoatConfig), m.Home, filepath.Dir(path))
		if !bytes.Equal(before, after) {
			if err := atomicWriteChecked(path, before, after, fileMode(path, 0600)); err != nil {
				return fmt.Errorf("migrate Boat sync Include: %w", err)
			}
		}
	}
	if err := os.Remove(legacy); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove migrated Box inventory: %w", err)
	}
	return nil
}

func migrateBoatInventory(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, syncMarkerPrefix+"box=") {
			lines[i] = strings.Replace(line, syncMarkerPrefix+"box=", syncMarkerPrefix+"boat=", 1)
		}
		parts, err := fields(trimmed)
		if err == nil && len(parts) == 2 && strings.EqualFold(parts[0], "hostname") && parts[1] == "box.stopped.invalid" {
			lines[i] = strings.Replace(line, "box.stopped.invalid", "boat.stopped.invalid", 1)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func migrateBoatInclude(content []byte, legacy, replacement, home, base string) []byte {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		parts, err := fields(strings.TrimSpace(line))
		if err != nil || len(parts) < 2 || !strings.EqualFold(parts[0], "include") {
			continue
		}
		changed := false
		for j := 1; j < len(parts); j++ {
			if cleanPath(expandPath(parts[j], home, base)) == cleanPath(legacy) {
				parts[j] = replacement
				changed = true
			}
		}
		if changed {
			for j := 1; j < len(parts); j++ {
				parts[j] = configValue(parts[j])
			}
			lines[i] = strings.Join(parts, " ")
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func canonicalSyncSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "box" {
		return "boat"
	}
	return source
}
