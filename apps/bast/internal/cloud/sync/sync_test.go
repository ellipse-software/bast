package sync

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	awscloud "bast/internal/cloud/aws"
	azurecloud "bast/internal/cloud/azure"
	"bast/internal/cloud/gcp"
	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

func TestStatusDoesNotHoldProviderLocksDuringExternalProbes(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	var probeOnce sync.Once
	engine.GCP.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		probeOnce.Do(func() { close(probeStarted) })
		select {
		case <-releaseProbe:
			return []byte(`[]`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	engine.AWS.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("aws-cli/2"), nil
		case strings.Contains(joined, "configure list-profiles"):
			return nil, nil
		default:
			return nil, errors.New("unexpected AWS status command")
		}
	}

	done := make(chan error, 1)
	go func() {
		_, statusErr := engine.Status(context.Background())
		done <- statusErr
	}()

	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("status probe did not start")
	}
	if !engine.gcpMu.TryLock() || !engine.awsMu.TryLock() || !engine.azureMu.TryLock() {
		close(releaseProbe)
		t.Fatal("status held a provider lock during an external probe")
	}
	engine.gcpMu.Unlock()
	engine.awsMu.Unlock()
	engine.azureMu.Unlock()
	close(releaseProbe)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestEnsureAWSAccessDoesNotBlockBehindGCPSync(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	engine.gcpMu.Lock()
	defer engine.gcpMu.Unlock()

	started := make(chan struct{})
	var once sync.Once
	engine.AWS.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		once.Do(func() { close(started) })
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("aws-cli/2"), nil
		case strings.Contains(joined, "configure list-profiles"):
			return []byte("default\n"), nil
		case strings.Contains(joined, "sts get-caller-identity"):
			return nil, errors.New("not authenticated")
		default:
			return nil, errors.New("unexpected: " + joined)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- engine.EnsureAWSAccess(context.Background(), sshconfig.Host{
			Synced: true, SyncSource: awscloud.ProviderName, SyncID: "arn:aws:ec2:eu-west-1:123456789012:instance/i-123",
		}, nil)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("EnsureAWSAccess blocked behind GCP lock")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected AWS auth error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("EnsureAWSAccess did not finish")
	}
}

