package ui

import (
	"sort"
	"strings"
	"sync"
)

type Action int

const (
	ActionNone Action = iota
	ActionQuit
	ActionHelpToggle
	ActionHelpClose
	ActionHelpScrollUp
	ActionHelpScrollDown
	ActionHelpPageUp
	ActionHelpPageDown
	ActionHelpTop
	ActionHelpBottom
	ActionCreditsOpen
	ActionCreditsClose
	ActionOnboardingReplay
	ActionSponsorOpen
	ActionDoctorOpen
	ActionDoctorClose
	ActionDoctorScrollUp
	ActionDoctorScrollDown
	ActionDoctorPageUp
	ActionDoctorPageDown
	ActionDoctorTop
	ActionDoctorBottom
	ActionTabHosts
	ActionTabKeys
	ActionTabVault
	ActionTabSync
	ActionTabFiles
	ActionMoveUp
	ActionMoveDown
	ActionMoveLeft
	ActionMoveRight
	ActionJumpTop
	ActionJumpBottom
	ActionSearch
	ActionReload
	ActionToggleHidden
	ActionToggleGroup
	ActionCollapseAll
	ActionExpandAll
	ActionAddHost
	ActionEdit
	ActionAssignGroup
	ActionDelete
	ActionDismissHistory
	ActionPromote
	ActionKnownHosts
	ActionStopSandbox
	ActionRestartHost
	ActionNewOrFork
	ActionFavorite
	ActionOpenFiles
	ActionHideHost
	ActionHostsEnter
	ActionSortOrSync
	ActionGenerateKey
	ActionInstallKey
	ActionImportKey
	ActionExportKey
	ActionCopyPublicKey
	ActionKeysPassphrase
	ActionVaultEnter
	ActionVaultSync
	ActionSyncEnter
	ActionSyncBack
	ActionSyncNow
	ActionFilesTab
	ActionFilesSwap
	ActionFilesLocal
	ActionFilesRemote
	ActionFilesParent
	ActionFilesEnter
	ActionFilesOpenDir
	ActionFilesPreview
	ActionFilesJump
	ActionFilesPath
	ActionFilesMark
	ActionFilesRange
	ActionFilesCopy
	ActionFilesMove
	ActionFilesMkdir
	ActionFilesRename
	ActionFilesDelete
	ActionFilesInfo
	ActionFilesChmod
	ActionFilesShell
	ActionFilesHidden
	ActionFilesDisconnect
	ActionFilesEscape
)

type Scope int

const (
	ScopeGlobal Scope = iota
	ScopeHelp
	ScopeCredits
	ScopeDoctor
	ScopeHosts
	ScopeKeys
	ScopeVault
	ScopeSync
	ScopeFiles
	ScopeDuringSSH
)

type Binding struct {
	ID        Action
	Keys      []string
	Scope     Scope
	Name      string
	Hint      string
	HintOf    func(*App) string
	Chord     string
	Footer    int
	Help      bool
	HelpOnly  bool
	SkipMatch bool
	When      func(*App) bool
}

func (b Binding) withFooter(n int) Binding {
	b.Footer = n
	return b
}

func (b Binding) when(fn func(*App) bool) Binding {
	b.When = fn
	return b
}

func (b Binding) chord(s string) Binding {
	b.Chord = s
	return b
}

func (b Binding) noHelp() Binding {
	b.Help = false
	return b
}

func (b Binding) hint(s string) Binding {
	b.Hint = s
	return b
}

func (b Binding) hintOf(fn func(*App) string) Binding {
	b.HintOf = fn
	return b
}

func (b Binding) helpOnly() Binding {
	b.HelpOnly = true
	b.Help = true
	return b
}

func (b Binding) skip() Binding {
	b.SkipMatch = true
	return b
}

func bind(id Action, keys []string, scope Scope, name, hint string) Binding {
	return Binding{ID: id, Keys: keys, Scope: scope, Name: name, Hint: hint, Help: true}
}

