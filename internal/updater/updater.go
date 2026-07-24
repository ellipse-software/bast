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
	InstallerURL     = "https://bast.sh/install"
	LatestReleaseURL = "https://api.github.com/repos/ellipse-software/bast/releases/latest"
	receiptSuffix    = ".install-receipt"
)

func IsStable(version string) bool {
	_, ok := parseVersion(version)
	return ok
}

func Check(ctx context.Context, client *http.Client, current string) (string, error) {
	return checkFrom(ctx, client, current, LatestReleaseURL)
}

func checkFrom(ctx context.Context, client *http.Client, current, url string) (string, error) {
	currentVersion, ok := parseVersion(current)
	if !ok {
		return "", nil
	}
	latest, err := latestFrom(ctx, client, url)
	if err != nil {
		return "", err
	}
	latestVersion, ok := parseVersion(latest)
	if !ok {
		return "", fmt.Errorf("latest release has unsupported version %q", latest)
	}
	for i := range currentVersion {
		if latestVersion[i] > currentVersion[i] {
			return latest, nil
		}
		if latestVersion[i] < currentVersion[i] {
			return "", nil
		}
	}
	return "", nil
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
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("decode GitHub release: %w", err)
	}
	return release.TagName, nil
}

func Suggestion(executable string) string {
	resolved := resolveExecutable(executable)
	if ScriptInstalled(resolved) {
		return "bast update"
	}
	if strings.Contains(filepath.ToSlash(resolved), "/Cellar/bast/") {
		return "brew upgrade bast"
	}
	return "https://bast.sh"
}

func ScriptInstalled(executable string) bool {
	b, err := os.ReadFile(resolveExecutable(executable) + receiptSuffix)
	return err == nil && strings.TrimSpace(string(b)) == InstallerURL
}

func Update(ctx context.Context, client *http.Client, executable string, stdout, stderr io.Writer) error {
	return updateFrom(ctx, client, InstallerURL, executable, stdout, stderr)
}

func updateFrom(ctx context.Context, client *http.Client, installerURL, executable string, stdout, stderr io.Writer) error {
	executable = resolveExecutable(executable)
	if !ScriptInstalled(executable) {
		if strings.Contains(filepath.ToSlash(executable), "/Cellar/bast/") {
			return errors.New("this Bast installation is managed by Homebrew; run \"brew upgrade bast\"")
		}
		return errors.New("self-update is only available for installs from https://bast.sh/install")
	}
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

func resolveExecutable(executable string) string {
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		return resolved
	}
	return executable
}

func parseVersion(version string) ([3]int, bool) {
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
