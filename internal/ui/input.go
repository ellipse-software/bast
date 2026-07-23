package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/keys"
)

func (m *App) updateMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || m.help || m.form != nil {
		return m, nil
	}

	if mouse.Y == 0 {
		tabsStart := lipgloss.Width(" BAST ") + 2
		hostsEnd := tabsStart + lipgloss.Width("[1] Hosts")
		keysStart := hostsEnd + 3
		keysEnd := keysStart + lipgloss.Width("[2] Keys")
		switch {
		case mouse.X >= tabsStart && mouse.X < hostsEnd:
			m.section, m.cursor, m.search = hostsSection, 0, ""
		case mouse.X >= keysStart && mouse.X < keysEnd:
			m.section, m.cursor, m.search = keysSection, 0, ""
		}
		return m, nil
	}

	listWidth, _, bodyHeight := m.columnDimensions()
	row := mouse.Y - 2
	if mouse.X < 0 || mouse.X >= listWidth || row < 0 || row >= bodyHeight {
		return m, nil
	}
	count := m.itemCount()
	if count == 0 {
		return m, nil
	}
	index := scrollStart(m.cursor, count, bodyHeight) + row
	if index < count {
		m.cursor = index
	}
	return m, nil
}

func (m *App) updateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.help {
		if key == "?" || key == "esc" || key == "q" {
			m.help = false
		}
		return m, nil
	}
	if strings.HasPrefix(m.search, "\x00") {
		return m.updateSearch(key)
	}
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.help = true
	case ".":
		if m.section == hostsSection {
			m.showHidden = !m.showHidden
			m.cursor = 0
			m.statusID++
			m.statusError = false
			if m.showHidden {
				m.status = "Showing hidden hosts"
			} else {
				m.status = "Hidden hosts concealed"
				statusID := m.statusID
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return clearStatusMsg(statusID)
				})
			}
		}
	case "1":
		m.section, m.cursor, m.search = hostsSection, 0, ""
	case "2":
		m.section, m.cursor, m.search = keysSection, 0, ""
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		if m.itemCount() > 0 {
			m.cursor = m.itemCount() - 1
		}
	case "/":
		m.search = "\x00"
		m.cursor = 0
	case "r":
		m.loading, m.status = true, "Reloading OpenSSH files…"
		return m, m.loadCmd()
	case "s":
		if m.section == hostsSection {
			m.cycleSort()
		}
	case "a":
		if m.section == hostsSection {
			m.openAddHostForm()
		} else {
			m.openGenerateForm()
		}
	case "e":
		if m.section == hostsSection {
			m.openEditHostForm()
		} else {
			m.openEditKeyForm()
		}
	case "d":
		if m.section == hostsSection {
			m.openDeleteHostForm()
		} else {
			m.openDeleteKeyForm()
		}
	case "i":
		if m.section == keysSection {
			m.openImportForm()
		}
	case "x":
		if m.section == keysSection {
			m.openExportForm()
		}
	case "p":
		if m.section == keysSection {
			return m.runPassphraseAction()
		}
	case "c":
		if m.section == keysSection {
			selected, ok := m.selectedKey()
			if ok {
				public, err := keys.PublicText(selected)
				if err != nil {
					m.setError(err)
				} else {
					m.status, m.statusError = "Public key copied", false
					return m, tea.SetClipboard(public)
				}
			}
		}
	case "K":
		if m.section == hostsSection {
			m.openKnownHostForm()
		}
	case "f":
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok {
				_, err := m.metadata.ToggleFavorite(host.Alias)
				if err != nil {
					m.setError(err)
				} else {
					m.sortHosts()
					m.cursor = 0
				}
			}
		}
	case "h":
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok {
				hidden, err := m.metadata.ToggleHidden(host.Alias)
				if err != nil {
					m.setError(err)
				} else {
					m.status, m.statusError = "Host hidden", false
					if !hidden {
						m.status = "Host shown"
					}
					m.clampCursor()
				}
			}
		}
	case "enter":
		if m.section == hostsSection {
			return m.connectSelected()
		}
	}
	return m, nil
}

func (m *App) updateSearch(key string) (tea.Model, tea.Cmd) {
	query := strings.TrimPrefix(m.search, "\x00")
	switch key {
	case "enter":
		m.search = query
	case "esc":
		m.search = ""
	case "backspace", "ctrl+h":
		if len(query) > 0 {
			query = query[:len(query)-1]
		}
		m.search = "\x00" + query
	default:
		if len([]rune(key)) == 1 {
			m.search = "\x00" + query + key
		}
	}
	m.cursor = 0
	return m, nil
}

func (m *App) connectSelected() (tea.Model, tea.Cmd) {
	host, ok := m.selectedHost()
	if !ok {
		return m, nil
	}
	cmd, err := m.openSSH.SSHCommand(host.Alias)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	if err := m.metadata.RecordUse(host.Alias); err != nil {
		m.setError(err)
		return m, nil
	}
	m.status = "Connected to " + host.Alias + "; exit or 󰌑 then ~. to return"
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return processDoneMsg{name: "SSH session", err: err} })
}

func (m *App) runPassphraseAction() (tea.Model, tea.Cmd) {
	key, ok := m.selectedKey()
	if !ok {
		return m, nil
	}
	cmd, err := m.keyring.PassphraseCommand(key)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return processDoneMsg{name: "Passphrase change", err: err} })
}
