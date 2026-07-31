package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/vault"
)

type vaultPullMsg struct {
	changed       bool
	revision      string
	err           error
	notice        string
	badPassphrase bool
}

type vaultPushMsg struct {
	revision         string
	err              error
	badPassphrase    bool
	resetPassphrase  bool
	rotatePassphrase bool
	passphrase       string
}

func (m *App) vaultSessionPath() string {
	return vault.SessionPath(m.paths.StateFile)
}

func (m *App) vaultLinked() bool {
	session, err := vault.LoadSession(m.vaultSessionPath())
	return err == nil && session.Token != ""
}

func (m *App) vaultStatusDetail() string {
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil || session.Token == "" {
		return "not linked"
	}
	if session.Revision == "" {
		return session.Email
	}
	return session.Email + " · " + shortRevision(session.Revision)
}

func shortRevision(rev string) string {
	if len(rev) <= 8 {
		return rev
	}
	return rev[:8]
}

func (m *App) vaultMenuItems() []syncMenuItem {
	if !m.vaultLinked() {
		return []syncMenuItem{
			{label: "Link account", action: "vault_login", detail: "email + code"},
		}
	}
	if m.vaultPassphrase == "" {
		return []syncMenuItem{
			{label: "Unlock", action: "vault_unlock", detail: "enter passphrase"},
			{label: "Rotate passphrase", action: "vault_rotate_passphrase", detail: "needs current"},
			{label: "Reset passphrase", action: "vault_reset_passphrase", detail: "overwrite remote"},
			{label: "Pull now", action: "vault_pull"},
			{label: "Push now", action: "vault_push"},
			{label: "Log out", action: "vault_logout"},
		}
	}
	return []syncMenuItem{
		{label: "Pull now", action: "vault_pull"},
		{label: "Push now", action: "vault_push"},
		{label: "Rotate passphrase", action: "vault_rotate_passphrase", detail: "needs current"},
		{label: "Reset passphrase", action: "vault_reset_passphrase", detail: "overwrite remote"},
		{label: "Log out", action: "vault_logout"},
	}
}

func (m *App) renderVaultStatus(s styleSet) string {
	width := m.terminalWidth()
	var b strings.Builder
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil || session.Token == "" {
		b.WriteString(compactRow(s, "Status", "not linked", width))
		b.WriteString("  " + s.muted.Render("Sync Bast-managed hosts and keys between machines.") + "\n")
		b.WriteString("  " + s.muted.Render("Encrypted locally. Bast never sees your passphrase.") + "\n")
		return b.String()
	}
	b.WriteString(compactRow(s, "Status", "linked", width))
	b.WriteString(compactRow(s, "Email", session.Email, width))
	if m.vaultPassphrase == "" {
		b.WriteString(compactRow(s, "Session", "locked", width))
	} else {
		b.WriteString(compactRow(s, "Session", "unlocked", width))
	}
	b.WriteString(compactRow(s, "Revision", noneValue(session.Revision), width))
	if m.vaultLastSync != "" {
		b.WriteString(compactRow(s, "Last sync", m.vaultLastSync, width))
	}
	if m.vaultStatus != "" {
		b.WriteString(compactRow(s, "Note", m.vaultStatus, width))
	}
	return b.String()
}

func (m *App) openVaultLoginForm() {
	m.openForm("Vault — link account", "vault_login", []field{
		{label: "Email", description: "We'll email a one-time code", placeholder: "you@example.com"},
	})
}

func (m *App) openVaultCodeForm(email string) {
	m.openForm("Vault — enter code", "vault_code", []field{
		{label: "Email", value: email, hidden: true},
		{label: "Code", description: "6-digit code from email", placeholder: "123456"},
	})
}

func (m *App) openVaultPassphraseForm(email, token, userID, deviceID, apiBase string, firstLink bool) {
	fields := []field{
		{label: "Email", value: email, hidden: true},
		{label: "Token", value: token, hidden: true},
		{label: "UserID", value: userID, hidden: true},
		{label: "DeviceID", value: deviceID, hidden: true},
		{label: "APIBase", value: apiBase, hidden: true},
		{label: "FirstLink", value: fmt.Sprintf("%t", firstLink), hidden: true},
		{label: "Passphrase", description: "Local encryption key — never sent to Bast", placeholder: "vault passphrase", secret: true},
		{label: "Confirm", description: "Re-enter passphrase", placeholder: "confirm passphrase", secret: true},
	}
	m.openForm("Vault — encryption passphrase", "vault_passphrase", fields)
}

