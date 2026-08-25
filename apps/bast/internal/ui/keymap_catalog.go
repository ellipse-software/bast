package ui

import (
	"strings"

	"bast/internal/cloud"
	"bast/internal/cloud/sync"
	"bast/internal/sshconfig"
)

func buildCatalog() []Binding {
	return []Binding{
		// Help overlay (content describes browse; these keys operate the overlay).
		bind(ActionCreditsOpen, []string{"v"}, ScopeHelp, "About", "").noHelp(),
		bind(ActionDoctorOpen, []string{"D"}, ScopeHelp, "Doctor", "").noHelp(),
		bind(ActionQuit, []string{"q"}, ScopeHelp, "Quit", "").noHelp(),
		bind(ActionHelpClose, []string{"?", "esc", "backspace", "ctrl+h"}, ScopeHelp, "Close", "close").chord("? / Esc / ⌫").withFooter(20).noHelp(),
		bind(ActionHelpScrollUp, []string{"up", "k"}, ScopeHelp, "Scroll up", "").noHelp(),
		bind(ActionHelpScrollDown, []string{"down", "j"}, ScopeHelp, "Scroll down", "").noHelp(),
		bind(ActionHelpPageUp, []string{"pgup"}, ScopeHelp, "Page up", "").noHelp(),
		bind(ActionHelpPageDown, []string{"pgdown"}, ScopeHelp, "Page down", "").noHelp(),
		bind(ActionHelpTop, []string{"g", "home"}, ScopeHelp, "Top", "").noHelp(),
		bind(ActionHelpBottom, []string{"G", "end"}, ScopeHelp, "Bottom", "").noHelp(),
		bind(ActionHelpScrollDown, []string{"up", "down"}, ScopeHelp, "Scroll", "scroll").chord("↑/↓").withFooter(10).when(func(m *App) bool { return m.helpCanScroll() }).noHelp().skip(),

		// Credits.
		bind(ActionHelpToggle, []string{"?"}, ScopeCredits, "Help", "").noHelp(),
		bind(ActionDoctorOpen, []string{"D"}, ScopeCredits, "Doctor", "").noHelp(),
		bind(ActionQuit, []string{"q"}, ScopeCredits, "Quit", "").noHelp(),
		bind(ActionCreditsClose, []string{"v", "esc", "backspace", "ctrl+h"}, ScopeCredits, "Close", "close").chord("v / Esc / ⌫").withFooter(10).noHelp(),

		// Doctor overlay.
		bind(ActionHelpToggle, []string{"?"}, ScopeDoctor, "Help", "").noHelp(),
		bind(ActionCreditsOpen, []string{"v"}, ScopeDoctor, "About", "").noHelp(),
		bind(ActionQuit, []string{"q"}, ScopeDoctor, "Quit", "").noHelp(),
		bind(ActionDoctorClose, []string{"D", "esc", "backspace", "ctrl+h"}, ScopeDoctor, "Close", "close").chord("D / Esc / ⌫").withFooter(20).noHelp(),
		bind(ActionDoctorScrollUp, []string{"up", "k"}, ScopeDoctor, "Scroll up", "").noHelp(),
		bind(ActionDoctorScrollDown, []string{"down", "j"}, ScopeDoctor, "Scroll down", "").noHelp(),
		bind(ActionDoctorPageUp, []string{"pgup"}, ScopeDoctor, "Page up", "").noHelp(),
		bind(ActionDoctorPageDown, []string{"pgdown"}, ScopeDoctor, "Page down", "").noHelp(),
		bind(ActionDoctorTop, []string{"g", "home"}, ScopeDoctor, "Top", "").noHelp(),
		bind(ActionDoctorBottom, []string{"G", "end"}, ScopeDoctor, "Bottom", "").noHelp(),
		bind(ActionDoctorScrollDown, []string{"up", "down"}, ScopeDoctor, "Scroll", "scroll").chord("↑/↓").withFooter(10).when(func(m *App) bool { return m.doctorCanScroll() }).noHelp().skip(),

		// Global browse.
		bind(ActionQuit, []string{"ctrl+c"}, ScopeGlobal, "Quit", "").noHelp(),
		bind(ActionQuit, []string{"q"}, ScopeGlobal, "Quit", ""),
		bind(ActionQuit, []string{"esc", "backspace", "ctrl+h"}, ScopeGlobal, "Quit", "").noHelp(),
		bind(ActionHelpToggle, []string{"?"}, ScopeGlobal, "Help", "").withFooter(1000),
		bind(ActionDoctorOpen, []string{"D"}, ScopeGlobal, "Doctor", "").when(func(m *App) bool { return m.section != filesSection }),
		bind(ActionTabHosts, []string{"1"}, ScopeGlobal, "Hosts", "").noHelp(),
		bind(ActionTabKeys, []string{"2"}, ScopeGlobal, "Keys", "").noHelp(),
		bind(ActionTabVault, []string{"3"}, ScopeGlobal, "Vault", "").noHelp(),
		bind(ActionTabSync, []string{"4"}, ScopeGlobal, "Sync", "").noHelp(),
		bind(ActionTabFiles, []string{"5"}, ScopeGlobal, "Files", "").noHelp(),
		bind(ActionCreditsOpen, []string{"v"}, ScopeGlobal, "About", "").when(func(m *App) bool { return m.section != filesSection }),
		bind(ActionMoveUp, []string{"up", "k"}, ScopeGlobal, "Move up", "").noHelp(),
		bind(ActionMoveDown, []string{"down", "j"}, ScopeGlobal, "Move down", "").noHelp(),
		bind(ActionJumpTop, []string{"g", "home"}, ScopeGlobal, "Top", ""),
		bind(ActionJumpBottom, []string{"G", "end"}, ScopeGlobal, "Bottom", ""),
		bind(ActionSearch, []string{"/"}, ScopeGlobal, "Search", "").when(func(m *App) bool {
			return m.section != syncSection && m.section != vaultSection && m.section != filesSection
		}),
		bind(ActionReload, []string{"r"}, ScopeGlobal, "Reload", ""),
		bind(ActionNone, nil, ScopeGlobal, "Move", "").chord("j k  ↑ ↓").helpOnly(),
		bind(ActionNone, nil, ScopeGlobal, "Tabs", "").chord("1 … 5").helpOnly(),

		// Hosts.
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Connect or add suggestion", ""),
		bind(ActionAddHost, []string{"a"}, ScopeHosts, "Add host", "add"),
		bind(ActionEdit, []string{"e"}, ScopeHosts, "Edit or review suggestion", "edit"),
		bind(ActionAssignGroup, []string{"m"}, ScopeHosts, "Move host to group", ""),
		bind(ActionDismissHistory, []string{"x"}, ScopeHosts, "Dismiss history suggestion", "dismiss"),
		bind(ActionDelete, []string{"d"}, ScopeHosts, "Delete host", ""),
		bind(ActionPromote, []string{"p"}, ScopeHosts, "Promote external host", ""),
		bind(ActionToggleGroup, []string{"space"}, ScopeHosts, "Collapse or expand group", "").chord("␣"),
		bind(ActionCollapseAll, []string{"["}, ScopeHosts, "Collapse all groups", ""),
		bind(ActionExpandAll, []string{"]"}, ScopeHosts, "Expand all groups", ""),
		bind(ActionSortOrSync, []string{"s"}, ScopeHosts, "Cycle sort, or sync provider group", "sync"),
		bind(ActionFavorite, []string{"f"}, ScopeHosts, "Toggle favorite", ""),
		bind(ActionOpenFiles, []string{"F"}, ScopeHosts, "Open Files for host", "files"),
		bind(ActionHideHost, []string{"h"}, ScopeHosts, "Hide or show selected", ""),
		bind(ActionToggleHidden, []string{"."}, ScopeHosts, "Toggle hidden and stopped hosts", "show hidden"),
		bind(ActionNewOrFork, []string{"n"}, ScopeHosts, "Fork sandbox, or new VM on a provider group", "new"),
		bind(ActionStopSandbox, []string{"o"}, ScopeHosts, "Stop or pause sandbox", "stop"),
		bind(ActionKnownHosts, []string{"K"}, ScopeHosts, "Remove known-host entry", ""),

		// Hosts footer variants.
		bind(ActionToggleHidden, []string{"."}, ScopeHosts, "Show hidden", "show hidden").withFooter(10).when(hostsFooterEmptyHidden).noHelp(),
		bind(ActionAddHost, []string{"a"}, ScopeHosts, "Add host", "add").withFooter(20).when(hostsFooterEmpty).noHelp(),
		bind(ActionToggleGroup, []string{"space"}, ScopeHosts, "Collapse", "").chord("␣").hintOf(func(m *App) string {
			if m.historySuggestionsCollapsed && m.searchText() == "" {
				return "expand"
			}
			return "collapse"
		}).withFooter(10).when(func(m *App) bool { return m.historySuggestionsHeaderSelected() }).noHelp(),
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Add", "add").withFooter(10).when(func(m *App) bool {
			_, ok := m.selectedHistorySuggestion()
			return ok
		}).noHelp(),
		bind(ActionEdit, []string{"e"}, ScopeHosts, "Review", "review").withFooter(20).when(func(m *App) bool {
			_, ok := m.selectedHistorySuggestion()
			return ok
		}).noHelp(),
		bind(ActionDismissHistory, []string{"x"}, ScopeHosts, "Dismiss", "dismiss").withFooter(30).when(func(m *App) bool {
			_, ok := m.selectedHistorySuggestion()
			return ok
		}).noHelp(),
		bind(ActionToggleGroup, []string{"space"}, ScopeHosts, "Collapse", "").chord("␣").hintOf(func(m *App) string { return m.collapseActionLabel() }).withFooter(10).when(hostsFooterGroup).noHelp(),
		bind(ActionNewOrFork, []string{"n"}, ScopeHosts, "New", "new").withFooter(20).when(hostsFooterProviderCreate).noHelp(),
		bind(ActionSortOrSync, []string{"s"}, ScopeHosts, "Sync", "sync").withFooter(30).when(hostsFooterProvider).noHelp(),
		bind(ActionToggleHidden, []string{"."}, ScopeHosts, "Show stopped", "show stopped").withFooter(40).when(hostsFooterShowStopped).noHelp(),
		bind(ActionEdit, []string{"e"}, ScopeHosts, "Rename", "rename").withFooter(20).when(hostsFooterGroupRename).noHelp(),
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Connect", "connect").withFooter(10).when(hostsFooterSandbox).noHelp(),
		bind(ActionStopSandbox, []string{"o"}, ScopeHosts, "Stop", "stop").withFooter(20).when(hostsFooterSandboxRunning).noHelp(),
		bind(ActionReload, []string{"r"}, ScopeHosts, "Resume", "resume").withFooter(20).when(hostsFooterSandboxStopped).noHelp(),
		bind(ActionNewOrFork, []string{"n"}, ScopeHosts, "Fork", "fork").withFooter(30).when(hostsFooterSandbox).noHelp(),
		bind(ActionDelete, []string{"d"}, ScopeHosts, "Delete", "delete").withFooter(40).when(hostsFooterSandboxDelete).noHelp(),
		bind(ActionOpenFiles, []string{"F"}, ScopeHosts, "Files", "files").withFooter(50).when(hostsFooterSandboxDesktop).noHelp(),
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Connect", "connect").withFooter(10).when(hostsFooterSyncedMobile).noHelp(),
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Connect", "").withFooter(10).when(hostsFooterSyncedDesktop).noHelp(),
		bind(ActionOpenFiles, []string{"F"}, ScopeHosts, "Files", "files").withFooter(20).when(hostsFooterSyncedDesktop).noHelp(),
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Connect", "connect").withFooter(10).when(hostsFooterLocalMobile).noHelp(),
		bind(ActionEdit, []string{"e"}, ScopeHosts, "Edit", "edit").withFooter(20).when(hostsFooterLocalMobile).noHelp(),
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Connect", "").withFooter(10).when(hostsFooterLocalDesktop).noHelp(),
		bind(ActionEdit, []string{"e"}, ScopeHosts, "Edit", "edit").withFooter(20).when(hostsFooterLocalDesktop).noHelp(),
		bind(ActionOpenFiles, []string{"F"}, ScopeHosts, "Files", "files").withFooter(30).when(hostsFooterLocalDesktop).noHelp(),

		// Keys.
		bind(ActionGenerateKey, []string{"g"}, ScopeKeys, "Generate key", "generate"),
		bind(ActionImportKey, []string{"i"}, ScopeKeys, "Import key", "import"),
		bind(ActionInstallKey, []string{"a"}, ScopeKeys, "Add to server", "add"),
		bind(ActionEdit, []string{"e"}, ScopeKeys, "Edit comment", "edit"),
		bind(ActionDelete, []string{"d"}, ScopeKeys, "Delete key", ""),
		bind(ActionExportKey, []string{"x"}, ScopeKeys, "Export key", ""),
		bind(ActionKeysPassphrase, []string{"p"}, ScopeKeys, "Promote external / change passphrase", ""),
		bind(ActionCopyPublicKey, []string{"c"}, ScopeKeys, "Copy public key", "copy"),
		bind(ActionGenerateKey, []string{"g"}, ScopeKeys, "Generate", "generate").withFooter(10).when(keysFooterEmpty).noHelp(),
		bind(ActionImportKey, []string{"i"}, ScopeKeys, "Import", "import").withFooter(20).when(keysFooterEmpty).noHelp(),
		bind(ActionInstallKey, []string{"a"}, ScopeKeys, "Add", "add").withFooter(10).when(keysFooterSelected).noHelp(),
		bind(ActionCopyPublicKey, []string{"c"}, ScopeKeys, "Copy", "copy").withFooter(20).when(keysFooterSelected).noHelp(),
		bind(ActionEdit, []string{"e"}, ScopeKeys, "Edit", "edit").withFooter(30).when(keysFooterSelected).noHelp(),

		// Vault.
		bind(ActionVaultEnter, []string{"enter"}, ScopeVault, "Link, unlock, or sync", ""),
		bind(ActionNone, nil, ScopeVault, "Secondary actions", "").chord("j k").helpOnly(),
		bind(ActionVaultSync, []string{"r"}, ScopeVault, "Sync now when unlocked", ""),
		bind(ActionVaultEnter, []string{"enter"}, ScopeVault, "Enter", "").withFooter(10).when(func(m *App) bool { return m.syncCursor >= 0 }).noHelp(),
		bind(ActionVaultEnter, []string{"enter"}, ScopeVault, "Link", "link").withFooter(10).when(func(m *App) bool {
			return m.syncCursor < 0 && !m.vaultLinked()
		}).noHelp(),
		bind(ActionVaultEnter, []string{"enter"}, ScopeVault, "Unlock", "unlock").withFooter(10).when(func(m *App) bool {
			return m.syncCursor < 0 && m.vaultLinked() && m.vaultPassphrase == ""
		}).noHelp(),
		bind(ActionVaultEnter, []string{"enter"}, ScopeVault, "Sync", "sync").withFooter(10).when(func(m *App) bool {
			return m.syncCursor < 0 && m.vaultLinked() && m.vaultPassphrase != ""
		}).noHelp(),

		// Sync.
		bind(ActionMoveLeft, []string{"left", "h"}, ScopeSync, "Move left", "").noHelp(),
		bind(ActionMoveRight, []string{"right", "l"}, ScopeSync, "Move right", "").noHelp(),
		bind(ActionSyncEnter, []string{"enter"}, ScopeSync, "Open provider, run action, or connect", ""),
		bind(ActionToggleGroup, []string{"space"}, ScopeSync, "Collapse or expand status group", "").chord("␣"),
		bind(ActionSyncNow, []string{"s"}, ScopeSync, "Sync", "sync"),
		bind(ActionNewOrFork, []string{"n"}, ScopeSync, "New box, or fork selected sandbox", "fork"),
		bind(ActionStopSandbox, []string{"o"}, ScopeSync, "Stop or pause selected sandbox", "stop"),
		bind(ActionDelete, []string{"d"}, ScopeSync, "Delete selected Upstash box", "delete"),
		bind(ActionSyncBack, []string{"esc", "backspace", "ctrl+h"}, ScopeSync, "Back", "back").when(func(m *App) bool { return m.syncProvider != "" }),
		bind(ActionReload, []string{"r"}, ScopeSync, "Resume selected sandbox, or refresh status", "resume"),
		bind(ActionNone, nil, ScopeSync, "Grid move, or cycle actions", "").chord("h j k l").helpOnly(),
		bind(ActionMoveLeft, []string{"h", "j", "k", "l"}, ScopeSync, "Move", "move").chord("hjkl").withFooter(10).when(func(m *App) bool { return m.syncProvider == "" }).noHelp().skip(),
		bind(ActionSyncEnter, []string{"enter"}, ScopeSync, "Open", "open").withFooter(20).when(func(m *App) bool { return m.syncProvider == "" }).noHelp(),
		bind(ActionSyncNow, []string{"s"}, ScopeSync, "Sync", "sync").withFooter(30).when(func(m *App) bool { return m.syncProvider == "" }).noHelp(),
		bind(ActionMoveLeft, []string{"h", "l"}, ScopeSync, "Cycle", "cycle").chord("h/l").withFooter(10).when(syncFooterCycle).noHelp().skip(),
		bind(ActionSyncEnter, []string{"enter"}, ScopeSync, "Enter", "").hintOf(syncFooterEnterHint).withFooter(20).when(syncFooterLifecycle).noHelp(),
		bind(ActionToggleGroup, []string{"space"}, ScopeSync, "Collapse", "").chord("␣").hintOf(syncFooterInvHint).withFooter(10).when(syncFooterInvHeader).noHelp(),
		bind(ActionSyncEnter, []string{"enter"}, ScopeSync, "Connect", "connect").withFooter(10).when(syncFooterInvHost).noHelp(),
		bind(ActionSyncEnter, []string{"enter"}, ScopeSync, "Connect", "connect").withFooter(10).when(syncFooterInvSandbox).noHelp(),
		bind(ActionStopSandbox, []string{"o"}, ScopeSync, "Stop", "stop").withFooter(20).when(syncFooterInvSandboxRunning).noHelp(),
		bind(ActionReload, []string{"r"}, ScopeSync, "Resume", "resume").withFooter(20).when(syncFooterInvSandboxStopped).noHelp(),
		bind(ActionNewOrFork, []string{"n"}, ScopeSync, "Fork", "fork").withFooter(30).when(syncFooterInvSandbox).noHelp(),
		bind(ActionDelete, []string{"d"}, ScopeSync, "Delete", "delete").withFooter(40).when(syncFooterInvSandboxDelete).noHelp(),
		bind(ActionSyncEnter, []string{"enter"}, ScopeSync, "Enter", "").withFooter(10).when(syncFooterConfig).noHelp(),
		bind(ActionSyncBack, []string{"esc"}, ScopeSync, "Back", "back").withFooter(90).when(func(m *App) bool { return m.syncProvider != "" }).noHelp(),

		// Files.
		bind(ActionFilesTab, []string{"tab"}, ScopeFiles, "Switch pane", "").chord("Tab"),
		bind(ActionFilesSwap, []string{"w"}, ScopeFiles, "Swap panes", ""),
		bind(ActionFilesLocal, []string{"L"}, ScopeFiles, "Pane local", "").noHelp(),
		bind(ActionFilesRemote, []string{"R"}, ScopeFiles, "Pane remote", "").noHelp(),
		bind(ActionNone, nil, ScopeFiles, "Pane local / remote", "").chord("L  R").helpOnly(),
		bind(ActionFilesParent, []string{"h", "backspace", "ctrl+h"}, ScopeFiles, "Parent", "").noHelp(),
		bind(ActionFilesEnter, []string{"l"}, ScopeFiles, "Enter", "").noHelp(),
		bind(ActionNone, nil, ScopeFiles, "Parent / enter", "").chord("h  l").helpOnly(),
		bind(ActionFilesEnter, []string{"enter"}, ScopeFiles, "Enter dir or connect host", ""),
		bind(ActionNone, nil, ScopeFiles, "Move / top / bottom", "").chord("j k  g G").helpOnly(),
		bind(ActionFilesJump, []string{"f"}, ScopeFiles, "Fuzzy jump", "jump"),
		bind(ActionFilesPath, []string{"/"}, ScopeFiles, "Path jump or host search", ""),
		bind(ActionFilesMark, []string{"space"}, ScopeFiles, "Toggle mark", "").chord("␣"),
		bind(ActionFilesRange, []string{"v"}, ScopeFiles, "Range mark", ""),
		bind(ActionFilesCopy, []string{"c"}, ScopeFiles, "Copy to other pane", "copy").noHelp(),
		bind(ActionFilesMove, []string{"m"}, ScopeFiles, "Move to other pane", "move").noHelp(),
		bind(ActionNone, nil, ScopeFiles, "Copy / move to other pane", "").chord("c  m").helpOnly(),
		bind(ActionFilesDelete, []string{"d"}, ScopeFiles, "Delete", ""),
		bind(ActionFilesMkdir, []string{"a"}, ScopeFiles, "New directory", ""),
		bind(ActionFilesRename, []string{"r"}, ScopeFiles, "Rename", ""),
		bind(ActionFilesInfo, []string{"i"}, ScopeFiles, "File info", ""),
		bind(ActionFilesChmod, []string{"p"}, ScopeFiles, "Permissions (chmod)", ""),
		bind(ActionFilesShell, []string{"t"}, ScopeFiles, "Shell in directory", ""),
		bind(ActionFilesHidden, []string{"."}, ScopeFiles, "Toggle hidden files", ""),
		bind(ActionFilesDisconnect, []string{"D"}, ScopeFiles, "Disconnect remote", ""),
		bind(ActionFilesEscape, []string{"esc"}, ScopeFiles, "Clear marks / disconnect / Hosts", "back"),
		bind(ActionNone, nil, ScopeFiles, "Cancel transfer or connect", "").chord("Esc  x").helpOnly(),
		bind(ActionFilesTab, []string{"tab"}, ScopeFiles, "Tab", "").withFooter(10).when(filesFooterBrowse).noHelp(),
		bind(ActionFilesCopy, []string{"c"}, ScopeFiles, "Copy", "copy").withFooter(20).when(filesFooterBrowse).noHelp(),
		bind(ActionFilesMove, []string{"m"}, ScopeFiles, "Move", "move").withFooter(30).when(filesFooterBrowse).noHelp(),
		bind(ActionFilesJump, []string{"f"}, ScopeFiles, "Jump", "jump").withFooter(40).when(filesFooterBrowse).noHelp(),
		bind(ActionFilesEscape, []string{"esc"}, ScopeFiles, "Back", "back").withFooter(90).when(filesFooterBrowse).noHelp(),
		bind(ActionFilesEnter, []string{"enter"}, ScopeFiles, "Connect", "connect").withFooter(10).when(filesFooterPickHost).noHelp(),
		bind(ActionFilesPath, []string{"/"}, ScopeFiles, "Search", "search").withFooter(20).when(filesFooterPickHost).noHelp(),
		bind(ActionFilesEscape, []string{"esc"}, ScopeFiles, "Back", "back").withFooter(90).when(filesFooterPickHost).noHelp(),

		// SSH session notes (help only).
		bind(ActionNone, nil, ScopeDuringSSH, "Return to Bast", "").chord("exit").helpOnly(),
		bind(ActionNone, nil, ScopeDuringSSH, "Force-close a stuck session", "").chord("󰌑 then ~.").helpOnly(),
	}
}

