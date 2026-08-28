package sync

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"bast/internal/cloud"
	boxcloud "bast/internal/cloud/box"
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

func TestBoxEngineLifecycle(t *testing.T) {
	engine, p, store := testEngine(t)
	fake := sandboxfake.NewBox()
	engine.Box.Run = fake.Runner()
	engine.Box.PollInterval = time.Millisecond

	caps := cloud.CapabilitiesFor(cloud.Box)
	exercised := cloud.Capabilities{}
	ctx := context.Background()

	result, alias, err := engine.NewBox(ctx, boxcloud.NewOpts{})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Create = true
	if alias == "" || result.Count != 1 || result.Provider != boxcloud.ProviderName {
		t.Fatalf("create result=%+v alias=%q", result, alias)
	}
	var syncID string
	for id := range fake.Boxes {
		syncID = id
	}
	block := hostBySyncID(loadProviderConfig(t, p.SyncBoxConfig), syncID)
	if block.Alias != alias || block.SyncSource != "box" {
		t.Fatalf("ssh block = %+v", block)
	}
	if boxcloud.HostLooksStopped(block.HostName, store.Host(alias).Tags) {
		t.Fatal("new box looks stopped")
	}

	result, err = engine.StopBox(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Stop = true
	meta := store.Host(alias)
	stoppedHost := hostBySyncID(loadProviderConfig(t, p.SyncBoxConfig), syncID)
	if !boxcloud.HostLooksStopped(stoppedHost.HostName, meta.Tags) {
		t.Fatalf("stopped host=%+v tags=%v", stoppedHost, meta.Tags)
	}

	result, err = engine.ResumeBox(ctx, syncID, boxcloud.ResumeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Start = true
	if boxcloud.HostLooksStopped(hostBySyncID(loadProviderConfig(t, p.SyncBoxConfig), syncID).HostName, store.Host(alias).Tags) {
		t.Fatal("resume left box stopped")
	}

	result, err = engine.StopBox(ctx, syncID)
	if err != nil {
		t.Fatal(err)
	}
	result, forkAlias, err := engine.ForkBox(ctx, syncID, boxcloud.ForkOpts{})
	if err != nil {
		t.Fatal(err)
	}
	exercised.Fork = true
	if forkAlias == "" || result.Count != 2 {
		t.Fatalf("fork result=%+v alias=%q", result, forkAlias)
	}

	if !caps.Delete {
		t.Fatal("box must advertise Delete")
	}
	var forkID string
	for id := range fake.Boxes {
		if id != syncID {
			forkID = id
			break
		}
	}
	result, err = engine.DeleteBox(ctx, forkID)
	if err != nil {
		t.Fatal(err)
	}
	exercised.Delete = true
	if fake.Get(forkID) != nil {
		t.Fatal("deleted fork still in fake")
	}
	if hostBySyncID(loadProviderConfig(t, p.SyncBoxConfig), forkID).Alias != "" {
		t.Fatal("deleted fork still in SSH config")
	}

	if exercised != (cloud.Capabilities{Create: true, Stop: true, Start: true, Fork: true, Delete: true}) {
		t.Fatalf("exercised %+v", exercised)
	}
	if exercised != caps {
		t.Fatalf("exercised %+v, advertised %+v", exercised, caps)
	}
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
		strings.Join([]string{"box", "upstash", "com"}, "."),
		vercelcloud.DefaultBaseURL,
		hetznercloud.DefaultBaseURL,
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
	if !strings.Contains(body, "engine.Box.Run = fake.Runner()") {
		t.Fatal("Box engine tests must drive the shipped client through the fake runner")
	}

	methods := map[cloud.Kind]map[string]string{
		cloud.Vercel:  {"Create": "NewVercel", "Stop": "StopVercel", "Start": "ResumeVercel", "Fork": "ForkVercel", "Delete": "DeleteVercel"},
		cloud.Upstash: {"Create": "NewUpstash", "Stop": "StopUpstash", "Start": "ResumeUpstash", "Fork": "ForkUpstash", "Delete": "DeleteUpstash"},
		cloud.Hetzner: {"Start": "StartHetzner", "Stop": "StopHetzner", "Restart": "RestartHetzner"},
		cloud.Box:     {"Create": "NewBox", "Stop": "StopBox", "Start": "ResumeBox", "Fork": "ForkBox", "Delete": "DeleteBox"},
	}
	for _, kind := range []cloud.Kind{cloud.Box, cloud.Upstash, cloud.Vercel, cloud.Hetzner} {
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