func (m *App) openVaultUnlockForm(next string) {
	m.openForm("Vault — unlock", "vault_unlock", []field{
		{label: "Next", value: next, hidden: true},
		{label: "Passphrase", description: "Local encryption key — never sent to Bast", placeholder: "vault passphrase", secret: true},
	})
}

func (m *App) openVaultResetPassphraseForm() {
	m.openForm("Vault — reset passphrase", "vault_reset_passphrase", []field{
		{label: "Confirmation", value: "RESET", hidden: true},
		{label: "Type RESET to confirm", description: "Overwrites the remote vault with this machine", placeholder: "RESET"},
		{label: "Passphrase", description: "New local encryption key — never sent to Bast", placeholder: "new passphrase", secret: true},
		{label: "Confirm", description: "Re-enter new passphrase", placeholder: "confirm passphrase", secret: true},
	})
}

func (m *App) openVaultRotatePassphraseForm() {
	fields := []field{
		{label: "Current", description: "Current vault passphrase", placeholder: "current passphrase", secret: true},
		{label: "New", description: "New local encryption key — never sent to Bast", placeholder: "new passphrase", secret: true},
		{label: "Confirm", description: "Re-enter new passphrase", placeholder: "confirm passphrase", secret: true},
	}
	if m.vaultPassphrase != "" {
		fields[0].value = m.vaultPassphrase
		fields[0].hidden = true
	}
	m.openForm("Vault — rotate passphrase", "vault_rotate_passphrase", fields)
}

func (m *App) submitVaultLogin() tea.Cmd {
	email := strings.ToLower(strings.TrimSpace(m.formValue("Email")))
	if email == "" || !strings.Contains(email, "@") {
		m.form.validationError = "Enter a valid email"
		return nil
	}
	apiBase := strings.TrimSpace(os.Getenv("BAST_VAULT_API"))
	if apiBase == "" {
		apiBase = vault.DefaultAPIBase
	}
	client := &vault.Client{BaseURL: apiBase}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		err := client.StartOTP(ctx, email)
		return vaultOTPStartedMsg{email: email, apiBase: apiBase, err: err}
	}
}

type vaultOTPStartedMsg struct {
	email   string
	apiBase string
	err     error
}

type vaultOTPVerifiedMsg struct {
	email    string
	apiBase  string
	token    string
	userID   string
	deviceID string
	err      error
}

func (m *App) submitVaultCode() tea.Cmd {
	email := m.formValue("Email")
	code := vault.NormalizeOTP(m.formValue("Code"))
	if code == "" {
		m.form.validationError = "Enter the code from your email"
		return nil
	}
	apiBase := strings.TrimSpace(os.Getenv("BAST_VAULT_API"))
	if apiBase == "" {
		apiBase = vault.DefaultAPIBase
	}
	client := &vault.Client{BaseURL: apiBase}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		verified, err := client.VerifyOTP(ctx, email, code)
		return vaultOTPVerifiedMsg{
			email: email, apiBase: apiBase,
			token: verified.Token, userID: verified.UserID, deviceID: verified.DeviceID,
			err: err,
		}
	}
}

func (m *App) submitVaultPassphrase() tea.Cmd {
	pass := m.formValue("Passphrase")
	confirm := m.formValue("Confirm")
	if pass == "" {
		m.form.validationError = "Passphrase is required"
		return nil
	}
	if pass != confirm {
		m.form.validationError = "Passphrases did not match"
		return nil
	}
	session := vault.Session{
		Email:    m.formValue("Email"),
		Token:    m.formValue("Token"),
		UserID:   m.formValue("UserID"),
		DeviceID: m.formValue("DeviceID"),
		APIBase:  m.formValue("APIBase"),
	}
	passphrase := pass
	return func() tea.Msg {
		return m.vaultCompleteLink(session, passphrase)
	}
}

func (m *App) submitVaultUnlock() tea.Cmd {
	pass := m.formValue("Passphrase")
	if pass == "" {
		m.form.validationError = "Passphrase is required"
		return nil
	}
	next := m.formValue("Next")
	passphrase := pass
	return func() tea.Msg {
		return m.vaultVerifyUnlock(passphrase, next)
	}
}

func (m *App) submitVaultResetPassphrase() tea.Cmd {
	typed := strings.TrimSpace(m.formValue("Type RESET to confirm"))
	if typed != "RESET" {
		m.form.validationError = "Type RESET to confirm"
		return nil
	}
	pass := m.formValue("Passphrase")
	confirm := m.formValue("Confirm")
	if pass == "" {
		m.form.validationError = "Passphrase is required"
		return nil
	}
	if pass != confirm {
		m.form.validationError = "Passphrases did not match"
		return nil
	}
	passphrase := pass
	return func() tea.Msg {
		return m.vaultForceResetPassphrase(passphrase)
	}
}

