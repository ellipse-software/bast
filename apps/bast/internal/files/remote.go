package files

import (
	"os"
	"strings"

	"github.com/pkg/sftp"
)

// ListRemote lists directory entries at dir on the session.
func ListRemote(session *Session, dir string, showHidden bool) ([]Entry, error) {
	dir, err := CleanRemote(dir)
	if err != nil {
		return nil, err
	}
	entries, err := session.client.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, info := range entries {
		name := info.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		full, err := JoinRemote(dir, name)
		if err != nil {
			continue
		}
		out = append(out, Entry{
			Name:    name,
			Path:    full,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		})
	}
	sortEntries(out)
	return out, nil
}

// MkdirRemote creates a directory on the remote session.
func MkdirRemote(session *Session, path string) error {
	path, err := CleanRemote(path)
	if err != nil {
		return err
	}
	return session.client.Mkdir(path)
}

// RenameRemote renames a remote path.
func RenameRemote(session *Session, oldPath, newPath string) error {
	oldPath, err := CleanRemote(oldPath)
	if err != nil {
		return err
	}
	newPath, err = CleanRemote(newPath)
	if err != nil {
		return err
	}
	return session.client.Rename(oldPath, newPath)
}

// RemoveRemote removes a remote file or recursively removes a directory.
func RemoveRemote(session *Session, path string) error {
	path, err := CleanRemote(path)
	if err != nil {
		return err
	}
	info, err := session.client.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return session.client.Remove(path)
	}
	return removeRemoteDir(session.client, path)
}

func removeRemoteDir(client *sftp.Client, dir string) error {
	entries, err := client.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range entries {
		child, err := JoinRemote(dir, info.Name())
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := removeRemoteDir(client, child); err != nil {
				return err
			}
			continue
		}
		if err := client.Remove(child); err != nil {
			return err
		}
	}
	return client.Remove(dir)
}

// StatRemote returns file info for a remote path.
func StatRemote(session *Session, path string) (os.FileInfo, error) {
	path, err := CleanRemote(path)
	if err != nil {
		return nil, err
	}
	return session.client.Stat(path)
}
