package boxd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"bast/internal/sshconfig"
)

type fakeMachine struct {
	Org, Region, Name, Status string
	SSHDisabled               bool
	// After lists the states a computer reports on later gets, one per call.
	After []string
}

type fakeCLI struct {
	mu        stdsync.Mutex
	loggedIn  bool
	active    string
	orgs      []Org
	machines  []*fakeMachine
	calls     [][]string
	listCalls int
	// unavailable regions are left out of a --region all list and named on
	// stderr, and the list still succeeds, like the CLI.
	unavailable []string
}

func newFakeCLI() *fakeCLI {
	return &fakeCLI{
		loggedIn: true,
		active:   "acme",
		orgs: []Org{
			{ID: "org-1111", Name: "acme"},
			{ID: "org-2222", Name: "Side Project's org"},
		},
	}
}

func (f *fakeCLI) add(m *fakeMachine) { f.machines = append(f.machines, m) }

func (f *fakeCLI) find(org, region, name string) *fakeMachine {
	for _, m := range f.machines {
		if m.Org == org && m.Region == region && m.Name == name {
			return m
		}
	}
	return nil
}

func flagValue(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func (f *fakeCLI) url(m *fakeMachine) string {
	if m.Region == "fra" {
		return m.Name + ".boxd.sh"
	}
	return m.Name + "." + m.Region + ".boxd.sh"
}

func (f *fakeCLI) run(_ context.Context, args []string, _ []string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	args = args[1:]
	f.calls = append(f.calls, append([]string(nil), args...))
	var stderr strings.Builder
	if len(args) > 1 && args[0] == "machine" && args[1] == "list" && flagValue(args, "--region") == "all" {
		for _, region := range f.unavailable {
			fmt.Fprintf(&stderr, "█ %s: unavailable (can't be reached right now: connection refused)\n", region)
		}
	}
	out, err := f.respond(args)
	return out, []byte(stderr.String()), err
}

func (f *fakeCLI) respond(args []string) ([]byte, error) {
	if len(args) > 0 && args[0] == "version" {
		return []byte("boxd 0.3.3\n"), nil
	}
	if !f.loggedIn {
		return nil, ErrNotLoggedIn
	}
	if len(args) >= 1 && args[0] == "auth" {
		type orgJSON struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Active bool   `json:"active"`
		}
		var orgs []orgJSON
		for _, org := range f.orgs {
			orgs = append(orgs, orgJSON{ID: org.ID, Name: org.Name, Active: org.Name == f.active})
		}
		return json.Marshal(map[string]any{"active_org": f.active, "display_name": "tester", "orgs": orgs, "user_id": "gh-tester"})
	}
	if len(args) < 2 || args[0] != "machine" {
		return nil, fmt.Errorf("boxd: unexpected command %v", args)
	}
	org, region := flagValue(args, "--org"), flagValue(args, "--region")
	switch args[1] {
	case "list":
		f.listCalls++
		var rows []map[string]any
		for _, m := range f.machines {
			if m.Org != org || (region != "all" && region != "" && m.Region != region) {
				continue
			}
			if region == "all" && slices.Contains(f.unavailable, m.Region) {
				continue
			}
			rows = append(rows, map[string]any{"region": m.Region, "name": m.Name, "status": m.Status, "size": "2x8G", "url": f.url(m)})
		}
		if rows == nil {
			return []byte("[]"), nil
		}
		return json.Marshal(rows)
	case "get":
		m := f.find(org, region, args[2])
		if m == nil {
			return nil, fmt.Errorf("%w: no machine %q in %s", ErrNotFound, args[2], region)
		}
		if len(m.After) > 0 {
			m.Status, m.After = m.After[0], m.After[1:]
		}
		return json.Marshal(map[string]any{"name": m.Name, "id": "id-" + m.Name, "status": m.Status, "url": f.url(m), "ssh_disabled": m.SSHDisabled})
	case "new":
		if flagValue(args, "--vcpu") != "" && flagValue(args, "--from-snapshot") != "" {
			return nil, errors.New("boxd: --vcpu/--memory only apply to fresh machines; forks and snapshot restores keep their source's size")
		}
		name := args[2]
		if region == "" {
			region = "fra"
		}
		m := &fakeMachine{Org: org, Region: region, Name: name, Status: "running"}
		f.add(m)
		return json.Marshal(map[string]any{"id": "id-" + name, "name": name, "url": f.url(m), "boot": "6ms"})
	case "fork":
		src := f.find(org, region, args[2])
		if src == nil {
			return nil, fmt.Errorf("%w: no machine %q", ErrNotFound, args[2])
		}
		name := src.Name + "-fork"
		if len(args) > 3 && !strings.HasPrefix(args[3], "-") {
			name = args[3]
		}
		state := "running"
		if src.Status == "rebooting" || src.Status == "starting" {
			state = "failed"
		}
		m := &fakeMachine{Org: org, Region: region, Name: name, Status: state}
		f.add(m)
		return json.Marshal(map[string]any{"id": "id-" + name, "name": name, "url": f.url(m), "boot": "starting"})
	case "stop", "start", "wake", "resume", "reboot":
		m := f.find(org, region, args[2])
		if m == nil {
			return nil, fmt.Errorf("%w: no machine %q", ErrNotFound, args[2])
		}
		m.Status = map[string]string{"stop": "stopped", "start": "running", "wake": "running", "resume": "running", "reboot": "running"}[args[1]]
		return []byte(`{"name":"` + m.Name + `","status":"ok"}`), nil
	case "remove":
		for i, m := range f.machines {
			if m.Org == org && m.Region == region && m.Name == args[2] {
				f.machines = append(f.machines[:i], f.machines[i+1:]...)
				return []byte(`{"name":"` + args[2] + `","status":"destroyed"}`), nil
			}
		}
		return nil, fmt.Errorf("%w: no machine %q", ErrNotFound, args[2])
	}
	return nil, fmt.Errorf("boxd: unexpected command %v", args)
}

