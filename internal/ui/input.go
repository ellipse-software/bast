package ui

import (
	"io"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	connectionBanner = "\x1b[1;38;2;139;92;246m BAST \x1b[0m  Connecting to server…\r\n" +
		"\x1b[38;2;107;114;128m Stuck? Press Enter, then ~. to return to Bast.\x1b[0m\r\n\r\n"
	// ClearTerminal clears both the visible screen and its scrollback.
	ClearTerminal = "\x1b[H\x1b[2J\x1b[3J"
)

type clearAfterProcess struct {
	cmd *exec.Cmd
}

func (c *clearAfterProcess) Run() error {
	output := c.cmd.Stdout
	if output == nil {
		output = os.Stdout
	}
	_, _ = io.WriteString(output, ClearTerminal+connectionBanner)
	err := c.cmd.Run()
	_, _ = io.WriteString(output, ClearTerminal)
	return err
}

func (c *clearAfterProcess) SetStdin(input io.Reader) {
	if c.cmd.Stdin == nil {
		c.cmd.Stdin = input
	}
}

func (c *clearAfterProcess) SetStdout(output io.Writer) {
	if c.cmd.Stdout == nil {
		c.cmd.Stdout = output
	}
}

func (c *clearAfterProcess) SetStderr(output io.Writer) {
	if c.cmd.Stderr == nil {
		c.cmd.Stderr = output
	}
}

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
	if row < 0 || row >= bodyHeight {
		return m, nil
	}
	if m.section == keysSection && mouse.X >= listWidth+3 && row == keyInstallActionRow {
		key, ok := m.selectedKey()
		if ok && (key.PublicPath != "" || key.PrivatePath != "") && mouse.X < listWidth+3+len(keyInstallAction) {
			m.openInstallKeyForm()
		}
		return m, nil
	}
	if mouse.X < 0 || mouse.X >= listWidth {
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
			if m.showHidden {
				return m, m.setNotice("Showing hidden hosts")
			}
			return m, m.setNotice("Hidden hosts concealed")
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
		m.loading = true
		return m, tea.Batch(m.loadCmd(), m.setNotice("Reloading OpenSSH files…"))
	case "s":
		if m.section == hostsSection {
			return m, m.cycleSort()
		}
	case "space":
		if m.section == hostsSection {
			return m, m.toggleSelectedGroup()
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
	case "u":
		if m.section == keysSection {
			m.openInstallKeyForm()
		}
	case "p":
		if m.section == keysSection {
			return m.runPassphraseAction()
		}
	case "c":
		if m.section == keysSection {
			selected, ok := m.selectedKey()
			if ok {
				public, err := m.keyring.PublicText(selected)
				if err != nil {
					m.setError(err)
				} else {
					return m, tea.Batch(tea.SetClipboard(public), m.setNotice("Public key copied"))
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
					notice := "Host hidden"
					if !hidden {
						notice = "Host shown"
					}
					m.clampCursor()
					return m, m.setNotice(notice)
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
	m.status = "Connected to " + m.hostLabel(host) + "; exit closes Bast; 󰌑 then ~. force-closes SSH"
	return m, tea.Exec(&clearAfterProcess{cmd: cmd}, func(err error) tea.Msg {
		return processDoneMsg{name: "SSH session", err: err, exitBast: true}
	})
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
