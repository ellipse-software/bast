package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if !IsInvocation([]string{"--json", "hosts", "list"}) || !IsInvocation([]string{"connect", "prod"}) || !IsInvocation([]string{"update"}) || IsInvocation([]string{"prod"}) {
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
	dir := t.TempDir()
	writeScript := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ssh := writeScript("ssh", `
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
