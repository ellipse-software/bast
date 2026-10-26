package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/rdp"
	"bast/internal/sshconfig"
)

var launchRDP = rdp.Launch

func rdpAddress(host sshconfig.Host) string {
	if name := strings.TrimSpace(host.Resolved.HostName); name != "" {
		return name
	}
	return strings.TrimSpace(host.Alias)
}

func (m *App) openSelectedRDP() (tea.Model, tea.Cmd) {
	host, ok := m.selectedHost()
	if !ok {
		return m, m.setNotice("Select a host to open Remote Desktop")
	}
	// User and Domain stay empty so the system client prompts. The SSH port
	// is not the RDP port, so it is not appended to the address.
	session := rdp.Session{Address: rdpAddress(host)}
	if err := launchRDP(session); err != nil {
		m.setError(err)
		return m, nil
	}
	return m, m.setNotice("Opened Remote Desktop for " + session.Address)
}
