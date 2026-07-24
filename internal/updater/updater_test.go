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

	available, err := checkFrom(context.Background(), client, "v1.3.9", "https://api.github.test/latest")
	if err != nil || available != "v1.4.0" {
		t.Fatalf("available=%q err=%v", available, err)
	}
	latest = "v1.3.9"
	available, err = checkFrom(context.Background(), client, "v1.3.9", "https://api.github.test/latest")
	if err != nil || available != "" {
		t.Fatalf("equal available=%q err=%v", available, err)
	}
	latest = "v1.3.8"
	available, err = checkFrom(context.Background(), client, "v1.3.9", "https://api.github.test/latest")
	if err != nil || available != "" {
		t.Fatalf("older available=%q err=%v", available, err)
	}
	before := requests
	available, err = checkFrom(context.Background(), client, "dev", "https://api.github.test/latest")
	if err != nil || available != "" || requests != before {
		t.Fatalf("development check available=%q err=%v requests=%d", available, err, requests-before)
	}
	if !IsStable("v1.2.3") || IsStable("dev") || IsStable("v1.2.3-beta") {
		t.Fatal("stable version detection is incorrect")
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

	homebrew := filepath.Join(dir, "Cellar", "bast", "1.2.3", "bin", "bast")
	if got := Suggestion(homebrew); got != "brew upgrade bast" {
		t.Fatalf("Homebrew suggestion=%q", got)
	}
}

func TestUpdateRefusesUnmanagedExecutablesBeforeDownloading(t *testing.T) {
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})}
	err := updateFrom(context.Background(), client, "https://bast.test/install", filepath.Join(t.TempDir(), "bast"), io.Discard, io.Discard)
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
}
