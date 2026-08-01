package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAndFormatMode(t *testing.T) {
	mode, err := ParseModeOctal("755")
	if err != nil {
		t.Fatal(err)
	}
	if mode.Perm() != 0o755 {
		t.Fatalf("mode = %04o", mode.Perm())
	}
	if got := FormatModeOctal(mode); got != "0755" {
		t.Fatalf("FormatModeOctal = %q", got)
	}
	if got := FormatModeSymbolic(mode); got != "rwxr-xr-x" {
		t.Fatalf("FormatModeSymbolic = %q", got)
	}
	mode, err = ParseModeOctal("0644")
	if err != nil || mode.Perm() != 0o644 {
		t.Fatalf("0644 => %04o %v", mode.Perm(), err)
	}
	if _, err := ParseModeOctal("999"); err == nil {
		t.Fatal("expected invalid octal")
	}
	if _, err := ParseModeOctal("75"); err == nil {
		t.Fatal("expected short mode rejection")
	}
}

func TestFormatSizeAndEntryKind(t *testing.T) {
	if got := FormatSize(128); got != "128 B" {
		t.Fatalf("FormatSize(128) = %q", got)
	}
	if got := FormatSize(1500); got != "1.5 KB" {
		t.Fatalf("FormatSize(1500) = %q", got)
	}
	if got := FormatSize(12_000_000); got != "12 MB" {
		t.Fatalf("FormatSize(12e6) = %q", got)
	}
	file := Entry{Name: "a.txt", Mode: 0o644}
	if got := EntryKind(file); got != "file" {
		t.Fatalf("EntryKind file = %q", got)
	}
	dir := Entry{Name: "dir", IsDir: true, Mode: 0o755 | os.ModeDir}
	if got := EntryKind(dir); got != "directory" {
		t.Fatalf("EntryKind dir = %q", got)
	}
}

func TestChmodLocalAndRecursive(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	dir := filepath.Join(root, "dir")
	nested := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(file, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ChmodLocal(file, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %04o", info.Mode().Perm())
	}
	if err := ChmodLocalRecursive(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("dir mode = %04o", info.Mode().Perm())
	}
	info, err = os.Stat(nested)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("nested mode = %04o", info.Mode().Perm())
	}
}
