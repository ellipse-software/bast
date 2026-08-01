package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/telemetry"
	"bast/internal/vault"
)

type vaultPullMsg struct {
	changed       bool
	revision      string
	err           error
	notice        string
	badPassphrase bool
	interactive   bool
	thenPush      bool
	linked        bool   // first-link flow (vs pull/sync)
	passphrase    string // set on successful link; applied in Update
	session       *vault.Session
	opGen         uint64
}

type vaultConflictMsg struct {
	count       int
	thenPush    bool
	interactive bool
	forLink     bool
	passphrase  string
	session     *vault.Session
	apiBase     string
	opGen       uint64
}

type vaultConflictState struct {
	count       int
	thenPush    bool
	interactive bool
	forLink     bool
	passphrase  string
	session     vault.Session
	apiBase     string
}

type vaultPushMsg struct {
	revision         string
	err              error
	badPassphrase    bool
	resetPassphrase  bool
	rotatePassphrase bool
	synced           bool
	applied          bool // local state changed during merge-before-push
	passphrase       string
	session          *vault.Session
	opGen            uint64
}

func (m *App) beginVaultBusy(label string) {
	m.cancelVaultOp()
	m.vaultBusy = label
	m.vaultBusyHoldLoad = false
}

// keepVaultBusy updates the busy label without cancelling the in-flight op generation
// (used when Sync continues from pull into push).
func (m *App) keepVaultBusy(label string) {
	m.vaultBusy = label
	m.vaultBusyHoldLoad = false
}

func (m *App) clearVaultBusy() {
	m.vaultBusy = ""
	m.vaultBusyHoldLoad = false
}

func (m *App) cancelVaultOp() {
	m.vaultBusy = ""
	m.vaultBusyHoldLoad = false
	m.vaultRemoteRetry = false
	m.vaultOpGen++
}

// vaultBusyBlocksSync replaces the Sync/Vault menu while a vault network step runs
// (send code, verify, link, unlock, sync from this screen). Other sections keep their
// normal body and only show the footer busy hint.
func (m *App) vaultBusyBlocksSync() bool {
	return m.vaultBusy != "" && m.section == syncSection
}

func (m *App) renderVaultBusy(s styleSet) string {
	title := "Vault"
	if m.syncProvider != "" && m.syncProvider != "vault" {
		title = strings.ToUpper(m.syncProvider)
	}
	return "\n  " + s.active.Render(title) + "\n\n  " + s.muted.Render(m.vaultBusy)
}

func (m *App) openVaultConflictForm(msg vaultConflictMsg) {
	state := &vaultConflictState{
		count:       msg.count,
		thenPush:    msg.thenPush,
		interactive: msg.interactive,
		forLink:     msg.forLink,
		passphrase:  msg.passphrase,
		apiBase:     msg.apiBase,
	}
	if msg.session != nil {
		state.session = *msg.session
	}
	m.vaultConflict = state
	m.openForm(fmt.Sprintf("Vault: %d conflicts", msg.count), "vault_resolve", []field{
		{
			label:       "Resolution",
			description: "Same host alias or key name on both sides with different identities",
			options: []fieldOption{
				{label: "Keep this machine (overwrite remote)", value: "replace_remote"},
				{label: "Keep remote (overwrite this machine)", value: "replace_local"},
				{label: "Cancel", value: "cancel"},
			},
		},
	})
}

func (m *App) submitVaultResolve() tea.Cmd {
	if m.form == nil || m.vaultConflict == nil {
		return nil
	}
	m.commitFormField()
	value := strings.TrimSpace(m.formValue("Resolution"))
	pending := m.vaultConflict
	m.form = nil
	if value == "" || value == "cancel" {
		m.vaultConflict = nil
		if pending.forLink {
			return m.setNotice("Link cancelled · choose keep local or keep remote to finish linking")
		}
		return m.setNotice("Vault sync cancelled")
	}
	mode := vault.MergeMode(value)
	m.vaultConflict = nil
	m.beginVaultBusy("Resolving vault…")
	if pending.forLink {
		return m.vaultResolveLinkCmd(pending, mode)
	}
	return m.vaultResolvePullCmd(pending, mode)
}

