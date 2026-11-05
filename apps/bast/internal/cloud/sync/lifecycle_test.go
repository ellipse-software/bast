package sync

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"bast/internal/cloud"
	bitqatecloud "bast/internal/cloud/bitqate"
	boatcloud "bast/internal/cloud/boat"
	boxdcloud "bast/internal/cloud/boxd"
	hetznercloud "bast/internal/cloud/hetzner"
	"bast/internal/cloud/sandboxfake"
	upstashcloud "bast/internal/cloud/upstash"
	vercelcloud "bast/internal/cloud/vercel"
	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

func testEngine(t *testing.T) (*Engine, paths.Paths, *metadata.Store) {
	t.Helper()
	home := t.TempDir()
	p := paths.ForHome(home)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	return New(p, store), p, store
}

func loadProviderConfig(t *testing.T, path string) []sshconfig.SyncHostInput {
	t.Helper()
	blocks, err := sshconfig.LoadSyncHosts(path)
	if err != nil {
		t.Fatal(err)
	}
	return blocks
}

func hostBySyncID(blocks []sshconfig.SyncHostInput, syncID string) sshconfig.SyncHostInput {
	for _, block := range blocks {
		if block.SyncID == syncID {
			return block
		}
	}
	return sshconfig.SyncHostInput{}
}

