package azure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"bast/internal/sshconfig"
)

func TestDiscoverMapsDirectAndBastionVMs(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "production"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "production.pub"), []byte("ssh-ed25519 AAAATEST local\n"), 0644); err != nil {
		t.Fatal(err)
	}
	client := &Client{AZ: "az", Run: func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.HasPrefix(joined, "version"):
			return []byte(`{"azure-cli":"2.62.0"}`), nil
		case strings.HasPrefix(joined, "account list"):
			return []byte(`[{"id":"sub-1","name":"Production","tenantId":"tenant-1","state":"Enabled","isDefault":true}]`), nil
		case strings.HasPrefix(joined, "vm list"):
			return []byte(`[
				{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Compute/virtualMachines/web","name":"web","location":"uksouth","resourceGroup":"apps","powerState":"VM running","publicIps":"203.0.113.10","osProfile":{"adminUsername":"azureuser","linuxConfiguration":{"ssh":{"publicKeys":[{"keyData":"ssh-ed25519 AAAATEST cloud"}]}}},"storageProfile":{"osDisk":{"osType":"Linux"}},"networkProfile":{"networkInterfaces":[{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Network/networkInterfaces/web-nic","primary":true}]},"tags":{"environment":"production"}},
				{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Compute/virtualMachines/api","name":"api","location":"uksouth","resourceGroup":"apps","powerState":"VM running","publicIps":"","privateIps":"10.0.0.5","osProfile":{"adminUsername":"admin","linuxConfiguration":{"ssh":{"publicKeys":[]}}},"storageProfile":{"osDisk":{"osType":"Linux"}},"networkProfile":{"networkInterfaces":[{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Network/networkInterfaces/api-nic","primary":true}]},"tags":{}},
				{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Compute/virtualMachines/windows","name":"windows","resourceGroup":"apps","powerState":"VM running","storageProfile":{"osDisk":{"osType":"Windows"}}},
				{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Compute/virtualMachines/scale-worker","name":"scale-worker","resourceGroup":"apps","powerState":"VM running","storageProfile":{"osDisk":{"osType":"Linux"}},"virtualMachineScaleSet":{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Compute/virtualMachineScaleSets/workers"}}
			]`), nil
		case strings.HasPrefix(joined, "network nic list"):
			return []byte(`[{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Network/networkInterfaces/api-nic","virtualMachine":{"id":"/subscriptions/sub-1/resourceGroups/apps/providers/Microsoft.Compute/virtualMachines/api"},"ipConfigurations":[{"primary":true,"privateIPAddress":"10.0.0.5","subnet":{"id":"/subscriptions/sub-1/resourceGroups/network/providers/Microsoft.Network/virtualNetworks/core/subnets/apps"}}]}]`), nil
		case strings.HasPrefix(joined, "extension show --name bastion"):
			return []byte(`{"azure-cli":"2.62.0"}`), nil
		case strings.HasPrefix(joined, "resource list"):
			return []byte(`[{"id":"/subscriptions/sub-1/resourceGroups/network/providers/Microsoft.Network/bastionHosts/core-bastion","name":"core-bastion","resourceGroup":"network"}]`), nil
		case strings.HasPrefix(joined, "network bastion show"):
			return []byte(`{"name":"core-bastion","resourceGroup":"network","enableTunneling":true,"sku":{"name":"Standard"},"ipConfigurations":[{"subnet":{"id":"/subscriptions/sub-1/resourceGroups/network/providers/Microsoft.Network/virtualNetworks/core/subnets/AzureBastionSubnet"}}]}`), nil
		default:
			t.Fatalf("unexpected Azure args: %v", args)
			return nil, nil
		}
	}}
	discovery, err := client.Discover(context.Background(), DiscoverConfig{Home: home, BastExecutable: "/usr/local/bin/bast"})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 || !discovery.ConfirmedSubscriptions["sub-1"] {
		t.Fatalf("discovery = %+v", discovery)
	}
	byName := map[string]Instance{}
	for _, instance := range discovery.Instances {
		byName[instance.Name] = instance
	}
	web := byName["web"]
	if web.UseBastion || web.HostName != "203.0.113.10" || web.IdentityFile != "~/.ssh/production" || !web.IdentitiesOnly {
		t.Fatalf("web = %+v", web)
	}
	api := byName["api"]
	if !api.UseBastion || api.HostName != "10.0.0.5" || api.BastionName != "core-bastion" || api.BastionResourceGroup != "network" {
		t.Fatalf("api = %+v", api)
	}
	block := ToSyncHost(api, "", "/usr/local/bin/bast")
	for _, expected := range []string{"__azure-bastion-proxy", "--subscription sub-1", "--resource-port %p"} {
		if !strings.Contains(block.ProxyCommand, expected) {
			t.Fatalf("ProxyCommand %q missing %q", block.ProxyCommand, expected)
		}
	}
	if got := GroupPath(web); got != "Microsoft Azure/Production/apps" {
		t.Fatalf("GroupPath = %q", got)
	}
}

