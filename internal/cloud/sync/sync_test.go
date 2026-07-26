package sync

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"bast/internal/cloud/gcp"
	"bast/internal/metadata"
	"bast/internal/paths"
)

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
		if !strings.HasPrefix(meta.Group, "GCP/") || meta.Label == "" {
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

func TestIsSyncedGroup(t *testing.T) {
	if !IsSyncedGroup("GCP") || !IsSyncedGroup("GCP/demo") || IsSyncedGroup("Work") {
		t.Fatal("IsSyncedGroup mismatch")
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
		case strings.Contains(joined, "compute instances list"):
			return mustJSON(t, responses["compute instances list"]), nil
		case strings.Contains(joined, "project-info describe"):
			if value, ok := responses["project-info describe"]; ok {
				return mustJSON(t, value), nil
			}
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "os-login describe-profile"):
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
