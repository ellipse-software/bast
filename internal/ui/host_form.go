package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/sshconfig"
)

const (
	formSectionBasics   = "basics"
	formSectionAuth     = "auth"
	formSectionAdvanced = "advanced"
	formSectionMetadata = "metadata"
)

type hostHubItem struct {
	id      string
	title   string
	section string
}

func isHostForm(f *form) bool {
	switch f.action {
	case "host_add", "host_edit", "metadata_edit":
		return true
	default:
		return false
	}
}

func (f *form) fieldByLabel(label string) *field {
	for i := range f.fields {
		if f.fields[i].label == label {
			return &f.fields[i]
		}
	}
	return nil
}

func (f *form) fieldIndex(label string) int {
	for i, item := range f.fields {
		if item.label == label {
			return i
		}
	}
	return -1
}

func (f *form) sectionFieldIndices(section string) []int {
	out := []int{}
	for i, item := range f.fields {
		if item.hidden || item.section != section {
			continue
		}
		out = append(out, i)
	}
	return out
}

func hostHubItems(f *form) []hostHubItem {
	items := []hostHubItem{
		{id: "label", title: "Label", section: formSectionBasics},
	}
	if f.action != "metadata_edit" {
		items = append(items, hostHubItem{id: "hostname", title: "Hostname", section: formSectionBasics})
	}
	if f.action != "metadata_edit" {
		items = append(items,
			hostHubItem{id: "auth", title: "Authentication", section: formSectionAuth},
			hostHubItem{id: "advanced", title: "Advanced", section: formSectionAdvanced},
		)
	}
	items = append(items, hostHubItem{id: "metadata", title: "Metadata", section: formSectionMetadata})
	return items
}

func (m *App) hostFormSummary(section string) string {
	f := m.form
	switch section {
	case formSectionAuth:
		user := fieldDisplay(f, "User")
		port := fieldDisplay(f, "Port")
		identity := authSummary(f)
		parts := []string{}
		if user != "" && user != "—" {
			parts = append(parts, user)
		}
		if port != "" && port != "—" && port != "22" {
			parts = append(parts, "port "+port)
		}
		if identity != "" {
			parts = append(parts, identity)
		}
		if len(parts) == 0 {
			return "OpenSSH defaults"
		}
		return strings.Join(parts, " · ")
	case formSectionAdvanced:
		return m.hostAdvancedSummary()
	case formSectionMetadata:
		parts := []string{}
		if group := fieldDisplay(f, "Group"); group != "" && group != "—" {
			parts = append(parts, group)
		}
		if env := fieldDisplay(f, "Environment"); env != "" && env != "—" {
			parts = append(parts, env)
		}
		if tags := fieldDisplay(f, "Tags"); tags != "" && tags != "—" {
			parts = append(parts, tags)
		}
		if len(parts) == 0 {
			return "—"
		}
		return strings.Join(parts, " · ")
	default:
		return ""
	}
}

func fieldDisplay(f *form, label string) string {
	item := f.fieldByLabel(label)
	if item == nil {
		return ""
	}
	if len(item.options) > 0 && !item.options[item.selected].custom {
		if item.options[item.selected].value == passwordOnlyIdentity {
			return "Password only"
		}
		return item.options[item.selected].label
	}
	value := strings.TrimSpace(item.value)
	if value == "" {
		return "—"
	}
	return value
}

func authSummary(f *form) string {
	item := f.fieldByLabel("Identity file")
	if item == nil {
		return ""
	}
	if len(item.options) > 0 {
		option := item.options[item.selected]
		if option.value == passwordOnlyIdentity {
			return "password"
		}
		if option.custom {
			if v := strings.TrimSpace(item.customValue); v != "" {
				return v
			}
			if v := strings.TrimSpace(item.value); v != "" {
				return v
			}
		}
		if option.value == "" && option.label == "OpenSSH defaults / agent" {
			return ""
		}
		if option.value != "" {
			return option.label
		}
	}
	if v := strings.TrimSpace(item.value); v != "" {
		return v
	}
	return ""
}

