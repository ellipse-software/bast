package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPathContainedRoot(t *testing.T) {
	root := string(filepath.Separator)
	child := filepath.Join(root, "tmp", "nested")
	if !LocalPathContained(root, child) {
		t.Fatalf("LocalPathContained(%q, %q) = false, want true", root, child)
	}
	if !LocalPathContained(root, root) {
		t.Fatalf("LocalPathContained(%q, %q) = false, want true", root, root)
	}
	parent := filepath.Join(root, "tmp")
	outside := filepath.Join(root, "var")
	if LocalPathContained(parent, outside) {
		t.Fatalf("LocalPathContained(%q, %q) = true, want false", parent, outside)
	}
}

func TestTransferLocalLocalCopiesSymlinkFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	dstDir := filepath.Join(root, "dst")
	if err := os.WriteFile(target, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := TransferAny(context.Background(), Endpoint{}, Endpoint{}, []string{link}, dstDir, false, func(Progress) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dstDir, "link.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("copied = %q, want payload", got)
	}
}

func TestCleanAndJoinRemote(t *testing.T) {
	got, err := CleanRemote("/tmp/../var//log")
	if err != nil || got != "/var/log" {
		t.Fatalf("CleanRemote = %q %v", got, err)
	}
	got, err = JoinRemote("/var/log", "app.log")
	if err != nil || got != "/var/log/app.log" {
		t.Fatalf("JoinRemote = %q %v", got, err)
	}
	got, err = ParentRemote("/var/log/app.log")
	if err != nil || got != "/var/log" {
		t.Fatalf("ParentRemote = %q %v", got, err)
	}
	if _, err := JoinRemote("/tmp", "../x"); err == nil {
		t.Fatal("expected invalid join")
	}
}

func TestLocalListMkdirRenameRemoveCopy(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ListLocal(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "a.txt" {
		t.Fatalf("list hidden=false = %+v", entries)
	}
	entries, err = ListLocal(src, true)
	if err != nil || len(entries) != 2 {
		t.Fatalf("list hidden=true = %+v %v", entries, err)
	}
	nested := filepath.Join(src, "dir")
	if err := MkdirLocal(nested); err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(src, "dir2")
	if err := RenameLocal(nested, renamed); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "dst")
	if err := CopyLocal(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLocal(dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("expected removed, got %v", err)
	}
}

func TestShellCommandRequiresDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ShellCommand(file); err == nil {
		t.Fatal("expected non-directory rejection")
	}
	cmd, err := ShellCommand(root)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir != root {
		t.Fatalf("Dir = %q", cmd.Dir)
	}
}

func TestCopyLocalRejectsSamePath(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyLocal(file, file); err == nil {
		t.Fatal("expected same-path rejection")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("file truncated to %q", data)
	}
}

func TestTransferLocalLocalProgressCountsNested(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var last Progress
	err := TransferAny(context.Background(), Endpoint{}, Endpoint{}, []string{src}, dstDir, false, func(p Progress) error {
		last = p
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if last.Total < 3 {
		t.Fatalf("total = %d, want nested items", last.Total)
	}
	if last.Done != last.Total {
		t.Fatalf("done=%d total=%d", last.Done, last.Total)
	}
	if last.Bytes == 0 {
		t.Fatal("expected bytes progress for local transfer")
	}
}