var (
	catalogOnce     sync.Once
	catalogBindings []Binding
)

func catalog() []Binding {
	catalogOnce.Do(func() { catalogBindings = buildCatalog() })
	return catalogBindings
}

func (m *App) matchBinding(key string) (Binding, bool) {
	for _, scope := range m.matchScopes() {
		for _, b := range catalog() {
			if b.HelpOnly || b.SkipMatch || b.Scope != scope {
				continue
			}
			if !keyIn(b.Keys, key) {
				continue
			}
			if b.When != nil && !b.When(m) {
				continue
			}
			return b, true
		}
	}
	return Binding{}, false
}

func (m *App) matchScopes() []Scope {
	if m.help {
		return []Scope{ScopeHelp}
	}
	if m.credits {
		return []Scope{ScopeCredits}
	}
	if m.doctor {
		return []Scope{ScopeDoctor}
	}
	return []Scope{m.tabScope(), ScopeGlobal}
}

func (m *App) tabScope() Scope {
	switch m.section {
	case keysSection:
		return ScopeKeys
	case vaultSection:
		return ScopeVault
	case syncSection:
		return ScopeSync
	case filesSection:
		return ScopeFiles
	default:
		return ScopeHosts
	}
}

func (m *App) tabTitle() string {
	switch m.section {
	case keysSection:
		return "Keys"
	case vaultSection:
		return "Vault"
	case syncSection:
		return "Sync"
	case filesSection:
		return "Files"
	default:
		return "Hosts"
	}
}

func keyIn(keys []string, key string) bool {
	for _, candidate := range keys {
		if candidate == key {
			return true
		}
	}
	return false
}

func formatKey(key string, nerd bool) string {
	switch key {
	case "enter":
		if nerd {
			return "󰌑"
		}
		return "enter"
	case "space":
		return "␣"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "esc":
		return "esc"
	case "backspace":
		return "⌫"
	case "tab":
		return "tab"
	case "home":
		return "Home"
	case "end":
		return "End"
	case "pgup":
		return "PgUp"
	case "pgdown":
		return "PgDn"
	case "ctrl+c":
		return "Ctrl+C"
	case "ctrl+h":
		return "Ctrl+H"
	case "ctrl+j":
		return "Ctrl+J"
	default:
		return key
	}
}

func formatChord(b Binding, nerd bool) string {
	if b.Chord != "" {
		return b.Chord
	}
	parts := make([]string, 0, len(b.Keys))
	for _, key := range b.Keys {
		parts = append(parts, formatKey(key, nerd))
	}
	return strings.Join(parts, " ")
}

func formatHint(b Binding, m *App) string {
	hint := b.Hint
	if b.HintOf != nil {
		hint = b.HintOf(m)
	}
	chord := formatChord(b, m.nerdFont)
	if hint == "" {
		return chord
	}
	return chord + " " + hint
}

func formatChip(b Binding) string {
	if len(b.Keys) == 0 {
		return b.Name
	}
	return "[" + b.Keys[0] + "] " + b.Name
}

func firstBinding(id Action) (Binding, bool) {
	for _, b := range catalog() {
		if b.ID == id && !b.HelpOnly {
			return b, true
		}
	}
	return Binding{}, false
}

func (m *App) keyInstallChip() string {
	b, ok := firstBinding(ActionInstallKey)
	if !ok {
		return "[a] Add to server"
	}
	return formatChip(b)
}

func (m *App) keyPromoteChip() string {
	b, ok := firstBinding(ActionPromote)
	if !ok {
		return "[p] Promote to Bast managed"
	}
	b.Name = "Promote to Bast managed"
	return formatChip(b)
}

