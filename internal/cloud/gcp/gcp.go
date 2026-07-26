package gcp

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
	"strings"

	"bast/internal/cloud"
)

const ProviderName = "gcp"

// Runner executes gcloud commands. Tests can override it.
type Runner func(ctx context.Context, args []string, env []string) ([]byte, error)

// Client discovers GCP Compute Engine instances via the gcloud CLI.
type Client struct {
	GCloud string
	Run    Runner
}

func New() *Client {
	return &Client{
		GCloud: "gcloud",
		Run:    defaultRunner,
	}
}

func (c *Client) Name() string { return ProviderName }

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gcloud: %s", msg)
	}
	return out, nil
}

func (c *Client) bin() string {
	if c.GCloud != "" {
		return c.GCloud
	}
	return "gcloud"
}

func (c *Client) run(ctx context.Context, args []string, env []string) ([]byte, error) {
	run := c.Run
	if run == nil {
		run = defaultRunner
	}
	full := append([]string{c.bin()}, args...)
	return run(ctx, full, env)
}

// Account is a gcloud-authenticated identity.
type Account struct {
	Account string `json:"account"`
	Status  string `json:"status"`
	Active  bool
}

// CheckAvailable verifies gcloud is installed.
func (c *Client) CheckAvailable(ctx context.Context) error {
	_, err := c.run(ctx, []string{"--version"}, nil)
	if err == nil {
		return nil
	}
	msg := err.Error()
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(msg, "executable file not found") || strings.Contains(msg, "not found in $PATH") {
		return fmt.Errorf("gcloud CLI not found; install Google Cloud SDK and run gcloud auth login")
	}
	if _, listErr := c.run(ctx, []string{"auth", "list", "--format=json"}, nil); listErr != nil {
		listMsg := listErr.Error()
		if strings.Contains(listMsg, "executable file not found") || strings.Contains(listMsg, "not found in $PATH") {
			return fmt.Errorf("gcloud CLI not found; install Google Cloud SDK and run gcloud auth login")
		}
		return fmt.Errorf("gcloud is not usable: %w", listErr)
	}
	return nil
}

// ListAccounts returns authenticated gcloud accounts.
func (c *Client) ListAccounts(ctx context.Context) ([]Account, error) {
	out, err := c.run(ctx, []string{"auth", "list", "--format=json"}, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Account string `json:"account"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse gcloud auth list: %w", err)
	}
	accounts := make([]Account, 0, len(raw))
	for _, a := range raw {
		if strings.TrimSpace(a.Account) == "" {
			continue
		}
		accounts = append(accounts, Account{
			Account: a.Account,
			Status:  a.Status,
			Active:  strings.EqualFold(a.Status, "ACTIVE"),
		})
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Account < accounts[j].Account
	})
	return accounts, nil
}

type credential struct {
	label string
	env   []string
	args  []string
}

func (c *Client) Discover(ctx context.Context, cfg cloud.DiscoverConfig) ([]cloud.Instance, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return nil, err
	}

	creds := []credential{}
	accounts, err := c.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		creds = append(creds, credential{
			label: account.Account,
			args:  []string{"--account=" + account.Account},
		})
	}
	for _, keyPath := range cfg.ServiceAccounts {
		expanded := expandHome(keyPath)
		if _, err := os.Stat(expanded); err != nil {
			return nil, fmt.Errorf("service account key %q: %w", keyPath, err)
		}
		creds = append(creds, credential{
			label: "sa:" + filepath.Base(expanded),
			env:   []string{"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=" + expanded},
		})
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf("no GCP credentials; run gcloud auth login or add a service account key")
	}

	osLoginUser := ""
	if cfg.DefaultSSHUser == "" {
		osLoginUser, _ = c.osLoginUsername(ctx, creds[0])
	}

	seen := map[string]bool{}
	var instances []cloud.Instance
	var lastErr error
	for _, cred := range creds {
		projects, err := c.listProjects(ctx, cred, cfg.ProjectFilter)
		if err != nil {
			lastErr = err
			continue
		}
		for _, project := range projects {
			found, err := c.listInstances(ctx, cred, project, cfg, osLoginUser)
			if err != nil {
				lastErr = err
				continue
			}
			for _, inst := range found {
				if seen[inst.SyncID] {
					continue
				}
				seen[inst.SyncID] = true
				instances = append(instances, inst)
			}
		}
	}
	if len(instances) == 0 && lastErr != nil {
		return nil, lastErr
	}
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].ProjectName != instances[j].ProjectName {
			return instances[i].ProjectName < instances[j].ProjectName
		}
		return instances[i].Name < instances[j].Name
	})
	return instances, nil
}

type project struct {
	ID      string
	Name    string
	SSHKeys string
	SSHUser string
}

func (c *Client) listProjects(ctx context.Context, cred credential, filter []string) ([]project, error) {
	args := append([]string{"projects", "list", "--format=json"}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ProjectID string `json:"projectId"`
		Name      string `json:"name"`
		Lifecycle string `json:"lifecycleState"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse gcloud projects list: %w", err)
	}
	filterSet := map[string]bool{}
	for _, f := range filter {
		if f = strings.TrimSpace(f); f != "" {
			filterSet[f] = true
		}
	}
	projects := make([]project, 0, len(raw))
	for _, p := range raw {
		if p.ProjectID == "" {
			continue
		}
		if p.Lifecycle != "" && !strings.EqualFold(p.Lifecycle, "ACTIVE") {
			continue
		}
		if len(filterSet) > 0 && !filterSet[p.ProjectID] && !filterSet[p.Name] {
			continue
		}
		name := p.Name
		if name == "" {
			name = p.ProjectID
		}
		projects = append(projects, project{ID: p.ProjectID, Name: name})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })
	return projects, nil
}

