package sync

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	awscloud "bast/internal/cloud/aws"
	"bast/internal/cloud/gcp"
	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

func TestStatusReleasesEngineLockBeforeExternalProbes(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(p, store)
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	engine.GCP.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		close(probeStarted)
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
	if !engine.mu.TryLock() {
		close(releaseProbe)
		t.Fatal("status held the engine lock during an external probe")
	}
	engine.mu.Unlock()
	close(releaseProbe)
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
