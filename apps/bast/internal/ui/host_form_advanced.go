package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/sshconfig"
)

const formScreenAdvancedHub = "advanced_hub"

const (
	formSectionAdvancedJump       = "advanced_jump"
	formSectionAdvancedSession    = "advanced_session"
	formSectionAdvancedForwarding = "advanced_forwarding"
	formSectionAdvancedEnv        = "advanced_env"
	formSectionAdvancedCustom     = "advanced_custom"
)

type advancedHubItem struct {
	id      string
	title   string
	section string
}

func isAdvancedSubsection(screen string) bool {
	switch screen {
	case formSectionAdvancedJump, formSectionAdvancedSession, formSectionAdvancedForwarding, formSectionAdvancedEnv, formSectionAdvancedCustom:
		return true
	default:
		return false
	}
}

var advancedHubList = []advancedHubItem{
	{id: "jump", title: "Jump & proxy", section: formSectionAdvancedJump},
	{id: "session", title: "Session", section: formSectionAdvancedSession},
	{id: "forwarding", title: "Forwarding", section: formSectionAdvancedForwarding},
	{id: "env", title: "Environment", section: formSectionAdvancedEnv},
	{id: "custom", title: "Custom flags", section: formSectionAdvancedCustom},
}

func (m *App) enterAdvancedHub() {
	f := m.form
	m.commitHostHubField()
	f.screen = formScreenAdvancedHub
	f.hubIndex = 0
	f.index = -1
	f.selecting = false
	f.input.Blur()
}

func (m *App) enterAdvancedSubsection(section string) {
	f := m.form
	indices := f.sectionFieldIndices(section)
	if len(indices) == 0 {
		return
	}
	f.screen = section
	f.index = indices[0]
	f.selecting = false
	m.focusFormField()
}

func (m *App) focusAdvancedHubItem() {
	f := m.form
	if f.hubIndex < 0 || f.hubIndex >= len(advancedHubList) {
		f.hubIndex = 0
	}
	f.index = -1
	f.selecting = false
	f.input.Blur()
}

func (m *App) moveAdvancedHub(direction int) {
	f := m.form
	next := f.hubIndex + direction
	if next < 0 || next >= len(advancedHubList) {
		return
	}
	f.hubIndex = next
	m.focusAdvancedHubItem()
}

func (m *App) advancedHubEnter() (tea.Model, tea.Cmd) {
	f := m.form
	items := advancedHubList
	if f.hubIndex >= len(items) {
		return m, nil
	}
	m.enterAdvancedSubsection(items[f.hubIndex].section)
	return m, nil
}

func (m *App) exitAdvancedSubsection() {
	f := m.form
	section := f.screen
	f.screen = formScreenAdvancedHub
	for i, item := range advancedHubList {
		if item.section == section {
			f.hubIndex = i
			break
		}
	}
	m.focusAdvancedHubItem()
}

func (m *App) advancedSubsectionSummary(section string) string {
	f := m.form
	switch section {
	case formSectionAdvancedJump:
		if jump := fieldDisplay(f, "Proxy jump"); jump != "" && jump != "-" && jump != "None" {
			return jump
		}
		return "Direct connection"
	case formSectionAdvancedSession:
		parts := []string{}
		if cmd := fieldDisplay(f, "Startup command"); cmd != "" && cmd != "-" {
			parts = append(parts, "command")
		}
		if tty := fieldDisplay(f, "Request TTY"); tty != "" && tty != "-" && tty != "Default" {
			parts = append(parts, strings.ToLower(tty))
		}
		if len(parts) == 0 {
			return "-"
		}
		return strings.Join(parts, " · ")
	case formSectionAdvancedForwarding:
		parts := []string{}
		if agent := fieldDisplay(f, "Agent forwarding"); agent != "" && agent != "-" && agent != "Default" {
			parts = append(parts, "agent "+strings.ToLower(agent))
		}
		if local := fieldDisplay(f, "Local forwards"); local != "" && local != "-" {
			parts = append(parts, "local")
		}
		if remote := fieldDisplay(f, "Remote forwards"); remote != "" && remote != "-" {
			parts = append(parts, "remote")
		}
		if dynamic := fieldDisplay(f, "Dynamic forward"); dynamic != "" && dynamic != "-" {
			parts = append(parts, "socks")
		}
		if len(parts) == 0 {
			return "-"
		}
		return strings.Join(parts, " · ")
	case formSectionAdvancedEnv:
		if env := fieldDisplay(f, "Environment variables"); env != "" && env != "-" {
			return env
		}
		return "-"
	case formSectionAdvancedCustom:
		if custom := fieldDisplay(f, "Custom SSH flags"); custom != "" && custom != "-" {
			return custom
		}
		return "-"
	default:
		return "-"
	}
}

func (m *App) hostAdvancedSummary() string {
	parts := []string{}
	for _, item := range advancedHubList {
		if summary := m.advancedSubsectionSummary(item.section); summary != "" && summary != "-" && summary != "Direct connection" {
			parts = append(parts, item.title)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ") + " configured"
}

func (m *App) updateAdvancedHubForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.exitHostSection()
		return m, nil
	case "backspace", "ctrl+h":
		m.exitHostSection()
		return m, nil
	case "up", "k", "shift+tab":
		m.moveAdvancedHub(-1)
	case "down", "j", "tab":
		m.moveAdvancedHub(1)
	case "enter":
		return m.advancedHubEnter()
	}
	return m, nil
}

