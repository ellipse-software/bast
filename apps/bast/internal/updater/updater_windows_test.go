//go:build windows

package updater

import "testing"

func TestWindowsInstallerURLsAndWinGetDetection(t *testing.T) {
	if got := platformInstallerURL(InstallerURL); got != WindowsInstallerURL {
		t.Fatalf("stable installer URL = %q", got)
	}
	if got := platformInstallerURL(NightlyInstallerURL); got != WindowsNightlyURL {
		t.Fatalf("nightly installer URL = %q", got)
	}
	path := `C:\Users\Ted\AppData\Local\Microsoft\WinGet\Packages\EllipseSoftware.Bast_Microsoft.Winget.Source_8wekyb3d8bbwe\bast.exe`
	if !wingetInstalled(path) {
		t.Fatal("expected WinGet install detection")
	}
}
