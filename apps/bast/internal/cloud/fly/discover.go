package fly

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Instance struct {
	SyncID   string
	Name     string
	OrgSlug  string
	OrgName  string
	App      string
	Region   string
	State    string
	HostName string
	User     string
	Image    string
	Running  bool
	Tags     []string
}

type Discovery struct {
	Instances     []Instance
	Warnings      []string
	Complete      bool
	ConfirmedOrgs map[string]bool
	ConfirmedApps map[string]bool
	ListedApps    map[string]bool
	ExcludedOrgs  map[string]bool
	ExcludedApps  map[string]bool
}

func (c *Client) Discover(ctx context.Context, cfg DiscoverConfig) (Discovery, error) {
	account, err := c.Account(ctx)
	if err != nil {
		return Discovery{}, err
	}
	if !account.Authenticated {
		msg := account.Error
		if msg == "" {
			msg = "not logged in; run fly auth login"
		}
		return Discovery{}, fmt.Errorf("%s", msg)
	}

	out := Discovery{
		ConfirmedOrgs: map[string]bool{},
		ConfirmedApps: map[string]bool{},
		ListedApps:    map[string]bool{},
		ExcludedOrgs:  map[string]bool{},
		ExcludedApps:  map[string]bool{},
	}
	orgs, err := c.listOrgs(ctx)
	if err != nil {
		return Discovery{}, err
	}
	orgFilter := normalizeFilter(cfg.OrgFilter)
	appFilter := normalizeFilter(cfg.AppFilter)
	user := strings.TrimSpace(cfg.DefaultSSHUser)
	if user == "" {
		user = SSHUser
	}

	complete := true
	for _, org := range orgs {
		if len(orgFilter) > 0 && !filterContains(orgFilter, org.Slug) && !filterContains(orgFilter, org.Name) {
			out.ExcludedOrgs[org.Slug] = true
			continue
		}
		apps, appErr := c.listApps(ctx, org.Slug)
		if appErr != nil {
			complete = false
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %s", org.Slug, appErr.Error()))
			continue
		}
		out.ConfirmedOrgs[org.Slug] = true
		for _, app := range apps {
			appKey := org.Slug + "/" + app
			out.ListedApps[appKey] = true
			if len(appFilter) > 0 && !filterContains(appFilter, app) {
				out.ExcludedApps[appKey] = true
				continue
			}
			machines, listErr := c.listMachines(ctx, app)
			if listErr != nil {
				complete = false
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s/%s: %s", org.Slug, app, listErr.Error()))
				continue
			}
			out.ConfirmedApps[appKey] = true
			for _, rec := range machines {
				if inst, ok := instanceFromMachine(rec, org, app, user); ok {
					out.Instances = append(out.Instances, inst)
				}
			}
		}
	}

	sort.Slice(out.Instances, func(i, j int) bool {
		if out.Instances[i].Running != out.Instances[j].Running {
			return out.Instances[i].Running
		}
		if out.Instances[i].OrgSlug != out.Instances[j].OrgSlug {
			return out.Instances[i].OrgSlug < out.Instances[j].OrgSlug
		}
		if out.Instances[i].App != out.Instances[j].App {
			return out.Instances[i].App < out.Instances[j].App
		}
		return out.Instances[i].Name < out.Instances[j].Name
	})
	out.Complete = complete
	return out, nil
}

func (c *Client) listOrgs(ctx context.Context) ([]Org, error) {
	out, err := c.runJSON(ctx, "orgs", "list")
	if err != nil {
		return nil, err
	}
	orgs, err := parseOrgsJSON(out)
	if err != nil {
		return nil, err
	}
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].Slug < orgs[j].Slug })
	return orgs, nil
}

func parseOrgsJSON(data []byte) ([]Org, error) {
	var bySlug map[string]string
	if err := json.Unmarshal(data, &bySlug); err == nil && (len(bySlug) > 0 || string(bytesTrim(data)) == "{}") {
		orgs := make([]Org, 0, len(bySlug))
		for slug, name := range bySlug {
			slug = strings.TrimSpace(slug)
			if slug == "" {
				continue
			}
			name = strings.TrimSpace(name)
			if name == "" {
				name = slug
			}
			orgs = append(orgs, Org{Slug: slug, Name: name})
		}
		return orgs, nil
	}
	var arr []struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("parse fly orgs: %w", err)
	}
	orgs := make([]Org, 0, len(arr))
	for _, raw := range arr {
		slug := strings.TrimSpace(raw.Slug)
		if slug == "" {
			slug = strings.TrimSpace(raw.ID)
		}
		if slug == "" {
			continue
		}
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			name = slug
		}
		orgs = append(orgs, Org{Slug: slug, Name: name})
	}
	return orgs, nil
}

