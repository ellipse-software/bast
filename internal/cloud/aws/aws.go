package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	stdsync "sync"

	"golang.org/x/sync/errgroup"
)

const ProviderName = "aws"

type Runner func(ctx context.Context, args []string, env []string) ([]byte, error)

type Client struct {
	AWS string
	Run Runner
}

type DiscoverConfig struct {
	ProfileFilter  []string
	RegionFilter   []string
	DefaultSSHUser string
	Home           string
	ManagedKeys    string
}

type Instance struct {
	SyncID         string
	Name           string
	AccountID      string
	Profile        string
	Region         string
	Zone           string
	HostName       string
	User           string
	IdentityFile   string
	IdentitiesOnly bool
	KeyName        string
	ImageID        string
	VPCID          string
	SubnetID       string
	EndpointID     string
	UseEICE        bool
	Tags           []string
}

type identity struct {
	Profile   string
	AccountID string
	Partition string
}

func New() *Client { return &Client{AWS: "aws", Run: defaultRunner} }

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "AWS_PAGER=")
	cmd.Env = append(cmd.Env, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("aws: %s", msg)
	}
	if len(out) == 0 && stderr.Len() > 0 {
		return bytes.TrimSpace(stderr.Bytes()), nil
	}
	return out, nil
}

func (c *Client) bin() string {
	if c.AWS != "" {
		return c.AWS
	}
	return "aws"
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
	out, err := c.run(ctx, "--version")
	if err == nil {
		if strings.Contains(strings.ToLower(string(out)), "aws-cli/2") {
			return nil
		}
		return fmt.Errorf("AWS CLI v2 is required")
	}
	msg := err.Error()
	if errors.Is(err, exec.ErrNotFound) || strings.Contains(msg, "executable file not found") || strings.Contains(msg, "command not found") || strings.Contains(msg, "not found in $PATH") {
		return fmt.Errorf("AWS CLI not found; install AWS CLI v2 and configure a profile")
	}
	return fmt.Errorf("AWS CLI is not usable: %w", err)
}

func (c *Client) ListProfiles(ctx context.Context) ([]string, error) {
	out, err := c.run(ctx, "configure", "list-profiles")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var profiles []string
	for _, line := range strings.Split(string(out), "\n") {
		profile := strings.TrimSpace(line)
		if profile != "" && !seen[profile] {
			seen[profile] = true
			profiles = append(profiles, profile)
		}
	}
	return orderProfiles(profiles, os.Getenv("AWS_PROFILE")), nil
}

func orderProfiles(profiles []string, active string) []string {
	sort.Strings(profiles)
	priority := []string{strings.TrimSpace(active), "default"}
	out := make([]string, 0, len(profiles))
	seen := map[string]bool{}
	for _, wanted := range priority {
		for _, profile := range profiles {
			if wanted != "" && profile == wanted && !seen[profile] {
				out = append(out, profile)
				seen[profile] = true
			}
		}
	}
	for _, profile := range profiles {
		if !seen[profile] {
			out = append(out, profile)
		}
	}
	return out
}

