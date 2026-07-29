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

	discovery, err := client.DiscoverAll(context.Background(), cloud.DiscoverConfig{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 || discovery.Instances[0].Name != "web" {
		t.Fatalf("DiscoverAll instances=%+v", discovery.Instances)
	}
	if !discovery.ConfirmedProjects["good"] || discovery.ConfirmedProjects["unavailable"] {
		t.Fatalf("confirmed projects=%v", discovery.ConfirmedProjects)
	}
	if len(discovery.Warnings) == 0 {
		t.Fatal("expected warning for unavailable project")
	}
}

func TestDiscoverSkipsDisabledComputeAPI(t *testing.T) {
	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"user@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "projects list"):
			return []byte(`[
				{"projectId":"good","name":"Good","lifecycleState":"ACTIVE"},
				{"projectId":"nocompute","name":"No Compute","lifecycleState":"ACTIVE"}
			]`), nil
		case strings.Contains(joined, "project-info describe") && strings.Contains(joined, "--project=good"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "instances list") && strings.Contains(joined, "--project=good"):
			return []byte(`[{"name":"web","zone":"zones/z","networkInterfaces":[{"networkIP":"10.0.0.1"}]}]`), nil
		case strings.Contains(joined, "instances list") && strings.Contains(joined, "--project=nocompute"):
			return nil, errors.New("API [compute.googleapis.com] not enabled on project [nocompute]. SERVICE_DISABLED")
		case strings.Contains(joined, "project-info describe") && strings.Contains(joined, "--project=nocompute"):
			t.Fatal("disabled compute projects should not load project metadata")
			return nil, nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	instances, err := client.Discover(context.Background(), cloud.DiscoverConfig{Home: t.TempDir()})
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances=%+v err=%v", instances, err)
	}
}

func TestDiscoverSkipsProjectMetadataForEmptyProjects(t *testing.T) {
	client := New()
	metaCalls := 0
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"user@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "projects list"):
			return []byte(`[{"projectId":"empty","name":"Empty","lifecycleState":"ACTIVE"}]`), nil
		case strings.Contains(joined, "instances list"):
			return []byte(`[]`), nil
		case strings.Contains(joined, "project-info describe"):
			metaCalls++
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}
	instances, err := client.Discover(context.Background(), cloud.DiscoverConfig{Home: t.TempDir()})
	if err != nil || len(instances) != 0 {
		t.Fatalf("instances=%+v err=%v", instances, err)
	}
	if metaCalls != 0 {
		t.Fatalf("expected no project-info calls for empty projects, got %d", metaCalls)
	}
}

func TestDiscoverAllKeepsGoingWhenSecondaryAccountAuthFails(t *testing.T) {
	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK\n"), nil
		case strings.Contains(joined, "auth list"):
			return []byte(`[
				{"account":"active@example.com","status":"ACTIVE"},
				{"account":"expired@example.com","status":""}
			]`), nil
		case strings.Contains(joined, "projects list") && strings.Contains(joined, "--account=expired@example.com"):
			return nil, errors.New("There was a problem refreshing your current auth tokens: Reauthentication failed. cannot prompt during non-interactive execution.")
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

	discovery, err := client.DiscoverAll(context.Background(), cloud.DiscoverConfig{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 {
		t.Fatalf("instances=%+v", discovery.Instances)
	}
	if len(discovery.Warnings) == 0 || !strings.Contains(discovery.Warnings[0], "expired@example.com") {
		t.Fatalf("warnings=%v", discovery.Warnings)
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
