package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndDiscoverSyncBlocks(t *testing.T) {
	m := testManager(t)
	m.SyncGCPConfig = filepath.Join(m.ManagedDir, "sync", "gcp", "config")
	if err := m.EnsureManaged(); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureSyncInclude(); err != nil {
		t.Fatal(err)
	}
	blocks := []SyncHostInput{
		{
			Alias: "gcp_proj_web", SyncSource: "gcp",
			SyncID:   "projects/proj/zones/us-central1-a/instances/web",
			HostName: "web", User: "ubuntu",
			ProxyCommand: "gcloud compute start-iap-tunnel web %p --listen-on-stdin --project=proj --zone=us-central1-a --verbosity=warning",
		},
		{
			Alias: "gcp_proj_api", SyncSource: "gcp",
			SyncID:   "projects/proj/zones/us-central1-a/instances/api",
			HostName: "1.2.3.4", User: "ubuntu",
		},
	}
	if err := WriteSyncConfig(m.SyncGCPConfig, blocks); err != nil {
		t.Fatal(err)
	}
	hosts, err := m.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts = %d", len(hosts))
	}
	byAlias := map[string]Host{}
	for _, host := range hosts {
		byAlias[host.Alias] = host
	}
	web := byAlias["gcp_proj_web"]
	if !web.Synced || web.SyncSource != "gcp" || web.SyncID == "" || web.Managed {
		t.Fatalf("web host = %+v", web)
	}
	api := byAlias["gcp_proj_api"]
	if !api.Synced || api.SyncSource != "gcp" {
		t.Fatalf("api host = %+v", api)
	}
	data, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Include ~/.ssh/bast/sync/gcp/config") && !strings.Contains(string(data), m.SyncGCPConfig) {
		t.Fatalf("managed config missing sync include: %s", data)
	}
}

func TestEnsureSyncIncludeIsBeforeHostBlocks(t *testing.T) {
	m := testManager(t)
	m.SyncGCPConfig = filepath.Join(m.ManagedDir, "sync", "gcp", "config")
	if err := m.EnsureManaged(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(HostInput{Alias: "existing", HostName: "existing.example"}); err != nil {
		t.Fatal(err)
	}
	// Simulate the old buggy append: Include after Host blocks.
	b, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, []byte("Include "+m.SyncGCPConfig+"\nInclude "+m.SyncGCPConfig+"\n")...)
	if err := os.WriteFile(m.ManagedConfig, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureSyncInclude(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	includeAt := strings.Index(text, "Include ")
	hostAt := strings.Index(text, "\nHost ")
	if includeAt < 0 || hostAt < 0 || includeAt > hostAt {
		t.Fatalf("Include must come before Host blocks:\n%s", text)
	}
	if strings.Count(text, "sync/gcp/config") != 1 {
		t.Fatalf("expected a single sync include, got:\n%s", text)
	}
	if err := WriteSyncConfig(m.SyncGCPConfig, []SyncHostInput{{
		Alias: "gcp_p_web", SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/web", HostName: "1.2.3.4",
	}}); err != nil {
		t.Fatal(err)
	}
	hosts, err := m.Discover()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, host := range hosts {
		if host.Alias == "gcp_p_web" && host.Synced {
			found = true
		}
	}
	if !found {
		t.Fatalf("synced host not discovered: %+v", hosts)
	}
}

func TestUpdateSyncHostAuth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := WriteSyncConfig(path, []SyncHostInput{{
		Alias: "gcp_p_web", SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/web",
		HostName:     "1.2.3.4",
		ProxyCommand: "gcloud compute start-iap-tunnel web %p --listen-on-stdin --project=p --zone=z --verbosity=warning",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSyncHostAuth(path, "gcp_p_web", "debian", "~/.ssh/google_compute_engine", true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "User debian") || !strings.Contains(text, "IdentityFile ~/.ssh/google_compute_engine") {
		t.Fatalf("auth missing:\n%s", text)
	}
	if !strings.Contains(text, "IdentitiesOnly yes") || !strings.Contains(text, "ProxyCommand gcloud") {
		t.Fatalf("expected IdentitiesOnly + ProxyCommand preserved:\n%s", text)
	}
	if !strings.Contains(text, "# bast:sync:gcp=projects/p/zones/z/instances/web") {
		t.Fatalf("sync marker lost:\n%s", text)
	}
	if strings.Count(text, syncMarkerEnd) != 1 {
		t.Fatalf("duplicate sync end markers:\n%s", text)
	}
}

func TestRenderSyncBlockIdentitiesOnlyOnlyWhenConfident(t *testing.T) {
	soft := string(RenderSyncBlock(SyncHostInput{
		Alias: "gcp_p_web", SyncSource: "gcp", SyncID: "id",
		HostName: "1.2.3.4", User: "ubuntu",
		IdentityFile: "~/.ssh/google_compute_engine", IdentitiesOnly: false,
	}))
	if strings.Contains(soft, "IdentitiesOnly") {
		t.Fatalf("soft identity should omit IdentitiesOnly:\n%s", soft)
	}
	hard := string(RenderSyncBlock(SyncHostInput{
		Alias: "gcp_p_web", SyncSource: "gcp", SyncID: "id",
		HostName: "1.2.3.4", User: "ubuntu",
		IdentityFile: "~/.ssh/bast/keys/IRIS", IdentitiesOnly: true,
	}))
	if !strings.Contains(hard, "IdentitiesOnly yes") {
		t.Fatalf("confident identity should set IdentitiesOnly:\n%s", hard)
	}
}

func TestRemoveSyncInclude(t *testing.T) {
	m := testManager(t)
	m.SyncGCPConfig = filepath.Join(m.ManagedDir, "sync", "gcp", "config")
	if err := m.EnsureSyncInclude(); err != nil {
		t.Fatal(err)
	}
	if err := WriteSyncConfig(m.SyncGCPConfig, []SyncHostInput{{
		Alias: "gcp_p_x", SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/x", HostName: "x",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveSyncInclude(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.SyncGCPConfig); !os.IsNotExist(err) {
		t.Fatalf("expected sync config removed, err=%v", err)
	}
	hosts, err := m.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 0 {
		t.Fatalf("hosts after remove = %+v", hosts)
	}
}