func (m *App) vaultResolvePullCmd(pending *vaultConflictState, mode vault.MergeMode) tea.Cmd {
	passphrase := m.vaultPassphrase
	if passphrase == "" {
		passphrase = pending.passphrase
	}
	thenPush := pending.thenPush
	interactive := pending.interactive
	opGen := m.vaultOpGen
	return func() tea.Msg {
		session, err := vault.LoadSession(m.vaultSessionPath())
		if err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		remoteGet, err := client.GetVault(ctx, "")
		if err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		if len(remoteGet.Ciphertext) == 0 {
			return vaultPullMsg{err: fmt.Errorf("no remote vault"), interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		remoteDoc, err := vault.Decrypt(remoteGet.Ciphertext, passphrase)
		if err != nil {
			return vaultPullMsg{err: err, badPassphrase: true, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata, Previous: remoteDoc}
		localDoc, err := packer.Pack()
		if err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		result := vault.Merge(localDoc, remoteDoc, mode)
		applier := vault.Applier{Paths: m.paths, Config: m.config, Store: m.metadata}
		if err := applier.Apply(result.Document); err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		session.Revision = remoteGet.Meta.Revision
		_ = vault.SaveSession(m.vaultSessionPath(), session)
		sessionCopy := session
		notice := "Vault resolved · kept this machine"
		if mode == vault.MergeModeReplaceLocal {
			notice = "Vault resolved · kept remote"
		}
		return vaultPullMsg{changed: true, revision: session.Revision, notice: notice, interactive: interactive, thenPush: thenPush, session: &sessionCopy, opGen: opGen}
	}
}

func (m *App) vaultResolveLinkCmd(pending *vaultConflictState, mode vault.MergeMode) tea.Cmd {
	session := pending.session
	passphrase := pending.passphrase
	apiBase := pending.apiBase
	if apiBase != "" {
		session.APIBase = apiBase
	}
	opGen := m.vaultOpGen
	return func() tea.Msg {
		if strings.TrimSpace(session.Token) == "" {
			return vaultPullMsg{err: fmt.Errorf("vault link missing token"), interactive: true, linked: true, opGen: opGen}
		}
		if strings.TrimSpace(session.APIBase) == "" {
			session.APIBase = vault.DefaultAPIBase
		}
		if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
			return vaultPullMsg{err: err, interactive: true, linked: true, opGen: opGen}
		}
		if err := vault.SavePassphrase(m.vaultPassphrasePath(), passphrase); err != nil {
			sessionCopy := session
			return vaultPullMsg{err: err, interactive: true, linked: true, session: &sessionCopy, passphrase: passphrase, opGen: opGen}
		}
		sessionCopy := session
		withSession := func(msg vaultPullMsg) vaultPullMsg {
			cp := sessionCopy
			msg.session = &cp
			msg.passphrase = passphrase
			msg.interactive = true
			msg.linked = true
			msg.opGen = opGen
			return msg
		}

		client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		remoteGet, err := client.GetVault(ctx, "")
		if err != nil {
			return withSession(vaultPullMsg{err: err})
		}
		packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata}
		localDoc, err := packer.Pack()
		if err != nil {
			return withSession(vaultPullMsg{err: err})
		}
		merged := localDoc
		notice := "Vault linked"
		if len(remoteGet.Ciphertext) > 0 {
			remoteDoc, decErr := vault.Decrypt(remoteGet.Ciphertext, passphrase)
			if decErr != nil {
				return withSession(vaultPullMsg{err: decErr})
			}
			result := vault.Merge(localDoc, remoteDoc, mode)
			merged = result.Document
			applier := vault.Applier{Paths: m.paths, Config: m.config, Store: m.metadata}
			if err := applier.Apply(merged); err != nil {
				return withSession(vaultPullMsg{err: err})
			}
			notice = "Vault linked · kept this machine"
			if mode == vault.MergeModeReplaceLocal {
				notice = "Vault linked · kept remote"
			}
		}
		blob, err := vault.Encrypt(merged, passphrase)
		if err != nil {
			return withSession(vaultPullMsg{err: err})
		}
		meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
		if err != nil {
			return withSession(vaultPullMsg{err: err})
		}
		session.Revision = meta.Revision
		sessionCopy.Revision = meta.Revision
		if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
			return withSession(vaultPullMsg{err: err})
		}
		return withSession(vaultPullMsg{changed: true, revision: meta.Revision, notice: notice})
	}
}

