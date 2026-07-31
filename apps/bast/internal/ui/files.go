package ui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/files"
	"bast/internal/sshconfig"
)

type filesPaneKind int

const (
	filesPaneLocal filesPaneKind = iota
	filesPaneRemote
)

type filesPane struct {
	kind        filesPaneKind
	cwd         string
	entries     []files.Entry
	cursor      int
	offset      int
	marked      map[string]struct{}
	rangeAnchor *int
	showHidden  bool
	pathEdit    bool
	pathInput   textinput.Model
	err         string
	loading     bool

	session    *files.Session
	alias      string
	connecting bool
	hostCursor int
	hostSearch string
}

type filesTransfer struct {
	cancel context.CancelFunc
	active bool
	move   bool
}

type filesJump struct {
	active bool
	query  string
	labels map[string]int // label -> index in pane.entries
}

type filesState struct {
	panes    [2]filesPane
	focus    int
	transfer filesTransfer
	jump     filesJump
	ready    bool
}

type filesListMsg struct {
	pane    int
	cwd     string
	entries []files.Entry
	err     error
}

type filesConnectMsg struct {
	pane    int
	session *files.Session
	cwd     string
	err     error
	alias   string
}

type filesTransferDoneMsg struct {
	err  error
	move bool
}

func (m *App) initFilesState() {
	if m.files.ready {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	m.files.panes[0] = filesPane{
		kind:      filesPaneLocal,
		cwd:       home,
		marked:    map[string]struct{}{},
		pathInput: newFilesPathInput(m.terminalWidth()),
	}
	m.files.panes[1] = filesPane{
		kind:      filesPaneRemote,
		marked:    map[string]struct{}{},
		pathInput: newFilesPathInput(m.terminalWidth()),
	}
	m.files.focus = 0
	m.files.ready = true
}

func newFilesPathInput(width int) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.SetWidth(max(20, width-8))
	return input
}

func (m *App) enterFilesSection() tea.Cmd {
	m.initFilesState()
	m.section = filesSection
	m.search = ""
	return m.refreshFilesPane(0)
}

func (m *App) openFilesForHost(host sshconfig.Host) tea.Cmd {
	m.initFilesState()
	m.section = filesSection
	m.search = ""
	m.files.focus = 1
	m.files.panes[1].kind = filesPaneRemote
	return tea.Batch(m.refreshFilesPane(0), m.connectFilesHost(1, host))
}

func (p *filesPane) clearMarks() {
	p.marked = map[string]struct{}{}
	p.rangeAnchor = nil
}

func (p *filesPane) selectedPaths() []string {
	if len(p.marked) > 0 {
		out := make([]string, 0, len(p.marked))
		for path := range p.marked {
			out = append(out, path)
		}
		return out
	}
	if p.cursor >= 0 && p.cursor < len(p.entries) {
		return []string{p.entries[p.cursor].Path}
	}
	return nil
}

func (p *filesPane) selectedEntry() (files.Entry, bool) {
	if p.cursor < 0 || p.cursor >= len(p.entries) {
		return files.Entry{}, false
	}
	return p.entries[p.cursor], true
}

