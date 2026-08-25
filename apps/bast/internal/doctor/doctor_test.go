package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
)

func testEngine(t *testing.T) (Engine, paths.Paths) {
	t.Helper()
	home := t.TempDir()
	p := paths.ForHome(home)
	if err := os.MkdirAll(p.SSHDir, 0700); err != nil {
		t.Fatal(err)
	}
	return New(p, openssh.Default(), "dev"), p
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func findingIDs(r Report) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.ID)
	}
	return out
}

func hasID(r Report, id string) bool {
	for _, f := range r.Findings {
		if f.ID == id {
			return true
		}
	}
	return false
}

func finding(r Report, id string) Finding {
	for _, f := range r.Findings {
		if f.ID == id {
			return f
		}
	}
	return Finding{}
}

func TestIncludeInsideHostBlock(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.ManagedConfig, "Host managed\n  HostName managed.example\n", 0600)
	write(t, p.MainConfig, "Host work\n  HostName work.example\n  Include ~/.ssh/bast/config\n", 0600)
	r := e.Run(context.Background(), Options{})
	if !hasID(r, "ssh_config.include_not_toplevel") {
		t.Fatalf("expected include_not_toplevel, got %v", findingIDs(r))
	}
	f := finding(r, "ssh_config.include_not_toplevel")
	if f.Severity != SeverityFail || !f.Fixable {
		t.Fatalf("finding = %+v", f)
	}
	if r.Healthy {
		t.Fatal("expected unhealthy report")
	}
}

func TestRelativeIdentityFile(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.MainConfig, "Host api\n  HostName api.example\n  IdentityFile id_rel\n", 0600)
	r := e.Run(context.Background(), Options{Categories: []string{"ssh_config"}})
	if !hasID(r, "ssh_config.identity_relative") {
		t.Fatalf("expected identity_relative, got %v", findingIDs(r))
	}
}

func TestMissingIdentityFile(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.MainConfig, "Host api\n  HostName api.example\n  IdentityFile ~/.ssh/missing_key\n", 0600)
	r := e.Run(context.Background(), Options{})
	if !hasID(r, "ssh_config.identity_missing") {
		t.Fatalf("expected identity_missing, got %v", findingIDs(r))
	}
	if finding(r, "ssh_config.identity_missing").Severity != SeverityFail {
		t.Fatal("identity_missing should be fail")
	}
}

func TestTooOpenPrivateKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	e, p := testEngine(t)
	key := filepath.Join(p.SSHDir, "id_test")
	write(t, key, "-----BEGIN OPENSSH PRIVATE KEY-----\ndata\n-----END OPENSSH PRIVATE KEY-----\n", 0644)
	write(t, p.MainConfig, "Host api\n  HostName api.example\n  IdentityFile ~/.ssh/id_test\n", 0600)
	r := e.Run(context.Background(), Options{})
	if !hasID(r, "permissions.private_key") {
		t.Fatalf("expected private_key permission fail, got %v", findingIDs(r))
	}
}

func TestDuplicateAliases(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.MainConfig, "Host prod\n  HostName first.example\nHost prod\n  HostName second.example\n", 0600)
	r := e.Run(context.Background(), Options{})
	if !hasID(r, "ssh_config.duplicate_alias") {
		t.Fatalf("expected duplicate_alias, got %v", findingIDs(r))
	}
	f := finding(r, "ssh_config.duplicate_alias")
	if f.Host != "prod" || f.Severity != SeverityWarn {
		t.Fatalf("finding = %+v", f)
	}
}

func TestCyclicIncludeStillReportsLaterHosts(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.MainConfig, "Include loop.conf\nHost after\n  HostName after.example\n", 0600)
	write(t, filepath.Join(p.SSHDir, "loop.conf"), "Include config\n", 0600)
	r := e.Run(context.Background(), Options{})
	if !hasID(r, "ssh_config.include_cycle") {
		t.Fatalf("expected include_cycle, got %v", findingIDs(r))
	}
}

func TestTooManyIdentitiesFromHostStar(t *testing.T) {
	e, p := testEngine(t)
	var b strings.Builder
	b.WriteString("Host *\n")
	for i := 0; i < 6; i++ {
		name := filepath.Join(p.SSHDir, "k"+string(rune('a'+i)))
		write(t, name, "-----BEGIN OPENSSH PRIVATE KEY-----\nx\n-----END OPENSSH PRIVATE KEY-----\n", 0600)
		b.WriteString("  IdentityFile ~/.ssh/k" + string(rune('a'+i)) + "\n")
	}
	b.WriteString("Host api\n  HostName api.example\n")
	write(t, p.MainConfig, b.String(), 0600)
	r := e.Run(context.Background(), Options{})
	if !hasID(r, "ssh_config.too_many_identities") {
		t.Fatalf("expected too_many_identities, got %v", findingIDs(r))
	}
}