func (m *App) submitVaultRotatePassphrase() tea.Cmd {
	current := m.formValue("Current")
	if current == "" {
		current = m.vaultPassphrase
	}
	if current == "" {
		m.form.validationError = "Current passphrase is required"
		return nil
	}
	newPass := m.formValue("New")
	confirm := m.formValue("Confirm")
	if newPass == "" {
		m.form.validationError = "New passphrase is required"
		return nil
	}
	if newPass != confirm {
		m.form.validationError = "Passphrases did not match"
		return nil
	}
	oldPass, nextPass := current, newPass
	return func() tea.Msg {
		return m.vaultRotatePassphrase(oldPass, nextPass)
	}
}

func (m *App) vaultForceResetPassphrase(passphrase string) tea.Msg {
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil || session.Token == "" {
		return vaultPushMsg{err: fmt.Errorf("vault is not linked")}
	}
	client := &vault.Client{BaseURL: session.APIBase, Token: session.Token}
	if env := strings.TrimSpace(os.Getenv("BAST_VAULT_API")); env != "" {
		client.BaseURL = env
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return vaultPushMsg{err: err}
	}
	packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata}
	doc, err := packer.Pack()
	if err != nil {
		return vaultPushMsg{err: err}
	}
	blob, err := vault.Encrypt(doc, passphrase)
	if err != nil {
		return vaultPushMsg{err: err}
	}
	meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
	if err != nil {
		return vaultPushMsg{err: err}
	}
	session.Revision = meta.Revision
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		return vaultPushMsg{err: err}
	}
	if err := vault.SavePassphrase(m.vaultPassphrasePath(), passphrase); err != nil {
		return vaultPushMsg{err: err}
	}
	return vaultPushMsg{revision: meta.Revision, resetPassphrase: true, passphrase: passphrase}
}

func (m *App) vaultRotatePassphrase(oldPass, newPass string) tea.Msg {
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil || session.Token == "" {
		return vaultPushMsg{err: fmt.Errorf("vault is not linked")}
	}
	client := &vault.Client{BaseURL: session.APIBase, Token: session.Token}
	if env := strings.TrimSpace(os.Getenv("BAST_VAULT_API")); env != "" {
		client.BaseURL = env
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return vaultPushMsg{err: err}
	}
	var doc vault.Document
	if len(remoteGet.Ciphertext) > 0 {
		doc, err = vault.Decrypt(remoteGet.Ciphertext, oldPass)
		if err != nil {
			return vaultPushMsg{err: err, badPassphrase: true}
		}
	} else {
		packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata}
		doc, err = packer.Pack()
		if err != nil {
			return vaultPushMsg{err: err}
		}
	}
	blob, err := vault.Encrypt(doc, newPass)
	if err != nil {
		return vaultPushMsg{err: err}
	}
	meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
	if err != nil {
		return vaultPushMsg{err: err}
	}
	session.Revision = meta.Revision
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		return vaultPushMsg{err: err}
	}
	if err := vault.SavePassphrase(m.vaultPassphrasePath(), newPass); err != nil {
		return vaultPushMsg{err: err}
	}
	return vaultPushMsg{revision: meta.Revision, rotatePassphrase: true, passphrase: newPass}
}

type vaultUnlockedMsg struct {
	passphrase string
	next       string
	err        error
}

func (m *App) vaultVerifyUnlock(passphrase, next string) tea.Msg {
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil || session.Token == "" {
		return vaultUnlockedMsg{err: fmt.Errorf("vault is not linked")}
	}
	client := &vault.Client{BaseURL: session.APIBase, Token: session.Token}
	if env := strings.TrimSpace(os.Getenv("BAST_VAULT_API")); env != "" {
		client.BaseURL = env
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return vaultUnlockedMsg{err: err}
	}
	if len(remoteGet.Ciphertext) > 0 {
		if err := vault.VerifyPassphrase(remoteGet.Ciphertext, passphrase); err != nil {
			return vaultUnlockedMsg{err: err}
		}
	}
	return vaultUnlockedMsg{passphrase: passphrase, next: next}
}

func (m *App) formValue(label string) string {
	if m.form == nil {
		return ""
	}
	if f := m.form.fieldByLabel(label); f != nil {
		return f.value
	}
	return ""
}

