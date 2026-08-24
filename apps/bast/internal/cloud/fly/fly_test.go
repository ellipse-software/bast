package fly

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bast/internal/sshconfig"
)

func TestDiscoverParsesRunningAndStopped(t *testing.T) {
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "version"):
				return []byte("fly v0.3.0"), nil
			case strings.Contains(cmd, "auth whoami"):
				return []byte("ada@example.com\n"), nil
			case strings.Contains(cmd, "orgs list"):
				return []byte(`{"personal":"Personal","acme":"Acme"}`), nil
			case strings.Contains(cmd, "apps list --org personal"):
				return []byte(`[{"name":"web","organization":{"slug":"personal"}},{"name":"flyctl-interactive-shells-abc"}]`), nil
			case strings.Contains(cmd, "apps list --org acme"):
				return []byte(`[{"name":"api","organization":"acme"}]`), nil
			case strings.Contains(cmd, "machine list --app web"):
				return json.Marshal([]map[string]any{
					{"id": "e286065f969386", "name": "web-1", "state": "started", "region": "iad", "config": map[string]any{"image": "nginx:latest"}},
					{"id": "a1111111111111", "name": "web-stop", "state": "stopped", "region": "iad"},
				})
			case strings.Contains(cmd, "machine list --app api"):
				return json.Marshal([]map[string]any{
					{"id": "b2222222222222", "name": "api-1", "state": "started", "region": "lhr"},
				})
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !discovery.Complete || len(discovery.Instances) != 3 {
		t.Fatalf("complete=%t instances=%d", discovery.Complete, len(discovery.Instances))
	}
	var running, stopped Instance
	for _, inst := range discovery.Instances {
		switch inst.SyncID {
		case "personal/web/e286065f969386":
			running = inst
		case "personal/web/a1111111111111":
			stopped = inst
		}
	}
	if running.SyncID == "" || !running.Running || running.HostName != "e286065f969386" {
		t.Fatalf("running = %+v", running)
	}
	if GroupPath(running) != "Fly.io/Personal/web" {
		t.Fatalf("group = %q", GroupPath(running))
	}
	if stopped.Running || stopped.HostName != StoppedHostName || !HostLooksStopped(stopped.HostName, stopped.Tags) {
		t.Fatalf("stopped = %+v", stopped)
	}
	if HostLooksStopped(running.HostName, running.Tags) {
		t.Fatal("running machine should not look stopped")
	}
}

