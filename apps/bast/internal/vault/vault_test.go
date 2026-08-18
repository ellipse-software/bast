package vault

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"bast/internal/metadata"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	doc := Document{
		Version:   DocumentVersion,
		UpdatedAt: time.Now().Unix(),
		Hosts: []HostEntry{{
			ManagedID: "abc123",
			Alias:     "prod",
			HostName:  "prod.example",
			UpdatedAt: 100,
		}},
		Keys: []KeyEntry{{
			Name:       "work",
			PrivatePEM: "-----BEGIN OPENSSH PRIVATE KEY-----\ntest\n-----END OPENSSH PRIVATE KEY-----\n",
			UpdatedAt:  100,
		}},
		Metadata: map[string]metadata.Host{
			"prod": {Label: "Production", Favorite: true},
		},
	}
	blob, err := Encrypt(doc, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hosts[0].Alias != "prod" || got.Keys[0].Name != "work" || !got.Metadata["prod"].Favorite {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if _, err := Decrypt(blob, "wrong"); err == nil {
		t.Fatal("expected wrong passphrase to fail")
	}
}

func TestMergeByManagedID(t *testing.T) {
	local := Document{
		UpdatedAt: 10,
		Hosts: []HostEntry{
			{ManagedID: "a", Alias: "alpha", HostName: "a.example", UpdatedAt: 10},
			{ManagedID: "b", Alias: "beta", HostName: "b.example", UpdatedAt: 5},
		},
		Keys: []KeyEntry{
			{Name: "work", Fingerprint: "fp1", PrivatePEM: "local", UpdatedAt: 10},
		},
	}
	remote := Document{
		UpdatedAt: 20,
		Hosts: []HostEntry{
			{ManagedID: "a", Alias: "alpha-renamed", HostName: "a.example", UpdatedAt: 20},
			{ManagedID: "c", Alias: "gamma", HostName: "c.example", UpdatedAt: 15},
		},
		Keys: []KeyEntry{
			{Name: "work", Fingerprint: "fp1", PrivatePEM: "remote", UpdatedAt: 20},
			{Name: "ci", Fingerprint: "fp2", PrivatePEM: "ci", UpdatedAt: 15},
		},
		Tombstones: Tombstones{Hosts: map[string]int64{"b": 30}},
	}
	result := Merge(local, remote, MergeModeMerge)
	if result.Summary.Conflicts != 0 {
		t.Fatalf("unexpected conflicts: %+v", result.Conflicts)
	}
	byID := map[string]HostEntry{}
	for _, h := range result.Document.Hosts {
		byID[h.ManagedID] = h
	}
	if byID["a"].Alias != "alpha-renamed" {
		t.Fatalf("expected remote rename to win: %+v", byID["a"])
	}
	if _, ok := byID["b"]; ok {
		t.Fatal("tombstoned host b should be absent")
	}
	if byID["c"].Alias != "gamma" {
		t.Fatalf("missing remote host c")
	}
	byFP := map[string]KeyEntry{}
	for _, k := range result.Document.Keys {
		byFP[k.Fingerprint] = k
	}
	if byFP["fp1"].PrivatePEM != "remote" || byFP["fp2"].Name != "ci" {
		t.Fatalf("keys = %+v", result.Document.Keys)
	}
}

func TestMergeAliasConflict(t *testing.T) {
	local := Document{Hosts: []HostEntry{{ManagedID: "a", Alias: "same", HostName: "a.example", UpdatedAt: 10}}}
	remote := Document{Hosts: []HostEntry{{ManagedID: "b", Alias: "same", HostName: "b.example", UpdatedAt: 20}}}
	result := Merge(local, remote, MergeModeMerge)
	if len(result.Conflicts) == 0 {
		t.Fatal("expected alias conflict")
	}
}

func TestMergeReplaceModes(t *testing.T) {
	local := Document{Hosts: []HostEntry{{ManagedID: "a", Alias: "local", HostName: "l.example", UpdatedAt: 1}}}
	remote := Document{Hosts: []HostEntry{{ManagedID: "b", Alias: "remote", HostName: "r.example", UpdatedAt: 2}}}
	got := Merge(local, remote, MergeModeReplaceLocal)
	if len(got.Document.Hosts) != 1 || got.Document.Hosts[0].Alias != "remote" {
		t.Fatalf("replace local: %+v", got.Document.Hosts)
	}
	got = Merge(local, remote, MergeModeReplaceRemote)
	if len(got.Document.Hosts) != 1 || got.Document.Hosts[0].Alias != "local" {
		t.Fatalf("replace remote: %+v", got.Document.Hosts)
	}
}

func TestPassphraseFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := PassphrasePath(dir + "/state.json")
	if err := SavePassphrase(path, "secret phrase"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPassphrase(path)
	if err != nil || got != "secret phrase" {
		t.Fatalf("got %q err %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("passphrase file should be owner-only, got %v", info.Mode())
	}
	if err := ClearPassphrase(path); err != nil {
		t.Fatal(err)
	}
	got, err = LoadPassphrase(path)
	if err != nil || got != "" {
		t.Fatalf("cleared passphrase still present: %q", got)
	}
}

func TestStagedSecretsAreRemovedWhenReplacementFails(t *testing.T) {
	for name, save := range map[string]func(string) error{
		"session": func(path string) error {
			return SaveSession(path, Session{Email: "ted@example.com", Token: "secret"})
		},
		"passphrase": func(path string) error {
			return SavePassphrase(path, "secret phrase")
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "destination")
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := save(path); err == nil {
				t.Fatal("expected replacement to fail for a directory destination")
			}
			if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("staged secret remains after failed replacement: %v", err)
			}
		})
	}
}

