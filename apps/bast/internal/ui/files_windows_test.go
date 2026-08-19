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
	_ = m.enterFilesSection()
	entries, err := files.ListLocal(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	applyFilesList(m, 0, dir, entries)
	found := false
	for i, entry := range m.files.panes[0].entries {
		if entry.Name == "secret.env" {
			m.files.panes[0].cursor = i
			found = true
			break
		}
	}
	if !found {
		t.Fatal("secret.env was not present in the local file pane")
	}

	m.updateFilesKeys("p")
	if m.files.chmod.active {
		t.Fatal("local POSIX permissions opened on Windows")
	}
}
