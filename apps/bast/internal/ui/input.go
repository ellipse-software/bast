package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/connectbanner"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
)

type connectionProcess struct {
	cmd     *exec.Cmd
	prepare func(status func(string)) error
	version string
}

func (c *connectionProcess) Run() error {
	output := c.cmd.Stdout
	if output == nil {
		output = os.Stdout
	}
	input := c.cmd.Stdin
	if input == nil {
		input = os.Stdin
	}
	connectbanner.Write(output)
	if c.prepare != nil {
		if err := c.prepare(connectbanner.Status(output)); err != nil {
			_, _ = fmt.Fprintf(output, "\r\nConnection failed: %v\r\n", err)
			if telemetry.Enabled() {
				telemetry.OfferReport(input, output, telemetry.Report{
					Message: err.Error(),
					Version: c.version,
					Context: "connect_prepare",
				})
			} else {
				connectbanner.WaitToContinue(input, output)
			}
			return fmt.Errorf("prepare connection: %w", err)
		}
		_, _ = io.WriteString(output, "\r\n")
	}
	if err := c.cmd.Run(); err != nil {
		connectbanner.WaitToContinue(input, output)
		return err
	}
	return nil
}

func (c *connectionProcess) SetStdin(input io.Reader) {
	if c.cmd.Stdin == nil {
		c.cmd.Stdin = input
	}
}

func (c *connectionProcess) SetStdout(output io.Writer) {
	if c.cmd.Stdout == nil {
		c.cmd.Stdout = output
	}
}

func (c *connectionProcess) SetStderr(output io.Writer) {
	if c.cmd.Stderr == nil {
		c.cmd.Stderr = output
	}
}

func (m *App) updateMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft || m.help || m.credits || (m.statusError && m.status != "") {
		return m, nil
	}
	m.scrollbarDragging = false
	if m.form != nil {
		if isHostForm(m.form) {
			x, y, width := m.hostFormSaveButtonBounds()
			if mouse.Y == y && mouse.X >= x && mouse.X < x+width {
				m.commitFormField()
				return m.submitForm()
			}
		}
		return m, nil
	}

	if mouse.Y == 0 {
		if m.syncBusy != "" {
			return m, nil
		}
		tabsStart := lipgloss.Width(headerTitle) + 2
		hostsEnd := tabsStart + lipgloss.Width(headerTabLabels[0])
		keysStart := hostsEnd + lipgloss.Width(headerTabSpacing)
		keysEnd := keysStart + lipgloss.Width(headerTabLabels[1])
		syncStart := keysEnd + lipgloss.Width(headerTabSpacing)
		syncEnd := syncStart + lipgloss.Width(headerTabLabels[2])
		filesStart := syncEnd + lipgloss.Width(headerTabSpacing)
		filesEnd := filesStart + lipgloss.Width(headerTabLabels[3])
		switch {
		case mouse.X >= tabsStart && mouse.X < hostsEnd:
			m.clearFilesOverlays()
			m.section, m.cursor, m.search = hostsSection, 0, ""
		case mouse.X >= keysStart && mouse.X < keysEnd:
			m.clearFilesOverlays()
			m.section, m.cursor, m.search = keysSection, 0, ""
		case mouse.X >= syncStart && mouse.X < syncEnd:
			m.clearFilesOverlays()
			m.section, m.syncProvider, m.syncCursor, m.search = syncSection, "", 0, ""
			return m, m.syncStatusCmd()
		case mouse.X >= filesStart && mouse.X < filesEnd:
			return m, m.enterFilesSection()
		}
		return m, nil
	}

	layout := m.panelLayout()
	listWidth := layout.listWidth

	if m.section == syncSection {
		return m, nil
	}
	if m.section == filesSection {
		return m.updateFilesMouse(msg)
	}

	if m.section == hostsSection {
		action := ""
		if host, ok := m.selectedHost(); ok {
			action = m.hostPrimaryAction(host)
		} else if _, ok := m.selectedHistorySuggestion(); ok {
			action = addAction
		}
		if action != "" {
			btnX, btnY, btnWidth := m.hostActionButtonBounds(layout, action)
			inDetail := layout.mobile || mouse.X > listWidth
			if inDetail && mouse.Y == btnY && mouse.X >= btnX && mouse.X < btnX+btnWidth {
				if action == addAction {
					return m.importSelectedHistorySuggestion()
				}
				return m.connectSelected()
			}
		}
	}

	if m.section == keysSection {
		detailX := listWidth + 3
		if layout.mobile {
			detailX = 2
		}
		detailRow := mouse.Y - layout.detailTop
		if detailRow == keyInstallActionRow && mouse.X >= detailX {
			key, ok := m.selectedKey()
			if ok && (key.PublicPath != "" || key.PrivatePath != "") && mouse.X < detailX+len(keyInstallAction) {
				m.openInstallKeyForm()
			}
			return m, nil
		}
	}

	if layout.mobile {
		if mouse.Y < layout.listTop || mouse.Y >= layout.listTop+layout.listHeight {
			return m, nil
		}
	} else if mouse.X < 0 || mouse.X >= listWidth {
		return m, nil
	}

	count := m.itemCount()
	if count == 0 {
		return m, nil
	}
	if layout.mobile && count > layout.listHeight && mouse.X >= layout.listWidth-mobileScrollbarWidth {
		row := mouse.Y - layout.listTop
		switch {
		case row == 0:
			m.cursor = 0
		case row == layout.listHeight-1:
			m.cursor = count - 1
		case row > 0 && row < layout.listHeight-1:
			m.scrollbarDragging = true
			track := max(1, layout.listHeight-3)
			m.cursor = ((row-1)*(count-1) + track/2) / track
		}
		return m, nil
	}
	row := mouse.Y - layout.listTop
	if row < 0 || row >= layout.listHeight {
		return m, nil
	}
	index := scrollStart(m.cursor, count, layout.listHeight) + row
	if index < count {
		m.cursor = index
	}
	return m, nil
}

