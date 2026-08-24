package digitalocean

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"bast/internal/platform"
)

const ProviderName = "digitalocean"

type Runner func(ctx context.Context, args []string, env []string) ([]byte, error)

type Client struct {
	DOCTL        string
	Run          Runner
	PollInterval time.Duration
}

type DiscoverConfig struct {
	ContextFilter  []string
	RegionFilter   []string
	DefaultSSHUser string
	Home           string
	ManagedKeys    string
}

// Discovery is a partial DigitalOcean inventory. Callers must not prune hosts
// whose context (or context/region, when a region filter is set) is missing
// from ConfirmedContexts / ConfirmedScopes.
type Discovery struct {
	Instances         []Instance
	ConfirmedContexts map[string]bool // context name after a successful account-wide list
	ConfirmedScopes   map[string]bool // "context/region" when a region filter is set
	Warnings          []string
}

type Instance struct {
	SyncID         string
	Name           string
	AccountUUID    string
	Context        string
	Region         string
	HostName       string
	User           string
	IdentityFile   string
	IdentitiesOnly bool
	Image          string
	ImageSlug      string
	SizeSlug       string
	Tags           []string
	Status         string
}

type authContext struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

type account struct {
	UUID  string `json:"uuid"`
	Email string `json:"email"`
	Team  *struct {
		Name string `json:"name"`
		UUID string `json:"uuid"`
	} `json:"team"`
}