func (p *filesPane) clamp() {
	if len(p.entries) == 0 {
		p.cursor = 0
		p.offset = 0
		return
	}
	if p.cursor >= len(p.entries) {
		p.cursor = len(p.entries) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *filesPane) ensureVisible(height int) {
	if height <= 0 || len(p.entries) == 0 {
		p.offset = 0
		return
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+height {
		p.offset = p.cursor - height + 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
}

func (p *filesPane) connected() bool {
	return p.kind == filesPaneRemote && p.session != nil
}

func (p *filesPane) pickingHost() bool {
	return p.kind == filesPaneRemote && p.session == nil
}

func (p *filesPane) closeSession() {
	if p.session != nil {
		_ = p.session.Close()
		p.session = nil
	}
	p.alias = ""
	p.connecting = false
	p.cwd = ""
	p.entries = nil
	p.cursor = 0
	p.err = ""
	p.clearMarks()
}

func (m *App) filesFocusedPane() *filesPane {
	return &m.files.panes[m.files.focus]
}

func (m *App) filesOtherPane() *filesPane {
	return &m.files.panes[1-m.files.focus]
}

func (m *App) filesEndpoint(pane *filesPane) files.Endpoint {
	if pane.kind == filesPaneLocal {
		return files.Endpoint{}
	}
	return files.Endpoint{Session: pane.session}
}

func (m *App) renderFiles(s styleSet) string {
	m.initFilesState()
	layout := m.panelLayout()
	left := m.renderFilesSide(s, 0, layout.listHeight, layout.listWidth)
	right := m.renderFilesSide(s, 1, layout.detailHeight, layout.detailWidth)
	if layout.mobile {
		rule := s.rule.Render(strings.Repeat("─", layout.listWidth))
		return left + "\n" + rule + "\n" + right
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(layout.listWidth).Height(layout.listHeight).Render(left),
		lipgloss.NewStyle().Width(layout.detailWidth).Height(layout.detailHeight).Render(right),
	)
}

func (m *App) renderFilesSide(s styleSet, index, height, width int) string {
	pane := &m.files.panes[index]
	focused := m.files.focus == index
	if pane.pickingHost() {
		return m.renderFilesHostPicker(s, pane, focused, height, width)
	}
	return m.renderFilesBrowser(s, pane, focused, height-1, width)
}

func filesPaneTone(s styleSet, pane *filesPane, focused bool) lipgloss.Style {
	color := "#94A3B8" // slate local
	switch {
	case pane.connected():
		color = "#14B8A6" // teal connected
	case pane.kind == filesPaneRemote:
		color = "#D97706" // amber host picker
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	if focused {
		style = style.Bold(true)
	}
	return style
}

func (m *App) renderFilesBrowser(s styleSet, pane *filesPane, focused bool, listHeight, width int) string {
	var b strings.Builder
	tone := filesPaneTone(s, pane, focused)
	label := "local"
	if pane.kind == filesPaneRemote {
		label = pane.alias
	}
	if pane.pathEdit {
		b.WriteString("  " + tone.Render(label) + "  " + pane.pathInput.View() + "\n")
	} else {
		header := label
		if pane.cwd != "" {
			header = label + "  " + truncate(pane.cwd, max(8, width-len(label)-4))
		}
		if focused && m.files.jump.active {
			header += "  f›" + m.files.jump.query
			if m.files.jump.query == "" {
				header += "█"
			} else {
				header += "█"
			}
		}
		marker := " "
		if focused {
			marker = "›"
		}
		b.WriteString(marker + " " + tone.Render(header) + "\n")
	}
	if pane.err != "" {
		b.WriteString("  " + s.error.Render(pane.err) + "\n")
	} else if pane.loading || pane.connecting {
		b.WriteString("  " + s.muted.Render("Loading…") + "\n")
	} else if len(pane.entries) == 0 {
		b.WriteString("  " + s.muted.Render("Empty") + "\n")
	} else {
		jumping := focused && m.files.jump.active
		pane.ensureVisible(max(1, listHeight-1))
		end := min(len(pane.entries), pane.offset+max(1, listHeight-1))
		for i := pane.offset; i < end; i++ {
			entry := pane.entries[i]
			match := !jumping || filesFuzzyMatch(entry.Name, m.files.jump.query)
			prefix := "  "
			if jumping {
				if lab, ok := m.files.jump.labelFor(i); ok && match {
					prefix = lab + " "
				} else {
					prefix = "  "
				}
			} else if _, marked := pane.marked[entry.Path]; marked {
				prefix = " •"
			}
			name := entry.Name
			if entry.IsDir {
				name += "/"
			}
			line := prefix + " " + name
			switch {
			case jumping && !match:
				b.WriteString(s.muted.Width(width).Render(truncate(line, width)) + "\n")
			case jumping && match:
				b.WriteString(s.value.Bold(true).Width(width).Render(truncate(line, width)) + "\n")
			case focused && i == pane.cursor:
				b.WriteString(s.selected.Width(width).Render(truncate(line, width)) + "\n")
			default:
				b.WriteString(s.value.Width(width).Render(truncate(line, width)) + "\n")
			}
		}
	}
	return b.String()
}

func (j filesJump) labelFor(index int) (string, bool) {
	for lab, idx := range j.labels {
		if idx == index {
			return lab, true
		}
	}
	return "", false
}

func (m *App) renderFilesHostPicker(s styleSet, pane *filesPane, focused bool, height, width int) string {
	var b strings.Builder
	tone := filesPaneTone(s, pane, focused)
	marker := " "
	if focused {
		marker = "›"
	}
	title := "remote"
	if strings.HasPrefix(pane.hostSearch, "\x00") || pane.hostSearch != "" {
		q := strings.TrimPrefix(pane.hostSearch, "\x00")
		title = "remote  /" + q
		if strings.HasPrefix(pane.hostSearch, "\x00") {
			title += "█"
		}
	}
	b.WriteString(marker + " " + tone.Render(title) + "\n")
	if pane.connecting {
		b.WriteString("  " + s.muted.Render("Connecting…") + "\n")
		return b.String()
	}
	hosts := m.filesHostList(pane)
	if len(hosts) == 0 {
		b.WriteString("  " + s.muted.Render("No hosts") + "\n")
		return b.String()
	}
	if pane.hostCursor >= len(hosts) {
		pane.hostCursor = len(hosts) - 1
	}
	listHeight := max(1, height-1)
	offset := 0
	if pane.hostCursor >= listHeight {
		offset = pane.hostCursor - listHeight + 1
	}
	end := min(len(hosts), offset+listHeight)
	for i := offset; i < end; i++ {
		line := "   " + m.hostLabel(hosts[i])
		if focused && i == pane.hostCursor {
			b.WriteString(s.selected.Width(width).Render(truncate(line, width)) + "\n")
		} else {
			b.WriteString(s.value.Width(width).Render(truncate(line, width)) + "\n")
		}
	}
	return b.String()
}

func (m *App) filesHostList(pane *filesPane) []sshconfig.Host {
	q := strings.ToLower(strings.TrimPrefix(pane.hostSearch, "\x00"))
	out := make([]sshconfig.Host, 0, len(m.hosts))
	meta := m.hostMetadata()
	for _, host := range m.hosts {
		if meta[host.Alias].Hidden && !m.showHidden {
			continue
		}
		if q != "" {
			label := strings.ToLower(m.hostLabel(host))
			if !strings.Contains(label, q) && !strings.Contains(strings.ToLower(host.Alias), q) {
				continue
			}
		}
		out = append(out, host)
	}
	return out
}

func (m *App) updateFilesKeys(key string) (tea.Model, tea.Cmd) {
	m.initFilesState()
	if m.files.transfer.active {
		switch key {
		case "esc", "x":
			if m.files.transfer.cancel != nil {
				m.files.transfer.cancel()
			}
			return m, m.setNotice("Cancelling transfer…")
		}
		return m, nil
	}
	pane := m.filesFocusedPane()
	if m.files.jump.active {
		return m.updateFilesJump(key)
	}
	if pane.pathEdit {
		return m.updateFilesPathEdit(key, pane)
	}
	if pane.pickingHost() && strings.HasPrefix(pane.hostSearch, "\x00") {
		return m.updateFilesHostSearch(key, pane)
	}

	switch key {
	case "tab":
		m.files.focus = 1 - m.files.focus
		return m, nil
	case "w":
		m.files.panes[0], m.files.panes[1] = m.files.panes[1], m.files.panes[0]
		m.files.focus = 1 - m.files.focus
		return m, nil
	case "L":
		return m, m.setFilesPaneLocal(m.files.focus)
	case "R":
		return m, m.setFilesPaneRemote(m.files.focus)
	case "f", "s":
		return m.beginFilesJump()
	case "esc":
		if len(pane.marked) > 0 || pane.rangeAnchor != nil {
			pane.clearMarks()
			return m, m.setNotice("Cleared selection")
		}
		if pane.connected() {
			return m, m.disconnectFilesPane(m.files.focus)
		}
		return m, tea.Quit
	case "up", "k":
		return m.moveFilesCursor(-1)
	case "down", "j":
		return m.moveFilesCursor(1)
	case "home", "g":
		return m.moveFilesCursorHome()
	case "end", "G":
		return m.moveFilesCursorEnd()
	case "enter", "l":
		return m.activateFilesSelection()
	case "backspace", "h", "ctrl+h":
		if pane.pickingHost() {
			return m, nil
		}
		return m.filesParent()
	case "space":
		return m.toggleFilesMark()
	case "v":
		return m.toggleFilesRange()
	case ".":
		if pane.pickingHost() {
			return m, nil
		}
		pane.showHidden = !pane.showHidden
		return m, m.refreshFilesPane(m.files.focus)
	case "/":
		if pane.pickingHost() {
			pane.hostSearch = "\x00"
			pane.hostCursor = 0
			return m, nil
		}
		return m.beginFilesPathEdit()
	case "a":
		return m.openFilesMkdirForm()
	case "r":
		return m.openFilesRenameForm()
	case "d":
		return m.openFilesDeleteForm()
	case "c":
		return m.startFilesTransfer(false)
	case "m":
		return m.startFilesTransfer(true)
	case "t":
		return m.filesOpenShell()
	case "D":
		if pane.kind == filesPaneRemote {
			return m, m.disconnectFilesPane(m.files.focus)
		}
	}
	return m, nil
}

func (m *App) filesTyping() bool {
	if m.section != filesSection || !m.files.ready {
		return false
	}
	if m.files.jump.active {
		return true
	}
	pane := m.filesFocusedPane()
	if pane.pathEdit {
		return true
	}
	if pane.pickingHost() && strings.HasPrefix(pane.hostSearch, "\x00") {
		return true
	}
	return false
}

func (m *App) updateFilesPathEdit(key string, pane *filesPane) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		pane.pathEdit = false
		pane.pathInput.Blur()
		return m, nil
	case "enter":
		value := strings.TrimSpace(pane.pathInput.Value())
		pane.pathEdit = false
		pane.pathInput.Blur()
		if value == "" {
			return m, nil
		}
		if pane.kind == filesPaneLocal {
			cleaned, err := files.CleanLocal(value)
			if err != nil {
				pane.err = err.Error()
				return m, nil
			}
			pane.cwd = cleaned
		} else {
			cleaned, err := files.CleanRemote(value)
			if err != nil {
				pane.err = err.Error()
				return m, nil
			}
			pane.cwd = cleaned
		}
		pane.clearMarks()
		return m, m.refreshFilesPane(m.files.focus)
	default:
		return m.updateFilesPathInputKey(pane, key)
	}
}

func (m *App) updateFilesPathInputKey(pane *filesPane, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "backspace", "ctrl+h":
		val := pane.pathInput.Value()
		if len(val) > 0 {
			runes := []rune(val)
			pane.pathInput.SetValue(string(runes[:len(runes)-1]))
			pane.pathInput.SetCursor(len(runes) - 1)
		}
	case "ctrl+u":
		pane.pathInput.SetValue("")
		pane.pathInput.SetCursor(0)
	default:
		if len([]rune(key)) == 1 {
			pane.pathInput.SetValue(pane.pathInput.Value() + key)
			pane.pathInput.SetCursor(len([]rune(pane.pathInput.Value())))
		}
	}
	return m, nil
}

func (m *App) updateFilesHostSearch(key string, pane *filesPane) (tea.Model, tea.Cmd) {
	query := strings.TrimPrefix(pane.hostSearch, "\x00")
	switch key {
	case "enter":
		pane.hostSearch = query
		return m.activateFilesSelection()
	case "esc":
		pane.hostSearch = ""
	case "backspace", "ctrl+h":
		if len(query) > 0 {
			query = query[:len(query)-1]
		}
		pane.hostSearch = "\x00" + query
		pane.hostCursor = 0
	default:
		if len([]rune(key)) == 1 {
			pane.hostSearch = "\x00" + query + key
			pane.hostCursor = 0
		}
	}
	return m, nil
}

func (m *App) beginFilesPathEdit() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	pane.pathEdit = true
	pane.pathInput.SetWidth(max(20, m.terminalWidth()/2-4))
	pane.pathInput.SetValue(pane.cwd)
	pane.pathInput.SetCursor(len([]rune(pane.cwd)))
	pane.pathInput.Focus()
	return m, nil
}

