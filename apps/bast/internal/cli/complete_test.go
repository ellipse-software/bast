package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bast/internal/openssh"
	"bast/internal/paths"
)

func TestIsInvocationRecognizesCompletion(t *testing.T) {
	if !IsCommand("completion") || !IsCommand("__complete") {
		t.Fatal("completion commands must be recognized")
	}
	if !IsInvocation([]string{"completion", "bash"}) || !IsInvocation([]string{"__complete", "--", "hosts"}) {
		t.Fatal("completion invocations must be detected")
	}
	if !IsInvocation([]string{"--json", "completion", "zsh"}) {
		t.Fatal("completion after global flags must be detected")
	}
	if IsInvocation([]string{"prod"}) {
		t.Fatal("host labels must not be treated as command invocations")
	}
}

func TestCompletionScripts(t *testing.T) {
	home := t.TempDir()
	for _, shell := range completionShells {
		out, errOut, err := runTestCLI(t, home, openssh.Client{}, "completion", shell)
		if err != nil || errOut != "" {
			t.Fatalf("%s: output=%q stderr=%q err=%v", shell, out, errOut, err)
		}
		if !strings.Contains(out, "__complete") {
			t.Fatalf("%s script missing __complete:\n%s", shell, out)
		}
	}

	cases := []struct {
		shell, marker string
	}{
		{"bash", "complete -F"},
		{"zsh", "compdef"},
		{"fish", "complete -c bast"},
		{"powershell", "Register-ArgumentCompleter"},
		{"pwsh", "Register-ArgumentCompleter"},
		{"elvish", "edit:completion:arg-completer"},
		{"nushell", "extern"},
		{"nu", "extern"},
	}
	for _, tc := range cases {
		out, _, err := runTestCLI(t, home, openssh.Client{}, "completion", tc.shell)
		if err != nil {
			t.Fatalf("%s: %v", tc.shell, err)
		}
		if !strings.Contains(out, tc.marker) {
			t.Fatalf("%s script missing %q:\n%s", tc.shell, tc.marker, out)
		}
	}
}

func TestCompletionRequiresKnownShell(t *testing.T) {
	home := t.TempDir()
	_, errOut, err := runTestCLI(t, home, openssh.Client{}, "completion")
	if code, ok := ExitCode(err); !ok || code != 2 || !strings.Contains(errOut, "usage: bast completion") {
		t.Fatalf("missing shell stderr=%q err=%v", errOut, err)
	}
	_, errOut, err = runTestCLI(t, home, openssh.Client{}, "completion", "csh")
	if code, ok := ExitCode(err); !ok || code != 2 || !strings.Contains(errOut, "unknown shell") {
		t.Fatalf("unknown shell stderr=%q err=%v", errOut, err)
	}
}

