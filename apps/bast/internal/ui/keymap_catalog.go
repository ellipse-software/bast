package ui

import (
	"bast/internal/cloud"
	"bast/internal/cloud/sync"
	"bast/internal/platform"
)

func buildCatalog() []Binding {
	return []Binding{
		// Credits.
		bind(ActionQuit, []string{"q"}, ScopeCredits, "Quit", "quit").app(),
		bind(ActionDoctorOpen, []string{"D"}, ScopeCredits, "Doctor", "doctor").app(),
		bind(ActionSponsorOpen, []string{"s"}, ScopeCredits, "Sponsor", "sponsor").app(),
		bind(ActionCreditsActivate, []string{"enter"}, ScopeCredits, "Open", "").hintOf(creditChoiceLabel),
		bind(ActionCreditsLeft, []string{"left", "h"}, ScopeCredits, "Previous", "previous").nav(),
		bind(ActionCreditsRight, []string{"right", "l"}, ScopeCredits, "Next", "next").nav(),
		bind(ActionOnboardingReplay, []string{"o"}, ScopeCredits, "Intro", "intro").app(),
		bind(ActionCreditsClose, []string{"v", "esc", "backspace", "ctrl+h"}, ScopeCredits, "Close", "close").chord("esc").app(),

		// Doctor.
		bind(ActionQuit, []string{"q"}, ScopeDoctor, "Quit", "quit").app(),
		bind(ActionCreditsOpen, []string{"v"}, ScopeDoctor, "About", "about").app(),
		bind(ActionDoctorClose, []string{"D", "esc", "backspace", "ctrl+h"}, ScopeDoctor, "Close", "close").chord("esc").app(),
		bind(ActionDoctorScrollUp, []string{"up", "k"}, ScopeDoctor, "Scroll up", "up").nav(),
		bind(ActionDoctorScrollDown, []string{"down", "j"}, ScopeDoctor, "Scroll down", "down").nav(),
		bind(ActionDoctorPageUp, []string{"pgup"}, ScopeDoctor, "Page up", "page up").nav(),
		bind(ActionDoctorPageDown, []string{"pgdown"}, ScopeDoctor, "Page down", "page down").nav(),
		bind(ActionDoctorTop, []string{"g", "home"}, ScopeDoctor, "Top", "top").nav(),
		bind(ActionDoctorBottom, []string{"G", "end"}, ScopeDoctor, "Bottom", "bottom").nav(),

		// Global browse. Digits are resolved from the visible tabs in matchBinding.
		bind(ActionQuit, []string{"ctrl+c"}, ScopeGlobal, "Quit", "quit").app().quiet(),
		bind(ActionQuit, []string{"q"}, ScopeGlobal, "Quit", "quit").app(),
		bind(ActionDoctorOpen, []string{"D"}, ScopeGlobal, "Doctor", "doctor").app().available(func(m *App) bool { return m.section != filesSection }),
		bind(ActionCreditsOpen, []string{"v"}, ScopeGlobal, "About", "about").app().available(func(m *App) bool { return m.section != filesSection }),
		bind(ActionTabHosts, []string{"1"}, ScopeGlobal, "Hosts", "").quiet(),
		bind(ActionTabKeys, []string{"2"}, ScopeGlobal, "Keys", "").quiet(),
		bind(ActionTabVault, []string{"3"}, ScopeGlobal, "Vault", "").quiet(),
		bind(ActionTabSync, []string{"4"}, ScopeGlobal, "Sync", "").quiet(),
		bind(ActionTabFiles, []string{"5"}, ScopeGlobal, "Files", "").quiet(),
		bind(ActionMoveUp, []string{"up", "k"}, ScopeGlobal, "Move up", "up").nav(),
		bind(ActionMoveDown, []string{"down", "j"}, ScopeGlobal, "Move down", "down").nav(),
		bind(ActionJumpTop, []string{"g", "home"}, ScopeGlobal, "Top", "top").nav(),
		bind(ActionJumpBottom, []string{"G", "end"}, ScopeGlobal, "Bottom", "bottom").nav(),
		bind(ActionSearch, []string{"ctrl+f"}, ScopeGlobal, "Filter", "filter").app().available(func(m *App) bool {
			return m.section == hostsSection || m.section == keysSection
		}),
		bind(ActionReload, []string{"r"}, ScopeGlobal, "Reload", "").app().hintOf(reloadLabel).available(func(m *App) bool {
			return m.section != filesSection && m.section != vaultSection && m.section != syncSection
		}),

		// Hosts.
		bind(ActionHostsEnter, []string{"enter"}, ScopeHosts, "Connect", "").hintOf(hostsEnterLabel).available(hostsEnterAvailable),
		bind(ActionAddHost, []string{"a"}, ScopeHosts, "Add host", "add"),
		bind(ActionAddEntry, []string{"A"}, ScopeHosts, "Add Remote Desktop or web page", "desktop or web"),
		bind(ActionEdit, []string{"e"}, ScopeHosts, "Edit", "").hintOf(hostsEditLabel).available(hostsEditAvailable),
		bind(ActionAssignGroup, []string{"m"}, ScopeHosts, "Move to group", "move").available(hostsMoveAvailable),
		bind(ActionDismissHistory, []string{"x"}, ScopeHosts, "Dismiss suggestion", "dismiss").available(func(m *App) bool {
			_, ok := m.selectedHistorySuggestion()
			return ok
		}),
		bind(ActionDelete, []string{"d"}, ScopeHosts, "Delete", "delete").available(hostsDeleteAvailable),
		bind(ActionPromote, []string{"p"}, ScopeHosts, "Promote", "promote").available(promoteAvailable),
		bind(ActionToggleGroup, []string{"space"}, ScopeHosts, "Collapse or expand", "").chord("␣").hintOf(hostsGroupLabel).available(hostsGroupAvailable),
		bind(ActionMoveLeft, []string{"left"}, ScopeHosts, "Collapse or parent", "parent").nav(),
		bind(ActionMoveRight, []string{"right"}, ScopeHosts, "Expand", "expand").nav(),
		bind(ActionCollapseAll, []string{"["}, ScopeHosts, "Collapse all", "collapse all").nav(),
		bind(ActionExpandAll, []string{"]"}, ScopeHosts, "Expand all", "expand all").nav(),
		bind(ActionSortOrSync, []string{"s"}, ScopeHosts, "Sort or sync", "").hintOf(sortOrSyncLabel),
		bind(ActionFavorite, []string{"f"}, ScopeHosts, "Favorite", "favorite").available(hostsMutable),
		bind(ActionOpenFiles, []string{"F"}, ScopeHosts, "Files", "files").available(filesForHostAvailable),
		bind(ActionHideHost, []string{"h"}, ScopeHosts, "Hide", "hide").available(hostsMutable),
		bind(ActionToggleHidden, []string{"."}, ScopeHosts, "Hidden hosts", "").hintOf(hiddenLabel),
		bind(ActionNewOrFork, []string{"n"}, ScopeHosts, "New or fork", "").hintOf(newOrForkLabel).available(newOrForkAvailable),
		bind(ActionStopSandbox, []string{"o"}, ScopeHosts, "Stop", "stop").available(stopAvailable),
		bind(ActionRestartHost, []string{"R"}, ScopeHosts, "Restart", "restart").available(restartAvailable),
		bind(ActionKnownHosts, []string{"K"}, ScopeHosts, "Forget known host", "known host").available(func(m *App) bool {
			_, ok := m.selectedHost()
			return ok
		}),
		bind(ActionOpenRDP, []string{"ctrl+o"}, ScopeHosts, "Remote Desktop", "rdp").available(rdpAvailable),
		bind(ActionStartForward, []string{"ctrl+p"}, ScopeHosts, "Forward", "forward").available(hostsFooterForward),
		bind(ActionCloseTunnel, []string{"c"}, ScopeHosts, "Close tunnel", "close").available(hostsFooterCloseTunnel),

		// Keys. g stays free so jump-to-top still works.
		bind(ActionGenerateKey, []string{"n"}, ScopeKeys, "Generate key", "generate"),
		bind(ActionImportKey, []string{"i"}, ScopeKeys, "Import key", "import"),
		bind(ActionInstallKey, []string{"a"}, ScopeKeys, "Add to server", "add").available(keyHasFile),
		bind(ActionEdit, []string{"e"}, ScopeKeys, "Edit comment", "edit").available(keyHasFile),
		bind(ActionDelete, []string{"d"}, ScopeKeys, "Delete key", "delete").available(keyHasFile),
		bind(ActionExportKey, []string{"x"}, ScopeKeys, "Export key", "export").available(keyHasFile),
		bind(ActionKeysPassphrase, []string{"p"}, ScopeKeys, "Passphrase", "").hintOf(keysPassphraseLabel).available(keyHasFile),
		bind(ActionCopyPublicKey, []string{"c"}, ScopeKeys, "Copy public key", "copy").available(keyHasFile),

		// Vault.
		bind(ActionVaultEnter, []string{"enter"}, ScopeVault, "Vault", "").hintOf(vaultEnterLabel),
		bind(ActionVaultSync, []string{"r"}, ScopeVault, "Sync", "sync"),

		// Sync. The pane is a provider list and a settings page. Lifecycle
		// actions for a machine live on the Hosts tab.
		bind(ActionMoveUp, []string{"up", "k"}, ScopeSync, "Move up", "up").nav().quiet(),
		bind(ActionMoveDown, []string{"down", "j"}, ScopeSync, "Move down", "down").nav().quiet(),
		bind(ActionMoveLeft, []string{"left", "h"}, ScopeSync, "Back", "back").nav().available(syncDetailOpen),
		bind(ActionMoveRight, []string{"right", "l"}, ScopeSync, "Open", "").nav().hintOf(syncEnterLabel),
		bind(ActionSyncEnter, []string{"enter"}, ScopeSync, "Open", "").hintOf(syncEnterLabel),
		bind(ActionSyncNow, []string{"s"}, ScopeSync, "Sync", "sync").available(func(m *App) bool { return !m.noSync }),
		bind(ActionNewOrFork, []string{"n"}, ScopeSync, "New", "new").available(syncCanCreate),
		bind(ActionSyncTab, []string{"tab"}, ScopeSync, "Pane", "").hintOf(syncTabLabel).nav(),
		bind(ActionSyncBack, []string{"esc", "backspace", "ctrl+h"}, ScopeSync, "Back", "back").available(syncDetailOpen),
		bind(ActionReload, []string{"r"}, ScopeSync, "Refresh", "refresh").available(func(m *App) bool { return !m.noSync }),

		// Files.
		bind(ActionFilesTab, []string{"tab"}, ScopeFiles, "Switch pane", "pane").nav(),
		bind(ActionFilesSwap, []string{"w"}, ScopeFiles, "Swap panes", "swap").nav(),
		bind(ActionFilesLocal, []string{"L"}, ScopeFiles, "Local pane", "local").nav(),
		bind(ActionFilesRemote, []string{"R"}, ScopeFiles, "Remote pane", "remote").nav(),
		bind(ActionFilesParent, []string{"h", "backspace", "ctrl+h", "left"}, ScopeFiles, "Parent", "").nav().hintOf(filesParentLabel),
		bind(ActionFilesOpenDir, []string{"l", "right"}, ScopeFiles, "Enter", "enter").nav(),
		bind(ActionFilesEnter, []string{"enter"}, ScopeFiles, "Enter", "").hintOf(filesEnterLabel),
		bind(ActionFilesJump, []string{"f"}, ScopeFiles, "Jump", "jump").available(filesCanJump),
		bind(ActionFilesPath, []string{"ctrl+f"}, ScopeFiles, "Path", "").hintOf(filesPathLabel),
		bind(ActionFilesMark, []string{"space"}, ScopeFiles, "Mark", "").chord("␣").hintOf(filesMarkLabel).available(filesMarkAvailable),
		bind(ActionFilesRange, []string{"v"}, ScopeFiles, "Range", "range").available(filesBrowsingDir),
		bind(ActionFilesCopy, []string{"c"}, ScopeFiles, "Copy", "copy").available(filesHasPaths),
		bind(ActionFilesMove, []string{"m"}, ScopeFiles, "Move", "move").available(filesHasPaths),
		bind(ActionFilesDelete, []string{"d"}, ScopeFiles, "Delete", "delete").available(filesHasPaths),
		bind(ActionFilesMkdir, []string{"a"}, ScopeFiles, "New directory", "directory").available(filesBrowsingDir),
		bind(ActionFilesRename, []string{"r"}, ScopeFiles, "Rename", "rename").available(filesHasSelection),
		bind(ActionFilesInfo, []string{"i"}, ScopeFiles, "Info", "info").available(filesHasSelection),
		bind(ActionFilesPreview, []string{"o"}, ScopeFiles, "Preview", "preview").available(filesBrowsingDir),
		bind(ActionFilesChmod, []string{"p"}, ScopeFiles, "Permissions", "permissions").available(filesCanChmod),
		bind(ActionFilesShell, []string{"t"}, ScopeFiles, "Shell", "shell").available(filesCanShell),
		bind(ActionFilesHidden, []string{"."}, ScopeFiles, "Hidden files", "hidden").available(filesBrowsingDir),
		bind(ActionFilesDisconnect, []string{"D"}, ScopeFiles, "Host picker", "picker"),
		bind(ActionFilesEscape, []string{"esc"}, ScopeFiles, "Back", "").hintOf(filesEscapeLabel),
	}
}

