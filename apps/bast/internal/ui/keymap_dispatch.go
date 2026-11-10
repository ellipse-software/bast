package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud"
)

func (m *App) dispatch(id Action) (tea.Model, tea.Cmd) {
	switch id {
	case ActionQuit:
		m.closeOwnedTunnels()
		return m, tea.Quit
	case ActionHelpToggle:
		m.closeDoctor()
		m.onboarding = false
		m.credits = false
		m.openPalette()
		return m, nil
	case ActionHelpClose:
		m.closePalette()
		return m, nil
	case ActionCreditsOpen:
		m.openCredits()
		return m, nil
	case ActionCreditsClose:
		m.credits = false
		return m, nil
	case ActionCreditsLeft:
		m.moveCreditChoice(-1)
		return m, nil
	case ActionCreditsRight:
		m.moveCreditChoice(1)
		return m, nil
	case ActionCreditsActivate:
		return m.openCreditChoice()
	case ActionOnboardingReplay:
		m.replayOnboarding()
		return m, nil
	case ActionSponsorOpen:
		return m.openSponsor()
	case ActionDoctorOpen:
		return m, m.openDoctor()
	case ActionDoctorClose:
		m.closeDoctor()
		return m, nil
	case ActionDoctorScrollUp:
		m.scrollDoctor(-1)
		return m, nil
	case ActionDoctorScrollDown:
		m.scrollDoctor(1)
		return m, nil
	case ActionDoctorPageUp:
		m.scrollDoctor(-m.bodyHeight())
		return m, nil
	case ActionDoctorPageDown:
		m.scrollDoctor(m.bodyHeight())
		return m, nil
	case ActionDoctorTop:
		m.doctorOffset = 0
		return m, nil
	case ActionDoctorBottom:
		m.doctorOffset = m.maxDoctorOffset()
		return m, nil
	case ActionTabHosts:
		return m, m.switchToSection(hostsSection)
	case ActionTabKeys:
		return m, m.switchToSection(keysSection)
	case ActionTabVault:
		return m, m.switchToSection(vaultSection)
	case ActionTabSync:
		return m, m.switchToSection(syncSection)
	case ActionTabFiles:
		return m, m.switchToSection(filesSection)
	case ActionMoveUp:
		return m.actionMove(-1)
	case ActionMoveDown:
		return m.actionMove(1)
	case ActionMoveLeft:
		return m.actionMoveX(-1)
	case ActionMoveRight:
		return m.actionMoveX(1)
	case ActionJumpTop:
		return m.actionJumpTop()
	case ActionJumpBottom:
		return m.actionJumpBottom()
	case ActionSearch:
		m.search = "\x00"
		m.cursor = 0
		return m, nil
	case ActionReload:
		return m.actionReload()
	case ActionToggleHidden:
		return m.actionToggleHidden()
	case ActionToggleGroup:
		return m.actionToggleGroup()
	case ActionCollapseAll:
		if m.section == hostsSection {
			return m, m.collapseAllGroups()
		}
		return m, nil
	case ActionExpandAll:
		if m.section == hostsSection {
			return m, m.expandAllGroups()
		}
		return m, nil
	case ActionAddHost:
		m.openAddHostForm()
		return m, nil
	case ActionAddEntry:
		if m.section == hostsSection {
			m.openAddEntryChooser()
		}
		return m, nil
	case ActionCloseTunnel:
		if m.section == hostsSection {
			if hostsFooterForwardOpen(m) {
				return m.closeSelectedForward()
			}
			return m.closeSelectedTunnel()
		}
		return m, nil
	case ActionStartForward:
		if m.section == hostsSection {
			return m.startSelectedForward()
		}
		return m, nil
	case ActionEdit:
		return m.actionEdit()
	case ActionAssignGroup:
		if m.section == hostsSection {
			if _, ok := m.selectedEntry(); ok {
				m.openEntryGroupForm()
				return m, nil
			}
			m.openGroupAssignmentForm()
		}
		return m, nil
	case ActionDelete:
		return m.actionDelete()
	case ActionDismissHistory:
		if m.section == hostsSection {
			return m, m.dismissSelectedHistorySuggestion()
		}
		return m, nil
	case ActionPromote:
		if m.section == hostsSection {
			return m, m.promoteSelectedHost()
		}
		return m, nil
	case ActionKnownHosts:
		if m.section == hostsSection {
			m.openKnownHostForm()
		}
		return m, nil
	case ActionStopSandbox:
		return m.actionStopSandbox()
	case ActionRestartHost:
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok {
				return m.restartSyncedHost(host)
			}
		}
		return m, nil
	case ActionNewOrFork:
		return m.actionNewOrFork()
	case ActionFavorite:
		return m.actionFavorite()
	case ActionOpenFiles:
		if m.section == hostsSection {
			if host, ok := m.selectedHost(); ok {
				return m, m.openFilesForHost(host)
			}
		}
		return m, nil
	case ActionOpenRDP:
		if m.section == hostsSection {
			if _, ok := m.selectedEntry(); ok {
				return m.openSelectedEntry()
			}
			return m.openSelectedRDP()
		}
		return m, nil
	case ActionHideHost:
		return m.actionHideHost()
	case ActionHostsEnter:
		return m.actionHostsEnter()
	case ActionSortOrSync:
		return m.actionSortOrSync()
	case ActionGenerateKey:
		m.openGenerateForm()
		return m, nil
	case ActionInstallKey:
		m.openInstallKeyForm()
		return m, nil
	case ActionImportKey:
		m.openImportForm()
		return m, nil
	case ActionExportKey:
		m.openExportForm()
		return m, nil
	case ActionCopyPublicKey:
		return m.actionCopyPublicKey()
	case ActionKeysPassphrase:
		return m.actionKeysPassphrase()
	case ActionVaultEnter, ActionVaultSync:
		return m.updateVaultKeys(canonicalVaultKey(id))
	case ActionSyncEnter, ActionSyncBack, ActionSyncNow:
		return m.updateSyncKeys(canonicalSyncKey(id))
	case ActionSyncTab:
		return m.updateSyncKeys("tab")
	case ActionFilesTab:
		m.initFilesState()
		m.files.focus = 1 - m.files.focus
		return m, nil
	case ActionFilesSwap:
		m.initFilesState()
		m.files.panes[0], m.files.panes[1] = m.files.panes[1], m.files.panes[0]
		m.files.focus = 1 - m.files.focus
		return m, nil
	case ActionFilesLocal:
		m.initFilesState()
		return m, m.setFilesPaneLocal(m.files.focus)
	case ActionFilesRemote:
		m.initFilesState()
		return m, m.setFilesPaneRemote(m.files.focus)
	case ActionFilesParent:
		m.initFilesState()
		if m.filesFocusedPane().pickingHost() {
			return m.filesPickerLeft()
		}
		return m.filesParent()
	case ActionFilesEnter:
		m.initFilesState()
		return m.activateFilesSelection()
	case ActionFilesOpenDir:
		m.initFilesState()
		return m.enterFilesDirectory()
	case ActionFilesPreview:
		m.initFilesState()
		return m.openFilesPreview()
	case ActionFilesJump:
		m.initFilesState()
		return m.beginFilesJump()
	case ActionFilesPath:
		m.initFilesState()
		pane := m.filesFocusedPane()
		if pane.pickingHost() {
			pane.hostSearch = "\x00"
			pane.hostCursor = 0
			return m, nil
		}
		return m.beginFilesPathEdit()
	case ActionFilesMark:
		m.initFilesState()
		if m.filesFocusedPane().pickingHost() {
			return m.filesPickerToggle()
		}
		return m.toggleFilesMark()
	case ActionFilesRange:
		m.initFilesState()
		return m.toggleFilesRange()
	case ActionFilesCopy:
		m.initFilesState()
		return m.startFilesTransfer(false)
	case ActionFilesMove:
		m.initFilesState()
		return m.startFilesTransfer(true)
	case ActionFilesMkdir:
		m.initFilesState()
		return m.openFilesMkdirForm()
	case ActionFilesRename:
		m.initFilesState()
		return m.openFilesRenameForm()
	case ActionFilesDelete:
		m.initFilesState()
		return m.openFilesDeleteForm()
	case ActionFilesInfo:
		m.initFilesState()
		return m.openFilesInfo()
	case ActionFilesChmod:
		m.initFilesState()
		return m.openFilesChmodMenu()
	case ActionFilesShell:
		m.initFilesState()
		return m.filesOpenShell()
	case ActionFilesHidden:
		m.initFilesState()
		pane := m.filesFocusedPane()
		if pane.pickingHost() {
			return m, nil
		}
		pane.showHidden = !pane.showHidden
		return m, m.refreshFilesPane(m.files.focus)
	case ActionFilesDisconnect:
		m.initFilesState()
		return m, m.returnFilesPaneToPicker(m.files.focus)
	case ActionFilesEscape:
		return m.actionFilesEscape()
	default:
		return m, nil
	}
}

