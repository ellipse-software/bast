//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
)

func ReplaceFile(staged, destination string) error {
	return os.Rename(staged, destination)
}

func SyncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func SecurePath(path string, mode os.FileMode) error {
	return os.Chmod(path, mode.Perm())
}

func SupportsPOSIXPermissions() bool { return true }

func SamePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func PathContained(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return true
	}
	sep := string(filepath.Separator)
	if strings.HasSuffix(parent, sep) {
		return strings.HasPrefix(child, parent)
	}
	return strings.HasPrefix(child, parent+sep)
}

func HasPathSeparator(value string) bool {
	return strings.ContainsRune(value, filepath.Separator)
}
