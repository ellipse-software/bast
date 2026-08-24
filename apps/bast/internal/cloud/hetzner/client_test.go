package hetzner

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	prevProbe := probeSSHPort
	probeSSHPort = func(context.Context, string, int) probeResult { return probeUnknown }
	t.Cleanup(func() { probeSSHPort = prevProbe })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	keyFile := filepath.Join(t.TempDir(), "hetzner-api-token")
	if err := WriteKeyFile(keyFile, "token-test"); err != nil {
		t.Fatal(err)
	}
	return &Client{
		BaseURL:  server.URL,
		KeyFile:  keyFile,
		PollWait: time.Millisecond,
		HTTP:     server.Client(),
		Getenv:   func(string) string { return "" },
	}
}

func TestDiscoverMapsRunningAndOff(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-test" {
			t.Errorf("auth = %q", got)
		}
		switch {
		case r.URL.Path == "/servers" && r.URL.Query().Get("page") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{
					serverJSON(42, "web", "running", "1.2.3.4", "", "ubuntu", "ubuntu-24.04", "fsn1", "cx22", []int{7}),
					serverJSON(43, "db", "off", "1.2.3.5", "", "debian", "debian-12", "nbg1", "cx22", nil),
					serverJSON(44, "win", "running", "1.2.3.6", "", "windows", "windows-2022", "fsn1", "cx22", nil),
					serverJSON(45, "gone", "deleting", "1.2.3.7", "", "ubuntu", "ubuntu-24.04", "fsn1", "cx22", nil),
				},
				"meta": map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil, "last_page": 1}},
			})
		case r.URL.Path == "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ssh_keys": []any{map[string]any{"id": 7, "name": "laptop", "public_key": "ssh-ed25519 AAAAtest comment"}},
				"meta":     map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %d want 2: %+v", len(discovery.Instances), discovery.Instances)
	}
	if discovery.Instances[0].SyncID != "hetzner/42" || discovery.Instances[0].HostName != "1.2.3.4" || discovery.Instances[0].User != "root" {
		t.Fatalf("running = %+v", discovery.Instances[0])
	}
	if !discovery.Instances[0].Running || discovery.Instances[0].State != "running" {
		t.Fatalf("running state = %+v", discovery.Instances[0])
	}
	if discovery.Instances[1].SyncID != "hetzner/43" || discovery.Instances[1].Running {
		t.Fatalf("off = %+v", discovery.Instances[1])
	}
	if !HostLooksStopped(discovery.Instances[1].Tags) {
		t.Fatal("off server should look stopped")
	}
	if AliasFor(discovery.Instances[0]) != "hetzner_default_fsn1_web" {
		t.Fatalf("alias = %s", AliasFor(discovery.Instances[0]))
	}
	if GroupPath(discovery.Instances[0]) != "Hetzner Cloud/default/fsn1" {
		t.Fatalf("group = %s", GroupPath(discovery.Instances[0]))
	}
}

func TestDiscoverIPv6AndPrivate(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/servers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{
					map[string]any{
						"id": 10, "name": "v6only", "status": "running",
						"public_net": map[string]any{
							"ipv4": nil,
							"ipv6": map[string]any{"ip": "2001:db8:1234::/64", "blocked": false},
						},
						"private_net": []any{},
						"server_type": map[string]any{"name": "cx22"},
						"datacenter":  map[string]any{"name": "fsn1-dc14", "location": map[string]any{"name": "fsn1"}},
						"image":       map[string]any{"name": "ubuntu-24.04", "os_flavor": "ubuntu"},
						"ssh_keys":    []any{},
						"labels":      map[string]any{},
					},
					map[string]any{
						"id": 11, "name": "priv", "status": "off",
						"public_net":  map[string]any{"ipv4": nil, "ipv6": nil},
						"private_net": []any{map[string]any{"ip": "10.0.0.8"}},
						"server_type": map[string]any{"name": "cx22"},
						"datacenter":  map[string]any{"name": "hel1-dc2", "location": map[string]any{"name": "hel1"}},
						"image":       map[string]any{"name": "debian-12", "os_flavor": "debian"},
						"ssh_keys":    []any{},
						"labels":      map[string]any{},
					},
				},
				"meta": map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil}},
			})
		case "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_keys": []any{}, "meta": map[string]any{"pagination": map[string]any{"page": 1}}})
		default:
			http.NotFound(w, r)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
	if discovery.Instances[0].HostName != "2001:db8:1234::1" || !discovery.Instances[0].Public {
		t.Fatalf("v6 = %+v", discovery.Instances[0])
	}
	if discovery.Instances[1].HostName != "10.0.0.8" || discovery.Instances[1].Public {
		t.Fatalf("private = %+v", discovery.Instances[1])
	}
}

