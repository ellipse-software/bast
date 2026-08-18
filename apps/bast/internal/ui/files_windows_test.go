//go:build windows

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"bast/internal/files"
)

func TestLocalFilesDoNotOpenPOSIXPermissionsOnWindows(t *testing.T) {
	m := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.env")
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.enterFilesSection(); err != nil {
		t.Fatal(err)
	}
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "secret.env" {
			m.files.panes[0].cursor = i
			break
		}
	}

	m.updateFilesKeys("p")
	if m.files.chmod.active {
		t.Fatal("local POSIX permissions opened on Windows")
	}
}
