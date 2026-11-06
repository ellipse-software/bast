package cloud

import (
	"strings"
	"time"
)

// AuthKind is how a provider proves identity to Bast.
type AuthKind int

const (
	// AuthCLI is a local CLI session (gcloud, az, fly, and the rest).
	AuthCLI AuthKind = iota
	// AuthToken is a token or key Bast stores.
	AuthToken
)

const (
	SettingText   = "text"
	SettingCSV    = "csv"
	SettingBool   = "bool"
	SettingSecret = "secret"
	SettingChoice = "choice"
	SettingAction = "action"

	KeySSHUser        = "ssh_user"
	KeySSHPort        = "ssh_port"
	KeyProjects       = "projects"
	KeyProfiles       = "profiles"
	KeyRegions        = "regions"
	KeySubscriptions  = "subscriptions"
	KeyResourceGroups = "resource_groups"
	KeyContexts       = "contexts"
	KeyLocations      = "locations"
	KeyPreferPrivate  = "prefer_private"
	KeyOrgs           = "orgs"
	KeyApps           = "apps"
	KeySAAdd          = "sa_add"
	KeySARemove       = "sa_remove"
	KeyRemoveToken    = "remove_token"
	KeyCleanup        = "cleanup"
	KeyTeam           = "team"
)

// AuthSpec is the login or token shortcut for a provider.
// Login is empty when there is no single command (AWS).
// TokenURL is empty when Bast does not know a stable create-token page (BitQate).
type AuthSpec struct {
	Kind       AuthKind
	Login      []string
	TokenURL   string
	TokenLabel string
	TokenHint  string
	TokenExtra []Setting
}

// Setting is one editable row. Hint is a constraint, not an explanation.
type Setting struct {
	Key         string
	Label       string
	Value       string
	Placeholder string
	Hint        string
	Kind        string
	Optional    bool
	Choices     []string
}

// Snapshot is the settings pane for one provider. Error is one fact.
type Snapshot struct {
	Enabled       bool
	AutoSync      bool
	Account       string
	Error         string
	LastSyncAt    *time.Time
	InstanceCount int
	Settings      []Setting
}

// Fact shortens a CLI status line to the condition. The Login or New token
// row is the remedy, so the text does not repeat the command.
func Fact(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not logged in"):
		return "not logged in"
	case strings.Contains(lower, "no api token"), strings.Contains(lower, "no access token"):
		return "no API token"
	case strings.Contains(lower, "no api key"):
		return "no API key"
	case strings.Contains(lower, "credentials expired"):
		return "credentials expired"
	case strings.Contains(lower, "not found"), strings.Contains(lower, "could not find"):
		switch {
		case strings.Contains(lower, "gcloud"):
			return "gcloud not found"
		case strings.Contains(lower, "doctl"):
			return "doctl not found"
		case strings.Contains(lower, "fly"):
			return "fly not found"
		case strings.Contains(lower, "boxd"):
			return "boxd not found"
		case strings.Contains(lower, "boat"):
			return "boat not found"
		case strings.Contains(lower, "azure"), strings.Contains(lower, " az"):
			return "az not found"
		case strings.Contains(lower, "aws"):
			return "aws not found"
		}
	}
	for _, sep := range []string{"; run ", "; install ", ". Run ", ". install "} {
		if i := strings.Index(strings.ToLower(msg), strings.ToLower(sep)); i > 0 {
			msg = strings.TrimSpace(strings.TrimRight(msg[:i], "."))
		}
	}
	return msg
}

// Unset reports a missing login, CLI, or token. The sync list shows that as off.
func Unset(msg string) bool {
	switch Fact(msg) {
	case "not logged in", "no API token", "no API key",
		"gcloud not found", "doctl not found", "fly not found",
		"boxd not found", "boat not found", "az not found", "aws not found":
		return true
	default:
		return false
	}
}
