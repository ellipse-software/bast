package cli

import (
	"strings"
	"testing"

	"bast/internal/feature"
	"bast/internal/openssh"
)

func TestHelpOmitsDisabledFeatures(t *testing.T) {
	if filterHelp(help, false, false) != help {
		t.Fatal("filtering with both features on changed the help text")
	}
	filtered := filterHelp(help, true, false)
	for _, gone := range []string{"bast sync ", "bast boat ", "bast boxd ", "Sync commands:", "Boat commands:"} {
		if strings.Contains(filtered, gone) {
			t.Fatalf("sync help still contains %q", gone)
		}
	}
	if !strings.Contains(filtered, "bast vault ") || !strings.Contains(filtered, "bast hosts ") {
		t.Fatal("vault and hosts should stay in help when only sync is off")
	}
	if strings.Contains(filtered, "\n\n\n") {
		t.Fatal("filtered help has a double blank line")
	}
	both := filterHelp(help, true, true)
	if strings.Contains(both, "bast vault ") || strings.Contains(both, "Vault commands:") {
		t.Fatal("vault help remained")
	}
}

func TestCommandsRejectDisabledFeatures(t *testing.T) {
	t.Setenv(feature.EnvNoSync, "1")
	t.Setenv(feature.EnvNoVault, "")
	home := t.TempDir()
	for _, args := range [][]string{{"sync"}, {"sync", "--help"}, {"boat", "new"}, {"--json", "boxd"}} {
		out, errOut, err := runTestCLI(t, home, openssh.Client{}, args...)
		code, ok := ExitCode(err)
		if !ok || code != 1 || out != "" || !strings.Contains(errOut, "BAST_NO_SYNC") {
			t.Fatalf("%v out=%q err=%q exit=%v", args, out, errOut, err)
		}
	}
	out, errOut, err := runTestCLI(t, home, openssh.Client{}, "--help")
	if err != nil || errOut != "" || strings.Contains(out, "bast sync ") || !strings.Contains(out, "bast vault ") {
		t.Fatalf("help out has sync=%v vault=%v err=%v stderr=%q", strings.Contains(out, "bast sync "), strings.Contains(out, "bast vault "), err, errOut)
	}

	t.Setenv(feature.EnvNoSync, "")
	t.Setenv(feature.EnvNoVault, "1")
	_, errOut, err = runTestCLI(t, home, openssh.Client{}, "vault", "status")
	if code, ok := ExitCode(err); !ok || code != 1 || !strings.Contains(errOut, "BAST_NO_VAULT") {
		t.Fatalf("vault status stderr=%q err=%v", errOut, err)
	}
}

func TestCompletionsOmitDisabledFeatures(t *testing.T) {
	t.Setenv(feature.EnvNoSync, "1")
	t.Setenv(feature.EnvNoVault, "")
	values, _ := runComplete(t, t.TempDir(), "")
	for _, gone := range []string{"sync", "boat", "boxd", "vercel"} {
		if containsValue(values, gone) {
			t.Fatalf("completion offered %q: %v", gone, values)
		}
	}
	if !containsValue(values, "hosts") || !containsValue(values, "vault") {
		t.Fatalf("completion dropped hosts or vault: %v", values)
	}
	t.Setenv(feature.EnvNoVault, "1")
	values, _ = runComplete(t, t.TempDir(), "")
	if containsValue(values, "vault") {
		t.Fatalf("completion offered vault: %v", values)
	}
}