func reloadLabel(m *App) string {
	host, ok := m.selectedHost()
	if !ok || m.noSync || !m.hostLooksStopped(host) || !m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Start }) {
		return "reload"
	}
	switch host.SyncSource {
	case "hetzner", "bitqate", "boxd":
		return "start"
	default:
		return "resume"
	}
}

func hostsEnterAvailable(m *App) bool {
	if m.itemCount() == 0 {
		return false
	}
	if _, ok := m.selectedForward(); ok || m.forwardHeaderSelected() {
		return false
	}
	return true
}

func hostsEnterLabel(m *App) string {
	if _, ok := m.selectedHistorySuggestion(); ok {
		return "add"
	}
	if _, ok := m.selectedEntry(); ok {
		return "open"
	}
	if m.historySuggestionsHeaderSelected() {
		return hostsGroupLabel(m)
	}
	if _, ok := m.selectedGroupHeader(); ok {
		return hostsGroupLabel(m)
	}
	return "connect"
}

func hostsEditLabel(m *App) string {
	if _, ok := m.selectedHistorySuggestion(); ok {
		return "review"
	}
	return "edit"
}

func hostsEditAvailable(m *App) bool {
	if _, ok := m.selectedEntry(); ok {
		return true
	}
	if _, ok := m.selectedHistorySuggestion(); ok {
		return true
	}
	if hostsFooterGroupRename(m) {
		return true
	}
	host, ok := m.selectedHost()
	return ok && !host.Synced
}

