package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/cloud"
	"bast/internal/cloud/sync"
	"bast/internal/sshconfig"
)

// browseFooterHint returns contextual key chords for the current browse mode.
// Keep these short: a few primary actions for the selection, then "?".
func (m *App) browseFooterHint(maxWidth int) string {
	var parts []string
	switch m.section {
	case hostsSection:
		parts = m.hostsFooterParts()
	case keysSection:
		parts = m.keysFooterParts()
	case vaultSection:
		parts = m.vaultFooterParts()
	case syncSection:
		parts = m.syncFooterParts()
	case filesSection:
		return fitFooterHint(splitFooterHint(m.filesFooterHint()), maxWidth)
	default:
		parts = []string{"?"}
	}
	return fitFooterHint(parts, maxWidth)
}

func (m *App) hostsFooterParts() []string {
	if m.itemCount() == 0 {
		if m.hasHiddenHosts() && !m.showHidden {
			return []string{". show hidden", "a add", "?"}
		}
		return []string{"a add", "?"}
	}
	if m.historySuggestionsHeaderSelected() {
		label := "collapse"
		if m.historySuggestionsCollapsed && m.searchText() == "" {
			label = "expand"
		}
		return []string{"␣ " + label, "?"}
	}
	if _, ok := m.selectedHistorySuggestion(); ok {
		return []string{"enter add", "e review", "x dismiss", "?"}
	}
	if group, ok := m.selectedGroupHeader(); ok {
		parts := []string{"␣ " + m.collapseActionLabel()}
		if kind, ok := m.selectedProviderRoot(); ok {
			caps := cloud.CapabilitiesFor(kind)
			if caps.Create {
				parts = append(parts, "n new")
			}
			parts = append(parts, "s sync")
			if caps.Stop && !m.showHidden {
				if _, stopped := m.providerGroupStats(group); stopped > 0 {
					parts = append(parts, ". show stopped")
				}
			}
			return append(parts, "?")
		}
		if !sync.IsSyncedGroup(group) {
			parts = append(parts, "e rename")
		}
		return append(parts, "?")
	}
	if host, ok := m.selectedHost(); ok {
		if host.Synced && m.hostHasCapability(host, func(c cloud.Capabilities) bool {
			return c.Stop || c.Start || c.Fork || c.Delete
		}) {
			parts := m.sandboxHostFooterParts(host)
			if !m.isMobileLayout() {
				parts = append(parts, "F files")
			}
			return append(parts, "?")
		}
		if host.Synced {
			if m.isMobileLayout() {
				return []string{"enter connect", "?"}
			}
			return []string{"enter", "F files", "?"}
		}
		if m.isMobileLayout() {
			return []string{"enter connect", "e edit", "?"}
		}
		return []string{"enter", "e edit", "F files", "?"}
	}
	return []string{"a add", "?"}
}

func (m *App) keysFooterParts() []string {
	if len(m.filteredKeys()) == 0 {
		return []string{"a generate", "i import", "?"}
	}
	key, ok := m.selectedKey()
	if !ok {
		return []string{"a generate", "i import", "?"}
	}
	if key.PublicPath == "" && key.PrivatePath == "" {
		return []string{"a generate", "i import", "?"}
	}
	return []string{"u add", "c copy", "e edit", "?"}
}

func (m *App) vaultFooterParts() []string {
	if m.syncCursor >= 0 {
		return []string{"enter", "?"}
	}
	if !m.vaultLinked() {
		return []string{"enter link", "?"}
	}
	if m.vaultPassphrase == "" {
		return []string{"enter unlock", "?"}
	}
	return []string{"enter sync", "?"}
}

func (m *App) syncFooterParts() []string {
	if m.syncProvider == "" {
		return []string{"hjkl move", "enter open", "s sync", "?"}
	}
	life, _ := m.providerActionLayout()
	inv := m.providerInventoryRows()
	L, I := len(life), len(inv)
	if m.syncCursor < L {
		parts := []string{"enter"}
		if L > 1 {
			parts = []string{"h/l cycle", "enter"}
		}
		if m.syncCursor >= 0 && m.syncCursor < len(life) {
			switch life[m.syncCursor].action {
			case "sync":
				parts[len(parts)-1] = "enter sync"
			case "enable":
				parts[len(parts)-1] = "enter connect"
			}
		}
		return append(parts, "esc back", "?")
	}
	if m.syncCursor < L+I {
		row := inv[m.syncCursor-L]
		if row.header {
			label := "collapse"
			if m.providerInvCollapsed(row.group) {
				label = "expand"
			}
			return []string{"␣ " + label, "esc back", "?"}
		}
		if m.hostHasCapability(row.host, func(c cloud.Capabilities) bool {
			return c.Stop || c.Start || c.Fork || c.Delete
		}) {
			return append(m.sandboxHostFooterParts(row.host), "esc back", "?")
		}
		return []string{"enter connect", "esc back", "?"}
	}
	return []string{"enter", "esc back", "?"}
}

func (m *App) sandboxHostFooterParts(host sshconfig.Host) []string {
	parts := []string{"enter connect", "o stop", "n fork"}
	canDelete := m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Delete })
	if canDelete {
		parts = append(parts, "d delete")
	}
	if m.hostLooksStopped(host) {
		parts = []string{"enter connect", "r resume", "n fork"}
		if canDelete {
			parts = append(parts, "d delete")
		}
	}
	return parts
}

func splitFooterHint(hint string) []string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return []string{"?"}
	}
	parts := strings.Split(hint, " · ")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"?"}
	}
	return out
}

// fitFooterHint joins chords with " · ", dropping trailing actions when the
// terminal is too narrow. The first chord and a trailing "?" are always kept.
func fitFooterHint(parts []string, maxWidth int) string {
	parts = append([]string(nil), parts...)
	if len(parts) == 0 {
		return "?"
	}
	if parts[len(parts)-1] != "?" {
		parts = append(parts, "?")
	}
	join := func(items []string) string {
		return strings.Join(items, " · ")
	}
	if maxWidth <= 0 || lipgloss.Width(join(parts)) <= maxWidth {
		return join(parts)
	}
	for len(parts) > 2 {
		// Drop the second-to-last action (keep primary + "?").
		parts = append(parts[:len(parts)-2], parts[len(parts)-1])
		if lipgloss.Width(join(parts)) <= maxWidth {
			return join(parts)
		}
	}
	if lipgloss.Width(parts[0]) <= maxWidth {
		return parts[0]
	}
	return "?"
}