func (c *Client) listApps(ctx context.Context, org string) ([]string, error) {
	out, err := c.runJSON(ctx, "apps", "list", "--org", org)
	if err != nil {
		return nil, err
	}
	names, err := parseAppsJSON(out, org)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func parseAppsJSON(data []byte, orgSlug string) ([]string, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("parse fly apps: %w", err)
	}
	names := make([]string, 0, len(arr))
	seen := map[string]bool{}
	for _, raw := range arr {
		var rec struct {
			Name         string          `json:"name"`
			NameAlt      string          `json:"Name"`
			Internal     string          `json:"internal_name"`
			Organization json.RawMessage `json:"organization"`
			Org          json.RawMessage `json:"Organization"`
		}
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		name := strings.TrimSpace(rec.Name)
		if name == "" {
			name = strings.TrimSpace(rec.NameAlt)
		}
		if name == "" {
			name = strings.TrimSpace(rec.Internal)
		}
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "flyctl-interactive-shells-") {
			continue
		}
		orgFromApp := orgSlugFromRaw(rec.Organization)
		if orgFromApp == "" {
			orgFromApp = orgSlugFromRaw(rec.Org)
		}
		if orgSlug != "" && orgFromApp != "" && !strings.EqualFold(orgFromApp, orgSlug) {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

func orgSlugFromRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var obj struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if slug := strings.TrimSpace(obj.Slug); slug != "" {
		return slug
	}
	return strings.TrimSpace(obj.Name)
}

type machineRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Region    string `json:"region"`
	PrivateIP string `json:"private_ip"`
	Config    *struct {
		Image string `json:"image"`
	} `json:"config"`
	ImageRef *struct {
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
	} `json:"image_ref"`
}

func (c *Client) listMachines(ctx context.Context, app string) ([]machineRecord, error) {
	out, err := c.runJSON(ctx, "machine", "list", "--app", app)
	if err != nil {
		return nil, err
	}
	var recs []machineRecord
	if err := json.Unmarshal(out, &recs); err != nil {
		return nil, fmt.Errorf("parse fly machines: %w", err)
	}
	return recs, nil
}

func instanceFromMachine(rec machineRecord, org Org, app, user string) (Instance, bool) {
	id := strings.TrimSpace(rec.ID)
	if id == "" {
		return Instance{}, false
	}
	state := normalizeState(rec.State)
	if state == "destroyed" {
		return Instance{}, false
	}
	name := strings.TrimSpace(rec.Name)
	if name == "" {
		name = id
	}
	running := isRunningState(state)
	hostName := id
	if !running {
		hostName = StoppedHostName
	}
	image := ""
	if rec.Config != nil {
		image = strings.TrimSpace(rec.Config.Image)
	}
	if image == "" && rec.ImageRef != nil {
		image = strings.TrimSpace(rec.ImageRef.Repository)
		if rec.ImageRef.Tag != "" {
			if image == "" {
				image = rec.ImageRef.Tag
			} else {
				image += ":" + rec.ImageRef.Tag
			}
		}
	}
	tags := []string{"state:" + state}
	if region := strings.TrimSpace(rec.Region); region != "" {
		tags = append(tags, "region:"+region)
	}
	if image != "" {
		tags = append(tags, "image:"+image)
	}
	return Instance{
		SyncID:   FormatSyncID(org.Slug, app, id),
		Name:     name,
		OrgSlug:  org.Slug,
		OrgName:  org.Name,
		App:      app,
		Region:   strings.TrimSpace(rec.Region),
		State:    state,
		HostName: hostName,
		User:     user,
		Image:    image,
		Running:  running,
		Tags:     tags,
	}, true
}

func normalizeState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "started", "starting", "replacing", "updating", "restarting":
		s := strings.ToLower(strings.TrimSpace(status))
		if s == "started" {
			return "running"
		}
		return s
	case "stopped", "created", "suspended", "stopping", "suspending", "failed":
		s := strings.ToLower(strings.TrimSpace(status))
		if s == "created" {
			return "stopped"
		}
		return s
	case "destroyed", "destroying":
		if strings.EqualFold(strings.TrimSpace(status), "destroying") {
			return "destroying"
		}
		return "destroyed"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func isRunningState(state string) bool {
	switch normalizeState(state) {
	case "running", "starting", "replacing", "updating", "restarting":
		return true
	default:
		return false
	}
}

func IsStoppedState(state string) bool {
	switch normalizeState(state) {
	case "stopped", "suspended", "stopping", "suspending", "failed":
		return true
	default:
		return false
	}
}

func HostLooksStopped(hostName string, tags []string) bool {
	if strings.TrimSpace(hostName) == StoppedHostName {
		return true
	}
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

func normalizeFilter(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func filterContains(filter []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range filter {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func bytesTrim(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}
