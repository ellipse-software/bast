package ui

import (
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/platform"
	"bast/internal/telemetry"
)

type paletteItem struct {
	keys  []string
	chord string
	label string
	group paletteGroup
}

func (m *App) openPalette() {
	m.palette = true
	m.paletteOffset = 0
}

func (m *App) closePalette() {
	m.palette = false
	m.paletteOffset = 0
}

// slashOpensSheet is false when "/" is a character or the chmod octal shortcut.
func (m *App) slashOpensSheet() bool {
	if m.typingSlash() {
		return false
	}
	if m.section == filesSection {
		m.initFilesState()
		if m.files.chmod.active && !m.files.chmod.octalEdit {
			return false
		}
	}
	return true
}

// typingSlash is true when "/" is a character in the focused field.
func (m *App) typingSlash() bool {
	if strings.HasPrefix(m.search, "\x00") {
		return true
	}
	if m.form != nil && m.formTextInputActive() {
		return true
	}
	if m.section != filesSection {
		return false
	}
	m.initFilesState()
	if m.files.jump.active {
		return true
	}
	if m.files.chmod.active && m.files.chmod.octalEdit {
		return true
	}
	pane := m.filesFocusedPane()
	if pane.pathEdit {
		return true
	}
	return pane.pickingHost() && strings.HasPrefix(pane.hostSearch, "\x00")
}

func (m *App) paletteItems() []paletteItem {
	switch {
	case m.statusError && m.status != "":
		return m.errorPaletteItems()
	case m.onboarding:
		return m.onboardingPaletteItems()
	case m.vaultBusy != "" && m.form == nil:
		return []paletteItem{{keys: []string{"esc"}, label: "cancel", group: groupApp}}
	case m.form != nil:
		return m.formPaletteItems()
	case m.section == filesSection && m.filesModeActive():
		return m.filesModePaletteItems()
	case strings.HasPrefix(m.search, "\x00"):
		return m.searchPaletteItems()
	default:
		return m.catalogPaletteItems()
	}
}

func (m *App) filesModeActive() bool {
	if m.section != filesSection {
		return false
	}
	m.initFilesState()
	if m.filesOverlay() {
		return true
	}
	return m.filesFocusedPane().pathEdit
}

func (m *App) catalogPaletteItems() []paletteItem {
	claimed := map[string]bool{}
	var items []paletteItem
	for _, scope := range m.matchScopes() {
		for _, b := range catalog() {
			if b.Scope != scope || !b.Show || b.ID == ActionNone {
				continue
			}
			if b.Available != nil && !b.Available(m) {
				continue
			}
			keys := make([]string, 0, len(b.Keys))
			for _, key := range b.Keys {
				if claimed[key] {
					continue
				}
				keys = append(keys, key)
			}
			if len(keys) == 0 {
				continue
			}
			for _, key := range keys {
				claimed[key] = true
			}
			items = append(items, paletteItem{
				keys:  keys,
				chord: b.Chord,
				label: bindingLabel(b, m),
				group: b.Group,
			})
		}
	}
	if m.scopeActive(ScopeGlobal) {
		for _, sec := range m.visibleSections() {
			digit, ok := m.tabDigit(sec)
			if !ok || claimed[digit] {
				continue
			}
			claimed[digit] = true
			items = append(items, paletteItem{keys: []string{digit}, label: sectionTitle(sec), group: groupNavigate})
		}
	}
	return items
}

func (m *App) searchPaletteItems() []paletteItem {
	return []paletteItem{
		{keys: []string{"enter"}, label: "apply", group: groupSelection},
		{keys: []string{"backspace", "ctrl+h"}, chord: "⌫", label: "delete", group: groupSelection},
		{keys: []string{"esc"}, label: "clear", group: groupApp},
	}
}

func (m *App) errorPaletteItems() []paletteItem {
	items := []paletteItem{
		{keys: []string{"enter", "esc", "backspace"}, chord: "enter", label: "return", group: groupSelection},
		{keys: []string{"q"}, label: "quit", group: groupApp},
	}
	if telemetry.Enabled() {
		items = append(items, paletteItem{keys: []string{"space"}, chord: "␣", label: "send report", group: groupSelection})
	}
	if m.maxErrorOffset() > 0 {
		items = append(items,
			paletteItem{keys: []string{"up", "k"}, chord: "↑ k", label: "up", group: groupNavigate},
			paletteItem{keys: []string{"down", "j"}, chord: "↓ j", label: "down", group: groupNavigate},
		)
	}
	return items
}

