package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"bast/internal/feature"
)

func (m *App) visibleSections() []section {
	sections := []section{hostsSection, keysSection}
	if !m.noVault {
		sections = append(sections, vaultSection)
	}
	if !m.noSync {
		sections = append(sections, syncSection)
	}
	return append(sections, filesSection)
}

func (m *App) sectionEnabled(sec section) bool {
	switch sec {
	case vaultSection:
		return !m.noVault
	case syncSection:
		return !m.noSync
	default:
		return true
	}
}

func (m *App) sectionForDigit(key string) (section, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	n := int(key[0] - '0')
	sections := m.visibleSections()
	if n < 1 || n > len(sections) {
		return 0, false
	}
	return sections[n-1], true
}

func (m *App) tabDigit(sec section) (string, bool) {
	for i, item := range m.visibleSections() {
		if item == sec {
			return strconv.Itoa(i + 1), true
		}
	}
	return "", false
}

func sectionTitle(sec section) string {
	switch sec {
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

func tabAction(sec section) Action {
	switch sec {
	case keysSection:
		return ActionTabKeys
	case vaultSection:
		return ActionTabVault
	case syncSection:
		return ActionTabSync
	case filesSection:
		return ActionTabFiles
	default:
		return ActionTabHosts
	}
}

func (m *App) syncOffNotice() tea.Cmd {
	return m.setNotice("Sync is off (" + feature.EnvNoSync + ")")
}