func TestStatusProbesProvidersConcurrently(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	gcpStarted := make(chan struct{})
	awsStarted := make(chan struct{})
	azureStarted := make(chan struct{})
	var gcpOnce, awsOnce, azureOnce sync.Once
	engine.GCP.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		gcpOnce.Do(func() { close(gcpStarted) })
		select {
		case <-release:
			return []byte(`[]`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	engine.AWS.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		if strings.Contains(strings.Join(args[1:], " "), "--version") {
			awsOnce.Do(func() { close(awsStarted) })
			select {
			case <-release:
				return []byte("aws-cli/2"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return nil, nil
	}
	engine.Azure.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		if strings.HasPrefix(joined, "version") {
			azureOnce.Do(func() { close(azureStarted) })
			select {
			case <-release:
				return []byte(`{"azure-cli":"2.62.0"}`), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		switch {
		case strings.HasPrefix(joined, "account list"):
			return []byte(`[]`), nil
		case strings.HasPrefix(joined, "extension show"):
			return []byte(`{}`), nil
		default:
			return nil, errors.New("unexpected Azure status command")
		}
	}

	done := make(chan error, 1)
	go func() {
		_, statusErr := engine.Status(context.Background())
		done <- statusErr
	}()
	for name, started := range map[string]<-chan struct{}{
		"GCP": gcpStarted, "AWS": awsStarted, "Azure": azureStarted,
	} {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("%s status probe did not start concurrently", name)
		}
	}
	close(release)
	released = true
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSyncAWSReconcileAndDisablePreservesGCP(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	engine.AWS.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("aws-cli/2"), nil
		case strings.Contains(joined, "configure list-profiles"):
			return []byte("default\n"), nil
		case strings.Contains(joined, "sts get-caller-identity"):
			return []byte(`{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`), nil
		case strings.Contains(joined, "describe-regions"):
			return []byte(`{"Regions":[{"RegionName":"eu-west-1"}]}`), nil
		case strings.Contains(joined, "describe-instances"):
			return []byte(`{"Reservations":[{"Instances":[{"InstanceId":"i-123","ImageId":"ami-1","PublicIpAddress":"203.0.113.10","VpcId":"vpc-1","SubnetId":"subnet-1","Placement":{"AvailabilityZone":"eu-west-1a"},"Tags":[{"Key":"Name","Value":"web"}]}]}]}`), nil
		case strings.Contains(joined, "describe-images"):
			return []byte(`{"Images":[{"ImageId":"ami-1","Name":"ubuntu"}]}`), nil
		case strings.Contains(joined, "describe-instance-connect-endpoints"):
			return []byte(`{"InstanceConnectEndpoints":[]}`), nil
		default:
			t.Fatalf("unexpected AWS args: %v", args)
			return nil, nil
		}
	}
	result, err := engine.SyncAWS(context.Background())
	if err != nil || result.Count != 1 || result.Provider != awscloud.ProviderName {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	hosts := mustDiscoverHosts(t, engine)
	if len(hosts) != 1 || hosts[0].SyncSource != "aws" {
		t.Fatalf("hosts = %+v", hosts)
	}
	meta := store.Host(hosts[0].Alias)
	if meta.Group != "Amazon EC2/default/eu-west-1" || meta.Label != "web" {
		t.Fatalf("metadata = %+v", meta)
	}
	if err := engine.Config.EnsureSyncInclude(p.SyncGCPConfig); err != nil {
		t.Fatal(err)
	}
	if err := engine.DisableAWS(context.Background()); err != nil {
		t.Fatal(err)
	}
	managed, err := os.ReadFile(p.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(managed), "sync/aws/config") || !strings.Contains(string(managed), "sync/gcp/config") {
		t.Fatalf("managed config after AWS disable:\n%s", managed)
	}
}

func TestSyncAzurePreservesFailedSubscriptionInventory(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	engine.BastExecutable = "/usr/local/bin/bast"
	failBad := false
	emptyGood := false
	engine.Azure.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.HasPrefix(joined, "version"):
			return []byte(`{"azure-cli":"2.62.0"}`), nil
		case strings.HasPrefix(joined, "account list"):
			return []byte(`[{"id":"good","name":"Good","state":"Enabled"},{"id":"bad","name":"Bad","state":"Enabled"}]`), nil
		case strings.HasPrefix(joined, "network nic list"):
			return []byte(`[]`), nil
		case strings.HasPrefix(joined, "vm list") && strings.Contains(joined, "--subscription bad"):
			if failBad {
				return nil, errors.New("permission denied")
			}
			return []byte(`[{
				"id":"/subscriptions/bad/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/bad-vm",
				"name":"bad-vm","resourceGroup":"rg","location":"uksouth","powerState":"VM running","publicIps":"203.0.113.20",
				"osProfile":{"adminUsername":"azureuser","linuxConfiguration":{"ssh":{"publicKeys":[]}}},"storageProfile":{"osDisk":{"osType":"Linux"}}
			}]`), nil
		case strings.HasPrefix(joined, "vm list") && strings.Contains(joined, "--subscription good"):
			if emptyGood {
				return []byte(`[]`), nil
			}
			return []byte(`[{
				"id":"/subscriptions/good/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/good-vm",
				"name":"good-vm","resourceGroup":"rg","location":"uksouth","powerState":"VM running","publicIps":"203.0.113.10",
				"osProfile":{"adminUsername":"azureuser","linuxConfiguration":{"ssh":{"publicKeys":[]}}},"storageProfile":{"osDisk":{"osType":"Linux"}}
			}]`), nil
		default:
			t.Fatalf("unexpected Azure args: %v", args)
			return nil, nil
		}
	}
	first, err := engine.SyncAzure(context.Background())
	if err != nil || first.Count != 2 || first.Provider != azurecloud.ProviderName {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	failBad, emptyGood = true, true
	second, err := engine.SyncAzure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Count != 1 || !strings.Contains(second.Error, "Bad") {
		t.Fatalf("second = %+v", second)
	}
	hosts := mustDiscoverHosts(t, engine)
	if len(hosts) != 1 || hosts[0].SyncID != "/subscriptions/bad/resourcegroups/rg/providers/microsoft.compute/virtualmachines/bad-vm" {
		t.Fatalf("hosts = %+v", hosts)
	}
	if store.Host("azure_Good_rg_good-vm").Label != "" {
		t.Fatal("confirmed subscription host metadata was not deleted")
	}
	if err := engine.DisableAzure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.Azure().Enabled {
		t.Fatal("Azure integration remained enabled")
	}
}

func TestSyncAzureRenamesPreservedHostWhenAliasIsReused(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	secondSync := false
	engine.Azure.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.HasPrefix(joined, "version"):
			return []byte(`{"azure-cli":"2.62.0"}`), nil
		case strings.HasPrefix(joined, "account list"):
			if secondSync {
				return []byte(`[{"id":"bad","name":"Shared","state":"Enabled"},{"id":"good","name":"Shared","state":"Enabled"}]`), nil
			}
			return []byte(`[{"id":"bad","name":"Shared","state":"Enabled"}]`), nil
		case strings.HasPrefix(joined, "network nic list"):
			return []byte(`[]`), nil
		case strings.HasPrefix(joined, "vm list") && strings.Contains(joined, "--subscription bad"):
			if secondSync {
				return nil, errors.New("permission denied")
			}
			return []byte(`[{"id":"/subscriptions/bad/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/web","name":"web","resourceGroup":"rg","powerState":"VM running","publicIps":"203.0.113.20","storageProfile":{"osDisk":{"osType":"Linux"}}}]`), nil
		case strings.HasPrefix(joined, "vm list") && strings.Contains(joined, "--subscription good"):
			return []byte(`[{"id":"/subscriptions/good/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/web","name":"web","resourceGroup":"rg","powerState":"VM running","publicIps":"203.0.113.10","storageProfile":{"osDisk":{"osType":"Linux"}}}]`), nil
		default:
			return nil, errors.New("unexpected Azure sync command")
		}
	}
	if _, err := engine.SyncAzure(context.Background()); err != nil {
		t.Fatal(err)
	}
	const alias = "azure_Shared_rg_web"
	if err := store.SetHost(alias, metadata.Host{Label: "Preserved", Favorite: true, Group: "Custom"}); err != nil {
		t.Fatal(err)
	}
	secondSync = true
	if _, err := engine.SyncAzure(context.Background()); err != nil {
		t.Fatal(err)
	}

	hosts := mustDiscoverHosts(t, engine)
	byAlias := make(map[string]sshconfig.Host, len(hosts))
	for _, host := range hosts {
		byAlias[host.Alias] = host
	}
	if !strings.Contains(byAlias[alias].SyncID, "/subscriptions/good/") {
		t.Fatalf("new host did not receive base alias: %+v", byAlias[alias])
	}
	preservedAlias := alias + "_2"
	if !strings.Contains(byAlias[preservedAlias].SyncID, "/subscriptions/bad/") {
		t.Fatalf("preserved host was not renamed: %+v", byAlias[preservedAlias])
	}
	if preserved := store.Host(preservedAlias); preserved.Label != "Preserved" || !preserved.Favorite || preserved.Group != "Custom" {
		t.Fatalf("preserved metadata = %+v", preserved)
	}
	if current := store.Host(alias); current.Label != "web" || current.Favorite {
		t.Fatalf("new host metadata = %+v", current)
	}
}

func TestSyncGCPReconcile(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	engine.GCP.Run = fakeGCloud(t, map[string]any{
		"auth list": []map[string]string{{"account": "user@example.com", "status": "ACTIVE"}},
		"projects list": []map[string]string{
			{"projectId": "demo", "name": "Demo Project", "lifecycleState": "ACTIVE"},
		},
		"compute instances list": []map[string]any{
			{
				"name": "web-01", "status": "RUNNING",
				"zone":     "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a",
				"selfLink": "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a/instances/web-01",
				"networkInterfaces": []map[string]any{
					{"networkIP": "10.0.0.2"},
				},
			},
			{
				"name": "api-01", "status": "RUNNING",
				"zone":     "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a",
				"selfLink": "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a/instances/api-01",
				"networkInterfaces": []map[string]any{
					{
						"networkIP": "10.0.0.3",
						"accessConfigs": []map[string]string{
							{"natIP": "203.0.113.10"},
						},
					},
				},
			},
		},
	})

	result, err := engine.SyncGCP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 2 {
		t.Fatalf("count = %d", result.Count)
	}
	if !store.GCP().Enabled || store.GCP().LastInstanceCount != 2 {
		t.Fatalf("integration = %+v", store.GCP())
	}

	hosts, err := engine.Config.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts = %+v", hosts)
	}
	for _, host := range hosts {
		if !host.Synced || host.SyncSource != "gcp" {
			t.Fatalf("host not synced: %+v", host)
		}
		meta := store.Host(host.Alias)
		if !strings.HasPrefix(meta.Group, "Google Cloud/") || meta.Label == "" {
			t.Fatalf("metadata = %+v for %s", meta, host.Alias)
		}
	}

	raw, err := os.ReadFile(p.SyncGCPConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "ProxyCommand") || !strings.Contains(string(raw), "203.0.113.10") {
		t.Fatalf("sync config unexpected: %s", raw)
	}

	result2, err := engine.SyncGCP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result2.Count != 2 {
		t.Fatalf("second count = %d", result2.Count)
	}
	hosts2, _ := engine.Config.Discover()
	aliases1, aliases2 := map[string]bool{}, map[string]bool{}
	for _, h := range hosts {
		aliases1[h.Alias] = true
	}
	for _, h := range hosts2 {
		aliases2[h.Alias] = true
	}
	for alias := range aliases1 {
		if !aliases2[alias] {
			t.Fatalf("alias %q changed on resync", alias)
		}
	}

	if err := engine.DisableGCP(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.GCP().Enabled {
		t.Fatal("expected disabled")
	}
	hosts3, err := engine.Config.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts3) != 0 {
		t.Fatalf("hosts after disable = %+v", hosts3)
	}
}