func (m *App) vaultCompleteLink(session vault.Session, passphrase string) tea.Msg {
	client := &vault.Client{BaseURL: session.APIBase, Token: session.Token}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return vaultPullMsg{err: err}
	}
	packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata}
	localDoc, err := packer.Pack()
	if err != nil {
		return vaultPullMsg{err: err}
	}
	merged := localDoc
	notice := "Vault linked"
	if len(remoteGet.Ciphertext) > 0 {
		remoteDoc, decErr := vault.Decrypt(remoteGet.Ciphertext, passphrase)
		if decErr != nil {
			return vaultPullMsg{err: decErr}
		}
		result := vault.Merge(localDoc, remoteDoc, vault.MergeModeMerge)
		if len(result.Conflicts) > 0 {
			return vaultPullMsg{err: fmt.Errorf("%d vault conflicts; run bast vault pull --mode replace_local or replace_remote", len(result.Conflicts))}
		}
		merged = result.Document
		applier := vault.Applier{Paths: m.paths, Config: m.config, Store: m.metadata}
		if err := applier.Apply(merged); err != nil {
			return vaultPullMsg{err: err}
		}
		notice = fmt.Sprintf("Vault linked · merged %d local / %d remote hosts", result.Summary.LocalHosts, result.Summary.RemoteHosts)
	}
	blob, err := vault.Encrypt(merged, passphrase)
	if err != nil {
		return vaultPullMsg{err: err}
	}
	meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
	if err != nil {
		return vaultPullMsg{err: err}
	}
	session.Revision = meta.Revision
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		return vaultPullMsg{err: err}
	}
	if err := vault.SavePassphrase(m.vaultPassphrasePath(), passphrase); err != nil {
		return vaultPullMsg{err: err}
	}
	m.vaultPassphrase = passphrase
	return vaultPullMsg{changed: true, revision: meta.Revision, notice: notice}
}

func (m *App) vaultPassphrasePath() string {
	return vault.PassphrasePath(m.paths.StateFile)
}

func (m *App) rememberVaultPassphrase(passphrase string) {
	m.vaultPassphrase = passphrase
	_ = vault.SavePassphrase(m.vaultPassphrasePath(), passphrase)
}

func (m *App) forgetVaultPassphrase() {
	m.vaultPassphrase = ""
	_ = vault.ClearPassphrase(m.vaultPassphrasePath())
}

func (m *App) vaultPullCmd(interactive bool) tea.Cmd {
	if !m.vaultLinked() {
		return nil
	}
	return func() tea.Msg {
		session, err := vault.LoadSession(m.vaultSessionPath())
		if err != nil {
			return vaultPullMsg{err: err}
		}
		passphrase := m.vaultPassphrase
		if passphrase == "" {
			if stored, loadErr := vault.LoadPassphrase(m.vaultPassphrasePath()); loadErr == nil {
				passphrase = stored
			}
		}
		if passphrase == "" {
			if !interactive {
				return vaultPullMsg{} // skip background unlock without passphrase
			}
			return vaultPullMsg{err: fmt.Errorf("vault passphrase required")}
		}
		client := &vault.Client{BaseURL: session.APIBase, Token: session.Token}
		if env := strings.TrimSpace(os.Getenv("BAST_VAULT_API")); env != "" {
			client.BaseURL = env
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		// Interactive pulls always fetch the blob so the passphrase is verified.
		ifNoneMatch := session.Revision
		if interactive {
			ifNoneMatch = ""
		}
		remoteGet, err := client.GetVault(ctx, ifNoneMatch)
		if err != nil {
			return vaultPullMsg{err: err}
		}
		if remoteGet.NotModified {
			return vaultPullMsg{changed: false, revision: session.Revision, notice: "Vault already up to date"}
		}
		if len(remoteGet.Ciphertext) == 0 {
			if interactive {
				return vaultPullMsg{changed: false, revision: session.Revision, notice: "No remote vault yet"}
			}
			return vaultPullMsg{changed: false, revision: session.Revision}
		}
		remoteDoc, err := vault.Decrypt(remoteGet.Ciphertext, passphrase)
		if err != nil {
			return vaultPullMsg{err: err, badPassphrase: true}
		}
		if remoteGet.Meta.Revision != "" && remoteGet.Meta.Revision == session.Revision {
			return vaultPullMsg{changed: false, revision: session.Revision, notice: "Vault already up to date"}
		}
		packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata}
		localDoc, err := packer.Pack()
		if err != nil {
			return vaultPullMsg{err: err}
		}
		result := vault.Merge(localDoc, remoteDoc, vault.MergeModeMerge)
		if len(result.Conflicts) > 0 {
			return vaultPullMsg{err: fmt.Errorf("%d vault conflicts", len(result.Conflicts))}
		}
		applier := vault.Applier{Paths: m.paths, Config: m.config, Store: m.metadata}
		if err := applier.Apply(result.Document); err != nil {
			return vaultPullMsg{err: err}
		}
		session.Revision = remoteGet.Meta.Revision
		_ = vault.SaveSession(m.vaultSessionPath(), session)
		return vaultPullMsg{changed: true, revision: session.Revision, notice: "Vault pulled"}
	}
}

