package ui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/files"
)

// filesChmod is an inline permissions editor for the focused Files pane.
// Cursor indexes a 3x3 rwx grid (0-8), optional recursive row (9), then octal (10).
type filesChmod struct {
	active    bool
	pane      int
	paths     []string
	title     string
	mode      os.FileMode
	cursor    int
	recursive bool
	hasDir    bool
	octalEdit bool
	octal     string
}

const (
	chmodBitCount      = 9
	chmodRecursiveSlot = 9
	chmodOctalSlot     = 10
)

var chmodBitMasks = [9]os.FileMode{
	0400, 0200, 0100,
	0040, 0020, 0010,
	0004, 0002, 0001,
}

func (m *App) openFilesChmodMenu() (tea.Model, tea.Cmd) {
	pane := m.filesFocusedPane()
	if pane.pickingHost() {
		return m, nil
	}
	paths := pane.selectedPaths()
	if len(paths) == 0 {
		return m, m.setNotice("Nothing selected")
	}

	byPath := make(map[string]files.Entry, len(pane.entries))
	for _, entry := range pane.entries {
		byPath[entry.Path] = entry
	}

	mode := os.FileMode(0o644)
	hasDir := false
	if entry, ok := pane.selectedEntry(); ok {
		if len(pane.marked) == 0 {
			mode = files.NormalizeMode(entry.Mode)
		} else if _, marked := pane.marked[entry.Path]; marked {
			mode = files.NormalizeMode(entry.Mode)
		} else if first, ok := byPath[paths[0]]; ok {
			mode = files.NormalizeMode(first.Mode)
		}
	} else if first, ok := byPath[paths[0]]; ok {
		mode = files.NormalizeMode(first.Mode)
	}
	for _, path := range paths {
		if entry, ok := byPath[path]; ok && entry.IsDir {
			hasDir = true
			break
		}
	}

	title := "Permissions"
	switch len(paths) {
	case 1:
		title = "Permissions: " + files.BaseName(paths[0])
	default:
		title = fmt.Sprintf("Permissions: %d items", len(paths))
	}

	m.files.info = false
	m.files.chmod = filesChmod{
		active:    true,
		pane:      m.files.focus,
		paths:     append([]string(nil), paths...),
		title:     title,
		mode:      mode,
		cursor:    0,
		recursive: false,
		hasDir:    hasDir,
		octal:     files.FormatModeOctal(mode),
	}
	return m, nil
}

func (m *App) closeFilesChmod() {
	m.files.chmod = filesChmod{}
}

func (c filesChmod) maxCursor() int {
	return chmodOctalSlot
}

func (c *filesChmod) setMode(mode os.FileMode) {
	c.mode = files.NormalizeMode(mode)
	if !c.octalEdit {
		c.octal = files.FormatModeOctal(c.mode)
	}
}

func (c *filesChmod) toggleBit(index int) {
	if index < 0 || index >= chmodBitCount {
		return
	}
	bit := chmodBitMasks[index]
	if c.mode&bit != 0 {
		c.setMode(c.mode &^ bit)
	} else {
		c.setMode(c.mode | bit)
	}
}

func (c *filesChmod) bitOn(index int) bool {
	if index < 0 || index >= chmodBitCount {
		return false
	}
	return c.mode&chmodBitMasks[index] != 0
}

