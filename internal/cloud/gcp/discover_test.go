package gcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"bast/internal/cloud"
)

func TestDiscoverRejectsPartialInventory(t *testing.T) {
	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
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
			return nil, errors.New("permission denied")
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	instances, err := client.Discover(context.Background(), cloud.DiscoverConfig{Home: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "incomplete GCP discovery") {
		t.Fatalf("expected incomplete discovery error, instances=%+v err=%v", instances, err)
	}
	if instances != nil {
		t.Fatalf("partial inventory must not be returned: %+v", instances)
	}
}

func TestDiscoverRecordsSuccessfulAccount(t *testing.T) {
	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK\n"), nil
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"secondary@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "projects list"):
			return []byte(`[{"projectId":"demo","name":"Demo","lifecycleState":"ACTIVE"}]`), nil
		case strings.Contains(joined, "project-info describe"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "instances list"):
			return []byte(`[{"name":"web","zone":"zones/z","networkInterfaces":[{"networkIP":"10.0.0.1"}]}]`), nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	instances, err := client.Discover(context.Background(), cloud.DiscoverConfig{Home: t.TempDir()})
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances=%+v err=%v", instances, err)
	}
	if instances[0].CredentialAccount != "secondary@example.com" {
		t.Fatalf("credential account = %q", instances[0].CredentialAccount)
	}
}
