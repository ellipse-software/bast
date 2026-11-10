package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud/sync"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

const (
	entrySectionBasics  = "Basics"
	entrySectionSignIn  = "Sign-in"
	entrySectionSession = "Session"
	entrySectionCustom  = "Customisation"
)

func (m *App) openAddEntryChooser() {
	m.openForm("Add", "entry_kind", []field{{
		label: "Kind",
		options: []fieldOption{
			{label: "Remote Desktop", value: metadata.EntryRDP},
			{label: "Web page", value: metadata.EntryWeb},
		},
	}})
}

func (m *App) openEditEntryForm() {
	entry, ok := m.selectedEntry()
	if !ok {
		return
	}
	m.openEntryForm(entry.Kind, entry)
}

func (m *App) openEntryForm(kind string, existing metadata.Entry) {
	title := "Add Remote Desktop"
	if kind == metadata.EntryWeb {
		title = "Add web page"
	}
	group := existing.Group
	label := existing.Label
	if existing.ID == "" {
		group = m.defaultAddGroup()
		label = ""
	} else {
		title = "Edit " + existing.Label
	}
	fields := []field{
		{label: "ID", value: existing.ID, hidden: true},
		{label: "Kind", value: kind, hidden: true},
		{label: "Label", section: entrySectionBasics, description: descHostLabel, value: metadata.JoinLabelPath(group, label)},
	}
	if kind == metadata.EntryRDP {
		fields = append(fields,
			field{label: "Address", section: entrySectionBasics, description: "Hostname or IP the jump connects to", value: existing.Address},
			field{label: "Port", section: entrySectionBasics, description: "Blank uses 3389", value: existing.Port, placeholder: "3389", optional: true},
			m.viaField(existing.Via),
			field{label: "Username", section: entrySectionSignIn, description: "Blank asks in the Remote Desktop client", value: existing.User, optional: true},
			field{label: "Domain", section: entrySectionSignIn, description: "Blank leaves the domain empty", value: existing.Domain, optional: true},
			onOffField("Full screen", entrySectionSession, "On opens full screen", existing.Fullscreen),
			onOffField("Admin session", entrySectionSession, "Connect to the admin session", existing.Admin),
		)
	} else {
		fields = append(fields,
			field{label: "URL", section: entrySectionBasics, description: "http or https address", value: existing.URL, placeholder: "https://192.168.0.1/"},
			m.viaField(existing.Via),
		)
	}
	fields = append(fields,
		field{label: "Icon", section: entrySectionCustom, description: descHostIcon, value: existing.Icon, placeholder: "🇩🇪", optional: true},
		colorFormField(existing.Color, entrySectionCustom),
		field{label: "Tags", section: entrySectionCustom, description: descHostTags, value: strings.Join(existing.Tags, ", "), optional: true},
		field{label: "Environment", section: entrySectionCustom, description: descHostEnvironment, value: existing.Environment, optional: true},
		field{label: "Notes", section: entrySectionCustom, description: descHostNotes, value: existing.Notes, optional: true},
	)
	m.openForm(title, "entry_save", fields)
	if m.form != nil {
		m.form.revealed = len(m.form.fields) - 1
	}
}

func onOffField(label, section, description string, on bool) field {
	item := field{
		label:       label,
		section:     section,
		description: description,
		optional:    true,
		options: []fieldOption{
			{label: "Off", value: ""},
			{label: "On", value: "on"},
		},
	}
	if on {
		item.selected = 1
		item.value = "on"
	}
	return item
}

func (m *App) submitEntryKind(values map[string]string) (tea.Model, tea.Cmd) {
	kind := values["Kind"]
	if kind == "" {
		kind = metadata.EntryRDP
	}
	m.openEntryForm(kind, metadata.Entry{})
	return m, nil
}