func (f *fakeCLI) commands(verb string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, call := range f.calls {
		if len(call) > 1 && call[0] == "machine" && call[1] == verb {
			out = append(out, call)
		}
	}
	return out
}

type fakeSSH struct {
	mu      stdsync.Mutex
	entries map[string]SSHDetails
}

func (s *fakeSSH) resolve(_ context.Context, host string) (SSHDetails, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.entries[host]; ok {
		return d, nil
	}
	return SSHDetails{HostName: host, Port: "22", User: "me"}, nil
}

func (s *fakeSSH) set(host string, d SSHDetails) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[host] = d
}

func newTestClient(f *fakeCLI, s *fakeSSH) *Client {
	return &Client{Boxd: "boxd", Run: f.run, ResolveSSH: s.resolve, PollInterval: time.Millisecond, Home: "/home/me"}
}

func boxdEntry(port string) SSHDetails {
	return SSHDetails{Port: port, User: "boxd", IdentityFile: "/home/me/.config/boxd/id_ed25519_dev_user", IdentitiesOnly: true, DeviceID: "dev-1"}
}

func TestDiscoverListsEveryRegionOfTheActiveOrg(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-1111", Region: "fra", Name: "web", Status: "running"})
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "web", Status: "hibernated"})
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "old", Status: "destroyed"})
	f.add(&fakeMachine{Org: "org-2222", Region: "fra", Name: "other", Status: "running"})
	s := &fakeSSH{entries: map[string]SSHDetails{"web.boxd.sh": boxdEntry("10022"), "web.nyc.boxd.sh": boxdEntry("10023")}}
	got, err := newTestClient(f, s).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Instances) != 2 || len(got.Warnings) != 0 || !got.ConfirmedOrgs["org-1111"] || got.ListedOrgs["org-2222"] {
		t.Fatalf("discovery = %+v", got)
	}
	for _, call := range f.commands("list") {
		if flagValue(call, "--region") != "all" || flagValue(call, "--org") != "org-1111" {
			t.Fatalf("list must ask every region of the org by id: %v", call)
		}
	}
	fra, nyc := got.Instances[0], got.Instances[1]
	if fra.SyncID != "org-1111/fra/web" || fra.HostName != "web.boxd.sh" || fra.Port != "10022" || fra.User != "boxd" {
		t.Fatalf("fra instance = %+v", fra)
	}
	if nyc.SyncID != "org-1111/nyc/web" || nyc.HostName != "web.nyc.boxd.sh" || nyc.Port != "10023" || HostLooksStopped(nyc.Tags) {
		t.Fatalf("nyc instance = %+v (hibernated computers wake on SSH, so they count as running)", nyc)
	}
	if fra.IdentityFile != "~/.config/boxd/id_ed25519_dev_user" || !fra.IdentitiesOnly || fra.DeviceID != "dev-1" {
		t.Fatalf("identity = %q only=%t device=%q", fra.IdentityFile, fra.IdentitiesOnly, fra.DeviceID)
	}
	if AliasFor(fra) != "boxd_fra_web" || AliasFor(nyc) != "boxd_nyc_web" || GroupPath(nyc) != "boxd/acme/nyc" {
		t.Fatalf("alias/group = %s %s %s", AliasFor(fra), AliasFor(nyc), GroupPath(nyc))
	}
}

