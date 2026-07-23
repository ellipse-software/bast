package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

const passwordOnlyIdentity = "\x00password-only"

func (m *App) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	f := m.form
	item := &f.fields[f.index]
	if f.selecting {
		switch key {
		case "esc":
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
	var cmd tea.Cmd
	m.form.input, cmd = m.form.input.Update(msg)
	return m, cmd
}

func isEditForm(f *form) bool {
	return f.action == "host_edit" || f.action == "metadata_edit" || f.action == "key_comment"
}

func (m *App) updateFormPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	f := m.form
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
	case "host_add", "host_edit":
		label := strings.TrimSpace(values["Label"])
		group := strings.TrimSpace(values["Group"])
		groupCreated := group != "" && !m.groupExists(group)
		identityFile := values["Identity file"]
		passwordOnly := identityFile == passwordOnlyIdentity
		if passwordOnly {
			identityFile = ""
		}
		input := sshconfig.HostInput{
			Alias: sshconfig.NormalizeAlias(label), HostName: values["Hostname"], User: values["User"], Port: values["Port"],
			IdentityFile: identityFile, IdentitiesOnly: !passwordOnly && strings.EqualFold(values["Identities only"], "yes"), PasswordOnly: passwordOnly, ProxyJump: values["Proxy jump"],
		}
		oldAlias := values["Original label"]
		var err error
		if f.action == "host_add" {
			_, err = m.config.Add(input)
		} else {
			host, ok := m.findHost(oldAlias)
			if !ok {
				err = fmt.Errorf("host %q no longer exists", oldAlias)
			} else {
				err = m.config.Update(host.ManagedID, input)
			}
		}
		if err == nil {
			meta := metadata.Host{Label: label, Group: group, Tags: splitCSV(values["Tags"]), Environment: values["Environment"], Color: values["Color"], Notes: values["Notes"]}
			if meta.Label == input.Alias {
				meta.Label = ""
			}
			if oldAlias != "" {
				old := m.metadata.Host(oldAlias)
				meta.Favorite, meta.Hidden, meta.LastUsedAt, meta.ConnectionCount = old.Favorite, old.Hidden, old.LastUsedAt, old.ConnectionCount
				if oldAlias != input.Alias {
					_ = m.metadata.RenameHost(oldAlias, input.Alias)
				}
			}
			err = m.metadata.SetHost(input.Alias, meta)
		}
		if err == nil {
			m.selectAfterLoadSection = hostsSection
			if groupCreated {
				m.selectAfterLoadName, m.selectAfterLoadGroup = group, true
			} else if f.action == "host_add" {
				m.selectAfterLoadName, m.selectAfterLoadGroup = input.Alias, false
			}
		}
		return m.finishMutation(err, "Host saved")
	case "metadata_edit":
		alias := values["Alias"]
		old := m.metadata.Host(alias)
		group := strings.TrimSpace(values["Group"])
		groupCreated := group != "" && !m.groupExists(group)
		old.Label, old.Group, old.Tags, old.Environment, old.Color, old.Notes = values["Label"], group, splitCSV(values["Tags"]), values["Environment"], values["Color"], values["Notes"]
		if old.Label == alias {
			old.Label = ""
		}
		err := m.metadata.SetHost(alias, old)
		if err == nil && groupCreated {
			m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = hostsSection, group, true
		}
		return m.finishMutation(err, "Host metadata saved")
	case "host_delete":
		alias := values["Alias"]
		if values["Type the name to confirm"] != values["Confirmation"] {
			return m.formError("confirmation did not match the exact host label")
		}
		host, ok := m.findHost(alias)
		if !ok {
			return m.formError("host no longer exists")
		}
		err := m.config.Delete(host.ManagedID)
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
			return m.formError("type EXPORT to acknowledge private-key export")
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
		return m.finishMutation(m.keyring.Delete(key, values["Type the name to confirm"]), "Key permanently deleted")
	case "known_delete":
		alias := values["Alias"]
		if values["Type the name to confirm"] != values["Confirmation"] {
			return m.formError("confirmation did not match the host label")
		}
		host, ok := m.findHost(alias)
		if !ok {
			return m.formError("host no longer exists")
		}
		err := m.openSSH.RemoveKnownHost(context.Background(), host.Resolved.HostName, host.Resolved.Port)
		return m.finishMutation(err, "Known-host entry removed")
	}
	return m.formError("unknown form action")
}

