package keys

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bast/internal/paths"
)

func keyManager(t *testing.T) Manager {
	t.Helper()
	home := t.TempDir()
	p := paths.ForHome(home)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	keygen := filepath.Join(bin, "ssh-keygen")
	script := "#!/bin/sh\nif [ \"$1\" = \"-lf\" ]; then\n  if grep -q mismatch \"$2\"; then echo '256 SHA256:other mismatch (ED25519)'; else echo '256 SHA256:test-fingerprint test-key (ED25519)'; fi\n  exit 0\nfi\nif [ \"$1\" = \"-y\" ]; then echo 'ssh-ed25519 AAA-derived test-key'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(keygen, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	sshadd := filepath.Join(bin, "ssh-add")
	if err := os.WriteFile(sshadd, []byte("#!/bin/sh\nif [ \"$1\" = \"-l\" ]; then exit 1; fi\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return Manager{Paths: p, SSHKeygen: keygen, SSHAdd: sshadd}
}

func TestDiscoverImportExportAndDelete(t *testing.T) {
	m := keyManager(t)
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "source")
	if err := os.WriteFile(source, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\ndata\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source+".pub", []byte("ssh-ed25519 AAA test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.Import(source, "", "work", ""); err != nil {
		t.Fatal(err)
	}
	list, err := m.Discover(context.Background(), map[string][]string{filepath.Join(m.Paths.ManagedKeys, "work"): {"prod"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Fingerprint != "SHA256:test-fingerprint" || !list[0].Managed || strings.Join(list[0].References, ",") != "prod" {
		t.Fatalf("unexpected keys: %+v", list)
	}
	exportDir := t.TempDir()
	if err := m.Export(list[0], exportDir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"work", "work.pub"} {
		if _, err := os.Stat(filepath.Join(exportDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Delete(list[0], "work"); err == nil || !strings.Contains(err.Error(), "referenced") {
		t.Fatalf("expected reference block, got %v", err)
	}
	list[0].References = nil
	if err := m.Delete(list[0], "wrong"); err == nil {
		t.Fatal("expected confirmation failure")
	}
	if err := m.Delete(list[0], "work"); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverInspectsMoreCandidatesThanWorkersAndMatchesAgent(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	if err := os.MkdirAll(p.ManagedKeys, 0700); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		name := fmt.Sprintf("key-%02d", i)
		if err := os.WriteFile(filepath.Join(p.ManagedKeys, name+".pub"), []byte("ssh-ed25519 AAA "+name+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	keygen := filepath.Join(bin, "ssh-keygen")
	keygenScript := "#!/bin/sh\nname=${2##*/}\nname=${name%.pub}\nprintf '256 SHA256:%s %s comment (ED25519)\\n' \"$name\" \"$name\"\n"
	if err := os.WriteFile(keygen, []byte(keygenScript), 0700); err != nil {
		t.Fatal(err)
	}
	sshAdd := filepath.Join(bin, "ssh-add")
	if err := os.WriteFile(sshAdd, []byte("#!/bin/sh\nprintf '256 SHA256:key-09 agent-listed (ED25519)\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}

	manager := Manager{Paths: p, SSHKeygen: keygen, SSHAdd: sshAdd}
	keys, err := manager.Discover(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 12 {
		t.Fatalf("keys = %d, want 12: %+v", len(keys), keys)
	}
	for i, key := range keys {
		name := fmt.Sprintf("key-%02d", i)
		if key.Name != name || key.Fingerprint != "SHA256:"+name || key.Comment != name+" comment" || key.Algorithm != "ED25519" {
			t.Fatalf("key %d = %+v", i, key)
		}
		if key.InAgent != (name == "key-09") {
			t.Fatalf("key %s InAgent = %t", name, key.InAgent)
		}
	}
}

func TestImportAcceptsPastedPrivateKey(t *testing.T) {
	m := keyManager(t)
	content := "-----BEGIN OPENSSH PRIVATE KEY-----\npasted-data\n-----END OPENSSH PRIVATE KEY-----\n"
	if err := m.Import(content, "", "pasted", ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.Paths.ManagedKeys, "pasted")
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != content {
		t.Fatalf("stored key content changed:\n%s", stored)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private key mode = %o", info.Mode().Perm())
	}
	public, err := os.ReadFile(path + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if string(public) != "ssh-ed25519 AAA-derived test-key\n" {
		t.Fatalf("derived public key = %q", public)
	}
}

func TestImportRejectsMismatchedPublicKey(t *testing.T) {
	m := keyManager(t)
	private := "-----BEGIN OPENSSH PRIVATE KEY-----\nprivate-data\n-----END OPENSSH PRIVATE KEY-----\n"
	err := m.Import(private, "ssh-ed25519 mismatch attached-key", "bad-pair", "")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatched key error, got %v", err)
	}
}

func TestImportDerivesPublicKeyWithOpenSSH(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not installed")
	}
	home := t.TempDir()
	source := filepath.Join(home, "source")
	if out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-f", source).CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, out)
	}
	private, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	m := Manager{Paths: paths.ForHome(home), SSHKeygen: keygen, SSHAdd: "ssh-add"}
	if err := m.Import(string(private), "", "derived", "edited comment"); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(m.Paths.ManagedKeys, "derived.pub")
	if out, err := exec.Command(keygen, "-lf", public).CombinedOutput(); err != nil {
		t.Fatalf("derived public key is invalid: %v: %s", err, out)
	}
	publicText, err := os.ReadFile(public)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.TrimSpace(string(publicText)), " edited comment") {
		t.Fatalf("edited comment was not saved: %q", publicText)
	}
}

func TestImportCanReplacePublicKeyComment(t *testing.T) {
	m := keyManager(t)
	private := "-----BEGIN OPENSSH PRIVATE KEY-----\nprivate-data\n-----END OPENSSH PRIVATE KEY-----\n"
	public := "ssh-ed25519 AAA-original old comment"
	if err := m.Import(private, public, "commented", "new descriptive comment"); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join(m.Paths.ManagedKeys, "commented.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "ssh-ed25519 AAA-original new descriptive comment\n" {
		t.Fatalf("public key = %q", stored)
	}
}

func TestSetCommentAfterImport(t *testing.T) {
	m := keyManager(t)
	private := "-----BEGIN OPENSSH PRIVATE KEY-----\nprivate-data\n-----END OPENSSH PRIVATE KEY-----\n"
	if err := m.Import(private, "", "editable", "original comment"); err != nil {
		t.Fatal(err)
	}
	key := Key{
		Name:        "editable",
		PrivatePath: filepath.Join(m.Paths.ManagedKeys, "editable"),
		PublicPath:  filepath.Join(m.Paths.ManagedKeys, "editable.pub"),
		Managed:     true,
	}
	if err := m.SetComment(key, "updated comment"); err != nil {
		t.Fatal(err)
	}
	public, err := PublicText(key)
	if err != nil {
		t.Fatal(err)
	}
	if public != "ssh-ed25519 AAA-derived updated comment" {
		t.Fatalf("updated public key = %q", public)
	}
	if err := m.SetComment(key, ""); err != nil {
		t.Fatal(err)
	}
	public, err = PublicText(key)
	if err != nil {
		t.Fatal(err)
	}
	if public != "ssh-ed25519 AAA-derived" {
		t.Fatalf("comment was not removed: %q", public)
	}
}

func TestSetCommentRejectsExternalKey(t *testing.T) {
	m := keyManager(t)
	path := filepath.Join(t.TempDir(), "external.pub")
	if err := os.WriteFile(path, []byte("ssh-ed25519 AAA external\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.SetComment(Key{PublicPath: path}, "changed"); err == nil || !strings.Contains(err.Error(), "external") {
		t.Fatalf("expected external-key error, got %v", err)
	}
}

func TestExportRefusesPartialOverwrite(t *testing.T) {
	m := keyManager(t)
	dir := t.TempDir()
	private := filepath.Join(dir, "key")
	public := private + ".pub"
	os.WriteFile(private, []byte("private"), 0600)
	os.WriteFile(public, []byte("public"), 0644)
	destination := t.TempDir()
	os.WriteFile(filepath.Join(destination, "key.pub"), []byte("existing"), 0644)
	err := m.Export(Key{Name: "key", PrivatePath: private, PublicPath: public}, destination)
	if err == nil {
		t.Fatal("expected overwrite error")
	}
	if _, err := os.Stat(filepath.Join(destination, "key")); !os.IsNotExist(err) {
		t.Fatal("private key was partially exported")
	}
}
