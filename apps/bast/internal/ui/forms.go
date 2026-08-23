package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud/sync"
	"bast/internal/hostpass"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

const (
	descHostLabel          = "Display name; use / for groups, up to 5 levels"
	descHostHostname       = "Required - Server hostname or IP address"
	descHostUser           = "Optional - Remote username; blank uses SSH default"
	descHostPort           = "Optional - Port number; blank defaults to 22"
	descHostIdentity       = "Optional - Key, saved password, or OpenSSH defaults"
	descHostPassword       = "Blank prompts on connect"
	descHostProxyJump      = "Optional - Route through a jump host (ProxyJump)"
	descHostRemoteCommand  = "Optional - Command to run after connecting (RemoteCommand)"
	descHostRequestTTY     = "Optional - Allocate a TTY for the startup command"
	descHostForwardAgent   = "Optional - Forward your local SSH agent to the server"
	descHostLocalForward   = "Optional - Local port forwards; use port target pairs separated by ;"
	descHostRemoteForward  = "Optional - Remote port forwards; use port target pairs separated by ;"
	descHostDynamicForward = "Optional - SOCKS proxy port (DynamicForward)"
	descHostKeepalive      = "Optional - ServerAliveInterval in seconds"
	descHostCompression    = "Optional - Enable SSH compression"
	descHostSetEnv         = "Optional - SetEnv pairs like FOO=bar, separated by ;"
	descHostSSHFlags       = "Optional - Any other OpenSSH options, separated by ;"
	descHostTags           = "Optional - Comma-separated tags included in search"
	descHostEnvironment    = "Optional - Environment name like production or staging"
	descHostColor          = "Optional - Hex colour for the host label"
	descHostNotes          = "Optional - Short note in details and search"
	descKeyName            = "Required - Short name for this keypair"
	descKeyAlgorithm       = "Required - Key algorithm: ed25519 or rsa"
	descKeyPrivate         = "Required - Private key path or pasted PEM"
	descKeyPublic          = "Optional - Public key; blank derives from private"
	descKeyComment         = "Optional - Public key comment; blank keeps existing"
	descKeyCommentEdit     = "Optional - Public key comment; blank removes it"
	descKeyExportDir       = "Required - Directory to write exported key files"
	descKeyExportConfirm   = "Required - Type EXPORT to confirm export"
	descKeyServer          = "Required - Target server; may prompt for password"
)

const (
	passwordOnlyIdentity = "\x00password-only"
	passwordKeepValue    = "\x00password-keep"
	passwordClearValue   = "\x00password-clear"
	methodFieldLabel     = "Method"
	passwordFieldLabel   = "Password"
)

func (m *App) formTextInputActive() bool {
	f := m.form
	if f == nil {
		return false
	}
	if isGroupAssignmentForm(f) {
		return true
	}
	if isHostForm(f) {
		if f.selecting {
			return false
		}
		if f.screen != "" && f.screen != "hub" {
			if f.screen == formScreenAdvancedHub {
				return false
			}
			item := &f.fields[f.index]
			return len(item.options) == 0 || item.options[item.selected].custom
		}
		items := hostHubItems(f)
		if f.hubIndex >= 0 && f.hubIndex < len(items) {
			return hostHubInline(items[f.hubIndex].id)
		}
		return false
	}
	item := &f.fields[f.index]
	if f.selecting {
		return false
	}
	return len(item.options) == 0 || item.options[item.selected].custom
}

func (m *App) updateFormQuit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit, true
	}
	if key == "q" && !m.formTextInputActive() {
		return m, tea.Quit, true
	}
	return m, nil, false
}

func (m *App) updateFormMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || m.form == nil {
		return m, nil
	}
	if isHostForm(m.form) || isGroupAssignmentForm(m.form) {
		return m, nil
	}
	f := m.form
	if f.index < 0 || f.index >= len(f.fields) {
		return m, nil
	}
	item := &f.fields[f.index]
	if !f.selecting || len(item.options) == 0 {
		return m, nil
	}
	y := m.formOptionListOriginY()
	if y < 0 {
		return m, nil
	}
	rows := min(7, len(item.options))
	start := scrollStart(item.selected, len(item.options), rows)
	end := min(len(item.options), start+rows)
	for optionIndex := start; optionIndex < end; optionIndex++ {
		if mouse.Y == y {
			if optionIndex == item.selected {
				return m.updateForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			}
			item.selected = optionIndex
			m.focusFormField()
			return m, nil
		}
		y++
	}
	return m, nil
}