func (m *App) moveFilesCursor(delta int) (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		hosts := m.filesHostList(pane)
		if len(hosts) == 0 {
			return m, nil
		}
		pane.hostCursor += delta
		if pane.hostCursor < 0 {
			pane.hostCursor = 0
		}
		if pane.hostCursor >= len(hosts) {
			pane.hostCursor = len(hosts) - 1
		}
		return m, nil
	}
	pane.cursor += delta
	pane.clamp()
	return m, nil
}

func (m *App) moveFilesCursorHome() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		pane.hostCursor = 0
		return m, nil
	}
	pane.cursor = 0
	pane.clamp()
	return m, nil
}

func (m *App) moveFilesCursorEnd() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		hosts := m.filesHostList(pane)
		if len(hosts) > 0 {
			pane.hostCursor = len(hosts) - 1
		}
		return m, nil
	}
	if len(pane.entries) > 0 {
		pane.cursor = len(pane.entries) - 1
	}
	pane.clamp()
	return m, nil
}

func (m *App) activateFilesSelection() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		hosts := m.filesHostList(pane)
		if pane.hostCursor < 0 || pane.hostCursor >= len(hosts) {
			return m, nil
		}
		return m, m.connectFilesHost(m.files.focus, hosts[pane.hostCursor])
	}
	entry, ok := pane.selectedEntry()
	if !ok || !entry.IsDir {
		return m, nil
	}
	pane.cwd = entry.Path
	pane.cursor = 0
	pane.clearMarks()
	return m, m.refreshFilesPane(m.files.focus)
}