func (m *App) onboardingPaletteItems() []paletteItem {
	return []paletteItem{
		{keys: []string{"enter"}, label: "hosts", group: groupSelection},
	}
}

func (m *App) formPaletteItems() []paletteItem {
	if isGroupAssignmentForm(m.form) {
		return []paletteItem{
			{keys: []string{"enter"}, label: "assign", group: groupSelection},
			{keys: []string{"up", "down"}, chord: "↑ ↓", label: "choose", group: groupNavigate},
			{keys: []string{"esc"}, label: "cancel", group: groupApp},
		}
	}
	if isHostForm(m.form) {
		return m.hostFormPaletteItems()
	}
	return m.genericFormPaletteItems()
}

func savePaletteItem() paletteItem {
	chord := "Ctrl+S"
	if runtime.GOOS == "darwin" {
		chord = "Cmd+S"
	}
	return paletteItem{
		keys:  []string{"ctrl+s", "ctrl+j", "super+s", "meta+s", "cmd+s", "ctrl+enter"},
		chord: chord,
		label: "save",
		group: groupSelection,
	}
}

func (m *App) hostFormPaletteItems() []paletteItem {
	f := m.form
	text := m.formTextInputActive()
	items := []paletteItem{savePaletteItem()}
	enter := "next"
	switch {
	case f.screen == formScreenAdvancedHub:
		enter = "open"
	case f.selecting:
		enter = "select"
	default:
		if f.index >= 0 && f.index < len(f.fields) {
			item := f.fields[f.index]
			if len(item.options) > 0 && !item.options[item.selected].custom && !text {
				enter = "change"
			}
		}
	}
	items = append(items, paletteItem{keys: []string{"enter"}, label: enter, group: groupSelection})
	switch {
	case f.selecting && f.index >= 0 && f.index < len(f.fields) && f.fields[f.index].label == "Color":
		items = append(items, paletteItem{keys: []string{"left", "right", "up", "down"}, chord: "← → ↑ ↓", label: "highlight", group: groupNavigate})
	case f.selecting:
		items = append(items, paletteItem{keys: []string{"up", "down"}, chord: "↑ ↓", label: "choose", group: groupNavigate})
	default:
		items = append(items, paletteItem{keys: []string{"up", "down"}, chord: "↑ ↓", label: "move", group: groupNavigate})
	}
	cancel := "cancel"
	if f.screen == formScreenAdvancedHub {
		cancel = "back"
	}
	if f.index >= 0 && f.index < len(f.fields) {
		if hint := customChoiceEscapeHint(f.fields[f.index], f.input.Value()); hint != "" {
			cancel = strings.TrimPrefix(strings.TrimPrefix(hint, "⌫/Esc "), "Esc ")
		}
	}
	items = append(items, paletteItem{keys: []string{"esc"}, label: cancel, group: groupApp})
	if !text {
		items = append(items, paletteItem{keys: []string{"q"}, label: "quit", group: groupApp})
	}
	return items
}

