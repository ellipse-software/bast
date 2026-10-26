package ui

import (
	"fmt"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

const (
	sponsorURL    = "https://bast.sh/sponsor"
	sponsorAction = " Sponsor "
)

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
	if err := openBrowser(sponsorURL); err != nil {
		m.setError(err)
		return m, nil
	}
	return m, nil
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
