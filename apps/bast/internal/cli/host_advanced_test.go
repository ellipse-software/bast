package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostAdvancedFlagsRoundTrip(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)

	out, errOut, err := runTestCLI(t, home, client, "--json", "hosts", "add", "Advanced host",
		"--hostname", "adv.example",
		"--user", "deploy",
		"--forward-agent", "yes",
		"--startup-command", "tmux attach",
		"--request-tty", "force",
		"--set-env", "FOO=bar",
		"--set-env", "CSV=a,b",
		"--compression", "yes",
		"--keepalive", "30",
		"--ssh-option", "TCPKeepAlive yes",
	)
	if err != nil || errOut != "" || !strings.Contains(out, `"alias":"Advanced_host"`) {
		t.Fatalf("add output=%q stderr=%q err=%v", out, errOut, err)
	}

	managed, err := os.ReadFile(filepath.Join(home, ".ssh", "bast", "config"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(managed)
	for _, want := range []string{
		"ForwardAgent yes",
		`RemoteCommand "tmux attach"`,
		"RequestTTY force",
		"SetEnv FOO=bar",
		"SetEnv CSV=a,b",
		"Compression yes",
		"ServerAliveInterval 30",
		"TCPKeepAlive yes",
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("managed config missing %q:\n%s", want, config)
		}
	}

	out, errOut, err = runTestCLI(t, home, client, "--json", "hosts", "show", "Advanced_host")
	if err != nil || errOut != "" {
		t.Fatalf("show output=%q stderr=%q err=%v", out, errOut, err)
	}
	for _, want := range []string{
		`"forwardAgent":"yes"`,
		`"remoteCommand":"tmux attach"`,
		`"requestTTY":"force"`,
		`"FOO=bar"`,
		`"CSV=a,b"`,
		`"compression":"yes"`,
		`"serverAliveInterval":"30"`,
		`"TCPKeepAlive yes"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}

	out, errOut, err = runTestCLI(t, home, client, "--json", "hosts", "edit", "Advanced_host",
		"--clear-forward-agent",
		"--clear-startup-command",
		"--clear-set-env",
	)
	if err != nil || errOut != "" || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("edit output=%q stderr=%q err=%v", out, errOut, err)
	}

	managed, err = os.ReadFile(filepath.Join(home, ".ssh", "bast", "config"))
	if err != nil {
		t.Fatal(err)
	}
	config = string(managed)
	for _, absent := range []string{"ForwardAgent", "RemoteCommand", "SetEnv FOO=bar"} {
		if strings.Contains(config, absent) {
			t.Fatalf("managed config still contains %q after clear:\n%s", absent, config)
		}
	}
	for _, present := range []string{"RequestTTY force", "Compression yes", "TCPKeepAlive yes"} {
		if !strings.Contains(config, present) {
			t.Fatalf("managed config missing %q after partial clear:\n%s", present, config)
		}
	}
}

func TestHostForwardFlagsAreRejected(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	_, errOut, err := runTestCLI(t, home, client, "hosts", "add", "web", "--hostname", "web.example", "--local-forward", "8080 localhost:80")
	if err == nil || !strings.Contains(errOut, "bast tunnel add") {
		t.Fatalf("add errOut=%q err=%v", errOut, err)
	}
	if _, _, err := runTestCLI(t, home, client, "hosts", "add", "web", "--hostname", "web.example"); err != nil {
		t.Fatal(err)
	}
	_, errOut, err = runTestCLI(t, home, client, "hosts", "edit", "web", "--clear-local-forward")
	if err == nil || !strings.Contains(errOut, "bast tunnel add") {
		t.Fatalf("edit errOut=%q err=%v", errOut, err)
	}
}
