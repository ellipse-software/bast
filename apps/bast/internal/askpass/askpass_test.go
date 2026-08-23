package askpass

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bast/internal/hostpass"
	"bast/internal/sshconfig"
)

func TestApplyReplacesAskPassEnv(t *testing.T) {
	cmd := exec.Command("true")
	cmd.Env = []string{"FOO=bar", "SSH_ASKPASS=old", KindEnv + "=stale", IDEnv + "=old-id"}
	Apply(cmd, "/usr/bin/bast", KindHost, "abc123")
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, Env+"="+Value) {
		t.Fatalf("env = %s", joined)
	}
	if !strings.Contains(joined, KindEnv+"="+KindHost) || strings.Contains(joined, KindEnv+"=stale") {
		t.Fatalf("kind env = %s", joined)
	}
	if !strings.Contains(joined, IDEnv+"=abc123") || strings.Contains(joined, IDEnv+"=old-id") {
		t.Fatalf("id env = %s", joined)
	}
	if !strings.Contains(joined, "SSH_ASKPASS=/usr/bin/bast") || strings.Contains(joined, "SSH_ASKPASS=old") {
		t.Fatalf("askpass env = %s", joined)
	}
	if !strings.Contains(joined, "SSH_ASKPASS_REQUIRE=force") {
		t.Fatal("missing SSH_ASKPASS_REQUIRE")
	}
	if !strings.Contains(joined, "FOO=bar") {
		t.Fatal("lost unrelated env")
	}
}

func TestKindDefaultsToUpstash(t *testing.T) {
	t.Setenv(KindEnv, "")
	if got := Kind(); got != KindUpstash {
		t.Fatalf("Kind() = %q", got)
	}
}

func TestPrepareUsesHostPassword(t *testing.T) {
	dir := t.TempDir()
	if err := hostpass.Save(dir, "abc123", "secret"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("true")
	cmd.Env = []string{}
	Prepare(cmd, "/usr/bin/bast", sshconfig.Host{Managed: true, ManagedID: "abc123"}, dir)
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, KindEnv+"="+KindHost) || !strings.Contains(joined, IDEnv+"=abc123") {
		t.Fatalf("env = %s", joined)
	}
}

func TestPreparePrefersUpstash(t *testing.T) {
	dir := t.TempDir()
	if err := hostpass.Save(dir, "abc123", "secret"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("true")
	cmd.Env = []string{}
	Prepare(cmd, "/usr/bin/bast", sshconfig.Host{
		Managed: true, ManagedID: "abc123", Synced: true, SyncSource: "upstash",
	}, dir)
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, KindEnv+"="+KindUpstash) {
		t.Fatalf("env = %s", joined)
	}
	if strings.Contains(joined, IDEnv+"=") {
		t.Fatalf("upstash askpass should not set a host id: %s", joined)
	}
}

func TestNeeded(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "passwords")
	host := sshconfig.Host{Managed: true, ManagedID: "abc123"}
	if Needed(host, dir) {
		t.Fatal("missing password should not need askpass")
	}
	if err := hostpass.Save(dir, "abc123", "secret"); err != nil {
		t.Fatal(err)
	}
	if !Needed(host, dir) {
		t.Fatal("stored password should need askpass")
	}
	if !Needed(sshconfig.Host{Synced: true, SyncSource: "upstash"}, dir) {
		t.Fatal("upstash should need askpass")
	}
}

func TestIsRequest(t *testing.T) {
	t.Setenv(Env, "")
	if IsRequest() {
		t.Fatal("empty env should not be an askpass request")
	}
	t.Setenv(Env, Value)
	if !IsRequest() {
		t.Fatal("expected askpass request")
	}
}

func TestApplyAbsolutePathLookup(t *testing.T) {
	cmd := exec.Command("true")
	cmd.Env = []string{}
	Apply(cmd, "true", KindUpstash, "")
	found := false
	for _, item := range cmd.Env {
		name, value, _ := strings.Cut(item, "=")
		if name != "SSH_ASKPASS" {
			continue
		}
		found = true
		if !filepath.IsAbs(value) {
			t.Fatalf("SSH_ASKPASS is not absolute: %q", value)
		}
	}
	if !found {
		t.Fatal("missing SSH_ASKPASS")
	}
}

func TestHostIDTrims(t *testing.T) {
	t.Setenv(IDEnv, "  abc  ")
	if got := HostID(); got != "abc" {
		t.Fatalf("HostID() = %q", got)
	}
}

func TestPrepareSkipsUnrelatedHosts(t *testing.T) {
	cmd := exec.Command("true")
	cmd.Env = []string{"FOO=1"}
	Prepare(cmd, "/usr/bin/bast", sshconfig.Host{Managed: true, ManagedID: "missing"}, t.TempDir())
	if strings.Contains(strings.Join(cmd.Env, "\n"), Env+"=") {
		t.Fatalf("unexpected askpass env: %v", cmd.Env)
	}
}