func TestSyncGCPPreservesInventoryAfterPartialDiscovery(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	failUnavailable := false
	engine.GCP.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK\n"), nil
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"user@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "projects list"):
			return []byte(`[
				{"projectId":"good","name":"Good","lifecycleState":"ACTIVE"},
				{"projectId":"unavailable","name":"Unavailable","lifecycleState":"ACTIVE"}
			]`), nil
		case strings.Contains(joined, "project-info describe"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "instances list") && strings.Contains(joined, "--project=good"):
			return []byte(`[{"name":"web","zone":"zones/z","networkInterfaces":[{"networkIP":"10.0.0.1"}]}]`), nil
		case strings.Contains(joined, "instances list") && strings.Contains(joined, "--project=unavailable"):
			if failUnavailable {
				return nil, errors.New("permission denied")
			}
			return []byte(`[{"name":"db","zone":"zones/z","networkInterfaces":[{"networkIP":"10.0.0.2"}]}]`), nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	if result, err := engine.SyncGCP(context.Background()); err != nil || result.Count != 2 {
		t.Fatalf("initial sync result=%+v err=%v", result, err)
	}
	var unavailableAlias string
	for _, host := range mustDiscoverHosts(t, engine) {
		if strings.Contains(host.SyncID, "projects/unavailable/") {
			unavailableAlias = host.Alias
		}
	}
	if unavailableAlias == "" {
		t.Fatal("unavailable project host not found")
	}
	meta := store.Host(unavailableAlias)
	meta.Notes = "preserve me"
	if err := store.SetHost(unavailableAlias, meta); err != nil {
		t.Fatal(err)
	}

	failUnavailable = true
	result, err := engine.SyncGCP(context.Background())
	if err != nil {
		t.Fatalf("partial discovery should succeed while preserving hosts: %v", err)
	}
	if result.Count != 2 {
		t.Fatalf("expected preserved inventory count 2, got %d (%+v)", result.Count, result)
	}
	if result.Error == "" || !strings.Contains(result.Error, "unavailable") {
		t.Fatalf("expected warning about unavailable project, got %q", result.Error)
	}
	if hosts := mustDiscoverHosts(t, engine); len(hosts) != 2 {
		t.Fatalf("partial sync replaced inventory: %+v", hosts)
	}
	if got := store.Host(unavailableAlias).Notes; got != "preserve me" {
		t.Fatalf("metadata was lost: %q", got)
	}
}