func TestCompletionHelp(t *testing.T) {
	out, errOut, err := runTestCLI(t, t.TempDir(), openssh.Client{}, "completion", "--help")
	if err != nil || errOut != "" || !strings.Contains(out, "Usage: bast completion") || !strings.Contains(out, "source <(bast completion bash)") {
		t.Fatalf("output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, errOut, err = runTestCLI(t, t.TempDir(), openssh.Client{}, "-h")
	if err != nil || errOut != "" || !strings.Contains(out, "bast completion <shell>") {
		t.Fatalf("root help output=%q stderr=%q err=%v", out, errOut, err)
	}
}

func TestCompleteQuery(t *testing.T) {
	home := t.TempDir()
	writeCompleteFixture(t, home)

	values, directive := runComplete(t, home, "")
	if directive != "nofiles" {
		t.Fatalf("directive=%q", directive)
	}
	for _, want := range []string{"hosts", "keys", "connect", "completion", "visible", "Visible host", "Production_web", "Production web", "ascii_box_dev"} {
		if !containsValue(values, want) {
			t.Fatalf("first token missing %q: %v", want, values)
		}
	}
	for _, blocked := range []string{"__complete", "hidden", "Hidden host"} {
		if containsValue(values, blocked) {
			t.Fatalf("first token included %q: %v", blocked, values)
		}
	}

	values, _ = runComplete(t, home, "hosts", "sh")
	if !containsValue(values, "show") || !containsValue(values, "show-hidden") {
		t.Fatalf("hosts sh = %v", values)
	}

	values, _ = runComplete(t, home, "hosts", "show", "")
	if !containsValue(values, "visible") || !containsValue(values, "Production_web") || !containsValue(values, "Production web") {
		t.Fatalf("hosts show = %v", values)
	}
	if containsValue(values, "hidden") {
		t.Fatalf("hosts show included hidden: %v", values)
	}

	values, _ = runComplete(t, home, "hosts", "show-hidden", "")
	if !containsValue(values, "hidden") || !containsValue(values, "Hidden host") {
		t.Fatalf("hosts show-hidden = %v", values)
	}

	values, _ = runComplete(t, home, "keys", "show", "")
	if !containsValue(values, "work") {
		t.Fatalf("keys show = %v", values)
	}

	values, _ = runComplete(t, home, "hosts", "list", "--sort", "")
	for _, want := range []string{"smart", "label", "recent", "group"} {
		if !containsValue(values, want) {
			t.Fatalf("hosts list --sort missing %q: %v", want, values)
		}
	}

	values, _ = runComplete(t, home, "hosts", "list", "--sort=s")
	if !containsValue(values, "--sort=smart") {
		t.Fatalf("hosts list --sort=s = %v", values)
	}

	values, _ = runComplete(t, home, "sync", "disable", "")
	for _, want := range syncProviders {
		if !containsValue(values, want) {
			t.Fatalf("sync disable missing %q: %v", want, values)
		}
	}

	values, _ = runComplete(t, home, "completion", "")
	for _, want := range completionShells {
		if !containsValue(values, want) {
			t.Fatalf("completion missing %q: %v", want, values)
		}
	}

	values, _ = runComplete(t, home, "hosts", "list", "--j")
	if !containsValue(values, "--json") {
		t.Fatalf("global flag after subcommand = %v", values)
	}

	values, _ = runComplete(t, home, "box", "fork", "")
	if !containsValue(values, "ascii_box_dev") || !containsValue(values, "box-123") {
		t.Fatalf("box fork = %v", values)
	}

	_, directive = runComplete(t, home, "keys", "import", "--private", "")
	if directive != "files" {
		t.Fatalf("keys import --private directive=%q", directive)
	}

	_, directive = runComplete(t, home, "keys", "export", "--directory", "")
	if directive != "dirs" {
		t.Fatalf("keys export --directory directive=%q", directive)
	}
}

func TestCompleteDoesNotInvokeOpenSSH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sentinel script")
	}
	home := t.TempDir()
	writeCompleteFixture(t, home)
	sentinel := filepath.Join(t.TempDir(), "invoked")
	binDir := t.TempDir()
	body := fmt.Sprintf("#!/bin/sh\nprintf invoked > %q\nexit 1\n", sentinel)
	write := func(name string) string {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	client := openssh.Client{SSH: write("ssh"), SSHKeygen: write("ssh-keygen"), SSHAdd: write("ssh-add")}
	var out, errOut bytes.Buffer
	runner, err := New(paths.ForHome(home), client, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Run([]string{"__complete", "--", "hosts", "show", ""}); err != nil {
		t.Fatalf("complete: stderr=%q err=%v", errOut.String(), err)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("completion invoked OpenSSH")
	}
	values, _ := parseComplete(out.String())
	if !containsValue(values, "visible") {
		t.Fatalf("values=%v", values)
	}
}

func runComplete(t *testing.T, home string, tokens ...string) ([]string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	runner, err := New(paths.ForHome(home), openssh.Client{}, strings.NewReader(""), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"__complete", "--"}
	args = append(args, tokens...)
	if err := runner.Run(args); err != nil {
		t.Fatalf("complete tokens=%v stderr=%q err=%v", tokens, errOut.String(), err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("complete wrote stderr=%q", errOut.String())
	}
	return parseComplete(out.String())
}

func parseComplete(out string) ([]string, string) {
	directive := ""
	var values []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.HasPrefix(line, ":") {
			directive = strings.TrimPrefix(line, ":")
			continue
		}
		if line == "" {
			continue
		}
		value, _, _ := strings.Cut(line, "\t")
		values = append(values, value)
	}
	return values, directive
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeCompleteFixture(t *testing.T, home string) {
	t.Helper()
	sshDir := filepath.Join(home, ".ssh")
	keysDir := filepath.Join(sshDir, "bast", "keys")
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "bast"), 0700); err != nil {
		t.Fatal(err)
	}
	config := `Host visible
    HostName visible.example
Host hidden
    HostName hidden.example
Host Production_web
    HostName prod.example
# bast:sync:box=box-123
Host ascii_box_dev
    HostName box.example
# bast:sync:end
`
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	state := `{
  "version": 7,
  "hosts": {
    "visible": {"label": "Visible host"},
    "hidden": {"label": "Hidden host", "hidden": true},
    "Production_web": {"label": "Production web"}
  }
}
`
	if err := os.WriteFile(filepath.Join(home, ".config", "bast", "state.json"), []byte(state), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keysDir, "work"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\ndata\n-----END OPENSSH PRIVATE KEY-----\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keysDir, "work.pub"), []byte("ssh-ed25519 AAA work\n"), 0644); err != nil {
		t.Fatal(err)
	}
}
