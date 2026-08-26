package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bast/internal/hostpass"
	"bast/internal/openssh"
	"bast/internal/paths"
)

func TestHostCommandsRoundTripWithJSON(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)

	out, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "add", "Production web", "--hostname", "prod.example", "--user", "deploy", "--group", "Work/Production", "--tag", "web")
	if err != nil || errOut != "" || !strings.Contains(out, `"alias":"Production_web"`) {
		t.Fatalf("add output=%q stderr=%q err=%v", out, errOut, err)
	}
	managed, err := os.ReadFile(filepath.Join(home, ".ssh", "bast", "config"))
	if err != nil || !strings.Contains(string(managed), "Host Production_web") || !strings.Contains(string(managed), "HostName prod.example") {
		t.Fatalf("managed config=%q err=%v", managed, err)
	}

	out, errOut, err = runTestCLI(t, home, client, "hosts", "edit", "Production_web", "--notes", "Primary server", "--clear-group", "--json")
	if err != nil || errOut != "" || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("edit output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "favorite", "Production web")
	if err != nil || !strings.Contains(out, `"favorite":true`) {
		t.Fatalf("favorite output=%q err=%v", out, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "Production_web")
	if err != nil || !strings.Contains(out, `"notes":"Primary server"`) || !strings.Contains(out, `"group":""`) {
		t.Fatalf("show output=%q err=%v", out, err)
	}

	_, errOut, err = runTestCLI(t, home, client, "--json", "hosts", "delete", "Production_web")
	if code, ok := ExitCode(err); !ok || code != 1 || !strings.Contains(errOut, `"code":"confirmation_required"`) {
		t.Fatalf("delete confirmation stderr=%q err=%v code=%d ok=%t", errOut, err, code, ok)
	}
	out, errOut, err = runTestCLI(t, home, client, "--json", "hosts", "delete", "Production_web", "--yes")
	if err != nil || errOut != "" || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("delete output=%q stderr=%q err=%v", out, errOut, err)
	}
}

func TestHostPasswordStorageAndJSON(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	out, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "add", "legacy", "--hostname", "legacy.example", "--password-only")
	if err != nil || errOut != "" {
		t.Fatalf("add output=%q stderr=%q err=%v", out, errOut, err)
	}
	managed, err := os.ReadFile(filepath.Join(home, ".ssh", "bast", "config"))
	if err != nil || !strings.Contains(string(managed), "PubkeyAuthentication no") {
		t.Fatalf("managed config=%q err=%v", managed, err)
	}
	if strings.Contains(string(managed), "s3cret") {
		t.Fatal("password leaked into SSH config")
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "legacy")
	if err != nil || strings.Contains(out, `"passwordStored":true`) || strings.Contains(out, "s3cret") {
		t.Fatalf("show before store = %q err=%v", out, err)
	}

	id := managedHostID(t, managed)
	if err := hostpass.Save(filepath.Join(home, ".config", "bast", "passwords"), id, "s3cret"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "legacy")
	if err != nil || !strings.Contains(out, `"passwordStored":true`) || strings.Contains(out, "s3cret") {
		t.Fatalf("show after store = %q err=%v", out, err)
	}

	_, errOut, err = runTestCLI(t, home, client, "--json", "hosts", "edit", "legacy", "--clear-password")
	if err != nil || errOut != "" {
		t.Fatalf("clear-password stderr=%q err=%v", errOut, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "legacy")
	if err != nil || strings.Contains(out, `"passwordStored":true`) {
		t.Fatalf("show after clear = %q err=%v", out, err)
	}

	if err := hostpass.Save(filepath.Join(home, ".config", "bast", "passwords"), id, "s3cret"); err != nil {
		t.Fatal(err)
	}
	_, errOut, err = runTestCLI(t, home, client, "--json", "hosts", "delete", "legacy", "--yes")
	if err != nil || errOut != "" {
		t.Fatalf("delete stderr=%q err=%v", errOut, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "bast", "passwords", id)); !os.IsNotExist(err) {
		t.Fatalf("password file survived delete: %v", err)
	}

	_, errOut, err = runTestCLI(t, home, client, "--json", "--no-input", "hosts", "add", "other", "--hostname", "other.example", "--password")
	if err == nil || !strings.Contains(errOut, "input_required") && !strings.Contains(errOut, "Password") {
		t.Fatalf("expected --password to require a terminal, stderr=%q err=%v", errOut, err)
	}
}

func managedHostID(t *testing.T, config []byte) string {
	t.Helper()
	for _, line := range strings.Split(string(config), "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "# bast:id="); ok {
			return after
		}
	}
	t.Fatalf("no managed id in config:\n%s", config)
	return ""
}

