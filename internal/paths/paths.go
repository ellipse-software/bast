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
	StateFile     string
}

func ForHome(home string) Paths {
	sshDir := filepath.Join(home, ".ssh")
	configDir, err := os.UserConfigDir()
	if err != nil || home != userHome() {
		configDir = filepath.Join(home, ".config")
	}
	return Paths{
		Home:          home,
		SSHDir:        sshDir,
		MainConfig:    filepath.Join(sshDir, "config"),
		ManagedDir:    filepath.Join(sshDir, "bast"),
		ManagedConfig: filepath.Join(sshDir, "bast", "config"),
		ManagedKeys:   filepath.Join(sshDir, "bast", "keys"),
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
