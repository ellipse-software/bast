//go:build windows

package updater

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemPowerShellUsesSystemDirectory(t *testing.T) {
	powershell := systemPowerShell()
	if !filepath.IsAbs(powershell) {
		t.Fatalf("PowerShell path is not absolute: %q", powershell)
	}
	if info, err := os.Stat(powershell); err != nil || info.IsDir() {
		t.Fatalf("PowerShell path is not an executable file: %q (%v)", powershell, err)
	}
}

func TestRunInstallerPreservesUTF8Script(t *testing.T) {
	testDir := t.TempDir()
	const banner = "██████╗  █████╗ ███████╗████████╗ → ✓"
	script := []byte(`$BastInstaller = $true
$banner = "` + banner + `"
[IO.File]::WriteAllText((Join-Path $env:BAST_INSTALL_DIR "utf8-result.txt"), $banner)
`)
	var stdout, stderr bytes.Buffer
	if err := runInstaller(
		context.Background(),
		script,
		filepath.Join(testDir, "bast.exe"),
		&stdout,
		&stderr,
		"",
	); err != nil {
		t.Fatalf("run UTF-8 installer: %v\nstderr: %s", err, stderr.String())
	}
	result, err := os.ReadFile(filepath.Join(testDir, "utf8-result.txt"))
	if err != nil {
		t.Fatalf("read UTF-8 result: %v", err)
	}
	if string(result) != banner {
		t.Fatalf("UTF-8 content was corrupted: got %q, want %q", result, banner)
	}
}
