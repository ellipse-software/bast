package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	InstallerURL        = "https://bast.sh/install"
	NightlyInstallerURL = "https://bast.sh/install-nightly"
	LatestReleaseURL    = "https://api.github.com/repos/ellipse-software/bast/releases/latest"
	NightlyReleaseURL   = "https://api.github.com/repos/ellipse-software/bast/releases/tags/nightly"
	receiptSuffix       = ".install-receipt"
)

type Channel int

const (
	ChannelOther Channel = iota
	ChannelScriptStable
	ChannelScriptNightly
	ChannelHomebrewStable
	ChannelHomebrewNightly
)

func IsStable(version string) bool {
	_, ok := parseVersion(version)
	return ok
}

func IsNightly(version string) bool {
	return strings.HasPrefix(version, "nightly.")
}

func ChannelFor(executable string) Channel {
	resolved := resolveExecutable(executable)
	path := filepath.ToSlash(resolved)
	switch {
	case strings.Contains(path, "/Cellar/bast-nightly/"):
		return ChannelHomebrewNightly
	case strings.Contains(path, "/Cellar/bast/"):
		return ChannelHomebrewStable
	case scriptInstalled(resolved, NightlyInstallerURL):
		return ChannelScriptNightly
	case scriptInstalled(resolved, InstallerURL):
		return ChannelScriptStable
	default:
		return ChannelOther
	}
}

func ScriptInstalled(executable string) bool {
	return scriptInstalled(resolveExecutable(executable), InstallerURL)
}

func NightlyScriptInstalled(executable string) bool {
	return scriptInstalled(resolveExecutable(executable), NightlyInstallerURL)
}

func Check(ctx context.Context, client *http.Client, current string) (string, error) {
	return checkFrom(ctx, client, current, LatestReleaseURL, parseVersion, compareStable)
}

func CheckNightly(ctx context.Context, client *http.Client, current string) (string, error) {
	return checkFrom(ctx, client, current, NightlyReleaseURL, parseNightlyVersion, compareNightly)
}

func Suggestion(executable string) string {
	switch ChannelFor(executable) {
	case ChannelScriptStable, ChannelScriptNightly:
		return "bast update"
	case ChannelHomebrewStable:
		return "brew upgrade bast"
	case ChannelHomebrewNightly:
		return "brew upgrade bast-nightly"
	default:
		return "https://bast.sh"
	}
}

func Update(ctx context.Context, client *http.Client, executable string, stdout, stderr io.Writer) error {
	switch ChannelFor(executable) {
	case ChannelScriptStable:
		return updateFrom(ctx, client, InstallerURL, executable, stdout, stderr)
	case ChannelScriptNightly:
		return updateFrom(ctx, client, NightlyInstallerURL, executable, stdout, stderr)
	case ChannelHomebrewStable:
		return errors.New("this Bast installation is managed by Homebrew; run \"brew upgrade bast\"")
	case ChannelHomebrewNightly:
		return errors.New("this Bast installation is managed by Homebrew; run \"brew upgrade bast-nightly\"")
	default:
		return errors.New("self-update is only available for installs from https://bast.sh/install or https://bast.sh/install-nightly")
	}
}

func checkFrom(
	ctx context.Context,
	client *http.Client,
	current, url string,
	parse func(string) (any, bool),
	compare func(any, any) int,
) (string, error) {
	currentVersion, ok := parse(current)
	if !ok {
		return "", nil
	}
	latest, err := latestFrom(ctx, client, url)
	if err != nil {
		return "", err
	}
	latestVersion, ok := parse(latest)
	if !ok {
		return "", fmt.Errorf("latest release has unsupported version %q", latest)
	}
	switch compare(latestVersion, currentVersion) {
	case 1:
		return latest, nil
	default:
		return "", nil
	}
}

func latestFrom(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "bast")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub release check returned %s", resp.Status)
	}
	var release struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("decode GitHub release: %w", err)
	}
	if url == NightlyReleaseURL {
		if version := nightlyVersionFromReleaseName(release.Name); version != "" {
			return version, nil
		}
	}
	return release.TagName, nil
}

func nightlyVersionFromReleaseName(name string) string {
	const prefix = "Bast nightly ("
	const suffix = ")"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return ""
	}
	version := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if IsNightly(version) {
		return version
	}
	return ""
}

func updateFrom(ctx context.Context, client *http.Client, installerURL, executable string, stdout, stderr io.Writer) error {
	executable = resolveExecutable(executable)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, installerURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download installer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download installer: %s", resp.Status)
	}
	script, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("download installer: %w", err)
	}
	if !strings.HasPrefix(string(script), "#!/bin/sh\n") {
		return errors.New("download installer: unexpected response")
	}
	tmp, err := os.CreateTemp("", "bast-update-*.sh")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0700); err == nil {
		_, err = tmp.Write(script)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("prepare installer: %w", err)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", tmpPath)
	cmd.Env = append(os.Environ(), "BAST_INSTALL_DIR="+filepath.Dir(executable))
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run installer: %w", err)
	}
	return nil
}

func scriptInstalled(executable, installerURL string) bool {
	b, err := os.ReadFile(executable + receiptSuffix)
	return err == nil && strings.TrimSpace(string(b)) == installerURL
}

func resolveExecutable(executable string) string {
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		return resolved
	}
	return executable
}

func parseVersion(version string) (any, bool) {
	var parsed [3]int
	if !strings.HasPrefix(version, "v") {
		return parsed, false
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != len(parsed) {
		return parsed, false
	}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return [3]int{}, false
		}
		parsed[i] = value
	}
	return parsed, true
}

type nightlyVersion struct {
	date string
	sha  string
}

func parseNightlyVersion(version string) (any, bool) {
	if !IsNightly(version) {
		return nightlyVersion{}, false
	}
	parts := strings.Split(strings.TrimPrefix(version, "nightly."), ".")
	if len(parts) != 2 || len(parts[0]) != 8 || len(parts[1]) != 7 {
		return nightlyVersion{}, false
	}
	for _, digit := range parts[0] {
		if digit < '0' || digit > '9' {
			return nightlyVersion{}, false
		}
	}
	return nightlyVersion{date: parts[0], sha: parts[1]}, true
}

func compareStable(latest, current any) int {
	latestParts := latest.([3]int)
	currentParts := current.([3]int)
	for i := range latestParts {
		if latestParts[i] > currentParts[i] {
			return 1
		}
		if latestParts[i] < currentParts[i] {
			return -1
		}
	}
	return 0
}

func compareNightly(latest, current any) int {
	latestParts := latest.(nightlyVersion)
	currentParts := current.(nightlyVersion)
	switch {
	case latestParts.date > currentParts.date:
		return 1
	case latestParts.date < currentParts.date:
		return -1
	case latestParts.sha > currentParts.sha:
		return 1
	case latestParts.sha < currentParts.sha:
		return -1
	default:
		return 0
	}
}