func (m *App) updateFilesChmod(key string) (tea.Model, tea.Cmd) {
	c := &m.files.chmod
	if !c.active {
		return m, nil
	}
	if c.octalEdit {
		return m.updateFilesChmodOctal(key)
	}
	switch key {
	case "esc", "q":
		m.closeFilesChmod()
		return m, nil
	case "enter":
		return m.applyFilesChmod()
	case "space", "x":
		if c.cursor < chmodBitCount {
			c.toggleBit(c.cursor)
		} else if c.cursor == chmodRecursiveSlot && c.hasDir {
			c.recursive = !c.recursive
		} else if c.cursor == chmodOctalSlot {
			c.octalEdit = true
			c.octal = files.FormatModeOctal(c.mode)
		}
		return m, nil
	case "left", "h":
		if c.cursor < chmodBitCount {
			col := c.cursor % 3
			if col > 0 {
				c.cursor--
			}
		}
		return m, nil
	case "right", "l":
		if c.cursor < chmodBitCount {
			col := c.cursor % 3
			if col < 2 {
				c.cursor++
			}
		}
		return m, nil
	case "up", "k":
		switch {
		case c.cursor < chmodBitCount:
			row := c.cursor / 3
			if row > 0 {
				c.cursor -= 3
			}
		case c.cursor == chmodRecursiveSlot:
			c.cursor = 6 // other row
		case c.cursor == chmodOctalSlot:
			if c.hasDir {
				c.cursor = chmodRecursiveSlot
			} else {
				c.cursor = 6
			}
		}
		return m, nil
	case "down", "j":
		switch {
		case c.cursor < chmodBitCount:
			row := c.cursor / 3
			if row < 2 {
				c.cursor += 3
			} else if c.hasDir {
				c.cursor = chmodRecursiveSlot
			} else {
				c.cursor = chmodOctalSlot
			}
		case c.cursor == chmodRecursiveSlot:
			c.cursor = chmodOctalSlot
		}
		return m, nil
	case "tab":
		c.cursor++
		if c.cursor > c.maxCursor() {
			c.cursor = 0
		}
		// Skip recursive slot when unused.
		if c.cursor == chmodRecursiveSlot && !c.hasDir {
			c.cursor = chmodOctalSlot
		}
		return m, nil
	case "shift+tab":
		c.cursor--
		if c.cursor < 0 {
			c.cursor = c.maxCursor()
		}
		if c.cursor == chmodRecursiveSlot && !c.hasDir {
			c.cursor = chmodBitCount - 1
		}
		return m, nil
	case "0", "1", "2", "3", "4", "5", "6", "7":
		// Set the triad for the current class (owner/group/other).
		if c.cursor < chmodBitCount {
			row := c.cursor / 3
			n := key[0] - '0'
			mask := os.FileMode(7 << (6 - row*3))
			value := os.FileMode(n) << (6 - row*3)
			c.setMode((c.mode &^ mask) | value)
		}
		return m, nil
	case "u":
		c.cursor = 0
		return m, nil
	case "g":
		c.cursor = 3
		return m, nil
	case "o":
		c.cursor = 6
		return m, nil
	case "/":
		c.cursor = chmodOctalSlot
		c.octalEdit = true
		c.octal = files.FormatModeOctal(c.mode)
		return m, nil
	}
	return m, nil
}

func (m *App) updateFilesChmodOctal(key string) (tea.Model, tea.Cmd) {
	c := &m.files.chmod
	switch key {
	case "esc":
		c.octalEdit = false
		c.octal = files.FormatModeOctal(c.mode)
		return m, nil
	case "q":
		m.closeFilesChmod()
		return m, nil
	case "enter":
		mode, err := files.ParseModeOctal(c.octal)
		if err != nil {
			return m, m.setNotice(err.Error())
		}
		c.setMode(mode)
		c.octalEdit = false
		return m.applyFilesChmod()
	case "backspace", "ctrl+h":
		if len(c.octal) > 0 {
			c.octal = c.octal[:len(c.octal)-1]
		}
		if mode, err := files.ParseModeOctal(c.octal); err == nil {
			c.mode = files.NormalizeMode(mode)
		}
		return m, nil
	default:
		if len(key) == 1 && key[0] >= '0' && key[0] <= '7' {
			if len(c.octal) >= 4 {
				return m, nil
			}
			c.octal += key
			if mode, err := files.ParseModeOctal(c.octal); err == nil {
				c.mode = files.NormalizeMode(mode)
			}
		}
		return m, nil
	}
}

