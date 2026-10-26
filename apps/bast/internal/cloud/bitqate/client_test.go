package bitqate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bast/internal/cloud/sandboxfake"
)

func testClient(t *testing.T, api *sandboxfake.Bitqate) *Client {
	t.Helper()
	keyFile := filepath.Join(t.TempDir(), "bitqate-token")
	if err := WriteTokenFile(keyFile, sandboxfake.BitqateToken); err != nil {
		t.Fatal(err)
	}
	return &Client{
		BaseURL:  api.URL(),
		KeyFile:  keyFile,
		HTTP:     api.Server.Client(),
		PollWait: time.Millisecond,
		Getenv:   func(string) string { return "" },
	}
}

func TestDiscoverMapsVMs(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "running", IPv4: "203.0.113.10", Region: "Falkenstein", OS: "Ubuntu 24.04", Fingerprint: "SHA256:abc"})
	api.Put(sandboxfake.BitqateVM{ID: "vm-2", Name: "db", Status: "stopped", IPv4: "203.0.113.20", Region: "Helsinki"})
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %d", len(discovery.Instances))
	}
	web := discovery.Instances[0]
	if web.SyncID != "bitqate/vm-1" || web.HostName != "203.0.113.10" || !web.Running ||
		web.User != "root" || web.Image != "Ubuntu 24.04" || web.Region != "Falkenstein" ||
		web.HostKeyFingerprint != "SHA256:abc" {
		t.Fatalf("web = %+v", web)
	}
	if GroupPath(web) != "BitQate/Falkenstein" {
		t.Fatalf("group = %q", GroupPath(web))
	}
	if discovery.Instances[1].State != "stopped" || discovery.Instances[1].Running {
		t.Fatalf("db = %+v", discovery.Instances[1])
	}
}

func TestDiscoverSkipsEmptyIPv4(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "noip", Status: "running"})
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 0 || len(discovery.Warnings) == 0 {
		t.Fatalf("instances=%d warnings=%v", len(discovery.Instances), discovery.Warnings)
	}
	if !discovery.Seen["bitqate/vm-1"] {
		t.Fatalf("unaddressed VM must stay seen: %+v", discovery.Seen)
	}
}

func TestDiscoverRejectsInvalidIPv4(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "bad", Status: "running", IPv4: "203.0.113.10\nHost evil"})
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 0 || !discovery.Seen["bitqate/vm-1"] {
		t.Fatalf("instances=%d seen=%v warnings=%v", len(discovery.Instances), discovery.Seen, discovery.Warnings)
	}
	if len(discovery.Warnings) == 0 || !strings.Contains(discovery.Warnings[0], "invalid IPv4") {
		t.Fatalf("warnings=%v", discovery.Warnings)
	}
}

func TestConfiguredSSHPort(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"22", ""},
		{" 22 ", ""},
		{"2222", "2222"},
		{"02222", "2222"},
		{"22abc", ""},
		{"0", ""},
		{"65536", ""},
		{"-1", ""},
		{"443", "443"},
	}
	for _, tc := range cases {
		if got := configuredSSHPort(tc.in); got != tc.want {
			t.Fatalf("configuredSSHPort(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestDiscoverNullOSRegion(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "bare", Status: "running", IPv4: "203.0.113.10"})
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	inst := discovery.Instances[0]
	if inst.Image != "" || inst.Region != "" || GroupPath(inst) != "BitQate" {
		t.Fatalf("inst = %+v", inst)
	}
}

func TestDiscoverSuspendedDistinct(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "sus", Status: "suspended", IPv4: "203.0.113.10"})
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	inst := discovery.Instances[0]
	if inst.State != "suspended" || inst.Running || IsStoppedState(inst.State) {
		t.Fatalf("suspended must not look stopped: %+v", inst)
	}
}

func TestDiscoverMatchesLocalIdentity(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaCtest test@host"
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), []byte(pub+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "running", IPv4: "203.0.113.10", AuthorizedKey: pub})
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	inst := discovery.Instances[0]
	if inst.IdentityFile == "" || !inst.IdentitiesOnly {
		t.Fatalf("identity not matched: %+v", inst)
	}
}

func TestAliasForAndUnique(t *testing.T) {
	a := AliasFor(Instance{Name: "My Web"})
	if a != "bitqate_My_Web" {
		t.Fatalf("alias = %q", a)
	}
	used := map[string]bool{a: true}
	if got := UniqueAlias(a, used); got == a {
		t.Fatalf("collision not uniquified: %q", got)
	}
	if AliasFor(Instance{SyncID: "bitqate/abc123"}) != "bitqate_abc123" {
		t.Fatal("fallback to id failed")
	}
}