func TestDiscoverPaginationAndIdentity(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pubText := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: priv})
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), block, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519.pub"), []byte(pubText+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/servers" && r.URL.Query().Get("page") == "1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{serverJSON(1, "a", "running", "1.1.1.1", "", "ubuntu", "ubuntu", "fsn1", "cx22", []int{9})},
				"meta":    map[string]any{"pagination": map[string]any{"page": 1, "next_page": 2, "last_page": 2}},
			})
		case r.URL.Path == "/servers" && r.URL.Query().Get("page") == "2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{serverJSON(2, "b", "running", "1.1.1.2", "", "ubuntu", "ubuntu", "fsn1", "cx22", nil)},
				"meta":    map[string]any{"pagination": map[string]any{"page": 2, "next_page": nil, "last_page": 2}},
			})
		case r.URL.Path == "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ssh_keys": []any{map[string]any{"id": 9, "name": "mine", "public_key": pubText}},
				"meta":     map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %d", len(discovery.Instances))
	}
	if discovery.Instances[0].IdentityFile == "" || !discovery.Instances[0].IdentitiesOnly {
		t.Fatalf("expected identity match: %+v", discovery.Instances[0])
	}
}

func TestDiscover401(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "unable to authenticate"}})
	})
	_, err := client.Discover(context.Background(), DiscoverConfig{})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestStartStopRestart(t *testing.T) {
	status := "off"
	rebootPolls := 0
	actions := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/servers/42":
			current := status
			if status == "rebooting" {
				rebootPolls++
				if rebootPolls == 1 {
					current = "stopping"
				} else {
					current = "running"
					status = "running"
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"server": serverJSON(42, "web", current, "1.2.3.4", "", "ubuntu", "ubuntu", "fsn1", "cx22", nil)})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/poweron"):
			status = "running"
			actions++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"action": map[string]any{"id": 1, "status": "running"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/shutdown"):
			status = "off"
			actions++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"action": map[string]any{"id": 2, "status": "running"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/reboot"):
			status = "rebooting"
			rebootPolls = 0
			actions++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"action": map[string]any{"id": 3, "status": "running"}})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/actions/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"action": map[string]any{"id": 1, "status": "success"}})
		default:
			http.NotFound(w, r)
		}
	})
	if err := client.Start(context.Background(), "hetzner/42"); err != nil {
		t.Fatal(err)
	}
	if err := client.Stop(context.Background(), "hetzner/42", false); err != nil {
		t.Fatal(err)
	}
	status = "running"
	if err := client.Restart(context.Background(), "hetzner/42", false); err != nil {
		t.Fatal(err)
	}
	if actions != 3 {
		t.Fatalf("actions = %d", actions)
	}
}

func TestRestartWhileOff(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/servers/42" {
			_ = json.NewEncoder(w).Encode(map[string]any{"server": serverJSON(42, "web", "off", "1.2.3.4", "", "ubuntu", "ubuntu", "fsn1", "cx22", nil)})
			return
		}
		http.Error(w, "no", 500)
	})
	err := client.Restart(context.Background(), "hetzner/42", false)
	if err == nil || !strings.Contains(err.Error(), "off") {
		t.Fatalf("err = %v", err)
	}
}

func TestLifecycleForbidden(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/servers/42" {
			_ = json.NewEncoder(w).Encode(map[string]any{"server": serverJSON(42, "web", "off", "1.2.3.4", "", "ubuntu", "ubuntu", "fsn1", "cx22", nil)})
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "insufficient permissions"}})
	})
	err := client.Start(context.Background(), "hetzner/42")
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "Read & Write") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseHCloudTOML(t *testing.T) {
	got := parseHCloudTOML([]byte(`
active_context = "prod"

[contexts.prod]
token = "aaa"

[contexts.staging]
token = "bbb"

[[contexts]]
name = "array"
token = "ccc"
`))
	if len(got) != 3 {
		t.Fatalf("got = %+v", got)
	}
	names := map[string]string{}
	for _, item := range got {
		names[item.Name] = item.Token
	}
	if names["prod"] != "aaa" || names["staging"] != "bbb" || names["array"] != "ccc" {
		t.Fatalf("names = %+v", names)
	}
}