func TestVerifyPassphrase(t *testing.T) {
	blob, err := Encrypt(Document{Version: DocumentVersion}, "right")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassphrase(blob, "right"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassphrase(blob, "wrong"); err == nil {
		t.Fatal("expected wrong passphrase to fail")
	}
	if err := VerifyPassphrase(nil, "anything"); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeOTP(t *testing.T) {
	if got := NormalizeOTP("12-34-56"); got != "123456" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeOTP("12345"); got != "" {
		t.Fatalf("short code should be empty, got %q", got)
	}
	if got := NormalizeOTP("0123456"); got != "" {
		t.Fatalf("long code should be empty, got %q", got)
	}
}

func TestMarkKeyDeleted(t *testing.T) {
	doc := Document{
		Keys: []KeyEntry{{Name: "work", Fingerprint: "fp1", UpdatedAt: 10}},
	}
	doc.MarkKeyDeleted("fp1", "work", 99)
	if doc.Keys[0].DeletedAt != 99 {
		t.Fatalf("DeletedAt = %d", doc.Keys[0].DeletedAt)
	}
	if doc.Tombstones.Keys["fp1"] != 99 {
		t.Fatalf("tombstone = %v", doc.Tombstones.Keys)
	}
}

func TestHostEntryEqualIgnoresUpdatedAt(t *testing.T) {
	a := HostEntry{ManagedID: "a", Alias: "x", HostName: "h", UpdatedAt: 1}
	b := HostEntry{ManagedID: "a", Alias: "x", HostName: "h", UpdatedAt: 99}
	if !hostEntryEqual(a, b) {
		t.Fatal("equal content should match regardless of UpdatedAt")
	}
	b.Alias = "y"
	if hostEntryEqual(a, b) {
		t.Fatal("different alias should not match")
	}
}

func TestNormalizeAndEffectiveAPIBase(t *testing.T) {
	if got := NormalizeAPIBase(" https://example.com/ "); got != "https://example.com" {
		t.Fatalf("NormalizeAPIBase = %q", got)
	}
	t.Setenv("BAST_VAULT_API", "")
	if got := EffectiveAPIBase(""); got != DefaultAPIBase {
		t.Fatalf("default = %q", got)
	}
	if got := EffectiveAPIBase("https://vault.example/"); got != "https://vault.example" {
		t.Fatalf("session = %q", got)
	}
	t.Setenv("BAST_VAULT_API", "https://env.example/")
	if got := EffectiveAPIBase("https://vault.example"); got != "https://env.example" {
		t.Fatalf("env override = %q", got)
	}
}