func (m *App) updateMouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if !m.scrollbarDragging || msg.Mouse().Button != tea.MouseLeft {
		return m, nil
	}
	layout := m.panelLayout()
	count := m.itemCount()
	if !layout.mobile || count <= layout.listHeight {
		m.scrollbarDragging = false
		return m, nil
	}
	track := max(1, layout.listHeight-3)
	row := min(layout.listHeight-2, max(1, msg.Mouse().Y-layout.listTop))
	m.cursor = ((row-1)*(count-1) + track/2) / track
	return m, nil
}

func (m *App) updateMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.credits || m.form != nil {
		return m, nil
	}
	if m.help {
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			m.scrollHelp(-3)
		case tea.MouseWheelDown:
			m.scrollHelp(3)
		}
		return m, nil
	}
	if m.section == filesSection {
		return m.updateFilesMouseWheel(msg)
	}
	mouse := msg.Mouse()
	layout := m.panelLayout()
	if mouse.Y < layout.listTop || mouse.Y >= layout.listTop+layout.listHeight {
		return m, nil
	}
	if !layout.mobile && (mouse.X < 0 || mouse.X >= layout.listWidth) {
		return m, nil
	}
	switch mouse.Button {
	case tea.MouseWheelUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.MouseWheelDown:
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	}
	return m, nil
}

