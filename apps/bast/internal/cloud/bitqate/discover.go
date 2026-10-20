package bitqate

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Instance struct {
	SyncID             string
	Name               string
	Region             string
	HostName           string
	Port               string
	User               string
	IdentityFile       string
	IdentitiesOnly     bool
	State              string
	Running            bool
	Image              string
	HostKeyFingerprint string
	SSHKeys            []string
	Tags               []string
}

type DiscoverConfig struct {
	RegionFilter   []string
	DefaultSSHUser string
	DefaultSSHPort string
	Home           string
	ManagedKeys    string
}

type Discovery struct {
	Instances []Instance
	Warnings  []string
	Complete  bool
}

func (c *Client) Discover(ctx context.Context, cfg DiscoverConfig) (Discovery, error) {
	if !c.HasToken() {
		return Discovery{}, fmt.Errorf("no API token; connect on the Sync tab, set %s, or run bast bitqate key", APIKeyEnv)
	}
	vms, err := c.listVMs(ctx)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "invalid") {
			return Discovery{}, fmt.Errorf("%s", msg)
		}
		return Discovery{}, err
	}
	out := Discovery{Complete: true}
	for _, vm := range vms {
		inst, skip, warn := instanceFromVM(vm, cfg)
		if warn != "" {
			out.Warnings = append(out.Warnings, warn)
		}
		if skip {
			continue
		}
		if cfg.Home != "" || cfg.ManagedKeys != "" {
			if identity := matchLocalIdentity(inst.SSHKeys, cfg.Home, cfg.ManagedKeys); identity != "" {
				inst.IdentityFile = identity
				inst.IdentitiesOnly = len(inst.SSHKeys) > 0
			}
		}
		out.Instances = append(out.Instances, inst)
	}
	sort.Slice(out.Instances, func(i, j int) bool {
		if out.Instances[i].Running != out.Instances[j].Running {
			return out.Instances[i].Running
		}
		if out.Instances[i].Region != out.Instances[j].Region {
			return out.Instances[i].Region < out.Instances[j].Region
		}
		return out.Instances[i].Name < out.Instances[j].Name
	})
	return out, nil
}

func instanceFromVM(vm apiVM, cfg DiscoverConfig) (Instance, bool, string) {
	id := strings.TrimSpace(vm.ID)
	if id == "" {
		return Instance{}, true, ""
	}
	status := normalizeState(vm.Status)
	region := ""
	if vm.Region != nil {
		region = strings.TrimSpace(vm.Region.Name)
	}
	if len(cfg.RegionFilter) > 0 && !stringInFold(cfg.RegionFilter, region) {
		return Instance{}, true, ""
	}
	name := strings.TrimSpace(vm.Name)
	if name == "" {
		name = id
	}
	host := strings.TrimSpace(vm.IPv4)
	if host == "" {
		return Instance{}, true, fmt.Sprintf("skipped %s: no IPv4 address", name)
	}
	user := strings.TrimSpace(cfg.DefaultSSHUser)
	if user == "" {
		user = "root"
	}
	port := configuredSSHPort(cfg.DefaultSSHPort)
	image := ""
	if vm.OS != nil {
		image = strings.TrimSpace(vm.OS.Name)
	}
	fingerprint := strings.TrimSpace(vm.SSH.HostKeyFingerprint)
	var pubs []string
	if key := strings.TrimSpace(vm.SSH.AuthorizedKey); key != "" {
		pubs = append(pubs, key)
	}
	tags := []string{"state:" + status}
	if region != "" {
		tags = append(tags, "region:"+region)
	}
	if fingerprint != "" {
		tags = append(tags, "hostkey:"+fingerprint)
	}
	sort.Strings(tags[1:])
	return Instance{
		SyncID:             FormatSyncID(id),
		Name:               name,
		Region:             region,
		HostName:           host,
		Port:               port,
		User:               user,
		State:              status,
		Running:            isRunningState(status),
		Image:              image,
		HostKeyFingerprint: fingerprint,
		SSHKeys:            pubs,
		Tags:               tags,
	}, false, ""
}

func configuredSSHPort(fallback string) string {
	port := strings.TrimSpace(fallback)
	if port == "" || port == "22" {
		return ""
	}
	var n int
	if _, err := fmt.Sscanf(port, "%d", &n); err != nil || n <= 0 || n > 65535 {
		return ""
	}
	return port
}

func normalizeState(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func isRunningState(status string) bool {
	return normalizeState(status) == "running"
}

func IsStoppedState(status string) bool {
	return normalizeState(status) == "stopped"
}

func IsSuspendedState(status string) bool {
	return normalizeState(status) == "suspended"
}

func HostLooksStopped(tags []string) bool {
	return IsStoppedState(StateFromTags(tags))
}

func HostKeyFingerprintFromTags(tags []string) string {
	for _, tag := range tags {
		if fp, ok := strings.CutPrefix(tag, "hostkey:"); ok {
			return fp
		}
	}
	return ""
}

func StateFromTags(tags []string) string {
	for _, tag := range tags {
		if state, ok := strings.CutPrefix(tag, "state:"); ok {
			return state
		}
	}
	return ""
}

func stringInFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func FormatSyncID(id string) string {
	return "bitqate/" + strings.TrimSpace(id)
}

func ParseSyncID(syncID string) (string, error) {
	id := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(syncID), "bitqate/"))
	if id == "" || strings.ContainsAny(id, "/\\ \t\r\n") {
		return "", fmt.Errorf("invalid BitQate sync id %q", syncID)
	}
	return id, nil
}
