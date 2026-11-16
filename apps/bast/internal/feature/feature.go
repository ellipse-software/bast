// Package feature reads process-wide switches that turn product areas off.
// A non-empty variable disables the feature, matching BAST_NO_TELEMETRY and
// BAST_NO_ONBOARDING. Hosts already imported stay in the SSH config.
package feature

import "os"

const (
	EnvNoSync  = "BAST_NO_SYNC"
	EnvNoVault = "BAST_NO_VAULT"
)

func SyncDisabled() bool  { return os.Getenv(EnvNoSync) != "" }
func VaultDisabled() bool { return os.Getenv(EnvNoVault) != "" }

// SyncCommand reports whether name is the sync feature or one of its provider CLIs.
func SyncCommand(name string) bool {
	switch name {
	case "sync", "boat", "box", "upstash", "vercel", "hetzner", "bitqate", "railway", "fly", "digitalocean", "do", "boxd":
		return true
	default:
		return false
	}
}

// CommandDisabled reports the variable that turns cmd off, when one is set.
func CommandDisabled(cmd string) (env string, disabled bool) {
	if SyncCommand(cmd) && SyncDisabled() {
		return EnvNoSync, true
	}
	if cmd == "vault" && VaultDisabled() {
		return EnvNoVault, true
	}
	return "", false
}
