package cloud

import "context"

// SSHKey is a provider metadata SSH key entry (user + public key).
type SSHKey struct {
	User      string
	PublicKey string
	Expired   bool // true when google-ssh expireOn is in the past
}

// Instance is a cloud VM mapped into Bast as a syncable SSH host.
type Instance struct {
	SyncID              string
	Name                string
	ProjectID           string
	ProjectName         string
	Zone                string
	User                string
	HostName            string
	UseIAP              bool
	OSLogin             bool
	BlockProjectSSHKeys bool
	SSHKeys             []SSHKey // merged instance + project ssh-keys metadata
	Image               string   // boot disk image hint for user heuristics
	IdentityFile        string   // filled during sync host mapping
	IdentitiesOnly      bool     // true when IdentityFile came from a confident metadata match
	Tags                []string
	CredentialAccount   string // gcloud account that successfully discovered the instance
	CredentialFile      string // service-account credential file used for discovery
}

// Provider discovers cloud instances for sync.
type Provider interface {
	Name() string
	Discover(ctx context.Context, cfg DiscoverConfig) ([]Instance, error)
}

// DiscoverConfig is shared discovery options.
type DiscoverConfig struct {
	ProjectFilter   []string
	DefaultSSHUser  string
	ServiceAccounts []string
	Home            string // used to resolve local identity files
	ManagedKeys     string // Bast managed keys directory
}