func (m *App) openHostForm(title, action string, fields []field) {
	input := textinput.New()
	input.Prompt = ""
	input.SetWidth(max(20, m.width-20))
	m.form = &form{
		title: title, action: action, fields: fields, input: input,
		screen: "hub", hubIndex: 0,
	}
	m.focusHostHubItem()
}

func (m *App) focusHostHubItem() {
	f := m.form
	items := hostHubItems(f)
	if f.hubIndex < 0 || f.hubIndex >= len(items) {
		f.hubIndex = 0
	}
	item := items[f.hubIndex]
	if item.id == "label" || item.id == "hostname" {
		idx := f.fieldIndex(item.title)
		if idx >= 0 {
			f.index = idx
			f.selecting = false
			m.focusFormField()
		}
		return
	}
	f.index = -1
	f.selecting = false
	f.input.Blur()
}

func (m *App) enterHostSection(section string) {
	if section == formSectionAdvanced {
		m.enterAdvancedHub()
		return
	}
	f := m.form
	indices := f.sectionFieldIndices(section)
	if len(indices) == 0 {
		return
	}
	m.commitHostHubField()
	f.screen = section
	f.index = indices[0]
	f.selecting = false
	m.focusFormField()
}

func (m *App) exitHostSection() {
	f := m.form
	if isAdvancedSubsection(f.screen) {
		m.commitFormField()
		m.exitAdvancedSubsection()
		return
	}
	if f.screen == formScreenAdvancedHub {
		f.screen = "hub"
		for i, item := range hostHubItems(f) {
			if item.id == "advanced" {
				f.hubIndex = i
				break
			}
		}
		m.focusHostHubItem()
		return
	}
	if f.screen == "" || f.screen == "hub" {
		return
	}
	m.commitFormField()
	section := f.screen
	f.screen = "hub"
	items := hostHubItems(f)
	for i, item := range items {
		if item.section == section {
			f.hubIndex = i
			break
		}
	}
	m.focusHostHubItem()
}

func (m *App) commitHostHubField() {
	f := m.form
	items := hostHubItems(f)
	if f.hubIndex >= len(items) {
		return
	}
	item := items[f.hubIndex]
	if item.id == "label" || item.id == "hostname" {
		m.commitFormField()
	}
}

func (m *App) moveHostHub(direction int) {
	f := m.form
	items := hostHubItems(f)
	m.commitHostHubField()
	next := f.hubIndex + direction
	if next < 0 || next >= len(items) {
		return
	}
	f.hubIndex = next
	m.focusHostHubItem()
}

func (m *App) moveHostSection(direction int) bool {
	f := m.form
	indices := f.sectionFieldIndices(f.screen)
	current := -1
	for i, idx := range indices {
		if idx == f.index {
			current = i
			break
		}
	}
	if current < 0 {
		return false
	}
	next := current + direction
	if next < 0 || next >= len(indices) {
		return false
	}
	m.commitFormField()
	f.index = indices[next]
	f.selecting = false
	m.focusFormField()
	return true
}

func (m *App) hostHubEnter() (tea.Model, tea.Cmd) {
	f := m.form
	items := hostHubItems(f)
	if f.hubIndex >= len(items) {
		return m, nil
	}
	item := items[f.hubIndex]
	switch item.id {
	case "label", "hostname":
		m.commitFormField()
		if f.hubIndex+1 < len(items) {
			f.hubIndex++
			m.focusHostHubItem()
		}
		return m, nil
	default:
		m.enterHostSection(item.section)
		return m, nil
	}
}

func (m *App) hostSectionEnter() (tea.Model, tea.Cmd) {
	f := m.form
	item := &f.fields[f.index]
	if f.selecting {
		f.selecting = false
		m.focusFormField()
		if item.options[item.selected].custom {
			return m, nil
		}
		m.commitFormField()
		return m, nil
	}
	if len(item.options) > 0 && !item.options[item.selected].custom {
		f.selecting = true
		m.focusFormField()
		return m, nil
	}
	m.commitFormField()
	if !m.moveHostSection(1) {
		m.exitHostSection()
	}
	return m, nil
}