func (m *App) filesParent() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	var parent string
	var err error
	if pane.kind == filesPaneLocal {
		parent, err = files.ParentLocal(pane.cwd)
	} else {
		parent, err = files.ParentRemote(pane.cwd)
	}
	if err != nil || parent == pane.cwd {
		return m, nil
	}
	pane.cwd = parent
	pane.cursor = 0
	pane.clearMarks()
	return m, m.refreshFilesPane(m.files.focus)
}

func (m *App) toggleFilesMark() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	entry, ok := pane.selectedEntry()
	if !ok {
		return m, nil
	}
	if _, exists := pane.marked[entry.Path]; exists {
		delete(pane.marked, entry.Path)
	} else {
		pane.marked[entry.Path] = struct{}{}
	}
	if pane.cursor+1 < len(pane.entries) {
		pane.cursor++
	}
	return m, nil
}

func (m *App) toggleFilesRange() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() || len(pane.entries) == 0 {
		return m, nil
	}
	if pane.rangeAnchor == nil {
		anchor := pane.cursor
		pane.rangeAnchor = &anchor
		pane.marked[pane.entries[pane.cursor].Path] = struct{}{}
		return m, m.setNotice("Range start")
	}
	start, end := *pane.rangeAnchor, pane.cursor
	if start > end {
		start, end = end, start
	}
	for i := start; i <= end && i < len(pane.entries); i++ {
		pane.marked[pane.entries[i].Path] = struct{}{}
	}
	pane.rangeAnchor = nil
	return m, m.setNotice(fmt.Sprintf("Marked %d", end-start+1))
}

