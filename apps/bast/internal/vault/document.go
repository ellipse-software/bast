package vault

import "bast/internal/metadata"

const DocumentVersion = 1

// Document is the plaintext vault payload before encryption.
type Document struct {
	Version      int                      `json:"version"`
	Revision     string                   `json:"revision,omitempty"`
	UpdatedAt    int64                    `json:"updatedAt"`
	Hosts        []HostEntry              `json:"hosts"`
	Keys         []KeyEntry               `json:"keys"`
	Metadata     map[string]metadata.Host `json:"metadata,omitempty"`
	Preferences  metadata.Preferences     `json:"preferences,omitempty"`
	Integrations VaultIntegrations        `json:"integrations,omitempty"`
	Tombstones   Tombstones               `json:"tombstones,omitempty"`
}

type HostEntry struct {
	ManagedID    string   `json:"managedId"`
	Alias        string   `json:"alias"`
	HostName     string   `json:"hostname"`
	User         string   `json:"user,omitempty"`
	Port         string   `json:"port,omitempty"`
	IdentityFile string   `json:"identityFile,omitempty"`
	ExtraOptions []string `json:"extraOptions,omitempty"`
	PasswordOnly bool     `json:"passwordOnly,omitempty"`
	ProxyJump    string   `json:"proxyJump,omitempty"`
	UpdatedAt    int64    `json:"updatedAt"`
	DeletedAt    int64    `json:"deletedAt,omitempty"`
}

type KeyEntry struct {
	Name          string `json:"name"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	PrivatePEM    string `json:"privatePem"`
	PublicOpenSSH string `json:"publicOpenSSH,omitempty"`
	Comment       string `json:"comment,omitempty"`
	Algorithm     string `json:"algorithm,omitempty"`
	UpdatedAt     int64  `json:"updatedAt"`
	DeletedAt     int64  `json:"deletedAt,omitempty"`
}

type Tombstones struct {
	Hosts map[string]int64 `json:"hosts,omitempty"` // managedId -> deletedAt
	Keys  map[string]int64 `json:"keys,omitempty"`  // fingerprint or name -> deletedAt
}

// VaultIntegrations stores sync settings without credentials or last-sync telemetry.
type VaultIntegrations struct {
	GCP          *VaultGCPIntegration          `json:"gcp,omitempty"`
	AWS          *VaultAWSIntegration          `json:"aws,omitempty"`
	Azure        *VaultAzureIntegration        `json:"azure,omitempty"`
	DigitalOcean *VaultDigitalOceanIntegration `json:"digitalocean,omitempty"`
	Upstash      *VaultUpstashIntegration      `json:"upstash,omitempty"`
}

type VaultGCPIntegration struct {
	Enabled        bool     `json:"enabled"`
	ProjectFilter  []string `json:"projectFilter,omitempty"`
	DefaultSSHUser string   `json:"defaultSshUser,omitempty"`
	AutoSync       bool     `json:"autoSync,omitempty"`
}

type VaultAWSIntegration struct {
	Enabled        bool     `json:"enabled"`
	ProfileFilter  []string `json:"profileFilter,omitempty"`
	RegionFilter   []string `json:"regionFilter,omitempty"`
	DefaultSSHUser string   `json:"defaultSshUser,omitempty"`
	AutoSync       bool     `json:"autoSync,omitempty"`
}

type VaultAzureIntegration struct {
	Enabled             bool     `json:"enabled"`
	SubscriptionFilter  []string `json:"subscriptionFilter,omitempty"`
	ResourceGroupFilter []string `json:"resourceGroupFilter,omitempty"`
	DefaultSSHUser      string   `json:"defaultSshUser,omitempty"`
	AutoSync            bool     `json:"autoSync,omitempty"`
}

type VaultDigitalOceanIntegration struct {
	Enabled        bool     `json:"enabled"`
	ContextFilter  []string `json:"contextFilter,omitempty"`
	RegionFilter   []string `json:"regionFilter,omitempty"`
	DefaultSSHUser string   `json:"defaultSshUser,omitempty"`
	AutoSync       bool     `json:"autoSync,omitempty"`
}

type VaultUpstashIntegration struct {
	Enabled  bool `json:"enabled"`
	AutoSync bool `json:"autoSync,omitempty"`
}

type Conflict struct {
	Kind    string `json:"kind"` // host_alias | key_name
	Key     string `json:"key"`
	Message string `json:"message"`
}

type MergeResult struct {
	Document  Document   `json:"document"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
	Summary   Summary    `json:"summary"`
}

type Summary struct {
	LocalHosts  int `json:"localHosts"`
	RemoteHosts int `json:"remoteHosts"`
	LocalKeys   int `json:"localKeys"`
	RemoteKeys  int `json:"remoteKeys"`
	Conflicts   int `json:"conflicts"`
}

type MergeMode string

const (
	MergeModeMerge         MergeMode = "merge"
	MergeModeReplaceLocal  MergeMode = "replace_local"
	MergeModeReplaceRemote MergeMode = "replace_remote"
)