func TestTokenContextsDedup(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "cli.toml")
	if err := os.WriteFile(cfg, []byte("[contexts.prod]\ntoken = \"same\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(home, "token")
	if err := WriteKeyFile(keyFile, "same"); err != nil {
		t.Fatal(err)
	}
	client := &Client{
		KeyFile:    keyFile,
		Home:       home,
		ConfigPath: cfg,
		Getenv: func(key string) string {
			if key == APIKeyEnv {
				return "same"
			}
			if key == ContextEnv {
				return "prod"
			}
			return ""
		},
	}
	contexts, err := client.TokenContexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 1 || contexts[0].Name != "prod" || contexts[0].Source != "env" {
		t.Fatalf("contexts = %+v", contexts)
	}
}

func TestEnsureAccessPrivateAllowed(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/servers/11":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"server": map[string]any{
					"id": 11, "name": "priv", "status": "running",
					"public_net":  map[string]any{"ipv4": nil, "ipv6": nil},
					"private_net": []any{map[string]any{"ip": "10.0.0.8"}},
					"server_type": map[string]any{"name": "cx22"},
					"datacenter":  map[string]any{"location": map[string]any{"name": "hel1"}},
					"image":       map[string]any{"os_flavor": "debian"},
					"ssh_keys":    []any{},
				},
			})
		case "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_keys": []any{}, "meta": map[string]any{"pagination": map[string]any{"page": 1}}})
		default:
			http.NotFound(w, r)
		}
	})
	result, err := client.EnsureAccess(context.Background(), "hetzner/11", EnsureConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if result.HostName != "10.0.0.8" || result.Public {
		t.Fatalf("result = %+v", result)
	}
}

func TestPreferPrivateIP(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/servers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{
					map[string]any{
						"id": 20, "name": "vpn", "status": "running",
						"public_net":  map[string]any{"ipv4": map[string]any{"ip": "203.0.113.9", "blocked": false}, "ipv6": nil},
						"private_net": []any{map[string]any{"ip": "10.0.0.20"}},
						"server_type": map[string]any{"name": "cx22"},
						"datacenter":  map[string]any{"location": map[string]any{"name": "fsn1"}},
						"image":       map[string]any{"os_flavor": "ubuntu"},
						"ssh_keys":    []any{},
						"labels":      map[string]any{},
					},
				},
				"meta": map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil}},
			})
		case "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_keys": []any{}, "meta": map[string]any{"pagination": map[string]any{"page": 1}}})
		default:
			http.NotFound(w, r)
		}
	})
	publicFirst, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if publicFirst.Instances[0].HostName != "203.0.113.9" || !publicFirst.Instances[0].Public {
		t.Fatalf("public first = %+v", publicFirst.Instances[0])
	}
	privateFirst, err := client.Discover(context.Background(), DiscoverConfig{PreferPrivateIP: true})
	if err != nil {
		t.Fatal(err)
	}
	if privateFirst.Instances[0].HostName != "10.0.0.20" || privateFirst.Instances[0].Public {
		t.Fatalf("private first = %+v", privateFirst.Instances[0])
	}
}

func TestNamedTokens(t *testing.T) {
	dir := t.TempDir()
	client := &Client{
		TokenDir: filepath.Join(dir, "tokens"),
		KeyFile:  filepath.Join(dir, "legacy"),
		Getenv:   func(string) string { return "" },
	}
	if err := client.SaveNamedToken("prod", "token-prod"); err != nil {
		t.Fatal(err)
	}
	if err := client.SaveNamedToken("staging", "token-staging"); err != nil {
		t.Fatal(err)
	}
	got, err := client.TokenContexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got = %+v", got)
	}
	names := map[string]string{}
	for _, item := range got {
		names[item.Name] = item.Token
	}
	if names["prod"] != "token-prod" || names["staging"] != "token-staging" {
		t.Fatalf("names = %+v", names)
	}
	if err := client.DeleteNamedToken("prod"); err != nil {
		t.Fatal(err)
	}
	got, err = client.TokenContexts()
	if err != nil || len(got) != 1 || got[0].Name != "staging" {
		t.Fatalf("after delete = %+v err=%v", got, err)
	}
}

func TestMigrateLegacyToken(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "hetzner-api-token")
	if err := WriteKeyFile(legacy, "legacy-token"); err != nil {
		t.Fatal(err)
	}
	client := &Client{
		KeyFile:  legacy,
		TokenDir: filepath.Join(dir, "tokens"),
		Getenv:   func(string) string { return "" },
	}
	got, err := client.TokenContexts()
	if err != nil || len(got) != 1 || got[0].Name != "default" || got[0].Token != "legacy-token" {
		t.Fatalf("got = %+v err=%v", got, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy token file should be moved")
	}
}

func TestDiscoverUsesTopLevelLocationAndLabels(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/servers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{
					map[string]any{
						"id": 107, "name": "vpn", "status": "running",
						"public_net":  map[string]any{"ipv4": map[string]any{"ip": "168.119.235.95", "blocked": false}, "ipv6": nil},
						"private_net": []any{},
						"server_type": map[string]any{"name": "cx22"},
						"location":    map[string]any{"name": "nbg1"},
						"datacenter":  nil,
						"image":       map[string]any{"name": "ubuntu-24.04", "os_flavor": "ubuntu"},
						"ssh_keys":    nil,
						"labels":      map[string]any{"ssh-port": "2022"},
					},
				},
				"meta": map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil}},
			})
		case "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_keys": []any{}, "meta": map[string]any{"pagination": map[string]any{"page": 1}}})
		default:
			http.NotFound(w, r)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 1 {
		t.Fatalf("instances = %+v", discovery.Instances)
	}
	inst := discovery.Instances[0]
	if inst.Location != "nbg1" || inst.Port != "2022" || AliasFor(inst) != "hetzner_default_nbg1_vpn" {
		t.Fatalf("inst = %+v alias=%s", inst, AliasFor(inst))
	}
	if GroupPath(inst) != "Hetzner Cloud/default/nbg1" {
		t.Fatalf("group = %s", GroupPath(inst))
	}
}