type droplet struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	SizeSlug string `json:"size_slug"`
	Region   struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"region"`
	Image struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Distribution string `json:"distribution"`
		Slug         string `json:"slug"`
		Type         string `json:"type"`
	} `json:"image"`
	Networks struct {
		V4 []networkAddr `json:"v4"`
		V6 []networkAddr `json:"v6"`
	} `json:"networks"`
	Tags []string `json:"tags"`
}

type networkAddr struct {
	IPAddress string `json:"ip_address"`
	Type      string `json:"type"`
}

type sshKey struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
}

func New() *Client { return &Client{DOCTL: "doctl", Run: defaultRunner} }

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("doctl: %s", msg)
	}
	if len(out) == 0 && stderr.Len() > 0 {
		return bytes.TrimSpace(stderr.Bytes()), nil
	}
	return out, nil
}

func (c *Client) bin() string {
	if c.DOCTL != "" {
		return c.DOCTL
	}
	return "doctl"
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	run := c.Run
	if run == nil {
		run = defaultRunner
	}
	full := append([]string{c.bin()}, args...)
	return run(ctx, full, nil)
}

func (c *Client) CheckAvailable(ctx context.Context) error {
	_, err := c.run(ctx, "version")
	if err == nil {
		return nil
	}
	msg := err.Error()
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "command not found") || strings.Contains(msg, "not found in $PATH") {
		return fmt.Errorf("doctl not found; install the DigitalOcean CLI and run doctl auth init")
	}
	return fmt.Errorf("doctl is not usable: %w", err)
}

func (c *Client) ListContexts(ctx context.Context) ([]string, error) {
	out, err := c.run(ctx, "auth", "list", "--output", "json")
	if err != nil {
		return nil, err
	}
	var raw []authContext
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse DigitalOcean auth contexts: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("no DigitalOcean auth contexts; run doctl auth init")
	}
	return orderContexts(raw), nil
}

func orderContexts(contexts []authContext) []string {
	current := ""
	seen := map[string]bool{}
	names := make([]string, 0, len(contexts))
	for _, item := range contexts {
		name := strings.TrimSpace(item.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
		if item.Current {
			current = name
		}
	}
	sort.Strings(names)
	priority := []string{current, "default"}
	out := make([]string, 0, len(names))
	used := map[string]bool{}
	for _, wanted := range priority {
		for _, name := range names {
			if wanted != "" && name == wanted && !used[name] {
				out = append(out, name)
				used[name] = true
			}
		}
	}
	for _, name := range names {
		if !used[name] {
			out = append(out, name)
		}
	}
	return out
}

func ScopeKey(contextName, region string) string {
	return strings.TrimSpace(contextName) + "/" + strings.TrimSpace(region)
}

func (c *Client) Discover(ctx context.Context, cfg DiscoverConfig) (Discovery, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return Discovery{}, err
	}
	contexts, err := c.ListContexts(ctx)
	if err != nil {
		return Discovery{}, err
	}
	selected := filterValues(contexts, cfg.ContextFilter)
	if len(selected) == 0 {
		return Discovery{}, fmt.Errorf("no DigitalOcean contexts matched the context filter %q; check casing or update the filter", strings.Join(cfg.ContextFilter, ", "))
	}

	type contextResult struct {
		context   string
		instances []Instance
		warning   string
		ok        bool
	}
	results := make([]contextResult, len(selected))
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for i, contextName := range selected {
		g.Go(func() error {
			instances, err := c.listContext(groupCtx, contextName, cfg)
			if err != nil {
				results[i] = contextResult{
					context: contextName,
					warning: fmt.Sprintf("authenticate DigitalOcean context %s: %v", contextName, err),
				}
				return nil
			}
			results[i] = contextResult{context: contextName, instances: instances, ok: true}
			return nil
		})
	}
	_ = g.Wait()

	var found []Instance
	var warnings []string
	confirmedContexts := map[string]bool{}
	confirmedScopes := map[string]bool{}
	for _, result := range results {
		if result.warning != "" {
			warnings = append(warnings, result.warning)
			continue
		}
		if !result.ok {
			continue
		}
		if len(cfg.RegionFilter) > 0 {
			for _, region := range cfg.RegionFilter {
				if region = strings.TrimSpace(region); region != "" {
					confirmedScopes[ScopeKey(result.context, region)] = true
				}
			}
		} else {
			confirmedContexts[result.context] = true
		}
		found = append(found, result.instances...)
	}

	if len(confirmedContexts) == 0 && len(confirmedScopes) == 0 {
		if len(warnings) > 0 {
			return Discovery{}, fmt.Errorf("incomplete DigitalOcean discovery: %s", strings.Join(warnings, "; "))
		}
		return Discovery{}, fmt.Errorf("no DigitalOcean contexts selected; update the context filter")
	}

	contextRank := map[string]int{}
	for i, contextName := range selected {
		contextRank[contextName] = i
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].SyncID != found[j].SyncID {
			return found[i].SyncID < found[j].SyncID
		}
		return contextRank[found[i].Context] < contextRank[found[j].Context]
	})
	seen := map[string]bool{}
	out := make([]Instance, 0, len(found))
	skippedPrivate := map[string]bool{}
	for _, inst := range found {
		if inst.HostName == "" {
			if inst.SyncID != "" {
				skippedPrivate[inst.SyncID] = true
			}
			continue
		}
		if seen[inst.SyncID] {
			continue
		}
		seen[inst.SyncID] = true
		out = append(out, inst)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Context != out[j].Context {
			return out[i].Context < out[j].Context
		}
		if out[i].Region != out[j].Region {
			return out[i].Region < out[j].Region
		}
		return out[i].Name < out[j].Name
	})
	if n := len(skippedPrivate); n > 0 {
		warnings = append(warnings, fmt.Sprintf("%d private-only Droplets skipped (no public IP)", n))
	}
	sort.Strings(warnings)
	return Discovery{
		Instances:         out,
		ConfirmedContexts: confirmedContexts,
		ConfirmedScopes:   confirmedScopes,
		Warnings:          warnings,
	}, nil
}

func (c *Client) listContext(ctx context.Context, contextName string, cfg DiscoverConfig) ([]Instance, error) {
	acct, err := c.account(ctx, contextName)
	if err != nil {
		return nil, err
	}
	uuid := scopeUUID(acct)
	if uuid == "" {
		return nil, fmt.Errorf("account for context %s did not include a UUID", contextName)
	}
	out, err := c.run(ctx, "compute", "droplet", "list", "--context", contextName, "--output", "json")
	if err != nil {
		return nil, err
	}
	droplets, err := parseDroplets(out)
	if err != nil {
		return nil, fmt.Errorf("parse Droplets: %w", err)
	}
	keys, err := c.listSSHKeys(ctx, contextName)
	if err != nil {
		return nil, err
	}
	identityFile := matchLocalKey(sshKeyBlobs(keys), cfg.Home, cfg.ManagedKeys)
	var mapped []Instance
	for _, item := range droplets {
		region := strings.TrimSpace(item.Region.Slug)
		if len(cfg.RegionFilter) > 0 && !stringInFold(cfg.RegionFilter, region) {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status == "archive" {
			continue
		}
		if isWindows(item) {
			continue
		}
		if status != "off" && status != "new" && status != "active" {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strconv.Itoa(item.ID)
		}
		userName := strings.TrimSpace(cfg.DefaultSSHUser)
		if userName == "" {
			userName = imageSSHUser(item)
		}
		tags := []string{"digitalocean", "synced"}
		if region != "" {
			tags = append(tags, region)
		}
		if status != "" {
			tags = append(tags, "state:"+status)
		}
		for _, tag := range item.Tags {
			if tag = strings.TrimSpace(tag); tag != "" && !stringInFold(tags, tag) {
				tags = append(tags, tag)
			}
		}
		mapped = append(mapped, Instance{
			SyncID: fmt.Sprintf("do:%s:%d", uuid, item.ID),
			Name:   name, AccountUUID: uuid, Context: contextName, Region: region,
			HostName: publicHost(item), User: userName, IdentityFile: identityFile,
			IdentitiesOnly: identityFile != "", Image: imageHint(item),
			ImageSlug: strings.TrimSpace(item.Image.Slug), SizeSlug: strings.TrimSpace(item.SizeSlug),
			Tags: tags, Status: status,
		})
	}
	return mapped, nil
}

func (c *Client) account(ctx context.Context, contextName string) (account, error) {
	out, err := c.run(ctx, "account", "get", "--context", contextName, "--output", "json")
	if err != nil {
		return account{}, err
	}
	var acct account
	if err := json.Unmarshal(out, &acct); err != nil {
		return account{}, fmt.Errorf("parse DigitalOcean account: %w", err)
	}
	return acct, nil
}

func (c *Client) listSSHKeys(ctx context.Context, contextName string) ([]sshKey, error) {
	out, err := c.run(ctx, "compute", "ssh-key", "list", "--context", contextName, "--output", "json")
	if err != nil {
		return nil, fmt.Errorf("list DigitalOcean SSH keys: %w", err)
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 || string(out) == "null" {
		return nil, nil
	}
	var keys []sshKey
	if err := json.Unmarshal(out, &keys); err != nil {
		return nil, fmt.Errorf("parse DigitalOcean SSH keys: %w", err)
	}
	return keys, nil
}

func (c *Client) getDroplet(ctx context.Context, contextName, dropletID string) (droplet, error) {
	out, err := c.run(ctx, "compute", "droplet", "get", dropletID, "--context", contextName, "--output", "json")
	if err != nil {
		return droplet{}, err
	}
	items, err := parseDroplets(out)
	if err != nil {
		return droplet{}, fmt.Errorf("parse Droplet: %w", err)
	}
	if len(items) == 0 {
		return droplet{}, fmt.Errorf("droplet %s not found", dropletID)
	}
	return items[0], nil
}

func parseDroplets(data []byte) ([]droplet, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	if data[0] == '[' {
		var list []droplet
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	var one droplet
	if err := json.Unmarshal(data, &one); err != nil {
		return nil, err
	}
	if one.ID == 0 && one.Name == "" {
		return nil, nil
	}
	return []droplet{one}, nil
}

func scopeUUID(acct account) string {
	if acct.Team != nil {
		if uuid := strings.TrimSpace(acct.Team.UUID); uuid != "" {
			return uuid
		}
	}
	return strings.TrimSpace(acct.UUID)
}

func publicHost(item droplet) string {
	for _, addr := range item.Networks.V4 {
		if strings.EqualFold(addr.Type, "public") && strings.TrimSpace(addr.IPAddress) != "" {
			return strings.TrimSpace(addr.IPAddress)
		}
	}
	for _, addr := range item.Networks.V6 {
		if strings.EqualFold(addr.Type, "public") && strings.TrimSpace(addr.IPAddress) != "" {
			return strings.TrimSpace(addr.IPAddress)
		}
	}
	return ""
}

func isWindows(item droplet) bool {
	return strings.Contains(strings.ToLower(imageHint(item)), "windows")
}

func imageHint(item droplet) string {
	return strings.TrimSpace(strings.Join([]string{
		item.Image.Distribution, item.Image.Name, item.Image.Slug, item.Image.Type,
	}, " "))
}

func imageSSHUser(item droplet) string {
	hint := strings.ToLower(imageHint(item))
	switch {
	case strings.Contains(hint, "coreos"), strings.Contains(hint, "flatcar"):
		return "core"
	case strings.Contains(hint, "freebsd"):
		return "freebsd"
	default:
		return "root"
	}
}

func sshKeyBlobs(keys []sshKey) []string {
	var out []string
	for _, key := range keys {
		if blob := publicKeyBlob(key.PublicKey); blob != "" {
			out = append(out, blob)
		}
	}
	return out
}

func filterValues(values, filter []string) []string {
	if len(filter) == 0 {
		return values
	}
	wanted := map[string]bool{}
	for _, value := range filter {
		if value = strings.TrimSpace(value); value != "" {
			wanted[strings.ToLower(value)] = true
		}
	}
	var out []string
	for _, value := range values {
		if wanted[strings.ToLower(value)] {
			out = append(out, value)
		}
	}
	return out
}

func stringInFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func matchLocalKey(keys []string, home, managedKeys string) string {
	if len(keys) == 0 {
		return ""
	}
	wanted := map[string]bool{}
	for _, key := range keys {
		if blob := publicKeyBlob(key); blob != "" {
			wanted[blob] = true
		}
	}
	dirs := []string{managedKeys}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".ssh"))
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pub") {
				continue
			}
			pubPath := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(pubPath)
			if err != nil || !wanted[publicKeyBlob(string(data))] {
				continue
			}
			privatePath := strings.TrimSuffix(pubPath, ".pub")
			if info, err := os.Stat(privatePath); err == nil && info.Mode().IsRegular() {
				return platform.HomeRelative(privatePath, home)
			}
		}
	}
	return ""
}

func publicKeyBlob(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) < 2 {
		return ""
	}
	return fields[0] + " " + fields[1]
}
