package hetzner

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

type Instance struct {
	SyncID         string
	Name           string
	Context        string
	Location       string
	HostName       string
	Port           string
	User           string
	IdentityFile   string
	IdentitiesOnly bool
	Public         bool
	State          string
	Running        bool
	ServerType     string
	Image          string
	Tags           []string
	SSHKeys        []string
}

type DiscoverConfig struct {
	ContextFilter   []string
	LocationFilter  []string
	DefaultSSHUser  string
	DefaultSSHPort  string
	PreferPrivateIP bool
	Home            string
	ManagedKeys     string
}

type Discovery struct {
	Instances         []Instance
	ConfirmedContexts map[string]bool
	Warnings          []string
}

func (c *Client) Discover(ctx context.Context, cfg DiscoverConfig) (Discovery, error) {
	contexts, err := c.TokenContexts()
	if err != nil {
		return Discovery{}, err
	}
	contexts = filterContexts(contexts, cfg.ContextFilter)
	if len(contexts) == 0 {
		if len(cfg.ContextFilter) > 0 {
			return Discovery{}, fmt.Errorf("no Hetzner contexts match the context filter")
		}
		return Discovery{}, fmt.Errorf("no API token; connect on the Sync tab, set %s, or run bast hetzner key", APIKeyEnv)
	}

	out := Discovery{ConfirmedContexts: map[string]bool{}}
	seenIDs := map[int]bool{}
	for _, tokenCtx := range contexts {
		servers, keys, listErr := c.inventory(ctx, tokenCtx.Token)
		if listErr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %s", tokenCtx.Name, listErr.Error()))
			continue
		}
		out.ConfirmedContexts[tokenCtx.Name] = true
		keyByID := map[int]apiSSHKey{}
		for _, key := range keys {
			keyByID[key.ID] = key
		}
		for _, srv := range servers {
			if seenIDs[srv.ID] {
				continue
			}
			inst, skip, warn := instanceFromServer(srv, tokenCtx.Name, keyByID, cfg)
			if warn != "" {
				out.Warnings = append(out.Warnings, warn)
			}
			if skip {
				continue
			}
			if cfg.Home != "" || cfg.ManagedKeys != "" {
				attached := inst.SSHKeys
				pubs := attached
				if len(pubs) == 0 {
					for _, key := range keys {
						if strings.TrimSpace(key.PublicKey) != "" {
							pubs = append(pubs, key.PublicKey)
						}
					}
				}
				if identity := matchLocalIdentity(pubs, cfg.Home, cfg.ManagedKeys); identity != "" {
					inst.IdentityFile = identity
					inst.IdentitiesOnly = len(attached) > 0
				}
			}
			seenIDs[srv.ID] = true
			out.Instances = append(out.Instances, inst)
		}
	}
	if len(out.ConfirmedContexts) == 0 {
		msg := "incomplete Hetzner discovery"
		if len(out.Warnings) > 0 {
			msg += ": " + strings.Join(out.Warnings, "; ")
		}
		return out, fmt.Errorf("%s", msg)
	}
	sort.Slice(out.Instances, func(i, j int) bool {
		if out.Instances[i].Running != out.Instances[j].Running {
			return out.Instances[i].Running
		}
		if out.Instances[i].Context != out.Instances[j].Context {
			return out.Instances[i].Context < out.Instances[j].Context
		}
		if out.Instances[i].Location != out.Instances[j].Location {
			return out.Instances[i].Location < out.Instances[j].Location
		}
		return out.Instances[i].Name < out.Instances[j].Name
	})
	return out, nil
}

func (c *Client) inventory(ctx context.Context, token string) ([]apiServer, []apiSSHKey, error) {
	servers, err := c.listServers(ctx, token)
	if err != nil {
		return nil, nil, err
	}
	keys, err := c.listSSHKeys(ctx, token)
	if err != nil {
		return servers, nil, nil
	}
	return servers, keys, nil
}

