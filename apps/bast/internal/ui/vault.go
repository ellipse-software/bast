package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// vaultBusyBlocksBody replaces Vault or Sync content while a network step or
// long Sync op (e.g. box create from the Sync tab) runs. Other sections keep
// their normal body and only show the footer busy hint.
func (m *App) vaultBusyBlocksBody() bool {
	if m.vaultBusy != "" && m.section == vaultSection {
		return true
	}
	return m.syncBusy != "" && m.section == syncSection
}

func (m *App) renderVaultBusy(s styleSet) string {
	title := "Vault"
	if m.section == syncSection && m.syncProvider != "" {
		title = m.syncProviderTitle()
	} else if m.section == syncSection {
		title = "Sync"
	}
	label := m.vaultBusy
	if label == "" {
		label = m.syncBusy
	}
	return "\n  " + s.active.Render(title) + "\n\n  " + s.muted.Render(label)
}

func (m *App) enterVaultSection() tea.Cmd {
	m.clearFilesOverlays()
	m.section, m.syncCursor, m.search = vaultSection, -1, ""
	return nil
}

func (m *App) switchToSection(sec section) tea.Cmd {
	if m.syncBusy != "" {
		return nil
	}
	m.clearFilesOverlays()
	switch sec {
	case hostsSection:
		m.section, m.cursor, m.search = hostsSection, 0, ""
	case keysSection:
		m.section, m.cursor, m.search = keysSection, 0, ""
	case vaultSection:
		return m.enterVaultSection()
	case syncSection:
		return m.enterSyncSection()
	case filesSection:
		return m.enterFilesSection()
	}
	return nil
}

func (m *App) updateVaultKeys(key string) (tea.Model, tea.Cmd) {
	if m.vaultBusy != "" {
		return m, nil
	}
	items := m.vaultMenuItems()
	m.clampVaultCursor(items)
	switch key {
	case "up", "k":
		if m.syncCursor >= 0 {
			m.syncCursor--
		}
	case "down", "j":
		if m.syncCursor+1 < len(items) {
			m.syncCursor++
		}
	case "home", "g":
		m.syncCursor = -1
	case "end", "G":
		if len(items) > 0 {
			m.syncCursor = len(items) - 1
		}
	case "r":
		return m.runVaultAction("vault_sync")
	case "enter":
		if m.syncCursor < 0 || m.syncCursor >= len(items) {
			return m.runVaultPrimary()
		}
		return m.runVaultAction(items[m.syncCursor].action)
	}
	return m, nil
}

func (m *App) clampVaultCursor(items []syncMenuItem) {
	if m.syncCursor < -1 {
		m.syncCursor = -1
	}
	if len(items) == 0 {
		m.syncCursor = -1
		return
	}
	if m.syncCursor >= len(items) {
		m.syncCursor = len(items) - 1
	}
}

func (m *App) vaultChipOriginY() int {
	n := visualLineCount(m.renderVaultStatus(m.styles()))
	return 2 + 1 + n + 1
}

func (m *App) vaultMenuOriginY() int {
	return m.vaultChipOriginY() + 2
}

func (m *App) updateVaultMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.vaultBusy != "" {
		return m, nil
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	x, y, width := m.vaultActionButtonBounds()
	if mouse.Y == y && mouse.X >= x && mouse.X < x+width {
		return m.runVaultPrimary()
	}
	if mouse.Y == y {
		m.syncCursor = -1
		return m, nil
	}
	items := m.vaultMenuItems()
	m.clampVaultCursor(items)
	rowY := m.vaultMenuOriginY()
	for i, item := range items {
		h := 1
		if i == m.syncCursor && item.description != "" {
			h = 2
		}
		if mouse.Y >= rowY && mouse.Y < rowY+h {
			if i == m.syncCursor {
				return m.runVaultAction(item.action)
			}
			m.syncCursor = i
			return m, nil
		}
		rowY += h
	}
	return m, nil
}

