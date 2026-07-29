package azure

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
	stdsync "sync"

	"golang.org/x/sync/errgroup"
)

const ProviderName = "azure"

type Runner func(ctx context.Context, args []string, env []string) ([]byte, error)

type Client struct {
	AZ  string
	Run Runner
}

type DiscoverConfig struct {
	SubscriptionFilter  []string
	ResourceGroupFilter []string
	DefaultSSHUser      string
	Home                string
	ManagedKeys         string
	BastExecutable      string
}

type Subscription struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TenantID string `json:"tenantId"`
	State    string `json:"state"`
	Default  bool   `json:"isDefault"`
}

type Discovery struct {
	Instances              []Instance
	ConfirmedSubscriptions map[string]bool
	Warnings               []string
}

type Instance struct {
	SyncID               string
	Name                 string
	SubscriptionID       string
	SubscriptionName     string
	TenantID             string
	ResourceGroup        string
	Location             string
	HostName             string
	PrivateIPAddress     string
	User                 string
	IdentityFile         string
	IdentitiesOnly       bool
	SSHKeys              []string
	UseBastion           bool
	BastionName          string
	BastionResourceGroup string
	Tags                 []string
}

func New() *Client { return &Client{AZ: "az", Run: defaultRunner} }

func defaultRunner(ctx context.Context, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), "AZURE_CORE_ONLY_SHOW_ERRORS=true")
	cmd.Env = append(cmd.Env, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("az: %s", msg)
	}
	return out, nil
}

func (c *Client) bin() string {
	if c.AZ != "" {
		return c.AZ
	}
	return "az"
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	run := c.Run
	if run == nil {
		run = defaultRunner
	}
	return run(ctx, append([]string{c.bin()}, args...), nil)
}

func (c *Client) CheckAvailable(ctx context.Context) error {
	if c.Run == nil {
		if _, err := exec.LookPath(c.bin()); err != nil {
			return fmt.Errorf("could not find Azure CLI; install Azure CLI 2.62 or later and run az login")
		}
	}
	out, err := c.run(ctx, "version", "--output", "json", "--only-show-errors")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(err.Error(), "executable file not found") {
			return fmt.Errorf("could not find Azure CLI; install Azure CLI 2.62 or later and run az login")
		}
		return fmt.Errorf("could not use Azure CLI: %w", err)
	}
	var versions struct {
		AzureCLI string `json:"azure-cli"`
	}
	if err := json.Unmarshal(out, &versions); err != nil {
		return fmt.Errorf("parse Azure CLI version: %w", err)
	}
	version := versions.AzureCLI
	if version == "" || !versionAtLeast(version, 2, 62) {
		return fmt.Errorf("requires Azure CLI 2.62 or later; found %q", version)
	}
	return nil
}

func versionAtLeast(version string, wantMajor, wantMinor int) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	return major > wantMajor || major == wantMajor && minor >= wantMinor
}

func (c *Client) CheckExtension(ctx context.Context, name string) error {
	if _, err := c.run(ctx, "extension", "show", "--name", name, "--output", "json", "--only-show-errors"); err != nil {
		return fmt.Errorf("could not use Azure CLI %s extension: %v; run az extension add --name %s", name, err, name)
	}
	return nil
}

func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	out, err := c.run(ctx, "account", "list", "--all", "--output", "json", "--only-show-errors")
	if err != nil {
		return nil, err
	}
	var subscriptions []Subscription
	if err := json.Unmarshal(out, &subscriptions); err != nil {
		return nil, fmt.Errorf("parse Azure subscriptions: %w", err)
	}
	filtered := subscriptions[:0]
	for _, sub := range subscriptions {
		if sub.ID != "" && strings.EqualFold(sub.State, "Enabled") {
			filtered = append(filtered, sub)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Default != filtered[j].Default {
			return filtered[i].Default
		}
		if !strings.EqualFold(filtered[i].Name, filtered[j].Name) {
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		}
		return filtered[i].ID < filtered[j].ID
	})
	return filtered, nil
}

func filterSubscriptions(subscriptions []Subscription, filters []string) []Subscription {
	if len(filters) == 0 {
		return subscriptions
	}
	wanted := make(map[string]bool, len(filters))
	for _, value := range filters {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			wanted[value] = true
		}
	}
	out := make([]Subscription, 0, len(subscriptions))
	for _, sub := range subscriptions {
		if wanted[strings.ToLower(sub.ID)] || wanted[strings.ToLower(sub.Name)] {
			out = append(out, sub)
		}
	}
	return out
}

