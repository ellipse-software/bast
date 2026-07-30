package cloud

import "context"

type SSHKey struct {
	User      string
	PublicKey string
	Expired   bool // true when google-ssh expireOn is in the past
}

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
	Image               string   // source image reported by the cloud provider
	IdentityFile        string   // filled during sync host mapping
	IdentitiesOnly      bool     // true when IdentityFile came from a confident metadata match
	Tags                []string
	CredentialAccount   string // account used to discover the instance
	CredentialFile      string // credential file used to discover the instance
}

type Provider interface {
	Name() string
	Discover(ctx context.Context, cfg DiscoverConfig) ([]Instance, error)
}

type DiscoverConfig struct {
	ProjectFilter   []string
	DefaultSSHUser  string
	ServiceAccounts []string
	Home            string // user home used when resolving local SSH material
	ManagedKeys     string // Bast-managed key directory used for identity matching
}