func mustDiscoverHosts(t *testing.T, engine *Engine) []sshconfig.Host {
	t.Helper()
	hosts, err := engine.Config.Discover()
	if err != nil {
		t.Fatal(err)
	}
	return hosts
}

func TestIsSyncedGroup(t *testing.T) {
	if !IsSyncedGroup("Google Cloud") || !IsSyncedGroup("Google Cloud/demo") ||
		!IsSyncedGroup("GCP") || !IsSyncedGroup("GCP/demo") ||
		!IsSyncedGroup("Amazon EC2") || !IsSyncedGroup("Amazon EC2/default") ||
		!IsSyncedGroup("AWS/default") || IsSyncedGroup("Work") {
		t.Fatal("IsSyncedGroup mismatch")
	}
}

func TestEnsureGCPAccessUpdatesSyncConfig(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1ensure bast-gcp\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "google_compute_engine"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "google_compute_engine.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}

	engine := New(p, store)
	engine.EnsureAccessWait = -1
	engine.GCP.Run = fakeGCloud(t, map[string]any{
		"auth list": []map[string]string{{"account": "user@example.com", "status": "ACTIVE"}},
		"projects list": []map[string]string{
			{"projectId": "demo", "name": "Demo Project", "lifecycleState": "ACTIVE"},
		},
		"compute instances list": []map[string]any{
			{
				"name": "web-01", "status": "RUNNING",
				"zone":     "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a",
				"selfLink": "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a/instances/web-01",
				"networkInterfaces": []map[string]any{
					{"networkIP": "10.0.0.2", "accessConfigs": []map[string]string{{"natIP": "203.0.113.10"}}},
				},
				"disks": []map[string]any{
					{"boot": true, "licenses": []string{"https://www.googleapis.com/compute/v1/projects/debian-cloud/global/licenses/debian-12-bookworm"}},
				},
			},
		},
		"instances describe": map[string]any{
			"name": "web-01", "status": "RUNNING",
			"zone":     "zones/us-central1-a",
			"selfLink": "projects/demo/zones/us-central1-a/instances/web-01",
			"networkInterfaces": []map[string]any{
				{"networkIP": "10.0.0.2", "accessConfigs": []map[string]string{{"natIP": "203.0.113.10"}}},
			},
			"disks": []map[string]any{
				{"boot": true, "licenses": []string{"https://www.googleapis.com/compute/v1/projects/debian-cloud/global/licenses/debian-12-bookworm"}},
			},
		},
		"os-login describe-profile": map[string]any{"posixAccounts": []any{}},
	})

	if _, err := engine.SyncGCP(context.Background()); err != nil {
		t.Fatal(err)
	}
	hosts, err := engine.Config.Discover()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("hosts = %+v err=%v", hosts, err)
	}
	if err := engine.EnsureGCPAccess(context.Background(), hosts[0], nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p.SyncGCPConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "User debian") || !strings.Contains(text, "IdentityFile ~/.ssh/google_compute_engine") {
		t.Fatalf("ensure did not update auth:\n%s", text)
	}
}

