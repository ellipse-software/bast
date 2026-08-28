package sshconfig

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAWSProxyConfigResolvesWithOpenSSH(t *testing.T) {
	dir := t.TempDir()
	syncPath := filepath.Join(dir, "aws", "config")
	if err := WriteSyncConfig(syncPath, []SyncHostInput{{
		Alias: "aws_default_eu-west-1_web", SyncSource: "aws",
		SyncID:   "arn:aws:ec2:eu-west-1:123456789012:instance/i-123",
		HostName: "i-123", User: "ubuntu", IdentityFile: "~/.ssh/bast/aws_compute", IdentitiesOnly: true,
		ProxyCommand: "aws ec2-instance-connect open-tunnel --instance-id=i-123 --instance-connect-endpoint-id=eice-123 --remote-port=%p --profile=default --region=eu-west-1 --no-cli-pager",
	}}); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "config")
	if err := os.WriteFile(mainPath, []byte("Include "+syncPath+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ssh", "-F", mainPath, "-G", "aws_default_eu-west-1_web").Output()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "hostname i-123") || !strings.Contains(text, "proxycommand aws ec2-instance-connect open-tunnel") || !strings.Contains(text, "--remote-port=%p") {
		t.Fatalf("ssh -G output:\n%s", text)
	}
}

func TestWriteAndDiscoverSyncBlocks(t *testing.T) {
	m := testManager(t)
	m.SyncGCPConfig = filepath.Join(m.ManagedDir, "sync", "gcp", "config")
	if err := m.EnsureManaged(); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureSyncInclude(m.SyncGCPConfig); err != nil {
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

func TestRewriteManagedHostsKeepsSyncIncludes(t *testing.T) {
	m := testManager(t)
	m.SyncGCPConfig = filepath.Join(m.ManagedDir, "sync", "gcp", "config")
	m.SyncBoxConfig = filepath.Join(m.ManagedDir, "sync", "box", "config")
	if err := m.EnsureManaged(); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureSyncInclude(m.SyncGCPConfig); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureSyncInclude(m.SyncBoxConfig); err != nil {
		t.Fatal(err)
	}
	if err := WriteSyncConfig(m.SyncGCPConfig, []SyncHostInput{{
		Alias: "gcp_proj_web", SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/web",
		HostName: "web", User: "ubuntu",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteSyncConfig(m.SyncBoxConfig, []SyncHostInput{{
		Alias: "box_dev", SyncSource: "box", SyncID: "bx_dev0001",
		HostName: "203.0.113.10", User: "user",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := m.RewriteManagedHosts(RenderManagedBlock("abc123", HostInput{
		Alias: "prod", HostName: "prod.example", User: "deploy",
	})); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "sync/gcp/config") || !strings.Contains(text, "sync/box/config") {
		t.Fatalf("vault rewrite dropped sync includes:\n%s", text)
	}
	if !strings.Contains(text, "Host prod") {
		t.Fatalf("managed host missing:\n%s", text)
	}
	hosts, err := m.Discover()
	if err != nil {
		t.Fatal(err)
	}
	byAlias := map[string]Host{}
	for _, host := range hosts {
		byAlias[host.Alias] = host
	}
	if !byAlias["gcp_proj_web"].Synced || !byAlias["box_dev"].Synced || byAlias["prod"].Alias == "" {
		t.Fatalf("discover after rewrite: %+v", byAlias)
	}
}

func TestRestoreSyncIncludesAfterWipedManagedConfig(t *testing.T) {
	m := testManager(t)
	m.SyncBoxConfig = filepath.Join(m.ManagedDir, "sync", "box", "config")
	if err := m.EnsureManaged(); err != nil {
		t.Fatal(err)
	}
	if err := WriteSyncConfig(m.SyncBoxConfig, []SyncHostInput{{
		Alias: "box_dev", SyncSource: "box", SyncID: "bx_dev0001",
		HostName: "203.0.113.10", User: "user",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := WriteManagedConfig(m.ManagedConfig, RenderManagedBlock("abc123", HostInput{
		Alias: "prod", HostName: "prod.example",
	})); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreSyncIncludes(); err != nil {
		t.Fatal(err)
	}
	hosts, err := m.Discover()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, host := range hosts {
		if host.Alias == "box_dev" && host.Synced {
			found = true
		}
	}
	if !found {
		t.Fatalf("restored include should expose synced box: %+v", hosts)
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
	if err := m.EnsureSyncInclude(m.SyncGCPConfig); err != nil {
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
	if err := UpdateSyncHostAuth(path, "gcp_p_web", "debian", "~/.ssh/google_compute_engine", "~/.ssh/bast/azure/cert.pub", true); err != nil {
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
	if !strings.Contains(text, "CertificateFile ~/.ssh/bast/azure/cert.pub") {
		t.Fatalf("certificate missing:\n%s", text)
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
	if err := UpdateSyncHostAuth(path, "gcp_p_web", "debian", "~/.ssh/id_ed25519", "", true); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text = string(raw)
	if strings.Contains(text, "CertificateFile") || !strings.Contains(text, "IdentityFile ~/.ssh/id_ed25519") {
		t.Fatalf("stale certificate was not cleared:\n%s", text)
	}
}

func TestUpdateSyncHostDetailsPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := WriteSyncConfig(path, []SyncHostInput{{
		Alias: "hetzner_vpn", SyncSource: "hetzner", SyncID: "hetzner/1",
		HostName: "168.119.235.95", User: "root",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateSyncHostDetails(path, "hetzner_vpn", "168.119.235.95", "ted", "~/.ssh/bast/keys/ted.ac", "", "2022", true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "User ted") || !strings.Contains(text, "Port 2022") || !strings.Contains(text, "IdentityFile ~/.ssh/bast/keys/ted.ac") {
		t.Fatalf("details missing:\n%s", text)
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

func TestRenderSyncBlockPasswordOnly(t *testing.T) {
	block := string(RenderSyncBlock(SyncHostInput{
		Alias: "upstash_dev", SyncSource: "upstash", SyncID: "current-wasp-05510",
		HostName: "us-east-1.box.upstash.com", User: "current-wasp-05510",
		PasswordOnly: true, ExtraOptions: []string{"StrictHostKeyChecking accept-new"},
	}))
	for _, want := range []string{
		"PubkeyAuthentication no",
		"PasswordAuthentication yes",
		"PreferredAuthentications keyboard-interactive,password",
		"StrictHostKeyChecking accept-new",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("missing %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "IdentityFile") {
		t.Fatalf("password-only block should omit IdentityFile:\n%s", block)
	}
}

func TestRemoveSyncInclude(t *testing.T) {
	m := testManager(t)
	m.SyncGCPConfig = filepath.Join(m.ManagedDir, "sync", "gcp", "config")
	if err := m.EnsureSyncInclude(m.SyncGCPConfig); err != nil {
		t.Fatal(err)
	}
	if err := WriteSyncConfig(m.SyncGCPConfig, []SyncHostInput{{
		Alias: "gcp_p_x", SyncSource: "gcp", SyncID: "projects/p/zones/z/instances/x", HostName: "x",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveSyncInclude(m.SyncGCPConfig); err != nil {
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
