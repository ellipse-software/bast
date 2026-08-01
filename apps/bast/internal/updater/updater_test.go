package updater

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCheckFindsOnlyNewerStableReleases(t *testing.T) {
	latest := "v1.4.0"
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Fatal("GitHub API headers are missing")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"` + latest + `"}`)),
		}, nil
	})}

	available, err := checkFrom(context.Background(), client, "v1.3.9", "https://api.github.test/latest", false, parseVersion, compareStable)
	if err != nil || available != "v1.4.0" {
		t.Fatalf("available=%q err=%v", available, err)
	}
	latest = "v1.3.9"
	available, err = checkFrom(context.Background(), client, "v1.3.9", "https://api.github.test/latest", false, parseVersion, compareStable)
	if err != nil || available != "" {
		t.Fatalf("equal available=%q err=%v", available, err)
	}
	latest = "v1.3.8"
	available, err = checkFrom(context.Background(), client, "v1.3.9", "https://api.github.test/latest", false, parseVersion, compareStable)
	if err != nil || available != "" {
		t.Fatalf("older available=%q err=%v", available, err)
	}
	before := requests
	available, err = checkFrom(context.Background(), client, "dev", "https://api.github.test/latest", false, parseVersion, compareStable)
	if err != nil || available != "" || requests != before {
		t.Fatalf("development check available=%q err=%v requests=%d", available, err, requests-before)
	}
	if !IsStable("v1.2.3") || IsStable("dev") || IsStable("v1.2.3-beta") {
		t.Fatal("stable version detection is incorrect")
	}
}

func TestCheckFindsOnlyNewerNightlyReleases(t *testing.T) {
	// Older nightly listed first on purpose: selection must ignore API order.
	releaseBody := `[
		{"tag_name":"nightly.20250720.oldold1","name":"Bast nightly (nightly.20250720.oldold1)","prerelease":true,"published_at":"2025-07-20T01:00:00Z"},
		{"tag_name":"v1.4.0","prerelease":false,"published_at":"2025-07-26T01:00:00Z"},
		{"tag_name":"nightly.20250725.deadbee","name":"Bast nightly (nightly.20250725.deadbee)","prerelease":true,"published_at":"2025-07-25T01:00:00Z"}
	]`
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(releaseBody)),
		}, nil
	})}

	available, err := CheckNightly(context.Background(), client, "nightly.20250724.abc1234")
	if err != nil || available != "nightly.20250725.deadbee" {
		t.Fatalf("available=%q err=%v", available, err)
	}
	available, err = CheckNightly(context.Background(), client, "nightly.20250725.deadbee")
	if err != nil || available != "" {
		t.Fatalf("equal available=%q err=%v", available, err)
	}
	if !IsNightly("nightly.20250724.abc1234") || IsNightly("v1.0.0") {
		t.Fatal("nightly version detection is incorrect")
	}
	if compareNightly(nightlyVersion{date: "20250725", sha: "0000000"}, nightlyVersion{date: "20250725", sha: "fffffff"}) != 1 {
		t.Fatal("a differing SHA on the same date was not treated as newer")
	}
}

func TestNewestNightlyReleasePrefersHigherDateOverListOrder(t *testing.T) {
	got, ok := newestNightlyRelease([]githubRelease{
		{TagName: "nightly.20250720.aaaaaaa", Prerelease: true, PublishedAt: "2025-07-28T00:00:00Z"},
		{TagName: "nightly.20250727.bbbbbbb", Prerelease: true, PublishedAt: "2025-07-27T00:00:00Z"},
		{TagName: "v1.0.0", Prerelease: false, PublishedAt: "2025-07-29T00:00:00Z"},
	})
	if !ok || got != "nightly.20250727.bbbbbbb" {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
}

func TestNewestNightlyReleaseSameDatePrefersNewerPublishedAt(t *testing.T) {
	got, ok := newestNightlyRelease([]githubRelease{
		{TagName: "nightly.20250727.aaaaaaa", Prerelease: true, PublishedAt: "2025-07-27T01:00:00Z"},
		{TagName: "nightly.20250727.bbbbbbb", Prerelease: true, PublishedAt: "2025-07-27T03:00:00Z"},
	})
	if !ok || got != "nightly.20250727.bbbbbbb" {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
}

func TestCheckNightlySupportsLegacyRollingReleaseDuringMigration(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[{"tag_name":"nightly","name":"Bast nightly (nightly.20250725.deadbee)","prerelease":true}]`)),
		}, nil
	})}

	available, err := CheckNightly(context.Background(), client, "nightly.20250724.abc1234")
	if err != nil || available != "nightly.20250725.deadbee" {
		t.Fatalf("available=%q err=%v", available, err)
	}
}