func hostsMoveAvailable(m *App) bool {
	if _, ok := m.selectedEntry(); ok {
		return true
	}
	host, ok := m.selectedHost()
	return ok && !host.Synced
}

func hostsDeleteAvailable(m *App) bool {
	if _, ok := m.selectedEntry(); ok {
		return true
	}
	host, ok := m.selectedHost()
	if !ok {
		return false
	}
	if host.Synced {
		return !m.noSync && m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Delete })
	}
	return host.Managed
}

func promoteAvailable(m *App) bool {
	host, ok := m.selectedHost()
	return ok && !host.Synced && !host.Managed
}

func hostsMutable(m *App) bool {
	if _, ok := m.selectedEntry(); ok {
		return true
	}
	host, ok := m.selectedHost()
	if !ok {
		return false
	}
	if host.Synced && (host.SyncSource == "boat" || host.SyncSource == "upstash" || host.SyncSource == "vercel") {
		return false
	}
	return true
}

func hostsGroupAvailable(m *App) bool {
	if m.historySuggestionsHeaderSelected() {
		return true
	}
	_, ok := m.selectedGroup()
	return ok
}

func hostsGroupLabel(m *App) string {
	if m.historySuggestionsHeaderSelected() {
		if m.historySuggestionsCollapsed && m.searchText() == "" {
			return "expand"
		}
		return "collapse"
	}
	return m.collapseActionLabel()
}

