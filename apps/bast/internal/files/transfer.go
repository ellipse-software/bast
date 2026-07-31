package files

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// Progress reports transfer status to the UI.
type Progress struct {
	CurrentName string
	Done        int
	Total       int
	Bytes       int64
}

// ProgressFunc is called during transfers. Returning a non-nil error cancels.
type ProgressFunc func(Progress) error

// Endpoint is one side of a transfer (local disk or an SFTP session).
type Endpoint struct {
	Session *Session // nil means local
}

func (e Endpoint) local() bool { return e.Session == nil }

// TransferAny copies sources into destDir between any local/remote combination.
// When move is true, sources are removed after a successful copy.
func TransferAny(ctx context.Context, src, dst Endpoint, sources []string, destDir string, move bool, progress ProgressFunc) error {
	if len(sources) == 0 {
		return nil
	}
	if progress == nil {
		progress = func(Progress) error { return nil }
	}
	if src.local() && dst.local() {
		return transferLocalLocal(ctx, sources, destDir, move, progress)
	}
	if src.local() && !dst.local() {
		return Transfer(ctx, dst.Session, sources, destDir, true, move, progress)
	}
	if !src.local() && dst.local() {
		return Transfer(ctx, src.Session, sources, destDir, false, move, progress)
	}
	return transferRemoteRemote(ctx, src.Session, dst.Session, sources, destDir, move, progress)
}

// Transfer copies sources into destDir. When move is true, sources are removed after a successful copy.
// localToRemote selects direction for a single remote session.
func Transfer(ctx context.Context, session *Session, sources []string, destDir string, localToRemote, move bool, progress ProgressFunc) error {
	if len(sources) == 0 {
		return nil
	}
	if progress == nil {
		progress = func(Progress) error { return nil }
	}
	total := 0
	for _, src := range sources {
		n, err := countItems(session, src, localToRemote)
		if err != nil {
			return err
		}
		total += n
	}
	state := Progress{Total: total}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := BaseName(src)
		var dest string
		var err error
		if localToRemote {
			dest, err = JoinRemote(destDir, name)
		} else {
			dest, err = JoinLocal(destDir, name)
		}
		if err != nil {
			return err
		}
		if err := transferOne(ctx, session, src, dest, localToRemote, &state, progress); err != nil {
			return err
		}
		if move {
			if localToRemote {
				if err := RemoveLocal(src); err != nil {
					return fmt.Errorf("remove local after move: %w", err)
				}
			} else {
				if err := RemoveRemote(session, src); err != nil {
					return fmt.Errorf("remove remote after move: %w", err)
				}
			}
		}
	}
	return nil
}

func transferLocalLocal(ctx context.Context, sources []string, destDir string, move bool, progress ProgressFunc) error {
	total := 0
	for _, src := range sources {
		n, err := countItems(nil, src, true)
		if err != nil {
			return err
		}
		total += n
	}
	state := Progress{Total: total}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := BaseName(src)
		dest, err := JoinLocal(destDir, name)
		if err != nil {
			return err
		}
		if err := transferLocalOne(ctx, src, dest, &state, progress); err != nil {
			return err
		}
		if move {
			if err := RemoveLocal(src); err != nil {
				return fmt.Errorf("remove local after move: %w", err)
			}
		}
	}
	return nil
}

func transferLocalOne(ctx context.Context, src, dest string, state *Progress, progress ProgressFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := CleanLocal(src)
	if err != nil {
		return err
	}
	dest, err = CleanLocal(dest)
	if err != nil {
		return err
	}
	if src == dest {
		return errSamePath
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	state.CurrentName = BaseName(src)
	if err := progress(*state); err != nil {
		return err
	}
	if info.IsDir() {
		if LocalPathContained(src, dest) {
			return errSamePath
		}
		if err := os.MkdirAll(dest, info.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Chmod(dest, info.Mode().Perm()); err != nil {
			return err
		}
		state.Done++
		if err := progress(*state); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, item := range entries {
			from := filepath.Join(src, item.Name())
			to := filepath.Join(dest, item.Name())
			if err := transferLocalOne(ctx, from, to, state, progress); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFileLocalLocal(ctx, src, dest, info.Mode(), state, progress)
}

func copyFileLocalLocal(ctx context.Context, src, dest string, mode os.FileMode, state *Progress, progress ProgressFunc) error {
	if src == dest {
		return errSamePath
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if err := copyWithProgress(ctx, out, in, state, progress); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	state.Done++
	return progress(*state)
}

func transferRemoteRemote(ctx context.Context, srcSession, dstSession *Session, sources []string, destDir string, move bool, progress ProgressFunc) error {
	total := 0
	for _, src := range sources {
		n, err := countItems(srcSession, src, false)
		if err != nil {
			return err
		}
		total += n
	}
	state := Progress{Total: total}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := BaseName(src)
		dest, err := JoinRemote(destDir, name)
		if err != nil {
			return err
		}
		if err := transferRemoteOne(ctx, srcSession, dstSession, src, dest, &state, progress); err != nil {
			return err
		}
		if move {
			if err := RemoveRemote(srcSession, src); err != nil {
				return fmt.Errorf("remove remote after move: %w", err)
			}
		}
	}
	return nil
}

func transferRemoteOne(ctx context.Context, srcSession, dstSession *Session, src, dest string, state *Progress, progress ProgressFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := CleanRemote(src)
	if err != nil {
		return err
	}
	dest, err = CleanRemote(dest)
	if err != nil {
		return err
	}
	if src == dest {
		return errSamePath
	}
	info, err := LstatRemote(srcSession, src)
	if err != nil {
		return err
	}
	state.CurrentName = BaseName(src)
	if err := progress(*state); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return copyRemoteFileToRemote(ctx, srcSession, dstSession, src, dest, info.Mode(), state, progress)
	}
	if info.IsDir() {
		if RemotePathContained(src, dest) {
			return errSamePath
		}
		if err := dstSession.client.MkdirAll(dest); err != nil {
			return err
		}
		_ = dstSession.client.Chmod(dest, info.Mode().Perm())
		state.Done++
		if err := progress(*state); err != nil {
			return err
		}
		entries, err := srcSession.client.ReadDir(src)
		if err != nil {
			return err
		}
		for _, item := range entries {
			from, err := JoinRemote(src, item.Name())
			if err != nil {
				return err
			}
			to := path.Join(dest, item.Name())
			if err := transferRemoteOne(ctx, srcSession, dstSession, from, to, state, progress); err != nil {
				return err
			}
		}
		return nil
	}
	return copyRemoteFileToRemote(ctx, srcSession, dstSession, src, dest, info.Mode(), state, progress)
}

func copyRemoteFileToRemote(ctx context.Context, srcSession, dstSession *Session, src, dest string, mode os.FileMode, state *Progress, progress ProgressFunc) error {
	in, err := srcSession.client.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	parent := path.Dir(dest)
	if parent != "." && parent != "/" {
		if err := dstSession.client.MkdirAll(parent); err != nil {
			return err
		}
	}
	out, err := dstSession.client.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return err
	}
	if err := copyWithProgress(ctx, out, in, state, progress); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = dstSession.client.Chmod(dest, mode.Perm())
	state.Done++
	return progress(*state)
}

func countItems(session *Session, src string, local bool) (int, error) {
	if local {
		info, err := os.Lstat(src)
		if err != nil {
			return 0, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return 1, nil
		}
		n := 1
		err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path == src {
				return nil
			}
			n++
			if info.Mode()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		})
		return n, err
	}
	info, err := LstatRemote(session, src)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 1, nil
	}
	return countRemote(session, src)
}