func (m *App) formOptionListOriginY() int {
	f := m.form
	if f == nil {
		return -1
	}
	y := 2
	y++ // leading blank in renderForm
	y++ // title
	y++ // blank after title
	if destructiveConfirmationTarget(f) != "" {
		y += 2
	}
	for i, item := range f.fields {
		if item.hidden || i > f.revealed {
			continue
		}
		if i != f.index {
			y++
			continue
		}
		y++ // label
		if item.description != "" {
			y++
		}
		if len(item.options) > 0 && f.selecting {
			return y
		}
		return -1
	}
	return -1
}

func (m *App) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model, cmd, quit := m.updateFormQuit(msg); quit {
		return model, cmd
	}
	if isGroupAssignmentForm(m.form) {
		return m.updateGroupAssignmentForm(msg)
	}
	if isHostForm(m.form) {
		return m.updateHostForm(msg)
	}
	key := msg.String()
	f := m.form
	if key == "ctrl+y" {
		if target := destructiveConfirmationTarget(f); target != "" {
			return m, tea.Batch(tea.SetClipboard(target), m.setNotice("Confirmation name copied"))
		}
	}
	item := &f.fields[f.index]
	if f.selecting {
		switch key {
		case "esc", "backspace", "ctrl+h":
			f.selecting = false
			m.focusFormField()
		case "up", "down", "j", "k":
			direction := 1
			if key == "up" || key == "k" {
				direction = -1
			}
			item.selected = (item.selected + direction + len(item.options)) % len(item.options)
			m.focusFormField()
		case "shift+tab":
			f.selecting = false
			m.commitFormField()
			m.moveForm(-1, false)
		case "enter", "tab":
			f.selecting = false
			m.focusFormField()
			if item.options[item.selected].custom {
				return m, nil
			}
			m.commitFormField()
			if key == "enter" && isEditForm(f) {
				m.focusFormField()
				return m, nil
			}
			if m.moveForm(1, true) {
				return m, nil
			}
			return m.submitForm()
		}
		return m, nil
	}
	if key == "esc" && len(item.options) > 0 && item.options[item.selected].custom {
		m.commitFormField()
		f.selecting = true
		m.focusFormField()
		return m, nil
	}
	if key == "esc" {
		if m.form != nil && m.form.action == "files_delete" {
			m.files.deletePaths = nil
		}
		if m.form != nil && m.form.action == "vault_resolve" {
			m.vaultConflict = nil
		}
		m.form = nil
		return m, nil
	}
	if (key == "backspace" || key == "ctrl+h") && !m.formTextInputActive() {
		if m.form != nil && m.form.action == "files_delete" {
			m.files.deletePaths = nil
		}
		if m.form != nil && m.form.action == "vault_resolve" {
			m.vaultConflict = nil
		}
		m.form = nil
		return m, nil
	}
	if key == "up" || key == "shift+tab" {
		m.commitFormField()
		m.moveForm(-1, false)
		return m, nil
	}
	if key == "down" {
		m.commitFormField()
		m.moveForm(1, false)
		return m, nil
	}
	if key == "enter" && isEditForm(f) {
		m.commitFormField()
		return m.submitForm()
	}
	if key == "space" && len(item.options) > 0 && !item.options[item.selected].custom {
		f.selecting = true
		m.focusFormField()
		return m, nil
	}
	if key == "enter" || key == "tab" {
		if len(item.options) > 0 && !item.options[item.selected].custom {
			f.selecting = true
			m.focusFormField()
			return m, nil
		}
		m.commitFormField()
		if m.moveForm(1, true) {
			return m, nil
		}
		return m.submitForm()
	}
	if m.form.action == "key_import" && m.form.fields[m.form.index].label == "Private key" && m.form.pastedPrivateKey != "" {
		m.form.pastedPrivateKey = ""
		m.form.input.SetValue("")
	}
	if m.form.action == "key_import" && m.form.fields[m.form.index].label == "Public key" && m.form.pastedPublicKey != "" {
		m.form.pastedPublicKey = ""
		m.form.input.SetValue("")
	}
	if len(item.options) > 0 && !item.options[item.selected].custom {
		return m, nil
	}
	f.validationError = ""
	var cmd tea.Cmd
	m.form.input, cmd = m.form.input.Update(msg)
	return m, cmd
}

func isEditForm(f *form) bool {
	return f.action == "group_edit" || f.action == "key_comment"
}