func (m *App) updateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if strings.HasPrefix(m.search, "\x00") {
		return m.updateSearch(key)
	}
	if m.credits {
		if key == "?" {
			m.credits, m.help, m.helpOffset = false, true, 0
		} else if key == "q" {
			return m, tea.Quit
		} else if key == "v" || key == "esc" || key == "backspace" || key == "ctrl+h" {
			m.credits = false
		}
		return m, nil
	}
	if m.help {
		switch key {
		case "v":
			m.help, m.credits, m.helpOffset = false, true, 0
		case "q":
			return m, tea.Quit
		case "?", "esc", "backspace", "ctrl+h":
			m.help, m.helpOffset = false, 0
		case "up", "k":
			m.scrollHelp(-1)
		case "down", "j":
			m.scrollHelp(1)
		case "pgup":
			m.scrollHelp(-m.helpBodyHeight())
		case "pgdown":
			m.scrollHelp(m.helpBodyHeight())
		case "g", "home":
			m.helpOffset = 0
		case "G", "end":
			m.helpOffset = m.maxHelpOffset()
		}
		return m, nil
	}
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.filesTyping() {
			break
		}
		return m, tea.Quit
	case "?":
		m.help, m.helpOffset = true, 0
		return m, nil
	case "1", "2", "3", "4":
		if m.filesTyping() {
			break
		}
		if m.syncBusy != "" {
			return m, nil
		}
		switch key {
		case "1":
			m.clearFilesOverlays()
			m.section, m.cursor, m.search = hostsSection, 0, ""
			return m, nil
		case "2":
			m.clearFilesOverlays()
			m.section, m.cursor, m.search = keysSection, 0, ""
			return m, nil
		case "3":
			m.clearFilesOverlays()
			m.section, m.syncProvider, m.syncCursor, m.search = syncSection, "", 0, ""
			return m, m.syncStatusCmd()
		case "4":
			return m, m.enterFilesSection()
		}
	case "v":
		if m.section != filesSection {
			m.credits = true
			return m, nil
		}
	}
	if m.section == filesSection {
		m.initFilesState()
		pane := m.filesFocusedPane()
		if pane.pathEdit && !m.files.transfer.active && !pane.connecting && !m.files.jump.active {
			return m.updateFilesPathInputMsg(pane, msg)
		}
		return m.updateFilesKeys(key)
	}
	switch key {
	case ".":
		if m.section == hostsSection {
			m.showHidden = !m.showHidden
			m.cursor = 0
			if m.showHidden {
				return m, m.setNotice("Showing hidden hosts")
			}
			return m, m.setNotice("Hidden hosts concealed")
		}
	case "esc":
		if m.section == syncSection && m.syncProvider != "" {
			return m.updateSyncKeys(key)
		}
		return m, tea.Quit
	case "backspace", "ctrl+h":
		if m.section == syncSection && m.syncProvider != "" {
			return m.updateSyncKeys(key)
		}
		return m, tea.Quit
	case "up", "k":
		if m.section == syncSection {
			return m.updateSyncKeys(key)
		}
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.section == syncSection {
			return m.updateSyncKeys(key)
		}
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	case "home", "g":
		if m.section == syncSection {
			return m.updateSyncKeys(key)
		}
		m.cursor = 0
	case "end", "G":
		if m.section == syncSection {
			return m.updateSyncKeys(key)
		}
		if m.itemCount() > 0 {
			m.cursor = m.itemCount() - 1
		}
	case "/":
		if m.section == syncSection {
			return m, nil
		}
		m.search = "\x00"
		m.cursor = 0
	case "r":
		if m.section == syncSection {
			return m.updateSyncKeys(key)
		}
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok && host.Synced && host.SyncSource == "box" && m.hostLooksStopped(host) {
				return m, m.resumeSelectedBox(host, false)
			}
		}
		m.loading = true
		m.enriching = false
		return m, tea.Batch(m.loadCmd(), m.setNotice("Reloading OpenSSH files…"))
	case "s":
		if m.section == hostsSection {
			return m, m.cycleSort()
		}
	case "space":
		if m.section == hostsSection {
			if m.historySuggestionsHeaderSelected() {
				return m, m.toggleHistorySuggestions()
			}
			return m, m.toggleSelectedGroup()
		}
	case "[":
		if m.section == hostsSection {
			return m, m.collapseAllGroups()
		}
	case "]":
		if m.section == hostsSection {
			return m, m.expandAllGroups()
		}
	case "a":
		if m.section == syncSection {
			return m, nil
		}
		if m.section == hostsSection {
			m.openAddHostForm()
		} else {
			m.openGenerateForm()
		}
	case "e":
		if m.section == syncSection {
			return m, nil
		}
		if m.section == hostsSection {
			if _, ok := m.selectedHistorySuggestion(); ok {
				m.openHistoryHostForm()
			} else if _, ok := m.selectedGroupHeader(); ok {
				m.openEditGroupForm()
			} else {
				if host, ok := m.selectedHost(); ok && (m.loading || m.enriching) && host.Resolved.HostName == "" {
					return m, m.setNotice("Host details are still loading")
				}
				m.openEditHostForm()
			}
		} else {
			m.openEditKeyForm()
		}
	case "m":
		if m.section == hostsSection {
			m.openGroupAssignmentForm()
		}
	case "d":
		if m.section == syncSection {
			return m, nil
		}
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
		if m.section == hostsSection {
			return m, m.dismissSelectedHistorySuggestion()
		}
		if m.section == keysSection {
			m.openExportForm()
		}
	case "u":
		if m.section == keysSection {
			m.openInstallKeyForm()
		}
	case "p":
		if m.section == hostsSection {
			return m, m.promoteSelectedHost()
		}
		if m.section == keysSection {
			if key, ok := m.selectedKey(); ok && !key.Managed {
				return m, m.promoteSelectedKey()
			}
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
	case "o":
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok && host.Synced && host.SyncSource == "box" {
				if m.hostLooksStopped(host) {
					return m, m.setNotice("Box is already stopped")
				}
				m.openBoxStopForm(host)
				return m, nil
			}
		}
	case "n":
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok && host.Synced && host.SyncSource == "box" {
				m.openBoxForkForm(host)
				return m, nil
			}
		}
	case "f":
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok {
				if host.Synced {
					return m, m.setNotice("Synced hosts are read-only")
				}
				_, err := m.metadata.ToggleFavorite(host.Alias)
				if err != nil {
					m.setError(err)
				} else {
					m.sortHosts()
					m.cursor = 0
				}
			}
		}
	case "F":
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok {
				return m, m.openFilesForHost(host)
			}
		}
	case "h":
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok {
				if host.Synced {
					return m, m.setNotice("Synced hosts are read-only")
				}
				hidden, err := m.metadata.ToggleHidden(host.Alias)
				if err != nil {
					m.setError(err)
				} else {
					m.refreshHostMetadata()
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
		if m.section == syncSection {
			return m.updateSyncKeys(key)
		}
		if m.section == hostsSection {
			if m.historySuggestionsHeaderSelected() {
				return m, m.toggleHistorySuggestions()
			}
			if _, ok := m.selectedHistorySuggestion(); ok {
				return m.importSelectedHistorySuggestion()
			}
			if _, groupSelected := m.selectedGroupHeader(); groupSelected {
				return m, m.toggleSelectedGroup()
			}
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
	if host.Synced && host.SyncSource == "box" && m.hostLooksStopped(host) {
		return m, m.resumeSelectedBox(host, true)
	}
	if host.Synced && host.SyncID != "" {
		var ensure func(context.Context, sshconfig.Host, func(string)) error
		switch host.SyncSource {
		case "gcp":
			ensure = m.syncer.EnsureGCPAccess
		case "aws":
			ensure = m.syncer.EnsureAWSAccess
		case "azure":
			ensure = m.syncer.EnsureAzureAccess
		case "box":
			ensure = m.syncer.EnsureBoxAccess
		}
		if ensure != nil {
			return m.startSSH(host, func(status func(string)) error {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				if err := ensure(ctx, host, status); err != nil {
					return err
				}
				return m.metadata.RecordUse(host.Alias)
			})
		}
	}
	return m.startSSH(host, nil)
}

func (m *App) startSSH(host sshconfig.Host, prepare func(func(string)) error) (tea.Model, tea.Cmd) {
	cmd, err := m.openSSH.SSHCommand(host.Alias)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	if prepare == nil {
		if err := m.metadata.RecordUse(host.Alias); err != nil {
			m.setError(err)
			return m, nil
		}
	}
	telemetry.Track("connect", m.version)
	m.status = "Connected to " + m.hostLabel(host) + "; exit returns to Bast; 󰌑 then ~. force-closes SSH"
	return m, tea.Exec(&connectionProcess{cmd: cmd, prepare: prepare, version: m.version}, func(err error) tea.Msg {
		return processDoneMsg{name: "SSH session", err: err, sshSession: true}
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

func (m *App) promoteSelectedHost() tea.Cmd {
	host, ok := m.selectedHost()
	if !ok {
		return nil
	}
	if host.Synced {
		return m.setNotice("Synced cloud hosts cannot be promoted")
	}
	if host.Managed {
		return m.setNotice("Host is already Bast managed")
	}
	if (m.loading || m.enriching) && host.Resolved.HostName == "" {
		return m.setNotice("Host details are still loading")
	}
	if _, err := m.config.Promote(host); err != nil {
		m.setError(err)
		return nil
	}
	m.loading = true
	m.enriching = false
	return tea.Batch(m.loadCmd(), m.setNotice("Host promoted to Bast managed"))
}

func (m *App) promoteSelectedKey() tea.Cmd {
	key, ok := m.selectedKey()
	if !ok {
		return nil
	}
	if key.Managed {
		return m.setNotice("Key is already Bast managed")
	}
	if err := m.keyring.Promote(key, key.Name); err != nil {
		m.setError(err)
		return nil
	}
	m.loading = true
	m.enriching = false
	return tea.Batch(m.loadCmd(), m.setNotice("Key promoted to Bast managed"))
}
