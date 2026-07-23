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

func (m *App) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" {
		m.form = nil
		return m, nil
	}
	if key == "up" || key == "shift+tab" {
		m.form.fields[m.form.index].value = strings.TrimSpace(m.form.input.Value())
		m.moveForm(-1, false)
		return m, nil
	}
	if key == "down" {
		m.form.fields[m.form.index].value = strings.TrimSpace(m.form.input.Value())
		m.moveForm(1, false)
		return m, nil
	}
	if key == "enter" || key == "tab" {
		m.form.fields[m.form.index].value = strings.TrimSpace(m.form.input.Value())
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
	var cmd tea.Cmd
	m.form.input, cmd = m.form.input.Update(msg)
	return m, cmd
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
		input := sshconfig.HostInput{
			Alias: values["Label"], HostName: values["Hostname"], User: values["User"], Port: values["Port"],
			IdentityFile: values["Identity file"], IdentitiesOnly: strings.EqualFold(values["Identities only"], "yes"), ProxyJump: values["Proxy jump"],
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
			meta := metadata.Host{Group: values["Group"], Tags: splitCSV(values["Tags"]), Environment: values["Environment"], Color: values["Color"], Notes: values["Notes"]}
			if oldAlias != "" {
				old := m.metadata.Host(oldAlias)
				meta.Favorite, meta.Hidden, meta.LastUsedAt, meta.ConnectionCount = old.Favorite, old.Hidden, old.LastUsedAt, old.ConnectionCount
				if oldAlias != input.Alias {
					_ = m.metadata.RenameHost(oldAlias, input.Alias)
				}
			}
			err = m.metadata.SetHost(input.Alias, meta)
		}
		return m.finishMutation(err, "Host saved")
	case "metadata_edit":
		alias := values["Label"]
		old := m.metadata.Host(alias)
		old.Group, old.Tags, old.Environment, old.Color, old.Notes = values["Group"], splitCSV(values["Tags"]), values["Environment"], values["Color"], values["Notes"]
		return m.finishMutation(m.metadata.SetHost(alias, old), "Host metadata saved")
	case "host_delete":
		alias := values["Label"]
		if values["Type the name to confirm"] != alias {
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
		return m.finishMutation(m.keyring.Import(privateSource, publicSource, values["Name"], values["Comment"]), "Key imported")
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
	case "key_delete":
		key, ok := m.findKey(values["Key"])
		if !ok {
			return m.formError("key no longer exists")
		}
		return m.finishMutation(m.keyring.Delete(key, values["Type the name to confirm"]), "Key permanently deleted")
	case "known_delete":
		alias := values["Label"]
		if values["Type the name to confirm"] != alias {
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
	m.loading, m.status, m.statusError = true, success, false
	return m, m.loadCmd()
}

func (m *App) formError(message string) (tea.Model, tea.Cmd) {
	m.status, m.statusError = message, true
	return m, nil
}

func (m *App) openAddHostForm() {
	m.openForm("Add host", "host_add", []field{
		{label: "Label", placeholder: "prod"}, {label: "Hostname", placeholder: "server.example.com"}, {label: "User", placeholder: "ubuntu"},
		{label: "Port", placeholder: "22"}, {label: "Identity file", placeholder: "~/.ssh/bast/keys/work"}, {label: "Identities only", placeholder: "yes or no"},
		{label: "Proxy jump", placeholder: "bastion"}, {label: "Group"}, {label: "Tags", placeholder: "web, production"}, {label: "Environment", placeholder: "production"},
		{label: "Color", placeholder: "#7C3AED"}, {label: "Notes"},
	})
}

func (m *App) openEditHostForm() {
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	meta := m.metadata.Host(host.Alias)
	if !host.Managed {
		m.openForm("Edit metadata — "+host.Alias, "metadata_edit", []field{
			{label: "Label", value: host.Alias, hidden: true}, {label: "Group", value: meta.Group}, {label: "Tags", value: strings.Join(meta.Tags, ", ")},
			{label: "Environment", value: meta.Environment}, {label: "Color", value: meta.Color}, {label: "Notes", value: meta.Notes},
		})
		return
	}
	identity := ""
	if len(host.Resolved.IdentityFiles) > 0 {
		identity = host.Resolved.IdentityFiles[0]
	}
	m.openForm("Edit host — "+host.Alias, "host_edit", []field{
		{label: "Original label", value: host.Alias, hidden: true}, {label: "Label", value: host.Alias}, {label: "Hostname", value: host.Resolved.HostName}, {label: "User", value: host.Resolved.User},
		{label: "Port", value: host.Resolved.Port}, {label: "Identity file", value: identity}, {label: "Identities only", value: host.Resolved.IdentitiesOnly},
		{label: "Proxy jump", value: emptyIfNone(host.Resolved.ProxyJump)}, {label: "Group", value: meta.Group}, {label: "Tags", value: strings.Join(meta.Tags, ", ")},
		{label: "Environment", value: meta.Environment}, {label: "Color", value: meta.Color}, {label: "Notes", value: meta.Notes},
	})
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
	m.openForm("Delete host — "+host.Alias, "host_delete", []field{{label: "Label", value: host.Alias, hidden: true}, {label: "Type the name to confirm"}})
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
func (m *App) openDeleteKeyForm() {
	if key, ok := m.selectedKey(); ok {
		m.openForm("Delete key — "+key.Name, "key_delete", []field{{label: "Key", value: key.Name, hidden: true}, {label: "Type the name to confirm"}})
	}
}
func (m *App) openKnownHostForm() {
	if host, ok := m.selectedHost(); ok {
		m.openForm("Remove known host — "+host.Alias, "known_delete", []field{{label: "Label", value: host.Alias, hidden: true}, {label: "Type the name to confirm"}})
	}
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
	m.focusFormField()
}

func (m *App) focusFormField() {
	f := m.form
	f.input.SetValue(f.fields[f.index].value)
	f.input.Placeholder = f.fields[f.index].placeholder
	f.input.SetCursor(len([]rune(f.fields[f.index].value)))
	f.input.Focus()
}

func (m *App) moveForm(direction int, reveal bool) bool {
	next := m.form.index + direction
	for next >= 0 && next < len(m.form.fields) && m.form.fields[next].hidden {
		next += direction
	}
	if next < 0 || next >= len(m.form.fields) {
		return false
	}
	if direction > 0 && next > m.form.revealed {
		if !reveal {
			return false
		}
		m.form.revealed = next
	}
	m.form.index = next
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
