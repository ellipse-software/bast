//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestReplaceFileOverwritesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "state.json")
	staged := filepath.Join(dir, "state.tmp")
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceFile(staged, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("destination = %q", data)
	}
}

func TestWindowsPathIdentity(t *testing.T) {
	if !SamePath(`C:\Users\Ted\.ssh`, `c:\users\ted\.ssh`) {
		t.Fatal("Windows paths should compare case-insensitively")
	}
	if !PathContained(`C:\Users\Ted`, `c:\users\ted\.ssh\config`) {
		t.Fatal("child should be contained across path casing")
	}
	if PathContained(`C:\Users\Ted`, `D:\Users\Ted`) {
		t.Fatal("different volumes cannot be contained")
	}
	if !HasPathSeparator("nested/file") || !HasPathSeparator(`nested\file`) {
		t.Fatal("both Windows path separators must be rejected in a file name")
	}
}

func TestSecurePathAppliesProtectedACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SecurePath(path, 0600); err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("secret DACL still inherits permissions")
	}
}
