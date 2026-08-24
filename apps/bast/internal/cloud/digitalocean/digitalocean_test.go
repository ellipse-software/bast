package digitalocean

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoverMapsPublicDropletsAndSkipsOthers(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItestkey do-test"
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), []byte(pub+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nkey\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1", "email": "user@example.com", "team": map[string]string{"uuid": "team-1", "name": "Work"}}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []map[string]any{{"id": 9, "name": "laptop", "public_key": pub}}, nil
		case strings.Contains(joined, "droplet list"):
			return []any{
				sampleDroplet(3164444, "web", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.10", "", []string{"web"}),
				sampleDroplet(3164445, "db", "off", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.11", "", nil),
				sampleDroplet(3164446, "win", "active", "nyc3", "Windows", "windows-2022", "203.0.113.12", "", nil),
				sampleDroplet(3164447, "priv", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "", "10.0.0.8", nil),
			}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	discovery, err := client.Discover(context.Background(), DiscoverConfig{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
	byName := map[string]Instance{}
	for _, inst := range discovery.Instances {
		byName[inst.Name] = inst
	}
	web := byName["web"]
	if web.Name != "web" || web.HostName != "203.0.113.10" || web.User != "root" ||
		web.SyncID != "do:team-1:3164444" || web.Context != "default" || web.Region != "nyc3" ||
		web.IdentityFile != "~/.ssh/id_ed25519" || !web.IdentitiesOnly || HostLooksStopped(web.Tags, web.Status) {
		t.Fatalf("web = %+v", web)
	}
	db := byName["db"]
	if db.Status != "off" || !HostLooksStopped(db.Tags, db.Status) || db.HostName != "203.0.113.11" {
		t.Fatalf("stopped droplet = %+v", db)
	}
	if GroupPath(web) != "DigitalOcean/default/nyc3" {
		t.Fatalf("GroupPath = %q", GroupPath(web))
	}
	if AliasFor(web) != "do_default_nyc3_web" {
		t.Fatalf("AliasFor = %q", AliasFor(web))
	}
	if !discovery.ConfirmedContexts["default"] {
		t.Fatalf("confirmed = %v", discovery.ConfirmedContexts)
	}
	if len(discovery.Warnings) != 1 || !strings.Contains(discovery.Warnings[0], "private-only") {
		t.Fatalf("warnings = %v", discovery.Warnings)
	}
}

func TestDiscoverPrefersPublicIPv6(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1"}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []any{}, nil
		case strings.Contains(joined, "droplet list"):
			item := sampleDroplet(7, "edge", "active", "ams3", "Ubuntu", "ubuntu-24-04-x64", "", "", nil)
			item["networks"] = map[string]any{
				"v4": []any{map[string]string{"ip_address": "10.1.0.9", "type": "private"}},
				"v6": []any{map[string]string{"ip_address": "2001:db8::10", "type": "public"}},
			}
			return []any{item}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 || discovery.Instances[0].HostName != "2001:db8::10" {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
}

func TestDiscoverDedupesAcrossContexts(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{
				{"name": "work", "current": true},
				{"name": "personal", "current": false},
			}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1", "team": map[string]string{"uuid": "team-1"}}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []any{}, nil
		case strings.Contains(joined, "droplet list"):
			return []any{sampleDroplet(1, "web", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.10", "", nil)}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 || discovery.Instances[0].Context != "work" {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
}

func TestDiscoverAppliesFilters(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{
				{"name": "work", "current": true},
				{"name": "personal", "current": false},
			}, nil
		case strings.Contains(joined, "--context personal"):
			return nil, errors.New("should not query filtered context")
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1"}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []any{}, nil
		case strings.Contains(joined, "droplet list"):
			return []any{
				sampleDroplet(1, "nyc", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.10", "", nil),
				sampleDroplet(2, "sfo", "active", "sfo3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.11", "", nil),
			}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{
		ContextFilter: []string{"WORK"}, RegionFilter: []string{"nyc3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 || discovery.Instances[0].Name != "nyc" {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
	if discovery.ConfirmedContexts["work"] {
		t.Fatalf("full context should not be confirmed when a region filter is set: %v", discovery.ConfirmedContexts)
	}
	if !discovery.ConfirmedScopes["work/nyc3"] {
		t.Fatalf("scopes = %v", discovery.ConfirmedScopes)
	}
}

func TestDiscoverIncompleteWhenEveryContextFails(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return nil, errors.New("401 unauthorized")
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	_, err := client.Discover(context.Background(), DiscoverConfig{})
	if err == nil || !strings.Contains(err.Error(), "incomplete DigitalOcean discovery") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverUsesCoreOSUser(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1"}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []any{}, nil
		case strings.Contains(joined, "droplet list"):
			return []any{sampleDroplet(3, "core", "active", "nyc3", "Fedora", "fedora-coreos-40", "203.0.113.20", "", nil)}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 || discovery.Instances[0].User != "core" {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
}

func TestEnsureAccessRefreshesIPAndPinsKey(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIrefresh do-test"
	if err := os.WriteFile(filepath.Join(sshDir, "do.pub"), []byte(pub+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "do"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nkey\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1", "team": map[string]string{"uuid": "team-1"}}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []map[string]any{{"public_key": pub}}, nil
		case strings.Contains(joined, "droplet get"):
			return sampleDroplet(3164444, "web", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "198.51.100.9", "", nil), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	result, err := client.EnsureAccess(context.Background(), "do:team-1:3164444", EnsureConfig{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if result.HostName != "198.51.100.9" || result.User != "root" || result.IdentityFile != "~/.ssh/do" || !result.IdentitiesOnly {
		t.Fatalf("result = %+v", result)
	}
}

func TestEnsureAccessLeavesIdentityUnsetWithoutLocalKey(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1"}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []map[string]any{{"public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAInomatch other"}}, nil
		case strings.Contains(joined, "droplet get"):
			return sampleDroplet(8, "web", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.10", "", nil), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	result, err := client.EnsureAccess(context.Background(), "do:acct-1:8", EnsureConfig{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if result.HostName != "203.0.113.10" || result.User != "root" || result.IdentityFile != "" || result.IdentitiesOnly {
		t.Fatalf("result = %+v", result)
	}
}

func TestEnsureAccessRejectsPoweredOffDroplet(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1"}, nil
		case strings.Contains(joined, "droplet get"):
			return sampleDroplet(8, "web", "off", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.10", "", nil), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	_, err := client.EnsureAccess(context.Background(), "do:acct-1:8", EnsureConfig{})
	if err == nil || !strings.Contains(err.Error(), "is off") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSyncID(t *testing.T) {
	uuid, id, err := ParseSyncID("do:team-1:3164444")
	if err != nil || uuid != "team-1" || id != "3164444" {
		t.Fatalf("ParseSyncID = %q %q %v", uuid, id, err)
	}
	if _, _, err := ParseSyncID("droplet/1"); err == nil {
		t.Fatal("expected invalid sync id")
	}
}

func TestToSyncHost(t *testing.T) {
	block := ToSyncHost(Instance{
		SyncID: "do:team-1:1", Name: "web", Context: "default", Region: "nyc3",
		HostName: "203.0.113.10", User: "root",
	}, "")
	if block.Alias != "do_default_nyc3_web" || block.SyncSource != ProviderName || block.HostName != "203.0.113.10" {
		t.Fatalf("block = %+v", block)
	}
}

func sampleDroplet(id int, name, status, region, distro, slug, publicV4, privateV4 string, tags []string) map[string]any {
	var v4 []any
	if publicV4 != "" {
		v4 = append(v4, map[string]string{"ip_address": publicV4, "type": "public"})
	}
	if privateV4 != "" {
		v4 = append(v4, map[string]string{"ip_address": privateV4, "type": "private"})
	}
	return map[string]any{
		"id": id, "name": name, "status": status, "size_slug": "s-1vcpu-1gb",
		"region":   map[string]string{"slug": region, "name": region},
		"image":    map[string]string{"distribution": distro, "name": slug, "slug": slug},
		"networks": map[string]any{"v4": v4, "v6": []any{}},
		"tags":     tags,
	}
}

func TestLifecycleCreateStopStartForkDelete(t *testing.T) {
	client := New()
	client.PollInterval = time.Millisecond
	var created, poweredOff, poweredOn, snapshotted, deleted bool
	var forkSnap string
	status := "active"
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1", "team": map[string]string{"uuid": "team-1"}}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []map[string]any{{"id": 9, "name": "laptop", "public_key": "ssh-ed25519 AAAA laptop"}}, nil
		case strings.Contains(joined, "droplet create") && strings.Contains(joined, "web-fork"):
			if !strings.Contains(joined, "--image 77") {
				t.Fatalf("fork create args = %s", joined)
			}
			return sampleDroplet(11, "web-fork", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.20", "", nil), nil
		case strings.Contains(joined, "droplet create"):
			created = true
			if !strings.Contains(joined, "--ssh-keys 9") || !strings.Contains(joined, "--region nyc3") {
				t.Fatalf("create args = %s", joined)
			}
			return sampleDroplet(10, "web", "active", "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.10", "", nil), nil
		case strings.Contains(joined, "droplet-action power-off"):
			poweredOff = true
			status = "off"
			return map[string]any{"status": "completed"}, nil
		case strings.Contains(joined, "droplet-action power-on"):
			poweredOn = true
			status = "active"
			return map[string]any{"status": "completed"}, nil
		case strings.Contains(joined, "droplet-action snapshot"):
			snapshotted = true
			if i := strings.Index(joined, "--snapshot-name "); i >= 0 {
				forkSnap = strings.Fields(joined[i+len("--snapshot-name "):])[0]
			}
			return map[string]any{"status": "completed"}, nil
		case strings.Contains(joined, "droplet snapshots"):
			return []map[string]any{{"id": 77, "name": forkSnap}}, nil
		case strings.Contains(joined, "droplet delete"):
			deleted = true
			return "", nil
		case strings.Contains(joined, "droplet get"):
			return sampleDroplet(10, "web", status, "nyc3", "Ubuntu", "ubuntu-24-04-x64", "203.0.113.10", "", nil), nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})

	id, err := client.New(context.Background(), NewOpts{Name: "web"})
	if err != nil || id != "do:team-1:10" || !created {
		t.Fatalf("New = %q %v created=%v", id, err, created)
	}
	if err := client.Stop(context.Background(), id); err != nil || !poweredOff {
		t.Fatalf("Stop = %v off=%v", err, poweredOff)
	}
	if err := client.Start(context.Background(), id); err != nil || !poweredOn {
		t.Fatalf("Start = %v on=%v", err, poweredOn)
	}
	forkID, err := client.Fork(context.Background(), id)
	if err != nil || forkID != "do:team-1:11" || !snapshotted {
		t.Fatalf("Fork = %q %v snap=%v", forkID, err, snapshotted)
	}
	if err := client.Delete(context.Background(), id); err != nil || !deleted {
		t.Fatalf("Delete = %v deleted=%v", err, deleted)
	}
}

func TestNewRejectsWindowsImage(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1"}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []map[string]any{{"id": 9, "name": "laptop", "public_key": "ssh-ed25519 AAAA laptop"}}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	_, err := client.New(context.Background(), NewOpts{Name: "win", Image: "windows-2022"})
	if err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewRequiresSSHKeys(t *testing.T) {
	client := New()
	client.Run = fakeDOCTL(t, func(joined string) (any, error) {
		switch {
		case joined == "version":
			return "doctl version 1.141.0", nil
		case strings.HasPrefix(joined, "auth list"):
			return []map[string]any{{"name": "default", "current": true}}, nil
		case strings.Contains(joined, "account get"):
			return map[string]any{"uuid": "acct-1"}, nil
		case strings.Contains(joined, "ssh-key list"):
			return []any{}, nil
		default:
			return nil, errors.New("unexpected command: " + joined)
		}
	})
	_, err := client.New(context.Background(), NewOpts{Name: "web"})
	if err == nil || !strings.Contains(err.Error(), "no SSH keys") {
		t.Fatalf("err = %v", err)
	}
}

func fakeDOCTL(t *testing.T, response func(joined string) (any, error)) Runner {
	t.Helper()
	return func(ctx context.Context, args []string, env []string) ([]byte, error) {
		value, err := response(strings.Join(args[1:], " "))
		if err != nil {
			return nil, err
		}
		if text, ok := value.(string); ok {
			return []byte(text), nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data, nil
	}
}