func TestDiscoverOrgFilterMatchesNamesCaseInsensitively(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-2222", Region: "fra", Name: "other", Status: "stopped"})
	s := &fakeSSH{entries: map[string]SSHDetails{}}
	got, err := newTestClient(f, s).Discover(context.Background(), DiscoverConfig{OrgFilter: []string{"side project's ORG", "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Instances) != 1 || got.Instances[0].OrgName != "Side Project's org" || !HostLooksStopped(got.Instances[0].Tags) {
		t.Fatalf("instances = %+v", got.Instances)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "missing") {
		t.Fatalf("warnings = %v", got.Warnings)
	}
	if got.Instances[0].Port != "" {
		t.Fatalf("a computer without a boxd SSH entry must not get port 22: %+v", got.Instances[0])
	}
}

func TestDiscoverFallsBackToTheOnlyOrgOfAnAPIKey(t *testing.T) {
	f := newFakeCLI()
	f.active = ""
	f.orgs = f.orgs[:1]
	got, err := newTestClient(f, &fakeSSH{entries: map[string]SSHDetails{}}).Discover(context.Background(), DiscoverConfig{})
	if err != nil || !got.ConfirmedOrgs["org-1111"] {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestDiscoverReportsMissingLogin(t *testing.T) {
	f := newFakeCLI()
	f.loggedIn = false
	_, err := newTestClient(f, &fakeSSH{}).Discover(context.Background(), DiscoverConfig{})
	if err == nil || !strings.Contains(err.Error(), "boxd auth login") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverMarksARegionTheListCouldNotRead(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-1111", Region: "fra", Name: "web", Status: "running"})
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "api", Status: "running"})
	f.unavailable = []string{"nyc"}
	got, err := newTestClient(f, &fakeSSH{entries: map[string]SSHDetails{}}).Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Instances) != 1 || got.Instances[0].Region != "fra" || !got.ConfirmedOrgs["org-1111"] {
		t.Fatalf("discovery = %+v", got)
	}
	if !got.RegionUnavailable("org-1111", "nyc") || got.RegionUnavailable("org-1111", "fra") {
		t.Fatalf("unavailable = %v", got.Unavailable)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "nyc is unavailable (can't be reached right now: connection refused)") {
		t.Fatalf("warnings = %v", got.Warnings)
	}
}

func TestParseUnavailableRegions(t *testing.T) {
	stderr := "█ nyc: unavailable (can't be reached right now: timed out)\n\n  A new version of boxd is available (v0.3.4). Update with:\n"
	got := parseUnavailableRegions([]byte(stderr))
	if len(got) != 1 || got[0].Region != "nyc" || got[0].Reason != "can't be reached right now: timed out" {
		t.Fatalf("got %+v", got)
	}
	if got := parseUnavailableRegions(nil); len(got) != 0 {
		t.Fatalf("empty stderr: %+v", got)
	}
}

func TestCreateFromASnapshotKeepsTheSnapshotsSize(t *testing.T) {
	f := newFakeCLI()
	c := newTestClient(f, &fakeSSH{})
	ctx := context.Background()
	if _, err := c.Create(ctx, CreateOpts{OrgID: "org-1111", Region: "nyc", Name: "restored", Snapshot: "golden"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(ctx, CreateOpts{OrgID: "org-1111", Region: "nyc", Name: "sized", Snapshot: "golden", VCPU: 4}); err == nil || !strings.Contains(err.Error(), "keeps the snapshot's size") {
		t.Fatalf("snapshot with a size: err = %v", err)
	}
	if _, err := c.Create(ctx, CreateOpts{OrgID: "org-1111", Region: "nyc", Name: "fresh", VCPU: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(ctx, CreateOpts{OrgID: "org-1111", Region: "nyc", Name: "plain"}); err != nil {
		t.Fatal(err)
	}
	created := f.commands("new")
	if len(created) != 3 {
		t.Fatalf("a snapshot with a size must not reach the CLI: %v", created)
	}
	restored, fresh, plain := created[0], created[1], created[2]
	if flagValue(restored, "--from-snapshot") != "golden" || containsArg(restored, "--vcpu") {
		t.Fatalf("from a snapshot: %v", restored)
	}
	if flagValue(fresh, "--vcpu") != "4" {
		t.Fatalf("with a size: %v", fresh)
	}
	if containsArg(plain, "--vcpu") {
		t.Fatalf("without a size the org's default applies: %v", plain)
	}
}

func TestCheckAvailableReportsAMissingCLI(t *testing.T) {
	for _, bin := range []string{filepath.Join(t.TempDir(), "boxd"), "boxd-not-on-path"} {
		c := &Client{Boxd: bin, Run: defaultRunner}
		if err := c.CheckAvailable(context.Background()); err == nil || !strings.Contains(err.Error(), "boxd CLI not found") {
			t.Fatalf("%s: err = %v", bin, err)
		}
	}
}

func TestLifecycleCallsUseTheComputersOwnOrgAndRegion(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-2222", Region: "nyc", Name: "api", Status: "running"})
	c := newTestClient(f, &fakeSSH{entries: map[string]SSHDetails{}})
	ctx := context.Background()
	id := FormatSyncID("org-2222", "nyc", "api")
	if err := c.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := c.Restart(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Fork(ctx, id, ForkOpts{Name: "api-copy"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.calls {
		if call[0] != "machine" {
			continue
		}
		if flagValue(call, "--org") != "org-2222" || flagValue(call, "--region") != "nyc" {
			t.Fatalf("call without the computer's org and region: %v", call)
		}
	}
	remove := f.commands("remove")
	if len(remove) != 1 || !containsArg(remove[0], "-y") {
		t.Fatalf("remove must confirm with -y: %v", remove)
	}
}

func TestStartPicksTheVerbForEachState(t *testing.T) {
	for state, verb := range map[string]string{"stopped": "start", "hibernated": "wake", "standby": "resume", "suspended": "resume", "running": ""} {
		f := newFakeCLI()
		f.add(&fakeMachine{Org: "org-1111", Region: "fra", Name: "web", Status: state})
		if err := newTestClient(f, &fakeSSH{}).Start(context.Background(), "org-1111/fra/web"); err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		for _, v := range []string{"start", "wake", "resume"} {
			if got := len(f.commands(v)); (v == verb) != (got == 1) {
				t.Fatalf("state %s: %d %q calls, want verb %q", state, got, v, verb)
			}
		}
	}
}

func TestForkWaitsForARebootingSourceToSettle(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "web", Status: "rebooting", After: []string{"rebooting", "running"}})
	created, err := newTestClient(f, &fakeSSH{}).Fork(context.Background(), "org-1111/nyc/web", ForkOpts{})
	if err != nil {
		t.Fatalf("fork of a rebooting computer: %v", err)
	}
	if created.Name != "web-fork" || created.HostName != "web-fork.nyc.boxd.sh" {
		t.Fatalf("created = %+v", created)
	}
	if fork := f.find("org-1111", "nyc", "web-fork"); fork == nil || fork.Status != "running" {
		t.Fatalf("fork = %+v", fork)
	}
}

func TestForkReportsAFailedCopy(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "web", Status: "running"})
	c := newTestClient(f, &fakeSSH{})
	c.Run = func(ctx context.Context, args []string, env []string) ([]byte, []byte, error) {
		out, stderr, err := f.run(ctx, args, env)
		if len(args) > 2 && args[2] == "fork" {
			f.find("org-1111", "nyc", "web-fork").Status = "failed"
		}
		return out, stderr, err
	}
	if _, err := c.Fork(context.Background(), "org-1111/nyc/web", ForkOpts{}); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteOfAMissingComputerSucceeds(t *testing.T) {
	if err := newTestClient(newFakeCLI(), &fakeSSH{}).Delete(context.Background(), "org-1111/fra/gone"); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureAccess(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "web", Status: "hibernated"})
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "off", Status: "stopped"})
	f.add(&fakeMachine{Org: "org-1111", Region: "nyc", Name: "nossh", Status: "running", SSHDisabled: true})
	s := &fakeSSH{entries: map[string]SSHDetails{"web.nyc.boxd.sh": boxdEntry("12345")}}
	c := newTestClient(f, s)
	ctx := context.Background()

	got, err := c.EnsureAccess(ctx, "org-1111/nyc/web", EnsureConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got.HostName != "web.nyc.boxd.sh" || got.Port != "12345" || got.User != "boxd" || !got.IdentitiesOnly {
		t.Fatalf("result = %+v", got)
	}
	if len(f.commands("wake")) != 0 || len(f.commands("start")) != 0 {
		t.Fatal("a hibernated computer wakes on the SSH connection; no lifecycle call")
	}
	if _, err := c.EnsureAccess(ctx, "org-1111/nyc/off", EnsureConfig{Alias: "boxd_nyc_off"}); err == nil || !strings.Contains(err.Error(), "bast boxd start boxd_nyc_off") {
		t.Fatalf("stopped err = %v", err)
	}
	if _, err := c.EnsureAccess(ctx, "org-1111/nyc/nossh", EnsureConfig{}); err == nil || !strings.Contains(err.Error(), "ssh on --org org-1111 --region nyc") {
		t.Fatalf("ssh off err = %v", err)
	}
}

func TestEnsureAccessRefreshesAMissingEntryOnce(t *testing.T) {
	f := newFakeCLI()
	f.add(&fakeMachine{Org: "org-1111", Region: "fra", Name: "new", Status: "running"})
	s := &fakeSSH{entries: map[string]SSHDetails{}}
	c := newTestClient(f, s)
	c.Run = func(ctx context.Context, args []string, env []string) ([]byte, []byte, error) {
		out, stderr, err := f.run(ctx, args, env)
		if len(args) > 2 && args[2] == "list" {
			s.set("new.boxd.sh", boxdEntry("20001"))
		}
		return out, stderr, err
	}
	got, err := c.EnsureAccess(context.Background(), "org-1111/fra/new", EnsureConfig{EntryWait: time.Second})
	if err != nil || got.Port != "20001" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if f.listCalls != 1 {
		t.Fatalf("list calls = %d, want one refresh", f.listCalls)
	}

	f2 := newFakeCLI()
	f2.add(&fakeMachine{Org: "org-1111", Region: "fra", Name: "new", Status: "running"})
	_, err = newTestClient(f2, &fakeSSH{entries: map[string]SSHDetails{}}).EnsureAccess(context.Background(), "org-1111/fra/new", EnsureConfig{EntryWait: 5 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "boxd auth login") {
		t.Fatalf("missing entry err = %v", err)
	}
}

func TestParseSSHG(t *testing.T) {
	out := "user boxd\nhostname web.nyc.boxd.sh\nport 10022\nidentitiesonly yes\nidentityfile /Users/me/Library/Application Support/boxd/id_ed25519_dev_user\nidentityfile ~/.ssh/id_rsa\nsetenv BOXD_DEVICE_ID=dev-1234\n"
	got := ParseSSHG([]byte(out))
	if got.Port != "10022" || got.IdentityFile != "/Users/me/Library/Application Support/boxd/id_ed25519_dev_user" || !got.IdentitiesOnly || got.DeviceID != "dev-1234" || !got.Found() {
		t.Fatalf("parsed = %+v", got)
	}
	if (SSHDetails{Port: "22"}).Found() || (SSHDetails{}).Found() {
		t.Fatal("port 22 reaches the boxd proxy, not a computer")
	}
}

func TestParseSyncID(t *testing.T) {
	org, region, name, err := ParseSyncID("org-1234abcd/nyc/web-1")
	if err != nil || org != "org-1234abcd" || region != "nyc" || name != "web-1" {
		t.Fatalf("got %q %q %q %v", org, region, name, err)
	}
	for _, bad := range []string{"", "a/b", "a/b/c/d", "a/-b/c", "a/b/c d", "a/b/c;rm"} {
		if _, _, _, err := ParseSyncID(bad); err == nil {
			t.Fatalf("ParseSyncID(%q) accepted", bad)
		}
	}
}

func TestSyncHostResolvesThroughOpenSSH(t *testing.T) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("ssh not installed")
	}
	inst := Instance{
		SyncID: "org-1/nyc/web", Name: "web", Region: "nyc", HostName: "web.nyc.boxd.sh", Port: "10022", User: "boxd",
		IdentityFile: "~/Library/Application Support/boxd/id_ed25519_dev_user", IdentitiesOnly: true, DeviceID: "dev-1",
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := sshconfig.WriteSyncConfig(path, []sshconfig.SyncHostInput{ToSyncHost(inst, "")}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(context.Background(), ssh, "-G", "-F", path, "boxd_nyc_web").CombinedOutput()
	if err != nil {
		t.Fatalf("ssh -G: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"hostname web.nyc.boxd.sh", "port 10022", "user boxd", "identitiesonly yes", "Application Support/boxd/id_ed25519_dev_user", "serveraliveinterval 30", "BOXD_DEVICE_ID=dev-1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ssh -G missing %q:\n%s", want, text)
		}
	}
}

func TestDefaultRunnerStopsTheBrowserSignIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "boxd")
	body := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'boxd 0.3.3'; exit 0; fi\necho \"Not logged in. Let's fix that.\" >&2\nexec sleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _, err := defaultRunner(context.Background(), []string{script, "auth", "--json"}, nil)
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("runner waited %s for a sign-in nobody watches", elapsed)
	}
	c := &Client{Boxd: script, Run: defaultRunner}
	account, err := c.Account(context.Background())
	if err != nil || account.Authenticated || !strings.Contains(account.Error, "boxd auth login") {
		t.Fatalf("account = %+v, %v", account, err)
	}
}

func TestCLIErrorDropsTheUpdateNotice(t *testing.T) {
	err := cliError("error: no machine \"web\" in nyc\n\n  A new version of boxd is available (v0.3.3). Update with:\n    curl -fsSL https://boxd.sh/downloads/install.sh | sh\nIf this call should have worked, `boxd feedback` reports it.\n", errors.New("exit status 1"))
	if !errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "new version") || strings.Contains(err.Error(), "feedback") {
		t.Fatalf("err = %v", err)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
