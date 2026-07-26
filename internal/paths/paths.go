package paths

import (
	"os"
	"path/filepath"
)

// Paths contains every file Bast may read or create.
type Paths struct {
	Home          string
	SSHDir        string
	MainConfig    string
	ManagedDir    string
	ManagedConfig string
	ManagedKeys   string
	SyncDir       string
	SyncGCPConfig string
	StateFile     string
}

func ForHome(home string) Paths {
	sshDir := filepath.Join(home, ".ssh")
	configDir, err := os.UserConfigDir()
	if err != nil || home != userHome() {
		configDir = filepath.Join(home, ".config")
	}
	managedDir := filepath.Join(sshDir, "bast")
	syncDir := filepath.Join(managedDir, "sync")
	return Paths{
		Home:          home,
		SSHDir:        sshDir,
		MainConfig:    filepath.Join(sshDir, "config"),
		ManagedDir:    managedDir,
		ManagedConfig: filepath.Join(managedDir, "config"),
		ManagedKeys:   filepath.Join(managedDir, "keys"),
		SyncDir:       syncDir,
		SyncGCPConfig: filepath.Join(syncDir, "gcp", "config"),
		StateFile:     filepath.Join(configDir, "bast", "state.json"),
	}
}

func Default() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	return ForHome(home), nil
}

func userHome() string {
	home, _ := os.UserHomeDir()
	return home
}
