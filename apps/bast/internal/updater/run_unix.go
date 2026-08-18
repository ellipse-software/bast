//go:build !windows

package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runInstaller(ctx context.Context, script []byte, executable string, stdout, stderr io.Writer, pinnedNightly string) error {
	if !strings.HasPrefix(string(script), "#!/bin/sh\n") {
		return errors.New("download installer: unexpected response")
	}
	tmp, err := os.CreateTemp("", "bast-update-*.sh")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0700); err == nil {
		_, err = tmp.Write(script)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("prepare installer: %w", err)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", tmpPath)
	env := append(os.Environ(), "BAST_INSTALL_DIR="+filepath.Dir(executable))
	if pinnedNightly != "" {
		env = append(env, "BAST_NIGHTLY_VERSION="+pinnedNightly)
	}
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run installer: %w", err)
	}
	return nil
}

func wingetInstalled(string) bool { return false }