func (c *Client) Discover(ctx context.Context, cfg DiscoverConfig) (Discovery, error) {
	if err := c.CheckAvailable(ctx); err != nil {
		return Discovery{}, err
	}
	subscriptions, err := c.ListSubscriptions(ctx)
	if err != nil {
		return Discovery{}, err
	}
	subscriptions = filterSubscriptions(subscriptions, cfg.SubscriptionFilter)
	if len(subscriptions) == 0 {
		return Discovery{}, fmt.Errorf("no Azure subscriptions selected; run az login or update the subscription filter")
	}

	type result struct {
		sub  Subscription
		scan subscriptionDiscovery
		err  error
	}
	results := make(chan result, len(subscriptions))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, sub := range subscriptions {
		sub := sub
		g.Go(func() error {
			scan, scanErr := c.discoverSubscription(gctx, sub, cfg)
			results <- result{sub: sub, scan: scan, err: scanErr}
			return nil
		})
	}
	_ = g.Wait()
	close(results)

	discovery := Discovery{ConfirmedSubscriptions: map[string]bool{}}
	scannedSubscriptions := 0
	for item := range results {
		if item.err != nil {
			discovery.Warnings = append(discovery.Warnings, fmt.Sprintf("%s: %v", item.sub.Name, item.err))
			continue
		}
		scannedSubscriptions++
		if item.scan.complete {
			discovery.ConfirmedSubscriptions[strings.ToLower(item.sub.ID)] = true
		}
		discovery.Instances = append(discovery.Instances, item.scan.instances...)
		for _, warning := range item.scan.warnings {
			discovery.Warnings = append(discovery.Warnings, fmt.Sprintf("%s: %s", item.sub.Name, warning))
		}
	}
	if scannedSubscriptions == 0 {
		return Discovery{}, fmt.Errorf("incomplete Azure discovery: %s", strings.Join(discovery.Warnings, "; "))
	}
	sort.Strings(discovery.Warnings)
	sort.Slice(discovery.Instances, func(i, j int) bool {
		a, b := discovery.Instances[i], discovery.Instances[j]
		if a.SubscriptionName != b.SubscriptionName {
			return a.SubscriptionName < b.SubscriptionName
		}
		if a.ResourceGroup != b.ResourceGroup {
			return a.ResourceGroup < b.ResourceGroup
		}
		return a.Name < b.Name
	})
	return discovery, nil
}

