package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/files"
)

const (
	filesPreviewLocalTimeout  = 8 * time.Second
	filesPreviewRemoteTimeout = 20 * time.Second
)

type filesPreview struct {
	active  bool
	loading bool
	err     string
	pane    int
	path    string
	gen     uint64
	offset  int
	result  files.Preview
	cancel  context.CancelFunc
}

type filesPreviewMsg struct {
	gen     uint64
	preview files.Preview
	err     error
}

func (m *App) openFilesPreview() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	entry, ok := pane.selectedEntry()
	if !ok {
		return m, m.setNotice("Nothing selected")
	}
	if entry.IsDir {
		return m, m.setNotice("Not a file")
	}
	if m.files.preview.active && m.files.preview.path == entry.Path {
		m.closeFilesPreview()
		return m, nil
	}

	m.closeFilesInfo()
	if m.files.preview.cancel != nil {
		m.files.preview.cancel()
	}
	m.files.preview.gen++
	gen := m.files.preview.gen
	timeout := filesPreviewLocalTimeout
	if pane.kind == filesPaneRemote {
		timeout = filesPreviewRemoteTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	m.files.preview = filesPreview{
		active:  true,
		loading: true,
		pane:    m.files.focus,
		path:    entry.Path,
		gen:     gen,
		cancel:  cancel,
	}
	ep := m.filesEndpoint(pane)
	name := entry.Name
	path := entry.Path
	size := entry.Size
	return m, func() tea.Msg {
		defer cancel()
		preview, err := files.ReadPreview(ctx, ep, path, size, name)
		return filesPreviewMsg{gen: gen, preview: preview, err: err}
	}
}

func (m *App) closeFilesPreview() {
	if m.files.preview.cancel != nil {
		m.files.preview.cancel()
	}
	m.files.preview = filesPreview{}
}

func (m *App) handleFilesPreviewMsg(msg filesPreviewMsg) tea.Cmd {
	if !m.files.preview.active || msg.gen != m.files.preview.gen {
		return nil
	}
	m.files.preview.loading = false
	m.files.preview.offset = 0
	if msg.err != nil {
		m.files.preview.err = msg.err.Error()
		m.files.preview.result = files.Preview{}
		return nil
	}
	m.files.preview.err = ""
	m.files.preview.result = msg.preview
	return nil
}

func (m *App) updateFilesPreview(key string) (tea.Model, tea.Cmd) {
	if !m.files.preview.active {
		return m, nil
	}
	page := m.filesPreviewPage()
	switch key {
	case "o", "esc", "q":
		m.closeFilesPreview()
		return m, nil
	case "i":
		m.closeFilesPreview()
		return m.openFilesInfo()
	case "p":
		m.closeFilesPreview()
		return m.openFilesChmodMenu()
	case "up", "k":
		m.scrollFilesPreview(-1)
	case "down", "j":
		m.scrollFilesPreview(1)
	case "pgup":
		m.scrollFilesPreview(-page)
	case "pgdown":
		m.scrollFilesPreview(page)
	case "g", "home":
		m.files.preview.offset = 0
	case "G", "end":
		m.files.preview.offset = m.maxFilesPreviewOffset()
	case "[":
		return m.stepFilesPreview(-1)
	case "]":
		return m.stepFilesPreview(1)
	}
	return m, nil
}

func (m *App) stepFilesPreview(delta int) (tea.Model, tea.Cmd) {
	paneIndex := m.files.preview.pane
	if paneIndex < 0 || paneIndex > 1 {
		paneIndex = m.files.focus
	}
	pane := &m.files.panes[paneIndex]
	start := pane.cursor
	for i := 1; i < len(pane.entries); i++ {
		idx := start + delta*i
		if idx < 0 || idx >= len(pane.entries) {
			return m, nil
		}
		if pane.entries[idx].IsDir {
			continue
		}
		pane.cursor = idx
		pane.clamp()
		m.files.focus = paneIndex
		return m.openFilesPreview()
	}
	return m, nil
}

