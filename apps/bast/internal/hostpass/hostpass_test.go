package hostpass

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveReadDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "passwords")
	if Exists(dir, "abc123") {
		t.Fatal("missing file reported as present")
	}
	if err := Save(dir, "abc123", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir, "abc123") {
		t.Fatal("saved password missing")
	}
	got, err := Read(dir, "abc123")
	if err != nil || got != "s3cret" {
		t.Fatalf("Read = %q %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "abc123"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("file mode = %v", info.Mode())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && dirInfo.Mode().Perm() != 0700 {
		t.Fatalf("dir mode = %v", dirInfo.Mode())
	}
	if err := Delete(dir, "abc123"); err != nil {
		t.Fatal(err)
	}
	if Exists(dir, "abc123") {
		t.Fatal("deleted password still present")
	}
	if err := Delete(dir, "abc123"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestSaveRejectsEmptyAndMultiline(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "abc123", ""); err == nil {
		t.Fatal("expected empty password to fail")
	}
	if err := Save(dir, "abc123", "one\ntwo"); err == nil {
		t.Fatal("expected multiline password to fail")
	}
}

func TestPathRejectsUnsafeIDs(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"", "../etc", "a/b", "a\\b", ".hidden", "-dash", "id with space", strings.Repeat("a", 129)} {
		if _, err := Path(dir, id); err == nil {
			t.Fatalf("accepted %q", id)
		}
		if Exists(dir, id) {
			t.Fatalf("Exists accepted %q", id)
		}
	}
	if _, err := Path(dir, "abc-123_def.9"); err != nil {
		t.Fatal(err)
	}
}

func TestLooksLikePassword(t *testing.T) {
	allow := []string{
		"user@host's password:",
		"Password:",
		"Enter password for deploy@db.example",
	}
	for _, prompt := range allow {
		if !LooksLikePassword(prompt) {
			t.Fatalf("should allow %q", prompt)
		}
	}
	deny := []string{
		"",
		"Enter passphrase for key '/home/ted/.ssh/id_ed25519':",
		"Verification code:",
		"One-time password (OTP):",
		"TOTP token",
		"Challenge response",
		"Authenticator PIN",
		"Passcode:",
	}
	for _, prompt := range deny {
		if LooksLikePassword(prompt) {
			t.Fatalf("should deny %q", prompt)
		}
	}
}

func TestPrint(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "abc123", "s3cret"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Print(&buf, dir, "abc123", "user@host's password:", "host"); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "s3cret\n" {
		t.Fatalf("Print = %q", got)
	}
	if err := Print(&buf, dir, "abc123", "Verification code:", "host"); err == nil {
		t.Fatal("expected OTP prompt to fail")
	}
	if strings.Contains(buf.String(), "Verification") {
		t.Fatal("OTP refusal should not write extra output")
	}
	if err := Print(&buf, dir, "abc123", "jump@bastion.example's password:", "host"); err == nil {
		t.Fatal("expected jump-host prompt to fail")
	}
}

func TestSaveDoesNotClobberDotTmpID(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "abc.tmp", "keep-me"); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, "abc", "other"); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir, "abc.tmp")
	if err != nil || got != "keep-me" {
		t.Fatalf("abc.tmp = %q %v", got, err)
	}
	got, err = Read(dir, "abc")
	if err != nil || got != "other" {
		t.Fatalf("abc = %q %v", got, err)
	}
}

func TestMatchesDestination(t *testing.T) {
	if !MatchesDestination("Password:", "legacy.example") {
		t.Fatal("generic password prompt should be allowed")
	}
	if !MatchesDestination("deploy@legacy.example's password:", "legacy.example", "legacy") {
		t.Fatal("destination host prompt should match")
	}
	if !MatchesDestination("deploy@legacy's password:", "legacy.example", "legacy") {
		t.Fatal("destination alias prompt should match")
	}
	if MatchesDestination("jump@bastion.example's password:", "legacy.example", "legacy") {
		t.Fatal("jump host prompt should not match")
	}
	if MatchesDestination("deploy@legacy.example's password:") {
		t.Fatal("named prompt with no expected host should not match")
	}
}