func (m *App) finishMutation(err error, success string) (tea.Model, tea.Cmd) {
	if err != nil {
		return m.formError(err.Error())
	}
	m.form = nil
	m.loading = true
	return m, tea.Batch(m.loadCmd(), m.setNotice(success))
}

func (m *App) formError(message string) (tea.Model, tea.Cmd) {
	if m.form != nil {
		message = m.form.title + " failed: " + message
	}
	m.statusID++
	m.status, m.statusError = message, true
	return m, nil
}

func (m *App) openAddHostForm() {
	m.openForm("Add host", "host_add", []field{
		{label: "Label", description: "Required — spaces are shown here and become underscores in the SSH name.", placeholder: "Production web"},
		{label: "Hostname", description: "Required — the server address or IP to connect to.", placeholder: "server.example.com"},
		{label: "User", description: "Remote login name; blank uses your OpenSSH default.", placeholder: "ubuntu", optional: true},
		{label: "Port", description: "Blank uses the standard SSH port, 22.", placeholder: "22", optional: true},
		m.identityField("", false),
		{label: "Identities only", description: "Limit SSH to configured identity files instead of every key in ssh-agent.", placeholder: "yes or no", optional: true},
		{label: "Proxy jump", description: "Route through another SSH host, such as a bastion.", placeholder: "bastion", optional: true},
		{label: "Group", description: "Organise related hosts for sorting and search.", optional: true},
		{label: "Tags", description: "Comma-separated terms included in search.", placeholder: "web, production", optional: true},
		{label: "Environment", description: "For example production, staging, or development.", placeholder: "production", optional: true},
		{label: "Color", description: "Hex colour used for the host label, for example #7C3AED.", placeholder: "#7C3AED", optional: true},
		{label: "Notes", description: "Short context shown in the host details and search.", optional: true},
	})
}

func (m *App) openEditHostForm() {
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	meta := m.metadata.Host(host.Alias)
	if !host.Managed {
		m.openForm("Edit metadata — "+m.hostLabel(host), "metadata_edit", []field{
			{label: "Alias", value: host.Alias, hidden: true},
			{label: "Label", description: "Friendly name shown in Bast; the SSH name remains " + host.Alias + ".", value: m.hostLabel(host)},
			{label: "Group", description: "Organise related hosts for sorting and search.", value: meta.Group, optional: true},
			{label: "Tags", description: "Comma-separated terms included in search.", value: strings.Join(meta.Tags, ", "), optional: true},
			{label: "Environment", description: "For example production, staging, or development.", value: meta.Environment, optional: true},
			{label: "Color", description: "Hex colour used for the host label, for example #7C3AED.", value: meta.Color, optional: true},
			{label: "Notes", description: "Short context shown in the host details and search.", value: meta.Notes, optional: true},
		})
		m.form.revealed = len(m.form.fields) - 1
		return
	}
	identity := ""
	isPasswordOnly := passwordOnly(host.Resolved)
	if !isPasswordOnly && len(host.Resolved.IdentityFiles) > 0 {
		identity = host.Resolved.IdentityFiles[0]
	}
	m.openForm("Edit host — "+m.hostLabel(host), "host_edit", []field{
		{label: "Original label", value: host.Alias, hidden: true},
		{label: "Label", description: "Friendly name shown in Bast; spaces become underscores in the SSH name.", value: m.hostLabel(host)},
		{label: "Hostname", description: "Required — the server address or IP to connect to.", value: host.Resolved.HostName},
		{label: "User", description: "Remote login name; blank uses your OpenSSH default.", value: host.Resolved.User, optional: true},
		{label: "Port", description: "Blank uses the standard SSH port, 22.", value: host.Resolved.Port, optional: true},
		m.identityField(identity, isPasswordOnly),
		{label: "Identities only", description: "Limit SSH to configured identity files instead of every key in ssh-agent.", value: host.Resolved.IdentitiesOnly, optional: true, hidden: isPasswordOnly},
		{label: "Proxy jump", description: "Route through another SSH host, such as a bastion.", value: emptyIfNone(host.Resolved.ProxyJump), optional: true},
		{label: "Group", description: "Organise related hosts for sorting and search.", value: meta.Group, optional: true},
		{label: "Tags", description: "Comma-separated terms included in search.", value: strings.Join(meta.Tags, ", "), optional: true},
		{label: "Environment", description: "For example production, staging, or development.", value: meta.Environment, optional: true},
		{label: "Color", description: "Hex colour used for the host label, for example #7C3AED.", value: meta.Color, optional: true},
		{label: "Notes", description: "Short context shown in the host details and search.", value: meta.Notes, optional: true},
	})
	m.form.revealed = len(m.form.fields) - 1
}

