package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bast/internal/cloud"
)

func TestOSLoginKeyExpired(t *testing.T) {
	if osLoginKeyExpired("") {
		t.Fatal("empty expiration should not expire")
	}
	past := fmt.Sprintf("%d", time.Now().Add(-time.Hour).UnixMicro())
	if !osLoginKeyExpired(past) {
		t.Fatal("expected expired")
	}
	future := fmt.Sprintf("%d", time.Now().Add(time.Hour).UnixMicro())
	if osLoginKeyExpired(future) {
		t.Fatal("expected active")
	}
}

func TestParseSyncID(t *testing.T) {
	project, zone, name, err := ParseSyncID("projects/demo/zones/us-central1-a/instances/web")
	if err != nil || project != "demo" || zone != "us-central1-a" || name != "web" {
		t.Fatalf("ParseSyncID = %s %s %s (%v)", project, zone, name, err)
	}
	full := "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a/instances/web"
	project, zone, name, err = ParseSyncID(full)
	if err != nil || project != "demo" || zone != "us-central1-a" || name != "web" {
		t.Fatalf("ParseSyncID full = %s %s %s (%v)", project, zone, name, err)
	}
	if _, _, _, err := ParseSyncID("not/a/sync/id"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSSHKeyExpired(t *testing.T) {
	past := `ssh-rsa AAAA google-ssh {"userName":"ted@example.com","expireOn":"2020-01-01T00:00:00+0000"}`
	if !sshKeyExpired(past) {
		t.Fatal("expected expired")
	}
	future := `ssh-rsa AAAA google-ssh {"userName":"ted@example.com","expireOn":"2099-01-01T00:00:00+0000"}`
	if sshKeyExpired(future) {
		t.Fatal("expected not expired")
	}
	if sshKeyExpired("ssh-ed25519 AAAA lasting") {
		t.Fatal("lasting key should not be expired")
	}
}

func TestMergeSSHKeyMetadataDropsExpiredAndDupes(t *testing.T) {
	existing := "ubuntu:ssh-ed25519 AAAA lasting\n" +
		`ted:ssh-rsa BBBB google-ssh {"userName":"ted@example.com","expireOn":"2020-01-01T00:00:00+0000"}` + "\n" +
		"ubuntu:ssh-rsa CCCC comment\n"
	entry := "debian:ssh-rsa CCCC comment"
	got := mergeSSHKeyMetadata(existing, entry)
	if strings.Contains(got, "BBBB") || strings.Contains(got, "expireOn") {
		t.Fatalf("expired key kept: %s", got)
	}
	if strings.Count(got, "CCCC") != 1 {
		t.Fatalf("duplicate not collapsed: %s", got)
	}
	if !strings.Contains(got, "debian:ssh-rsa CCCC") || !strings.Contains(got, "ubuntu:ssh-ed25519 AAAA") {
		t.Fatalf("unexpected merge: %s", got)
	}
}

func TestEnsureAccessPublishesProjectKey(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1testkey bast-gcp\n"
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}

	var published string
	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK 500.0.0\n"), nil
		case strings.Contains(joined, "auth list"):
			return mustJSON(t, []map[string]string{{"account": "user@example.com", "status": "ACTIVE"}}), nil
		case strings.Contains(joined, "instances describe"):
			return mustJSON(t, map[string]any{
				"name": "web", "status": "RUNNING",
				"zone":     "zones/us-central1-a",
				"selfLink": "https://www.googleapis.com/compute/v1/projects/demo/zones/us-central1-a/instances/web",
				"networkInterfaces": []map[string]any{
					{"networkIP": "10.0.0.2", "accessConfigs": []map[string]string{{"natIP": "203.0.113.10"}}},
				},
				"disks": []map[string]any{
					{"boot": true, "licenses": []string{"https://www.googleapis.com/compute/v1/projects/debian-cloud/global/licenses/debian-12-bookworm"}},
				},
			}), nil
		case strings.Contains(joined, "project-info describe"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "os-login describe-profile"):
			return []byte(`{"posixAccounts":[]}`), nil
		case strings.Contains(joined, "instances add-metadata"):
			for _, arg := range args {
				if strings.HasPrefix(arg, "--metadata-from-file=ssh-keys=") {
					path := strings.TrimPrefix(arg, "--metadata-from-file=ssh-keys=")
					body, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					published = string(body)
				}
			}
			return []byte(""), nil
		case strings.Contains(joined, "project-info add-metadata"):
			t.Fatal("should prefer instance metadata")
			return nil, nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	result, err := client.EnsureAccess(context.Background(), "projects/demo/zones/us-central1-a/instances/web", EnsureConfig{
		Home:            home,
		PropagationWait: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.User != "debian" || result.IdentityFile != gcloudIdentityFile || !result.IdentitiesOnly || !result.KeyAdded {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(published, "debian:ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1testkey") {
		t.Fatalf("published keys = %q", published)
	}
}

func TestEnsureAccessSkipsPublishWhenKeyPresent(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1testkey bast-gcp\n"
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}

	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK 500.0.0\n"), nil
		case strings.Contains(joined, "auth list"):
			return mustJSON(t, []map[string]string{{"account": "user@example.com", "status": "ACTIVE"}}), nil
		case strings.Contains(joined, "instances describe"):
			return mustJSON(t, map[string]any{
				"name": "web", "status": "RUNNING",
				"zone":     "zones/us-central1-a",
				"selfLink": "projects/demo/zones/us-central1-a/instances/web",
				"networkInterfaces": []map[string]any{
					{"networkIP": "10.0.0.2", "accessConfigs": []map[string]string{{"natIP": "203.0.113.10"}}},
				},
				"disks": []map[string]any{
					{"boot": true, "initializeParams": map[string]string{"sourceImage": "projects/ubuntu-os-cloud/global/images/ubuntu-2204"}},
				},
			}), nil
		case strings.Contains(joined, "project-info describe"):
			return mustJSON(t, map[string]any{
				"commonInstanceMetadata": map[string]any{
					"items": []map[string]string{
						{"key": "ssh-keys", "value": "ubuntu:ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1testkey bast-gcp"},
					},
				},
			}), nil
		case strings.Contains(joined, "os-login describe-profile"):
			return []byte(`{"posixAccounts":[]}`), nil
		case strings.Contains(joined, "add-metadata"):
			t.Fatal("should not publish when key already present")
			return nil, nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	result, err := client.EnsureAccess(context.Background(), "projects/demo/zones/us-central1-a/instances/web", EnsureConfig{
		Home:            home,
		PropagationWait: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.KeyAdded || result.User != "ubuntu" {
		t.Fatalf("result = %+v", result)
	}
}

func TestEnsureAccessPrefersLocalManagedKey(t *testing.T) {
	home := t.TempDir()
	keys := filepath.Join(home, ".ssh", "bast", "keys")
	if err := os.MkdirAll(keys, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG9M+so2dj/OeGlcJcRbIvVQ76hrcdJjU2WC3x6wEjos ted@gcp\n"
	if err := os.WriteFile(filepath.Join(keys, "IRIS.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keys, "IRIS"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}

	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK\n"), nil
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"user@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "instances describe"):
			return mustJSON(t, map[string]any{
				"name": "web", "status": "RUNNING",
				"zone": "zones/us-central1-a",
				"networkInterfaces": []map[string]any{
					{"networkIP": "10.0.0.2", "accessConfigs": []map[string]string{{"natIP": "203.0.113.10"}}},
				},
				"metadata": map[string]any{
					"items": []map[string]string{
						{"key": "ssh-keys", "value": "ubuntu:" + strings.TrimSpace(pub)},
					},
				},
			}), nil
		case strings.Contains(joined, "project-info describe"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "add-metadata"), strings.Contains(joined, "os-login"):
			t.Fatalf("local key match should not publish or touch OS Login: %v", args)
			return nil, nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	var messages []string
	result, err := client.EnsureAccess(context.Background(), "projects/demo/zones/us-central1-a/instances/web", EnsureConfig{
		Home: home, ManagedKeys: keys, PropagationWait: -1,
		Status: func(msg string) { messages = append(messages, msg) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.KeyAdded || result.User != "ubuntu" || result.IdentityFile != "~/.ssh/bast/keys/IRIS" || !result.IdentitiesOnly {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "google_compute_engine")); !os.IsNotExist(err) {
		t.Fatalf("should not create google_compute_engine when a local key matches: %v", err)
	}
	if len(messages) == 0 || !strings.Contains(messages[len(messages)-1], "Using local SSH key") {
		t.Fatalf("status messages = %#v", messages)
	}
}

func TestEnsureAccessPublishesOSLoginKey(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1oslogin bast-gcp\n"
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}

	added := false
	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK\n"), nil
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"user@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "instances describe"):
			return []byte(`{
				"name":"web","zone":"zones/us-central1-a",
				"metadata":{"items":[{"key":"enable-oslogin","value":"TRUE"}]},
				"networkInterfaces":[{"networkIP":"10.0.0.2"}]
			}`), nil
		case strings.Contains(joined, "project-info describe"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "os-login describe-profile"):
			return []byte(`{"posixAccounts":[{"username":"oslogin_user","primary":true}]}`), nil
		case strings.Contains(joined, "os-login ssh-keys add"):
			if !strings.Contains(joined, "--key-file="+filepath.Join(sshDir, "google_compute_engine.pub")) {
				t.Fatalf("missing OS Login key file: %v", args)
			}
			added = true
			return nil, nil
		case strings.Contains(joined, "add-metadata"):
			t.Fatal("OS Login must not publish metadata keys")
			return nil, nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	result, err := client.EnsureAccess(context.Background(), "projects/demo/zones/us-central1-a/instances/web", EnsureConfig{
		Home: home, PropagationWait: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !added || result.User != "oslogin_user" || !result.KeyAdded {
		t.Fatalf("result=%+v added=%t", result, added)
	}
}

func TestEnsureAccessOSLoginSkipsWhenKeyPresent(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC1oslogin bast-gcp\n"
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "google_compute_engine.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}

	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK\n"), nil
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"user@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "instances describe"):
			return []byte(`{
				"name":"web","zone":"zones/us-central1-a",
				"metadata":{"items":[{"key":"enable-oslogin","value":"TRUE"}]},
				"networkInterfaces":[{"networkIP":"10.0.0.2"}]
			}`), nil
		case strings.Contains(joined, "project-info describe"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "os-login describe-profile"):
			return mustJSON(t, map[string]any{
				"posixAccounts": []map[string]any{{"username": "oslogin_user", "primary": true}},
				"sshPublicKeys": map[string]any{
					"fp": map[string]string{"key": strings.TrimSpace(pub)},
				},
			}), nil
		case strings.Contains(joined, "os-login ssh-keys add"):
			t.Fatal("should not re-add an OS Login key that is already present")
			return nil, nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	result, err := client.EnsureAccess(context.Background(), "projects/demo/zones/us-central1-a/instances/web", EnsureConfig{
		Home: home, PropagationWait: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.KeyAdded || result.User != "oslogin_user" || result.IdentityFile != gcloudIdentityFile {
		t.Fatalf("result=%+v", result)
	}
}

func TestEnsureAccessOSLoginPrefersLocalKey(t *testing.T) {
	home := t.TempDir()
	keys := filepath.Join(home, ".ssh", "bast", "keys")
	if err := os.MkdirAll(keys, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG9M+so2dj/OeGlcJcRbIvVQ76hrcdJjU2WC3x6wEjos ted@gcp\n"
	if err := os.WriteFile(filepath.Join(keys, "IRIS.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keys, "IRIS"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}

	client := New()
	client.Run = func(ctx context.Context, args []string, env []string) ([]byte, error) {
		joined := strings.Join(args[1:], " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("Google Cloud SDK\n"), nil
		case strings.Contains(joined, "auth list"):
			return []byte(`[{"account":"user@example.com","status":"ACTIVE"}]`), nil
		case strings.Contains(joined, "instances describe"):
			return []byte(`{
				"name":"web","zone":"zones/us-central1-a",
				"metadata":{"items":[{"key":"enable-oslogin","value":"TRUE"}]},
				"networkInterfaces":[{"networkIP":"10.0.0.2"}]
			}`), nil
		case strings.Contains(joined, "project-info describe"):
			return []byte(`{"commonInstanceMetadata":{"items":[]}}`), nil
		case strings.Contains(joined, "os-login describe-profile"):
			return mustJSON(t, map[string]any{
				"posixAccounts": []map[string]any{{"username": "oslogin_user", "primary": true}},
				"sshPublicKeys": map[string]any{
					"fp": map[string]string{"key": strings.TrimSpace(pub)},
				},
			}), nil
		case strings.Contains(joined, "os-login ssh-keys add"), strings.Contains(joined, "add-metadata"):
			t.Fatalf("should use existing local OS Login key: %v", args)
			return nil, nil
		default:
			t.Fatalf("unexpected gcloud args: %v", args)
			return nil, nil
		}
	}

	result, err := client.EnsureAccess(context.Background(), "projects/demo/zones/us-central1-a/instances/web", EnsureConfig{
		Home: home, ManagedKeys: keys, PropagationWait: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.KeyAdded || result.User != "oslogin_user" || result.IdentityFile != "~/.ssh/bast/keys/IRIS" {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "google_compute_engine")); !os.IsNotExist(err) {
		t.Fatalf("should not create google_compute_engine: %v", err)
	}
}

func TestBootImageUsesLicenses(t *testing.T) {
	inst := decodeInstance(t, `{
		"name":"test",
		"disks":[{"boot":true,"source":"projects/p/zones/z/disks/test","licenses":["https://www.googleapis.com/compute/v1/projects/debian-cloud/global/licenses/debian-13-trixie"]}]
	}`)
	got := bootImage(inst)
	if !strings.Contains(got, "debian") {
		t.Fatalf("bootImage = %q", got)
	}
	if imageSSHUser(got) != "debian" {
		t.Fatalf("imageSSHUser = %q", imageSSHUser(got))
	}
}

func TestResolveAuthSkipsExpiredMetadataKeys(t *testing.T) {
	home := t.TempDir()
	keys := filepath.Join(home, ".ssh", "bast", "keys")
	if err := os.MkdirAll(keys, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG9M+so2dj/OeGlcJcRbIvVQ76hrcdJjU2WC3x6wEjos ted@gcp\n"
	if err := os.WriteFile(filepath.Join(keys, "IRIS.pub"), []byte(pub), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keys, "IRIS"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	expired := strings.TrimSpace(pub) + ` google-ssh {"userName":"ted@example.com","expireOn":"2020-01-01T00:00:00+0000"}`
	inst := cloudInstance(t, expired)
	ResolveAuth(&inst, home, keys, "")
	if inst.IdentitiesOnly {
		t.Fatalf("expired key should not match: %+v", inst)
	}
}

func cloudInstance(t *testing.T, pub string) cloud.Instance {
	t.Helper()
	keys := parseSSHKeys("ubuntu:" + pub)
	return cloud.Instance{SSHKeys: keys, Image: "ubuntu-2204"}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
