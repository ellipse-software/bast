package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/telemetry"
)

const onboardingCopy = "The fast way into the servers you use every day."

const onboardingWelcome = "Welcome to Bast. This tool will help you replace your heavy GUIs like Termius. Bast can do everything, connect you to a server, or transfer a file between two. Thank you for supporting my project. - Ted"

func (m *App) renderOnboarding(s styleSet) string {
	if !m.onboardingReplay && !m.onboardingTracked {
		m.onboardingTracked = true
		telemetry.Track("onboarding_show", m.version)
	}
	width := min(52, max(24, m.terminalWidth()-4))
	var b strings.Builder
	b.WriteString(s.active.Width(width).Align(lipgloss.Center).Render(bastLogo))
	b.WriteString("\n\n")
	b.WriteString(s.muted.Width(width).Align(lipgloss.Center).Render(onboardingCopy))
	b.WriteString("\n\n")
	for i, line := range wrapWords(onboardingWelcome, width) {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s.value.Width(width).Render(line))
	}
	b.WriteString("\n\n")
	b.WriteString(s.muted.Width(width).Align(lipgloss.Center).Render("any key to continue"))
	return lipgloss.Place(m.terminalWidth(), m.bodyHeight(), lipgloss.Center, lipgloss.Center, b.String())
}

func (m *App) replayOnboarding() {
	m.closeDoctor()
	m.credits = false
	m.palette = false
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

func (m *App) updateOnboarding(string) (tea.Model, tea.Cmd) {
	return m.afterOnboarding("onboarding_continue", m.landOnboarding)
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
