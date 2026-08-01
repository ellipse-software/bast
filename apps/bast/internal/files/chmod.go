package files

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// permissionBits are the FileMode flags chmod can set.
const permissionBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky

// NormalizeMode keeps only permission, setuid, setgid, and sticky bits.
func NormalizeMode(mode os.FileMode) os.FileMode {
	return mode & permissionBits
}

// ParseModeOctal parses a Unix permission mode from 3 or 4 octal digits.
// A leading digit sets sticky/setuid/setgid (for example 4755 → setuid + 0755).
func ParseModeOctal(raw string) (os.FileMode, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("mode is required")
	}
	if strings.HasPrefix(raw, "0o") || strings.HasPrefix(raw, "0O") {
		raw = raw[2:]
	}
	if len(raw) < 3 || len(raw) > 4 {
		return 0, fmt.Errorf("mode must be 3 or 4 octal digits")
	}
	for _, c := range raw {
		if c < '0' || c > '7' {
			return 0, fmt.Errorf("mode must be octal")
		}
	}
	value, err := strconv.ParseUint(raw, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("mode must be octal")
	}
	return fileModeFromUnix(value), nil
}

func fileModeFromUnix(value uint64) os.FileMode {
	mode := os.FileMode(value & 0o777)
	if value&0o4000 != 0 {
		mode |= os.ModeSetuid
	}
	if value&0o2000 != 0 {
		mode |= os.ModeSetgid
	}
	if value&0o1000 != 0 {
		mode |= os.ModeSticky
	}
	return mode
}

func unixFromFileMode(mode os.FileMode) uint32 {
	n := uint32(mode.Perm())
	if mode&os.ModeSetuid != 0 {
		n |= 0o4000
	}
	if mode&os.ModeSetgid != 0 {
		n |= 0o2000
	}
	if mode&os.ModeSticky != 0 {
		n |= 0o1000
	}
	return n
}

// FormatModeOctal returns a 4-digit octal permission string (e.g. "0644", "4755").
func FormatModeOctal(mode os.FileMode) string {
	return fmt.Sprintf("%04o", unixFromFileMode(mode))
}

// FormatModeSymbolic returns a 9-character rwx string (e.g. "rw-r--r--").
// Setuid/setgid/sticky are shown with the usual s/S/t forms on the exec bits.
func FormatModeSymbolic(mode os.FileMode) string {
	perm := mode.Perm()
	chars := []byte{'-', '-', '-', '-', '-', '-', '-', '-', '-'}
	bits := []os.FileMode{0400, 0200, 0100, 0040, 0020, 0010, 0004, 0002, 0001}
	letters := []byte{'r', 'w', 'x', 'r', 'w', 'x', 'r', 'w', 'x'}
	for i, bit := range bits {
		if perm&bit != 0 {
			chars[i] = letters[i]
		}
	}
	if mode&os.ModeSetuid != 0 {
		if chars[2] == 'x' {
			chars[2] = 's'
		} else {
			chars[2] = 'S'
		}
	}
	if mode&os.ModeSetgid != 0 {
		if chars[5] == 'x' {
			chars[5] = 's'
		} else {
			chars[5] = 'S'
		}
	}
	if mode&os.ModeSticky != 0 {
		if chars[8] == 'x' {
			chars[8] = 't'
		} else {
			chars[8] = 'T'
		}
	}
	return string(chars)
}

// EntryKind returns a short type label for a listing entry.
func EntryKind(entry Entry) string {
	switch {
	case entry.Mode&os.ModeSymlink != 0:
		return "symlink"
	case entry.IsDir:
		return "directory"
	case entry.Mode&os.ModeNamedPipe != 0:
		return "fifo"
	case entry.Mode&os.ModeSocket != 0:
		return "socket"
	case entry.Mode&os.ModeDevice != 0:
		if entry.Mode&os.ModeCharDevice != 0 {
			return "char device"
		}
		return "block device"
	default:
		return "file"
	}
}

// FormatSize returns a compact human-readable byte size.
func FormatSize(n int64) string {
	if n < 0 {
		return "—"
	}
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"K", "M", "G", "T"}
	value := float64(n)
	for _, unit := range units {
		value /= 1000
		if value < 1000 {
			if value < 10 {
				return fmt.Sprintf("%.1f %sB", value, unit)
			}
			return fmt.Sprintf("%.0f %sB", value, unit)
		}
	}
	return fmt.Sprintf("%.0f TB", value/1000)
}

// ChmodLocal sets permission bits on a local path.
func ChmodLocal(path string, mode os.FileMode) error {
	path, err := CleanLocal(path)
	if err != nil {
		return err
	}
	return os.Chmod(path, NormalizeMode(mode))
}

// ChmodLocalRecursive sets permission bits on path and, when path is a
// directory, on every non-symlink file and directory beneath it. Symlink
// entries encountered while walking are skipped (not followed, not chmod'd).
// If path itself is a symlink, only that path is passed to os.Chmod (which
// follows the link on most systems).
func ChmodLocalRecursive(path string, mode os.FileMode) error {
	path, err := CleanLocal(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	mode = NormalizeMode(mode)
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(path, func(child string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if child == path {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		return os.Chmod(child, mode)
	})
}

// ChmodRemote sets permission bits on a remote path.
func ChmodRemote(session *Session, path string, mode os.FileMode) error {
	path, err := CleanRemote(path)
	if err != nil {
		return err
	}
	return session.client.Chmod(path, NormalizeMode(mode))
}

// ChmodRemoteRecursive sets permission bits on a remote path and its contents
// when the path is a directory. Symlink entries are skipped and not descended
// into.
func ChmodRemoteRecursive(session *Session, path string, mode os.FileMode) error {
	path, err := CleanRemote(path)
	if err != nil {
		return err
	}
	info, err := session.client.Lstat(path)
	if err != nil {
		return err
	}
	mode = NormalizeMode(mode)
	if err := session.client.Chmod(path, mode); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	return chmodRemoteDir(session, path, mode)
}

func chmodRemoteDir(session *Session, dir string, mode os.FileMode) error {
	entries, err := session.client.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range entries {
		child, err := JoinRemote(dir, info.Name())
		if err != nil {
			return err
		}
		childInfo, err := session.client.Lstat(child)
		if err != nil {
			return err
		}
		if childInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if err := session.client.Chmod(child, mode); err != nil {
			return err
		}
		if childInfo.IsDir() {
			if err := chmodRemoteDir(session, child, mode); err != nil {
				return err
			}
		}
	}
	return nil
}