func (m *App) updateHostForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	f := m.form

	if msg.Code == tea.KeyEnter && msg.Mod&tea.ModCtrl != 0 {
		m.commitFormField()
		return m.submitForm()
	}

	if key == "ctrl+j" {
		m.commitFormField()
		return m.submitForm()
	}

	if f.screen == formScreenAdvancedHub {
		return m.updateAdvancedHubForm(msg)
	}
	if isAdvancedSubsection(f.screen) {
		return m.updateHostSectionForm(msg)
	}
	if f.screen != "" && f.screen != "hub" {
		return m.updateHostSectionForm(msg)
	}

	item := hostHubItems(f)[f.hubIndex]
	if item.id == "label" || item.id == "hostname" {
		switch key {
		case "esc":
			m.form = nil
			return m, nil
		case "up", "shift+tab":
			m.moveHostHub(-1)
			return m, nil
		case "down", "tab":
			m.moveHostHub(1)
			return m, nil
		case "enter":
			return m.hostHubEnter()
		case "backspace", "ctrl+h":
			var cmd tea.Cmd
			m.form.input, cmd = m.form.input.Update(msg)
			return m, cmd
		default:
			var cmd tea.Cmd
			m.form.input, cmd = m.form.input.Update(msg)
			return m, cmd
		}
	}

	switch key {
	case "esc":
		m.form = nil
		return m, nil
	case "up", "k", "shift+tab":
		m.moveHostHub(-1)
	case "down", "j", "tab":
		m.moveHostHub(1)
	case "backspace", "ctrl+h":
		m.form = nil
		return m, nil
	case "enter":
		return m.hostHubEnter()
	}
	return m, nil
}

func (m *App) updateHostSectionForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	f := m.form
	item := &f.fields[f.index]

	if msg.Code == tea.KeyEnter && msg.Mod&tea.ModCtrl != 0 {
		m.commitFormField()
		return m.submitForm()
	}

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
			if !m.moveHostSection(-1) {
				m.exitHostSection()
			}
		case "enter", "tab":
			return m.hostSectionEnter()
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
		m.exitHostSection()
		return m, nil
	}
	if (key == "backspace" || key == "ctrl+h") && len(item.options) > 0 && !item.options[item.selected].custom {
		m.commitFormField()
		m.exitHostSection()
		return m, nil
	}
	if key == "up" || key == "shift+tab" {
		m.commitFormField()
		if !m.moveHostSection(-1) {
			m.exitHostSection()
		}
		return m, nil
	}
	if key == "down" || key == "tab" {
		m.commitFormField()
		if !m.moveHostSection(1) {
			m.exitHostSection()
		}
		return m, nil
	}
	if key == "space" && len(item.options) > 0 && !item.options[item.selected].custom {
		f.selecting = true
		m.focusFormField()
		return m, nil
	}
	if key == "enter" {
		return m.hostSectionEnter()
	}
	if len(item.options) > 0 && !item.options[item.selected].custom {
		return m, nil
	}
	var cmd tea.Cmd
	m.form.input, cmd = m.form.input.Update(msg)
	return m, cmd
}

func (m *App) renderHostForm(s styleSet) string {
	f := m.form
	if f.screen == formScreenAdvancedHub {
		return m.renderAdvancedHubForm(s)
	}
	if f.screen != "" && f.screen != "hub" {
		return m.renderHostSectionForm(s)
	}
	return m.renderHostHubForm(s)
}

func (m *App) renderHostHubForm(s styleSet) string {
	f := m.form
	items := hostHubItems(f)
	var b strings.Builder
	b.WriteString("\n  " + s.active.Render(f.title) + "\n\n")
	b.WriteString("  " + s.active.Render("Basics") + "\n")
	for _, hub := range items {
		if hub.section == formSectionBasics {
			m.renderHostHubBasicsRow(&b, s, hub)
		}
	}

	b.WriteString("\n  " + s.muted.Render("Configure") + "\n")
	for _, hub := range items {
		if hub.section == formSectionBasics {
			continue
		}
		m.renderHostHubMenuRow(&b, s, hub)
	}

	return b.String()
}