func canonicalVaultKey(id Action) string {
	if id == ActionVaultSync {
		return "r"
	}
	return "enter"
}

func canonicalSyncKey(id Action) string {
	switch id {
	case ActionSyncBack:
		return "esc"
	case ActionSyncNow:
		return "s"
	default:
		return "enter"
	}
}

func (m *App) actionMove(delta int) (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		if delta < 0 {
			return m.updateSyncKeys("k")
		}
		return m.updateSyncKeys("j")
	}
	if m.section == vaultSection {
		if delta < 0 {
			return m.updateVaultKeys("k")
		}
		return m.updateVaultKeys("j")
	}
	if m.section == filesSection {
		return m.moveFilesCursor(delta)
	}
	if delta < 0 {
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	}
	if m.cursor+1 < m.itemCount() {
		m.cursor++
	}
	return m, nil
}

func (m *App) actionMoveX(delta int) (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		if delta < 0 {
			return m.updateSyncKeys("h")
		}
		return m.updateSyncKeys("l")
	}
	if m.section == filesSection && delta > 0 {
		return m.activateFilesSelection()
	}
	if m.section == hostsSection {
		return m, m.moveHostGroup(delta)
	}
	return m, nil
}

func (m *App) actionJumpTop() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		return m.updateSyncKeys("g")
	}
	if m.section == vaultSection {
		return m.updateVaultKeys("g")
	}
	if m.section == filesSection {
		return m.moveFilesCursorHome()
	}
	m.cursor = 0
	return m, nil
}