func (m *App) runVaultPrimary() (tea.Model, tea.Cmd) {
	if !m.vaultLinked() {
		return m.runVaultAction("vault_login")
	}
	if m.vaultPassphrase == "" {
		return m.runVaultAction("vault_unlock")
	}
	return m.runVaultAction("vault_sync")
}

func (m *App) vaultPrimaryChip() syncMenuItem {
	return syncMenuItem{label: strings.TrimSpace(m.vaultPrimaryAction())}
}

func (m *App) vaultActionButtonBounds() (x, y, width int) {
	bounds := m.actionChipBounds([]syncMenuItem{m.vaultPrimaryChip()})
	y = m.vaultChipOriginY()
	return bounds[0][0], y, bounds[0][1] - bounds[0][0]
}

func (m *App) vaultPrimaryAction() string {
	if !m.vaultLinked() {
		return " Link "
	}
	if m.vaultPassphrase == "" {
		return " Unlock "
	}
	return " Sync "
}

func (m *App) beginSyncBusy(label string) {
	m.syncBusy = label
}

func (m *App) clearSyncBusy() {
	m.syncBusy = ""
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
	m.vaultSessionChecked = true
	m.vaultSession = session
	if session != nil {
		if base := vault.NormalizeAPIBase(session.APIBase); base != "" {
			m.vaultAPIBase = base
		}
	}
}

func (m *App) refreshVaultSessionCache() {
	m.vaultSessionChecked = true
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
	if !m.vaultSessionChecked {
		m.refreshVaultSessionCache()
	}
	return m.vaultSession != nil && m.vaultSession.Token != ""
}