func (m *App) vaultPushCmd() tea.Cmd {
	if !m.vaultLinked() {
		return nil
	}
	return func() tea.Msg {
		passphrase := m.vaultPassphrase
		if passphrase == "" {
			if stored, err := vault.LoadPassphrase(m.vaultPassphrasePath()); err == nil {
				passphrase = stored
			}
		}
		if passphrase == "" {
			return vaultPushMsg{err: fmt.Errorf("vault passphrase required")}
		}
		session, err := vault.LoadSession(m.vaultSessionPath())
		if err != nil {
			return vaultPushMsg{err: err}
		}
		client := &vault.Client{BaseURL: session.APIBase, Token: session.Token}
		if env := strings.TrimSpace(os.Getenv("BAST_VAULT_API")); env != "" {
			client.BaseURL = env
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		remoteGet, err := client.GetVault(ctx, "")
		if err != nil {
			return vaultPushMsg{err: err}
		}
		if err := vault.VerifyPassphrase(remoteGet.Ciphertext, passphrase); err != nil {
			return vaultPushMsg{err: err, badPassphrase: true}
		}
		packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata}
		doc, err := packer.Pack()
		if err != nil {
			return vaultPushMsg{err: err}
		}
		blob, err := vault.Encrypt(doc, passphrase)
		if err != nil {
			return vaultPushMsg{err: err}
		}
		ifMatch := remoteGet.Meta.Revision
		if ifMatch == "" {
			ifMatch = session.Revision
		}
		meta, err := client.PutVault(ctx, blob, ifMatch)
		if err != nil {
			return vaultPushMsg{err: err}
		}
		session.Revision = meta.Revision
		_ = vault.SaveSession(m.vaultSessionPath(), session)
		return vaultPushMsg{revision: meta.Revision}
	}
}

func (m *App) scheduleVaultPush() tea.Cmd {
	if !m.vaultLinked() {
		return nil
	}
	if m.vaultPassphrase == "" {
		if stored, err := vault.LoadPassphrase(m.vaultPassphrasePath()); err == nil && stored != "" {
			m.vaultPassphrase = stored
		}
	}
	if m.vaultPassphrase == "" {
		return nil
	}
	m.vaultDirty = true
	id := m.vaultPushID + 1
	m.vaultPushID = id
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return vaultPushDebounceMsg{id: id}
	})
}

type vaultPushDebounceMsg struct{ id uint64 }

func (m *App) runVaultAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case "vault_login":
		m.openVaultLoginForm()
	case "vault_unlock":
		m.openVaultUnlockForm("")
	case "vault_rotate_passphrase":
		m.openVaultRotatePassphraseForm()
	case "vault_reset_passphrase":
		m.openVaultResetPassphraseForm()
	case "vault_pull":
		if m.vaultPassphrase == "" {
			if stored, err := vault.LoadPassphrase(m.vaultPassphrasePath()); err == nil && stored != "" {
				m.vaultPassphrase = stored
			}
		}
		if m.vaultPassphrase == "" {
			m.openVaultUnlockForm("vault_pull")
			return m, nil
		}
		m.vaultStatus = "pulling…"
		m.vaultBusy = "Pulling vault…"
		return m, m.vaultPullCmd(true)
	case "vault_push":
		if m.vaultPassphrase == "" {
			if stored, err := vault.LoadPassphrase(m.vaultPassphrasePath()); err == nil && stored != "" {
				m.vaultPassphrase = stored
			}
		}
		if m.vaultPassphrase == "" {
			m.openVaultUnlockForm("vault_push")
			return m, nil
		}
		m.vaultStatus = "pushing…"
		m.vaultBusy = "Pushing vault…"
		return m, m.vaultPushCmd()
	case "vault_logout":
		session, err := vault.LoadSession(m.vaultSessionPath())
		if err == nil && session.Token != "" {
			client := &vault.Client{BaseURL: session.APIBase, Token: session.Token}
			if env := strings.TrimSpace(os.Getenv("BAST_VAULT_API")); env != "" {
				client.BaseURL = env
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_ = client.Logout(ctx)
			cancel()
		}
		_ = vault.ClearSession(m.vaultSessionPath())
		m.forgetVaultPassphrase()
		m.vaultStatus = ""
		m.vaultLastSync = ""
		return m, m.setNotice("Vault logged out")
	}
	return m, nil
}