func (m *App) renderAdvancedHubForm(s styleSet) string {
	f := m.form
	var b strings.Builder
	b.WriteString(m.renderHostFormHeader(s, "› Advanced"))
	for i, item := range advancedHubList {
		summary := m.advancedSubsectionSummary(item.section)
		line := item.title + "  " + summary
		if i == f.hubIndex {
			b.WriteString("  " + s.selected.Render("› "+truncate(line, max(20, m.terminalWidth()-6))) + "\n")
			b.WriteString("    " + s.muted.Render("Enter to configure") + "\n")
			continue
		}
		b.WriteString("  " + s.muted.Render("  "+truncate(line, max(20, m.terminalWidth()-4))) + "\n")
	}
	return b.String()
}

func (m *App) proxyJumpField(current string) field {
	item := field{
		label:       "Proxy jump",
		description: descHostProxyJump,
		placeholder: "bastion",
		optional:    true,
	}
	item.options = append(item.options, fieldOption{label: "None"})
	for _, host := range m.hosts {
		label := m.hostLabel(host)
		if target := destination(host); target != "" && target != host.Alias {
			label += " · " + target
		}
		item.options = append(item.options, fieldOption{label: label, value: host.Alias})
		if current == host.Alias {
			item.selected = len(item.options) - 1
			item.value = host.Alias
		}
	}
	item.options = append(item.options, fieldOption{label: "Manual host…", custom: true})
	current = strings.TrimSpace(current)
	if current != "" && item.value == "" {
		item.selected = len(item.options) - 1
		item.customValue = current
		item.value = current
	}
	return item
}

func (m *App) triStateField(label, description string, current string, choices [3]fieldOption) field {
	item := field{label: label, description: description, optional: true}
	item.options = append(item.options, choices[0], choices[1], choices[2])
	current = strings.ToLower(strings.TrimSpace(current))
	for i, option := range item.options {
		if option.value == current {
			item.selected = i
			item.value = option.value
			break
		}
	}
	return item
}

func advancedFormFields(m *App, adv sshconfig.AdvancedSettings) []field {
	jump := m.proxyJumpField(adv.ProxyJump)
	jump.section = formSectionAdvancedJump

	forwardAgent := m.triStateField("Agent forwarding", descHostForwardAgent, adv.ForwardAgent, [3]fieldOption{
		{label: "Default"},
		{label: "Yes", value: "yes"},
		{label: "No", value: "no"},
	})
	forwardAgent.section = formSectionAdvancedForwarding

	requestTTY := m.triStateField("Request TTY", descHostRequestTTY, adv.RequestTTY, [3]fieldOption{
		{label: "Default"},
		{label: "Force yes", value: "force"},
		{label: "Disable", value: "no"},
	})
	requestTTY.section = formSectionAdvancedSession

	compression := m.triStateField("Compression", descHostCompression, adv.Compression, [3]fieldOption{
		{label: "Default"},
		{label: "Yes", value: "yes"},
		{label: "No", value: "no"},
	})
	compression.section = formSectionAdvancedForwarding

	return []field{
		jump,
		{label: "Startup command", section: formSectionAdvancedSession, description: descHostRemoteCommand, value: adv.RemoteCommand, placeholder: "tmux attach -t main", optional: true},
		requestTTY,
		forwardAgent,
		{label: "Local forwards", section: formSectionAdvancedForwarding, description: descHostLocalForward, value: sshconfig.FormatForwardList(adv.LocalForwards), placeholder: "8080 localhost:80", optional: true},
		{label: "Remote forwards", section: formSectionAdvancedForwarding, description: descHostRemoteForward, value: sshconfig.FormatForwardList(adv.RemoteForwards), placeholder: "8080 localhost:80", optional: true},
		{label: "Dynamic forward", section: formSectionAdvancedForwarding, description: descHostDynamicForward, value: adv.DynamicForward, placeholder: "1080", optional: true},
		compression,
		{label: "Keepalive (seconds)", section: formSectionAdvancedForwarding, description: descHostKeepalive, value: adv.ServerAliveInterval, placeholder: "30", optional: true},
		{label: "Environment variables", section: formSectionAdvancedEnv, description: descHostSetEnv, value: sshconfig.FormatSetEnvList(adv.SetEnv), placeholder: "FOO=bar; BAZ=qux", optional: true},
		{label: "Custom SSH flags", section: formSectionAdvancedCustom, description: descHostSSHFlags, value: sshconfig.FormatSSHFlags(adv.Custom), placeholder: "IdentitiesOnly yes", optional: true},
	}
}

func advancedSettingsFromForm(values map[string]string) sshconfig.AdvancedSettings {
	return sshconfig.AdvancedSettings{
		ProxyJump:           strings.TrimSpace(values["Proxy jump"]),
		ForwardAgent:        values["Agent forwarding"],
		RemoteCommand:       strings.TrimSpace(values["Startup command"]),
		RequestTTY:          values["Request TTY"],
		SetEnv:              sshconfig.ParseSetEnvList(values["Environment variables"]),
		LocalForwards:       sshconfig.ParseForwardList(values["Local forwards"]),
		RemoteForwards:      sshconfig.ParseForwardList(values["Remote forwards"]),
		DynamicForward:      strings.TrimSpace(values["Dynamic forward"]),
		ServerAliveInterval: strings.TrimSpace(values["Keepalive (seconds)"]),
		Compression:         values["Compression"],
		Custom:              sshconfig.ParseSSHFlags(values["Custom SSH flags"]),
	}
}