func sortOrSyncLabel(m *App) string {
	if _, ok := m.selectedProviderRoot(); ok && !m.noSync {
		return "sync"
	}
	return "sort"
}

func hiddenLabel(m *App) string {
	if m.showHidden {
		return "hide hidden"
	}
	if hostsFooterShowStopped(m) {
		return "show stopped"
	}
	return "show hidden"
}

func newOrForkLabel(m *App) string {
	if _, ok := m.selectedProviderRoot(); ok {
		return "new"
	}
	return "fork"
}

func newOrForkAvailable(m *App) bool {
	if m.noSync {
		return false
	}
	if kind, ok := m.selectedProviderRoot(); ok && hostsFooterGroup(m) && cloud.CapabilitiesFor(kind).Create {
		return true
	}
	host, ok := m.selectedHost()
	return ok && m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Fork })
}

func stopAvailable(m *App) bool {
	if m.noSync {
		return false
	}
	host, ok := m.selectedHost()
	if !ok || m.hostLooksStopped(host) {
		return false
	}
	return m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Stop })
}

func restartAvailable(m *App) bool {
	if m.noSync {
		return false
	}
	host, ok := m.selectedHost()
	if !ok || m.hostLooksStopped(host) {
		return false
	}
	return m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Restart })
}

func filesForHostAvailable(m *App) bool {
	host, ok := m.selectedHost()
	if !ok {
		return false
	}
	return !(host.Synced && host.SyncSource == "vercel")
}

func rdpAvailable(m *App) bool {
	if m.itemCount() == 0 {
		return false
	}
	if _, ok := m.selectedEntry(); ok {
		return true
	}
	if host, ok := m.selectedHost(); ok {
		return m.hostMetadata()[host.Alias].RDP
	}
	return true
}