func TestHostAddLabelPathSetsGroup(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	out, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "add", "abc/test", "--hostname", "test.example")
	if err != nil || errOut != "" || !strings.Contains(out, `"alias":"test"`) {
		t.Fatalf("add output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "test")
	if err != nil || !strings.Contains(out, `"group":"abc"`) || !strings.Contains(out, `"label":"test"`) {
		t.Fatalf("show output=%q err=%v", out, err)
	}
}

func TestHostEditPlainLabelPreservesGroup(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	_, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "add", "abc/test", "--hostname", "test.example")
	if err != nil || errOut != "" {
		t.Fatalf("add stderr=%q err=%v", errOut, err)
	}
	out, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "edit", "test", "--label", "renamed")
	if err != nil || errOut != "" || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("edit output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "renamed")
	if err != nil || !strings.Contains(out, `"group":"abc"`) || !strings.Contains(out, `"label":"renamed"`) {
		t.Fatalf("show output=%q err=%v", out, err)
	}
}

func TestHostEditLabelPathWithExplicitGroupPeelsLeaf(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	_, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "add", "old", "--hostname", "old.example", "--group", "Legacy")
	if err != nil || errOut != "" {
		t.Fatalf("add stderr=%q err=%v", errOut, err)
	}
	out, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "edit", "old", "--label", "abc/test", "--group", "Work")
	if err != nil || errOut != "" || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("edit output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "test")
	if err != nil || !strings.Contains(out, `"group":"Work"`) || !strings.Contains(out, `"label":"test"`) {
		t.Fatalf("show output=%q err=%v", out, err)
	}
}