func (m *App) scrollFilesPreview(delta int) {
	m.files.preview.offset += delta
	m.clampFilesPreviewOffset()
}

func (m *App) clampFilesPreviewOffset() {
	if m.files.preview.offset < 0 {
		m.files.preview.offset = 0
	}
	if maxOffset := m.maxFilesPreviewOffset(); m.files.preview.offset > maxOffset {
		m.files.preview.offset = maxOffset
	}
}

func (m *App) maxFilesPreviewOffset() int {
	body := m.filesPreviewBodyLines(m.filesPreviewContentWidth())
	return max(0, len(body)-m.filesPreviewPage())
}

func (m *App) filesPreviewPage() int {
	return max(1, m.filesPreviewInnerHeight()-2)
}

func (m *App) filesPreviewContentWidth() int {
	termW := m.terminalWidth()
	if m.isMobileLayout() {
		return max(16, termW-8)
	}
	return max(16, min(80, termW-10))
}

func (m *App) filesPreviewInnerHeight() int {
	bodyH := max(1, m.terminalHeight()-3)
	if m.isMobileLayout() {
		return max(6, bodyH-4)
	}
	return max(8, bodyH-6)
}

func (m *App) renderFilesPreview(s styleSet) string {
	innerW := m.filesPreviewContentWidth()
	title := m.filesPreviewTitle(innerW)
	body := m.filesPreviewBodyLines(innerW)
	viewH := m.filesPreviewPage()
	m.clampFilesPreviewOffset()
	offset := m.files.preview.offset
	end := min(len(body), offset+viewH)
	var b strings.Builder
	b.WriteString(s.active.Render(title))
	visible := body[offset:end]
	if len(visible) == 0 {
		b.WriteString("\n")
	}
	for i, line := range visible {
		b.WriteString("\n")
		if m.files.preview.err != "" && offset+i == 0 {
			b.WriteString(s.error.Width(innerW).Render(line))
			continue
		}
		if m.files.preview.loading || m.files.preview.result.Text == "" {
			b.WriteString(s.muted.Width(innerW).Render(line))
			continue
		}
		b.WriteString(s.value.Width(innerW).Render(line))
	}
	panel := lipgloss.NewStyle().
		Width(innerW).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6B7280")).
		Render(b.String())
	return lipgloss.Place(m.terminalWidth(), max(1, m.terminalHeight()-3), lipgloss.Center, lipgloss.Center, panel)
}

func (m *App) filesPreviewTitle(width int) string {
	p := m.files.preview
	if p.loading {
		name := files.BaseName(p.path)
		if name == "" {
			name = "file"
		}
		return truncate(name, width)
	}
	if p.err != "" {
		name := files.BaseName(p.path)
		return truncate(name, width)
	}
	res := p.result
	name := res.Name
	if name == "" {
		name = files.BaseName(p.path)
	}
	parts := []string{name, string(res.Kind), files.FormatSize(res.Size)}
	if res.Truncated {
		parts = append(parts, "truncated")
	}
	return truncate(strings.Join(parts, " · "), width)
}

func (m *App) filesPreviewBodyLines(width int) []string {
	p := m.files.preview
	if p.loading {
		return []string{"Loading…"}
	}
	if p.err != "" {
		return wrapPreviewLines(p.err, width)
	}
	if p.result.Text != "" {
		return wrapPreviewLines(p.result.Text, width)
	}
	if p.result.Reason != "" {
		return wrapPreviewLines(p.result.Reason, width)
	}
	return nil
}

func wrapPreviewLines(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	maxSource := width * 2
	var out []string
	for _, line := range strings.Split(text, "\n") {
		runes := []rune(line)
		if len(runes) > maxSource {
			runes = append(runes[:maxSource-1], '…')
		}
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		for len(runes) > width {
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		out = append(out, string(runes))
	}
	return out
}
