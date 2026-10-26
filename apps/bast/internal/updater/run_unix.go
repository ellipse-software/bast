//go:build !windows

package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runInstaller(ctx context.Context, script []byte, executable string, stdout, stderr io.Writer, pinnedNightly string) error {
	if !strings.HasPrefix(string(script), "#!/bin/sh\n") {
		return errors.New("download installer: unexpected response")
	}
	tmp, err := os.CreateTemp("", "bast-update-*.sh")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0700); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("prepare installer: %w", err)
	}
	if _, err := tmp.Write(script); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("prepare installer: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("prepare installer: %w", err)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", tmpPath)
	env := append(os.Environ(), "BAST_INSTALL_DIR="+filepath.Dir(executable))
	if pinnedNightly != "" {
		env = append(env, "BAST_NIGHTLY_VERSION="+pinnedNightly)
	}
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run installer: %w", err)
	}
	return nil
}

func wingetInstalled(string) bool { return false }

func linuxPackageChannel(executable string) Channel {
	if !systemLinuxBinary(executable) {
		return ChannelOther
	}
	if name, ok := dpkgOwner(executable); ok && name == "bast" {
		return ChannelDeb
	}
	if name, ok := rpmOwner(executable); ok && name == "bast" {
		return ChannelRpm
	}
	if name, ok := pacmanOwner(executable); ok {
		switch name {
		case "bast":
			return ChannelPacman
		case "bast-bin":
			return ChannelPacmanBin
		}
	}
	if name, ok := apkOwner(executable); ok && name == "bast" {
		return ChannelApk
	}
	return ChannelOther
}

func systemLinuxBinary(executable string) bool {
	path := filepath.ToSlash(executable)
	if strings.Contains(path, "/Cellar/") {
		return false
	}
	if filepath.Base(path) != "bast" {
		return false
	}
	switch filepath.ToSlash(filepath.Dir(path)) {
	case "/usr/bin", "/bin", "/usr/local/bin":
		return true
	default:
		return false
	}
}

func dpkgOwner(executable string) (string, bool) {
	out, err := exec.Command("dpkg-query", "-S", executable).Output()
	if err != nil {
		return "", false
	}
	return packageNameFromDpkgQuery(string(out))
}

func rpmOwner(executable string) (string, bool) {
	out, err := exec.Command("rpm", "-qf", "--qf", "%{NAME}\n", executable).Output()
	if err != nil {
		return "", false
	}
	return packageNameFromLine(string(out))
}

func pacmanOwner(executable string) (string, bool) {
	out, err := exec.Command("pacman", "-Qqo", executable).Output()
	if err != nil {
		return "", false
	}
	return packageNameFromLine(string(out))
}

func apkOwner(executable string) (string, bool) {
	out, err := exec.Command("apk", "info", "--who-owns", executable).Output()
	if err != nil {
		return "", false
	}
	return packageNameFromApkWhoOwns(string(out))
}

func packageNameFromDpkgQuery(out string) (string, bool) {
	line := strings.TrimSpace(out)
	if line == "" {
		return "", false
	}
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	name, _, ok := strings.Cut(line, ":")
	if !ok {
		return "", false
	}
	name = strings.TrimSpace(name)
	if arch, _, split := strings.Cut(name, ":"); split {
		name = arch
	}
	if name == "" {
		return "", false
	}
	return name, true
}

func packageNameFromLine(out string) (string, bool) {
	name := strings.TrimSpace(out)
	if name == "" || strings.ContainsAny(name, " \t") {
		return "", false
	}
	return name, true
}

func packageNameFromApkWhoOwns(out string) (string, bool) {
	line := strings.TrimSpace(out)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	const marker = " is owned by "
	idx := strings.LastIndex(line, marker)
	if idx < 0 {
		return "", false
	}
	pkg := strings.TrimSpace(line[idx+len(marker):])
	if pkg == "" {
		return "", false
	}
	if i := strings.LastIndex(pkg, "-"); i > 0 {
		rest := pkg[:i]
		if j := strings.LastIndex(rest, "-"); j > 0 {
			pkg = rest[:j]
		}
	}
	if pkg == "" {
		return "", false
	}
	return pkg, true
}

func rpmUpgradeSuggestion() string {
	return rpmUpgradeSuggestionFrom(readOSRelease(), lookPath("zypper"), lookPath("dnf"), lookPath("yum"))
}

func rpmUpgradeSuggestionFrom(osRelease string, hasZypper, hasDnf, hasYum bool) string {
	lower := strings.ToLower(osRelease)
	if hasZypper && (strings.Contains(lower, "suse") || strings.Contains(lower, "sles")) {
		return "sudo zypper update bast"
	}
	if hasDnf {
		return "sudo dnf upgrade bast"
	}
	if hasYum {
		return "sudo yum update bast"
	}
	return "sudo dnf upgrade bast"
}

func pacmanBinUpgradeSuggestion() string {
	if lookPath("yay") {
		return "yay -Syu bast-bin"
	}
	if lookPath("paru") {
		return "paru -Syu bast-bin"
	}
	return "sudo pacman -Syu bast-bin"
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func readOSRelease() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return string(b)
}