func (m *App) updateFilesMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		return m.moveFilesCursor(-1)
	case tea.MouseWheelDown:
		return m.moveFilesCursor(1)
	}
	return m, nil
}

func (m *App) filesFooterHint() string {
	if m.files.transfer.active {
		if m.files.transfer.move {
			return "moving… esc"
		}
		return "copying… esc"
	}
	if m.files.jump.active {
		return "type · label jumps · esc"
	}
	return "?"
}

var filesJumpLabels = []string{
	"a", "s", "d", "g", "h", "j", "k", "w", "e", "r", "t", "y", "u", "i", "o", "p", "z", "x", "c", "v", "b", "n", "m",
}

func filesFuzzyMatch(name, query string) bool {
	if query == "" {
		return false
	}
	name = strings.ToLower(name)
	query = strings.ToLower(query)
	ni := 0
	for qi := 0; qi < len(query); qi++ {
		found := false
		for ; ni < len(name); ni++ {
			if name[ni] == query[qi] {
				ni++
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// filesFuzzyContinuations returns keys that could still extend query for name.
func filesFuzzyContinuations(name, query string) map[string]bool {
	out := map[string]bool{}
	lower := strings.ToLower(name)
	seen := map[byte]bool{}
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if c < 'a' || c > 'z' || seen[c] {
			continue
		}
		seen[c] = true
		ch := string(c)
		if filesFuzzyMatch(name, query+ch) {
			out[ch] = true
		}
	}
	return out
}

func (m *App) filesJumpMatches() []int {
	pane := m.filesFocusedPane()
	out := make([]int, 0)
	for i, entry := range pane.entries {
		if filesFuzzyMatch(entry.Name, m.files.jump.query) {
			out = append(out, i)
		}
	}
	return out
}

func (m *App) beginFilesJump() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() || len(pane.entries) == 0 {
		return m, nil
	}
	m.files.jump = filesJump{active: true, query: "", labels: map[string]int{}}
	return m, nil
}

func (m *App) clearFilesJump() {
	m.files.jump = filesJump{}
}

func (m *App) rebuildFilesJumpLabels() {
	pane := m.filesFocusedPane()
	m.files.jump.labels = map[string]int{}
	if m.files.jump.query == "" {
		return
	}
	matches := m.filesJumpMatches()
	reserved := map[string]bool{}
	for _, index := range matches {
		for ch := range filesFuzzyContinuations(pane.entries[index].Name, m.files.jump.query) {
			reserved[ch] = true
		}
	}
	labelIdx := 0
	for _, index := range matches {
		for labelIdx < len(filesJumpLabels) && reserved[filesJumpLabels[labelIdx]] {
			labelIdx++
		}
		if labelIdx >= len(filesJumpLabels) {
			break
		}
		m.files.jump.labels[filesJumpLabels[labelIdx]] = index
		labelIdx++
	}
}

// applyFilesJumpQuery rebuilds labels and auto-jumps when only one match remains.
func (m *App) applyFilesJumpQuery() (jumped bool) {
	pane := m.filesFocusedPane()
	matches := m.filesJumpMatches()
	if len(matches) == 1 {
		pane.cursor = matches[0]
		pane.clamp()
		m.clearFilesJump()
		return true
	}
	m.rebuildFilesJumpLabels()
	if len(matches) > 0 {
		pane.cursor = matches[0]
		pane.clamp()
	}
	return false
}

func (m *App) updateFilesJump(key string) (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	switch key {
	case "esc":
		m.clearFilesJump()
		return m, nil
	case "enter":
		matches := m.filesJumpMatches()
		if len(matches) > 0 {
			pane.cursor = matches[0]
			pane.clamp()
			m.clearFilesJump()
		}
		return m, nil
	case "backspace", "ctrl+h":
		if m.files.jump.query == "" {
			m.clearFilesJump()
			return m, nil
		}
		runes := []rune(m.files.jump.query)
		m.files.jump.query = string(runes[:len(runes)-1])
		m.applyFilesJumpQuery()
		return m, nil
	}
	if len([]rune(key)) != 1 {
		return m, nil
	}

	// Shown labels always jump. Labels never use keys that could continue the query.
	if index, ok := m.files.jump.labels[key]; ok {
		pane.cursor = index
		pane.clamp()
		m.clearFilesJump()
		return m, nil
	}

	extended := m.files.jump.query + key
	hasMatch := false
	for _, entry := range pane.entries {
		if filesFuzzyMatch(entry.Name, extended) {
			hasMatch = true
			break
		}
	}
	if !hasMatch {
		return m, nil
	}
	m.files.jump.query = extended
	m.applyFilesJumpQuery()
	return m, nil
}
