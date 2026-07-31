package files

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	errEmptyPath   = errors.New("path is required")
	errInvalidPath = errors.New("invalid path")
	errSamePath    = errors.New("source and destination are the same")
)

// SameLocalPath reports whether two local paths resolve to the same location.
func SameLocalPath(a, b string) bool {
	ca, errA := CleanLocal(a)
	cb, errB := CleanLocal(b)
	if errA != nil || errB != nil {
		return false
	}
	return ca == cb
}

// SameRemotePath reports whether two remote paths resolve to the same location.
func SameRemotePath(a, b string) bool {
	ca, errA := CleanRemote(a)
	cb, errB := CleanRemote(b)
	if errA != nil || errB != nil {
		return false
	}
	return ca == cb
}

// LocalPathContained reports whether child is equal to parent or nested under it.
func LocalPathContained(parent, child string) bool {
	parent, err := CleanLocal(parent)
	if err != nil {
		return false
	}
	child, err = CleanLocal(child)
	if err != nil {
		return false
	}
	if parent == child {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(child, parent+sep)
}

// RemotePathContained reports whether child is equal to parent or nested under it.
func RemotePathContained(parent, child string) bool {
	parent, err := CleanRemote(parent)
	if err != nil {
		return false
	}
	child, err = CleanRemote(child)
	if err != nil {
		return false
	}
	if parent == child {
		return true
	}
	if parent == "/" {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}

// CleanRemote returns a cleaned absolute remote (slash) path.
func CleanRemote(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errEmptyPath
	}
	if strings.ContainsAny(p, "\x00") {
		return "", errInvalidPath
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(p, "/"))
	if cleaned == "." {
		return "/", nil
	}
	return cleaned, nil
}

// JoinRemote joins base and name as a remote slash path.
func JoinRemote(base, name string) (string, error) {
	base, err := CleanRemote(base)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.ContainsAny(name, "\x00") {
		return "", errInvalidPath
	}
	if base == "/" {
		return "/" + name, nil
	}
	return base + "/" + name, nil
}

// ParentRemote returns the parent of a remote path.
func ParentRemote(p string) (string, error) {
	cleaned, err := CleanRemote(p)
	if err != nil {
		return "", err
	}
	if cleaned == "/" {
		return "/", nil
	}
	parent := path.Dir(cleaned)
	if parent == "." {
		return "/", nil
	}
	return parent, nil
}

// CleanLocal returns a cleaned absolute local path.
func CleanLocal(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errEmptyPath
	}
	if strings.ContainsAny(p, "\x00") {
		return "", errInvalidPath
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~/"))
	} else if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = home
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// JoinLocal joins base and name as a local path.
func JoinLocal(base, name string) (string, error) {
	base, err := CleanLocal(base)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) || strings.ContainsAny(name, "\x00") {
		return "", errInvalidPath
	}
	return filepath.Join(base, name), nil
}

// ParentLocal returns the parent of a local path.
func ParentLocal(p string) (string, error) {
	cleaned, err := CleanLocal(p)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		return cleaned, nil
	}
	return parent, nil
}

// BaseName returns the final path element for local or remote paths.
func BaseName(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" {
		return "/"
	}
	return path.Base(p)
}