func (m *App) genericFormPaletteItems() []paletteItem {
	f := m.form
	items := []paletteItem{}
	action := "next"
	switch {
	case f.selecting:
		action = "select"
	case isEditForm(f) && !f.selecting:
		action = "save"
	case !isEditForm(f) && !hasNextFormField(f):
		action = "save"
		switch f.action {
		case "host_delete", "key_delete", "known_delete", "files_delete":
			action = "delete"
		case "vault_reset_passphrase":
			action = "reset"
		case "vault_rotate_passphrase":
			action = "rotate"
		}
	}
	if !f.selecting && len(f.fields[f.index].options) > 0 && !f.fields[f.index].options[f.fields[f.index].selected].custom {
		if isEditForm(f) {
			action = "save"
			items = append(items, paletteItem{keys: []string{"space"}, chord: "␣", label: "change", group: groupSelection})
		} else {
			action = "change"
		}
	}
	items = append(items, paletteItem{keys: []string{"enter"}, label: action, group: groupSelection})
	if destructiveConfirmationTarget(f) != "" {
		items = append(items, paletteItem{keys: []string{"ctrl+y"}, chord: "Ctrl+Y", label: "copy name", group: groupSelection})
	}
	move := "revisit"
	if isEditForm(f) {
		move = "move"
	}
	if f.selecting && f.fields[f.index].label == "Color" {
		items = append(items, paletteItem{keys: []string{"left", "right", "up", "down"}, chord: "← → ↑ ↓", label: "highlight", group: groupNavigate})
	} else {
		items = append(items, paletteItem{keys: []string{"up", "down"}, chord: "↑ ↓", label: move, group: groupNavigate})
	}
	escape := "cancel"
	if hint := customChoiceEscapeHint(f.fields[f.index], f.input.Value()); hint != "" {
		escape = strings.TrimPrefix(strings.TrimPrefix(hint, "⌫/Esc "), "Esc ")
	}
	items = append(items, paletteItem{keys: []string{"esc"}, label: escape, group: groupApp})
	return items
}

func (m *App) filesModePaletteItems() []paletteItem {
	m.initFilesState()
	switch {
	case m.files.chmod.active && m.files.chmod.octalEdit:
		return []paletteItem{
			{keys: []string{"enter"}, label: "apply", group: groupSelection},
			{keys: []string{"esc"}, label: "back", group: groupApp},
		}
	case m.files.chmod.active:
		return []paletteItem{
			{keys: []string{"space"}, chord: "␣", label: "toggle", group: groupSelection},
			{keys: []string{"enter"}, label: "apply", group: groupSelection},
			{keys: []string{"up", "down", "left", "right"}, chord: "↑ ↓ ← →", label: "move", group: groupNavigate},
			{keys: []string{"esc"}, label: "close", group: groupApp},
		}
	case m.files.preview.active:
		items := []paletteItem{
			{keys: []string{"o", "esc"}, chord: "o", label: "close", group: groupSelection},
			{keys: []string{"j", "k"}, chord: "j k", label: "scroll", group: groupNavigate},
			{keys: []string{"[", "]"}, chord: "[ ]", label: "file", group: groupNavigate},
		}
		if m.filesCanChmodFromPreview() {
			items = append(items, paletteItem{keys: []string{"p"}, label: "permissions", group: groupSelection})
		}
		return items
	case m.files.info:
		items := []paletteItem{
			{keys: []string{"i", "esc"}, chord: "esc", label: "close", group: groupSelection},
			{keys: []string{"j", "k"}, chord: "j k", label: "next", group: groupNavigate},
			{keys: []string{"o"}, label: "preview", group: groupSelection},
		}
		pane := m.filesFocusedPane()
		if !(pane.kind == filesPaneLocal && !platform.SupportsPOSIXPermissions()) {
			items = append(items, paletteItem{keys: []string{"p"}, label: "permissions", group: groupSelection})
		}
		return items
	case m.files.transfer.active:
		return []paletteItem{{keys: []string{"esc", "x"}, chord: "esc", label: "cancel", group: groupSelection}}
	case m.filesFocusedPane().connecting:
		return []paletteItem{{keys: []string{"esc", "x"}, chord: "esc", label: "cancel", group: groupSelection}}
	case m.files.jump.active:
		return []paletteItem{
			{keys: []string{"enter"}, label: "jump", group: groupSelection},
			{keys: []string{"backspace"}, chord: "⌫", label: "delete", group: groupSelection},
			{keys: []string{"esc"}, label: "close", group: groupApp},
		}
	case m.filesFocusedPane().pathEdit:
		return []paletteItem{
			{keys: []string{"enter"}, label: "open", group: groupSelection},
			{keys: []string{"esc"}, label: "cancel", group: groupApp},
		}
	default:
		return []paletteItem{
			{keys: []string{"enter"}, label: "connect", group: groupSelection},
			{keys: []string{"backspace"}, chord: "⌫", label: "delete", group: groupSelection},
			{keys: []string{"esc"}, label: "clear", group: groupApp},
		}
	}
}

func (m *App) filesCanChmodFromPreview() bool {
	pane := m.filesFocusedPane()
	if pane.kind == filesPaneLocal && !platform.SupportsPOSIXPermissions() {
		return false
	}
	return true
}

