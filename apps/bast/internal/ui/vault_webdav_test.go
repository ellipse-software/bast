package ui

import (
	"os"
	"strings"
	"testing"

	"bast/internal/vault"
)

func TestVaultWebDAVEntryAndSecretFields(t *testing.T) {
	m := testApp(t)
	found := false
	for _, item := range m.vaultMenuItems() {
		if item.action == "vault_webdav" {
			found = true
		}
	}
	if !found {
		t.Fatal("unlinked vault has no WebDAV action")
	}
	m.runVaultAction("vault_webdav")
	if m.form == nil || m.form.action != "vault_webdav" {
		t.Fatal("WebDAV action did not open setup form")
	}
	for _, label := range []string{"Password", "Passphrase", "Confirm"} {
		if f := m.form.fieldByLabel(label); f == nil || !f.secret {
			t.Fatalf("%s is not a secret field", label)
		}
	}
	m.form.fieldByLabel("Directory URL").value = "https://dav.example.com/bast/"
	m.form.fieldByLabel("Username").value = "alice"
	m.form.fieldByLabel("Password").value = "app-password"
	m.form.fieldByLabel("Passphrase").value = "encryption-secret"
	m.form.fieldByLabel("Confirm").value = "different"
	if cmd := m.submitVaultWebDAV(); cmd != nil || m.form.validationError == "" {
		t.Fatal("mismatched passphrases accepted")
	}
	if _, err := os.Stat(m.vaultSessionPath()); !os.IsNotExist(err) {
		t.Fatal("invalid form persisted a session")
	}
}

func TestVaultWebDAVSessionCacheAndLogout(t *testing.T) {
	m := testApp(t)
	session := vault.Session{Backend: "webdav", WebDAV: &vault.WebDAVConfig{
		URL: "https://dav.example.com/bast/", Username: "alice", Password: "app-password",
	}}
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		t.Fatal(err)
	}
	m.refreshVaultSessionCache()
	if !m.vaultLinked() {
		t.Fatal("WebDAV session without API token is not linked")
	}
	body := m.renderVaultStatus(m.styles())
	if !strings.Contains(body, "WebDAV") || !strings.Contains(body, "dav.example.com") || strings.Contains(body, "app-password") {
		t.Fatalf("unsafe or missing WebDAV status: %s", body)
	}
	for _, item := range m.vaultMenuItems() {
		if item.action == "vault_api_base" || item.action == "vault_webdav" {
			t.Fatalf("linked WebDAV exposes backend switch: %+v", item)
		}
	}
	m.runVaultAction("vault_logout")
	if m.vaultLinked() {
		t.Fatal("logout retained linked WebDAV state")
	}
	if _, err := os.Stat(m.vaultSessionPath()); !os.IsNotExist(err) {
		t.Fatal("logout retained WebDAV credentials")
	}
}