func (m *App) applyFilesChmod() (tea.Model, tea.Cmd) {
	c := m.files.chmod
	if !c.active {
		return m, nil
	}
	if c.octalEdit {
		mode, err := files.ParseModeOctal(c.octal)
		if err != nil {
			return m, m.setNotice(err.Error())
		}
		c.mode = files.NormalizeMode(mode)
	}
	index := c.pane
	if index < 0 || index > 1 {
		m.closeFilesChmod()
		return m, m.setNotice("Invalid pane")
	}
	pane := &m.files.panes[index]
	paths := append([]string(nil), c.paths...)
	mode := files.NormalizeMode(c.mode)
	recursive := c.recursive && c.hasDir
	m.closeFilesChmod()

	notice := files.FormatModeOctal(mode)
	if recursive {
		notice += " recursive"
	}

	if pane.kind == filesPaneLocal {
		for _, path := range paths {
			var err error
			if recursive {
				err = files.ChmodLocalRecursive(path, mode)
			} else {
				err = files.ChmodLocal(path, mode)
			}
			if err != nil {
				return m, m.setNotice(err.Error())
			}
		}
		pane.clearMarks()
		return m, tea.Batch(m.refreshFilesPane(index), m.setNotice(notice))
	}
	if pane.session == nil {
		return m, m.setNotice("not connected")
	}
	session := pane.session
	return m, func() tea.Msg {
		for _, path := range paths {
			var err error
			if recursive {
				err = files.ChmodRemoteRecursive(session, path, mode)
			} else {
				err = files.ChmodRemote(session, path, mode)
			}
			if err != nil {
				return filesOpDoneMsg{pane: index, action: "files_chmod", err: err}
			}
		}
		return filesOpDoneMsg{pane: index, action: "files_chmod", notice: notice}
	}
}

func (m *App) renderFilesChmod(s styleSet, width int) string {
	c := m.files.chmod
	var b strings.Builder
	title := c.title
	if strings.HasPrefix(title, "Permissions: ") {
		title = strings.TrimPrefix(title, "Permissions: ")
	} else if title == "Permissions" {
		title = "permissions"
	}
	b.WriteString("  " + s.active.Render(truncate(title, max(8, width-2))) + "\n")

	// Fixed columns: "› Owner  " / "  Owner  " (9) then three 4-wide cells.
	const labelWidth = 9
	headers := []string{"r", "w", "x"}
	classes := []string{"Owner", "Group", "Other"}
	b.WriteString(strings.Repeat(" ", labelWidth))
	for _, h := range headers {
		b.WriteString(s.muted.Render(fmt.Sprintf("%-4s", h)))
	}
	b.WriteString("\n")

	for row, class := range classes {
		focusedRow := c.cursor/3 == row && c.cursor < chmodBitCount
		marker := " "
		if focusedRow {
			marker = "›"
		}
		label := marker + " " + fmt.Sprintf("%-7s", class) // always 9 runes
		if focusedRow {
			b.WriteString(s.active.Render(label))
		} else {
			b.WriteString(s.muted.Render(label))
		}
		for col := 0; col < 3; col++ {
			idx := row*3 + col
			on := c.bitOn(idx)
			mark := "[ ]"
			if on {
				mark = "[x]"
			}
			cell := fmt.Sprintf("%-4s", mark)
			switch {
			case c.cursor == idx:
				b.WriteString(s.selected.Render(cell))
			case on:
				b.WriteString(s.value.Render(cell))
			default:
				b.WriteString(s.muted.Render(cell))
			}
		}
		b.WriteString("\n")
	}

	modeLine := files.FormatModeOctal(c.mode) + "  " + files.FormatModeSymbolic(c.mode)
	b.WriteString("  " + s.value.Render(truncate(modeLine, max(8, width-2))) + "\n")

	if c.hasDir {
		rec := "[ ] contents"
		if c.recursive {
			rec = "[x] contents"
		}
		marker := " "
		if c.cursor == chmodRecursiveSlot {
			marker = "›"
		}
		label := marker + " Recursive"
		if c.cursor == chmodRecursiveSlot {
			b.WriteString(s.active.Render(label) + " " + s.selected.Render(rec) + "\n")
		} else {
			b.WriteString(s.muted.Render(label) + " " + s.value.Render(rec) + "\n")
		}
	}

	octalValue := files.FormatModeOctal(c.mode)
	if c.octalEdit {
		octalValue = c.octal + "█"
	}
	marker := " "
	if c.cursor == chmodOctalSlot {
		marker = "›"
	}
	label := marker + " Octal   "
	if c.cursor == chmodOctalSlot {
		b.WriteString(s.active.Render(label))
		if c.octalEdit {
			b.WriteString(s.value.Render(octalValue) + "\n")
		} else {
			b.WriteString(s.selected.Render(octalValue) + "\n")
		}
	} else {
		b.WriteString(s.muted.Render(label) + s.value.Render(octalValue) + "\n")
	}

	return b.String()
}

func (m *App) filesChmodHint() string {
	c := m.files.chmod
	if c.octalEdit {
		return "type octal · 󰌑 apply · esc back"
	}
	return "␣ toggle · 0-7 set · 󰌑 apply · esc"
}