func (m *App) updateFormPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	f := m.form
	if isGroupAssignmentForm(f) {
		return m.updateGroupAssignmentInput(msg)
	}
	f.validationError = ""
	content := strings.TrimSpace(msg.Content)
	if f.action == "key_import" && f.fields[f.index].label == "Private key" && strings.Contains(content, "PRIVATE KEY-----") {
		f.pastedPrivateKey = content + "\n"
		f.fields[f.index].value = "Pasted private key"
		f.input.SetValue("Pasted private key")
		m.moveForm(1, true)
		return m, nil
	}
	if f.action == "key_import" && f.fields[f.index].label == "Public key" && (strings.HasPrefix(content, "ssh-") || strings.HasPrefix(content, "ecdsa-") || strings.HasPrefix(content, "sk-")) {
		f.pastedPublicKey = content + "\n"
		f.fields[f.index].value = "Pasted public key"
		f.input.SetValue("Pasted public key")
		parts := strings.Fields(content)
		if len(parts) > 2 && f.index+1 < len(f.fields) && f.fields[f.index+1].label == "Comment" {
			f.fields[f.index+1].value = strings.Join(parts[2:], " ")
		}
		m.moveForm(1, true)
		return m, nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return m, cmd
}

func (m *App) submitForm() (tea.Model, tea.Cmd) {
	f := m.form
	values := map[string]string{}
	for _, item := range f.fields {
		values[item.label] = item.value
	}
	switch f.action {
	case "host_add", "host_edit", "history_host_add":
		group, label, pathErr := metadata.SplitLabelPath(values["Label"])
		if pathErr != nil {
			return m.formError(pathErr.Error())
		}
		if strings.TrimSpace(label) == "" {
			return m.formError("label is required")
		}
		groupCreated := group != "" && !m.groupExists(group)
		adv := advancedSettingsFromForm(values)
		if err := sshconfig.ValidateAdvanced(adv); err != nil {
			return m.formError(err.Error())
		}
		identityFile := values[methodFieldLabel]
		passwordOnly := identityFile == passwordOnlyIdentity
		if passwordOnly {
			identityFile = ""
		}
		input := sshconfig.HostInput{
			Alias: sshconfig.NormalizeAlias(label), HostName: values["Hostname"], User: values["User"], Port: values["Port"],
			IdentityFile: identityFile, ExtraOptions: adv.ExtraOptions(), PasswordOnly: passwordOnly, ProxyJump: adv.ProxyJump,
		}
		if identityFile != "" && !passwordOnly && !sshconfig.HasDirective(input.ExtraOptions, "IdentitiesOnly") {
			input.ExtraOptions = append([]string{"IdentitiesOnly yes"}, input.ExtraOptions...)
		}
		oldAlias := values["Original label"]
		meta := metadata.Host{Label: label, Group: group, Tags: splitCSV(values["Tags"]), Environment: values["Environment"], Color: values["Color"], Notes: values["Notes"]}
		if meta.Label == input.Alias {
			meta.Label = ""
		}
		if oldAlias != "" {
			old := m.metadata.Host(oldAlias)
			meta.Favorite, meta.Hidden, meta.LastUsedAt, meta.ConnectionCount = old.Favorite, old.Hidden, old.LastUsedAt, old.ConnectionCount
		}
		var err error
		var managedID string
		if f.action == "history_host_add" {
			var added sshconfig.Host
			added, err = m.addHistoryHost(input, meta, values["History suggestion"])
			managedID = added.ManagedID
		} else if f.action == "host_add" {
			var added sshconfig.Host
			added, err = m.config.Add(input)
			managedID = added.ManagedID
		} else {
			host, ok := m.findHost(oldAlias)
			if !ok {
				err = fmt.Errorf("host %q no longer exists", oldAlias)
			} else {
				managedID = host.ManagedID
				err = m.config.Update(host.ManagedID, input)
			}
		}
		if err == nil && f.action != "history_host_add" {
			if oldAlias != "" && oldAlias != input.Alias {
				err = m.metadata.RenameHost(oldAlias, input.Alias)
			}
			if err != nil {
				return m.finishMutation(err, "Host saved")
			}
			err = m.metadata.SetHost(input.Alias, meta)
		}
		if err == nil {
			err = m.applyHostPassword(managedID, passwordOnly, values[passwordFieldLabel])
		}
		if err == nil {
			if f.action == "history_host_add" {
				m.removeHistorySuggestion(values["History suggestion"])
			}
			m.selectAfterLoadSection = hostsSection
			if groupCreated {
				m.selectAfterLoadName, m.selectAfterLoadGroup = group, true
			} else if f.action == "host_add" || f.action == "history_host_add" {
				m.selectAfterLoadName, m.selectAfterLoadGroup = input.Alias, false
			}
		}
		return m.finishMutation(err, "Host saved")
	case "group_edit":
		oldPath := values["Original path"]
		newPath, err := m.renameGroup(oldPath, values["Name"])
		if err == nil {
			m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = hostsSection, newPath, true
		}
		return m.finishMutation(err, "Group renamed")
	case "metadata_edit":
		alias := values["Alias"]
		old := m.metadata.Host(alias)
		group, label, err := metadata.SplitLabelPath(values["Label"])
		if err != nil {
			return m.formError(err.Error())
		}
		groupCreated := group != "" && !m.groupExists(group)
		old.Label, old.Group, old.Tags, old.Environment, old.Color, old.Notes = label, group, splitCSV(values["Tags"]), values["Environment"], values["Color"], values["Notes"]
		if old.Label == alias {
			old.Label = ""
		}
		err = m.metadata.SetHost(alias, old)
		if err == nil && groupCreated {
			m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = hostsSection, group, true
		}
		return m.finishMutation(err, "Host metadata saved")
	case "group_assign":
		alias := values["Alias"]
		group, err := metadata.NormalizeGroupPath(values["Group"])
		if err != nil {
			return m.formValidationError(err.Error())
		}
		if sync.IsSyncedGroup(group) {
			return m.formValidationError("cloud sync groups are read-only")
		}
		collapsedGroups, collapsedPaths, collapseChanged := m.collapsedGroupsRevealing(group)
		err = m.metadata.MoveHost(alias, group, collapsedPaths)
		if err == nil {
			m.collapsedGroups = collapsedGroups
			if collapseChanged {
				m.collapseRevision++
			}
			m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = hostsSection, alias, false
		}
		return m.finishMutation(err, "Host group saved")
	case "host_delete":
		alias := values["Alias"]
		if values["Type the name to confirm"] != values["Confirmation"] {
			return m.formValidationError("Name does not match the host label")
		}
		host, ok := m.findHost(alias)
		if !ok {
			return m.formError("host no longer exists")
		}
		err := m.config.Delete(host.ManagedID)
		if err == nil {
			err = hostpass.Delete(m.paths.PasswordsDir, host.ManagedID)
		}
		if err == nil {
			err = m.metadata.DeleteHost(alias)
		}
		return m.finishMutation(err, "Host deleted")
	case "key_generate":
		cmd, _, err := m.keyring.GenerateCommand(values["Name"], strings.ToLower(values["Algorithm"]))
		if err != nil {
			return m.formError(err.Error())
		}
		m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = keysSection, values["Name"], false
		m.form = nil
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return processDoneMsg{name: "Key generation", err: err} })
	case "key_import":
		privateSource := values["Private key"]
		if f.pastedPrivateKey != "" {
			privateSource = f.pastedPrivateKey
		}
		publicSource := values["Public key"]
		if f.pastedPublicKey != "" {
			publicSource = f.pastedPublicKey
		}
		err := m.keyring.Import(privateSource, publicSource, values["Name"], values["Comment"])
		if err == nil {
			m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = keysSection, values["Name"], false
		}
		return m.finishMutation(err, "Key imported")
	case "key_comment":
		key, ok := m.findKey(values["Key"])
		if !ok {
			return m.formError("key no longer exists")
		}
		return m.finishMutation(m.keyring.SetComment(key, values["Comment"]), "Key comment saved")
	case "key_export":
		if values["Type EXPORT"] != "EXPORT" {
			return m.formValidationError("type EXPORT to acknowledge private-key export")
		}
		key, ok := m.findKey(values["Key"])
		if !ok {
			return m.formError("key no longer exists")
		}
		return m.finishMutation(m.keyring.Export(key, values["Directory"]), "Key exported")
	case "key_install":
		key, ok := m.findKey(values["Key"])
		if !ok {
			return m.formError("key no longer exists")
		}
		host, ok := m.findHost(values["Server"])
		if !ok {
			return m.formError("server no longer exists")
		}
		public, err := m.keyring.PublicText(key)
		if err != nil {
			return m.formError(err.Error())
		}
		cmd, err := m.openSSH.InstallPublicKeyCommand(values["Server"], public)
		if err != nil {
			return m.formError(err.Error())
		}
		m.prepareSSH(cmd, host)
		hostLabel := m.hostLabel(host)
		m.form = nil
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return processDoneMsg{name: "Install public key on " + hostLabel, err: err}
		})
	case "key_delete":
		key, ok := m.findKey(values["Key"])
		if !ok {
			return m.formError("key no longer exists")
		}
		if values["Type the name to confirm"] != key.Name {
			return m.formValidationError("Name does not match the key name")
		}
		return m.finishMutation(m.keyring.Delete(key, values["Type the name to confirm"]), "Key permanently deleted")
	case "sync_gcp_user", "sync_gcp_projects", "sync_gcp_sa_add", "sync_gcp_sa_remove", "sync_aws_user", "sync_aws_profiles", "sync_aws_regions", "sync_azure_user", "sync_azure_subscriptions", "sync_azure_resource_groups", "box_new", "box_stop", "box_fork", "upstash_new", "upstash_stop", "upstash_fork", "upstash_delete", "upstash_key":
		return m, m.submitSyncForm(f.action, values)
	case "known_delete":
		alias := values["Alias"]
		if values["Type the name to confirm"] != values["Confirmation"] {
			return m.formValidationError("Name does not match the host label")
		}
		host, ok := m.findHost(alias)
		if !ok {
			return m.formError("host no longer exists")
		}
		err := m.openSSH.RemoveKnownHost(context.Background(), host.Resolved.HostName, host.Resolved.Port)
		return m.finishMutation(err, "Known-host entry removed")
	case "vault_login":
		cmd := m.submitVaultLogin()
		if m.form != nil && m.form.validationError != "" {
			return m, nil
		}
		if m.form != nil && m.form.action == "vault_terms" {
			return m, nil
		}
		m.form = nil
		m.beginVaultBusy("Sending code…")
		return m, cmd
	case "vault_terms":
		return m, m.submitVaultTerms()
	case "vault_code":
		cmd := m.submitVaultCode()
		if m.form != nil && m.form.validationError != "" {
			return m, nil
		}
		m.form = nil
		m.beginVaultBusy("Verifying code…")
		return m, cmd
	case "vault_passphrase":
		cmd := m.submitVaultPassphrase()
		if m.form != nil && m.form.validationError != "" {
			return m, nil
		}
		m.form = nil
		m.beginVaultBusy("Linking vault…")
		return m, cmd
	case "vault_unlock":
		cmd := m.submitVaultUnlock()
		if m.form != nil && m.form.validationError != "" {
			return m, nil
		}
		m.form = nil
		m.beginVaultBusy("Unlocking vault…")
		return m, cmd
	case "vault_reset_passphrase":
		cmd := m.submitVaultResetPassphrase()
		if m.form != nil && m.form.validationError != "" {
			return m, nil
		}
		m.form = nil
		m.beginVaultBusy("Resetting vault passphrase…")
		return m, cmd
	case "vault_rotate_passphrase":
		cmd := m.submitVaultRotatePassphrase()
		if m.form != nil && m.form.validationError != "" {
			return m, nil
		}
		m.form = nil
		m.beginVaultBusy("Rotating vault passphrase…")
		return m, cmd
	case "vault_api_base":
		cmd := m.submitVaultAPIBase()
		if m.form != nil && m.form.validationError != "" {
			return m, nil
		}
		m.form = nil
		return m, cmd
	case "vault_resolve":
		return m, m.submitVaultResolve()
	case "files_mkdir", "files_rename", "files_delete":
		return m.submitFilesForm(values)
	}
	return m.formError("unknown form action")
}

