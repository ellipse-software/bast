package boxd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
)

type DiscoverConfig struct {
	OrgFilter []string
}

type Instance struct {
	SyncID         string
	Name           string
	OrgID          string
	OrgName        string
	Region         string
	State          string
	HostName       string
	Port           string
	User           string
	IdentityFile   string
	IdentitiesOnly bool
	DeviceID       string
	Tags           []string
}

// Discovery is the inventory of the selected orgs. Callers must not prune a
// host whose org is in ListedOrgs but not in ConfirmedOrgs (its list failed),
// or whose org and region RegionUnavailable reports (the list could not read
// that region): its computers are unknown, not gone.
type Discovery struct {
	Instances     []Instance
	Warnings      []string
	ListedOrgs    map[string]bool
	ConfirmedOrgs map[string]bool
	// Unavailable holds "<org id>/<region>" for each region a list left out.
	Unavailable map[string]bool
}

// RegionUnavailable reports whether the list of the org could not read region.
func (d Discovery) RegionUnavailable(orgID, region string) bool {
	return d.Unavailable[orgID+"/"+region]
}

type machineRecord struct {
	Region string `json:"region"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Size   string `json:"size"`
	URL    string `json:"url"`
}

// unavailableRegion is a region a `--region all` list could not read.
type unavailableRegion struct {
	Region string
	Reason string
}

func (c *Client) Discover(ctx context.Context, cfg DiscoverConfig) (Discovery, error) {
	account, err := c.Account(ctx)
	if err != nil {
		return Discovery{}, err
	}
	if !account.Authenticated {
		msg := account.Error
		if msg == "" {
			msg = ErrNotLoggedIn.Error()
		}
		return Discovery{}, fmt.Errorf("%s", msg)
	}
	orgs, orgErr := SelectOrgs(account, cfg.OrgFilter)
	if len(orgs) == 0 && orgErr != nil {
		return Discovery{}, orgErr
	}
	out := Discovery{ListedOrgs: map[string]bool{}, ConfirmedOrgs: map[string]bool{}, Unavailable: map[string]bool{}}
	if orgErr != nil {
		out.Warnings = append(out.Warnings, orgErr.Error())
	}
	for _, org := range orgs {
		out.ListedOrgs[org.ID] = true
		records, unavailable, listErr := c.listMachines(ctx, org.ID)
		if listErr != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %s", org.Name, listErr.Error()))
			continue
		}
		out.ConfirmedOrgs[org.ID] = true
		for _, region := range unavailable {
			out.Unavailable[org.ID+"/"+region.Region] = true
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %s is unavailable (%s); kept its computers", org.Name, region.Region, region.Reason))
		}
		for _, rec := range records {
			if inst, ok := instanceFromRecord(rec, org); ok {
				out.Instances = append(out.Instances, inst)
			}
		}
	}
	c.resolveAll(ctx, out.Instances)
	sort.SliceStable(out.Instances, func(i, j int) bool {
		a, b := out.Instances[i], out.Instances[j]
		if a.OrgName != b.OrgName {
			return a.OrgName < b.OrgName
		}
		if a.Region != b.Region {
			return a.Region < b.Region
		}
		return a.Name < b.Name
	})
	return out, nil
}

// listMachines lists every region of an org. A region the CLI cannot read is
// left out of the rows and named on stderr, and the command still exits 0.
func (c *Client) listMachines(ctx context.Context, orgID string) ([]machineRecord, []unavailableRegion, error) {
	out, stderr, err := c.runWithStderr(ctx, "machine", "list", "--org", orgID, "--region", "all", "--json")
	if err != nil {
		return nil, nil, err
	}
	var recs []machineRecord
	if err := decodeFirstJSON(out, &recs); err != nil {
		return nil, nil, fmt.Errorf("parse boxd machine list: %w", err)
	}
	return recs, parseUnavailableRegions(stderr), nil
}

// parseUnavailableRegions reads the lines a `--region all` list prints after a
// bar for each region it could not read: "nyc: unavailable (why)".
func parseUnavailableRegions(stderr []byte) []unavailableRegion {
	var out []unavailableRegion
	for _, line := range strings.Split(string(stderr), "\n") {
		before, after, ok := strings.Cut(line, ": unavailable")
		if !ok {
			continue
		}
		fields := strings.Fields(before)
		if len(fields) == 0 || !valuePattern.MatchString(fields[len(fields)-1]) {
			continue
		}
		reason := strings.TrimSpace(after)
		reason = strings.TrimSuffix(strings.TrimPrefix(reason, "("), ")")
		out = append(out, unavailableRegion{Region: fields[len(fields)-1], Reason: reason})
	}
	return out
}

// resolveAll fills in each computer's SSH port and key from the entries the
// boxd CLI keeps in ~/.ssh/config. A computer without one keeps empty fields.
func (c *Client) resolveAll(ctx context.Context, instances []Instance) {
	if c.ResolveSSH == nil {
		return
	}
	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(8)
	for i := range instances {
		if instances[i].HostName == "" {
			continue
		}
		group.Go(func() error {
			details, err := c.ResolveSSH(gctx, instances[i].HostName)
			if err == nil && details.Found() {
				applySSHDetails(&instances[i], details, c.Home)
			}
			return nil
		})
	}
	_ = group.Wait()
}

func applySSHDetails(inst *Instance, details SSHDetails, home string) {
	inst.Port = strings.TrimSpace(details.Port)
	if user := strings.TrimSpace(details.User); user != "" {
		inst.User = user
	}
	inst.IdentityFile = shortenHomePath(strings.TrimSpace(details.IdentityFile), home)
	inst.IdentitiesOnly = inst.IdentityFile != "" && details.IdentitiesOnly
	inst.DeviceID = strings.TrimSpace(details.DeviceID)
}

func instanceFromRecord(rec machineRecord, org Org) (Instance, bool) {
	name := strings.TrimSpace(rec.Name)
	region := strings.TrimSpace(rec.Region)
	if name == "" || region == "" {
		return Instance{}, false
	}
	state := normalizeState(rec.Status)
	if state == "destroyed" || state == "destroying" {
		return Instance{}, false
	}
	tags := []string{"state:" + state, "region:" + region}
	if size := strings.TrimSpace(rec.Size); size != "" {
		tags = append(tags, "size:"+size)
	}
	return Instance{
		SyncID:   FormatSyncID(org.ID, region, name),
		Name:     name,
		OrgID:    org.ID,
		OrgName:  org.Name,
		Region:   region,
		State:    state,
		HostName: hostFromURL(rec.URL),
		User:     SSHUser,
		Tags:     tags,
	}, true
}

func hostFromURL(raw string) string {
	host := strings.TrimSpace(raw)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if idx := strings.IndexAny(host, "/?#"); idx >= 0 {
		host = host[:idx]
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return ""
	}
	return host
}

func normalizeState(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "suspended", "paused", "pausing":
		return "standby"
	case "created", "creating":
		return "starting"
	default:
		return s
	}
}

// IsStoppedState is true when the computer has to be started before SSH works.
// Standby and hibernated computers are not stopped: an SSH connection wakes them.
func IsStoppedState(state string) bool {
	switch normalizeState(state) {
	case "stopped", "stopping", "failed":
		return true
	default:
		return false
	}
}

func isTransitionalState(state string) bool {
	switch normalizeState(state) {
	case "starting", "rebooting", "restarting", "booting":
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

func resolveWithOpenSSH(ctx context.Context, host string) (SSHDetails, error) {
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\r\n") {
		return SSHDetails{}, fmt.Errorf("invalid boxd host %q", host)
	}
	out, err := exec.CommandContext(ctx, "ssh", "-G", "--", host).Output()
	if err != nil {
		return SSHDetails{}, fmt.Errorf("resolve %s: %w", host, err)
	}
	return ParseSSHG(out), nil
}

// ParseSSHG reads the settings Bast needs from `ssh -G` output.
func ParseSSHG(out []byte) SSHDetails {
	var details SSHDetails
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(key) {
		case "hostname":
			details.HostName = value
		case "port":
			details.Port = value
		case "user":
			details.User = value
		case "identityfile":
			if details.IdentityFile == "" {
				details.IdentityFile = value
			}
		case "identitiesonly":
			details.IdentitiesOnly = strings.EqualFold(value, "yes")
		case "setenv":
			for _, pair := range strings.Fields(value) {
				if id, ok := strings.CutPrefix(pair, "BOXD_DEVICE_ID="); ok {
					details.DeviceID = strings.Trim(id, `"`)
				}
			}
		}
	}
	return details
}

func shortenHomePath(path, home string) string {
	home = strings.TrimRight(home, `\/`)
	if home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	for _, sep := range []string{"/", `\`} {
		if strings.HasPrefix(path, home+sep) {
			return "~/" + strings.TrimPrefix(path, home+sep)
		}
	}
	return path
}