func (m *App) openDeleteHostForm() {
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	if !host.Managed {
		m.status, m.statusError = "External hosts cannot be deleted by Bast", true
		return
	}
	label := m.hostLabel(host)
	m.openForm("Delete host — "+label, "host_delete", []field{{label: "Alias", value: host.Alias, hidden: true}, {label: "Confirmation", value: label, hidden: true}, {label: "Type the name to confirm", placeholder: label}})
}

func (m *App) openGenerateForm() {
	m.openForm("Generate key", "key_generate", []field{{label: "Name", placeholder: "work"}, {label: "Algorithm", value: "ed25519", placeholder: "ed25519 or rsa"}})
}
func (m *App) openImportForm() {
	m.openForm("Import native keypair", "key_import", []field{
		{label: "Private key", placeholder: "file path or paste key contents"},
		{label: "Public key", placeholder: "optional path/content; Enter to derive"},
		{label: "Comment", placeholder: "optional; blank keeps an existing comment"},
		{label: "Name", placeholder: "work"},
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
	m.openForm("Edit key comment — "+key.Name, "key_comment", []field{
		{label: "Key", value: key.Name, hidden: true},
		{label: "Comment", value: key.Comment, placeholder: "blank removes the comment"},
	})
}
func (m *App) openExportForm() {
	if key, ok := m.selectedKey(); ok {
		m.openForm("Export key — "+key.Name, "key_export", []field{{label: "Key", value: key.Name, hidden: true}, {label: "Directory", placeholder: "~/Desktop"}, {label: "Type EXPORT"}})
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
		description: "SSH may ask for the server password. Existing authorized keys are left unchanged.",
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
		m.openForm("Delete key — "+key.Name, "key_delete", []field{{label: "Key", value: key.Name, hidden: true}, {label: "Type the name to confirm", placeholder: key.Name}})
	}
}
func (m *App) openKnownHostForm() {
	if host, ok := m.selectedHost(); ok {
		label := m.hostLabel(host)
		m.openForm("Remove known host — "+label, "known_delete", []field{{label: "Alias", value: host.Alias, hidden: true}, {label: "Confirmation", value: label, hidden: true}, {label: "Type the name to confirm", placeholder: label}})
	}
}

func (m *App) identityField(current string, passwordOnly bool) field {
	item := field{
		label:       "Identity file",
		description: "Choose password authentication, a key, or let OpenSSH and ssh-agent decide.",
		placeholder: "~/.ssh/id_ed25519",
		optional:    true,
	}
	item.options = append(item.options, fieldOption{label: "OpenSSH defaults / agent"})
	item.options = append(item.options, fieldOption{label: "Password only", value: passwordOnlyIdentity})
	if passwordOnly {
		item.selected = len(item.options) - 1
		item.value = passwordOnlyIdentity
		current = ""
	}
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
	return item
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
	item := &m.form.fields[m.form.index]
	if len(item.options) > 0 {
		option := item.options[item.selected]
		if option.custom {
			item.customValue = strings.TrimSpace(m.form.input.Value())
			item.value = item.customValue
		} else {
			item.value = option.value
		}
	} else {
		item.value = strings.TrimSpace(m.form.input.Value())
	}
	if item.label == "Identity file" {
		for i := range m.form.fields {
			if m.form.fields[i].label == "Identities only" {
				m.form.fields[i].hidden = item.value == passwordOnlyIdentity
				break
			}
		}
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