func TestDiscoverRegionFilter(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "a", Status: "running", IPv4: "203.0.113.10", Region: "Falkenstein"})
	api.Put(sandboxfake.BitqateVM{ID: "vm-2", Name: "b", Status: "running", IPv4: "203.0.113.20", Region: "Helsinki"})
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{RegionFilter: []string{"helsinki"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 || discovery.Instances[0].Region != "Helsinki" {
		t.Fatalf("filter: %+v", discovery.Instances)
	}
	if !discovery.Seen["bitqate/vm-2"] || discovery.Seen["bitqate/vm-1"] {
		t.Fatalf("seen=%v", discovery.Seen)
	}
}

func TestDiscoverNoPagination(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	for _, vm := range []sandboxfake.BitqateVM{
		{ID: "a", Name: "a", Status: "running", IPv4: "203.0.113.1"},
		{ID: "b", Name: "b", Status: "running", IPv4: "203.0.113.2"},
		{ID: "c", Name: "c", Status: "running", IPv4: "203.0.113.3"},
	} {
		api.Put(vm)
	}
	discovery, err := testClient(t, api).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 3 || !discovery.Complete {
		t.Fatalf("instances = %d", len(discovery.Instances))
	}
}

func TestLifecycle(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "stopped", IPv4: "203.0.113.10"})
	c := testClient(t, api)
	ctx := context.Background()
	if err := c.Start(ctx, "bitqate/vm-1"); err != nil {
		t.Fatal(err)
	}
	if api.Get("vm-1").Status != "running" {
		t.Fatal("not running")
	}
	if err := c.Stop(ctx, "bitqate/vm-1"); err != nil {
		t.Fatal(err)
	}
	if api.Get("vm-1").Status != "stopped" {
		t.Fatal("not stopped")
	}
	if err := c.Start(ctx, "vm-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Restart(ctx, "bitqate/vm-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Restart(ctx, "bitqate/vm-1"); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendedBlocksPower(t *testing.T) {
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "sus", Status: "suspended", IPv4: "203.0.113.10"})
	c := testClient(t, api)
	if err := c.Start(context.Background(), "vm-1"); err == nil || !strings.Contains(err.Error(), "suspended") {
		t.Fatalf("start err = %v", err)
	}
}

func TestErrorTranslations(t *testing.T) {
	ctx := context.Background()
	api := sandboxfake.NewBitqate(t)

	keyFile := filepath.Join(t.TempDir(), "bitqate-token")
	if err := WriteTokenFile(keyFile, "wrong"); err != nil {
		t.Fatal(err)
	}
	bad := &Client{BaseURL: api.URL(), KeyFile: keyFile, HTTP: api.Server.Client(), Getenv: func(string) string { return "" }}
	if _, err := bad.Discover(ctx, DiscoverConfig{}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("401 err = %v", err)
	}

	c := testClient(t, api)
	if _, err := c.Discover(ctx, DiscoverConfig{}); err != nil {
		t.Fatal(err)
	}
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "stopped", IPv4: "203.0.113.10", ReadOnlyTokens: nil})
	if _, err := c.getVM(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("404 err = %v", err)
	}

	ro := filepath.Join(t.TempDir(), "bitqate-token")
	if err := WriteTokenFile(ro, "read-only-token"); err != nil {
		t.Fatal(err)
	}
	api.ReadOnlyTokens = map[string]bool{"read-only-token": true}
	roc := &Client{BaseURL: api.URL(), KeyFile: ro, HTTP: api.Server.Client(), Getenv: func(string) string { return "" }}
	api.Put(sandboxfake.BitqateVM{ID: "vm-2", Name: "x", Status: "stopped", IPv4: "203.0.113.20"})
	if err := roc.vmAction(ctx, "vm-2", "start"); err == nil || !strings.Contains(err.Error(), "vms:power") {
		t.Fatalf("403 err = %v", err)
	}

	po := filepath.Join(t.TempDir(), "bitqate-token")
	if err := WriteTokenFile(po, "power-only-token"); err != nil {
		t.Fatal(err)
	}
	api.PowerOnlyTokens = map[string]bool{"power-only-token": true}
	poc := &Client{BaseURL: api.URL(), KeyFile: po, HTTP: api.Server.Client(), Getenv: func(string) string { return "" }}
	err := poc.Start(ctx, "vm-2")
	if err == nil || !strings.Contains(err.Error(), "vms:read") || strings.Contains(err.Error(), "vms:power") {
		t.Fatalf("power-only 403 err = %v", err)
	}

	api.Conflict = true
	if err := c.vmAction(ctx, "vm-1", "start"); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("409 err = %v", err)
	}
	api.Conflict = false
	api.RateHit = true
	if err := c.vmAction(ctx, "vm-1", "start"); err != nil {
		t.Fatalf("429 retry should recover, err = %v", err)
	}
	if api.Get("vm-1").Status != "running" {
		t.Fatal("retry did not start VM")
	}
}