func (m *App) vaultSessionPath() string {
	return vault.SessionPath(m.paths.StateFile)
}

func (m *App) setVaultSession(session *vault.Session) {
	m.vaultSession = session
	if session != nil {
		if base := vault.NormalizeAPIBase(session.APIBase); base != "" {
			m.vaultAPIBase = base
		}
	}
}

func (m *App) refreshVaultSessionCache() {
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil {
		m.vaultSession = nil
		m.vaultAPIBase = ""
		return
	}
	m.vaultAPIBase = vault.NormalizeAPIBase(session.APIBase)
	if session.Token == "" {
		m.vaultSession = nil
		return
	}
	cp := session
	m.vaultSession = &cp
}

func (m *App) vaultLinked() bool {
	if m.vaultSession != nil && m.vaultSession.Token != "" {
		return true
	}
	m.refreshVaultSessionCache()
	return m.vaultSession != nil && m.vaultSession.Token != ""
}

func (m *App) vaultStatusDetail() string {
	if !m.vaultLinked() || m.vaultSession == nil {
		return "not linked"
	}
	if m.vaultSession.Revision == "" {
		return m.vaultSession.Email
	}
	return m.vaultSession.Email + " · " + shortRevision(m.vaultSession.Revision)
}

func shortRevision(rev string) string {
	if len(rev) <= 8 {
		return rev
	}
	return rev[:8]
}

func (m *App) vaultMenuItems() []syncMenuItem {
	apiBaseItem := syncMenuItem{
		label:       "API base URL",
		action:      "vault_api_base",
		description: "Vault server for this machine (self-hosted or bast.sh)",
	}
	if !m.vaultLinked() {
		return []syncMenuItem{
			{label: "Link account", action: "vault_login", description: "Email a one-time code to link this machine"},
			apiBaseItem,
		}
	}
	if m.vaultPassphrase == "" {
		return []syncMenuItem{
			{label: "Unlock", action: "vault_unlock", description: "Enter passphrase to decrypt the vault"},
			{label: "Sync now", action: "vault_sync", description: syncNowDescription(m.vaultStatus)},
			{label: "Rotate passphrase", action: "vault_rotate_passphrase", description: "Requires the current passphrase"},
			{label: "Reset passphrase", action: "vault_reset_passphrase", description: "Overwrites the remote vault with this machine"},
			apiBaseItem,
			{label: "Log out", action: "vault_logout"},
		}
	}
	return []syncMenuItem{
		{label: "Sync now", action: "vault_sync", description: syncNowDescription(m.vaultStatus)},
		{label: "Rotate passphrase", action: "vault_rotate_passphrase", description: "Requires the current passphrase"},
		{label: "Reset passphrase", action: "vault_reset_passphrase", description: "Overwrites the remote vault with this machine"},
		apiBaseItem,
		{label: "Log out", action: "vault_logout"},
	}
}

func syncNowDescription(status string) string {
	if status != "" && (strings.Contains(status, "updated elsewhere") || strings.Contains(status, "conflict")) {
		return "Remote vault changed · sync to pull and resolve"
	}
	return "Keep this machine and the vault in sync"
}