func TestDiscoverAppliesFiltersAndKeepsPartialInventory(t *testing.T) {
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "version"), strings.Contains(cmd, "auth whoami"):
				return []byte("ok"), nil
			case strings.Contains(cmd, "orgs list"):
				return []byte(`{"personal":"Personal","other":"Other"}`), nil
			case strings.Contains(cmd, "apps list --org personal"):
				return []byte(`[{"name":"web"},{"name":"worker"}]`), nil
			case strings.Contains(cmd, "machine list --app web"):
				return []byte(`[{"id":"e286065f969386","name":"web-1","state":"started","region":"iad"}]`), nil
			case strings.Contains(cmd, "machine list --app worker"):
				return nil, fmt.Errorf("permission denied")
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	discovery, err := client.Discover(context.Background(), DiscoverConfig{
		OrgFilter: []string{"personal"}, AppFilter: []string{"web", "worker"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if discovery.Complete {
		t.Fatal("worker list failed; discovery should be incomplete")
	}
	if !discovery.ExcludedOrgs["other"] || !discovery.ConfirmedApps["personal/web"] || discovery.ConfirmedApps["personal/worker"] {
		t.Fatalf("maps orgs=%v apps=%v", discovery.ExcludedOrgs, discovery.ConfirmedApps)
	}
	if len(discovery.Instances) != 1 {
		t.Fatalf("instances = %d", len(discovery.Instances))
	}
}

func TestParseSyncIDAndAlias(t *testing.T) {
	org, app, id, err := ParseSyncID("personal/web/e286065f969386")
	if err != nil || org != "personal" || app != "web" || id != "e286065f969386" {
		t.Fatalf("%s %s %s %v", org, app, id, err)
	}
	if _, _, _, err := ParseSyncID("bad"); err == nil {
		t.Fatal("expected invalid id")
	}
	inst := Instance{OrgSlug: "personal", App: "web", Name: "web-1", SyncID: "personal/web/e286065f969386"}
	if got := AliasFor(inst); got != "fly_personal_web_web-1" {
		t.Fatalf("alias = %q", got)
	}
}

func TestToSyncHostWritesProxyCommand(t *testing.T) {
	inst := Instance{
		SyncID: "personal/web/e286065f969386", Name: "web-1", OrgSlug: "personal", App: "web",
		HostName: "e286065f969386", User: "root", Running: true,
	}
	block := ToSyncHost(inst, "fly_web", "/Applications/Bast Preview/bast")
	if block.ProxyCommand == "" || !strings.Contains(block.ProxyCommand, "__fly-proxy") {
		t.Fatalf("proxy = %q", block.ProxyCommand)
	}
	if !strings.Contains(block.ProxyCommand, "--resource-port %p") {
		t.Fatalf("expected unquoted %%p in %q", block.ProxyCommand)
	}
	if runtime.GOOS == "windows" {
		if !strings.Contains(block.ProxyCommand, `"/Applications/Bast Preview/bast"`) {
			t.Fatalf("windows quote missing: %q", block.ProxyCommand)
		}
	} else if !strings.Contains(block.ProxyCommand, "'/Applications/Bast Preview/bast'") {
		t.Fatalf("posix quote missing: %q", block.ProxyCommand)
	}
}

func TestParseProxyOptionsRejectsExtraArgs(t *testing.T) {
	if _, err := ParseProxyOptions([]string{"--org", "personal", "--app", "web", "--machine", "abc", "extra"}); err == nil {
		t.Fatal("expected error")
	}
	options, err := ParseProxyOptions([]string{"--org", "personal", "--app", "web", "--machine", "e286065f969386", "--resource-port", "22"})
	if err != nil {
		t.Fatal(err)
	}
	if options.App != "web" || options.Machine != "e286065f969386" {
		t.Fatalf("%+v", options)
	}
}

func TestLifecycleParsesMachineIDs(t *testing.T) {
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "machine run"):
				return []byte("Machine e286065f969386 has been created"), nil
			case strings.Contains(cmd, "machine clone"):
				return []byte("Cloning machine a1111111111111\nMachine b2222222222222 has been created"), nil
			case strings.Contains(cmd, "machine list"):
				return []byte(`[{"id":"e286065f969386","state":"started"},{"id":"b2222222222222","state":"started"}]`), nil
			case strings.Contains(cmd, "machine start"), strings.Contains(cmd, "machine stop"), strings.Contains(cmd, "machine destroy"):
				return []byte("ok"), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	id, err := client.Create(context.Background(), CreateOpts{App: "web", Image: "nginx"})
	if err != nil || id != "e286065f969386" {
		t.Fatalf("create id=%q err=%v", id, err)
	}
	cloned, err := client.Clone(context.Background(), "personal", "web", "a1111111111111", ForkOpts{})
	if err != nil || cloned != "b2222222222222" {
		t.Fatalf("clone id=%q err=%v", cloned, err)
	}
}

func TestEnsureAccessIssuesCertificate(t *testing.T) {
	home := t.TempDir()
	keys := filepath.Join(home, "keys")
	issued := false
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "version"):
				return []byte("fly"), nil
			case strings.Contains(cmd, "machine list"):
				return []byte(`[{"id":"e286065f969386","state":"started"}]`), nil
			case strings.Contains(cmd, "ssh issue"):
				issued = true
				path := ""
				for i, arg := range args {
					if arg == "issue" && i+2 < len(args) {
						path = args[i+2]
						break
					}
				}
				if path == "" {
					return nil, fmt.Errorf("missing issue path: %s", cmd)
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					return nil, err
				}
				if err := os.WriteFile(path, []byte("key"), 0o600); err != nil {
					return nil, err
				}
				if err := os.WriteFile(path+"-cert.pub", []byte("cert"), 0o600); err != nil {
					return nil, err
				}
				return []byte("issued"), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	result, err := client.EnsureAccess(context.Background(), "personal/web/e286065f969386", EnsureConfig{
		Home: home, ManagedKeys: keys,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !issued || result.User != "root" || result.HostName != "e286065f969386" || result.IdentityFile == "" {
		t.Fatalf("result=%+v issued=%t", result, issued)
	}
	issued = false
	if _, err := client.EnsureAccess(context.Background(), "personal/web/e286065f969386", EnsureConfig{
		Home: home, ManagedKeys: keys,
	}); err != nil {
		t.Fatal(err)
	}
	if issued {
		t.Fatal("fresh certificate should be reused")
	}
}

func TestFlyctlErrorStripsMetricsAndPrompt(t *testing.T) {
	stderr := "Warning: Metrics token unavailable: failed to run query($slug: String!) { organization(slug: $slug) { id internalNumericId slug rawSlug name type billable } }: context canceled\nError: prompt: non interactive\n"
	got := flyctlError(stderr, fmt.Errorf("exit status 1"))
	if strings.Contains(strings.ToLower(got), "metrics") {
		t.Fatalf("metrics warning leaked: %q", got)
	}
	if strings.Contains(got, "Error:") {
		t.Fatalf("Error prefix leaked: %q", got)
	}
	if !strings.Contains(got, "fly auth login") {
		t.Fatalf("got %q", got)
	}
}

func TestAccountUsesJSONWhoami(t *testing.T) {
	var whoami string
	client := &Client{
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "version"):
				return []byte("fly v0.3.0"), nil
			case strings.Contains(cmd, "auth whoami"):
				whoami = cmd
				if !strings.Contains(cmd, "--json") {
					return nil, fmt.Errorf("whoami must use --json: %s", cmd)
				}
				return []byte(`{"email":"ada@example.com"}`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	status, err := client.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Authenticated || status.Login != "ada@example.com" {
		t.Fatalf("%+v", status)
	}
	if !strings.Contains(whoami, "--json") {
		t.Fatalf("whoami = %q", whoami)
	}
}

func TestCreatePassesDetachRegionAndOrg(t *testing.T) {
	var runCmd string
	client := &Client{
		PollInterval: time.Millisecond,
		Run: func(_ context.Context, args []string, _ []string) ([]byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "machine run"):
				runCmd = cmd
				return []byte("Machine e286065f969386 has been created"), nil
			case strings.Contains(cmd, "machine list"):
				return []byte(`[{"id":"e286065f969386","state":"started"}]`), nil
			default:
				return nil, fmt.Errorf("unexpected %s", cmd)
			}
		},
	}
	id, err := client.Create(context.Background(), CreateOpts{
		App: "web", Image: "nginx", Region: "iad", Org: "personal",
	})
	if err != nil || id != "e286065f969386" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	for _, want := range []string{"--detach", "--region iad", "--org personal", "--app web"} {
		if !strings.Contains(runCmd, want) {
			t.Fatalf("missing %q in %q", want, runCmd)
		}
	}
}

func TestDefaultRunnerIsolatesFlyctl(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "fly")
	body := `#!/bin/sh
echo "FLY_NO_UPDATE_CHECK=${FLY_NO_UPDATE_CHECK}"
echo "FLY_SEND_METRICS=${FLY_SEND_METRICS}"
echo "FLY_APP=[${FLY_APP}]"
echo "PWD=$(pwd)"
if [ -t 0 ]; then echo STDIN_TTY=1; else echo STDIN_TTY=0; fi
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := defaultRunner(context.Background(), []string{script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "FLY_NO_UPDATE_CHECK=1") || !strings.Contains(got, "FLY_SEND_METRICS=0") {
		t.Fatalf("env:\n%s", got)
	}
	if !strings.Contains(got, "FLY_APP=[]") {
		t.Fatalf("FLY_APP should be cleared:\n%s", got)
	}
	if !strings.Contains(got, "STDIN_TTY=0") {
		t.Fatalf("stdin should not be a TTY:\n%s", got)
	}
	if want := isolatedWorkDir(); want != "" {
		resolved, err := filepath.EvalSymlinks(want)
		if err != nil {
			resolved = want
		}
		if !strings.Contains(got, "PWD="+want) && !strings.Contains(got, "PWD="+resolved) {
			t.Fatalf("cwd:\n%s\nwant %s", got, resolved)
		}
	}
}

func TestGeneratedFlyConfigIsAcceptedByOpenSSH(t *testing.T) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH client is not installed")
	}
	path := filepath.Join(t.TempDir(), "config")
	inst := Instance{
		SyncID: "personal/web/e286065f969386", Name: "web-1", OrgSlug: "personal", App: "web",
		HostName: "e286065f969386", User: "root", Running: true,
	}
	block := ToSyncHost(inst, "fly_test", "/Applications/Bast Preview/bast")
	block.IdentityFile = "~/.ssh/bast/keys/fly_personal"
	block.CertificateFile = "~/.ssh/bast/keys/fly_personal-cert.pub"
	if err := sshconfig.WriteSyncConfig(path, []sshconfig.SyncHostInput{block}); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(context.Background(), ssh, "-G", "-F", path, "fly_test").CombinedOutput()
	if err != nil {
		t.Fatalf("ssh -G failed: %v\n%s", err, out)
	}
	config := strings.ToLower(string(out))
	if !strings.Contains(config, "hostname e286065f969386") || !strings.Contains(config, "__fly-proxy") {
		t.Fatalf("ssh -G output missing fly proxy:\n%s", out)
	}
}
