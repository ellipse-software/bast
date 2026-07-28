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
	"strconv"
	"strings"
	"time"

	"bast/internal/cloud"
	"golang.org/x/sync/errgroup"
)

const (
	ProviderName = "gcp"
	// discoverConcurrency caps parallel gcloud calls during project scans.
	discoverConcurrency = 8
)

// Runner executes gcloud commands. Tests can override it.
type Runner func(ctx context.Context, args []string, env []string) ([]byte, error)

// Client discovers GCP Compute Engine instances via the gcloud CLI.
type Client struct {
	GCloud string
	Run    Runner
}

func New() *Client {
	return &Client{GCloud: "gcloud"}
}

func (c *Client) Name() string { return ProviderName }

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	// Never prompt to enable APIs or reauth during Bast sync.
	cmd.Env = append(os.Environ(), "CLOUDSDK_CORE_DISABLE_PROMPTS=1")
	if len(env) > 0 {
		cmd.Env = append(cmd.Env, env...)
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

// CheckAvailable verifies gcloud is installed. Avoids spawning gcloud just to check.
func (c *Client) CheckAvailable(ctx context.Context) error {
	if c.Run != nil {
		// Test harnesses inject a runner; treat gcloud as available.
		return nil
	}
	if _, err := exec.LookPath(c.bin()); err != nil {
		return fmt.Errorf("gcloud CLI not found; install Google Cloud SDK and run gcloud auth login")
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
	label          string
	account        string
	credentialFile string
	env            []string
	args           []string
}

// Discovery is the result of scanning one or more GCP credentials for instances.
type Discovery struct {
	Instances []cloud.Instance
	// ConfirmedProjects are project IDs whose instance inventory was successfully
	// listed (including empty projects and projects with Compute Engine disabled).
	// Hosts in projects absent from this set must not be pruned on sync unless they
	// appear in ExcludedProjects.
	ConfirmedProjects map[string]bool
	// ExcludedProjects are project IDs intentionally filtered out by ProjectFilter.
	ExcludedProjects map[string]bool
	// Warnings are non-fatal discovery problems (expired account, permission denied
	// on a project). Sync may still succeed using ConfirmedProjects + Instances.
	Warnings []string
}

func (c *Client) Discover(ctx context.Context, cfg cloud.DiscoverConfig) ([]cloud.Instance, error) {
	d, err := c.DiscoverAll(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if len(d.Warnings) > 0 {
		return nil, fmt.Errorf("incomplete GCP discovery: %s", strings.Join(d.Warnings, "\n"))
	}
	return d.Instances, nil
}

// DiscoverAll scans all configured credentials and returns a partial inventory
// when some accounts or projects fail. Callers must not prune hosts whose
// project is missing from ConfirmedProjects.
func (c *Client) DiscoverAll(ctx context.Context, cfg cloud.DiscoverConfig) (Discovery, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return Discovery{}, err
	}

	creds, err := c.credentials(ctx, cfg.ServiceAccounts, cfg.Home)
	if err != nil {
		return Discovery{}, err
	}

	type projectJob struct {
		cred    credential
		project project
	}
	type projectResult struct {
		projectID string
		instances []cloud.Instance
		err       error
		skippable bool
	}

	var (
		instances         []cloud.Instance
		warnings          []string
		credentialErrors  []error
		failedProjects    = map[string]error{}
		confirmedProjects = map[string]bool{}
		excludedProjects  = map[string]bool{}
		listedAnyProjects bool
		jobs              []projectJob
	)

	type credProjects struct {
		cred     credential
		projects []project
		excluded []string
		err      error
	}
	credResults := make([]credProjects, len(creds))
	var listGroup errgroup.Group
	for i, cred := range creds {
		i, cred := i, cred
		listGroup.Go(func() error {
			projects, excluded, err := c.listProjects(ctx, cred, cfg.ProjectFilter)
			credResults[i] = credProjects{cred: cred, projects: projects, excluded: excluded, err: err}
			return nil
		})
	}
	_ = listGroup.Wait()

	for _, result := range credResults {
		if result.err != nil {
			wrapped := fmt.Errorf("list projects for %s: %w", result.cred.label, result.err)
			credentialErrors = append(credentialErrors, wrapped)
			warnings = append(warnings, shortenDiscoveryWarning(wrapped))
			continue
		}
		listedAnyProjects = true
		for _, projectID := range result.excluded {
			excludedProjects[projectID] = true
		}
		for _, project := range result.projects {
			jobs = append(jobs, projectJob{cred: result.cred, project: project})
		}
	}

	if !listedAnyProjects {
		if len(credentialErrors) > 0 {
			return Discovery{}, fmt.Errorf("incomplete GCP discovery: %w", errors.Join(credentialErrors...))
		}
		return Discovery{}, fmt.Errorf("incomplete GCP discovery: no accessible GCP projects")
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(discoverConcurrency)
	results := make(chan projectResult, len(jobs))

	for _, job := range jobs {
		job := job
		g.Go(func() error {
			found, err := c.listInstances(gctx, job.cred, job.project, cfg)
			res := projectResult{projectID: job.project.ID, instances: found}
			if err != nil {
				res.err = fmt.Errorf("list instances in %s using %s: %w", job.project.ID, job.cred.label, err)
				res.skippable = isSkippableProjectError(err)
			}
			select {
			case results <- res:
			case <-gctx.Done():
				return gctx.Err()
			}
			return nil
		})
	}

	waitErr := g.Wait()
	close(results)
	if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
		return Discovery{}, waitErr
	}

	seen := map[string]bool{}
	for res := range results {
		if res.err != nil {
			if res.skippable {
				confirmedProjects[res.projectID] = true
				delete(failedProjects, res.projectID)
				continue
			}
			if !confirmedProjects[res.projectID] {
				failedProjects[res.projectID] = res.err
			}
			continue
		}
		confirmedProjects[res.projectID] = true
		delete(failedProjects, res.projectID)
		for _, inst := range res.instances {
			if seen[inst.SyncID] {
				continue
			}
			seen[inst.SyncID] = true
			instances = append(instances, inst)
		}
	}

	for _, err := range failedProjects {
		warnings = append(warnings, shortenDiscoveryWarning(err))
	}

	sort.Slice(instances, func(i, j int) bool {
		if instances[i].ProjectName != instances[j].ProjectName {
			return instances[i].ProjectName < instances[j].ProjectName
		}
		return instances[i].Name < instances[j].Name
	})
	return Discovery{
		Instances:         instances,
		ConfirmedProjects: confirmedProjects,
		ExcludedProjects:  excludedProjects,
		Warnings:          uniqueNonEmpty(warnings),
	}, nil
}

func isSkippableProjectError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "service_disabled") ||
		strings.Contains(msg, "has not been used in project") ||
		strings.Contains(msg, "api [compute.googleapis.com] not enabled") ||
		strings.Contains(msg, "compute engine api has not been used") ||
		strings.Contains(msg, "compute.googleapis.com] not enabled")
}

