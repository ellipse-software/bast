package ui

import (
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

const (
	formSectionBasics        = "basics"
	formSectionAuth          = "auth"
	formSectionAdvanced      = "advanced"
	formSectionMetadata      = "metadata"
	formSectionCustomisation = "customisation"
	hostFormSaveButtonLabel  = " Save "
	hostSaveHintInterval     = 2 * time.Second
	rdpFieldLabel            = "Open RDP"
	rdpOnValue               = "on"
)

type hostHubItem struct {
	id      string
	title   string
	section string
}

func isHostForm(f *form) bool {
	switch f.action {
	case "host_add", "host_edit", "history_host_add", "metadata_edit":
		return true
	default:
		return false
	}
}

func (m *App) hostFormSaveButtonBounds() (x, y, width int) {
	width = lipgloss.Width(hostFormSaveButtonLabel)
	x = max(2, m.terminalWidth()-2-width)
	s := m.styles()
	header := m.renderHeader(s) + "\n" + m.renderHeaderRule(s)
	y = lipgloss.Height(lipgloss.NewStyle().Width(m.terminalWidth()).Render(header)) + 1
	return x, y, width
}

func (m *App) renderHostFormHeader(s styleSet, breadcrumb string) string {
	x, _, _ := m.hostFormSaveButtonBounds()
	text := m.form.title
	if breadcrumb != "" {
		text += "  " + breadcrumb
	}
	available := max(0, x-3)
	if available == 0 {
		text = ""
	} else {
		text = ansi.Truncate(text, available, "…")
	}

	var b strings.Builder
	b.WriteString("\n  ")
	if text != "" {
		b.WriteString(s.active.Render(text))
	}
	currentX := 2 + lipgloss.Width(text)
	if currentX < x {
		b.WriteString(strings.Repeat(" ", x-currentX))
	}
	b.WriteString(s.title.Render(hostFormSaveButtonLabel))
	b.WriteString("\n\n")
	return b.String()
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

func hostHubInline(id string) bool {
	return id != "advanced"
}

func hostHubItems(f *form) []hostHubItem {
	var items []hostHubItem
	if f.action != "metadata_edit" {
		items = append(items, hostHubFields(f, []string{"Hostname", "User", methodFieldLabel, passwordFieldLabel, "Port"}, formSectionBasics)...)
	}
	items = append(items, hostHubFields(f, []string{"Label", "Icon", "Color", "Tags", "Environment", "Notes", rdpFieldLabel}, formSectionCustomisation)...)
	if f.action != "metadata_edit" {
		items = append(items, hostHubItem{id: "advanced", title: "Advanced", section: formSectionAdvanced})
	}
	return items
}

func hostHubFields(f *form, labels []string, section string) []hostHubItem {
	items := make([]hostHubItem, 0, len(labels))
	for _, label := range labels {
		field := f.fieldByLabel(label)
		if field == nil || field.hidden {
			continue
		}
		items = append(items, hostHubItem{id: label, title: label, section: section})
	}
	return items
}

func (m *App) hostFormSummary(section string) string {
	f := m.form
	switch section {
	case formSectionAuth:
		identity := authSummary(f)
		if identity == "" {
			return "OpenSSH defaults"
		}
		return identity
	case formSectionAdvanced:
		return m.hostAdvancedSummary()
	case formSectionMetadata:
		parts := []string{}
		if env := fieldDisplay(f, "Environment"); env != "" && env != "-" {
			parts = append(parts, env)
		}
		if tags := fieldDisplay(f, "Tags"); tags != "" && tags != "-" {
			parts = append(parts, tags)
		}
		if rdp := fieldDisplay(f, rdpFieldLabel); rdp == "On" {
			parts = append(parts, "RDP")
		}
		if len(parts) == 0 {
			return "-"
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
			return "Password"
		}
		return item.options[item.selected].label
	}
	value := strings.TrimSpace(item.value)
	if value == "" {
		return "-"
	}
	return value
}

func authSummary(f *form) string {
	item := f.fieldByLabel(methodFieldLabel)
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
	m.hostSaveHintID++
	m.hostSaveHintEnter = false
	m.syncHostPasswordField()
	m.focusHostHubItem()
}

func (m *App) hostSaveHintTickCmd() tea.Cmd {
	hintID := m.hostSaveHintID
	return tea.Tick(hostSaveHintInterval, func(time.Time) tea.Msg {
		return hostSaveHintTickMsg(hintID)
	})
}

func (m *App) focusHostHubItem() {
	f := m.form
	items := hostHubItems(f)
	if f.hubIndex < 0 || f.hubIndex >= len(items) {
		f.hubIndex = 0
	}
	item := items[f.hubIndex]
	if hostHubInline(item.id) {
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
	want := ""
	switch section {
	case formSectionAuth:
		want = methodFieldLabel
	case formSectionMetadata:
		section = formSectionCustomisation
	}
	for i, item := range hostHubItems(m.form) {
		if item.id == "advanced" || item.section != section {
			continue
		}
		if want != "" && item.title != want {
			continue
		}
		m.commitHostHubField()
		m.form.screen = "hub"
		m.form.hubIndex = i
		m.form.selecting = false
		m.focusHostHubItem()
		return
	}
}

func (m *App) syncHostHubIndex() {
	f := m.form
	if f == nil || !isHostForm(f) || (f.screen != "" && f.screen != "hub") || f.index < 0 || f.index >= len(f.fields) {
		return
	}
	label := f.fields[f.index].label
	for i, item := range hostHubItems(f) {
		if item.id != "advanced" && item.title == label {
			f.hubIndex = i
			return
		}
	}
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
	if hostHubInline(item.id) {
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
	if hostHubInline(item.id) {
		m.commitFormField()
		if f.hubIndex+1 < len(items) {
			f.hubIndex++
			m.focusHostHubItem()
		}
		return m, nil
	}
	m.enterHostSection(item.section)
	return m, nil
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
		if item.label == methodFieldLabel && item.value == passwordOnlyIdentity {
			if idx := f.fieldIndex(passwordFieldLabel); idx >= 0 && !f.fields[idx].hidden {
				f.index = idx
				f.selecting = false
				m.focusFormField()
			}
		}
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

	if isHostFormSaveKey(msg) {
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
	if hostHubInline(item.id) {
		return m.updateHostHubField(msg)
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

func (m *App) updateHostHubField(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	f := m.form
	if f.index < 0 || f.index >= len(f.fields) {
		return m, nil
	}
	item := &f.fields[f.index]

	if f.selecting {
		switch key {
		case "esc", "backspace", "ctrl+h":
			f.selecting = false
			m.focusFormField()
		case "up", "down", "j", "k", "left", "right", "h", "l":
			m.applyChoiceMove(item, key)
		case "shift+tab":
			f.selecting = false
			m.commitFormField()
			m.moveHostHub(-1)
		case "enter", "tab":
			return m.hostHubChoiceEnter()
		}
		return m, nil
	}

	if key == "esc" && len(item.options) > 0 && item.options[item.selected].custom {
		m.commitFormField()
		f.selecting = true
		m.focusFormField()
		return m, nil
	}
	if m.backspaceOpensChoices(item, key) {
		return m, nil
	}
	if key == "esc" {
		m.form = nil
		return m, nil
	}
	if (key == "backspace" || key == "ctrl+h") && len(item.options) > 0 && !item.options[item.selected].custom {
		return m, nil
	}
	if key == "up" || key == "shift+tab" {
		m.commitFormField()
		m.moveHostHub(-1)
		return m, nil
	}
	if key == "down" || key == "tab" {
		m.commitFormField()
		m.moveHostHub(1)
		return m, nil
	}
	if key == "space" && len(item.options) > 0 && !item.options[item.selected].custom {
		f.selecting = true
		m.focusFormField()
		return m, nil
	}
	if key == "enter" {
		if len(item.options) > 0 && !item.options[item.selected].custom {
			f.selecting = true
			m.focusFormField()
			return m, nil
		}
		m.commitFormField()
		m.moveHostHub(1)
		return m, nil
	}
	if len(item.options) > 0 && !item.options[item.selected].custom {
		return m, nil
	}
	var cmd tea.Cmd
	m.form.input, cmd = m.form.input.Update(msg)
	return m, cmd
}

func (m *App) hostHubChoiceEnter() (tea.Model, tea.Cmd) {
	f := m.form
	item := &f.fields[f.index]
	f.selecting = false
	m.focusFormField()
	if item.options[item.selected].custom {
		return m, nil
	}
	m.commitFormField()
	if item.label == methodFieldLabel && item.value == passwordOnlyIdentity {
		if idx := f.fieldIndex(passwordFieldLabel); idx >= 0 && !f.fields[idx].hidden {
			f.index = idx
			f.selecting = false
			m.syncHostHubIndex()
			m.focusFormField()
		}
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
		case "up", "down", "j", "k", "left", "right", "h", "l":
			m.applyChoiceMove(item, key)
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
	if m.backspaceOpensChoices(item, key) {
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
	header := m.renderHostFormHeader(s, "")
	lines, focus := m.hostHubLines(s)
	room := max(1, m.terminalHeight()-3-lipgloss.Height(header))
	start := 0
	if len(lines) > room {
		start = scrollStart(focus, len(lines), room)
	}
	end := min(len(lines), start+room)
	var b strings.Builder
	b.WriteString(header)
	for _, line := range lines[start:end] {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func hostHubSectionTitle(section string) string {
	switch section {
	case formSectionBasics:
		return "Basics"
	case formSectionCustomisation:
		return "Customisation"
	case formSectionAdvanced:
		return "Advanced"
	default:
		return ""
	}
}

func (m *App) hostHubLines(s styleSet) ([]string, int) {
	f := m.form
	var lines []string
	focus := 0
	section := ""
	for _, hub := range hostHubItems(f) {
		if hub.section != section {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "  "+s.active.Render(hostHubSectionTitle(hub.section)))
			section = hub.section
		}
		start := len(lines)
		if hub.id == "advanced" {
			lines = append(lines, m.hostHubMenuLines(s, hub)...)
		} else {
			lines = append(lines, m.hostHubFieldLines(s, hub)...)
		}
		if f.hubIndex >= 0 && hostHubItems(f)[f.hubIndex].id == hub.id && start < len(lines) {
			focus = start
			if f.selecting {
				focus = len(lines) - 1
			}
		}
	}
	return lines, focus
}

func (m *App) hostHubFieldLines(s styleSet, hub hostHubItem) []string {
	f := m.form
	idx := f.fieldIndex(hub.title)
	if idx < 0 {
		return nil
	}
	item := f.fields[idx]
	active := f.hubIndex >= 0 && hostHubItems(f)[f.hubIndex].id == hub.id
	if !active {
		value := hubFieldValue(item)
		if hub.id == "Label" {
			value = metadata.LabelLeaf(strings.TrimSpace(item.value))
			if value == "" {
				value = "-"
			}
		}
		return []string{"  " + s.muted.Render("  "+item.label+"  "+value)}
	}
	lines := []string{"  " + s.active.Render("› "+item.label)}
	if item.description != "" {
		lines = append(lines, "    "+s.muted.Render(truncate(item.description, max(20, m.terminalWidth()-8))))
	}
	switch {
	case len(item.options) > 0 && f.selecting && item.label == "Color":
		for _, line := range renderColorGrid(s, item.options, item.selected, item.customValue, m.terminalWidth()) {
			lines = append(lines, "    "+line)
		}
	case len(item.options) > 0 && f.selecting:
		rows := min(7, len(item.options))
		start := scrollStart(item.selected, len(item.options), rows)
		for optionIndex := start; optionIndex < min(len(item.options), start+rows); optionIndex++ {
			option := item.options[optionIndex]
			lines = append(lines, "    "+renderSelectOption(s, option.label, option.value, optionIndex == item.selected))
		}
	case len(item.options) > 0 && item.options[item.selected].custom:
		lines = append(lines, "    "+f.input.View())
		if hint := customColorReturnHint(s, item, f.input.Value()); hint != "" {
			lines = append(lines, hint)
		}
	case len(item.options) > 0:
		option := item.options[item.selected]
		lines = append(lines, "    "+renderClosedChoice(s, option.label, option.value))
	case hub.id == "Label":
		lines = append(lines, "    "+m.renderLabelPathInput(s))
	default:
		lines = append(lines, "    "+f.input.View())
	}
	if item.label == "Color" {
		if preview := colorPreviewLine(s, formColorSample(f), focusedColorValue(item, f.input.Value())); preview != "" {
			preview = strings.TrimRight(preview, "\n")
			for _, line := range strings.Split(preview, "\n") {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

func hubFieldValue(item field) string {
	value := strings.TrimSpace(item.value)
	if len(item.options) > 0 && !item.options[item.selected].custom {
		value = item.options[item.selected].label
	} else if item.secret {
		if value != "" {
			return "********"
		}
		return "-"
	}
	if value == "" {
		return "-"
	}
	return value
}

// Slashes stay muted. The inactive side of the path (group vs leaf) is muted
// based on cursor position: cursor in the leaf mutes the group; cursor in the
// group mutes the leaf.
func (m *App) renderLabelPathInput(s styleSet) string {
	f := m.form
	value := f.input.Value()
	if value == "" {
		return f.input.View()
	}
	runes := []rune(value)
	pos := f.input.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	lastSlash := -1
	for i, r := range runes {
		if r == '/' {
			lastSlash = i
		}
	}
	cursorInLeaf := lastSlash >= 0 && pos > lastSlash

	var out strings.Builder
	for i, r := range runes {
		char := string(r)
		style := s.value
		switch {
		case r == '/':
			style = s.muted
		case lastSlash >= 0 && cursorInLeaf && i < lastSlash:
			style = s.muted
		case lastSlash >= 0 && !cursorInLeaf && i > lastSlash:
			style = s.muted
		}
		if i == pos {
			out.WriteString(lipgloss.NewStyle().Reverse(true).Render(char))
			continue
		}
		out.WriteString(style.Render(char))
	}
	if pos >= len(runes) {
		out.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	return out.String()
}

func (m *App) hostHubMenuLines(s styleSet, hub hostHubItem) []string {
	f := m.form
	active := f.hubIndex >= 0 && hostHubItems(f)[f.hubIndex].id == hub.id
	summary := m.hostFormSummary(hub.section)
	if summary == "" {
		summary = "-"
	}
	line := hub.title + "  " + summary
	if active {
		return []string{
			"  " + s.selected.Render("› "+truncate(line, max(20, m.terminalWidth()-6))),
			"    " + s.muted.Render("Enter to configure"),
		}
	}
	return []string{"  " + s.muted.Render("  "+truncate(line, max(20, m.terminalWidth()-4)))}
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
	b.WriteString(m.renderHostFormHeader(s, breadcrumb))

	indices := f.sectionFieldIndices(f.screen)
	for _, idx := range indices {
		item := f.fields[idx]
		if idx == f.index {
			b.WriteString("  " + s.active.Render("› "+item.label) + "\n")
			if item.description != "" {
				b.WriteString("    " + s.muted.Render(truncate(item.description, max(20, m.terminalWidth()-8))) + "\n")
			}
			if len(item.options) > 0 {
				if f.selecting && item.label == "Color" {
					for _, line := range renderColorGrid(s, item.options, item.selected, item.customValue, m.terminalWidth()) {
						b.WriteString("    " + line + "\n")
					}
				} else if f.selecting {
					rows := min(7, len(item.options))
					start := scrollStart(item.selected, len(item.options), rows)
					for optionIndex := start; optionIndex < min(len(item.options), start+rows); optionIndex++ {
						option := item.options[optionIndex]
						b.WriteString("    " + renderSelectOption(s, option.label, option.value, optionIndex == item.selected) + "\n")
					}
				} else if item.options[item.selected].custom {
					b.WriteString("    " + f.input.View() + "\n")
					if hint := customColorReturnHint(s, item, f.input.Value()); hint != "" {
						b.WriteString(hint + "\n")
					}
				} else {
					option := item.options[item.selected]
					b.WriteString("    " + renderClosedChoice(s, option.label, option.value) + "\n")
				}
			} else {
				b.WriteString("    " + f.input.View() + "\n")
			}
			if item.label == "Color" {
				if preview := colorPreviewLine(s, formColorSample(f), focusedColorValue(item, f.input.Value())); preview != "" {
					b.WriteString(preview)
				}
			}
			continue
		}
		value := item.value
		if len(item.options) > 0 && !item.options[item.selected].custom {
			value = item.options[item.selected].label
		} else if item.secret {
			if strings.TrimSpace(item.value) != "" {
				value = "********"
			} else {
				value = "-"
			}
		}
		if value == "" {
			value = "-"
		}
		b.WriteString("  " + s.muted.Render("  "+item.label+"  "+value) + "\n")
	}
	return b.String()
}

func isHostFormSaveKey(msg tea.KeyPressMsg) bool {
	if msg.Code == tea.KeyEnter && msg.Mod&tea.ModCtrl != 0 {
		return true
	}
	// Keystroke keeps the modifier when Text is also set. Ctrl+J stays as an
	// unadvertised alias for terminals that swallow Ctrl+S.
	switch msg.Keystroke() {
	case "ctrl+s", "ctrl+j", "super+s", "meta+s", "cmd+s":
		return true
	default:
		return false
	}
}

func hostFormSaveHint(showEnterKey bool) string {
	if showEnterKey {
		return "Ctrl+↵ save"
	}
	if runtime.GOOS == "darwin" {
		return "Cmd+S save"
	}
	return "Ctrl+S save"
}

func hostFormHint(f *form, textInputActive, showEnterKey bool) string {
	save := hostFormSaveHint(showEnterKey)
	if f.screen == formScreenAdvancedHub {
		return "Enter open section • ↑/↓ or j/k move • ⌫/Esc back • " + save + " • q quit"
	}
	if f.screen != "" && f.screen != "hub" {
		if f.selecting {
			if f.fields[f.index].label == "Color" {
				return "←/→/↑/↓ highlight • Enter select • Esc back • q quit"
			}
			return "↑/↓ or j/k choose • Enter select • ⌫/Esc back • q quit"
		}
		item := f.fields[f.index]
		action := "Enter next"
		if len(item.options) > 0 && !item.options[item.selected].custom {
			action = "Enter change"
		}
		if hint := customChoiceEscapeHint(item, f.input.Value()); hint != "" {
			return action + " • " + hint + " • " + save
		}
		hint := action + " • ↑/↓ or Tab move • ⌫/Esc back"
		if !textInputActive {
			hint += " • q quit"
		}
		return hint + " • " + save
	}

	items := hostHubItems(f)
	if f.hubIndex >= 0 && f.hubIndex < len(items) {
		hub := items[f.hubIndex]
		if hostHubInline(hub.id) {
			idx := f.fieldIndex(hub.title)
			if idx >= 0 {
				item := f.fields[idx]
				if f.selecting && item.label == "Color" {
					return "←/→/↑/↓ highlight • Enter select • Esc back • q quit"
				}
				if hint := customChoiceEscapeHint(item, f.input.Value()); hint != "" && !f.selecting {
					return "Enter next • ↑/↓ move • " + hint + " • " + save
				}
				if len(item.options) > 0 && !item.options[item.selected].custom {
					if f.selecting {
						return "↑/↓ or j/k choose • Enter select • ⌫/Esc back • q quit"
					}
					return "Enter change • ↑/↓ or Tab move • " + save + " • q quit • Esc cancel"
				}
			}
			return "Enter next • ↑/↓ or Tab move • " + save + " • q type • Esc cancel"
		}
	}
	return "Enter open section • ↑/↓ or j/k move • Tab next • ⌫/Esc cancel • " + save + " • q quit"
}

func hostFormFields(m *App, meta metadataHostValues, conn hostConnectionValues, hidden []field) []field {
	fields := append([]field{}, hidden...)
	labelDesc := meta.labelDesc
	if labelDesc == "" {
		labelDesc = descHostLabel
	}
	fields = append(fields,
		field{label: "Label", section: formSectionCustomisation, description: labelDesc, value: meta.label, placeholder: "Work/api"},
	)
	if conn.includeConnection {
		method := m.methodField(conn.identity, conn.passwordOnly)
		method.section = formSectionBasics
		fields = append(fields,
			field{label: "Hostname", section: formSectionBasics, description: descHostHostname, value: conn.hostname, placeholder: "server.example.com"},
			field{label: "User", section: formSectionBasics, description: descHostUser, value: conn.user, placeholder: "ubuntu", optional: true},
			method,
			m.passwordField(conn.passwordStored),
			field{label: "Port", section: formSectionBasics, description: descHostPort, value: conn.port, placeholder: "22", optional: true},
		)
		fields = append(fields, advancedFormFields(m, conn.advanced)...)
	}
	fields = append(fields,
		field{label: "Tags", section: formSectionCustomisation, description: descHostTags, value: meta.tags, placeholder: "web, production", optional: true},
		field{label: "Environment", section: formSectionCustomisation, description: descHostEnvironment, value: meta.environment, placeholder: "production", optional: true},
		field{label: "Icon", section: formSectionCustomisation, description: descHostIcon, value: meta.icon, placeholder: "🇩🇪", optional: true},
		colorFormField(meta.color, formSectionCustomisation),
		field{label: "Notes", section: formSectionCustomisation, description: descHostNotes, value: meta.notes, optional: true},
		rdpFormField(meta.rdp),
	)
	return fields
}

func colorFormField(current, section string) field {
	item := field{
		label:       "Color",
		section:     section,
		description: descHostColor,
		placeholder: "#7C3AED",
		optional:    true,
		options:     []fieldOption{{label: "Default", value: ""}},
	}
	for _, color := range metadata.ANSIPalette() {
		item.options = append(item.options, fieldOption{label: color.Label, value: color.Token})
	}
	item.options = append(item.options, fieldOption{label: "Custom hex", custom: true})
	normalized, err := metadata.NormalizeColor(current)
	if err != nil || normalized == "" {
		item.value = ""
		return item
	}
	if _, ok := metadata.ANSICode(normalized); ok {
		for i, option := range item.options {
			if !option.custom && option.value == normalized {
				item.selected = i
				item.value = normalized
				return item
			}
		}
	}
	item.selected = len(item.options) - 1
	item.customValue = normalized
	item.value = normalized
	return item
}

func rdpFormField(enabled bool) field {
	item := field{
		label:       rdpFieldLabel,
		section:     formSectionCustomisation,
		description: descHostRDP,
		optional:    true,
		options: []fieldOption{
			{label: "Off", value: ""},
			{label: "On", value: rdpOnValue},
		},
	}
	if enabled {
		item.selected = 1
		item.value = rdpOnValue
	}
	return item
}

type metadataHostValues struct {
	label, tags, environment, color, icon, notes, labelDesc string
	rdp                                                     bool
}

type hostConnectionValues struct {
	includeConnection    bool
	hostname, user, port string
	identity             string
	passwordOnly         bool
	passwordStored       bool
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