func (m *App) submitEntrySave(values map[string]string) (tea.Model, tea.Cmd) {
	group, label, err := metadata.SplitLabelPath(values["Label"])
	if err != nil {
		return m.formError(err.Error())
	}
	if strings.TrimSpace(label) == "" {
		return m.formError("label is required")
	}
	via, err := m.resolveVia(values["Via"])
	if err != nil {
		return m.formError(err.Error())
	}
	icon, color, appearanceErr := normalizeAppearance(values["Icon"], values["Color"])
	if appearanceErr != nil {
		return m.formError(appearanceErr.Error())
	}
	entry := metadata.Entry{
		ID:          values["ID"],
		Kind:        values["Kind"],
		Label:       label,
		Group:       group,
		Tags:        splitCSV(values["Tags"]),
		Environment: values["Environment"],
		Color:       color,
		Icon:        icon,
		Notes:       values["Notes"],
		Via:         via,
	}
	if entry.Kind == metadata.EntryWeb {
		entry.URL = values["URL"]
	} else {
		entry.Kind = metadata.EntryRDP
		entry.Address = values["Address"]
		entry.Port = values["Port"]
		entry.User = values["Username"]
		entry.Domain = values["Domain"]
		entry.Fullscreen = values["Full screen"] == "on"
		entry.Admin = values["Admin session"] == "on"
	}
	saved, err := m.metadata.PutEntry(entry)
	if err != nil {
		return m.formError(err.Error())
	}
	m.selectAfterLoadSection = hostsSection
	m.selectAfterLoadEntry = saved.ID
	notice := "Remote Desktop saved"
	if saved.Kind == metadata.EntryWeb {
		notice = "Web page saved"
	}
	return m.finishMutation(nil, notice)
}

func (m *App) openDeleteEntryForm() {
	entry, ok := m.selectedEntry()
	if !ok {
		return
	}
	m.openForm("Delete "+entry.Label, "entry_delete", []field{
		{label: "ID", value: entry.ID, hidden: true},
		{label: "Confirmation", value: entry.Label, hidden: true},
		{label: "Type the name to confirm", placeholder: entry.Label},
	})
}

func (m *App) submitEntryDelete(values map[string]string) (tea.Model, tea.Cmd) {
	if values["Type the name to confirm"] != values["Confirmation"] {
		return m.formValidationError("Name does not match")
	}
	entry, ok := m.metadata.Entry(values["ID"])
	if !ok {
		return m.formError("that entry is gone")
	}
	if m.tunnelSup != nil {
		if entry.Kind == metadata.EntryWeb {
			m.tunnelSup.CloseSOCKS(entry.Via)
		} else {
			m.tunnelSup.CloseRDP(entry.ID)
		}
	}
	err := m.metadata.DeleteEntry(entry.ID)
	return m.finishMutation(err, "Entry deleted")
}

func (m *App) openEntryGroupForm() {
	entry, ok := m.selectedEntry()
	if !ok {
		return
	}
	m.openForm("Move "+entry.Label, "entry_group", []field{
		{label: "ID", value: entry.ID, hidden: true},
		{label: "Current group", value: entry.Group, hidden: true},
		m.groupAssignmentField(entry.Group),
	})
	m.form.selecting = false
	m.form.input.SetValue("")
	m.form.input.Placeholder = "Search or create a group"
	m.form.input.Focus()
}

func (m *App) submitEntryGroup(values map[string]string) (tea.Model, tea.Cmd) {
	group, err := metadata.NormalizeGroupPath(values["Group"])
	if err != nil {
		return m.formValidationError(err.Error())
	}
	if sync.IsSyncedGroup(group) {
		return m.formValidationError("cloud sync groups are read-only")
	}
	collapsedGroups, collapsedPaths, collapseChanged := m.collapsedGroupsRevealing(group)
	err = m.metadata.MoveEntry(values["ID"], group, collapsedPaths)
	if err == nil {
		m.collapsedGroups = collapsedGroups
		if collapseChanged {
			m.collapseRevision++
		}
		m.selectAfterLoadSection = hostsSection
		m.selectAfterLoadEntry = values["ID"]
	}
	return m.finishMutation(err, "Entry group saved")
}