func TestDiscoverMatchesProjectKeyWithoutIdentitiesOnly(t *testing.T) {
	home := t.TempDir()
	keysDir := filepath.Join(home, ".ssh", "bast", "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFApGA+VaN3ebkgMflY+CZXZdlrwwXLZMvCEFlCcFUYa ted.ac"
	if err := os.WriteFile(filepath.Join(keysDir, "ted.ac"), []byte("dummy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keysDir, "ted.ac.pub"), []byte(pub+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/servers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{serverJSON(42, "web", "running", "1.2.3.4", "", "ubuntu", "ubuntu-24.04", "fsn1", "cx22", nil)},
				"meta":    map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil}},
			})
		case "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ssh_keys": []any{map[string]any{"id": 7, "name": "t3d.uk", "public_key": pub}},
				"meta":     map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil}},
			})
		default:
			http.NotFound(w, r)
		}
	})
	discovery, err := client.Discover(context.Background(), DiscoverConfig{Home: home, ManagedKeys: keysDir})
	if err != nil {
		t.Fatal(err)
	}
	inst := discovery.Instances[0]
	if !strings.Contains(inst.IdentityFile, "ted.ac") {
		t.Fatalf("identity = %q", inst.IdentityFile)
	}
	if inst.IdentitiesOnly {
		t.Fatal("project-key fallback should not set IdentitiesOnly")
	}
}

func TestEnsureAccessKeepsCurrentUserAndDetectsPort(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/servers/11":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"server": map[string]any{
					"id": 11, "name": "vpn", "status": "running",
					"public_net":  map[string]any{"ipv4": map[string]any{"ip": "168.119.235.95", "blocked": false}, "ipv6": nil},
					"private_net": []any{},
					"server_type": map[string]any{"name": "cx22"},
					"location":    map[string]any{"name": "nbg1"},
					"image":       map[string]any{"os_flavor": "ubuntu"},
					"ssh_keys":    nil,
				},
			})
		case "/ssh_keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"ssh_keys": []any{}, "meta": map[string]any{"pagination": map[string]any{"page": 1}}})
		default:
			http.NotFound(w, r)
		}
	})
	probeSSHPort = func(_ context.Context, _ string, port int) probeResult {
		if port == 2022 {
			return probeSSHBanner
		}
		if port == 22 {
			return probeRefused
		}
		return probeUnknown
	}
	result, err := client.EnsureAccess(context.Background(), "hetzner/11", EnsureConfig{CurrentUser: "ted"})
	if err != nil {
		t.Fatal(err)
	}
	if result.User != "ted" || result.Port != "2022" || result.HostName != "168.119.235.95" {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseSyncID(t *testing.T) {
	id, err := ParseSyncID("hetzner/42")
	if err != nil || id != 42 {
		t.Fatalf("id = %d err = %v", id, err)
	}
	if _, err := ParseSyncID("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func serverJSON(id int, name, status, ipv4, ipv6, flavor, image, location, typ string, keys []int) map[string]any {
	var keyIDs []any
	for _, k := range keys {
		keyIDs = append(keyIDs, k)
	}
	pub := map[string]any{}
	if ipv4 != "" {
		pub["ipv4"] = map[string]any{"ip": ipv4, "blocked": false}
	} else {
		pub["ipv4"] = nil
	}
	if ipv6 != "" {
		pub["ipv6"] = map[string]any{"ip": ipv6, "blocked": false}
	} else {
		pub["ipv6"] = nil
	}
	return map[string]any{
		"id": id, "name": name, "status": status,
		"public_net":  pub,
		"private_net": []any{},
		"server_type": map[string]any{"name": typ},
		"datacenter":  map[string]any{"name": location + "-dc", "location": map[string]any{"name": location}},
		"image":       map[string]any{"name": image, "os_flavor": flavor},
		"ssh_keys":    keyIDs,
		"labels":      map[string]any{"env": "test"},
	}
}

func TestIPv6Host(t *testing.T) {
	if got := ipv6Host("2001:db8:1234::/64"); got != "2001:db8:1234::1" {
		t.Fatalf("got %q", got)
	}
	if got := ipv6Host("2001:db8::1"); got != "2001:db8::1" {
		t.Fatalf("got %q", got)
	}
}