func (m *App) actionJumpBottom() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		return m.updateSyncKeys("G")
	}
	if m.section == vaultSection {
		return m.updateVaultKeys("G")
	}
	if m.section == filesSection {
		return m.moveFilesCursorEnd()
	}
	if m.itemCount() > 0 {
		m.cursor = m.itemCount() - 1
	}
	return m, nil
}

func (m *App) actionReload() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		if m.noSync {
			return m, m.syncOffNotice()
		}
		return m.updateSyncKeys("r")
	}
	if m.section == vaultSection {
		return m.updateVaultKeys("r")
	}
	if m.section == hostsSection {
		if host, ok := m.selectedHost(); ok && !m.noSync {
			if cmd := m.resumeSyncedHost(host, false); cmd != nil {
				return m, cmd
			}
		}
	}
	m.loading = true
	m.enriching = false
	return m, tea.Batch(m.loadCmd(), m.setNotice("Reloading OpenSSH files…"))
}

func (m *App) actionToggleHidden() (tea.Model, tea.Cmd) {
	if m.section != hostsSection {
		return m, nil
	}
	kind, name := m.hostCursorKey()
	m.showHidden = !m.showHidden
	m.restoreHostCursor(kind, name)
	if m.showHidden {
		return m, m.setNotice("Showing hidden and stopped hosts")
	}
	return m, m.setNotice("Hidden and stopped hosts concealed")
}

func (m *App) actionToggleGroup() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		return m.updateSyncKeys("space")
	}
	if m.section != hostsSection {
		return m, nil
	}
	if m.historySuggestionsHeaderSelected() {
		return m, m.toggleHistorySuggestions()
	}
	return m, m.toggleSelectedGroup()
}

func (m *App) actionEdit() (tea.Model, tea.Cmd) {
	if m.section == hostsSection {
		if _, ok := m.selectedEntry(); ok {
			m.openEditEntryForm()
			return m, nil
		}
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
		return m, nil
	}
	if m.section == keysSection {
		m.openEditKeyForm()
	}
	return m, nil
}

func (m *App) actionDelete() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		if m.noSync {
			return m, m.syncOffNotice()
		}
		return m.updateSyncKeys("d")
	}
	if m.section == hostsSection {
		if _, ok := m.selectedEntry(); ok {
			m.openDeleteEntryForm()
			return m, nil
		}
		if host, ok := m.selectedHost(); ok {
			if m.noSync && m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Delete }) {
				return m, m.syncOffNotice()
			}
			if m.deleteSyncedHost(host) {
				return m, nil
			}
		}
		m.openDeleteHostForm()
		return m, nil
	}
	if m.section == keysSection {
		m.openDeleteKeyForm()
	}
	if m.section == filesSection {
		m.initFilesState()
		return m.openFilesDeleteForm()
	}
	return m, nil
}

func (m *App) actionStopSandbox() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		if m.noSync {
			return m, m.syncOffNotice()
		}
		return m.updateSyncKeys("o")
	}
	if m.section == hostsSection {
		if host, ok := m.selectedHost(); ok {
			if m.noSync && m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Stop }) {
				return m, m.syncOffNotice()
			}
			return m.stopSyncedHost(host)
		}
	}
	return m, nil
}

func (m *App) actionNewOrFork() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		if m.noSync {
			return m, m.syncOffNotice()
		}
		return m.updateSyncKeys("n")
	}
	if m.section == hostsSection {
		if kind, ok := m.selectedProviderRoot(); ok && cloud.CapabilitiesFor(kind).Create {
			if m.noSync {
				return m, m.syncOffNotice()
			}
			return m.runProviderGroupCreate(kind)
		}
		if host, ok := m.selectedHost(); ok {
			if m.noSync && m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Fork }) {
				return m, m.syncOffNotice()
			}
			return m.forkSyncedHost(host)
		}
	}
	return m, nil
}

