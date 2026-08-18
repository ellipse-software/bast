//go:build windows

package history

import (
	"path/filepath"
	"testing"
)

func TestSourcePathsIncludePowerShellHistories(t *testing.T) {
	home := `C:\Users\Ted`
	appData := filepath.Join(home, "AppData", "Roaming")
	paths := sourcePaths(home, func(name string) string {
		if name == "APPDATA" {
			return appData
		}
		return ""
	})
	want := map[string]bool{
		filepath.Join(appData, "Microsoft", "Windows", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"): true,
		filepath.Join(appData, "Microsoft", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"):            true,
	}
	for _, path := range paths {
		delete(want, path)
	}
	if len(want) != 0 {
		t.Fatalf("missing PowerShell histories: %v", want)
	}
}