func (m *App) renderVaultStatus(s styleSet) string {
	width := m.terminalWidth()
	var b strings.Builder
	if !m.vaultLinked() || m.vaultSession == nil {
		b.WriteString(compactRow(s, "Status", "not linked", width))
		b.WriteString(compactRow(s, "API base", m.preferredVaultAPIBase(), width))
		b.WriteString("  " + s.muted.Render("Syncs Bast-managed hosts, keys, and metadata.") + "\n")
		b.WriteString("  " + s.muted.Render("Does not sync external SSH config or cloud VMs.") + "\n")
		b.WriteString("  " + s.muted.Render("Encrypted locally. Bast never sees your passphrase.") + "\n")
		if m.vaultStatus != "" {
			b.WriteString(compactRow(s, "Note", m.vaultStatus, width))
		}
		return b.String()
	}
	session := m.vaultSession
	b.WriteString(compactRow(s, "Status", "linked", width))
	b.WriteString(compactRow(s, "Email", session.Email, width))
	b.WriteString(compactRow(s, "API base", vault.EffectiveAPIBase(session.APIBase), width))
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

func (m *App) preferredVaultAPIBase() string {
	if m.vaultSession != nil {
		return vault.EffectiveAPIBase(m.vaultSession.APIBase)
	}
	return vault.EffectiveAPIBase(m.vaultAPIBase)
}

func (m *App) openVaultLoginForm() {
	m.openForm("Vault: link account", "vault_login", []field{
		{label: "Email", description: "We'll email a one-time code", placeholder: "you@example.com"},
		{
			label:       "API base",
			description: "Vault server URL. Leave as bast.sh unless you self-host",
			value:       m.preferredVaultAPIBase(),
			placeholder: vault.DefaultAPIBase,
			optional:    true,
		},
	})
}

func (m *App) openVaultCodeForm(email, apiBase string) {
	m.openForm("Vault: enter code", "vault_code", []field{
		{label: "Email", value: email, hidden: true},
		{label: "APIBase", value: apiBase, hidden: true},
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
		{label: "Passphrase", description: "Encrypts the vault locally. Never sent to Bast", placeholder: "vault passphrase", secret: true},
		{label: "Confirm", description: "Re-enter passphrase", placeholder: "confirm passphrase", secret: true},
	}
	m.openForm("Vault: encryption passphrase", "vault_passphrase", fields)
}

func (m *App) openVaultUnlockForm(next string) {
	m.openForm("Vault: unlock", "vault_unlock", []field{
		{label: "Next", value: next, hidden: true},
		{label: "Passphrase", description: "Local encryption key. Never sent to Bast", placeholder: "vault passphrase", secret: true},
	})
}

func (m *App) openVaultResetPassphraseForm() {
	m.openForm("Vault: reset passphrase", "vault_reset_passphrase", []field{
		{label: "Confirmation", value: "RESET", hidden: true},
		{label: "Type RESET to confirm", description: "Overwrites the remote vault with this machine", placeholder: "RESET"},
		{label: "Passphrase", description: "New local encryption key. Never sent to Bast", placeholder: "new passphrase", secret: true},
		{label: "Confirm", description: "Re-enter new passphrase", placeholder: "confirm passphrase", secret: true},
	})
}

func (m *App) openVaultRotatePassphraseForm() {
	fields := []field{
		{label: "Current", description: "Current vault passphrase", placeholder: "current passphrase", secret: true},
		{label: "New", description: "New local encryption key. Never sent to Bast", placeholder: "new passphrase", secret: true},
		{label: "Confirm", description: "Re-enter new passphrase", placeholder: "confirm passphrase", secret: true},
	}
	if m.vaultPassphrase != "" {
		fields[0].value = m.vaultPassphrase
		fields[0].hidden = true
	}
	m.openForm("Vault: rotate passphrase", "vault_rotate_passphrase", fields)
}

func (m *App) openVaultAPIBaseForm() {
	desc := "Self-hosted vault origin, or https://bast.sh"
	if m.vaultLinked() {
		desc = "Must match the server for this session. Log out to switch servers"
	}
	m.openForm("Vault: API base URL", "vault_api_base", []field{
		{
			label:       "API base",
			description: desc,
			value:       m.preferredVaultAPIBase(),
			placeholder: vault.DefaultAPIBase,
		},
	})
}

func (m *App) submitVaultLogin() tea.Cmd {
	email := strings.ToLower(strings.TrimSpace(m.formValue("Email")))
	if email == "" || !strings.Contains(email, "@") {
		m.form.validationError = "Enter a valid email"
		return nil
	}
	apiBase, err := validateVaultAPIBase(m.formValue("API base"))
	if err != nil {
		m.form.validationError = err.Error()
		return nil
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
	apiBase, err := validateVaultAPIBase(m.formValue("APIBase"))
	if err != nil {
		m.form.validationError = err.Error()
		return nil
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

func validateVaultAPIBase(raw string) (string, error) {
	base := vault.NormalizeAPIBase(raw)
	if base == "" {
		base = vault.EffectiveAPIBase("")
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return "", fmt.Errorf("URL must start with http:// or https://")
	}
	return base, nil
}

func (m *App) submitVaultAPIBase() tea.Cmd {
	base, err := validateVaultAPIBase(m.formValue("API base"))
	if err != nil {
		m.form.validationError = err.Error()
		return nil
	}
	previous := m.preferredVaultAPIBase()
	linked := m.vaultLinked()
	session, loadErr := vault.LoadSession(m.vaultSessionPath())
	if loadErr != nil && !os.IsNotExist(loadErr) {
		m.form.validationError = loadErr.Error()
		return nil
	}
	if os.IsNotExist(loadErr) {
		session = vault.Session{}
	}
	session.APIBase = base
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		m.form.validationError = err.Error()
		return nil
	}
	m.vaultAPIBase = base
	if session.Token != "" {
		cp := session
		m.setVaultSession(&cp)
	} else {
		m.vaultSession = nil
	}
	notice := "API base set to " + base
	if linked && previous != base {
		notice = "API base updated · log out and link again if this is a different server"
	}
	return m.setNotice(notice)
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
	opGen := m.vaultOpGen
	return func() tea.Msg {
		msg := m.vaultForceResetPassphrase(passphrase)
		msg.opGen = opGen
		return msg
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
	opGen := m.vaultOpGen
	return func() tea.Msg {
		msg := m.vaultRotatePassphrase(oldPass, nextPass)
		msg.opGen = opGen
		return msg
	}
}

func (m *App) vaultForceResetPassphrase(passphrase string) vaultPushMsg {
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil || session.Token == "" {
		return vaultPushMsg{err: fmt.Errorf("vault is not linked")}
	}
	client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
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
	sessionCopy := session
	return vaultPushMsg{revision: meta.Revision, resetPassphrase: true, passphrase: passphrase, session: &sessionCopy}
}

func (m *App) vaultRotatePassphrase(oldPass, newPass string) vaultPushMsg {
	session, err := vault.LoadSession(m.vaultSessionPath())
	if err != nil || session.Token == "" {
		return vaultPushMsg{err: fmt.Errorf("vault is not linked")}
	}
	client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
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
	sessionCopy := session
	return vaultPushMsg{revision: meta.Revision, rotatePassphrase: true, passphrase: newPass, session: &sessionCopy}
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
	client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
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
	if strings.TrimSpace(session.Token) == "" {
		return vaultPullMsg{err: fmt.Errorf("vault link missing token"), interactive: true, linked: true}
	}
	if strings.TrimSpace(session.APIBase) == "" {
		session.APIBase = vault.DefaultAPIBase
	}
	// Persist auth before merge/apply/push so a later network error cannot leave
	// hosts/keys on disk while the machine still looks "not linked".
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		return vaultPullMsg{err: err, interactive: true, linked: true}
	}
	if err := vault.SavePassphrase(m.vaultPassphrasePath(), passphrase); err != nil {
		sessionCopy := session
		return vaultPullMsg{err: err, interactive: true, linked: true, session: &sessionCopy}
	}
	sessionCopy := session
	withSession := func(msg vaultPullMsg) vaultPullMsg {
		cp := sessionCopy
		msg.session = &cp
		msg.passphrase = passphrase
		msg.interactive = true
		msg.linked = true
		return msg
	}

	client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return withSession(vaultPullMsg{err: err})
	}
	packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata}
	localDoc, err := packer.Pack()
	if err != nil {
		return withSession(vaultPullMsg{err: err})
	}
	merged := localDoc
	notice := "Vault linked"
	if len(remoteGet.Ciphertext) > 0 {
		remoteDoc, decErr := vault.Decrypt(remoteGet.Ciphertext, passphrase)
		if decErr != nil {
			return withSession(vaultPullMsg{err: decErr})
		}
		result := vault.Merge(localDoc, remoteDoc, vault.MergeModeMerge)
		if len(result.Conflicts) > 0 {
			return vaultConflictMsg{
				count: len(result.Conflicts), forLink: true, passphrase: passphrase,
				session: &sessionCopy, interactive: true,
			}
		}
		merged = result.Document
		applier := vault.Applier{Paths: m.paths, Config: m.config, Store: m.metadata}
		if err := applier.Apply(merged); err != nil {
			return withSession(vaultPullMsg{err: err})
		}
		notice = fmt.Sprintf("Vault linked · merged %d local / %d remote hosts", result.Summary.LocalHosts, result.Summary.RemoteHosts)
	}
	blob, err := vault.Encrypt(merged, passphrase)
	if err != nil {
		return withSession(vaultPullMsg{err: err})
	}
	meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
	if err != nil {
		return withSession(vaultPullMsg{err: err})
	}
	session.Revision = meta.Revision
	sessionCopy.Revision = meta.Revision
	if err := vault.SaveSession(m.vaultSessionPath(), session); err != nil {
		return withSession(vaultPullMsg{err: err})
	}
	return withSession(vaultPullMsg{changed: true, revision: meta.Revision, notice: notice})
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
	return m.vaultPullCmdOpts(interactive, false)
}

func (m *App) vaultPullCmdOpts(interactive, thenPush bool) tea.Cmd {
	if !m.vaultLinked() {
		return nil
	}
	passphrase := m.vaultPassphrase
	if passphrase == "" {
		if stored, loadErr := vault.LoadPassphrase(m.vaultPassphrasePath()); loadErr == nil {
			passphrase = stored
		}
	}
	if passphrase == "" {
		if !interactive {
			return nil
		}
		return func() tea.Msg {
			return vaultPullMsg{err: fmt.Errorf("vault passphrase required"), interactive: true, thenPush: thenPush}
		}
	}
	passSnapshot := passphrase
	opGen := m.vaultOpGen
	return func() tea.Msg {
		session, err := vault.LoadSession(m.vaultSessionPath())
		if err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		ifNoneMatch := session.Revision
		if interactive {
			ifNoneMatch = ""
		}
		remoteGet, err := client.GetVault(ctx, ifNoneMatch)
		if err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		if remoteGet.NotModified {
			return vaultPullMsg{changed: false, revision: session.Revision, notice: "Vault already up to date", interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		if len(remoteGet.Ciphertext) == 0 {
			notice := ""
			if interactive {
				notice = "No remote vault yet"
			}
			return vaultPullMsg{changed: false, revision: session.Revision, notice: notice, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		remoteDoc, err := vault.Decrypt(remoteGet.Ciphertext, passSnapshot)
		if err != nil {
			return vaultPullMsg{err: err, badPassphrase: true, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		if remoteGet.Meta.Revision != "" && remoteGet.Meta.Revision == session.Revision {
			return vaultPullMsg{changed: false, revision: session.Revision, notice: "Vault already up to date", interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata, Previous: remoteDoc}
		localDoc, err := packer.Pack()
		if err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		result := vault.Merge(localDoc, remoteDoc, vault.MergeModeMerge)
		if len(result.Conflicts) > 0 {
			return vaultConflictMsg{count: len(result.Conflicts), interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		applier := vault.Applier{Paths: m.paths, Config: m.config, Store: m.metadata}
		if err := applier.Apply(result.Document); err != nil {
			return vaultPullMsg{err: err, interactive: interactive, thenPush: thenPush, opGen: opGen}
		}
		session.Revision = remoteGet.Meta.Revision
		_ = vault.SaveSession(m.vaultSessionPath(), session)
		sessionCopy := session
		return vaultPullMsg{changed: true, revision: session.Revision, notice: "Vault pulled", interactive: interactive, thenPush: thenPush, session: &sessionCopy, opGen: opGen}
	}
}

func (m *App) vaultPushCmd() tea.Cmd {
	return m.vaultPushCmdOpts(false)
}

func (m *App) vaultPushCmdOpts(synced bool) tea.Cmd {
	if !m.vaultLinked() {
		return nil
	}
	passphrase := m.vaultPassphrase
	if passphrase == "" {
		if stored, err := vault.LoadPassphrase(m.vaultPassphrasePath()); err == nil {
			passphrase = stored
		}
	}
	if passphrase == "" {
		return func() tea.Msg {
			return vaultPushMsg{err: fmt.Errorf("vault passphrase required"), synced: synced, opGen: m.vaultOpGen}
		}
	}
	passSnapshot := passphrase
	opGen := m.vaultOpGen
	return func() tea.Msg {
		session, err := vault.LoadSession(m.vaultSessionPath())
		if err != nil {
			return vaultPushMsg{err: err, synced: synced, opGen: opGen}
		}
		client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		remoteGet, err := client.GetVault(ctx, "")
		if err != nil {
			return vaultPushMsg{err: err, synced: synced, opGen: opGen}
		}
		if len(remoteGet.Ciphertext) == 0 && session.Revision != "" {
			return vaultPushMsg{err: fmt.Errorf("remote vault missing; use Reset passphrase to replace it"), synced: synced, opGen: opGen}
		}
		if err := vault.VerifyPassphrase(remoteGet.Ciphertext, passSnapshot); err != nil {
			return vaultPushMsg{err: err, badPassphrase: true, synced: synced, opGen: opGen}
		}
		prev := vault.Document{}
		var remoteDoc vault.Document
		haveRemote := len(remoteGet.Ciphertext) > 0
		if haveRemote {
			doc, decErr := vault.Decrypt(remoteGet.Ciphertext, passSnapshot)
			if decErr != nil {
				return vaultPushMsg{err: decErr, badPassphrase: true, synced: synced, opGen: opGen}
			}
			remoteDoc = doc
			prev = doc
		}
		packer := vault.Packer{Paths: m.paths, Config: m.config, Keyring: m.keyring, Store: m.metadata, Previous: prev}
		localDoc, err := packer.Pack()
		if err != nil {
			return vaultPushMsg{err: err, synced: synced, opGen: opGen}
		}
		doc := localDoc
		applied := false
		if haveRemote {
			result := vault.Merge(localDoc, remoteDoc, vault.MergeModeMerge)
			if len(result.Conflicts) > 0 {
				return vaultConflictMsg{
					count:       len(result.Conflicts),
					interactive: true,
					thenPush:    true,
					opGen:       opGen,
				}
			}
			doc = result.Document
			applier := vault.Applier{Paths: m.paths, Config: m.config, Store: m.metadata}
			if err := applier.Apply(doc); err != nil {
				return vaultPushMsg{err: err, synced: synced, opGen: opGen}
			}
			applied = true
		}
		blob, err := vault.Encrypt(doc, passSnapshot)
		if err != nil {
			return vaultPushMsg{err: err, synced: synced, opGen: opGen}
		}
		ifMatch := remoteGet.Meta.Revision
		if ifMatch == "" {
			ifMatch = session.Revision
		}
		if haveRemote && ifMatch == "" {
			return vaultPushMsg{
				err:    fmt.Errorf("remote vault revision missing"),
				synced: synced,
				opGen:  opGen,
			}
		}
		meta, err := client.PutVault(ctx, blob, ifMatch)
		if err != nil {
			return vaultPushMsg{err: err, synced: synced, opGen: opGen}
		}
		session.Revision = meta.Revision
		_ = vault.SaveSession(m.vaultSessionPath(), session)
		sessionCopy := session
		return vaultPushMsg{revision: meta.Revision, session: &sessionCopy, synced: synced, applied: applied, opGen: opGen}
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
	case "vault_sync":
		if m.vaultPassphrase == "" {
			if stored, err := vault.LoadPassphrase(m.vaultPassphrasePath()); err == nil && stored != "" {
				m.vaultPassphrase = stored
			}
		}
		if m.vaultPassphrase == "" {
			m.openVaultUnlockForm("vault_sync")
			return m, nil
		}
		m.vaultStatus = "syncing…"
		m.beginVaultBusy("Syncing vault…")
		return m, m.vaultPullCmdOpts(true, true)
	case "vault_logout":
		session, err := vault.LoadSession(m.vaultSessionPath())
		apiBase := ""
		if err == nil {
			apiBase = vault.NormalizeAPIBase(session.APIBase)
			if session.Token != "" {
				client := &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				_ = client.Logout(ctx)
				cancel()
			}
		}
		_ = vault.ClearSession(m.vaultSessionPath())
		if apiBase != "" && apiBase != vault.DefaultAPIBase {
			_ = vault.SaveSession(m.vaultSessionPath(), vault.Session{APIBase: apiBase})
			m.vaultAPIBase = apiBase
		} else {
			m.vaultAPIBase = ""
		}
		m.forgetVaultPassphrase()
		m.vaultSession = nil
		m.vaultStatus = ""
		m.vaultLastSync = ""
		telemetry.Track("vault_logout", m.version)
		return m, m.setNotice("Vault logged out")
	case "vault_api_base":
		m.openVaultAPIBaseForm()
	}
	return m, nil
}