func (m *App) finishMutation(err error, success string) (tea.Model, tea.Cmd) {
	if err != nil {
		return m.formError(err.Error())
	}
	m.form = nil
	m.loading = true
	m.enriching = false
	cmds := []tea.Cmd{m.loadCmd(), m.setNotice(success)}
	if push := m.scheduleVaultPush(); push != nil {
		cmds = append(cmds, push)
	}
	return m, tea.Batch(cmds...)
}

func (m *App) formError(message string) (tea.Model, tea.Cmd) {
	if m.form != nil {
		message = m.form.title + " failed: " + message
	}
	m.statusID++
	m.status, m.statusError = message, true
	return m, nil
}

func (m *App) formValidationError(message string) (tea.Model, tea.Cmd) {
	if m.form != nil {
		m.form.validationError = message
		m.form.input.Focus()
	}
	return m, nil
}

func destructiveConfirmationTarget(f *form) string {
	if f == nil {
		return ""
	}
	label := "Confirmation"
	switch f.action {
	case "key_delete":
		label = "Key"
	case "host_delete", "known_delete", "vault_reset_passphrase", "files_delete":
		// keep Confirmation
	default:
		return ""
	}
	for _, item := range f.fields {
		if item.label == label {
			return item.value
		}
	}
	return ""
}