func hostsFooterEmpty(m *App) bool {
	return m.itemCount() == 0
}

func hostsFooterEmptyHidden(m *App) bool {
	return m.itemCount() == 0 && m.hasHiddenHosts() && !m.showHidden
}

func hostsFooterGroup(m *App) bool {
	if m.itemCount() == 0 || m.historySuggestionsHeaderSelected() {
		return false
	}
	if _, ok := m.selectedHistorySuggestion(); ok {
		return false
	}
	_, ok := m.selectedGroupHeader()
	return ok
}

func hostsFooterProvider(m *App) bool {
	if !hostsFooterGroup(m) {
		return false
	}
	_, ok := m.selectedProviderRoot()
	return ok
}

func hostsFooterProviderCreate(m *App) bool {
	kind, ok := m.selectedProviderRoot()
	return ok && hostsFooterGroup(m) && cloud.CapabilitiesFor(kind).Create
}

func hostsFooterShowStopped(m *App) bool {
	group, ok := m.selectedGroupHeader()
	if !ok || !hostsFooterProvider(m) {
		return false
	}
	kind, ok := m.selectedProviderRoot()
	if !ok || !cloud.CapabilitiesFor(kind).Stop || m.showHidden {
		return false
	}
	_, stopped := m.providerGroupStats(group)
	return stopped > 0
}

