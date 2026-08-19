package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/files"
	"bast/internal/platform"
)

func (m *App) openFilesInfo() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	if _, ok := pane.selectedEntry(); !ok {
		return m, m.setNotice("Nothing selected")
	}
	m.files.info = true
	return m, nil
}

func (m *App) closeFilesInfo() {
	m.files.info = false
}

// clearFilesOverlays dismisses chmod/info when leaving the Files section
// so they do not stick over Hosts, Keys, or Sync.
func (m *App) clearFilesOverlays() {
	m.closeFilesChmod()
	m.closeFilesInfo()
}

func (m *App) updateFilesInfo(key string) (tea.Model, tea.Cmd) {
	if !m.files.info {
		return m, nil
	}
	switch key {
	case "esc", "i", "q":
		m.closeFilesInfo()
		return m, nil
	case "tab":
		m.files.focus = 1 - m.files.focus
		return m, nil
	case "up", "k":
		return m.moveFilesCursor(-1)
	case "down", "j":
		return m.moveFilesCursor(1)
	case "home", "g":
		return m.moveFilesCursorHome()
	case "end", "G":
		return m.moveFilesCursorEnd()
	case "p":
		pane := m.filesFocusedPane()
		if pane.kind == filesPaneLocal && !platform.SupportsPOSIXPermissions() {
			return m, nil
		}
		m.closeFilesInfo()
		return m.openFilesChmodMenu()
	}
	return m, nil
}

func (m *App) renderFilesInfo(s styleSet, pane *filesPane, width int) string {
	entry, ok := pane.selectedEntry()
	if !ok {
		return "  " + s.muted.Render("Nothing selected") + "\n"
	}

	kind := files.EntryKind(entry)
	mode := files.FormatModeOctal(entry.Mode) + "  " + files.FormatModeSymbolic(entry.Mode)
	size := files.FormatSize(entry.Size)
	if entry.IsDir {
		size = "-"
	}
	modified := "-"
	if !entry.ModTime.IsZero() {
		modified = entry.ModTime.Local().Format("2006-01-02 15:04")
	}

	type infoRow struct {
		label string
		value string
	}
	rows := []infoRow{
		{"Name", entry.Name},
		{"Type", kind},
		{"Size", size},
	}
	if pane.kind == filesPaneRemote || platform.SupportsPOSIXPermissions() {
		rows = append(rows, infoRow{"Mode", mode})
	}
	rows = append(rows, infoRow{"Modified", modified})

	var b strings.Builder
	labelWidth := 10
	for _, row := range rows {
		label := s.muted.Render(row.label + strings.Repeat(" ", max(1, labelWidth-len(row.label))))
		value := s.value.Render(truncate(row.value, max(8, width-labelWidth-4)))
		b.WriteString("  " + label + value + "\n")
	}
	return b.String()
}