func (m *App) actionFavorite() (tea.Model, tea.Cmd) {
	if m.section != hostsSection {
		return m, nil
	}
	if entry, ok := m.selectedEntry(); ok {
		_, err := m.metadata.ToggleEntryFavorite(entry.ID)
		if err != nil {
			m.setError(err)
			return m, nil
		}
		return m, nil
	}
	host, ok := m.selectedHost()
	if !ok {
		return m, nil
	}
	if host.Synced && (host.SyncSource == "boat" || host.SyncSource == "upstash" || host.SyncSource == "vercel") {
		return m, m.setNotice("Synced sandbox hosts are read-only")
	}
	_, err := m.metadata.ToggleFavorite(host.Alias)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	m.sortHosts()
	m.cursor = 0
	return m, nil
}

func (m *App) actionHideHost() (tea.Model, tea.Cmd) {
	if m.section != hostsSection {
		return m, nil
	}
	if entry, ok := m.selectedEntry(); ok {
		hidden, err := m.metadata.ToggleEntryHidden(entry.ID)
		if err != nil {
			m.setError(err)
			return m, nil
		}
		notice := "Entry hidden"
		if !hidden {
			notice = "Entry shown"
		}
		m.clampCursor()
		return m, m.setNotice(notice)
	}
	host, ok := m.selectedHost()
	if !ok {
		return m, nil
	}
	if host.Synced && (host.SyncSource == "boat" || host.SyncSource == "upstash" || host.SyncSource == "vercel") {
		return m, m.setNotice("Synced sandbox hosts are read-only")
	}
	hidden, err := m.metadata.ToggleHidden(host.Alias)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	m.refreshHostMetadata()
	notice := "Host hidden"
	if !hidden {
		notice = "Host shown"
	}
	m.clampCursor()
	return m, m.setNotice(notice)
}

func (m *App) actionHostsEnter() (tea.Model, tea.Cmd) {
	if m.historySuggestionsHeaderSelected() {
		return m, m.toggleHistorySuggestions()
	}
	if _, ok := m.selectedHistorySuggestion(); ok {
		return m.importSelectedHistorySuggestion()
	}
	if _, groupSelected := m.selectedGroupHeader(); groupSelected {
		return m, m.toggleSelectedGroup()
	}
	if _, ok := m.selectedEntry(); ok {
		return m.openSelectedEntry()
	}
	if _, ok := m.selectedForward(); ok || m.forwardHeaderSelected() {
		return m, nil
	}
	return m.connectSelected()
}

func (m *App) actionSortOrSync() (tea.Model, tea.Cmd) {
	if m.section == syncSection {
		if m.noSync {
			return m, m.syncOffNotice()
		}
		return m.updateSyncKeys("s")
	}
	if m.section != hostsSection {
		return m, nil
	}
	if kind, ok := m.selectedProviderRoot(); ok {
		if m.noSync {
			return m, m.syncOffNotice()
		}
		return m.syncProviderFromHosts(kind)
	}
	return m, m.cycleSort()
}

func (m *App) actionCopyPublicKey() (tea.Model, tea.Cmd) {
	selected, ok := m.selectedKey()
	if !ok {
		return m, nil
	}
	public, err := m.keyring.PublicText(selected)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	return m, tea.Batch(tea.SetClipboard(public), m.setNotice("Public key copied"))
}

func (m *App) actionKeysPassphrase() (tea.Model, tea.Cmd) {
	key, ok := m.selectedKey()
	if !ok {
		return m, nil
	}
	if !key.Managed {
		return m, m.promoteSelectedKey()
	}
	return m.runPassphraseAction()
}

func (m *App) actionFilesEscape() (tea.Model, tea.Cmd) {
	m.initFilesState()
	pane := m.filesFocusedPane()
	if len(pane.marked) > 0 || pane.rangeAnchor != nil {
		pane.clearMarks()
		return m, m.setNotice("Cleared selection")
	}
	if pane.connected() {
		return m, m.disconnectFilesPane(m.files.focus)
	}
	m.clearFilesOverlays()
	m.section, m.cursor, m.search = hostsSection, 0, ""
	return m, nil
}

func (m *App) filesOverlay() bool {
	if m.section != filesSection {
		return false
	}
	m.initFilesState()
	if m.files.chmod.active || m.files.info || m.files.preview.active || m.files.transfer.active || m.files.jump.active {
		return true
	}
	pane := m.filesFocusedPane()
	if pane.connecting {
		return true
	}
	if pane.pickingHost() && strings.HasPrefix(pane.hostSearch, "\x00") {
		return true
	}
	return false
}