func hostsFooterGroupRename(m *App) bool {
	group, ok := m.selectedGroupHeader()
	return ok && hostsFooterGroup(m) && !hostsFooterProvider(m) && !sync.IsSyncedGroup(group)
}

func selectedSandboxHost(m *App) (sshconfig.Host, bool) {
	host, ok := m.selectedHost()
	if !ok || !host.Synced {
		return host, false
	}
	return host, host.SyncSource == "box" || host.SyncSource == "upstash"
}

func hostsFooterSandbox(m *App) bool {
	_, ok := selectedSandboxHost(m)
	return ok
}

func hostsFooterSandboxRunning(m *App) bool {
	host, ok := selectedSandboxHost(m)
	return ok && !m.hostLooksStopped(host)
}

func hostsFooterSandboxStopped(m *App) bool {
	host, ok := selectedSandboxHost(m)
	return ok && m.hostLooksStopped(host)
}

func hostsFooterSandboxDelete(m *App) bool {
	host, ok := selectedSandboxHost(m)
	return ok && host.SyncSource == "upstash"
}

func hostsFooterSandboxDesktop(m *App) bool {
	return hostsFooterSandbox(m) && !m.isMobileLayout()
}

func hostsFooterSyncedMobile(m *App) bool {
	host, ok := m.selectedHost()
	return ok && host.Synced && !hostsFooterSandbox(m) && m.isMobileLayout()
}

