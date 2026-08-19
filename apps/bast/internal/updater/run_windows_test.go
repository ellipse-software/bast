//go:build windows

package updater

import (
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