func (m *App) openAddHostForm() {
	group := m.defaultAddGroup()
	m.openHostForm("Add host", "host_add", hostFormFields(m, metadataHostValues{
		label: metadata.JoinLabelPath(group, ""),
	}, hostConnectionValues{includeConnection: true}, nil))
}

func (m *App) openEditGroupForm() {
	group, ok := m.selectedGroupHeader()
	if !ok {
		return
	}
	if sync.IsSyncedGroup(group) {
		m.status, m.statusError = "Cloud sync groups cannot be renamed", true
		return
	}
	m.openForm("Rename group: "+groupShortName(group), "group_edit", []field{
		{label: "Original path", value: group, hidden: true},
		{label: "Name", description: "Renames this group for every host inside it", value: groupShortName(group)},
	})
}

func (m *App) openEditHostForm() {
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	if host.Synced {
		m.status, m.statusError = "Synced hosts are read-only; manage them in the Sync tab", true
		return
	}
	meta := m.metadata.Host(host.Alias)
	labelPath := metadata.JoinLabelPath(meta.Group, m.hostLabel(host))
	if !host.Managed {
		m.openHostForm("Edit metadata: "+m.hostLabel(host), "metadata_edit", hostFormFields(m, metadataHostValues{
			label: labelPath, tags: strings.Join(meta.Tags, ", "),
			environment: meta.Environment, color: meta.Color, notes: meta.Notes,
			labelDesc: "Optional - Display name; use / for groups; SSH alias stays " + host.Alias,
		}, hostConnectionValues{}, []field{{label: "Alias", value: host.Alias, hidden: true}}))
		return
	}
	identity := ""
	isPasswordOnly := passwordOnly(host.Resolved)
	if !isPasswordOnly && len(host.Resolved.IdentityFiles) > 0 {
		identity = host.Resolved.IdentityFiles[0]
	}
	extras, _ := m.config.ManagedExtras(host.ManagedID)
	adv := sshconfig.ParseAdvanced(extras, emptyIfNone(host.Resolved.ProxyJump))
	passwordStored := hostpass.Exists(m.paths.PasswordsDir, host.ManagedID)
	m.openHostForm("Edit host: "+m.hostLabel(host), "host_edit", hostFormFields(m, metadataHostValues{
		label: labelPath, tags: strings.Join(meta.Tags, ", "),
		environment: meta.Environment, color: meta.Color, notes: meta.Notes,
	}, hostConnectionValues{
		includeConnection: true,
		hostname:          host.Resolved.HostName,
		user:              host.Resolved.User,
		port:              host.Resolved.Port,
		identity:          identity,
		passwordOnly:      isPasswordOnly,
		passwordStored:    passwordStored,
		advanced:          adv,
	}, []field{{label: "Original label", value: host.Alias, hidden: true}}))
}