func TestFixPrependsInclude(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.ManagedConfig, "Host managed\n  HostName managed.example\n", 0600)
	write(t, p.MainConfig, "Host work\n  HostName work.example\n  Include ~/.ssh/bast/config\n", 0600)
	r := e.Run(context.Background(), Options{Fix: true})
	if len(r.Fixed) == 0 {
		t.Fatalf("expected a repair, findings=%v", findingIDs(r))
	}
	main, err := os.ReadFile(p.MainConfig)
	if err != nil {
		t.Fatal(err)
	}
	text := string(main)
	if !strings.Contains(text, "Include ~/.ssh/bast/config") {
		t.Fatalf("include not present:\n%s", text)
	}
	if !strings.Contains(text, "# Added by Bast\nInclude ~/.ssh/bast/config") {
		t.Fatalf("top-level include missing:\n%s", text)
	}
	if !strings.Contains(text, "Host work") {
		t.Fatalf("user host was lost:\n%s", text)
	}
}

func TestCategoryFilter(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.MainConfig, "Host api\n  HostName api.example\n  IdentityFile ~/.ssh/missing_key\n", 0600)
	r := e.Run(context.Background(), Options{Categories: []string{"ssh_config"}})
	for _, f := range r.Findings {
		if f.Category != CatSSHConfig {
			t.Fatalf("unexpected category %s", f.Category)
		}
	}
	if !hasID(r, "ssh_config.identity_missing") {
		t.Fatalf("filter dropped identity_missing: %v", findingIDs(r))
	}
}

func TestBoxCLIFoundAtAsciiInstallWithoutPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX Box CLI fixture")
	}
	e, p := testEngine(t)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBox(metadata.BoxIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	boxBin := filepath.Join(p.Home, ".ascii", "bin", "box")
	write(t, boxBin, "#!/bin/sh\n", 0755)
	t.Setenv("HOME", p.Home)
	t.Setenv("BOX_CLI", "")
	t.Setenv("PATH", "/nonexistent")
	r := e.Run(context.Background(), Options{Categories: []string{"sync"}})
	if hasID(r, "sync.cli_missing") {
		t.Fatalf("Box CLI at ~/.ascii/bin/box should be found without PATH: %v", findingIDs(r))
	}
}

func TestBoxCLIMissingWhenEnabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX HOME/PATH isolation")
	}
	e, p := testEngine(t)
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBox(metadata.BoxIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", p.Home)
	t.Setenv("BOX_CLI", "")
	t.Setenv("PATH", "/nonexistent")
	r := e.Run(context.Background(), Options{Categories: []string{"sync"}})
	if !hasID(r, "sync.cli_missing") {
		t.Fatalf("expected box CLI missing, got %v", findingIDs(r))
	}
}

func TestFormatUsesTUIPaletteAndPlainPipes(t *testing.T) {
	r := Report{Findings: []Finding{
		{ID: "env.openssh_ok", Severity: SeverityOK, Category: CatEnv, Title: "ssh ok"},
		{ID: "ssh_config.include_not_toplevel", Severity: SeverityFail, Category: CatSSHConfig, Title: "Include is inside a Host block", Path: "/tmp/config", Line: 12, Detail: "Move the Include to the top.", Fix: "Move Include to the top.", Fixable: true},
	}}
	r.finalize()
	styled := render(r, 80)
	if !strings.Contains(styled, "38;2;139;92;246") {
		t.Fatalf("header/category should use TUI purple #8B5CF6:\n%q", styled)
	}
	if !strings.Contains(styled, "38;2;239;68;68") {
		t.Fatalf("fail should use TUI red #EF4444:\n%q", styled)
	}
	if !strings.Contains(styled, "38;2;16;185;129") {
		t.Fatalf("ok should use TUI green #10B981:\n%q", styled)
	}
	var buf strings.Builder
	if err := Format(&buf, r); err != nil {
		t.Fatal(err)
	}
	plain := buf.String()
	if strings.Contains(plain, "\x1b") {
		t.Fatalf("piped output should strip ANSI:\n%q", plain)
	}
	for _, want := range []string{"BAST", "doctor", "OpenSSH", "SSH config", "fail", "ok", "Include is inside a Host block", "/tmp/config:12", "Fix: Move Include to the top."} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q\n%s", want, plain)
		}
	}
}

func TestTopLevelIncludeIsNotReportedAsScoped(t *testing.T) {
	e, p := testEngine(t)
	write(t, p.MainConfig, "# Added by Bast\nInclude ~/.ssh/bast/config\n\nHost api\n  HostName api.example\n", 0600)
	write(t, p.ManagedConfig, "", 0600)
	r := e.Run(context.Background(), Options{Categories: []string{"ssh_config"}})
	if hasID(r, "ssh_config.include_not_toplevel") {
		t.Fatalf("false positive scoped include: %v", findingIDs(r))
	}
}
