package ui

import (
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
	ActionCreditsLeft
	ActionCreditsRight
	ActionCreditsActivate
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
	ActionAddEntry
	ActionStartForward
	ActionCloseTunnel
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
	ActionOpenRDP
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
	ActionSyncTab
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
	ScopeCredits
	ScopeDoctor
	ScopeHosts
	ScopeKeys
	ScopeVault
	ScopeSync
	ScopeFiles
)

// paletteGroup is where a binding sits in the action sheet.
type paletteGroup int

const (
	groupSelection paletteGroup = iota
	groupNavigate
	groupApp
)

type Binding struct {
	ID        Action
	Keys      []string
	Scope     Scope
	Name      string
	Hint      string
	HintOf    func(*App) string
	Chord     string
	Group     paletteGroup
	Available func(*App) bool
	// Show is false for aliases that should match but stay off the sheet.
	Show bool
}

func (b Binding) nav() Binding {
	b.Group = groupNavigate
	return b
}

func (b Binding) app() Binding {
	b.Group = groupApp
	return b
}

func (b Binding) available(fn func(*App) bool) Binding {
	b.Available = fn
	return b
}

func (b Binding) quiet() Binding {
	b.Show = false
	return b
}

func (b Binding) chord(s string) Binding {
	b.Chord = s
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

func bind(id Action, keys []string, scope Scope, name, hint string) Binding {
	return Binding{ID: id, Keys: keys, Scope: scope, Name: name, Hint: hint, Show: true, Group: groupSelection}
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
	// Tab numbers follow the tabs that are actually shown. The catalog keys are
	// the full set, so a hidden Vault would otherwise leave "3" opening Vault.
	if m.scopeActive(ScopeGlobal) && len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		sec, ok := m.sectionForDigit(key)
		if !ok {
			return Binding{}, false
		}
		return Binding{ID: tabAction(sec), Keys: []string{key}, Scope: ScopeGlobal, Name: sectionTitle(sec)}, true
	}
	for _, scope := range m.matchScopes() {
		for _, b := range catalog() {
			if b.Scope != scope {
				continue
			}
			if !keyIn(b.Keys, key) {
				continue
			}
			if b.Available != nil && !b.Available(m) {
				continue
			}
			return b, true
		}
	}
	return Binding{}, false
}

func (m *App) scopeActive(scope Scope) bool {
	for _, item := range m.matchScopes() {
		if item == scope {
			return true
		}
	}
	return false
}

func (m *App) matchScopes() []Scope {
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

func formatKey(key string, nerd, windowsTerminal bool) string {
	switch key {
	case "enter":
		if nerd {
			return enterGlyph(windowsTerminal)
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
	case "ctrl+o":
		return "Ctrl+O"
	case "ctrl+s":
		return "Ctrl+S"
	default:
		return key
	}
}

func formatChord(b Binding, nerd, windowsTerminal bool) string {
	if b.Chord != "" {
		return strings.ReplaceAll(b.Chord, nerdEnterGlyph, enterGlyph(windowsTerminal))
	}
	parts := make([]string, 0, len(b.Keys))
	for _, key := range b.Keys {
		parts = append(parts, formatKey(key, nerd, windowsTerminal))
	}
	return strings.Join(parts, " ")
}

func formatChip(b Binding) string {
	if len(b.Keys) == 0 {
		return b.Name
	}
	return "[" + b.Keys[0] + "] " + b.Name
}

func (m *App) keyInstallChip() string {
	return "Add to server"
}

func (m *App) keyPromoteChip() string {
	return "Promote to Bast managed"
}

func (m *App) tabChip(sec section) string {
	name := sectionTitle(sec)
	key, ok := m.tabDigit(sec)
	if !ok {
		return name
	}
	return formatChip(Binding{Keys: []string{key}, Name: name})
}

func bindingLabel(b Binding, m *App) string {
	if b.HintOf != nil {
		if hint := b.HintOf(m); hint != "" {
			return hint
		}
	}
	if b.Hint != "" {
		return b.Hint
	}
	return b.Name
}