func (m *App) openGroupAssignmentForm() {
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	if host.Synced {
		m.status, m.statusError = "Synced hosts are read-only; manage them in the Sync tab", true
		return
	}
	meta := m.metadata.Host(host.Alias)
	m.openForm("Move host: "+m.hostLabel(host), "group_assign", []field{
		{label: "Alias", value: host.Alias, hidden: true},
		{label: "Current group", value: meta.Group, hidden: true},
		m.groupAssignmentField(meta.Group),
	})
	m.form.selecting = false
	m.form.input.SetValue("")
	m.form.input.Placeholder = "Search or create a group"
	m.form.input.Focus()
}

func (m *App) groupAssignmentField(current string) field {
	paths := map[string]bool{}
	metadataByHost := m.hostMetadata()
	for _, host := range m.hosts {
		parts := groupPathParts(metadataByHost[host.Alias].Group)
		path := ""
		for _, part := range parts {
			if path == "" {
				path = part
			} else {
				path += "/" + part
			}
			if !sync.IsSyncedGroup(path) {
				paths[path] = true
			}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := strings.ToLower(ordered[i]), strings.ToLower(ordered[j])
		if left == right {
			return ordered[i] < ordered[j]
		}
		return left < right
	})

	item := field{
		label:       "Group",
		description: "Choose an existing group, clear the group, or enter a new path",
		placeholder: "Work/Production",
		options:     []fieldOption{{label: "No group"}},
	}
	for _, path := range ordered {
		item.options = append(item.options, fieldOption{label: path, value: path})
		if path == current {
			item.selected = len(item.options) - 1
		}
	}
	return item
}

func (m *App) openDeleteHostForm() {
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	if host.Synced {
		m.status, m.statusError = "Synced hosts cannot be deleted; disconnect the provider in Sync", true
		return
	}
	if !host.Managed {
		m.status, m.statusError = "External hosts cannot be deleted by Bast", true
		return
	}
	label := m.hostLabel(host)
	m.openForm("Delete host: "+label, "host_delete", []field{{label: "Alias", value: host.Alias, hidden: true}, {label: "Confirmation", value: label, hidden: true}, {label: "Type the name to confirm", placeholder: label}})
}

