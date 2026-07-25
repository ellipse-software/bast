package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testManager(t *testing.T) Manager {
	t.Helper()
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	return Manager{
		Home: home, MainConfig: filepath.Join(sshDir, "config"),
		ManagedDir: filepath.Join(sshDir, "bast"), ManagedConfig: filepath.Join(sshDir, "bast", "config"),
		ManagedKeys: filepath.Join(sshDir, "bast", "keys"),
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverIncludesAndLiteralAliases(t *testing.T) {
	m := testManager(t)
	writeTestFile(t, m.MainConfig, "# main\nHost prod *.wild !blocked\n  HostName prod.example\nInclude conf.d/*.conf\n", 0600)
	writeTestFile(t, filepath.Join(filepath.Dir(m.MainConfig), "conf.d", "team.conf"), "Host staging other\n  User deploy\n", 0600)
	hosts, err := m.Discover()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(hosts))
	for _, host := range hosts {
		got = append(got, host.Alias)
	}
	if strings.Join(got, ",") != "other,prod,staging" {
		t.Fatalf("aliases = %v", got)
	}
}

func TestDiscoverRejectsIncludeCycle(t *testing.T) {
	m := testManager(t)
	writeTestFile(t, m.MainConfig, "Include loop.conf\n", 0600)
	writeTestFile(t, filepath.Join(filepath.Dir(m.MainConfig), "loop.conf"), "Include config\n", 0600)
	if _, err := m.Discover(); err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestManagedHostLifecyclePreservesExternalConfig(t *testing.T) {
	m := testManager(t)
	original := "# hand-written config\nHost legacy\n  HostName old.example\n"
	writeTestFile(t, m.MainConfig, original, 0640)
	host, err := m.Add(HostInput{Alias: "prod", HostName: "prod.example", User: "deploy", Port: "2222", IdentityFile: "~/.ssh/bast/keys/work", ExtraOptions: []string{"IdentitiesOnly yes"}})
	if err != nil {
		t.Fatal(err)
	}
	main, _ := os.ReadFile(m.MainConfig)
	if strings.Count(string(main), "Include ~/.ssh/bast/config") != 1 || !strings.Contains(string(main), original) {
		t.Fatalf("main config was not preserved:\n%s", main)
	}
	info, _ := os.Stat(m.MainConfig)
	if info.Mode().Perm() != 0640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	hosts, err := m.Discover()
	if err != nil {
		t.Fatal(err)
	}
	var managed Host
	for _, item := range hosts {
		if item.Alias == "prod" {
			managed = item
		}
	}
	if !managed.Managed || managed.ManagedID != host.ManagedID {
		t.Fatalf("managed host not recognized: %+v", managed)
	}
	if err := m.Update(host.ManagedID, HostInput{Alias: "production", HostName: "new.example"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(host.ManagedID); err != nil {
		t.Fatal(err)
	}
	managedData, _ := os.ReadFile(m.ManagedConfig)
	if len(managedData) != 0 {
		t.Fatalf("managed config not empty: %q", managedData)
	}
	main, _ = os.ReadFile(m.MainConfig)
	if strings.Count(string(main), "Include ~/.ssh/bast/config") != 1 {
		t.Fatalf("include duplicated:\n%s", main)
	}
}

func TestManagedAliasesCannotCollide(t *testing.T) {
	m := testManager(t)
	writeTestFile(t, m.MainConfig, "Host external\n  HostName x\n", 0600)
	if _, err := m.Add(HostInput{Alias: "external", HostName: "different"}); err == nil {
		t.Fatal("expected add collision")
	}
	first, err := m.Add(HostInput{Alias: "one", HostName: "one.example"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(HostInput{Alias: "two", HostName: "two.example"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(first.ManagedID, HostInput{Alias: "two", HostName: "collision"}); err == nil {
		t.Fatal("expected update collision")
	}
}

func TestConditionalIncludeDoesNotSuppressTopLevelInclude(t *testing.T) {
	m := testManager(t)
	writeTestFile(t, m.MainConfig, "Host only-this-one\n  Include ~/.ssh/bast/config\n", 0600)
	if err := m.EnsureManaged(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(m.MainConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(b), "Include ~/.ssh/bast/config") != 2 || !strings.HasPrefix(string(b), "# Added by Bast") {
		t.Fatalf("expected a new top-level include:\n%s", b)
	}
}

func TestValidateHost(t *testing.T) {
	bad := []HostInput{
		{Alias: "-oProxyCommand=x", HostName: "x"},
		{Alias: "*.example", HostName: "x"},
		{Alias: "ok", HostName: "x", Port: "70000"},
		{Alias: "ok", HostName: "x\nHost evil"},
		{Alias: "ok", HostName: "two hosts"},
		{Alias: "ok", HostName: "x", User: "two users"},
		{Alias: "ok", HostName: "x", ProxyJump: "first second"},
	}
	for _, input := range bad {
		if Validate(input) == nil {
			t.Fatalf("expected invalid: %+v", input)
		}
	}
	if err := Validate(HostInput{Alias: "good", HostName: "example.com", Port: "22"}); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAliasUsesUnderscoresForWhitespace(t *testing.T) {
	if got := NormalizeAlias("  Production   web server  "); got != "Production_web_server" {
		t.Fatalf("NormalizeAlias() = %q", got)
	}
}

func TestRenderBlockQuotesIdentityFile(t *testing.T) {
	identity := `~/.ssh/Work Keys/deploy "primary"`
	block := string(renderBlock("test", HostInput{Alias: "prod", HostName: "prod.example", IdentityFile: identity}))
	if !strings.Contains(block, `IdentityFile "~/.ssh/Work Keys/deploy \"primary\""`) {
		t.Fatalf("identity file was not safely quoted:\n%s", block)
	}
	for _, line := range strings.Split(block, "\n") {
		if !strings.Contains(line, "IdentityFile") {
			continue
		}
		parts, err := fields(strings.TrimSpace(line))
		if err != nil || len(parts) != 2 || parts[1] != identity {
			t.Fatalf("quoted identity file did not round trip: parts=%q err=%v", parts, err)
		}
	}
}

func TestRenderBlockSupportsPasswordOnlyAuthentication(t *testing.T) {
	block := string(renderBlock("test", HostInput{Alias: "legacy", HostName: "legacy.example", PasswordOnly: true}))
	for _, directive := range []string{
		"PubkeyAuthentication no",
		"PasswordAuthentication yes",
		"PreferredAuthentications keyboard-interactive,password",
	} {
		if !strings.Contains(block, directive) {
			t.Fatalf("password-only block is missing %q:\n%s", directive, block)
		}
	}
	if strings.Contains(block, "IdentityFile") {
		t.Fatalf("password-only block unexpectedly contains an identity file:\n%s", block)
	}
}

func TestRenderBlockWritesExtraOptions(t *testing.T) {
	block := string(renderBlock("test", HostInput{Alias: "prod", HostName: "prod.example", ExtraOptions: []string{"IdentitiesOnly yes", "ForwardAgent yes"}}))
	for _, directive := range []string{"IdentitiesOnly yes", "ForwardAgent yes"} {
		if !strings.Contains(block, directive) {
			t.Fatalf("block is missing %q:\n%s", directive, block)
		}
	}
}

func TestParseSSHFlags(t *testing.T) {
	got := ParseSSHFlags("IdentitiesOnly yes; ForwardAgent yes")
	if len(got) != 2 || got[0] != "IdentitiesOnly yes" || got[1] != "ForwardAgent yes" {
		t.Fatalf("ParseSSHFlags() = %#v", got)
	}
	got = ParseSSHFlags("IdentitiesOnly yes\nForwardAgent yes")
	if len(got) != 2 {
		t.Fatalf("ParseSSHFlags(newline) = %#v", got)
	}
}

func TestValidateExtraOptionsRejectsManagedAndForbiddenDirectives(t *testing.T) {
	bad := [][]string{
		{"HostName evil.example"},
		{"ProxyCommand /bin/sh"},
		{""},
	}
	for _, options := range bad {
		if err := validateExtraOptions(options); err == nil {
			t.Fatalf("expected invalid: %#v", options)
		}
	}
	if err := validateExtraOptions([]string{"IdentitiesOnly yes", "TCPKeepAlive yes"}); err != nil {
		t.Fatal(err)
	}
}

func TestExtractBlockExtras(t *testing.T) {
	block := renderBlock("abc123", HostInput{
		Alias: "prod", HostName: "prod.example", User: "deploy",
		IdentityFile: "~/.ssh/work", ExtraOptions: []string{"IdentitiesOnly yes", "ForwardAgent yes"},
	})
	extras := extractBlockExtras(block, "abc123")
	if len(extras) != 2 || extras[0] != "IdentitiesOnly yes" || extras[1] != "ForwardAgent yes" {
		t.Fatalf("extractBlockExtras() = %#v", extras)
	}
}

func TestManagedExtras(t *testing.T) {
	m := testManager(t)
	host, err := m.Add(HostInput{Alias: "prod", HostName: "prod.example", ExtraOptions: []string{"ForwardAgent yes"}})
	if err != nil {
		t.Fatal(err)
	}
	extras, err := m.ManagedExtras(host.ManagedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(extras) != 1 || extras[0] != "ForwardAgent yes" {
		t.Fatalf("ManagedExtras() = %#v", extras)
	}
}
