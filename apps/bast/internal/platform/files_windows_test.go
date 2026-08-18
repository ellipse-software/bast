//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

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
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl.AceCount != 3 {
		t.Fatalf("secret DACL has %d entries, want 3", dacl.AceCount)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	administrators, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		user.User.Sid.String():  false,
		system.String():         false,
		administrators.String(): false,
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("ACE %d is not an allow entry", i)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if _, ok := want[sid]; !ok {
			t.Fatalf("unexpected SID in secret DACL: %s", sid)
		}
		want[sid] = true
	}
	for sid, found := range want {
		if !found {
			t.Fatalf("expected SID is missing from secret DACL: %s", sid)
		}
	}
}