func (m *App) viaField(currentAlias string) field {
	labels := map[string]int{}
	for _, host := range m.hosts {
		labels[strings.ToLower(m.hostLabel(host))]++
	}
	item := field{
		label:       "Via",
		section:     entrySectionBasics,
		description: "SSH host to tunnel through. Direct opens on this machine",
		optional:    true,
		options:     []fieldOption{{label: "Direct", value: ""}},
	}
	for _, host := range m.hosts {
		label := m.hostLabel(host)
		if labels[strings.ToLower(label)] > 1 {
			label += " · " + host.Alias
		}
		item.options = append(item.options, fieldOption{label: label, value: host.Alias})
		if host.Alias == currentAlias {
			item.selected = len(item.options) - 1
			item.value = host.Alias
		}
	}
	return item
}

func viaPickerField(f *form, item *field) bool {
	return f != nil && item != nil && f.action == "entry_save" && item.label == "Via"
}

func (m *App) viaPickerChoices() []fieldOption {
	f := m.form
	item := f.fieldByLabel("Via")
	if item == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(f.input.Value()))
	if query == "" {
		return item.options
	}
	choices := make([]fieldOption, 0, len(item.options))
	for _, option := range item.options {
		hay := strings.ToLower(option.label + " " + option.value)
		if strings.Contains(hay, query) {
			choices = append(choices, option)
		}
	}
	return choices
}

func (m *App) commitViaSelection() {
	f := m.form
	if f == nil {
		return
	}
	index := f.fieldIndex("Via")
	if index < 0 {
		return
	}
	choices := m.viaPickerChoices()
	if len(choices) == 0 {
		return
	}
	selected := f.fields[index].selected
	if selected < 0 || selected >= len(choices) {
		selected = 0
	}
	choice := choices[selected]
	item := &f.fields[index]
	for i, option := range item.options {
		if option.value == choice.value && option.label == choice.label {
			item.selected = i
			item.value = option.value
			return
		}
	}
}

func (m *App) updateViaPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := m.form
	index := f.fieldIndex("Via")
	if index < 0 {
		return m, nil
	}
	item := &f.fields[index]
	key := msg.String()
	switch key {
	case "up", "k", "shift+tab":
		choices := m.viaPickerChoices()
		if len(choices) == 0 {
			return m, nil
		}
		item.selected = (item.selected - 1 + len(choices)) % len(choices)
		return m, nil
	case "down", "j":
		choices := m.viaPickerChoices()
		if len(choices) == 0 {
			return m, nil
		}
		item.selected = (item.selected + 1) % len(choices)
		return m, nil
	case "tab", "enter":
		m.commitViaSelection()
		f.input.SetValue("")
		if !m.moveForm(1, true) {
			return m.submitForm()
		}
		return m, nil
	case "esc", "backspace", "ctrl+h":
		if strings.TrimSpace(f.input.Value()) != "" && key != "esc" {
			break
		}
		if strings.TrimSpace(f.input.Value()) != "" {
			f.input.SetValue("")
			m.restoreViaSelection()
			return m, nil
		}
		if key == "esc" {
			if !m.moveForm(-1, false) {
				m.form = nil
			}
			return m, nil
		}
	}
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != before {
		item.selected = 0
	}
	return m, cmd
}

func (m *App) restoreViaSelection() {
	f := m.form
	item := f.fieldByLabel("Via")
	if item == nil {
		return
	}
	for i, option := range item.options {
		if option.value == item.value {
			item.selected = i
			return
		}
	}
	item.selected = 0
}

func (m *App) resolveVia(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if host, ok := m.findHost(raw); ok {
		return host.Alias, nil
	}
	normalized := sshconfig.NormalizeAlias(raw)
	if host, ok := m.findHost(normalized); ok {
		return host.Alias, nil
	}
	var matches []string
	for _, host := range m.hosts {
		if strings.EqualFold(m.hostLabel(host), raw) {
			matches = append(matches, host.Alias)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", errViaAmbiguous(raw)
	}
	return "", errViaMissing(raw)
}

func errViaAmbiguous(name string) error {
	return viaError("more than one host is named " + name + ". Use the SSH name")
}

func errViaMissing(name string) error {
	return viaError("no host named " + name)
}

type viaError string

func (e viaError) Error() string { return string(e) }

func (m *App) viaLabel(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ""
	}
	if host, ok := m.findHost(alias); ok {
		return m.hostLabel(host)
	}
	return alias
}
