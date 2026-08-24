package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForHomeUsesDotConfig(t *testing.T) {
	home := t.TempDir()
	p := ForHome(home)
	want := filepath.Join(home, ".config", "bast", "state.json")
	if p.StateFile != want {
		t.Fatalf("StateFile = %q, want %q", p.StateFile, want)
	}
	if p.SyncAWSConfig != filepath.Join(home, ".ssh", "bast", "sync", "aws", "config") {
		t.Fatalf("SyncAWSConfig = %q", p.SyncAWSConfig)
	}
	if p.SyncAzureConfig != filepath.Join(home, ".ssh", "bast", "sync", "azure", "config") {
		t.Fatalf("SyncAzureConfig = %q", p.SyncAzureConfig)
	}
	if p.SyncBoxConfig != filepath.Join(home, ".ssh", "bast", "sync", "box", "config") {
		t.Fatalf("SyncBoxConfig = %q", p.SyncBoxConfig)
	}
	if p.SyncUpstashConfig != filepath.Join(home, ".ssh", "bast", "sync", "upstash", "config") {
		t.Fatalf("SyncUpstashConfig = %q", p.SyncUpstashConfig)
	}
	if p.UpstashAPIKey != filepath.Join(home, ".config", "bast", "upstash-box-api-key") {
		t.Fatalf("UpstashAPIKey = %q", p.UpstashAPIKey)
	}
	if p.SyncHetznerConfig != filepath.Join(home, ".ssh", "bast", "sync", "hetzner", "config") {
		t.Fatalf("SyncHetznerConfig = %q", p.SyncHetznerConfig)
	}
	if p.HetznerAPIKey != filepath.Join(home, ".config", "bast", "hetzner-api-token") {
		t.Fatalf("HetznerAPIKey = %q", p.HetznerAPIKey)
	}
	if p.HetznerTokenDir != filepath.Join(home, ".config", "bast", "hetzner", "tokens") {
		t.Fatalf("HetznerTokenDir = %q", p.HetznerTokenDir)
	}
	if p.AzureDir != filepath.Join(home, ".ssh", "bast", "azure") {
		t.Fatalf("AzureDir = %q", p.AzureDir)
	}
}

func TestMigrateStateFrom(t *testing.T) {
	home := t.TempDir()
	stateFile := filepath.Join(home, ".config", "bast", "state.json")
	legacyDir := filepath.Join(home, "Library", "Application Support", "bast")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(legacyDir, "state.json")
	const body = `{"version":3,"hosts":{}}`
	if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateStateFrom(legacy, stateFile); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("migrated contents = %q, want %q", got, body)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy state still present: %v", err)
	}
}

func TestMigrateStateFromNoopWhenPresent(t *testing.T) {
	home := t.TempDir()
	stateFile := filepath.Join(home, ".config", "bast", "state.json")
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, []byte(`{"version":3,"hosts":{"a":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	legacyDir := filepath.Join(home, "Library", "Application Support", "bast")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(legacyDir, "state.json")
	if err := os.WriteFile(legacy, []byte(`{"version":3,"hosts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateStateFrom(legacy, stateFile); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"version":3,"hosts":{"a":{}}}` {
		t.Fatalf("existing state was overwritten: %s", got)
	}
}
