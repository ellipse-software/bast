package files

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ListLocal lists directory entries at dir. When showHidden is false, names
// starting with '.' are omitted.
func ListLocal(dir string, showHidden bool) ([]Entry, error) {
	dir, err := CleanLocal(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, item := range entries {
		name := item.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{
			Name:    name,
			Path:    filepath.Join(dir, name),
			IsDir:   item.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		})
	}
	sortEntries(out)
	return out, nil
}

// MkdirLocal creates a directory at path.
func MkdirLocal(path string) error {
	path, err := CleanLocal(path)
	if err != nil {
		return err
	}
	return os.Mkdir(path, 0o755)
}

// RenameLocal renames oldPath to newPath.
func RenameLocal(oldPath, newPath string) error {
	oldPath, err := CleanLocal(oldPath)
	if err != nil {
		return err
	}
	newPath, err = CleanLocal(newPath)
	if err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

// RemoveLocal removes a file or recursively removes a directory.
func RemoveLocal(path string) error {
	path, err := CleanLocal(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// CopyLocal copies src to dst. Directories are copied recursively.
func CopyLocal(src, dst string) error {
	src, err := CleanLocal(src)
	if err != nil {
		return err
	}
	dst, err = CleanLocal(dst)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyLocalDir(src, dst, info.Mode())
	}
	return copyLocalFile(src, dst, info.Mode())
}

func copyLocalDir(src, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(dst, mode.Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, item := range entries {
		from := filepath.Join(src, item.Name())
		to := filepath.Join(dst, item.Name())
		info, err := item.Info()
		if err != nil {
			return err
		}
		if item.IsDir() {
			if err := copyLocalDir(from, to, info.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := copyLocalFile(from, to, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func copyLocalFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}