func (c *Client) Discover(ctx context.Context, cfg DiscoverConfig) ([]Instance, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return nil, err
	}
	profiles, err := c.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	profiles = filterValues(profiles, cfg.ProfileFilter)
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no AWS profiles selected; configure a profile or update the profile filter")
	}

	type scope struct {
		identity identity
		region   string
	}
	var scopes []scope
	for _, profile := range profiles {
		id, err := c.callerIdentity(ctx, profile)
		if err != nil {
			return nil, fmt.Errorf("authenticate AWS profile %s: %w", profile, err)
		}
		regions, err := c.listRegions(ctx, id, cfg.RegionFilter)
		if err != nil {
			return nil, fmt.Errorf("list regions for AWS profile %s: %w", profile, err)
		}
		for _, region := range regions {
			scopes = append(scopes, scope{identity: id, region: region})
		}
	}

	var mu stdsync.Mutex
	var found []Instance
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(6)
	for _, item := range scopes {
		g.Go(func() error {
			instances, err := c.listRegion(groupCtx, item.identity, item.region, cfg)
			if err != nil {
				return fmt.Errorf("list instances for %s in %s: %w", item.identity.Profile, item.region, err)
			}
			mu.Lock()
			found = append(found, instances...)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("incomplete AWS discovery: %w", err)
	}

	profileRank := map[string]int{}
	for i, profile := range profiles {
		profileRank[profile] = i
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].SyncID != found[j].SyncID {
			return found[i].SyncID < found[j].SyncID
		}
		return profileRank[found[i].Profile] < profileRank[found[j].Profile]
	})
	seen := map[string]bool{}
	out := make([]Instance, 0, len(found))
	for _, inst := range found {
		if !seen[inst.SyncID] {
			seen[inst.SyncID] = true
			out = append(out, inst)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Profile != out[j].Profile {
			return out[i].Profile < out[j].Profile
		}
		if out[i].Region != out[j].Region {
			return out[i].Region < out[j].Region
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func filterValues(values, filter []string) []string {
	if len(filter) == 0 {
		return values
	}
	wanted := map[string]bool{}
	for _, value := range filter {
		if value = strings.TrimSpace(value); value != "" {
			wanted[value] = true
		}
	}
	var out []string
	for _, value := range values {
		if wanted[value] {
			out = append(out, value)
		}
	}
	return out
}

func (c *Client) callerIdentity(ctx context.Context, profile string) (identity, error) {
	out, err := c.run(ctx, "sts", "get-caller-identity", "--profile", profile, "--output", "json", "--no-cli-pager")
	if err != nil {
		return identity{}, err
	}
	var raw struct {
		Account string `json:"Account"`
		Arn     string `json:"Arn"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return identity{}, fmt.Errorf("parse caller identity: %w", err)
	}
	parts := strings.Split(raw.Arn, ":")
	partition := "aws"
	if len(parts) > 1 && parts[0] == "arn" && parts[1] != "" {
		partition = parts[1]
	}
	if raw.Account == "" {
		return identity{}, fmt.Errorf("caller identity did not include an account ID")
	}
	return identity{Profile: profile, AccountID: raw.Account, Partition: partition}, nil
}

func seedRegion(partition string) string {
	switch partition {
	case "aws-cn":
		return "cn-north-1"
	case "aws-us-gov":
		return "us-gov-west-1"
	default:
		return "us-east-1"
	}
}

func (c *Client) listRegions(ctx context.Context, id identity, filter []string) ([]string, error) {
	if len(filter) > 0 {
		regions := make([]string, 0, len(filter))
		seen := map[string]bool{}
		for _, region := range filter {
			if region = strings.TrimSpace(region); region != "" && !seen[region] {
				seen[region] = true
				regions = append(regions, region)
			}
		}
		sort.Strings(regions)
		return regions, nil
	}
	out, err := c.run(ctx, "ec2", "describe-regions", "--all-regions", "--filters", "Name=opt-in-status,Values=opt-in-not-required,opted-in", "--profile", id.Profile, "--region", seedRegion(id.Partition), "--output", "json", "--no-cli-pager")
	if err != nil {
		return nil, err
	}
	var raw struct {
		Regions []struct {
			Name string `json:"RegionName"`
		} `json:"Regions"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse regions: %w", err)
	}
	regions := make([]string, 0, len(raw.Regions))
	for _, region := range raw.Regions {
		if region.Name != "" {
			regions = append(regions, region.Name)
		}
	}
	sort.Strings(regions)
	return regions, nil
}

type ec2Instance struct {
	InstanceID       string `json:"InstanceId"`
	ImageID          string `json:"ImageId"`
	KeyName          string `json:"KeyName"`
	PublicIPAddress  string `json:"PublicIpAddress"`
	PrivateIPAddress string `json:"PrivateIpAddress"`
	VpcID            string `json:"VpcId"`
	SubnetID         string `json:"SubnetId"`
	Platform         string `json:"Platform"`
	PlatformDetails  string `json:"PlatformDetails"`
	Placement        struct {
		AvailabilityZone string `json:"AvailabilityZone"`
	} `json:"Placement"`
	Tags []struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	} `json:"Tags"`
	NetworkInterfaces []struct {
		IPv6Addresses []struct {
			Address string `json:"Ipv6Address"`
		} `json:"Ipv6Addresses"`
	} `json:"NetworkInterfaces"`
}

type imageInfo struct {
	Name        string `json:"Name"`
	Description string `json:"Description"`
	OwnerAlias  string `json:"ImageOwnerAlias"`
}

type endpoint struct {
	ID       string `json:"InstanceConnectEndpointId"`
	State    string `json:"State"`
	VpcID    string `json:"VpcId"`
	SubnetID string `json:"SubnetId"`
}

func (c *Client) listRegion(ctx context.Context, id identity, region string, cfg DiscoverConfig) ([]Instance, error) {
	base := []string{"--profile", id.Profile, "--region", region, "--output", "json", "--no-cli-pager"}
	args := append([]string{"ec2", "describe-instances", "--filters", "Name=instance-state-name,Values=running"}, base...)
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Reservations []struct {
			Instances []ec2Instance `json:"Instances"`
		} `json:"Reservations"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse instances: %w", err)
	}
	var instances []ec2Instance
	imageIDs := map[string]bool{}
	for _, reservation := range raw.Reservations {
		for _, inst := range reservation.Instances {
			instances = append(instances, inst)
			if inst.ImageID != "" {
				imageIDs[inst.ImageID] = true
			}
		}
	}
	images := map[string]imageInfo{}
	if strings.TrimSpace(cfg.DefaultSSHUser) == "" {
		images, err = c.describeImages(ctx, id.Profile, region, imageIDs)
		if err != nil {
			return nil, err
		}
	}
	endpoints, err := c.describeEndpoints(ctx, id.Profile, region)
	if err != nil {
		return nil, err
	}

	var mapped []Instance
	for _, inst := range instances {
		if isWindows(inst) || inst.InstanceID == "" {
			continue
		}
		endpointID := ""
		hostName := strings.TrimSpace(inst.PublicIPAddress)
		if hostName == "" {
			hostName = firstIPv6(inst)
		}
		useEICE := hostName == ""
		if useEICE {
			endpointID = chooseEndpoint(endpoints, inst.VpcID, inst.SubnetID)
			if endpointID == "" || inst.PrivateIPAddress == "" {
				continue
			}
			hostName = inst.InstanceID
		}
		name := tagValue(inst, "Name")
		if name == "" {
			name = inst.InstanceID
		}
		userName := strings.TrimSpace(cfg.DefaultSSHUser)
		if userName == "" {
			userName = imageSSHUser(images[inst.ImageID])
		}
		identityFile := findLaunchKey(cfg.Home, cfg.ManagedKeys, inst.KeyName)
		tags := []string{"aws", "synced", region}
		if useEICE {
			tags = append(tags, "eice")
		} else {
			tags = append(tags, "direct")
		}
		mapped = append(mapped, Instance{
			SyncID: fmt.Sprintf("arn:%s:ec2:%s:%s:instance/%s", id.Partition, region, id.AccountID, inst.InstanceID),
			Name:   name, AccountID: id.AccountID, Profile: id.Profile, Region: region,
			Zone: inst.Placement.AvailabilityZone, HostName: hostName, User: userName,
			IdentityFile: identityFile, IdentitiesOnly: identityFile != "", KeyName: inst.KeyName,
			ImageID: inst.ImageID, VPCID: inst.VpcID, SubnetID: inst.SubnetID,
			EndpointID: endpointID, UseEICE: useEICE, Tags: tags,
		})
	}
	return mapped, nil
}

func (c *Client) describeImages(ctx context.Context, profile, region string, ids map[string]bool) (map[string]imageInfo, error) {
	result := map[string]imageInfo{}
	if len(ids) == 0 {
		return result, nil
	}
	values := make([]string, 0, len(ids))
	for id := range ids {
		values = append(values, id)
	}
	sort.Strings(values)
	for start := 0; start < len(values); start += 100 {
		end := min(start+100, len(values))
		args := []string{"ec2", "describe-images", "--image-ids"}
		args = append(args, values[start:end]...)
		args = append(args, "--profile", profile, "--region", region, "--output", "json", "--no-cli-pager")
		out, err := c.run(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("describe images: %w", err)
		}
		var raw struct {
			Images []struct {
				ImageID string `json:"ImageId"`
				imageInfo
			} `json:"Images"`
		}
		if err := json.Unmarshal(out, &raw); err != nil {
			return nil, fmt.Errorf("parse images: %w", err)
		}
		for _, image := range raw.Images {
			result[image.ImageID] = image.imageInfo
		}
	}
	return result, nil
}

func (c *Client) describeEndpoints(ctx context.Context, profile, region string) ([]endpoint, error) {
	out, err := c.run(ctx, "ec2", "describe-instance-connect-endpoints", "--profile", profile, "--region", region, "--output", "json", "--no-cli-pager")
	if err != nil {
		return nil, fmt.Errorf("describe Instance Connect endpoints: %w", err)
	}
	var raw struct {
		Endpoints []endpoint `json:"InstanceConnectEndpoints"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse Instance Connect endpoints: %w", err)
	}
	return raw.Endpoints, nil
}

func endpointActive(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "create-complete", "update-in-progress", "update-complete", "update-failed":
		return true
	default:
		return false
	}
}