func TestVercelEngineLifecycle(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewVercel(t)
	engine.Vercel.BaseURL = api.URL()
	engine.Vercel.Token = sandboxfake.VercelToken
	engine.Vercel.TeamID = sandboxfake.VercelTeam
	engine.Vercel.ProjectID = sandboxfake.VercelProject
	engine.Vercel.HTTP = api.Server.Client()
	engine.Vercel.PollWait = time.Millisecond
	if err := store.SetVercel(metadata.VercelIntegration{TeamID: sandboxfake.VercelTeam, ProjectID: sandboxfake.VercelProject}); err != nil {
		t.Fatal(err)
	}

	caps := cloud.CapabilitiesFor(cloud.Vercel)
	exercised := cloud.Capabilities{}
	ctx := context.Background()

	if !caps.Create {
		t.Fatal("vercel must advertise Create")
	}
	result, alias, err := engine.NewVercel(ctx, vercelcloud.CreateOpts{Name: "dev", VCPUs: 2, Timeout: time.Hour, Persistent: true})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Create = true
	if alias != "vercel_dev" || result.Count != 1 || result.Provider != vercelcloud.ProviderName {
		t.Fatalf("create result=%+v alias=%q", result, alias)
	}
	if api.ListCalls == 0 {
		t.Fatal("discover did not list sandboxes")
	}
	syncID := vercelcloud.SyncID(sandboxfake.VercelProject, "dev")
	blocks := loadProviderConfig(t, p.SyncVercelConfig)
	block := hostBySyncID(blocks, syncID)
	if block.Alias != "vercel_dev" || block.SyncSource != "vercel" || block.HostName != vercelcloud.StoppedHost {
		t.Fatalf("ssh block = %+v", block)
	}
	meta := store.Host("vercel_dev")
	if meta.Group != "Vercel" || meta.Label != "dev" || !strings.Contains(strings.Join(meta.Tags, ","), "state:running") {
		t.Fatalf("metadata = %+v", meta)
	}

	if !caps.Stop {
		t.Fatal("vercel must advertise Stop")
	}
	result, err = engine.StopVercel(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Stop = true
	if result.Count != 1 {
		t.Fatalf("stop count = %d", result.Count)
	}
	meta = store.Host("vercel_dev")
	if !vercelcloud.HostLooksStopped(meta.Tags) {
		t.Fatalf("stopped tags = %v", meta.Tags)
	}
	if hostBySyncID(loadProviderConfig(t, p.SyncVercelConfig), syncID).Alias == "" {
		t.Fatal("stopped sandbox disappeared from SSH config")
	}

	if !caps.Start {
		t.Fatal("vercel must advertise Start")
	}
	result, err = engine.ResumeVercel(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Start = true
	meta = store.Host("vercel_dev")
	if vercelcloud.HostLooksStopped(meta.Tags) || !strings.Contains(strings.Join(meta.Tags, ","), "state:running") {
		t.Fatalf("resumed tags = %v", meta.Tags)
	}

	if !caps.Fork {
		t.Fatal("vercel must advertise Fork")
	}
	result, forkAlias, err := engine.ForkVercel(ctx, syncID, "dev-fork")
	if err != nil {
		t.Fatal(err)
	}
	exercised.Fork = true
	if forkAlias != "vercel_dev-fork" || result.Count != 2 {
		t.Fatalf("fork result=%+v alias=%q", result, forkAlias)
	}
	forkID := vercelcloud.SyncID(sandboxfake.VercelProject, "dev-fork")
	if hostBySyncID(loadProviderConfig(t, p.SyncVercelConfig), forkID).Alias != "vercel_dev-fork" {
		t.Fatal("fork missing from SSH config")
	}

	if !caps.Delete {
		t.Fatal("vercel must advertise Delete")
	}
	result, err = engine.DeleteVercel(ctx, forkID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Delete = true
	if result.Count != 1 {
		t.Fatalf("delete count = %d", result.Count)
	}
	if hostBySyncID(loadProviderConfig(t, p.SyncVercelConfig), forkID).Alias != "" {
		t.Fatal("deleted fork still in SSH config")
	}
	if _, ok := store.Hosts()["vercel_dev-fork"]; ok {
		t.Fatal("deleted fork still in metadata")
	}

	api.Put(sandboxfake.VercelSandbox{Name: "dead", Status: "failed", Persistent: false})
	deleted, err := engine.ListVercelUnrestorable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deleted, ",") != "prj_1/dead" {
		t.Fatalf("unrestorable = %v", deleted)
	}
	result, cleaned, err := engine.CleanupVercel(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cleaned, ",") != "dead" || result.Count != 1 {
		t.Fatalf("cleanup result=%+v deleted=%v", result, cleaned)
	}
	if api.Get("dead") != nil {
		t.Fatal("cleanup left unrestorable sandbox")
	}

	if exercised != caps {
		t.Fatalf("exercised %+v, advertised %+v", exercised, caps)
	}
}

func TestUpstashEngineLifecycle(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewUpstash(t)
	if err := upstashcloud.WriteKeyFile(p.UpstashAPIKey, sandboxfake.UpstashKey); err != nil {
		t.Fatal(err)
	}
	engine.Upstash.BaseURL = api.URL()
	engine.Upstash.HTTP = api.Server.Client()
	engine.Upstash.PollWait = time.Millisecond

	caps := cloud.CapabilitiesFor(cloud.Upstash)
	exercised := cloud.Capabilities{}
	ctx := context.Background()

	result, alias, err := engine.NewUpstash(ctx, upstashcloud.CreateOpts{Name: "dev", Runtime: "node", Size: "small"})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Create = true
	if alias == "" || result.Count != 1 || result.Provider != upstashcloud.ProviderName {
		t.Fatalf("create result=%+v alias=%q", result, alias)
	}
	if api.ListCalls != 1 {
		t.Fatalf("discover listed %d times, want 1", api.ListCalls)
	}
	ids := api.IDs()
	if len(ids) != 1 {
		t.Fatalf("ids = %v", ids)
	}
	syncID := ids[0]
	blocks := loadProviderConfig(t, p.SyncUpstashConfig)
	block := hostBySyncID(blocks, syncID)
	if block.Alias != alias || block.SyncSource != "upstash" || !block.PasswordOnly {
		t.Fatalf("ssh block = %+v", block)
	}
	meta := store.Host(alias)
	if meta.Group != "Upstash" || upstashcloud.HostLooksStopped(meta.Tags) {
		t.Fatalf("metadata = %+v", meta)
	}

	result, err = engine.StopUpstash(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Stop = true
	meta = store.Host(alias)
	if !upstashcloud.HostLooksStopped(meta.Tags) {
		t.Fatalf("paused tags = %v", meta.Tags)
	}

	result, err = engine.ResumeUpstash(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Start = true
	if upstashcloud.HostLooksStopped(store.Host(alias).Tags) {
		t.Fatal("resume left host stopped")
	}

	result, forkAlias, err := engine.ForkUpstash(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Fork = true
	if forkAlias == "" || result.Count != 2 {
		t.Fatalf("fork result=%+v alias=%q", result, forkAlias)
	}
	var forkID string
	for _, id := range api.IDs() {
		if id != syncID {
			forkID = id
		}
	}
	if forkID == "" {
		t.Fatal("fork id missing from fake inventory")
	}

	result, err = engine.DeleteUpstash(ctx, forkID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Delete = true
	if result.Count != 1 {
		t.Fatalf("delete count = %d", result.Count)
	}
	if hostBySyncID(loadProviderConfig(t, p.SyncUpstashConfig), forkID).Alias != "" {
		t.Fatal("deleted fork still in SSH config")
	}

	if exercised != caps {
		t.Fatalf("exercised %+v, advertised %+v", exercised, caps)
	}
}

func TestBitqateEngineLifecycle(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "running", IPv4: "203.0.113.10", Region: "Falkenstein", Fingerprint: "SHA256:abc"})
	if err := bitqatecloud.WriteTokenFile(p.BitqateToken, sandboxfake.BitqateToken); err != nil {
		t.Fatal(err)
	}
	engine.Bitqate.BaseURL = api.URL()
	engine.Bitqate.HTTP = api.Server.Client()
	engine.Bitqate.PollWait = time.Millisecond
	engine.Bitqate.Getenv = func(string) string { return "" }
	ctx := context.Background()

	caps := cloud.CapabilitiesFor(cloud.Bitqate)
	exercised := cloud.Capabilities{}
	if caps.Create || caps.Fork || caps.Delete {
		t.Fatalf("bitqate should not advertise create/fork/delete: %+v", caps)
	}
	result, err := engine.SyncBitqate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 {
		t.Fatalf("count = %d", result.Count)
	}
	syncID := bitqatecloud.FormatSyncID("vm-1")
	block := hostBySyncID(loadProviderConfig(t, p.SyncBitqateConfig), syncID)
	if block.Alias == "" {
		t.Fatal("no synced block")
	}
	alias := block.Alias
	if !strings.Contains(strings.Join(store.Host(alias).Tags, " "), "hostkey:SHA256:abc") {
		t.Fatalf("fingerprint tag missing: %+v", store.Host(alias).Tags)
	}
	if bitqatecloud.HostLooksStopped(store.Host(alias).Tags) {
		t.Fatal("running VM looks stopped")
	}
	if _, err = engine.StopBitqate(ctx, syncID); err != nil {
		t.Fatal(err)
	}
	exercised.Stop = true
	if api.Get("vm-1").Status != "stopped" {
		t.Fatal("vm not stopped")
	}
	if !bitqatecloud.HostLooksStopped(store.Host(alias).Tags) {
		t.Fatal("stopped VM does not look stopped")
	}
	if _, err = engine.StartBitqate(ctx, syncID); err != nil {
		t.Fatal(err)
	}
	exercised.Start = true
	if api.Get("vm-1").Status != "running" || bitqatecloud.HostLooksStopped(store.Host(alias).Tags) {
		t.Fatal("vm not running")
	}
	if _, err = engine.RestartBitqate(ctx, syncID); err != nil {
		t.Fatal(err)
	}
	exercised.Restart = true
	if api.Get("vm-1").Status != "running" {
		t.Fatal("vm not running after restart")
	}
	if exercised != caps {
		t.Fatalf("exercised %+v, advertised %+v", exercised, caps)
	}
}

func TestBitqateSyncRemovesDeletedVMs(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "running", IPv4: "203.0.113.10", Region: "Falkenstein"})
	api.Put(sandboxfake.BitqateVM{ID: "vm-2", Name: "db", Status: "running", IPv4: "203.0.113.20", Region: "Helsinki"})
	if err := bitqatecloud.WriteTokenFile(p.BitqateToken, sandboxfake.BitqateToken); err != nil {
		t.Fatal(err)
	}
	engine.Bitqate.BaseURL = api.URL()
	engine.Bitqate.HTTP = api.Server.Client()
	engine.Bitqate.PollWait = time.Millisecond
	engine.Bitqate.Getenv = func(string) string { return "" }
	ctx := context.Background()
	if _, err := engine.SyncBitqate(ctx); err != nil {
		t.Fatal(err)
	}
	kept := hostBySyncID(loadProviderConfig(t, p.SyncBitqateConfig), bitqatecloud.FormatSyncID("vm-1"))
	gone := hostBySyncID(loadProviderConfig(t, p.SyncBitqateConfig), bitqatecloud.FormatSyncID("vm-2"))
	if kept.Alias == "" || gone.Alias == "" {
		t.Fatal("expected both VMs in the first sync")
	}
	api.Delete("vm-2")
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "running", Region: "Falkenstein"})
	result, err := engine.SyncBitqate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 {
		t.Fatalf("count = %d", result.Count)
	}
	blocks := loadProviderConfig(t, p.SyncBitqateConfig)
	if hostBySyncID(blocks, bitqatecloud.FormatSyncID("vm-2")).Alias != "" {
		t.Fatal("deleted VM stayed in SSH config")
	}
	if _, ok := store.Hosts()[gone.Alias]; ok {
		t.Fatal("deleted VM stayed in metadata")
	}
	retained := hostBySyncID(blocks, bitqatecloud.FormatSyncID("vm-1"))
	if retained.Alias == "" || retained.HostName != "203.0.113.10" {
		t.Fatalf("VM with no current address was dropped: %+v", retained)
	}
}

