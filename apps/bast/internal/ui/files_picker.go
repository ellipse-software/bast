package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud"
)

type filesPickerRow struct {
	local bool
	row   hostRow
}

func (m *App) filesPickerQuery(pane *filesPane) string {
	return strings.TrimSpace(strings.TrimPrefix(pane.hostSearch, "\x00"))
}

func (m *App) filesPickerRows(pane *filesPane) []filesPickerRow {
	built := m.hostRowsForQuery(m.filesPickerQuery(pane))
	rows := make([]filesPickerRow, 0, 1+len(built))
	rows = append(rows, filesPickerRow{local: true})
	for _, row := range built {
		rows = append(rows, filesPickerRow{row: row})
	}
	return rows
}

func (m *App) filesPickerRowAt(pane *filesPane) (filesPickerRow, bool) {
	rows := m.filesPickerRows(pane)
	if pane.hostCursor < 0 || pane.hostCursor >= len(rows) {
		return filesPickerRow{}, false
	}
	return rows[pane.hostCursor], true
}

func (m *App) renderFilesPickerLine(s styleSet, row filesPickerRow, selected bool, width int, query string, syncErrors map[cloud.Kind]bool) string {
	if row.local {
		line := "   Local"
		if selected {
			return s.selected.Width(width).Render(line)
		}
		return s.plain.Width(width).Render(line)
	}
	return m.renderHostRowLine(s, row.row, selected, width, query, syncErrors)
}

func (m *App) activateFilesPicker() (tea.Model, tea.Cmd) {
	return m.openFilesPickerRow(true)
}

func (m *App) expandFilesPickerRow() (tea.Model, tea.Cmd) {
	return m.openFilesPickerRow(false)
}

func (m *App) openFilesPickerRow(toggleHeader bool) (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	row, ok := m.filesPickerRowAt(pane)
	if !ok {
		return m, nil
	}
	if row.local {
		return m, m.setFilesPaneLocal(m.files.focus)
	}
	if row.row.header {
		collapsed := m.collapsedGroups != nil && m.collapsedGroups[row.row.group]
		if !collapsed && !toggleHeader {
			return m, nil
		}
		_, cmd := m.applyGroupCollapsed(row.row.group, !collapsed, m.filesPickerQuery(pane))
		return m, cmd
	}
	return m, m.connectFilesHost(m.files.focus, row.row.host)
}

func (m *App) filesPickerLeft() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	row, ok := m.filesPickerRowAt(pane)
	if !ok || row.local {
		return m, nil
	}
	query := m.filesPickerQuery(pane)
	if row.row.header {
		if m.collapsedGroups == nil || !m.collapsedGroups[row.row.group] {
			_, cmd := m.applyGroupCollapsed(row.row.group, true, query)
			return m, cmd
		}
		if parent, ok := parentGroupPath(row.row.group); ok {
			m.focusFilesPickerGroup(pane, parent)
		}
		return m, nil
	}
	if row.row.group != "" {
		m.focusFilesPickerGroup(pane, row.row.group)
	}
	return m, nil
}

func (m *App) filesPickerToggle() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	row, ok := m.filesPickerRowAt(pane)
	if !ok || row.local || !row.row.header {
		return m, nil
	}
	query := m.filesPickerQuery(pane)
	if query != "" {
		return m, m.setNotice("Clear the search filter to collapse groups")
	}
	collapsed := m.collapsedGroups != nil && m.collapsedGroups[row.row.group]
	_, cmd := m.applyGroupCollapsed(row.row.group, !collapsed, "")
	return m, cmd
}

func (m *App) focusFilesPickerGroup(pane *filesPane, group string) {
	for i, row := range m.filesPickerRows(pane) {
		if !row.local && row.row.header && row.row.group == group {
			pane.hostCursor = i
			return
		}
	}
}