func (m *App) paletteHasKey(key string) bool {
	for _, item := range m.paletteItems() {
		if keyIn(item.keys, key) {
			return true
		}
	}
	return false
}

func (m *App) paletteContent(s styleSet, width int) []string {
	items := m.paletteItems()
	if len(items) == 0 {
		return []string{s.muted.Render("Nothing to do in this view")}
	}
	var lines []string
	for _, group := range []paletteGroup{groupSelection, groupNavigate, groupApp} {
		var groupItems []paletteItem
		for _, item := range items {
			if item.group == group {
				groupItems = append(groupItems, item)
			}
		}
		if len(groupItems) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, s.muted.Render(paletteGroupTitle(group)))
		lines = append(lines, packPaletteRows(s, groupItems, width, m.nerdFont, m.windowsTerminal)...)
	}
	return lines
}

func paletteGroupTitle(group paletteGroup) string {
	switch group {
	case groupNavigate:
		return "Move"
	case groupApp:
		return "App"
	default:
		return "Selection"
	}
}

func packPaletteRows(s styleSet, items []paletteItem, width int, nerd, windows bool) []string {
	if width < 1 {
		width = 1
	}
	cells := make([]string, 0, len(items))
	for _, item := range items {
		cells = append(cells, paletteCell(s, item, width, nerd, windows))
	}
	var rows []string
	current := ""
	for _, cell := range cells {
		if current == "" {
			current = cell
			continue
		}
		next := current + "  " + cell
		if lipgloss.Width(next) > width {
			rows = append(rows, current)
			current = cell
			continue
		}
		current = next
	}
	if current != "" {
		rows = append(rows, current)
	}
	return rows
}

func paletteCell(s styleSet, item paletteItem, width int, nerd, windows bool) string {
	chord := item.chord
	if chord == "" {
		chord = formatChord(Binding{Keys: item.keys}, nerd, windows)
	}
	label := item.label
	cell := s.value.Render(chord) + "  " + s.muted.Render(label)
	if lipgloss.Width(cell) <= width {
		return cell
	}
	room := width - lipgloss.Width(chord) - 2
	if room < 1 {
		return s.value.Render(truncate(chord, width))
	}
	return s.value.Render(chord) + "  " + s.muted.Render(truncate(label, room))
}

func (m *App) paletteBand() int {
	if !m.palette {
		return 0
	}
	content := len(m.paletteContent(m.styles(), m.terminalWidth()))
	n := content + 1
	room := max(2, m.terminalHeight()-4)
	half := max(2, m.terminalHeight()/2)
	limit := min(room, half)
	if n > limit {
		return limit
	}
	return n
}

func (m *App) paletteVisibleRows() int {
	return max(1, m.paletteBand()-1)
}

func (m *App) paletteMaxOffset() int {
	content := len(m.paletteContent(m.styles(), m.terminalWidth()))
	return max(0, content-m.paletteVisibleRows())
}

func (m *App) clampPaletteOffset() {
	if m.paletteOffset < 0 {
		m.paletteOffset = 0
	}
	if maxOffset := m.paletteMaxOffset(); m.paletteOffset > maxOffset {
		m.paletteOffset = maxOffset
	}
}

func (m *App) scrollPalette(delta int) {
	m.paletteOffset += delta
	m.clampPaletteOffset()
}

func (m *App) paletteOverflow() bool {
	return m.paletteMaxOffset() > 0
}

func (m *App) renderPalette(s styleSet) string {
	width := m.terminalWidth()
	rule := s.rule.Render(strings.Repeat("─", width))
	content := m.paletteContent(s, width)
	visible := m.paletteVisibleRows()
	m.clampPaletteOffset()
	offset := m.paletteOffset
	end := min(len(content), offset+visible)
	lines := content[offset:end]
	for len(lines) < visible {
		lines = append(lines, "")
	}
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			lines[i] = truncate(line, width)
		}
	}
	return rule + "\n" + strings.Join(lines, "\n")
}

func (m *App) sheetRows() int {
	return m.paletteBand()
}

func (m *App) bodyHeight() int {
	return max(1, m.terminalHeight()-3-m.sheetRows())
}
