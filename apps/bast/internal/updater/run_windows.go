//go:build windows

package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

const utf8BOM = "\xef\xbb\xbf"

func runInstaller(ctx context.Context, script []byte, executable string, stdout, stderr io.Writer, pinnedNightly string) error {
	if !strings.Contains(string(script), "$BastInstaller = $true") {
		return errors.New("download installer: unexpected response")
	}
	tmp, err := os.CreateTemp("", "bast-update-*.ps1")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	// PowerShell 5.1 needs a BOM to parse UTF-8 when running -File. The hosted
	// installer is served without one so `irm | iex` does not treat it as syntax.
	if !bytes.HasPrefix(script, []byte(utf8BOM)) {
		_, err = io.WriteString(tmp, utf8BOM)
	}
	if err == nil {
		_, err = tmp.Write(script)
	}
	if err == nil {
		err = tmp.Close()
	} else {
		_ = tmp.Close()
	}
	if err != nil {
		return fmt.Errorf("prepare installer: %w", err)
	}
	cmd := exec.CommandContext(ctx, systemPowerShell(), "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", tmpPath)
	env := append(os.Environ(),
		"BAST_INSTALL_DIR="+filepath.Dir(executable),
		"BAST_UPDATE_PARENT_PID="+strconv.Itoa(os.Getpid()),
	)
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

func systemPowerShell() string {
	windowsDirectory, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return "powershell.exe"
	}
	powershell := filepath.Join(windowsDirectory, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if info, err := os.Stat(powershell); err == nil && !info.IsDir() {
		return powershell
	}
	return "powershell.exe"
}

func wingetInstalled(executable string) bool {
	path := strings.ToLower(filepath.ToSlash(executable))
	return strings.Contains(path, "/microsoft/winget/packages/ellipsesoftware.bast_") ||
		strings.Contains(path, "/microsoft/windowsapps/bast.exe")
}