func hostsFooterSyncedDesktop(m *App) bool {
	host, ok := m.selectedHost()
	return ok && host.Synced && !hostsFooterSandbox(m) && !m.isMobileLayout()
}

func hostsFooterLocalMobile(m *App) bool {
	host, ok := m.selectedHost()
	return ok && !host.Synced && m.isMobileLayout()
}

func hostsFooterLocalDesktop(m *App) bool {
	host, ok := m.selectedHost()
	return ok && !host.Synced && !m.isMobileLayout()
}

func keysFooterEmpty(m *App) bool {
	if len(m.filteredKeys()) == 0 {
		return true
	}
	key, ok := m.selectedKey()
	if !ok {
		return true
	}
	return key.PublicPath == "" && key.PrivatePath == ""
}

func keysFooterSelected(m *App) bool {
	return !keysFooterEmpty(m)
}

func syncFooterLifecycle(m *App) bool {
	if m.syncProvider == "" {
		return false
	}
	life, _ := m.providerActionLayout()
	return m.syncCursor < len(life)
}

func syncFooterCycle(m *App) bool {
	if !syncFooterLifecycle(m) {
		return false
	}
	life, _ := m.providerActionLayout()
	return len(life) > 1
}

func syncFooterEnterHint(m *App) string {
	life, _ := m.providerActionLayout()
	if m.syncCursor >= 0 && m.syncCursor < len(life) {
		switch life[m.syncCursor].action {
		case "sync":
			return "sync"
		case "enable":
			return "connect"
		}
	}
	return ""
}