func TestExternalHostAllowsMetadataOnlyEdit(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host external\n    HostName external.example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client := fakeOpenSSH(t)
	out, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "edit", "external", "--label", "Friendly external", "--notes", "Read only config")
	if err != nil || errOut != "" || !strings.Contains(out, `"alias":"external"`) {
		t.Fatalf("output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "hosts", "show", "Friendly external")
	if err != nil || !strings.Contains(out, `"notes":"Read only config"`) {
		t.Fatalf("show output=%q err=%v", out, err)
	}
	_, errOut, err = runTestCLI(t, home, client, "--json", "hosts", "edit", "external", "--hostname", "changed.example")
	if code, ok := ExitCode(err); !ok || code != 1 || !strings.Contains(errOut, `"code":"external_host"`) {
		t.Fatalf("stderr=%q err=%v", errOut, err)
	}
}

func TestJSONListHasStableEnvelopeAndNoNullCollections(t *testing.T) {
	home := t.TempDir()
	out, errOut, err := runTestCLI(t, home, fakeOpenSSH(t), "hosts", "list", "--json")
	if err != nil || errOut != "" {
		t.Fatalf("stderr=%q err=%v", errOut, err)
	}
	var envelope struct {
		OK   bool         `json:"ok"`
		Data []hostRecord `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil || !envelope.OK || envelope.Data == nil {
		t.Fatalf("output=%q envelope=%+v err=%v", out, envelope, err)
	}
}

func TestKeyCommandsImportEditExportAndDelete(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	sourceDir := t.TempDir()
	private := filepath.Join(sourceDir, "source")
	if err := os.WriteFile(private, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\ndata\n-----END OPENSSH PRIVATE KEY-----\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(private+".pub", []byte("ssh-ed25519 AAA-test original\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := runTestCLI(t, home, client, "--json", "keys", "import", "work", "--private", private, "--comment", "imported key")
	if err != nil || errOut != "" || !strings.Contains(out, `"name":"work"`) {
		t.Fatalf("import output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "keys", "public", "work")
	if err != nil || !strings.Contains(out, "imported key") {
		t.Fatalf("public output=%q err=%v", out, err)
	}
	out, _, err = runTestCLI(t, home, client, "--json", "keys", "comment", "work", "--comment", "updated key")
	if err != nil || !strings.Contains(out, "updated key") {
		t.Fatalf("comment output=%q err=%v", out, err)
	}
	exportDir := t.TempDir()
	out, errOut, err = runTestCLI(t, home, client, "--json", "keys", "export", "work", "--directory", exportDir, "--yes")
	if err != nil || errOut != "" || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("export output=%q stderr=%q err=%v", out, errOut, err)
	}
	if _, err := os.Stat(filepath.Join(exportDir, "work")); err != nil {
		t.Fatal(err)
	}
	out, errOut, err = runTestCLI(t, home, client, "--json", "keys", "delete", "work", "--yes")
	if err != nil || errOut != "" || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("delete output=%q stderr=%q err=%v", out, errOut, err)
	}
}

func TestInvocationAndCommandHelp(t *testing.T) {
	if !IsInvocation([]string{"--json", "hosts", "list"}) || !IsInvocation([]string{"connect", "prod"}) || !IsInvocation([]string{"update"}) || !IsInvocation([]string{"doctor"}) || !IsInvocation([]string{"completion", "bash"}) || IsInvocation([]string{"prod"}) {
		t.Fatal("command invocation detection is incorrect")
	}
	out, errOut, err := runTestCLI(t, t.TempDir(), fakeOpenSSH(t), "hosts", "add", "--help")
	if err != nil || errOut != "" || !strings.Contains(out, "Usage: bast hosts add") {
		t.Fatalf("output=%q stderr=%q err=%v", out, errOut, err)
	}
	for _, option := range []string{"--forward-agent", "--request-tty", "--local-forward", "--ssh-option"} {
		if !strings.Contains(out, option) {
			t.Fatalf("host add help missing %q:\n%s", option, out)
		}
	}
	out, errOut, err = runTestCLI(t, t.TempDir(), fakeOpenSSH(t), "sync", "aws", "--help")
	if err != nil || errOut != "" || out != "Usage: bast sync aws\n" {
		t.Fatalf("sync aws help output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, errOut, err = runTestCLI(t, t.TempDir(), fakeOpenSSH(t), "sync", "azure", "--help")
	if err != nil || errOut != "" || out != "Usage: bast sync azure\n" {
		t.Fatalf("sync azure help output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, errOut, err = runTestCLI(t, t.TempDir(), fakeOpenSSH(t), "sync", "gcp", "--help")
	if err != nil || errOut != "" || out != "Usage: bast sync gcp\n" {
		t.Fatalf("sync gcp help output=%q stderr=%q err=%v", out, errOut, err)
	}
}

func TestHostAddRejectsDeepGroupPaths(t *testing.T) {
	home := t.TempDir()
	_, errOut, err := runTestCLI(t, home, fakeOpenSSH(t), "--json", "hosts", "add", "leaf", "--hostname", "host.example", "--group", "a/b/c/d/e/f")
	if err == nil || !strings.Contains(errOut, "validation") {
		t.Fatalf("expected validation error, stderr=%q err=%v", errOut, err)
	}
	_, errOut, err = runTestCLI(t, home, fakeOpenSSH(t), "--json", "hosts", "add", "Work/", "--hostname", "host.example")
	if err == nil || !strings.Contains(errOut, "validation") {
		t.Fatalf("expected trailing-slash validation error, stderr=%q err=%v", errOut, err)
	}
}

func runTestCLI(t *testing.T, home string, client openssh.Client, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	runner, err := New(paths.ForHome(home), client, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	err = runner.Run(args)
	return out.String(), errOut.String(), err
}

func fakeOpenSSH(t *testing.T) openssh.Client {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX OpenSSH fixtures")
	}
	dir := t.TempDir()
	writeScript := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ssh := writeScript("ssh", `
if [ "$1" = "-V" ]; then
  echo OpenSSH_9.8p1 >&2
  exit 0
fi
if [ "$1" = "-G" ]; then
  printf 'hostname resolved.example\nuser deploy\nport 22\nidentitiesonly no\npubkeyauthentication yes\npasswordauthentication yes\nproxyjump none\n'
  exit 0
fi
exit 0`)
	keygen := writeScript("ssh-keygen", `
if [ "$1" = "-F" ]; then exit 1; fi
if [ "$1" = "-lf" ]; then printf '256 SHA256:test test-key (ED25519)\n'; exit 0; fi
if [ "$1" = "-y" ]; then printf 'ssh-ed25519 AAA-test derived\n'; exit 0; fi
exit 0`)
	sshAdd := writeScript("ssh-add", "exit 1")
	return openssh.Client{SSH: ssh, SSHKeygen: keygen, SSHAdd: sshAdd}
}

func TestDoctorJSONReportsIncludeScoping(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(sshDir, "bast"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "bast", "config"), []byte("Host managed\n  HostName managed.example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host work\n  HostName work.example\n  Include ~/.ssh/bast/config\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runTestCLI(t, home, fakeOpenSSH(t), "--json", "doctor")
	code, ok := ExitCode(err)
	if !ok || code != 1 {
		t.Fatalf("expected exit 1, err=%v stderr=%q", err, errOut)
	}
	if errOut != "" {
		t.Fatalf("stderr=%q", errOut)
	}
	if !strings.Contains(out, `"ok":true`) || !strings.Contains(out, `"healthy":false`) || !strings.Contains(out, "ssh_config.include_not_toplevel") {
		t.Fatalf("output=%q", out)
	}
}

func TestVaultLoginRequiresAcceptTerms(t *testing.T) {
	home := t.TempDir()
	_, errOut, err := runTestCLI(t, home, fakeOpenSSH(t), "--no-input", "vault", "login", "--email", "you@example.com")
	if err == nil || !strings.Contains(errOut, "accept-terms") {
		t.Fatalf("expected --accept-terms gate, stderr=%q err=%v", errOut, err)
	}
}
