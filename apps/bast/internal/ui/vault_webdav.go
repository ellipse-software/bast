package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/vault"
)

func (m *App) openVaultWebDAVForm() {
	m.openForm("Vault: link WebDAV", "vault_webdav", []field{
		{label: "Directory URL", description: "Existing HTTPS WebDAV directory; stores bast-vault.json", placeholder: "https://dav.example.com/bast/"},
		{label: "Username", description: "WebDAV account username"},
		{label: "Password", description: "WebDAV password or app password; stored only on this machine", secret: true},
		{label: "Passphrase", description: "Vault encryption passphrase; never sent to WebDAV", secret: true},
		{label: "Confirm", description: "Re-enter encryption passphrase", secret: true},
	})
}

func (m *App) submitVaultWebDAV() tea.Cmd {
	cfg := &vault.WebDAVConfig{
		URL:      strings.TrimSpace(m.formValue("Directory URL")),
		Username: m.formValue("Username"),
		Password: m.formValue("Password"),
	}
	if err := cfg.Validate(); err != nil {
		m.form.validationError = err.Error()
		return nil
	}
	pass := m.formValue("Passphrase")
	if pass == "" || pass != m.formValue("Confirm") {
		m.form.validationError = "Encryption passphrases must match and must not be empty"
		return nil
	}
	session := vault.Session{Backend: "webdav", WebDAV: cfg}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := vault.ClientForSession(session).CheckWebDAV(ctx); err != nil {
			return vaultPullMsg{err: err, interactive: true, linked: true}
		}
		return m.vaultCompleteLink(session, pass)
	}
}