func keyHasFile(m *App) bool {
	key, ok := m.selectedKey()
	if !ok {
		return false
	}
	return key.PublicPath != "" || key.PrivatePath != ""
}

func keysPassphraseLabel(m *App) string {
	key, ok := m.selectedKey()
	if ok && !key.Managed {
		return "promote"
	}
	return "passphrase"
}

func vaultEnterLabel(m *App) string {
	if m.syncCursor >= 0 {
		return "run"
	}
	if !m.vaultLinked() {
		return "link"
	}
	if m.vaultPassphrase == "" {
		return "unlock"
	}
	return "sync"
}

func syncDetailOpen(m *App) bool { return m.syncProvider != "" }

func syncEnterLabel(m *App) string {
	if m.syncProvider == "" {
		return "open"
	}
	return "run"
}

func syncTabLabel(m *App) string {
	if m.syncProvider != "" {
		return "back"
	}
	return "open"
}

func syncCanCreate(m *App) bool {
	if m.noSync {
		return false
	}
	kind := m.syncFocusKind()
	return kind != "" && cloud.CapabilitiesFor(kind).Create
}

func filesReadyPane(m *App) *filesPane {
	m.initFilesState()
	return m.filesFocusedPane()
}

func filesBrowsingDir(m *App) bool {
	return !filesReadyPane(m).pickingHost()
}

func filesHasSelection(m *App) bool {
	pane := filesReadyPane(m)
	if pane.pickingHost() {
		return false
	}
	_, ok := pane.selectedEntry()
	return ok
}

func filesHasPaths(m *App) bool {
	pane := filesReadyPane(m)
	return !pane.pickingHost() && len(pane.selectedPaths()) > 0
}

func filesCanJump(m *App) bool {
	pane := filesReadyPane(m)
	return !pane.pickingHost() && len(pane.entries) > 0
}

func filesCanPreview(m *App) bool {
	pane := filesReadyPane(m)
	if pane.pickingHost() {
		return false
	}
	entry, ok := pane.selectedEntry()
	return ok && !entry.IsDir
}

func filesEnterLabel(m *App) string {
	pane := filesReadyPane(m)
	if pane.pickingHost() {
		return "connect"
	}
	entry, ok := pane.selectedEntry()
	if ok && entry.IsDir {
		return "open"
	}
	return "preview"
}

func filesParentLabel(m *App) string {
	if filesReadyPane(m).pickingHost() {
		return "back"
	}
	return "parent"
}

func filesPathLabel(m *App) string {
	if filesReadyPane(m).pickingHost() {
		return "search"
	}
	return "path"
}

func filesMarkAvailable(m *App) bool {
	pane := filesReadyPane(m)
	if pane.pickingHost() {
		return true
	}
	_, ok := pane.selectedEntry()
	return ok
}

func filesMarkLabel(m *App) string {
	if filesReadyPane(m).pickingHost() {
		return "group"
	}
	return "mark"
}

func filesEscapeLabel(m *App) string {
	pane := filesReadyPane(m)
	if len(pane.marked) > 0 || pane.rangeAnchor != nil {
		return "clear"
	}
	if pane.connected() {
		return "disconnect"
	}
	return "hosts"
}

func filesCanChmod(m *App) bool {
	pane := filesReadyPane(m)
	if pane.pickingHost() {
		return false
	}
	if pane.kind == filesPaneLocal && !platform.SupportsPOSIXPermissions() {
		return false
	}
	return len(pane.selectedPaths()) > 0
}

func filesCanShell(m *App) bool {
	pane := filesReadyPane(m)
	if !pane.pickingHost() {
		return true
	}
	row, ok := m.filesPickerRowAt(pane)
	return ok && !row.local && !row.row.header
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

func hostsFooterForward(m *App) bool {
	host, ok := m.selectedHost()
	return ok && m.hostHasForwards(host) && !m.forwardOpen(host.Alias)
}

func hostsFooterCloseTunnel(m *App) bool {
	return hostsFooterEntryTunnel(m) || hostsFooterForwardOpen(m)
}

func hostsFooterForwardOpen(m *App) bool {
	if _, ok := m.selectedForward(); ok {
		return true
	}
	host, ok := m.selectedHost()
	return ok && m.forwardOpen(host.Alias)
}

func hostsFooterEntryTunnel(m *App) bool {
	entry, ok := m.selectedEntry()
	return ok && m.entryTunnelOpen(entry)
}

func hostsFooterRDP(m *App) bool {
	host, ok := m.selectedHost()
	return ok && m.hostMetadata()[host.Alias].RDP
}