func (m *App) openGenerateForm() {
	m.openForm("Generate key", "key_generate", []field{
		{label: "Name", description: descKeyName, placeholder: "work"},
		{label: "Algorithm", description: descKeyAlgorithm, value: "ed25519", placeholder: "ed25519 or rsa"},
	})
}
func (m *App) openImportForm() {
	m.openForm("Import native keypair", "key_import", []field{
		{label: "Private key", description: descKeyPrivate, placeholder: "file path or paste key contents"},
		{label: "Public key", description: descKeyPublic, placeholder: "optional path/content; Enter to derive", optional: true},
		{label: "Comment", description: descKeyComment, placeholder: "optional; blank keeps an existing comment", optional: true},
		{label: "Name", description: descKeyName, placeholder: "work"},
	})
}
func (m *App) openEditKeyForm() {
	key, ok := m.selectedKey()
	if !ok {
		return
	}
	if !key.Managed {
		m.status, m.statusError = "External keys cannot be edited by Bast", true
		return
	}
	m.openForm("Edit key comment: "+key.Name, "key_comment", []field{
		{label: "Key", value: key.Name, hidden: true},
		{label: "Comment", description: descKeyCommentEdit, value: key.Comment, placeholder: "blank removes the comment", optional: true},
	})
}
func (m *App) openExportForm() {
	if key, ok := m.selectedKey(); ok {
		m.openForm("Export key: "+key.Name, "key_export", []field{
			{label: "Key", value: key.Name, hidden: true},
			{label: "Directory", description: descKeyExportDir, placeholder: "~/Desktop"},
			{label: "Type EXPORT", description: descKeyExportConfirm},
		})
	}
}
func (m *App) openInstallKeyForm() {
	key, ok := m.selectedKey()
	if !ok {
		return
	}
	if len(m.hosts) == 0 {
		m.status, m.statusError = "Add a server before installing a public key", true
		return
	}
	server := field{
		label:       "Server",
		description: descKeyServer,
	}
	for _, host := range m.hosts {
		label := m.hostLabel(host)
		if target := destination(host); target != "" && target != host.Alias {
			label += " · " + target
		}
		server.options = append(server.options, fieldOption{label: label, value: host.Alias})
	}
	m.openForm("Add "+key.Name+" to server", "key_install", []field{
		{label: "Key", value: key.Name, hidden: true},
		server,
	})
}
func (m *App) openDeleteKeyForm() {
	if key, ok := m.selectedKey(); ok {
		m.openForm("Delete key: "+key.Name, "key_delete", []field{{label: "Key", value: key.Name, hidden: true}, {label: "Type the name to confirm", placeholder: key.Name}})
	}
}
func (m *App) openKnownHostForm() {
	if host, ok := m.selectedHost(); ok {
		label := m.hostLabel(host)
		m.openForm("Remove known host: "+label, "known_delete", []field{{label: "Alias", value: host.Alias, hidden: true}, {label: "Confirmation", value: label, hidden: true}, {label: "Type the name to confirm", placeholder: label}})
	}
}

func (m *App) methodField(current string, passwordOnly bool) field {
	item := field{
		label:       methodFieldLabel,
		description: descHostIdentity,
		placeholder: "~/.ssh/id_ed25519",
		optional:    true,
	}
	item.options = append(item.options, fieldOption{label: "OpenSSH defaults / agent"})
	if current != "" {
		current = shortPath(current, m.paths.Home)
	}
	seen := map[string]bool{}
	for _, key := range m.keys {
		if key.PrivatePath == "" {
			continue
		}
		path := shortPath(key.PrivatePath, m.paths.Home)
		if seen[path] {
			continue
		}
		seen[path] = true
		item.options = append(item.options, fieldOption{label: key.Name + " · " + path, value: path})
		if current == path {
			item.selected = len(item.options) - 1
			item.value = path
		}
	}
	item.options = append(item.options, fieldOption{label: "Manual path…", custom: true})
	if current != "" && item.value == "" {
		item.selected = len(item.options) - 1
		item.customValue = current
		item.value = current
	}
	item.options = append(item.options, fieldOption{label: "Password", value: passwordOnlyIdentity})
	if passwordOnly {
		item.selected = len(item.options) - 1
		item.value = passwordOnlyIdentity
	}
	return item
}

func (m *App) passwordField(stored bool) field {
	item := field{
		label:       passwordFieldLabel,
		section:     formSectionAuth,
		description: descHostPassword,
		optional:    true,
		secret:      true,
		hidden:      true,
	}
	if stored {
		item.description = ""
		item.options = []fieldOption{
			{label: "Keep stored password", value: passwordKeepValue},
			{label: "Prompt on connect", value: passwordClearValue},
			{label: "Replace…", custom: true},
		}
		item.value = passwordKeepValue
	}
	return item
}