func (m *App) renderHostHubBasicsRow(b *strings.Builder, s styleSet, hub hostHubItem) {
	f := m.form
	idx := f.fieldIndex(hub.title)
	if idx < 0 {
		return
	}
	item := f.fields[idx]
	active := f.hubIndex >= 0 && hostHubItems(f)[f.hubIndex].id == hub.id
	value := strings.TrimSpace(item.value)

	if active {
		b.WriteString("  " + s.active.Render("› "+item.label) + "\n")
		if item.description != "" {
			b.WriteString("    " + s.muted.Render(truncate(item.description, max(20, m.terminalWidth()-8))) + "\n")
		}
		b.WriteString("    " + f.input.View() + "\n")
		return
	}
	if value == "" {
		value = "—"
	}
	b.WriteString("  " + s.muted.Render("  "+item.label+"  "+value) + "\n")
}

func (m *App) renderHostHubMenuRow(b *strings.Builder, s styleSet, hub hostHubItem) {
	f := m.form
	active := f.hubIndex >= 0 && hostHubItems(f)[f.hubIndex].id == hub.id
	summary := m.hostFormSummary(hub.section)
	if summary == "" {
		summary = "—"
	}
	line := hub.title + "  " + summary
	if active {
		b.WriteString("  " + s.selected.Render("› "+truncate(line, max(20, m.terminalWidth()-6))) + "\n")
		b.WriteString("    " + s.muted.Render("Enter to configure") + "\n")
		return
	}
	b.WriteString("  " + s.muted.Render("  "+truncate(line, max(20, m.terminalWidth()-4))) + "\n")
}

func (m *App) renderHostSectionForm(s styleSet) string {
	f := m.form
	sectionTitle := f.screen
	breadcrumb := "› " + sectionTitle
	switch f.screen {
	case formSectionAuth:
		sectionTitle, breadcrumb = "Authentication", "› Authentication"
	case formSectionAdvancedJump:
		sectionTitle, breadcrumb = "Jump & proxy", "› Advanced › Jump & proxy"
	case formSectionAdvancedSession:
		sectionTitle, breadcrumb = "Session", "› Advanced › Session"
	case formSectionAdvancedForwarding:
		sectionTitle, breadcrumb = "Forwarding", "› Advanced › Forwarding"
	case formSectionAdvancedEnv:
		sectionTitle, breadcrumb = "Environment", "› Advanced › Environment"
	case formSectionAdvancedCustom:
		sectionTitle, breadcrumb = "Custom flags", "› Advanced › Custom flags"
	case formSectionMetadata:
		sectionTitle, breadcrumb = "Metadata", "› Metadata"
	}
	var b strings.Builder
	b.WriteString("\n  " + s.active.Render(f.title) + "  " + s.muted.Render(breadcrumb) + "\n\n")

	indices := f.sectionFieldIndices(f.screen)
	for _, idx := range indices {
		item := f.fields[idx]
		if idx == f.index {
			b.WriteString("  " + s.active.Render("› "+item.label) + "\n")
			if item.description != "" {
				b.WriteString("    " + s.muted.Render(truncate(item.description, max(20, m.terminalWidth()-8))) + "\n")
			}
			if len(item.options) > 0 {
				if f.selecting {
					rows := min(7, len(item.options))
					start := scrollStart(item.selected, len(item.options), rows)
					for optionIndex := start; optionIndex < min(len(item.options), start+rows); optionIndex++ {
						option := "  " + item.options[optionIndex].label
						if optionIndex == item.selected {
							option = s.selected.Render("› " + item.options[optionIndex].label)
						} else {
							option = s.muted.Render(option)
						}
						b.WriteString("    " + option + "\n")
					}
				} else if item.options[item.selected].custom {
					b.WriteString("    " + f.input.View() + "\n")
				} else {
					b.WriteString("    " + s.value.Render(item.options[item.selected].label) + "\n")
				}
			} else {
				b.WriteString("    " + f.input.View() + "\n")
			}
			if item.label == "Color" {
				colour := strings.TrimSpace(f.input.Value())
				if foreground, ok := contrastingTextColor(colour); ok {
					preview := lipgloss.NewStyle().Bold(true).
						Foreground(lipgloss.Color(foreground)).
						Background(lipgloss.Color(colour)).
						Padding(0, 1).
						Render("Host label preview")
					b.WriteString("    " + preview + "\n")
				}
			}
			continue
		}
		value := item.value
		if len(item.options) > 0 && !item.options[item.selected].custom {
			value = item.options[item.selected].label
		}
		if value == "" {
			value = "—"
		}
		b.WriteString("  " + s.muted.Render("  "+item.label+"  "+value) + "\n")
	}
	return b.String()
}

