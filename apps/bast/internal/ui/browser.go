package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	sponsorURL    = "https://bast.sh/sponsor"
	discordURL    = "https://discord.gg/bast"
	websiteURL    = "https://bast.sh"
	sponsorAction = " Sponsor "
)

type creditLink struct {
	label string
	url   string
}

func creditLinks() []creditLink {
	return []creditLink{
		{label: "Sponsor", url: sponsorURL},
		{label: "Discord", url: discordURL},
		{label: "Website", url: websiteURL},
	}
}

func (m *App) openCredits() {
	m.closeDoctor()
	m.onboarding = false
	m.credits = true
	m.creditsChoice = 0
	m.closePalette()
}

func (m *App) moveCreditChoice(delta int) {
	links := creditLinks()
	if len(links) == 0 {
		return
	}
	next := m.creditsChoice + delta
	if next < 0 {
		next = 0
	}
	if next >= len(links) {
		next = len(links) - 1
	}
	m.creditsChoice = next
}

func creditChoiceLabel(m *App) string {
	links := creditLinks()
	if m.creditsChoice < 0 || m.creditsChoice >= len(links) {
		return "sponsor"
	}
	return strings.ToLower(links[m.creditsChoice].label)
}

func (m *App) openCreditChoice() (tea.Model, tea.Cmd) {
	links := creditLinks()
	if m.creditsChoice < 0 || m.creditsChoice >= len(links) {
		m.creditsChoice = 0
	}
	return m.openURL(links[m.creditsChoice].url)
}

func (m *App) openURL(raw string) (tea.Model, tea.Cmd) {
	if err := openBrowser(raw); err != nil {
		m.setError(err)
		return m, nil
	}
	return m, nil
}

var openBrowser = openBrowserDefault

func openBrowserDefault(raw string) error {
	cmd, err := browserCommand(raw)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open %s: %w", raw, err)
	}
	return nil
}

func (m *App) openSponsor() (tea.Model, tea.Cmd) {
	return m.openURL(sponsorURL)
}

func browserCommand(raw string) (*exec.Cmd, error) {
	if raw == "" {
		return nil, fmt.Errorf("empty url")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", raw), nil
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", raw), nil
	default:
		return exec.Command("xdg-open", raw), nil
	}
}