func (m *App) applyHostPassword(managedID string, passwordOnly bool, value string) error {
	if strings.TrimSpace(managedID) == "" {
		return nil
	}
	dir := m.paths.PasswordsDir
	if !passwordOnly {
		return hostpass.Delete(dir, managedID)
	}
	switch strings.TrimRight(value, "\r\n") {
	case passwordKeepValue, "":
		return nil
	case passwordClearValue:
		return hostpass.Delete(dir, managedID)
	default:
		return hostpass.Save(dir, managedID, value)
	}
}

func (m *App) syncHostPasswordField() {
	f := m.form
	if f == nil || !isHostForm(f) {
		return
	}
	method := f.fieldByLabel(methodFieldLabel)
	pwd := f.fieldByLabel(passwordFieldLabel)
	if method == nil || pwd == nil {
		return
	}
	show := method.value == passwordOnlyIdentity
	pwd.hidden = !show
	if pwd.hidden && f.index >= 0 && f.index < len(f.fields) && f.fields[f.index].label == passwordFieldLabel {
		if idx := f.fieldIndex(methodFieldLabel); idx >= 0 {
			f.index = idx
			f.selecting = false
			m.focusFormField()
		}
	}
}

func (m *App) hostPasswordStored(h sshconfig.Host) bool {
	return h.Managed && h.ManagedID != "" && hostpass.Exists(m.paths.PasswordsDir, h.ManagedID)
}

func passwordOnly(resolved sshconfig.Resolved) bool {
	pubkey := strings.ToLower(resolved.PubkeyAuthentication)
	password := strings.ToLower(resolved.PasswordAuthentication)
	return (pubkey == "no" || pubkey == "false") && (password == "yes" || password == "true")
}

func (m *App) openForm(title, action string, fields []field) {
	input := textinput.New()
	input.Prompt = ""
	input.SetWidth(max(20, m.width-20))
	m.form = &form{title: title, action: action, fields: fields, input: input}
	for m.form.index < len(fields) && fields[m.form.index].hidden {
		m.form.index++
	}
	m.form.revealed = m.form.index
	m.form.selecting = len(m.form.fields[m.form.index].options) > 0
	m.focusFormField()
}

func (m *App) focusFormField() {
	f := m.form
	item := &f.fields[f.index]
	if item.secret {
		f.input.EchoMode = textinput.EchoPassword
		f.input.EchoCharacter = '*'
	} else {
		f.input.EchoMode = textinput.EchoNormal
		f.input.EchoCharacter = '*'
	}
	if len(item.options) > 0 {
		option := item.options[item.selected]
		if option.custom {
			item.value = item.customValue
			f.input.SetValue(item.customValue)
			f.input.Placeholder = item.placeholder
			f.input.SetCursor(len([]rune(item.customValue)))
			if f.selecting {
				f.input.Blur()
			} else {
				f.input.Focus()
			}
		} else {
			item.value = option.value
			f.input.SetValue("")
			f.input.Blur()
		}
		return
	}
	f.input.SetValue(item.value)
	f.input.Placeholder = item.placeholder
	f.input.SetCursor(len([]rune(item.value)))
	f.input.Focus()
}

func (m *App) commitFormField() {
	if m.form == nil || m.form.index < 0 || m.form.index >= len(m.form.fields) {
		return
	}
	item := &m.form.fields[m.form.index]
	if len(item.options) > 0 {
		option := item.options[item.selected]
		if option.custom {
			if item.secret {
				item.customValue = m.form.input.Value()
			} else {
				item.customValue = strings.TrimSpace(m.form.input.Value())
			}
			item.value = item.customValue
		} else {
			item.value = option.value
		}
	} else if item.secret {
		item.value = m.form.input.Value()
	} else {
		item.value = strings.TrimSpace(m.form.input.Value())
	}
	if item.label == methodFieldLabel {
		m.syncHostPasswordField()
	}
}

func (m *App) moveForm(direction int, reveal bool) bool {
	next := m.form.index + direction
	for next >= 0 && next < len(m.form.fields) && m.form.fields[next].hidden {
		next += direction
	}
	if next < 0 || next >= len(m.form.fields) {
		return false
	}
	newlyRevealed := direction > 0 && next > m.form.revealed
	if newlyRevealed {
		if !reveal {
			return false
		}
		m.form.revealed = next
	}
	m.form.index = next
	m.form.selecting = newlyRevealed && len(m.form.fields[next].options) > 0
	m.focusFormField()
	return true
}

func formProgress(f *form) (current, total int) {
	for i, item := range f.fields {
		if item.hidden {
			continue
		}
		total++
		if i <= f.index {
			current++
		}
	}
	return current, total
}

func hasNextFormField(f *form) bool {
	for i := f.index + 1; i < len(f.fields); i++ {
		if !f.fields[i].hidden {
			return true
		}
	}
	return false
}