func countRemote(session *Session, dir string) (int, error) {
	n := 1
	entries, err := session.client.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, info := range entries {
		child, err := JoinRemote(dir, info.Name())
		if err != nil {
			return 0, err
		}
		childInfo, err := LstatRemote(session, child)
		if err != nil {
			return 0, err
		}
		if childInfo.Mode()&os.ModeSymlink != 0 {
			n++
			continue
		}
		if childInfo.IsDir() {
			sub, err := countRemote(session, child)
			if err != nil {
				return 0, err
			}
			n += sub
			continue
		}
		n++
	}
	return n, nil
}

func transferOne(ctx context.Context, session *Session, src, dest string, localToRemote bool, state *Progress, progress ProgressFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if localToRemote {
		src, err := CleanLocal(src)
		if err != nil {
			return err
		}
		dest, err = CleanRemote(dest)
		if err != nil {
			return err
		}
		info, err := os.Lstat(src)
		if err != nil {
			return err
		}
		state.CurrentName = BaseName(src)
		if err := progress(*state); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return copyFileLocalToRemote(ctx, session, src, dest, info.Mode(), state, progress)
		}
		if info.IsDir() {
			if err := session.client.MkdirAll(dest); err != nil {
				return err
			}
			_ = session.client.Chmod(dest, info.Mode().Perm())
			state.Done++
			if err := progress(*state); err != nil {
				return err
			}
			entries, err := os.ReadDir(src)
			if err != nil {
				return err
			}
			for _, item := range entries {
				from := filepath.Join(src, item.Name())
				to := path.Join(dest, item.Name())
				if err := transferOne(ctx, session, from, to, true, state, progress); err != nil {
					return err
				}
			}
			return nil
		}
		return copyFileLocalToRemote(ctx, session, src, dest, info.Mode(), state, progress)
	}

	src, err := CleanRemote(src)
	if err != nil {
		return err
	}
	dest, err = CleanLocal(dest)
	if err != nil {
		return err
	}
	info, err := LstatRemote(session, src)
	if err != nil {
		return err
	}
	state.CurrentName = BaseName(src)
	if err := progress(*state); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return copyFileRemoteToLocal(ctx, session, src, dest, info.Mode(), state, progress)
	}
	if info.IsDir() {
		if err := os.MkdirAll(dest, info.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Chmod(dest, info.Mode().Perm()); err != nil {
			return err
		}
		state.Done++
		if err := progress(*state); err != nil {
			return err
		}
		entries, err := session.client.ReadDir(src)
		if err != nil {
			return err
		}
		for _, item := range entries {
			from, err := JoinRemote(src, item.Name())
			if err != nil {
				return err
			}
			to := filepath.Join(dest, item.Name())
			if err := transferOne(ctx, session, from, to, false, state, progress); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFileRemoteToLocal(ctx, session, src, dest, info.Mode(), state, progress)
}

func copyFileLocalToRemote(ctx context.Context, session *Session, src, dest string, mode os.FileMode, state *Progress, progress ProgressFunc) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	parent := path.Dir(dest)
	if parent != "." && parent != "/" {
		if err := session.client.MkdirAll(parent); err != nil {
			return err
		}
	}
	out, err := session.client.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return err
	}
	if err := copyWithProgress(ctx, out, in, state, progress); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = session.client.Chmod(dest, mode.Perm())
	state.Done++
	return progress(*state)
}

func copyFileRemoteToLocal(ctx context.Context, session *Session, src, dest string, mode os.FileMode, state *Progress, progress ProgressFunc) error {
	in, err := session.client.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if err := copyWithProgress(ctx, out, in, state, progress); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	state.Done++
	return progress(*state)
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, state *Progress, progress ProgressFunc) error {
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			state.Bytes += int64(written)
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			if err := progress(*state); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