func syncInvIndex(m *App) (row hostRow, ok bool) {
	if m.syncProvider == "" {
		return hostRow{}, false
	}
	life, _ := m.providerActionLayout()
	inv := m.providerInventoryRows()
	if m.syncCursor < len(life) || m.syncCursor >= len(life)+len(inv) {
		return hostRow{}, false
	}
	return inv[m.syncCursor-len(life)], true
}

func syncFooterInvHeader(m *App) bool {
	row, ok := syncInvIndex(m)
	return ok && row.header
}

func syncFooterInvHint(m *App) string {
	row, ok := syncInvIndex(m)
	if !ok {
		return "collapse"
	}
	if m.providerInvCollapsed(row.group) {
		return "expand"
	}
	return "collapse"
}

func syncFooterInvHost(m *App) bool {
	row, ok := syncInvIndex(m)
	if !ok || row.header {
		return false
	}
	return !m.hostHasCapability(row.host, func(c cloud.Capabilities) bool {
		return c.Stop || c.Start || c.Fork || c.Delete
	})
}

func syncFooterInvSandbox(m *App) bool {
	row, ok := syncInvIndex(m)
	if !ok || row.header {
		return false
	}
	return m.hostHasCapability(row.host, func(c cloud.Capabilities) bool {
		return c.Stop || c.Start || c.Fork || c.Delete
	})
}