func chooseEndpoint(endpoints []endpoint, vpcID, subnetID string) string {
	candidates := make([]endpoint, 0, len(endpoints))
	for _, item := range endpoints {
		if item.VpcID == vpcID && item.ID != "" && endpointActive(item.State) {
			candidates = append(candidates, item)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		iSame, jSame := candidates[i].SubnetID == subnetID, candidates[j].SubnetID == subnetID
		if iSame != jSame {
			return iSame
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].ID
}

func isWindows(inst ec2Instance) bool {
	return strings.EqualFold(inst.Platform, "windows") || strings.Contains(strings.ToLower(inst.PlatformDetails), "windows")
}

func firstIPv6(inst ec2Instance) string {
	for _, nic := range inst.NetworkInterfaces {
		for _, address := range nic.IPv6Addresses {
			if address.Address != "" {
				return address.Address
			}
		}
	}
	return ""
}

func tagValue(inst ec2Instance, key string) string {
	for _, tag := range inst.Tags {
		if tag.Key == key {
			return strings.TrimSpace(tag.Value)
		}
	}
	return ""
}

func imageSSHUser(image imageInfo) string {
	hint := strings.ToLower(strings.Join([]string{image.Name, image.Description, image.OwnerAlias}, " "))
	switch {
	case strings.Contains(hint, "ubuntu"):
		return "ubuntu"
	case strings.Contains(hint, "debian"):
		return "admin"
	case strings.Contains(hint, "centos"):
		return "centos"
	case strings.Contains(hint, "fedora"):
		return "fedora"
	case strings.Contains(hint, "bitnami"):
		return "bitnami"
	case strings.Contains(hint, "amazon linux"), strings.Contains(hint, "al202"), strings.Contains(hint, "rhel"), strings.Contains(hint, "red hat"), strings.Contains(hint, "suse"), strings.Contains(hint, "oracle"):
		return "ec2-user"
	default:
		return ""
	}
}
