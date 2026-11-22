package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func TestImportConfigForwardsMovesManagedDirectives(t *testing.T) {
	mgr, store := importFixture(t)
	if _, err := mgr.Add(sshconfig.HostInput{
		Alias: "web", HostName: "web.example", User: "deploy",
		ExtraOptions: []string{
			"ForwardAgent yes",
			"LocalForward 8080 localhost:80",
			"RemoteForward 9000 localhost:3000",
			"DynamicForward 1080",
			"DynamicForward 1081",
			"ServerAliveInterval 30",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutTunnel(metadata.Tunnel{
		Name: "already", Host: "web", Mode: ModeLocal, Local: "8080", Remote: "localhost:80",
	}, ""); err != nil {
		t.Fatal(err)
	}

	result, err := ImportConfigForwards(mgr, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 3 || result.Updated != 1 || len(result.Migrated) != 1 || result.Migrated[0] != "web" {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Notice(), "Moved 4 forwards") || !strings.Contains(result.Notice(), "already connected") {
		t.Fatalf("notice = %q", result.Notice())
	}

	config, err := os.ReadFile(mgr.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	for _, gone := range []string{"LocalForward", "RemoteForward", "DynamicForward"} {
		if strings.Contains(text, gone) {
			t.Fatalf("config still has %s:\n%s", gone, text)
		}
	}
	for _, kept := range []string{"ForwardAgent yes", "ServerAliveInterval 30", "HostName web.example", "User deploy"} {
		if !strings.Contains(text, kept) {
			t.Fatalf("config missing %q:\n%s", kept, text)
		}
	}

	got := map[string]metadata.Tunnel{}
	for _, item := range store.Tunnels() {
		got[item.Name] = item
	}
	already, ok := got["already"]
	if !ok || !already.OpenWithHost {
		t.Fatalf("existing tunnel = %+v", already)
	}
	for _, name := range []string{"web-9000", "web-1080", "web-1081"} {
		item, ok := got[name]
		if !ok || !item.OpenWithHost || item.Host != "web" {
			t.Fatalf("tunnel %s = %+v ok=%v", name, item, ok)
		}
	}
	if got["web-9000"].Mode != ModeRemote || got["web-9000"].Local != "localhost:3000" || got["web-9000"].Remote != "9000" {
		t.Fatalf("remote = %+v", got["web-9000"])
	}
	if got["web-1080"].Mode != ModeSOCKS || got["web-1081"].Mode != ModeSOCKS {
		t.Fatalf("socks = %+v %+v", got["web-1080"], got["web-1081"])
	}

	again, err := ImportConfigForwards(mgr, store)
	if err != nil || again.Created != 0 || again.Notice() != "" {
		t.Fatalf("second import = %+v err=%v", again, err)
	}
}

func TestImportConfigForwardsLeavesAnInvalidHost(t *testing.T) {
	mgr, store := importFixture(t)
	if _, err := mgr.Add(sshconfig.HostInput{
		Alias: "odd", HostName: "odd.example",
		ExtraOptions: []string{"LocalForward 8080 localhost:80", "LocalForward only-one-side"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Add(sshconfig.HostInput{
		Alias: "ok", HostName: "ok.example",
		ExtraOptions: []string{"DynamicForward 1080"},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := ImportConfigForwards(mgr, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || len(result.Problems) != 1 || !strings.Contains(result.Problems[0], "odd") {
		t.Fatalf("result = %+v", result)
	}
	config, err := os.ReadFile(mgr.ManagedConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(config)
	if !strings.Contains(text, "LocalForward 8080 localhost:80") || !strings.Contains(text, "LocalForward only-one-side") {
		t.Fatalf("invalid host was rewritten:\n%s", text)
	}
	if strings.Contains(text, "DynamicForward") {
		t.Fatalf("valid host was not stripped:\n%s", text)
	}
	if _, ok := store.Tunnel("ok-1080"); !ok {
		t.Fatalf("tunnels = %+v", store.Tunnels())
	}
}

func TestTunnelImportNameAvoidsADifferentEndpoint(t *testing.T) {
	existing := []metadata.Tunnel{{
		Name: "web-8080", Host: "web", Mode: ModeSOCKS, Local: "8080",
	}}
	got := tunnelImportName(metadata.Tunnel{Host: "web", Mode: ModeLocal, Local: "8080", Remote: "localhost:80"}, existing)
	if got != "web-local-8080" {
		t.Fatalf("name = %q", got)
	}
	existing = append(existing, metadata.Tunnel{Name: got})
	got = tunnelImportName(metadata.Tunnel{Host: "web", Mode: ModeLocal, Local: "127.0.0.1:8080", Remote: "localhost:80"}, existing)
	if got != "web-127.0.0.1-8080" {
		t.Fatalf("address name = %q", got)
	}
}

func importFixture(t *testing.T) (sshconfig.Manager, *metadata.Store) {
	t.Helper()
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	mgr := sshconfig.Manager{
		Home: home, MainConfig: filepath.Join(sshDir, "config"),
		ManagedDir: filepath.Join(sshDir, "bast"), ManagedConfig: filepath.Join(sshDir, "bast", "config"),
		ManagedKeys: filepath.Join(sshDir, "bast", "keys"),
	}
	store, err := metadata.Open(filepath.Join(home, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return mgr, store
}