func syncFooterInvSandboxRunning(m *App) bool {
	row, ok := syncInvIndex(m)
	return ok && syncFooterInvSandbox(m) && !m.hostLooksStopped(row.host)
}

func syncFooterInvSandboxStopped(m *App) bool {
	row, ok := syncInvIndex(m)
	return ok && syncFooterInvSandbox(m) && m.hostLooksStopped(row.host)
}

func syncFooterInvSandboxDelete(m *App) bool {
	row, ok := syncInvIndex(m)
	return ok && syncFooterInvSandbox(m) && row.host.SyncSource == "upstash"
}

func syncFooterConfig(m *App) bool {
	if m.syncProvider == "" {
		return false
	}
	life, _ := m.providerActionLayout()
	inv := m.providerInventoryRows()
	return m.syncCursor >= len(life)+len(inv)
}

func filesFooterBrowse(m *App) bool {
	if m.section != filesSection || !m.files.ready {
		return false
	}
	if m.files.chmod.active || m.files.info || m.files.transfer.active || m.files.jump.active {
		return false
	}
	pane := m.filesFocusedPane()
	return !pane.connecting && !pane.pickingHost() && !pane.pathEdit
}

func filesFooterPickHost(m *App) bool {
	if m.section != filesSection || !m.files.ready {
		return false
	}
	if m.files.chmod.active || m.files.info || m.files.transfer.active || m.files.jump.active {
		return false
	}
	pane := m.filesFocusedPane()
	return pane.pickingHost() && !strings.HasPrefix(pane.hostSearch, "\x00")
}