func TestDiscoverPreservesConfirmedSubscriptionsWhenOneFails(t *testing.T) {
	client := &Client{Run: func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.HasPrefix(joined, "version"):
			return []byte(`{"azure-cli":"2.62.0"}`), nil
		case strings.HasPrefix(joined, "account list"):
			return []byte(`[{"id":"good","name":"Good","state":"Enabled"},{"id":"bad","name":"Bad","state":"Enabled"}]`), nil
		case strings.Contains(joined, "vm list") && strings.Contains(joined, "--subscription bad"):
			return nil, errors.New("forbidden")
		case strings.Contains(joined, "vm list"):
			return []byte(`[]`), nil
		case strings.Contains(joined, "network nic list"):
			return []byte(`[]`), nil
		default:
			return nil, fmt.Errorf("unexpected command %s", joined)
		}
	}}
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !discovery.ConfirmedSubscriptions["good"] || discovery.ConfirmedSubscriptions["bad"] || len(discovery.Warnings) != 1 {
		t.Fatalf("discovery = %+v", discovery)
	}
}

func TestEnsureAccessPrefersMatchingLocalKey(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"id_azure": "private", "id_azure.pub": "ssh-ed25519 AAAAKEY local\n"} {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	client := &Client{Run: func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.HasPrefix(joined, "version"):
			return []byte(`{"azure-cli":"2.62.0"}`), nil
		case strings.HasPrefix(joined, "vm show"):
			return []byte(`{"id":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm","name":"vm","osProfile":{"adminUsername":"azureuser","linuxConfiguration":{"ssh":{"publicKeys":[{"keyData":"ssh-ed25519 AAAAKEY cloud"}]}}}}`), nil
		default:
			t.Fatalf("unexpected command: %s", joined)
			return nil, nil
		}
	}}
	result, err := client.EnsureAccess(context.Background(), "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm", EnsureConfig{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if result.User != "azureuser" || result.IdentityFile != "~/.ssh/id_azure" || result.CertificateFile != "" || !result.IdentitiesOnly {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckAvailableRequiresSupportedAzureCLIVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{name: "minimum", version: "2.62.0"},
		{name: "newer major", version: "3.0.0"},
		{name: "too old", version: "2.61.9", wantErr: true},
		{name: "malformed", version: "preview", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{Run: func(context.Context, []string, []string) ([]byte, error) {
				return []byte(fmt.Sprintf(`{"azure-cli":%q,"azure-cli-core":%q,"extensions":{}}`, test.version, test.version)), nil
			}}
			err := client.CheckAvailable(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("CheckAvailable() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestEnsureAccessGeneratesEntraCertificate(t *testing.T) {
	home := t.TempDir()
	azureDir := filepath.Join(home, ".ssh", "bast", "azure")
	client := &Client{Run: func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.HasPrefix(joined, "version"):
			return []byte(`{"azure-cli":"2.62.0"}`), nil
		case strings.HasPrefix(joined, "extension show --name ssh"), strings.HasPrefix(joined, "vm extension show"):
			return []byte(`{}`), nil
		case strings.HasPrefix(joined, "vm show"):
			return []byte(`{"id":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm","name":"vm","osProfile":{"adminUsername":"azureuser","linuxConfiguration":{"ssh":{"publicKeys":[]}}}}`), nil
		case strings.HasPrefix(joined, "ssh config"):
			configPath := argumentValue(args, "--file")
			keyPath := filepath.Join(azureDir, "entra key")
			certPath := filepath.Join(azureDir, "entra key-aadcert.pub")
			if err := os.WriteFile(keyPath, []byte("private"), 0666); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(certPath, []byte("cert"), 0666); err != nil {
				t.Fatal(err)
			}
			body := "Host rg-vm\n  User user@example.com\n  IdentityFile \"" + keyPath + "\"\n  CertificateFile \"" + certPath + "\"\n"
			if err := os.WriteFile(configPath, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			return nil, nil
		default:
			t.Fatalf("unexpected command: %s", joined)
			return nil, nil
		}
	}}
	result, err := client.EnsureAccess(context.Background(), "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm", EnsureConfig{Home: home, AzureDir: azureDir})
	if err != nil {
		t.Fatal(err)
	}
	if result.User != "user@example.com" || result.IdentityFile != "~/.ssh/bast/azure/entra key" || result.CertificateFile != "~/.ssh/bast/azure/entra key-aadcert.pub" {
		t.Fatalf("result = %+v", result)
	}
	for path, want := range map[string]os.FileMode{filepath.Join(azureDir, "entra key"): 0600, filepath.Join(azureDir, "entra key-aadcert.pub"): 0644} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("mode for %s = %v", path, info.Mode().Perm())
		}
	}
}

func TestProxyArguments(t *testing.T) {
	options, err := ParseProxyOptions([]string{"--subscription", "sub", "--bastion-group", "network", "--bastion", "core", "--target", "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm", "--resource-port", "2222"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"network", "bastion", "tunnel", "--name", "core", "--resource-group", "network", "--target-resource-id", options.TargetResourceID, "--resource-port", "2222", "--port", "50022", "--subscription", "sub", "--only-show-errors"}
	if got := BastionTunnelArgs(options, 50022); !slices.Equal(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBastionProxyCommandQuotesOpenSSHAndShellValues(t *testing.T) {
	command := BastionProxyCommand(Instance{
		SubscriptionID:       "sub%prod",
		BastionResourceGroup: "network group",
		BastionName:          "core'bastion",
		SyncID:               "/subscriptions/sub%prod/resourceGroups/apps/providers/Microsoft.Compute/virtualMachines/api",
	}, "/Applications/Bast Preview/bast")
	for _, expected := range []string{
		"'/Applications/Bast Preview/bast'",
		"sub%%prod",
		"'network group'",
		`'core'"'"'bastion'`,
		"--resource-port %p",
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("ProxyCommand %q missing %q", command, expected)
		}
	}
}

func TestProxyStreamsReturnsWhenTunnelCloses(t *testing.T) {
	client, server := net.Pipe()
	input, inputWriter := io.Pipe()
	defer input.Close()
	defer inputWriter.Close()
	done := make(chan error, 1)
	go func() { done <- proxyStreams(client, input, io.Discard) }()
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("proxyStreams did not return after the tunnel closed")
	}
}

func TestGeneratedAzureConfigIsAcceptedByOpenSSH(t *testing.T) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH client is not installed")
	}
	path := filepath.Join(t.TempDir(), "config")
	instance := Instance{
		SyncID: "/subscriptions/sub/resourceGroups/apps/providers/Microsoft.Compute/virtualMachines/api",
		Name:   "api", SubscriptionID: "sub", SubscriptionName: "Production", ResourceGroup: "apps",
		HostName: "10.0.0.5", User: "azureuser", IdentityFile: "~/.ssh/bast/azure/entra key",
		IdentitiesOnly: true, UseBastion: true, BastionName: "core", BastionResourceGroup: "network",
	}
	block := ToSyncHost(instance, "azure_test", "/Applications/Bast Preview/bast")
	block.CertificateFile = "~/.ssh/bast/azure/entra key-aadcert.pub"
	if err := sshconfig.WriteSyncConfig(path, []sshconfig.SyncHostInput{block}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(ssh, "-G", "-F", path, "azure_test").CombinedOutput()
	if err != nil {
		t.Fatalf("ssh -G failed: %v\n%s", err, out)
	}
	config := strings.ToLower(string(out))
	for _, expected := range []string{"hostname 10.0.0.5", "user azureuser", "certificatefile ~/.ssh/bast/azure/entra key-aadcert.pub", "proxycommand '/applications/bast preview/bast' __azure-bastion-proxy"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("ssh -G output missing %q:\n%s", expected, out)
		}
	}
}

func argumentValue(args []string, name string) string {
	for i := range args {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
