package ui

import (
	"context"
	"errors"
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

func (m *App) webDAVSubmission() (vault.Session, string, error) {
	cfg := &vault.WebDAVConfig{
		URL:      strings.TrimSpace(m.formValue("Directory URL")),
		Username: strings.TrimSpace(m.formValue("Username")),
		Password: m.formValue("Password"),
	}
	if err := cfg.Validate(); err != nil {
		return vault.Session{}, "", err
	}
	pass := m.formValue("Passphrase")
	if pass == "" || pass != m.formValue("Confirm") {
		return vault.Session{}, "", errors.New("Encryption passphrases must match and must not be empty")
	}
	return vault.Session{Backend: "webdav", WebDAV: cfg}, pass, nil
}

func (m *App) linkVaultWebDAV(session vault.Session, pass string) tea.Cmd {
	opGen := m.vaultOpGen
	parent := m.vaultOpCtx
	if parent == nil {
		parent = context.Background()
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 60*time.Second)
		defer cancel()
		if err := vault.ClientForSession(session).CheckWebDAV(ctx); err != nil {
			return vaultPullMsg{err: err, interactive: true, linked: true, opGen: opGen}
		}
		if parent.Err() != nil {
			return vaultPullMsg{opGen: opGen}
		}
		return withVaultOp(m.vaultCompleteLink(session, pass), opGen)
	}
}

func withVaultOp(msg tea.Msg, opGen uint64) tea.Msg {
	switch typed := msg.(type) {
	case vaultPullMsg:
		typed.opGen = opGen
		return typed
	case vaultConflictMsg:
		typed.opGen = opGen
		return typed
	default:
		return msg
	}
}