func (m *App) vaultMenuItems() []syncMenuItem {
	apiBaseItem := syncMenuItem{
		label:       "API base URL",
		action:      "vault_api_base",
		description: "Vault server for this machine (self-hosted or bast.sh)",
	}
	if !m.vaultLinked() {
		return []syncMenuItem{apiBaseItem}
	}
	return []syncMenuItem{
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

func (m *App) renderVault(s styleSet) string {
	var b strings.Builder
	title := s.active.Render("Vault")
	b.WriteString("  " + title + "\n")
	b.WriteString(m.renderVaultStatus(s))
	b.WriteString("\n")
	chipSel := -1
	if m.syncCursor < 0 {
		chipSel = 0
	}
	b.WriteString(m.renderActionChips(s, []syncMenuItem{m.vaultPrimaryChip()}, chipSel) + "\n")
	items := m.vaultMenuItems()
	m.clampVaultCursor(items)
	if len(items) > 0 {
		b.WriteString("\n")
		for i, item := range items {
			b.WriteString(m.renderSyncMenuLine(s, i, item) + "\n")
		}
	}
	return b.String()
}

func (m *App) renderVaultStatus(s styleSet) string {
	if !m.vaultLinked() || m.vaultSession == nil {
		return m.renderUnlinkedVault(s)
	}
	var b strings.Builder
	session := m.vaultSession
	b.WriteString("  " + s.value.Render(session.Email) + "\n")
	state := "unlocked"
	stateStyle := s.success
	if m.vaultPassphrase == "" {
		state = "locked"
		stateStyle = s.muted
	}
	meta := state
	if m.vaultLastSync != "" {
		meta += " · " + m.vaultLastSync
	}
	if rev := strings.TrimSpace(session.Revision); rev != "" {
		if len(rev) > 8 {
			rev = rev[:8]
		}
		meta += " · " + rev
	}
	b.WriteString("  " + stateStyle.Render(state) + s.muted.Render(strings.TrimPrefix(meta, state)) + "\n")
	if m.vaultStatus != "" {
		b.WriteString("  " + s.muted.Render(m.vaultStatus) + "\n")
	}
	return b.String()
}

func (m *App) renderUnlinkedVault(s styleSet) string {
	inner := min(50, max(8, m.terminalWidth()-2))
	content := max(4, inner-2)
	border := s.muted
	top := border.Render("╭" + strings.Repeat("─", inner) + "╮")
	bot := border.Render("╰" + strings.Repeat("─", inner) + "╯")
	side := border.Render("│")
	row := func(line string) string {
		if lipgloss.Width(line) > content {
			line = truncate(line, content)
		}
		return side + " " + padVisual(line, content) + " " + side
	}

	var b strings.Builder
	b.WriteString(top + "\n")
	b.WriteString(row(s.active.Render("◇  No vault yet")) + "\n")
	b.WriteString(row("") + "\n")
	for _, line := range []string{
		"Sync hosts and keys between your computers.",
		"The passphrase never leaves this machine.",
	} {
		for _, wrapped := range wrapWords(line, content) {
			b.WriteString(row(s.muted.Render(wrapped)) + "\n")
		}
	}
	b.WriteString(row("") + "\n")
	b.WriteString(row(unlinkedVaultMeta(s, m.preferredVaultAPIBase(), content)) + "\n")
	if m.vaultStatus != "" {
		for _, wrapped := range wrapWords(m.vaultStatus, content) {
			b.WriteString(row(s.error.Render(wrapped)) + "\n")
		}
	}
	b.WriteString(bot)
	placed := lipgloss.PlaceHorizontal(m.terminalWidth(), lipgloss.Center, b.String())
	return "\n" + placed + "\n"
}

func unlinkedVaultMeta(s styleSet, apiBase string, width int) string {
	label := s.value.Render("not linked")
	if strings.TrimSpace(apiBase) == "" {
		return label
	}
	sep := s.muted.Render(" · ")
	restWidth := width - lipgloss.Width("not linked · ")
	if restWidth < 8 {
		return label
	}
	return label + sep + s.muted.Render(truncate(apiBase, restWidth))
}

func wrapWords(s string, width int) []string {
	if width < 1 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var cur string
	flush := func() {
		if cur == "" {
			return
		}
		lines = append(lines, cur)
		cur = ""
	}
	for _, word := range words {
		if lipgloss.Width(word) > width {
			flush()
			lines = append(lines, truncate(word, width))
			continue
		}
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if lipgloss.Width(next) <= width {
			cur = next
			continue
		}
		flush()
		cur = word
	}
	flush()
	return lines
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
	if vault.HostedTermsRequired(apiBase) {
		m.form = nil
		m.openVaultTermsForm(email, apiBase)
		return nil
	}
	return m.startVaultOTP(email, apiBase, false)
}

func (m *App) openVaultTermsForm(email, apiBase string) {
	m.openForm("Vault: terms", "vault_terms", []field{
		{label: "Email", value: email, hidden: true},
		{label: "APIBase", value: apiBase, hidden: true},
		{
			label:       "Agreement",
			description: vault.TermsURL + "  ·  " + vault.PrivacyURL,
			options: []fieldOption{
				{label: "Accept", value: "accept"},
				{label: "Cancel", value: "cancel"},
			},
		},
	})
}

func (m *App) submitVaultTerms() tea.Cmd {
	if m.form == nil {
		return nil
	}
	m.commitFormField()
	value := strings.TrimSpace(m.formValue("Agreement"))
	email := m.formValue("Email")
	apiBase := m.formValue("APIBase")
	m.form = nil
	if value != "accept" {
		return m.setNotice("Link cancelled")
	}
	m.beginVaultBusy("Sending code…")
	return m.startVaultOTP(email, apiBase, true)
}

func (m *App) startVaultOTP(email, apiBase string, acceptTerms bool) tea.Cmd {
	client := &vault.Client{BaseURL: apiBase}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		err := client.StartOTP(ctx, email, acceptTerms)
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
		m.setVaultSession(nil)
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
		m.setVaultSession(nil)
		m.vaultStatus = ""
		m.vaultLastSync = ""
		telemetry.Track("vault_logout", m.version)
		return m, m.setNotice("Vault logged out")
	case "vault_api_base":
		m.openVaultAPIBaseForm()
	}
	return m, nil
}