func isCredentialError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "reauthentication failed") ||
		strings.Contains(msg, "cannot prompt during non-interactive") ||
		strings.Contains(msg, "refresh your current auth tokens") ||
		strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "not logged in") ||
		strings.Contains(msg, "there is no valid ticket")
}

func shortenDiscoveryWarning(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if isCredentialError(err) {
		if account := accountFromDiscoveryError(msg); account != "" {
			return account + ": credentials expired — run gcloud auth login"
		}
		return "GCP credentials expired — run gcloud auth login"
	}
	if project := projectFromDiscoveryError(msg); project != "" {
		if isSkippableProjectError(err) {
			return ""
		}
		firstLine := strings.Split(msg, "\n")[0]
		if len(firstLine) > 160 {
			firstLine = firstLine[:157] + "..."
		}
		return firstLine
	}
	firstLine := strings.Split(msg, "\n")[0]
	if len(firstLine) > 160 {
		firstLine = firstLine[:157] + "..."
	}
	return firstLine
}

func accountFromDiscoveryError(msg string) string {
	const prefix = "list projects for "
	if i := strings.Index(msg, prefix); i >= 0 {
		rest := msg[i+len(prefix):]
		if j := strings.Index(rest, ":"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	return ""
}

func projectFromDiscoveryError(msg string) string {
	const prefix = "list instances in "
	if i := strings.Index(msg, prefix); i >= 0 {
		rest := msg[i+len(prefix):]
		if j := strings.Index(rest, " "); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	return ""
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

type project struct {
	ID            string
	Name          string
	SSHKeys       string
	SSHUser       string
	EnableOSLogin bool
}

func (c *Client) listProjects(ctx context.Context, cred credential, filter []string) ([]project, []string, error) {
	args := append([]string{"projects", "list", "--format=json"}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return nil, nil, err
	}
	var raw []struct {
		ProjectID string `json:"projectId"`
		Name      string `json:"name"`
		Lifecycle string `json:"lifecycleState"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse gcloud projects list: %w", err)
	}
	filterSet := map[string]bool{}
	for _, f := range filter {
		if f = strings.TrimSpace(f); f != "" {
			filterSet[strings.ToLower(f)] = true
		}
	}
	projects := make([]project, 0, len(raw))
	var excluded []string
	for _, p := range raw {
		if p.ProjectID == "" {
			continue
		}
		if p.Lifecycle != "" && !strings.EqualFold(p.Lifecycle, "ACTIVE") {
			continue
		}
		if len(filterSet) > 0 && !filterSet[strings.ToLower(p.ProjectID)] && !filterSet[strings.ToLower(p.Name)] {
			excluded = append(excluded, p.ProjectID)
			continue
		}
		name := p.Name
		if name == "" {
			name = p.ProjectID
		}
		projects = append(projects, project{ID: p.ProjectID, Name: name})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })
	sort.Strings(excluded)
	return projects, excluded, nil
}

func (c *Client) loadProjectSSHMetadata(ctx context.Context, cred credential, projectID string) (sshKeys, sshUser string, enableOSLogin bool, err error) {
	args := append([]string{
		"compute", "project-info", "describe",
		"--project=" + projectID,
		"--format=json",
	}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return "", "", false, err
	}
	var info struct {
		CommonInstanceMetadata *struct {
			Items []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"items"`
		} `json:"commonInstanceMetadata"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", "", false, fmt.Errorf("parse project metadata for %s: %w", projectID, err)
	}
	if info.CommonInstanceMetadata == nil {
		return "", "", false, nil
	}
	for _, item := range info.CommonInstanceMetadata.Items {
		switch strings.ToLower(item.Key) {
		case "ssh-keys":
			sshKeys = strings.TrimSpace(item.Value)
		case "ssh-user":
			sshUser = strings.TrimSpace(item.Value)
		case "enable-oslogin":
			enableOSLogin = metadataEnabled(item.Value)
		}
	}
	return sshKeys, sshUser, enableOSLogin, nil
}

func (c *Client) listInstances(ctx context.Context, cred credential, project project, cfg cloud.DiscoverConfig) ([]cloud.Instance, error) {
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

	// Empty projects (and already-filtered statuses) do not need project metadata.
	candidates := make([]gceInstance, 0, len(raw))
	for _, inst := range raw {
		if skipInstanceStatus(inst.Status) || isWindowsInstance(inst) {
			continue
		}
		candidates = append(candidates, inst)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	if sshKeys, sshUser, enableOSLogin, metaErr := c.loadProjectSSHMetadata(ctx, cred, project.ID); metaErr == nil {
		project.SSHKeys, project.SSHUser, project.EnableOSLogin = sshKeys, sshUser, enableOSLogin
	}

	osLoginUser := ""
	if cfg.DefaultSSHUser == "" && projectUsesOSLogin(candidates, project.EnableOSLogin) {
		osLoginUser, _ = c.osLoginUsername(ctx, cred, project.ID)
	}
	instances := make([]cloud.Instance, 0, len(candidates))
	for _, inst := range candidates {
		mapped := mapInstance(inst, project, cfg.DefaultSSHUser)
		if mapped.SyncID == "" {
			continue
		}
		mapped.CredentialAccount = cred.account
		mapped.CredentialFile = cred.credentialFile
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

type osLoginProfile struct {
	Username string
	SSHKeys  []cloud.SSHKey
}

func (c *Client) osLoginUsername(ctx context.Context, cred credential, projectID string) (string, error) {
	profile, err := c.describeOSLoginProfile(ctx, cred, projectID)
	if err != nil {
		return "", err
	}
	return profile.Username, nil
}

func (c *Client) describeOSLoginProfile(ctx context.Context, cred credential, projectID string) (osLoginProfile, error) {
	args := append([]string{
		"compute", "os-login", "describe-profile",
		"--project=" + projectID,
		"--format=json",
	}, cred.args...)
	out, err := c.run(ctx, args, cred.env)
	if err != nil {
		return osLoginProfile{}, err
	}
	var raw struct {
		POSIXAccounts []struct {
			Username string `json:"username"`
			Primary  bool   `json:"primary"`
		} `json:"posixAccounts"`
		SSHPublicKeys map[string]struct {
			Key                string `json:"key"`
			ExpirationTimeUsec string `json:"expirationTimeUsec"`
		} `json:"sshPublicKeys"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return osLoginProfile{}, err
	}
	profile := osLoginProfile{}
	for _, account := range raw.POSIXAccounts {
		if account.Primary && account.Username != "" {
			profile.Username = account.Username
			break
		}
	}
	if profile.Username == "" {
		for _, account := range raw.POSIXAccounts {
			if account.Username != "" {
				profile.Username = account.Username
				break
			}
		}
	}
	for _, key := range raw.SSHPublicKeys {
		pub := strings.TrimSpace(key.Key)
		if pub == "" {
			continue
		}
		profile.SSHKeys = append(profile.SSHKeys, cloud.SSHKey{
			PublicKey: pub,
			Expired:   sshKeyExpired(pub) || osLoginKeyExpired(key.ExpirationTimeUsec),
		})
	}
	return profile, nil
}

func osLoginKeyExpired(expirationTimeUsec string) bool {
	expirationTimeUsec = strings.TrimSpace(expirationTimeUsec)
	if expirationTimeUsec == "" {
		return false
	}
	usec, err := strconv.ParseInt(expirationTimeUsec, 10, 64)
	if err != nil || usec <= 0 {
		return false
	}
	return !time.Unix(0, usec*int64(time.Microsecond)).After(time.Now().UTC())
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
	Disks  []gceDisk         `json:"disks"`
	Labels map[string]string `json:"labels"`
}

type gceDisk struct {
	Boot            bool     `json:"boot"`
	Licenses        []string `json:"licenses"`
	Source          string   `json:"source"`
	GuestOsFeatures []struct {
		Type string `json:"type"`
	} `json:"guestOsFeatures"`
	InitializeParams *struct {
		SourceImage string `json:"sourceImage"`
	} `json:"initializeParams"`
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

	osLogin := effectiveMetadataEnabled(inst, "enable-oslogin", project.EnableOSLogin)
	blockProjectSSHKeys := effectiveMetadataEnabled(inst, "block-project-ssh-keys", false)
	instanceKeys := metadataValue(inst, "ssh-keys")
	var keys []cloud.SSHKey
	if !osLogin {
		if blockProjectSSHKeys {
			keys = parseSSHKeys(instanceKeys)
		} else {
			keys = mergeSSHKeys(instanceKeys, project.SSHKeys)
		}
	}

	user := strings.TrimSpace(defaultUser)
	if user == "" && !osLogin {
		user = metadataValue(inst, "ssh-user")
	}
	if user == "" && !osLogin {
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
		SyncID:              syncID,
		Name:                inst.Name,
		ProjectID:           project.ID,
		ProjectName:         project.Name,
		Zone:                zone,
		User:                user,
		HostName:            hostName,
		UseIAP:              useIAP,
		OSLogin:             osLogin,
		BlockProjectSSHKeys: blockProjectSSHKeys,
		SSHKeys:             keys,
		Image:               bootImage(inst),
		Tags:                tags,
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
		if hint := diskImageHint(disk); hint != "" {
			return hint
		}
	}
	for _, disk := range inst.Disks {
		if hint := diskImageHint(disk); hint != "" {
			return hint
		}
	}
	return ""
}

func diskImageHint(disk gceDisk) string {
	if disk.InitializeParams != nil && disk.InitializeParams.SourceImage != "" {
		return disk.InitializeParams.SourceImage
	}
	// Prefer licenses over the persistent-disk URL — source is often
	// .../disks/<name> with no distro hint, while licenses carry debian/ubuntu/etc.
	if len(disk.Licenses) > 0 {
		return strings.Join(disk.Licenses, " ")
	}
	return disk.Source
}

func metadataValue(inst gceInstance, key string) string {
	value, _ := metadataValueOK(inst, key)
	return value
}

func metadataValueOK(inst gceInstance, key string) (string, bool) {
	if inst.Metadata == nil {
		return "", false
	}
	for _, item := range inst.Metadata.Items {
		if strings.EqualFold(item.Key, key) {
			return strings.TrimSpace(item.Value), true
		}
	}
	return "", false
}

func metadataEnabled(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func effectiveMetadataEnabled(inst gceInstance, key string, projectValue bool) bool {
	if value, ok := metadataValueOK(inst, key); ok {
		return metadataEnabled(value)
	}
	return projectValue
}

func projectUsesOSLogin(instances []gceInstance, projectEnabled bool) bool {
	for _, inst := range instances {
		if effectiveMetadataEnabled(inst, "enable-oslogin", projectEnabled) {
			return true
		}
	}
	return false
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

func expandHomeAt(path, home string) string {
	if path == "~" {
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
