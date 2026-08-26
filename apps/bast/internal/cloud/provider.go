package cloud

import (
	"context"
	"strings"
)

type Kind string

const (
	GCP     Kind = "gcp"
	AWS     Kind = "aws"
	Azure   Kind = "azure"
	Box     Kind = "box"
	Upstash Kind = "upstash"
	Vercel  Kind = "vercel"
)

// Descriptor is the stable identity for a cloud provider. UI, grouping, and
// help text should read this table instead of matching group name strings.
type Descriptor struct {
	Kind        Kind
	Title       string // short chooser label, e.g. "GCP"
	FullTitle   string // detail title, e.g. "box.ascii.dev"
	GroupRoot   string // host-list group, e.g. "Google Cloud"
	LegacyRoots []string
	Description string
	BrandColor  string
	NerdIcon    string
}

type Capabilities struct {
	Create bool
	Stop   bool
	Start  bool // resume
	Fork   bool
	Delete bool
}

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
	State               string // provider state when known (box, later hyperscalers)
	Running             bool
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

var descriptors = []Descriptor{
	{
		Kind: GCP, Title: "GCP", FullTitle: "GCP", GroupRoot: "Google Cloud",
		LegacyRoots: []string{"GCP"}, Description: "Import Compute Engine VMs into Bast",
		BrandColor: "#4285F4", NerdIcon: "\ue7f1",
	},
	{
		Kind: AWS, Title: "AWS", FullTitle: "AWS", GroupRoot: "Amazon EC2",
		LegacyRoots: []string{"AWS"}, Description: "Import EC2 instances into Bast",
		BrandColor: "#FF9900", NerdIcon: "\ue7ad",
	},
	{
		Kind: Azure, Title: "Azure", FullTitle: "Azure", GroupRoot: "Microsoft Azure",
		Description: "Import Azure VMs into Bast",
		BrandColor:  "#0078D4", NerdIcon: "\ue754",
	},
	{
		Kind: Box, Title: "Box", FullTitle: "box.ascii.dev", GroupRoot: "Box",
		Description: "Import ASCII Box sandboxes into Bast",
		BrandColor:  "#FFFFFF", NerdIcon: "\uf1b2",
	},
	{
		Kind: Upstash, Title: "Upstash", FullTitle: "Upstash Box", GroupRoot: "Upstash",
		Description: "Import Upstash Box sandboxes into Bast",
		BrandColor:  "#00E9A3", NerdIcon: "\uf1b2",
	},
	{
		Kind: Vercel, Title: "Vercel", FullTitle: "Vercel Sandbox", GroupRoot: "Vercel",
		Description: "Import Vercel Sandboxes into Bast",
		BrandColor:  "#FFFFFF", NerdIcon: "\u25b2",
	},
}

var capabilities = map[Kind]Capabilities{
	GCP:     {},
	AWS:     {},
	Azure:   {},
	Box:     {Create: true, Stop: true, Start: true, Fork: true},
	Upstash: {Create: true, Stop: true, Start: true, Fork: true, Delete: true},
	Vercel:  {Create: true, Stop: true, Start: true, Fork: true, Delete: true},
}

func Descriptors() []Descriptor {
	out := make([]Descriptor, len(descriptors))
	copy(out, descriptors)
	return out
}

func DescriptorForKind(kind Kind) (Descriptor, bool) {
	for _, d := range descriptors {
		if d.Kind == kind {
			return d, true
		}
	}
	return Descriptor{}, false
}

func KindForSource(syncSource string) (Kind, bool) {
	kind := Kind(strings.ToLower(strings.TrimSpace(syncSource)))
	_, ok := DescriptorForKind(kind)
	return kind, ok
}

func KindForGroup(group string) (Kind, bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return "", false
	}
	for _, d := range descriptors {
		if matchGroupRoot(group, d.GroupRoot) {
			return d.Kind, true
		}
		for _, root := range d.LegacyRoots {
			if matchGroupRoot(group, root) {
				return d.Kind, true
			}
		}
	}
	return "", false
}

func IsSyncedGroup(group string) bool {
	_, ok := KindForGroup(group)
	return ok
}

func CapabilitiesFor(kind Kind) Capabilities {
	return capabilities[kind]
}

func IsProviderRoot(group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	for _, d := range descriptors {
		if group == d.GroupRoot {
			return true
		}
		for _, root := range d.LegacyRoots {
			if group == root {
				return true
			}
		}
	}
	return false
}

func matchGroupRoot(group, root string) bool {
	return group == root || strings.HasPrefix(group, root+"/")
}