func (c *Client) loadProjectSSHMetadata(ctx context.Context, cred credential, projectID string) (sshKeys, sshUser string) {
	args := append([]string{
		"compute", "project-info", "describe",
		"--project=" + projectID,
		"--format=json",
	}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return "", ""
	}
	var info struct {
		CommonInstanceMetadata *struct {
			Items []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"items"`
		} `json:"commonInstanceMetadata"`
	}
	if err := json.Unmarshal(out, &info); err != nil || info.CommonInstanceMetadata == nil {
		return "", ""
	}
	for _, item := range info.CommonInstanceMetadata.Items {
		switch strings.ToLower(item.Key) {
		case "ssh-keys":
			sshKeys = strings.TrimSpace(item.Value)
		case "ssh-user":
			sshUser = strings.TrimSpace(item.Value)
		}
	}
	return sshKeys, sshUser
}

func (c *Client) listInstances(ctx context.Context, cred credential, project project, cfg cloud.DiscoverConfig, osLoginUser string) ([]cloud.Instance, error) {
	project.SSHKeys, project.SSHUser = c.loadProjectSSHMetadata(ctx, cred, project.ID)

	args := append([]string{
		"compute", "instances", "list",
		"--project=" + project.ID,
		"--format=json",
	}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return nil, err
	}
	var raw []gceInstance
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse gcloud compute instances list: %w", err)
	}
	instances := make([]cloud.Instance, 0, len(raw))
	for _, inst := range raw {
		if skipInstanceStatus(inst.Status) {
			continue
		}
		if isWindowsInstance(inst) {
			continue
		}
		mapped := mapInstance(inst, project, cfg.DefaultSSHUser)
		if mapped.SyncID == "" {
			continue
		}
		ResolveAuth(&mapped, cfg.Home, cfg.ManagedKeys, osLoginUser)
		instances = append(instances, mapped)
	}
	return instances, nil
}

func skipInstanceStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "TERMINATED", "STOPPING", "SUSPENDED", "SUSPENDING", "REPAIRING":
		return true
	default:
		return false
	}
}

func (c *Client) osLoginUsername(ctx context.Context, cred credential) (string, error) {
	args := append([]string{
		"compute", "os-login", "describe-profile",
		"--format=json",
	}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return "", err
	}
	var profile struct {
		POSIXAccounts []struct {
			Username string `json:"username"`
			Primary  bool   `json:"primary"`
		} `json:"posixAccounts"`
	}
	if err := json.Unmarshal(out, &profile); err != nil {
		return "", err
	}
	for _, account := range profile.POSIXAccounts {
		if account.Primary && account.Username != "" {
			return account.Username, nil
		}
	}
	for _, account := range profile.POSIXAccounts {
		if account.Username != "" {
			return account.Username, nil
		}
	}
	return "", nil
}

type gceInstance struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Zone     string `json:"zone"`
	ID       string `json:"id"`
	SelfLink string `json:"selfLink"`
	Metadata *struct {
		Items []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"items"`
	} `json:"metadata"`
	NetworkInterfaces []struct {
		NetworkIP     string `json:"networkIP"`
		AccessConfigs []struct {
			NatIP string `json:"natIP"`
		} `json:"accessConfigs"`
	} `json:"networkInterfaces"`
	Disks []struct {
		Boot             bool     `json:"boot"`
		Licenses         []string `json:"licenses"`
		Source           string   `json:"source"`
		GuestOsFeatures  []struct {
			Type string `json:"type"`
		} `json:"guestOsFeatures"`
		InitializeParams *struct {
			SourceImage string `json:"sourceImage"`
		} `json:"initializeParams"`
	} `json:"disks"`
	Labels map[string]string `json:"labels"`
}

func mapInstance(inst gceInstance, project project, defaultUser string) cloud.Instance {
	zone := zoneShort(inst.Zone)
	syncID := inst.SelfLink
	if syncID == "" {
		if zone == "" || inst.Name == "" {
			return cloud.Instance{}
		}
		syncID = fmt.Sprintf("projects/%s/zones/%s/instances/%s", project.ID, zone, inst.Name)
	} else {
		syncID = normalizeSelfLink(syncID)
	}

	externalIP := ""
	internalIP := ""
	for _, nic := range inst.NetworkInterfaces {
		if internalIP == "" {
			internalIP = nic.NetworkIP
		}
		for _, ac := range nic.AccessConfigs {
			if ac.NatIP != "" {
				externalIP = ac.NatIP
				break
			}
		}
		if externalIP != "" {
			break
		}
	}

	useIAP := externalIP == ""
	hostName := externalIP
	if useIAP {
		hostName = inst.Name
		if hostName == "" {
			hostName = internalIP
		}
	}

	instanceKeys := metadataValue(inst, "ssh-keys")
	keys := mergeSSHKeys(instanceKeys, project.SSHKeys)

	user := strings.TrimSpace(defaultUser)
	if user == "" {
		user = metadataValue(inst, "ssh-user")
	}
	if user == "" {
		user = project.SSHUser
	}

	tags := []string{"gcp", "synced"}
	if zone != "" {
		tags = append(tags, zone)
	}
	if useIAP {
		tags = append(tags, "iap")
	} else {
		tags = append(tags, "public-ip")
	}

	return cloud.Instance{
		SyncID:      syncID,
		Name:        inst.Name,
		ProjectID:   project.ID,
		ProjectName: project.Name,
		Zone:        zone,
		User:        user,
		HostName:    hostName,
		UseIAP:      useIAP,
		SSHKeys:     keys,
		Image:       bootImage(inst),
		Tags:        tags,
	}
}

func isWindowsInstance(inst gceInstance) bool {
	for _, disk := range inst.Disks {
		for _, license := range disk.Licenses {
			if strings.Contains(strings.ToLower(license), "windows") {
				return true
			}
		}
		for _, feature := range disk.GuestOsFeatures {
			if strings.Contains(strings.ToLower(feature.Type), "windows") {
				return true
			}
		}
		if disk.InitializeParams != nil && strings.Contains(strings.ToLower(disk.InitializeParams.SourceImage), "windows") {
			return true
		}
		if strings.Contains(strings.ToLower(disk.Source), "windows") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(bootImage(inst)), "windows")
}

func bootImage(inst gceInstance) string {
	for _, disk := range inst.Disks {
		if !disk.Boot {
			continue
		}
		if disk.InitializeParams != nil && disk.InitializeParams.SourceImage != "" {
			return disk.InitializeParams.SourceImage
		}
		if disk.Source != "" {
			return disk.Source
		}
	}
	for _, disk := range inst.Disks {
		if disk.InitializeParams != nil && disk.InitializeParams.SourceImage != "" {
			return disk.InitializeParams.SourceImage
		}
		if disk.Source != "" {
			return disk.Source
		}
	}
	return ""
}

func metadataValue(inst gceInstance, key string) string {
	if inst.Metadata == nil {
		return ""
	}
	for _, item := range inst.Metadata.Items {
		if strings.EqualFold(item.Key, key) {
			return strings.TrimSpace(item.Value)
		}
	}
	return ""
}

func zoneShort(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return ""
	}
	if i := strings.LastIndex(zone, "/"); i >= 0 {
		return zone[i+1:]
	}
	return zone
}

func normalizeSelfLink(link string) string {
	link = strings.TrimSpace(link)
	const marker = "/compute/v1/"
	if i := strings.Index(link, marker); i >= 0 {
		return link[i+len(marker):]
	}
	return strings.TrimPrefix(link, "https://www.googleapis.com/compute/v1/")
}

func expandHome(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
