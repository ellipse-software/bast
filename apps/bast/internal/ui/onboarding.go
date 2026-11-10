package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/metadata"
	"bast/internal/telemetry"
)

const onboardingCopy = "The fast way into the servers you use every day."

func (m *App) renderOnboarding(s styleSet) string {
	if !m.onboardingReplay && !m.onboardingTracked {
		m.onboardingTracked = true
		telemetry.Track("onboarding_show", m.version)
	}
	inner := min(50, max(8, m.terminalWidth()-2))
	content := max(4, inner-2)
	lines := []string{onboardingCopy, ""}
	lines = append(lines, m.onboardingFacts()...)
	var actions []string
	for _, item := range m.onboardingActions() {
		actions = append(actions, onboardingActionLine(s, item.key, item.desc, content))
	}
	return m.renderSeal(s, "◇  Welcome to Bast", lines, actions)
}

func (m *App) onboardingFacts() []string {
	hosts, keys := len(m.hosts), len(m.keys)
	var lines []string
	switch {
	case hosts > 0 && keys > 0:
		lines = append(lines, counted(hosts, "host")+" · "+counted(keys, "key"))
	case hosts > 0:
		lines = append(lines, counted(hosts, "host"))
	case keys > 0:
		lines = append(lines, counted(keys, "key")+" on this machine")
	}
	if history := m.visibleHistorySuggestions(); len(history) > 0 {
		lines = append(lines, fmt.Sprintf("%d from %s history", len(history), historyShellLabel(history)))
	}
	return lines
}

type onboardingAction struct {
	key, desc string
}

func (m *App) onboardingActions() []onboardingAction {
	var actions []onboardingAction
	if len(m.hosts) == 0 {
		actions = append(actions, onboardingAction{"a", "add a host"})
		if len(m.visibleHistorySuggestions()) > 0 {
			actions = append(actions, onboardingAction{"enter", "history"})
		}
	} else {
		actions = append(actions, onboardingAction{"enter", "hosts"})
	}
	if key, ok := m.tabDigit(vaultSection); ok {
		actions = append(actions, onboardingAction{key, "vault · other computers"})
	}
	if key, ok := m.tabDigit(syncSection); ok {
		actions = append(actions, onboardingAction{key, "sync · cloud VMs"})
	}
	return actions
}

func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func historyShellLabel(suggestions []metadata.HistorySuggestion) string {
	seen := map[string]bool{}
	var names []string
	for _, suggestion := range suggestions {
		name := historyShellName(suggestion.Source)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 1 {
		return names[0]
	}
	return "shell"
}

func historyShellName(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	case "fish":
		return "fish"
	case "powershell":
		return "PowerShell"
	default:
		return strings.TrimSpace(source)
	}
}

func onboardingActionLine(s styleSet, key, desc string, width int) string {
	const keyCol = 6
	keys := s.value.Render(key)
	gap := max(1, keyCol-lipgloss.Width(key))
	descWidth := max(4, width-keyCol)
	return keys + strings.Repeat(" ", gap) + s.muted.Render(truncate(desc, descWidth))
}

func (m *App) replayOnboarding() {
	m.closeDoctor()
	m.credits = false
	m.help = false
	m.onboardingPending = false
	m.onboarding = true
	m.onboardingReplay = true
}

func (m *App) decideOnboarding() {
	if m.onboardingReplay || !m.onboardingPending {
		return
	}
	m.onboardingPending = false
	if len(m.hosts) == 0 && m.metadata.ShouldOnboard() {
		m.onboarding = true
		return
	}
	m.onboarding = false
	if m.metadata.ShouldOnboard() {
		_ = m.metadata.DismissOnboarding()
	}
}

func (m *App) updateOnboarding(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		m.closeOwnedTunnels()
		return m, tea.Quit
	case "esc", "backspace", "ctrl+h":
		return m.afterOnboarding("onboarding_skip", m.skipOnboarding)
	case "enter":
		return m.afterOnboarding("onboarding_continue", m.landOnboarding)
	case "a":
		return m.afterOnboarding("onboarding_add", func() tea.Cmd {
			m.section, m.cursor, m.search = hostsSection, 0, ""
			m.openAddHostForm()
			return nil
		})
	case "?":
		return m.afterOnboarding("onboarding_help", func() tea.Cmd {
			m.help, m.helpOffset = true, 0
			return nil
		})
	case "v":
		return m.afterOnboarding("onboarding_skip", func() tea.Cmd {
			m.credits = true
			return nil
		})
	}
	if sec, ok := m.sectionForDigit(key); ok {
		event := "onboarding_skip"
		switch sec {
		case hostsSection:
			event = "onboarding_continue"
		case vaultSection:
			event = "onboarding_vault"
		case syncSection:
			event = "onboarding_sync"
		}
		return m.afterOnboarding(event, func() tea.Cmd {
			return m.switchToSection(sec)
		})
	}
	return m, nil
}

func (m *App) afterOnboarding(event string, next func() tea.Cmd) (tea.Model, tea.Cmd) {
	replay := m.onboardingReplay
	if !replay {
		if err := m.metadata.DismissOnboarding(); err != nil {
			m.setError(err)
			return m, nil
		}
		telemetry.Track(event, m.version)
	}
	m.onboarding = false
	m.onboardingReplay = false
	return m, next()
}

func (m *App) skipOnboarding() tea.Cmd {
	m.section, m.search, m.cursor = hostsSection, "", 0
	m.clampCursor()
	return nil
}

func (m *App) landOnboarding() tea.Cmd {
	m.section, m.search = hostsSection, ""
	if m.hasVisibleConfiguredHosts() {
		m.cursor = 0
		m.clampCursor()
		return nil
	}
	for i, row := range m.hostListRows() {
		if row.suggestion != nil {
			m.cursor = i
			return nil
		}
	}
	m.cursor = 0
	return nil
}

func (m *App) hasVisibleConfiguredHosts() bool {
	for _, row := range m.hostRows() {
		if !row.header && row.suggestion == nil {
			return true
		}
	}
	return false
}
