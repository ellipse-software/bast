package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bast/internal/cloud/sandboxfake"
	vercelcloud "bast/internal/cloud/vercel"
	"bast/internal/openssh"
	"bast/internal/sshconfig"
)

func TestVercelCLILifecycleAgainstFakeAPI(t *testing.T) {
	home := t.TempDir()
	api := sandboxfake.NewVercel(t)
	t.Setenv(vercelcloud.BaseURLEnv, api.URL())
	t.Setenv(vercelcloud.TokenEnv, sandboxfake.VercelToken)
	t.Setenv(vercelcloud.TeamEnv, sandboxfake.VercelTeam)
	t.Setenv(vercelcloud.ProjectEnv, sandboxfake.VercelProject)

	out, errOut, err := runTestCLI(t, home, openssh.Client{}, "--json", "vercel", "new", "--name", "dev", "--vcpus", "2")
	if err != nil {
		t.Fatalf("new stderr=%q err=%v", errOut, err)
	}
	payload := decodeCLIJSON(t, out)
	if payload["ok"] != true {
		t.Fatalf("new json=%s", out)
	}
	data := payload["data"].(map[string]any)
	if data["alias"] != "vercel_dev" || data["provider"] != "vercel" {
		t.Fatalf("new data=%v", data)
	}
	if data["count"].(float64) != 1 {
		t.Fatalf("new count=%v", data["count"])
	}

	configPath := filepath.Join(home, ".ssh", "bast", "sync", "vercel", "config")
	blocks, err := sshconfig.LoadSyncHosts(configPath)
	if err != nil || len(blocks) != 1 || blocks[0].Alias != "vercel_dev" {
		t.Fatalf("ssh config blocks=%+v err=%v", blocks, err)
	}

	out, errOut, err = runTestCLI(t, home, openssh.Client{}, "--json", "vercel", "stop", "vercel_dev")
	if err != nil {
		t.Fatalf("stop stderr=%q err=%v out=%q", errOut, err, out)
	}
	if !strings.Contains(out, `"ok":true`) || api.Get("dev") == nil || api.Get("dev").Status != "stopped" {
		t.Fatalf("stop out=%q sandbox=%+v", out, api.Get("dev"))
	}

	out, errOut, err = runTestCLI(t, home, openssh.Client{}, "--json", "vercel", "resume", "dev")
	if err != nil {
		t.Fatalf("resume stderr=%q err=%v out=%q", errOut, err, out)
	}
	if api.Get("dev").Status != "running" {
		t.Fatalf("resume status=%s out=%q", api.Get("dev").Status, out)
	}

	out, errOut, err = runTestCLI(t, home, openssh.Client{}, "--json", "vercel", "fork", "--name", "dev-fork", "vercel_dev")
	if err != nil {
		t.Fatalf("fork stderr=%q err=%v out=%q", errOut, err, out)
	}
	payload = decodeCLIJSON(t, out)
	data = payload["data"].(map[string]any)
	if data["alias"] != "vercel_dev-fork" || data["count"].(float64) != 2 {
		t.Fatalf("fork data=%v", data)
	}

	out, errOut, err = runTestCLI(t, home, openssh.Client{}, "--json", "vercel", "delete", "--yes", "vercel_dev-fork")
	if err != nil {
		t.Fatalf("delete stderr=%q err=%v out=%q", errOut, err, out)
	}
	if api.Get("dev-fork") != nil {
		t.Fatal("fork still present after delete")
	}
	blocks, err = sshconfig.LoadSyncHosts(configPath)
	if err != nil || len(blocks) != 1 || blocks[0].Alias != "vercel_dev" {
		t.Fatalf("after delete blocks=%+v err=%v", blocks, err)
	}

	api.Put(sandboxfake.VercelSandbox{Name: "dead", Status: "failed", Persistent: false})
	out, errOut, err = runTestCLI(t, home, openssh.Client{}, "--json", "vercel", "cleanup", "--yes")
	if err != nil {
		t.Fatalf("cleanup stderr=%q err=%v out=%q", errOut, err, out)
	}
	payload = decodeCLIJSON(t, out)
	data = payload["data"].(map[string]any)
	deleted, _ := data["deleted"].([]any)
	if len(deleted) != 1 || deleted[0] != "dead" {
		t.Fatalf("cleanup data=%v", data)
	}
	if api.Get("dead") != nil {
		t.Fatal("cleanup left dead sandbox")
	}

	out, errOut, err = runTestCLI(t, home, openssh.Client{}, "vercel", "--help")
	if err != nil || errOut != "" || !strings.Contains(out, "Usage: bast vercel") {
		t.Fatalf("help out=%q stderr=%q err=%v", out, errOut, err)
	}
}

func TestDoctorJSONContentWithoutOpenSSHFixtures(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runTestCLI(t, home, openssh.Client{}, "--json", "doctor")
	if errOut != "" {
		t.Fatalf("stderr=%q", errOut)
	}
	payload := decodeCLIJSON(t, out)
	if payload["ok"] != true {
		t.Fatalf("json=%s err=%v", out, err)
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		t.Fatalf("missing data: %s", out)
	}
	if _, ok := data["healthy"]; !ok {
		t.Fatalf("missing healthy: %s", out)
	}
	if _, ok := data["summary"]; !ok {
		t.Fatalf("missing summary: %s", out)
	}
	if _, ok := data["findings"]; !ok {
		t.Fatalf("missing findings: %s", out)
	}
}

func TestVercelCLIDoesNotSkipOnThisGOOS(t *testing.T) {
	if runtime.GOOS == "" {
		t.Fatal("GOOS unset")
	}
	home := t.TempDir()
	out, errOut, err := runTestCLI(t, home, openssh.Client{}, "vercel", "--help")
	if err != nil || !strings.Contains(out, "new") || !strings.Contains(out, "cleanup") {
		t.Fatalf("os=%s out=%q stderr=%q err=%v", runtime.GOOS, out, errOut, err)
	}
}

func TestVercelCLITestsStayOffline(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	forbidden := []string{
		strings.Join([]string{"t", "Skip"}, "."),
		strings.Join([]string{"api", "vercel", "com"}, "."),
		vercelcloud.DefaultBaseURL,
	}
	for _, needle := range forbidden {
		if strings.Contains(body, needle) {
			t.Fatalf("CLI Vercel tests must not skip or call the live API; found %q", needle)
		}
	}
	if !strings.Contains(body, "t.Setenv(vercelcloud.BaseURLEnv, api.URL())") {
		t.Fatal("CLI Vercel tests must override VERCEL_API_URL with the fake server")
	}
	if !strings.Contains(body, "sandboxfake.NewVercel") {
		t.Fatal("CLI Vercel tests must use the in-process fake")
	}
	for _, command := range []string{`"vercel", "new"`, `"vercel", "stop"`, `"vercel", "resume"`, `"vercel", "fork"`, `"vercel", "delete"`, `"vercel", "cleanup"`, `"vercel", "--help"`} {
		if !strings.Contains(body, command) {
			t.Fatalf("CLI lifecycle missing %s", command)
		}
	}
}

func decodeCLIJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return payload
}