func fakeGCloud(t *testing.T, responses map[string]any) gcp.Runner {
	t.Helper()
	return func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK 500.0.0\n"), nil
		case strings.Contains(joined, "auth list"):
			return mustJSON(t, responses["auth list"]), nil
		case strings.Contains(joined, "projects list"):
			return mustJSON(t, responses["projects list"]), nil
		case strings.Contains(joined, "instances describe"):
			if value, ok := responses["instances describe"]; ok {
				return mustJSON(t, value), nil
			}
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		case strings.Contains(joined, "compute instances list"):
			return mustJSON(t, responses["compute instances list"]), nil
		case strings.Contains(joined, "project-info describe"):
			if value, ok := responses["project-info describe"]; ok {
				return mustJSON(t, value), nil
			}
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "instances add-metadata"):
			return []byte(""), nil
		case strings.Contains(joined, "project-info add-metadata"):
			return []byte(""), nil
		case strings.Contains(joined, "os-login describe-profile"):
			if value, ok := responses["os-login describe-profile"]; ok {
				return mustJSON(t, value), nil
			}
			return []byte(`{"posixAccounts":[{"username":"oslogin_user","primary":true}]}`), nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateServiceAccountPath(t *testing.T) {
	if err := ValidateServiceAccountPath(""); err == nil {
		t.Fatal("expected empty path error")
	}
	dir := t.TempDir()
	if err := ValidateServiceAccountPath(dir); err == nil || !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("expected directory error, got %v", err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateServiceAccountPath(bad); err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("expected JSON error, got %v", err)
	}
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`{"type":"service_account"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateServiceAccountPath(good); err != nil {
		t.Fatal(err)
	}
}