func (m *App) tabChip(sec section) string {
	id := ActionTabHosts
	name := "Hosts"
	switch sec {
	case keysSection:
		id, name = ActionTabKeys, "Keys"
	case vaultSection:
		id, name = ActionTabVault, "Vault"
	case syncSection:
		id, name = ActionTabSync, "Sync"
	case filesSection:
		id, name = ActionTabFiles, "Files"
	}
	b, ok := firstBinding(id)
	if !ok {
		return formatChip(Binding{Keys: []string{"?"}, Name: name})
	}
	b.Name = name
	return formatChip(b)
}

func (m *App) catalogFooterParts() []string {
	return m.footerPartsFor(m.tabScope(), ScopeGlobal)
}

func (m *App) overlayFooterParts(scope Scope) []string {
	return m.footerPartsFor(scope)
}

func (m *App) footerPartsFor(scopes ...Scope) []string {
	active := map[Scope]bool{}
	for _, scope := range scopes {
		active[scope] = true
	}
	type ranked struct {
		rank int
		text string
		idx  int
	}
	var items []ranked
	for i, b := range catalog() {
		if b.Footer <= 0 || b.HelpOnly || b.ID == ActionNone || !active[b.Scope] {
			continue
		}
		if b.When != nil && !b.When(m) {
			continue
		}
		items = append(items, ranked{rank: b.Footer, text: formatHint(b, m), idx: i})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].rank != items[j].rank {
			return items[i].rank < items[j].rank
		}
		return items[i].idx < items[j].idx
	})
	parts := make([]string, 0, len(items)+1)
	for _, item := range items {
		if item.text != "" {
			parts = append(parts, item.text)
		}
	}
	return parts
}

type helpRow struct {
	chord   string
	name    string
	enabled bool
}

type helpGroup struct {
	title string
	rows  []helpRow
}

func (m *App) helpGroups() []helpGroup {
	tab := m.tabScope()
	shadowed := map[string]bool{}
	var tabBindings []Binding
	for _, b := range catalog() {
		if b.Scope != tab || !b.Help {
			continue
		}
		tabBindings = append(tabBindings, b)
		for _, key := range b.Keys {
			shadowed[key] = true
		}
	}
	tabGroup := helpGroup{rows: m.helpRows(tabBindings, nil)}
	appBindings := make([]Binding, 0)
	for _, b := range catalog() {
		if b.Scope != ScopeGlobal || !b.Help {
			continue
		}
		appBindings = append(appBindings, b)
	}
	appGroup := helpGroup{rows: m.helpRows(appBindings, shadowed)}
	groups := []helpGroup{tabGroup, appGroup}
	if tab == ScopeHosts {
		var ssh []Binding
		for _, b := range catalog() {
			if b.Scope == ScopeDuringSSH && b.Help {
				ssh = append(ssh, b)
			}
		}
		groups = append(groups, helpGroup{title: "During SSH", rows: m.helpRows(ssh, nil)})
	}
	return groups
}

func (m *App) helpRows(bindings []Binding, shadowed map[string]bool) []helpRow {
	var enabled, disabled []helpRow
	seen := map[Action]bool{}
	for _, b := range bindings {
		if !b.HelpOnly && b.ID != ActionNone && seen[b.ID] {
			continue
		}
		if !b.HelpOnly {
			seen[b.ID] = true
		}
		keys := b.Keys
		if len(shadowed) > 0 && !b.HelpOnly {
			keys = nil
			for _, key := range b.Keys {
				if !shadowed[key] {
					keys = append(keys, key)
				}
			}
			if len(keys) == 0 && b.Chord == "" {
				continue
			}
		}
		display := b
		if len(shadowed) > 0 && !b.HelpOnly {
			display.Keys = keys
			if b.Chord != "" && len(keys) < len(b.Keys) {
				display.Chord = ""
			}
		}
		row := helpRow{
			chord:   formatChord(display, m.nerdFont),
			name:    b.Name,
			enabled: b.HelpOnly || b.When == nil || b.When(m),
		}
		if row.chord == "" {
			continue
		}
		if row.enabled {
			enabled = append(enabled, row)
		} else {
			disabled = append(disabled, row)
		}
	}
	return append(enabled, disabled...)
}