func TestDeleteBitqateTokenWithoutRemainingCredential(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewBitqate(t)
	api.Put(sandboxfake.BitqateVM{ID: "vm-1", Name: "web", Status: "running", IPv4: "203.0.113.10"})
	if err := bitqatecloud.WriteTokenFile(p.BitqateToken, sandboxfake.BitqateToken); err != nil {
		t.Fatal(err)
	}
	engine.Bitqate.BaseURL = api.URL()
	engine.Bitqate.HTTP = api.Server.Client()
	engine.Bitqate.PollWait = time.Millisecond
	engine.Bitqate.Getenv = func(string) string { return "" }
	ctx := context.Background()
	if _, err := engine.SyncBitqate(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := engine.DeleteBitqateToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != bitqatecloud.ProviderName || result.Count != 0 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(p.BitqateToken); !os.IsNotExist(err) {
		t.Fatalf("token file err = %v", err)
	}
	if strings.Contains(store.Bitqate().LastSyncError, "no API token") {
		t.Fatalf("clearing the token recorded a sync error: %q", store.Bitqate().LastSyncError)
	}
	if hostBySyncID(loadProviderConfig(t, p.SyncBitqateConfig), bitqatecloud.FormatSyncID("vm-1")).Alias == "" {
		t.Fatal("clearing the token removed synced hosts")
	}
}

func TestHetznerEngineLifecycle(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewHetzner(t)
	api.Put(sandboxfake.HetznerServer{ID: 42, Name: "web", Status: "running", IPv4: "203.0.113.10", Location: "fsn1"})
	if err := hetznercloud.WriteKeyFile(p.HetznerAPIKey, sandboxfake.HetznerToken); err != nil {
		t.Fatal(err)
	}
	engine.Hetzner.BaseURL = api.URL()
	engine.Hetzner.HTTP = api.Server.Client()
	engine.Hetzner.PollWait = time.Millisecond
	engine.Hetzner.Getenv = func(string) string { return "" }

	caps := cloud.CapabilitiesFor(cloud.Hetzner)
	exercised := cloud.Capabilities{}
	ctx := context.Background()

	result, err := engine.SyncHetzner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 {
		t.Fatalf("sync count = %d", result.Count)
	}
	syncID := hetznercloud.FormatSyncID(42)
	block := hostBySyncID(loadProviderConfig(t, p.SyncHetznerConfig), syncID)
	if block.Alias == "" || block.HostName != "203.0.113.10" {
		t.Fatalf("ssh block = %+v", block)
	}
	alias := block.Alias
	if hetznercloud.HostLooksStopped(store.Host(alias).Tags) {
		t.Fatalf("running tags = %v", store.Host(alias).Tags)
	}

	result, err = engine.StopHetzner(ctx, syncID, false)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Stop = true
	if api.Get(42).Status != "off" {
		t.Fatalf("status after stop = %s", api.Get(42).Status)
	}
	if !hetznercloud.HostLooksStopped(store.Host(alias).Tags) {
		t.Fatalf("stopped tags = %v", store.Host(alias).Tags)
	}
	if hostBySyncID(loadProviderConfig(t, p.SyncHetznerConfig), syncID).Alias == "" {
		t.Fatal("stopped server disappeared from SSH config")
	}

	result, err = engine.StartHetzner(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Start = true
	if api.Get(42).Status != "running" || hetznercloud.HostLooksStopped(store.Host(alias).Tags) {
		t.Fatalf("start status=%s tags=%v", api.Get(42).Status, store.Host(alias).Tags)
	}

	result, err = engine.RestartHetzner(ctx, syncID, false)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Restart = true
	if api.Get(42).Status != "running" {
		t.Fatalf("restart status = %s", api.Get(42).Status)
	}

	if caps.Create || caps.Fork || caps.Delete {
		t.Fatalf("hetzner should not advertise create/fork/delete: %+v", caps)
	}
	if exercised != caps {
		t.Fatalf("exercised %+v, advertised %+v", exercised, caps)
	}
}

func TestBoatEngineLifecycle(t *testing.T) {
	engine, p, store := testEngine(t)
	fake := sandboxfake.NewBoat()
	engine.Boat.Run = fake.Runner()
	engine.Boat.PollInterval = time.Millisecond

	caps := cloud.CapabilitiesFor(cloud.Boat)
	exercised := cloud.Capabilities{}
	ctx := context.Background()

	result, alias, err := engine.NewBoat(ctx, boatcloud.NewOpts{})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Create = true
	if alias == "" || result.Count != 1 || result.Provider != boatcloud.ProviderName {
		t.Fatalf("create result=%+v alias=%q", result, alias)
	}
	var syncID string
	for id := range fake.Sandboxes {
		syncID = id
	}
	block := hostBySyncID(loadProviderConfig(t, p.SyncBoatConfig), syncID)
	if block.Alias != alias || block.SyncSource != "boat" {
		t.Fatalf("ssh block = %+v", block)
	}
	if boatcloud.HostLooksStopped(block.HostName, store.Host(alias).Tags) {
		t.Fatal("new sandbox looks stopped")
	}

	result, err = engine.StopBoat(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Stop = true
	meta := store.Host(alias)
	stoppedHost := hostBySyncID(loadProviderConfig(t, p.SyncBoatConfig), syncID)
	if !boatcloud.HostLooksStopped(stoppedHost.HostName, meta.Tags) {
		t.Fatalf("stopped host=%+v tags=%v", stoppedHost, meta.Tags)
	}

	result, err = engine.ResumeBoat(ctx, syncID, boatcloud.ResumeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Start = true
	if boatcloud.HostLooksStopped(hostBySyncID(loadProviderConfig(t, p.SyncBoatConfig), syncID).HostName, store.Host(alias).Tags) {
		t.Fatal("resume left boat stopped")
	}

	result, err = engine.StopBoat(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	result, forkAlias, err := engine.ForkBoat(ctx, syncID, boatcloud.ForkOpts{})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Fork = true
	if forkAlias == "" || result.Count != 2 {
		t.Fatalf("fork result=%+v alias=%q", result, forkAlias)
	}

	if !caps.Delete {
		t.Fatal("boat must advertise Delete")
	}
	var forkID string
	for id := range fake.Sandboxes {
		if id != syncID {
			forkID = id
			break
		}
	}
	result, err = engine.DeleteBoat(ctx, forkID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Delete = true
	if fake.Get(forkID) != nil {
		t.Fatal("deleted fork still in fake")
	}
	if hostBySyncID(loadProviderConfig(t, p.SyncBoatConfig), forkID).Alias != "" {
		t.Fatal("deleted fork still in SSH config")
	}

	if exercised != (cloud.Capabilities{Create: true, Stop: true, Start: true, Fork: true, Delete: true}) {
		t.Fatalf("exercised %+v", exercised)
	}
	if exercised != caps {
		t.Fatalf("exercised %+v, advertised %+v", exercised, caps)
	}
}

func boxdTestEngine(t *testing.T) (*Engine, paths.Paths, *metadata.Store, *sandboxfake.Boxd) {
	t.Helper()
	engine, p, store := testEngine(t)
	fake := sandboxfake.NewBoxd()
	engine.Boxd.Run = fake.Runner()
	resolve := fake.ResolveSSH()
	engine.Boxd.ResolveSSH = func(ctx context.Context, host string) (boxdcloud.SSHDetails, error) {
		port, user, identity, ok := resolve(ctx, host)
		return boxdcloud.SSHDetails{Port: port, User: user, IdentityFile: identity, IdentitiesOnly: ok}, nil
	}
	engine.Boxd.Home = "/home/test"
	engine.Boxd.PollInterval = time.Millisecond
	return engine, p, store, fake
}

func TestBoxdEngineLifecycle(t *testing.T) {
	engine, p, store, fake := boxdTestEngine(t)
	fake.Put(sandboxfake.BoxdComputer{Region: "fra", Name: "web", Status: "running", Port: "10022"})
	ctx := context.Background()
	caps := cloud.CapabilitiesFor(cloud.Boxd)
	exercised := cloud.Capabilities{}

	result, err := engine.SyncBoxd(ctx)
	if err != nil || result.Count != 1 {
		t.Fatalf("sync = %+v, %v", result, err)
	}
	webID := boxdcloud.FormatSyncID(sandboxfake.BoxdOrgID, "fra", "web")
	web := hostBySyncID(loadProviderConfig(t, p.SyncBoxdConfig), webID)
	if web.Alias != "boxd_fra_web" || web.HostName != "web.boxd.sh" || web.Port != "10022" || web.User != "boxd" ||
		web.IdentityFile != "~/.config/boxd/id_ed25519_test" || !web.IdentitiesOnly {
		t.Fatalf("web block = %+v", web)
	}
	if got := store.Host(web.Alias).Group; got != "boxd/acme/fra" {
		t.Fatalf("group = %q", got)
	}

	result, alias, err := engine.NewBoxd(ctx, boxdcloud.CreateOpts{Region: "nyc", Name: "api"})
	if err != nil || alias != "boxd_nyc_api" || result.Count != 2 {
		t.Fatalf("create = %+v alias=%q err=%v", result, alias, err)
	}
	exercised.Create = true
	apiID := boxdcloud.FormatSyncID(sandboxfake.BoxdOrgID, "nyc", "api")
	if api := hostBySyncID(loadProviderConfig(t, p.SyncBoxdConfig), apiID); api.HostName != "api.nyc.boxd.sh" || api.Port == "" {
		t.Fatalf("api block = %+v", api)
	}

	if _, err := engine.StopBoxd(ctx, apiID); err != nil {
		t.Fatal(err)
	}
	exercised.Stop = true
	if !boxdcloud.HostLooksStopped(store.Host("boxd_nyc_api").Tags) {
		t.Fatalf("stopped computer tags = %v", store.Host("boxd_nyc_api").Tags)
	}
	if _, err := engine.StartBoxd(ctx, apiID); err != nil {
		t.Fatal(err)
	}
	exercised.Start = true
	if fake.Get("nyc", "api").Status != "running" || boxdcloud.HostLooksStopped(store.Host("boxd_nyc_api").Tags) {
		t.Fatal("computer not running after start")
	}
	if _, err := engine.RestartBoxd(ctx, apiID); err != nil {
		t.Fatal(err)
	}
	exercised.Restart = true

	_, forkAlias, err := engine.ForkBoxd(ctx, apiID, boxdcloud.ForkOpts{Name: "api-copy"})
	if err != nil || forkAlias != "boxd_nyc_api-copy" {
		t.Fatalf("fork alias=%q err=%v", forkAlias, err)
	}
	exercised.Fork = true

	host := sshconfig.Host{Alias: "boxd_nyc_api-copy", Synced: true, SyncSource: "boxd", SyncID: boxdcloud.FormatSyncID(sandboxfake.BoxdOrgID, "nyc", "api-copy")}
	if err := engine.EnsureBoxdAccess(ctx, host, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := engine.DeleteBoxd(ctx, host.SyncID); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.DeleteBoxd(ctx, apiID); err != nil {
		t.Fatal(err)
	}
	exercised.Delete = true
	blocks := loadProviderConfig(t, p.SyncBoxdConfig)
	if len(blocks) != 1 || blocks[0].SyncID != webID {
		t.Fatalf("after delete: %+v", blocks)
	}
	if store.Host("boxd_nyc_api").Group != "" {
		t.Fatal("deleted computer kept its metadata")
	}
	if exercised != caps {
		t.Fatalf("exercised %+v, advertised %+v", exercised, caps)
	}
	for _, call := range fake.Calls {
		if call[0] == "machine" && call[1] != "list" && boxdFlagValue(call, "--region") == "" {
			t.Fatalf("a call about one computer must name its region: %v", call)
		}
	}
}

func TestBoxdSyncKeepsTheLastPortWhileTheCLIRewritesItsEntries(t *testing.T) {
	engine, p, _, fake := boxdTestEngine(t)
	fake.Put(sandboxfake.BoxdComputer{Region: "nyc", Name: "web", Status: "running", Port: "10022"})
	ctx := context.Background()
	if _, err := engine.SyncBoxd(ctx); err != nil {
		t.Fatal(err)
	}
	engine.Boxd.ResolveSSH = func(context.Context, string) (boxdcloud.SSHDetails, error) {
		return boxdcloud.SSHDetails{Port: "22"}, nil
	}
	if _, err := engine.SyncBoxd(ctx); err != nil {
		t.Fatal(err)
	}
	block := hostBySyncID(loadProviderConfig(t, p.SyncBoxdConfig), boxdcloud.FormatSyncID(sandboxfake.BoxdOrgID, "nyc", "web"))
	if block.Port != "10022" || block.IdentityFile == "" {
		t.Fatalf("block lost its SSH entry: %+v", block)
	}
}

func TestDisableBoxdRemovesSSHAndMetadata(t *testing.T) {
	engine, p, store, fake := boxdTestEngine(t)
	fake.Put(sandboxfake.BoxdComputer{Region: "fra", Name: "web", Status: "running"})
	ctx := context.Background()
	if _, err := engine.SyncBoxd(ctx); err != nil {
		t.Fatal(err)
	}
	if err := engine.DisableBoxd(ctx); err != nil {
		t.Fatal(err)
	}
	if store.Boxd().Enabled || !store.Boxd().Disabled || store.Host("boxd_fra_web").Group != "" {
		t.Fatalf("integration = %+v", store.Boxd())
	}
	main, err := os.ReadFile(p.MainConfig)
	if err == nil && strings.Contains(string(main), p.SyncBoxdConfig) {
		t.Fatal("include line still present")
	}
}

func TestBoxdSyncKeepsTheHostsOfAnUnavailableRegion(t *testing.T) {
	engine, p, store, fake := boxdTestEngine(t)
	fake.Put(sandboxfake.BoxdComputer{Region: "fra", Name: "web", Status: "running"})
	fake.Put(sandboxfake.BoxdComputer{Region: "nyc", Name: "api", Status: "running"})
	ctx := context.Background()
	if _, err := engine.SyncBoxd(ctx); err != nil {
		t.Fatal(err)
	}
	meta := store.Host("boxd_nyc_api")
	meta.Favorite = true
	meta.Notes = "staging API"
	if err := store.SetHost("boxd_nyc_api", meta); err != nil {
		t.Fatal(err)
	}

	// The CLI leaves an unreadable region out of a --region all list, names it
	// on stderr, and exits 0.
	fake.SetUnavailable("nyc")
	result, err := engine.SyncBoxd(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Error, "nyc is unavailable") || !strings.Contains(store.Boxd().LastSyncError, "nyc is unavailable") {
		t.Fatalf("no warning for the unavailable region: result=%q last=%q", result.Error, store.Boxd().LastSyncError)
	}
	apiID := boxdcloud.FormatSyncID(sandboxfake.BoxdOrgID, "nyc", "api")
	if block := hostBySyncID(loadProviderConfig(t, p.SyncBoxdConfig), apiID); block.Alias != "boxd_nyc_api" || block.Port == "" {
		t.Fatalf("host of the unavailable region was dropped: %+v", block)
	}
	if got := store.Host("boxd_nyc_api"); !got.Favorite || got.Notes != "staging API" || got.Group != "boxd/acme/nyc" {
		t.Fatalf("metadata of the unavailable region was lost: %+v", got)
	}

	// Once nyc answers again, a computer that is gone is pruned.
	fake.SetUnavailable()
	fake.Remove("nyc", "api")
	if _, err := engine.SyncBoxd(ctx); err != nil {
		t.Fatal(err)
	}
	if block := hostBySyncID(loadProviderConfig(t, p.SyncBoxdConfig), apiID); block.SyncID != "" {
		t.Fatalf("deleted computer kept its host: %+v", block)
	}
	if store.Host("boxd_nyc_api").Group != "" {
		t.Fatal("deleted computer kept its metadata")
	}
}

func boxdFlagValue(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func TestAdvertisedLifecycleOpsMatchEngineCoverage(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	forbidden := []string{
		strings.Join([]string{"t", "Skip"}, "."),
		strings.Join([]string{"api", "vercel", "com"}, "."),
		strings.Join([]string{"api", "hetzner", "cloud"}, "."),
		strings.Join([]string{"boat", "upstash", "com"}, "."),
		vercelcloud.DefaultBaseURL,
		hetznercloud.DefaultBaseURL,
		bitqatecloud.DefaultBaseURL,
		upstashcloud.DefaultBaseURL,
	}
	for _, needle := range forbidden {
		if strings.Contains(body, needle) {
			t.Fatalf("lifecycle tests must stay on in-process fakes; found %q", needle)
		}
	}
	if !strings.Contains(body, "engine.Vercel.BaseURL = api.URL()") {
		t.Fatal("Vercel engine tests must point the shipped client at the fake BaseURL")
	}
	if !strings.Contains(body, "engine.Upstash.BaseURL = api.URL()") {
		t.Fatal("Upstash engine tests must point the shipped client at the fake BaseURL")
	}
	if !strings.Contains(body, "engine.Hetzner.BaseURL = api.URL()") {
		t.Fatal("Hetzner engine tests must point the shipped client at the fake BaseURL")
	}
	if !strings.Contains(body, "engine.Bitqate.BaseURL = api.URL()") {
		t.Fatal("BitQate engine tests must point the shipped client at the fake BaseURL")
	}
	if !strings.Contains(body, "engine.Boxd.Run = fake.Runner()") {
		t.Fatal("boxd engine tests must drive the shipped client through the fake runner")
	}
	if !strings.Contains(body, "engine.Boat.Run = fake.Runner()") {
		t.Fatal("Boat engine tests must drive the shipped client through the fake runner")
	}

	methods := map[cloud.Kind]map[string]string{
		cloud.Vercel:  {"Create": "NewVercel", "Stop": "StopVercel", "Start": "ResumeVercel", "Fork": "ForkVercel", "Delete": "DeleteVercel"},
		cloud.Upstash: {"Create": "NewUpstash", "Stop": "StopUpstash", "Start": "ResumeUpstash", "Fork": "ForkUpstash", "Delete": "DeleteUpstash"},
		cloud.Hetzner: {"Start": "StartHetzner", "Stop": "StopHetzner", "Restart": "RestartHetzner"},
		cloud.Bitqate: {"Start": "StartBitqate", "Stop": "StopBitqate", "Restart": "RestartBitqate"},
		cloud.Boat:    {"Create": "NewBoat", "Stop": "StopBoat", "Start": "ResumeBoat", "Fork": "ForkBoat", "Delete": "DeleteBoat"},
		cloud.Boxd:    {"Create": "NewBoxd", "Stop": "StopBoxd", "Start": "StartBoxd", "Restart": "RestartBoxd", "Fork": "ForkBoxd", "Delete": "DeleteBoxd"},
	}
	for _, kind := range []cloud.Kind{cloud.Boat, cloud.Upstash, cloud.Vercel, cloud.Hetzner, cloud.Bitqate, cloud.Boxd} {
		caps := cloud.CapabilitiesFor(kind)
		want, ok := methods[kind]
		if !ok {
			t.Fatalf("no engine method map for %s", kind)
		}
		check := func(flag bool, name, method string) {
			t.Helper()
			if !flag || method == "" {
				return
			}
			if !strings.Contains(body, "engine."+method+"(") {
				t.Fatalf("%s advertises %s but this file never calls engine.%s", kind, name, method)
			}
		}
		check(caps.Create, "Create", want["Create"])
		check(caps.Stop, "Stop", want["Stop"])
		check(caps.Start, "Start", want["Start"])
		check(caps.Restart, "Restart", want["Restart"])
		check(caps.Fork, "Fork", want["Fork"])
		check(caps.Delete, "Delete", want["Delete"])
	}
}

func TestDisableVercelRemovesSSHAndMetadata(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewVercel(t)
	engine.Vercel.BaseURL = api.URL()
	engine.Vercel.Token = sandboxfake.VercelToken
	engine.Vercel.TeamID = sandboxfake.VercelTeam
	engine.Vercel.ProjectID = sandboxfake.VercelProject
	engine.Vercel.HTTP = api.Server.Client()
	engine.Vercel.PollWait = time.Millisecond
	if err := store.SetVercel(metadata.VercelIntegration{TeamID: sandboxfake.VercelTeam, ProjectID: sandboxfake.VercelProject}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.NewVercel(context.Background(), vercelcloud.CreateOpts{Name: "gone", Persistent: true}); err != nil {
		t.Fatal(err)
	}
	if err := engine.DisableVercel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.Vercel().Enabled {
		t.Fatal("vercel remained enabled")
	}
	if _, err := os.Stat(p.SyncVercelConfig); err == nil {
		raw, _ := os.ReadFile(p.SyncVercelConfig)
		if strings.Contains(string(raw), "Host vercel_gone") {
			t.Fatalf("disable left host in config:\n%s", raw)
		}
	}
	if _, ok := store.Hosts()["vercel_gone"]; ok {
		t.Fatal("disable left host metadata")
	}
}

func TestAddAndRemoveVercelProject(t *testing.T) {
	engine, p, store := testEngine(t)
	api := sandboxfake.NewVercel(t)
	api.Put(sandboxfake.VercelSandbox{Name: "one", Status: "running", Persistent: true, Project: "prj_1"})
	api.Put(sandboxfake.VercelSandbox{Name: "two", Status: "running", Persistent: true, Project: "prj_2"})
	engine.Vercel.BaseURL = api.URL()
	engine.Vercel.Token = sandboxfake.VercelToken
	engine.Vercel.TeamID = sandboxfake.VercelTeam
	engine.Vercel.HTTP = api.Server.Client()
	engine.Vercel.PollWait = time.Millisecond
	if err := store.SetVercel(metadata.VercelIntegration{TeamID: sandboxfake.VercelTeam, ProjectID: "prj_1"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := engine.SyncVercel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.Count != 1 {
		t.Fatalf("one project count = %d", first.Count)
	}
	added, err := engine.AddVercelProject(ctx, "prj_2")
	if err != nil {
		t.Fatal(err)
	}
	if added.Count != 2 {
		t.Fatalf("two project count = %d", added.Count)
	}
	if got := strings.Join(store.Vercel().Projects(), ","); got != "prj_1,prj_2" {
		t.Fatalf("stored = %s", got)
	}
	removed, err := engine.RemoveVercelProject(ctx, "prj_2")
	if err != nil {
		t.Fatal(err)
	}
	if removed.Count != 1 {
		t.Fatalf("after remove count = %d", removed.Count)
	}
	blocks := loadProviderConfig(t, p.SyncVercelConfig)
	if hostBySyncID(blocks, "prj_2/two").Alias != "" {
		t.Fatalf("removed project host still present: %+v", blocks)
	}
}