func hostFormHint(f *form, textInputActive bool) string {
	if f.screen == formScreenAdvancedHub {
		return "Enter open section • ↑/↓ or j/k move • ⌫/Esc back • Ctrl+Enter save • q quit"
	}
	if f.screen != "" && f.screen != "hub" {
		if f.selecting {
			return "↑/↓ or j/k choose • Enter select • ⌫/Esc back • q quit"
		}
		item := f.fields[f.index]
		action := "Enter next"
		if len(item.options) > 0 && !item.options[item.selected].custom {
			action = "Enter change"
		}
		if len(item.options) > 0 && item.options[item.selected].custom {
			return action + " • Esc choices • ⌫ edit • Ctrl+Enter save"
		}
		hint := action + " • ↑/↓ or Tab move • ⌫/Esc back"
		if !textInputActive {
			hint += " • q quit"
		}
		return hint + " • Ctrl+Enter save"
	}

	items := hostHubItems(f)
	if f.hubIndex >= 0 && f.hubIndex < len(items) {
		hub := items[f.hubIndex]
		if hub.id == "label" || hub.id == "hostname" {
			return "Enter next • ↑/↓ or Tab move • Ctrl+Enter save • q type • Esc cancel"
		}
	}
	return "Enter open section • ↑/↓ or j/k move • Tab next • ⌫/Esc cancel • Ctrl+Enter save • q quit"
}

func hostFormFields(m *App, meta metadataHostValues, conn hostConnectionValues, hidden []field) []field {
	fields := append([]field{}, hidden...)
	labelDesc := meta.labelDesc
	if labelDesc == "" {
		labelDesc = descHostLabel
	}
	fields = append(fields,
		field{label: "Label", section: formSectionBasics, description: labelDesc, value: meta.label, placeholder: "Production web"},
	)
	if conn.includeConnection {
		fields = append(fields,
			field{label: "Hostname", section: formSectionBasics, description: descHostHostname, value: conn.hostname, placeholder: "server.example.com"},
			field{label: "User", section: formSectionAuth, description: descHostUser, value: conn.user, placeholder: "ubuntu", optional: true},
			field{label: "Port", section: formSectionAuth, description: descHostPort, value: conn.port, placeholder: "22", optional: true},
		)
		fields = append(fields, m.identityField(conn.identity, conn.passwordOnly))
		fields[len(fields)-1].section = formSectionAuth
		fields = append(fields, advancedFormFields(m, conn.advanced)...)
	}
	fields = append(fields,
		field{label: "Group", section: formSectionMetadata, description: descHostGroup, value: meta.group, placeholder: "Work/Production", optional: true},
		field{label: "Tags", section: formSectionMetadata, description: descHostTags, value: meta.tags, placeholder: "web, production", optional: true},
		field{label: "Environment", section: formSectionMetadata, description: descHostEnvironment, value: meta.environment, placeholder: "production", optional: true},
		field{label: "Color", section: formSectionMetadata, description: descHostColor, value: meta.color, placeholder: "#7C3AED", optional: true},
		field{label: "Notes", section: formSectionMetadata, description: descHostNotes, value: meta.notes, optional: true},
	)
	return fields
}

type metadataHostValues struct {
	label, group, tags, environment, color, notes, labelDesc string
}

type hostConnectionValues struct {
	includeConnection    bool
	hostname, user, port string
	identity             string
	passwordOnly         bool
	advanced             sshconfig.AdvancedSettings
}

func (m *App) defaultAddGroup() string {
	if group, ok := m.selectedGroupHeader(); ok {
		return group
	}
	if group, ok := m.selectedGroup(); ok {
		return group
	}
	return ""
}
