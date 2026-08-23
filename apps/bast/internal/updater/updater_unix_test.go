//go:build !windows

package updater

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemLinuxBinary(t *testing.T) {
	cases := map[string]bool{
		"/usr/bin/bast":           true,
		"/bin/bast":               true,
		"/usr/local/bin/bast":     true,
		"/home/u/.local/bin/bast": false,
		"/usr/bin/not-bast":       false,
		"/home/linuxbrew/.linuxbrew/Cellar/bast/1.2.3/bin/bast": false,
	}
	for path, want := range cases {
		if got := systemLinuxBinary(path); got != want {
			t.Fatalf("%s: got %t want %t", path, got, want)
		}
	}
}

func TestPackageNameParsers(t *testing.T) {
	name, ok := packageNameFromDpkgQuery("bast: /usr/bin/bast\n")
	if !ok || name != "bast" {
		t.Fatalf("dpkg query: name=%q ok=%t", name, ok)
	}
	name, ok = packageNameFromDpkgQuery("bast:amd64: /usr/bin/bast")
	if !ok || name != "bast" {
		t.Fatalf("dpkg multiarch: name=%q ok=%t", name, ok)
	}
	if _, ok = packageNameFromDpkgQuery(""); ok {
		t.Fatal("empty dpkg query should fail")
	}

	name, ok = packageNameFromLine("bast\n")
	if !ok || name != "bast" {
		t.Fatalf("rpm line: name=%q ok=%t", name, ok)
	}
	if _, ok = packageNameFromLine("file is not owned"); ok {
		t.Fatal("rpm noise should fail")
	}

	name, ok = packageNameFromApkWhoOwns("/usr/bin/bast is owned by bast-0.9.2-r1\n")
	if !ok || name != "bast" {
		t.Fatalf("apk who-owns: name=%q ok=%t", name, ok)
	}
}

func TestRpmUpgradeSuggestionFrom(t *testing.T) {
	suse := rpmUpgradeSuggestionFrom("ID=opensuse-tumbleweed\nID_LIKE=\"suse\"\n", true, true, false)
	if suse != "sudo zypper update bast" {
		t.Fatalf("suse=%q", suse)
	}
	fedora := rpmUpgradeSuggestionFrom("ID=fedora\n", false, true, true)
	if fedora != "sudo dnf upgrade bast" {
		t.Fatalf("fedora=%q", fedora)
	}
	el := rpmUpgradeSuggestionFrom("ID=rhel\n", false, false, true)
	if el != "sudo yum update bast" {
		t.Fatalf("el=%q", el)
	}
}

func writeFakeCommand(t *testing.T, dir, name, script string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxPackageChannelUsesPackageDatabases(t *testing.T) {
	dir := t.TempDir()
	writeFakeCommand(t, dir, "dpkg-query", "#!/bin/sh\necho 'bast: /usr/bin/bast'\n")
	t.Setenv("PATH", dir)
	if got := linuxPackageChannel("/usr/bin/bast"); got != ChannelDeb {
		t.Fatalf("deb channel=%v", got)
	}
	if got := linuxPackageChannel("/home/u/.local/bin/bast"); got != ChannelOther {
		t.Fatalf("user binary should not look like a package: %v", got)
	}

	writeFakeCommand(t, dir, "dpkg-query", "#!/bin/sh\nexit 1\n")
	writeFakeCommand(t, dir, "rpm", "#!/bin/sh\necho bast\n")
	if got := linuxPackageChannel("/usr/bin/bast"); got != ChannelRpm {
		t.Fatalf("rpm channel=%v", got)
	}

	writeFakeCommand(t, dir, "rpm", "#!/bin/sh\nexit 1\n")
	writeFakeCommand(t, dir, "pacman", "#!/bin/sh\necho bast-bin\n")
	if got := linuxPackageChannel("/usr/bin/bast"); got != ChannelPacmanBin {
		t.Fatalf("aur channel=%v", got)
	}

	writeFakeCommand(t, dir, "pacman", "#!/bin/sh\necho bast\n")
	if got := linuxPackageChannel("/usr/bin/bast"); got != ChannelPacman {
		t.Fatalf("pacman channel=%v", got)
	}

	writeFakeCommand(t, dir, "pacman", "#!/bin/sh\nexit 1\n")
	writeFakeCommand(t, dir, "apk", "#!/bin/sh\necho '/usr/bin/bast is owned by bast-0.9.2-r1'\n")
	if got := linuxPackageChannel("/usr/bin/bast"); got != ChannelApk {
		t.Fatalf("apk channel=%v", got)
	}
}

func TestSuggestionAndUpdateForLinuxPackages(t *testing.T) {
	dir := t.TempDir()
	writeFakeCommand(t, dir, "dpkg-query", "#!/bin/sh\necho 'bast: /usr/bin/bast'\n")
	t.Setenv("PATH", dir)

	if got := Suggestion("/usr/bin/bast"); got != "sudo apt upgrade bast" {
		t.Fatalf("apt suggestion=%q", got)
	}

	err := Update(context.Background(), nil, "/usr/bin/bast", io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "managed by apt") {
		t.Fatalf("err=%v", err)
	}

	writeFakeCommand(t, dir, "dpkg-query", "#!/bin/sh\nexit 1\n")
	writeFakeCommand(t, dir, "rpm", "#!/bin/sh\nexit 1\n")
	writeFakeCommand(t, dir, "pacman", "#!/bin/sh\necho bast-bin\n")
	writeFakeCommand(t, dir, "yay", "#!/bin/sh\nexit 0\n")
	if got := Suggestion("/usr/bin/bast"); got != "yay -Syu bast-bin" {
		t.Fatalf("aur suggestion=%q", got)
	}
	err = Update(context.Background(), nil, "/usr/bin/bast", io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "managed by the AUR") {
		t.Fatalf("aur err=%v", err)
	}
}
