package paths

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Paths struct {
	Home              string
	SSHDir            string
	MainConfig        string
	ManagedDir        string
	ManagedConfig     string
	ManagedKeys       string
	SyncDir           string
	SyncGCPConfig     string
	SyncAWSConfig     string
	SyncAzureConfig   string
	SyncBoxConfig     string
	SyncUpstashConfig string
	SyncVercelConfig  string
	SyncHetznerConfig string
	AzureDir          string
	StateFile         string
	UpstashAPIKey     string
	PasswordsDir      string
	VercelToken       string
	HetznerAPIKey     string
	HetznerTokenDir   string
}

func ForHome(home string) Paths {
	sshDir := filepath.Join(home, ".ssh")
	managedDir := filepath.Join(sshDir, "bast")
	syncDir := filepath.Join(managedDir, "sync")
	configDir := filepath.Join(home, ".config", "bast")
	return Paths{
		Home:              home,
		SSHDir:            sshDir,
		MainConfig:        filepath.Join(sshDir, "config"),
		ManagedDir:        managedDir,
		ManagedConfig:     filepath.Join(managedDir, "config"),
		ManagedKeys:       filepath.Join(managedDir, "keys"),
		SyncDir:           syncDir,
		SyncGCPConfig:     filepath.Join(syncDir, "gcp", "config"),
		SyncAWSConfig:     filepath.Join(syncDir, "aws", "config"),
		SyncAzureConfig:   filepath.Join(syncDir, "azure", "config"),
		SyncBoxConfig:     filepath.Join(syncDir, "box", "config"),
		SyncUpstashConfig: filepath.Join(syncDir, "upstash", "config"),
		SyncVercelConfig:  filepath.Join(syncDir, "vercel", "config"),
		SyncHetznerConfig: filepath.Join(syncDir, "hetzner", "config"),
		AzureDir:          filepath.Join(managedDir, "azure"),
		StateFile:         filepath.Join(configDir, "state.json"),
		UpstashAPIKey:     filepath.Join(configDir, "upstash-box-api-key"),
		PasswordsDir:      filepath.Join(configDir, "passwords"),
		VercelToken:       filepath.Join(configDir, "vercel-token"),
		HetznerAPIKey:     filepath.Join(configDir, "hetzner-api-token"),
		HetznerTokenDir:   filepath.Join(configDir, "hetzner", "tokens"),
	}
}

func Default() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	p := ForHome(home)
	if err := migrateLegacyState(p.StateFile); err != nil {
		return Paths{}, err
	}
	return p, nil
}

// migrateLegacyState moves state from the old os.UserConfigDir location
// (e.g. ~/Library/Application Support/bast on macOS) into ~/.config/bast.
func migrateLegacyState(stateFile string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	return migrateStateFrom(filepath.Join(configDir, "bast", "state.json"), stateFile)
}

func migrateStateFrom(legacy, stateFile string) error {
	if _, err := os.Stat(stateFile); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat state file: %w", err)
	}
	if legacy == stateFile {
		return nil
	}
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(stateFile), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.Rename(legacy, stateFile); err == nil {
		return nil
	}

	if err := copyFile(legacy, stateFile); err != nil {
		return fmt.Errorf("migrate state from %s: %w", legacy, err)
	}
	_ = os.Remove(legacy)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
