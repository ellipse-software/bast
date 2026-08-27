package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bast/internal/cli"
)

func TestBuildVersionPrefersLinkerValue(t *testing.T) {
	original := version
	version = "v1.2.3"
	t.Cleanup(func() { version = original })
	if got := buildVersion(); got != "v1.2.3" {
		t.Fatalf("buildVersion() = %q", got)
	}
}

func TestBuildVersionFallsBackWhenLinkerValueIsEmpty(t *testing.T) {
	original := version
	version = ""
	t.Cleanup(func() { version = original })
	if got := buildVersion(); got == "" {
		t.Fatal("buildVersion() returned an empty version")
	}
}

func TestShippedBinaryHelpTwice(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	name := "bast"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".")
	build.Dir = filepath.Dir(file)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	run := func() string {
		cmd := exec.Command(bin, "--help")
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bast --help: %v\n%s", err, out)
		}
		text := string(out)
		if strings.TrimSpace(text) == "" {
			t.Fatal("bast --help was empty")
		}
		return text
	}
	first := run()
	second := run()
	if first != second {
		t.Fatalf("help output differed\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	for _, name := range []string{"vercel", "upstash", "hetzner", "doctor"} {
		if !strings.Contains(first, name) {
			t.Fatalf("help missing %q:\n%s", name, first)
		}
	}
	var printed strings.Builder
	cli.PrintHelp(&printed)
	if !strings.Contains(printed.String(), "vercel") {
		t.Fatal("PrintHelp drifted from shipped help")
	}
}