func instanceFromServer(srv apiServer, contextName string, keys map[int]apiSSHKey, cfg DiscoverConfig) (Instance, bool, string) {
	if srv.ID <= 0 {
		return Instance{}, true, ""
	}
	status := normalizeState(srv.Status)
	if status == "deleting" || status == "unknown" {
		return Instance{}, true, fmt.Sprintf("skipped server %s: status %s", srv.Name, srv.Status)
	}
	imageName := ""
	if srv.Image != nil {
		imageName = strings.TrimSpace(srv.Image.Name)
		if imageName == "" {
			imageName = strings.TrimSpace(srv.Image.Description)
		}
		if isWindows(*srv.Image) {
			return Instance{}, true, fmt.Sprintf("skipped %s: Windows images are not imported", displayName(srv))
		}
	}
	location := strings.TrimSpace(srv.Location.Name)
	if location == "" {
		location = strings.TrimSpace(srv.Datacenter.Location.Name)
	}
	if location == "" {
		location = strings.TrimSpace(srv.Datacenter.Name)
	}
	if len(cfg.LocationFilter) > 0 && !stringInFold(cfg.LocationFilter, location) {
		return Instance{}, true, ""
	}
	host, public := hostForServer(srv, cfg.PreferPrivateIP)
	if host == "" {
		return Instance{}, true, fmt.Sprintf("skipped %s: no IPv4, IPv6, or private Cloud Network address", displayName(srv))
	}
	user := strings.TrimSpace(cfg.DefaultSSHUser)
	if user == "" {
		user = "root"
	}
	port := labeledSSHPort(srv.Labels)
	if port == "" {
		port = configuredSSHPort(cfg.DefaultSSHPort)
	}
	var pubs []string
	for _, ref := range srv.SSHKeys {
		if key, ok := keys[ref.ID]; ok && strings.TrimSpace(key.PublicKey) != "" {
			pubs = append(pubs, key.PublicKey)
		}
	}
	name := displayName(srv)
	serverType := strings.TrimSpace(srv.ServerType.Name)
	tags := []string{"state:" + status}
	if location != "" {
		tags = append(tags, "location:"+location)
	}
	if serverType != "" {
		tags = append(tags, "type:"+serverType)
	}
	if !public {
		tags = append(tags, "net:private")
	}
	for key, value := range srv.Labels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.TrimSpace(value) == "" {
			tags = append(tags, key)
			continue
		}
		tags = append(tags, key+":"+strings.TrimSpace(value))
	}
	sort.Strings(tags[1:])
	return Instance{
		SyncID:     FormatSyncID(srv.ID),
		Name:       name,
		Context:    contextName,
		Location:   location,
		HostName:   host,
		Port:       port,
		User:       user,
		Public:     public,
		State:      status,
		Running:    isRunningState(status),
		ServerType: serverType,
		Image:      imageName,
		Tags:       tags,
		SSHKeys:    pubs,
	}, false, ""
}

func displayName(srv apiServer) string {
	name := strings.TrimSpace(srv.Name)
	if name == "" {
		return strconv.Itoa(srv.ID)
	}
	return name
}

func isWindows(image apiImage) bool {
	flavor := strings.ToLower(strings.TrimSpace(image.OSFlavor))
	if flavor == "windows" {
		return true
	}
	blob := strings.ToLower(image.Name + " " + image.Description)
	return strings.Contains(blob, "windows")
}

func hostForServer(srv apiServer, preferPrivate bool) (string, bool) {
	private := firstPrivateIP(srv)
	public, publicOK := firstPublicIP(srv)
	if preferPrivate && private != "" {
		return private, false
	}
	if publicOK {
		return public, true
	}
	if private != "" {
		return private, false
	}
	return "", false
}

func firstPublicIP(srv apiServer) (string, bool) {
	if srv.PublicNet.IPv4 != nil && !srv.PublicNet.IPv4.Blocked {
		if ip := strings.TrimSpace(srv.PublicNet.IPv4.IP); ip != "" {
			return ip, true
		}
	}
	if srv.PublicNet.IPv6 != nil && !srv.PublicNet.IPv6.Blocked {
		if host := ipv6Host(srv.PublicNet.IPv6.IP); host != "" {
			return host, true
		}
	}
	return "", false
}

func labeledSSHPort(labels map[string]string) string {
	for _, key := range []string{"ssh-port", "ssh_port", "bast.ssh-port"} {
		port := strings.TrimSpace(labels[key])
		if validSSHPort(port) && port != "22" {
			return port
		}
	}
	return ""
}

func configuredSSHPort(fallback string) string {
	port := strings.TrimSpace(fallback)
	if validSSHPort(port) && port != "22" {
		return port
	}
	return ""
}

func validSSHPort(port string) bool {
	port = strings.TrimSpace(port)
	if port == "" || port == "22" {
		return port == "22"
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

func firstPrivateIP(srv apiServer) string {
	for _, priv := range srv.PrivateNet {
		if ip := strings.TrimSpace(priv.IP); ip != "" {
			return ip
		}
	}
	return ""
}

func ipv6Host(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "/") {
		ip, ipnet, err := net.ParseCIDR(raw)
		if err != nil {
			return ""
		}
		ip = ip.To16()
		if ip == nil {
			return ""
		}
		host := make(net.IP, len(ip))
		copy(host, ip.Mask(ipnet.Mask))
		host[len(host)-1] |= 1
		return host.String()
	}
	if parsed := net.ParseIP(raw); parsed != nil {
		return parsed.String()
	}
	return ""
}

func normalizeState(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func isRunningState(status string) bool {
	switch normalizeState(status) {
	case "running", "starting", "initializing", "migrating", "rebuilding":
		return true
	default:
		return false
	}
}

func IsStoppedState(status string) bool {
	switch normalizeState(status) {
	case "off", "stopping":
		return true
	default:
		return false
	}
}

func HostLooksStopped(tags []string) bool {
	return IsStoppedState(StateFromTags(tags))
}

func StateFromTags(tags []string) string {
	for _, tag := range tags {
		if state, ok := strings.CutPrefix(tag, "state:"); ok {
			return state
		}
	}
	return ""
}

func FormatSyncID(id int) string {
	return fmt.Sprintf("hetzner/%d", id)
}

func ParseSyncID(syncID string) (int, error) {
	id := strings.TrimSpace(syncID)
	id = strings.TrimPrefix(id, "hetzner/")
	n, err := strconv.Atoi(id)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid Hetzner sync id %q", syncID)
	}
	return n, nil
}

func ContextFromGroup(group string) string {
	rest, ok := strings.CutPrefix(strings.TrimSpace(group), "Hetzner Cloud/")
	if !ok {
		return ""
	}
	ctx, _, _ := strings.Cut(rest, "/")
	return ctx
}