type vmRecord struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Location      string            `json:"location"`
	ResourceGroup string            `json:"resourceGroup"`
	PowerState    string            `json:"powerState"`
	PublicIPs     string            `json:"publicIps"`
	PrivateIPs    string            `json:"privateIps"`
	Tags          map[string]string `json:"tags"`
	OSProfile     struct {
		AdminUsername      string `json:"adminUsername"`
		LinuxConfiguration *struct {
			SSH struct {
				PublicKeys []struct {
					KeyData string `json:"keyData"`
				} `json:"publicKeys"`
			} `json:"ssh"`
		} `json:"linuxConfiguration"`
	} `json:"osProfile"`
	StorageProfile struct {
		OSDisk struct {
			OSType string `json:"osType"`
		} `json:"osDisk"`
	} `json:"storageProfile"`
	NetworkProfile struct {
		NetworkInterfaces []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"networkInterfaces"`
	} `json:"networkProfile"`
	VirtualMachineScaleSet *struct {
		ID string `json:"id"`
	} `json:"virtualMachineScaleSet"`
}

type nicRecord struct {
	ID             string `json:"id"`
	VirtualMachine *struct {
		ID string `json:"id"`
	} `json:"virtualMachine"`
	IPConfigurations []struct {
		Primary          bool   `json:"primary"`
		PrivateIPAddress string `json:"privateIPAddress"`
		Subnet           *struct {
			ID string `json:"id"`
		} `json:"subnet"`
	} `json:"ipConfigurations"`
}

type bastionResource struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ResourceGroup string `json:"resourceGroup"`
}

type bastionRecord struct {
	Name            string `json:"name"`
	ResourceGroup   string `json:"resourceGroup"`
	EnableTunneling bool   `json:"enableTunneling"`
	SKU             struct {
		Name string `json:"name"`
	} `json:"sku"`
	IPConfigurations []struct {
		Subnet *struct {
			ID string `json:"id"`
		} `json:"subnet"`
	} `json:"ipConfigurations"`
}

type subscriptionDiscovery struct {
	instances []Instance
	warnings  []string
	complete  bool
}

func (c *Client) discoverSubscription(ctx context.Context, sub Subscription, cfg DiscoverConfig) (subscriptionDiscovery, error) {
	var vmList []vmRecord
	var nicList []nicRecord
	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := c.runJSON(groupCtx, &vmList, "vm", "list", "--show-details", "--subscription", sub.ID, "--output", "json", "--only-show-errors"); err != nil {
			return fmt.Errorf("list VMs: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		if err := c.runJSON(groupCtx, &nicList, "network", "nic", "list", "--subscription", sub.ID, "--output", "json", "--only-show-errors"); err != nil {
			return fmt.Errorf("list network interfaces: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return subscriptionDiscovery{}, err
	}
	nics := map[string]nicRecord{}
	for _, nic := range nicList {
		nics[strings.ToLower(nic.ID)] = nic
	}

	filtered := make([]vmRecord, 0, len(vmList))
	needBastion := false
	for _, vm := range vmList {
		if !resourceGroupSelected(vm.ResourceGroup, cfg.ResourceGroupFilter) || !isRunningLinux(vm) {
			continue
		}
		filtered = append(filtered, vm)
		if firstAddress(vm.PublicIPs) == "" {
			needBastion = true
		}
	}

	scan := subscriptionDiscovery{complete: true}
	bastions := map[string]bastionRecord{}
	if needBastion {
		found, err := c.listBastions(ctx, sub.ID)
		if err != nil {
			scan.complete = false
			scan.warnings = append(scan.warnings, fmt.Sprintf("could not discover Bastion resources, preserving previously synced private VMs: %v", err))
		} else {
			bastions = found
		}
	}

	scan.instances = make([]Instance, 0, len(filtered))
	for _, vm := range filtered {
		hostName := firstAddress(vm.PublicIPs)
		privateIP, vnetID := vmNetwork(vm, nics)
		if privateIP == "" {
			privateIP = firstAddress(vm.PrivateIPs)
		}
		var selected bastionRecord
		useBastion := hostName == ""
		if useBastion {
			selected = bastions[strings.ToLower(vnetID)]
			if privateIP == "" || selected.Name == "" {
				if scan.complete {
					reason := "no matching Bastion resource"
					if privateIP == "" {
						reason = "no private IP address"
					} else if vnetID == "" {
						reason = "could not determine its virtual network"
					}
					scan.warnings = append(scan.warnings, fmt.Sprintf("skipped private VM %s/%s: %s", vm.ResourceGroup, vm.Name, reason))
				}
				continue
			}
			hostName = privateIP
		}
		keys := vmSSHKeys(vm)
		identity := matchLocalKey(keys, cfg.Home, cfg.ManagedKeys)
		userName := strings.TrimSpace(cfg.DefaultSSHUser)
		if userName == "" {
			userName = strings.TrimSpace(vm.OSProfile.AdminUsername)
		}
		tags := []string{"azure", "synced", strings.ToLower(strings.TrimSpace(vm.Location))}
		if useBastion {
			tags = append(tags, "bastion")
		} else {
			tags = append(tags, "direct")
		}
		for key, value := range vm.Tags {
			if key = strings.TrimSpace(key); key != "" {
				tag := "azure:" + key
				if value = strings.TrimSpace(value); value != "" {
					tag += "=" + value
				}
				tags = append(tags, tag)
			}
		}
		sort.Strings(tags)
		scan.instances = append(scan.instances, Instance{
			SyncID: strings.ToLower(vm.ID), Name: vm.Name, SubscriptionID: sub.ID,
			SubscriptionName: sub.Name, TenantID: sub.TenantID, ResourceGroup: vm.ResourceGroup,
			Location: vm.Location, HostName: hostName, PrivateIPAddress: privateIP,
			User: userName, IdentityFile: identity, IdentitiesOnly: identity != "", SSHKeys: keys,
			UseBastion: useBastion, BastionName: selected.Name,
			BastionResourceGroup: selected.ResourceGroup, Tags: tags,
		})
	}
	return scan, nil
}

func (c *Client) runJSON(ctx context.Context, target any, args ...string) error {
	out, err := c.run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, target); err != nil {
		return fmt.Errorf("parse Azure CLI response: %w", err)
	}
	return nil
}

func (c *Client) listBastions(ctx context.Context, subscriptionID string) (map[string]bastionRecord, error) {
	if err := c.CheckExtension(ctx, "bastion"); err != nil {
		return nil, err
	}
	var resources []bastionResource
	if err := c.runJSON(ctx, &resources, "resource", "list", "--resource-type", "Microsoft.Network/bastionHosts", "--subscription", subscriptionID, "--output", "json", "--only-show-errors"); err != nil {
		return nil, fmt.Errorf("list Azure Bastion resources: %w", err)
	}
	out := map[string]bastionRecord{}
	var mu stdsync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, resource := range resources {
		resource := resource
		g.Go(func() error {
			var bastion bastionRecord
			if err := c.runJSON(gctx, &bastion, "network", "bastion", "show", "--ids", resource.ID, "--subscription", subscriptionID, "--output", "json", "--only-show-errors"); err != nil {
				return err
			}
			if bastion.Name == "" {
				bastion.Name = resource.Name
			}
			if bastion.ResourceGroup == "" {
				bastion.ResourceGroup = resource.ResourceGroup
			}
			if !bastionUsable(bastion) {
				return nil
			}
			for _, ip := range bastion.IPConfigurations {
				if ip.Subnet == nil {
					continue
				}
				vnetID := vnetFromSubnet(ip.Subnet.ID)
				if vnetID == "" {
					continue
				}
				mu.Lock()
				if current, exists := out[strings.ToLower(vnetID)]; !exists || bastion.Name < current.Name {
					out[strings.ToLower(vnetID)] = bastion
				}
				mu.Unlock()
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("describe Azure Bastion resources: %w", err)
	}
	return out, nil
}

func bastionUsable(b bastionRecord) bool {
	if !b.EnableTunneling {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(b.SKU.Name)) {
	case "standard", "premium":
		return true
	default:
		return false
	}
}

func resourceGroupSelected(group string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if strings.EqualFold(strings.TrimSpace(filter), strings.TrimSpace(group)) {
			return true
		}
	}
	return false
}

func isRunningLinux(vm vmRecord) bool {
	linux := strings.EqualFold(vm.StorageProfile.OSDisk.OSType, "Linux") || vm.OSProfile.LinuxConfiguration != nil
	standalone := vm.VirtualMachineScaleSet == nil || strings.TrimSpace(vm.VirtualMachineScaleSet.ID) == ""
	return standalone && linux && strings.EqualFold(strings.TrimSpace(vm.PowerState), "VM running")
}

func firstAddress(value string) string {
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			return part
		}
	}
	return ""
}

func vmNetwork(vm vmRecord, nics map[string]nicRecord) (privateIP, vnetID string) {
	var candidates []nicRecord
	for _, ref := range vm.NetworkProfile.NetworkInterfaces {
		if nic, ok := nics[strings.ToLower(ref.ID)]; ok {
			if ref.Primary {
				candidates = append([]nicRecord{nic}, candidates...)
			} else {
				candidates = append(candidates, nic)
			}
		}
	}
	for _, nic := range candidates {
		for _, ip := range nic.IPConfigurations {
			if ip.PrivateIPAddress == "" || ip.Subnet == nil {
				continue
			}
			if privateIP == "" || ip.Primary {
				privateIP = ip.PrivateIPAddress
				vnetID = vnetFromSubnet(ip.Subnet.ID)
			}
			if ip.Primary {
				return privateIP, vnetID
			}
		}
	}
	return privateIP, vnetID
}

func vnetFromSubnet(id string) string {
	lower := strings.ToLower(strings.TrimSpace(id))
	idx := strings.LastIndex(lower, "/subnets/")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(id[:idx])
}

func vmSSHKeys(vm vmRecord) []string {
	if vm.OSProfile.LinuxConfiguration == nil {
		return nil
	}
	var keys []string
	for _, key := range vm.OSProfile.LinuxConfiguration.SSH.PublicKeys {
		if key.KeyData = strings.TrimSpace(key.KeyData); key.KeyData != "" {
			keys = append(keys, key.KeyData)
		}
	}
	return keys
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
	for _, dir := range []string{managedKeys, filepath.Join(home, ".ssh")} {
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
				return shortenHome(privatePath, home)
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

func shortenHome(path, home string) string {
	if home != "" {
		prefix := strings.TrimRight(home, string(filepath.Separator)) + string(filepath.Separator)
		if strings.HasPrefix(path, prefix) {
			return "~/" + filepath.ToSlash(strings.TrimPrefix(path, prefix))
		}
	}
	return path
}