func TestSuggestionRequiresInstallerReceiptForSelfUpdate(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "bast")
	if got := Suggestion(executable); got != "https://bast.sh" {
		t.Fatalf("source suggestion=%q", got)
	}
	if err := os.WriteFile(executable+receiptSuffix, []byte(InstallerURL+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Suggestion(executable); got != "bast update" || !ScriptInstalled(executable) {
		t.Fatalf("installer suggestion=%q installed=%t", got, ScriptInstalled(executable))
	}
	if err := os.WriteFile(executable+receiptSuffix, []byte(NightlyInstallerURL+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Suggestion(executable); got != "bast update" || !NightlyScriptInstalled(executable) {
		t.Fatalf("nightly installer suggestion=%q installed=%t", got, NightlyScriptInstalled(executable))
	}

	homebrew := filepath.Join(dir, "Cellar", "bast", "1.2.3", "bin", "bast")
	if got := Suggestion(homebrew); got != "brew upgrade bast" {
		t.Fatalf("Homebrew suggestion=%q", got)
	}
	homebrewNightly := filepath.Join(dir, "Cellar", "bast-nightly", "20250724", "bin", "bast")
	if got := Suggestion(homebrewNightly); got != "brew upgrade bast-nightly" {
		t.Fatalf("Homebrew nightly suggestion=%q", got)
	}
}

func TestUpdateRefusesUnmanagedExecutablesBeforeDownloading(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})}
	err := Update(context.Background(), client, filepath.Join(t.TempDir(), "bast"), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "only available") || called {
		t.Fatalf("err=%v called=%t", err, called)
	}
}

func TestUpdateRunsInstallerInTheExecutableDirectory(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "bast")
	if err := os.WriteFile(executable+receiptSuffix, []byte(InstallerURL+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s' \"$BAST_INSTALL_DIR\" > \"$BAST_INSTALL_DIR/update-result\"\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(script)),
		}, nil
	})}
	if err := Update(context.Background(), client, executable, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	result, err := os.ReadFile(filepath.Join(dir, "update-result"))
	if err != nil || string(result) != dir {
		t.Fatalf("installer directory=%q err=%v", result, err)
	}

	nonOKClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("<html>upstream failed</html>")),
		}, nil
	})}
	if err := os.Remove(filepath.Join(dir, "update-result")); err != nil {
		t.Fatal(err)
	}
	err = Update(context.Background(), nonOKClient, executable, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "download installer: unexpected response") {
		t.Fatalf("unexpected non-200 error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "update-result")); !os.IsNotExist(statErr) {
		t.Fatalf("installer executed for non-200 response: %v", statErr)
	}
}

func TestUpdateUsesNightlyInstallerReceipt(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "bast")
	if err := os.WriteFile(executable+receiptSuffix, []byte(NightlyInstallerURL+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var gotURL string
	var gotPinned string
	script := "#!/bin/sh\nprintf '%s' \"$BAST_NIGHTLY_VERSION\" > \"$BAST_INSTALL_DIR/pinned\"\nprintf '%s' \"$BAST_INSTALL_DIR\" > \"$BAST_INSTALL_DIR/update-result\"\n"
	releases := `[{"tag_name":"nightly.20260801.30f41d9","name":"Bast nightly (nightly.20260801.30f41d9)","prerelease":true,"published_at":"2026-08-01T15:35:04Z"},{"tag_name":"nightly.20260801.f248f48","name":"Bast nightly (nightly.20260801.f248f48)","prerelease":true,"published_at":"2026-08-01T10:51:40Z"}]`
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := script
		if strings.Contains(req.URL.String(), "/releases") {
			body = releases
		} else {
			gotURL = req.URL.String()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
	if err := Update(context.Background(), client, executable, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if gotURL != NightlyInstallerURL {
		t.Fatalf("installer URL=%q", gotURL)
	}
	pinned, err := os.ReadFile(filepath.Join(dir, "pinned"))
	if err != nil {
		t.Fatal(err)
	}
	gotPinned = string(pinned)
	if gotPinned != "nightly.20260801.30f41d9" {
		t.Fatalf("pinned nightly=%q", gotPinned)
	}
}
