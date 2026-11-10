package ui

import (
	"context"
	"errors"
	"hash/fnv"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/askpass"
	"bast/internal/hostlife"
	"bast/internal/metadata"
	"bast/internal/page"
	"bast/internal/rdp"
	"bast/internal/sshconfig"
	"bast/internal/tunnel"
)

const (
	openAction        = " Open "
	closeTunnelAction = " Close "
)

var (
	startEntryFile = rdp.StartFile
	openEntryPage  = page.Open
	openProxied    = page.OpenProxied
	startEntryRDP  = func(m *App, id, via, target string, configure func(*exec.Cmd)) (int, bool, error) {
		sup, err := m.tunnelSupervisor()
		if err != nil {
			return 0, false, err
		}
		return sup.RDP(id, via, target, configure)
	}
	startEntrySOCKS = func(m *App, via string, configure func(*exec.Cmd)) (int, bool, error) {
		sup, err := m.tunnelSupervisor()
		if err != nil {
			return 0, false, err
		}
		return sup.SOCKS(via, configure)
	}
)

type entryOpenedMsg struct {
	id     string
	notice string
	err    error
}

type entryStatusMsg struct {
	text string
	next tea.Cmd
}

func (m *App) selectEntry(id string) {
	entry, ok := m.metadata.Entry(id)
	if !ok {
		m.clampCursor()
		return
	}
	if entry.Group != "" {
		next, _, changed := m.collapsedGroupsRevealing(entry.Group)
		if changed {
			m.collapsedGroups = next
			m.collapseRevision++
			_ = m.persistCollapsedGroups()
		}
	}
	m.section = hostsSection
	m.search = ""
	for i, row := range m.hostListRows() {
		if row.entry != nil && row.entry.ID == id {
			m.cursor = i
			m.clampCursor()
			return
		}
	}
	m.clampCursor()
}

func (m *App) selectedEntry() (metadata.Entry, bool) {
	rows := m.hostListRows()
	if m.cursor < 0 || m.cursor >= len(rows) || rows[m.cursor].entry == nil {
		return metadata.Entry{}, false
	}
	return *rows[m.cursor].entry, true
}

func (m *App) listedEntries(query string) []metadata.Entry {
	q := strings.ToLower(strings.TrimSpace(query))
	all := m.metadata.Entries()
	out := make([]metadata.Entry, 0, len(all))
	for _, entry := range all {
		if entry.Hidden && !m.showHidden {
			continue
		}
		if q != "" && !strings.Contains(m.entryHaystack(entry), q) {
			continue
		}
		out = append(out, entry)
	}
	sortEntries(out)
	return out
}

func sortEntries(entries []metadata.Entry) {
	for i := 1; i < len(entries); i++ {
		item := entries[i]
		j := i
		for j > 0 && strings.ToLower(entries[j-1].Label) > strings.ToLower(item.Label) {
			entries[j] = entries[j-1]
			j--
		}
		entries[j] = item
	}
}

func (m *App) entryHaystack(entry metadata.Entry) string {
	jump := entry.Via
	if label := m.viaLabel(entry.Via); label != "" && label != entry.Via {
		jump += " " + label
	}
	return strings.ToLower(strings.Join([]string{
		entry.Label, entry.Address, entry.Port, entry.URL, entry.User, entry.Domain,
		jump, entry.Group, strings.Join(entry.Tags, " "), entry.Environment, entry.Notes, entry.Kind,
	}, " "))
}

func entrySignature(entries []metadata.Entry) uint64 {
	hash := fnv.New64a()
	for _, entry := range entries {
		_, _ = hash.Write([]byte(entry.ID))
		_, _ = hash.Write([]byte(entry.Label))
		_, _ = hash.Write([]byte(entry.Kind))
		_, _ = hash.Write([]byte(strconv.FormatInt(entry.UpdatedAt, 10)))
		if entry.Hidden {
			_, _ = hash.Write([]byte("h"))
		}
	}
	return hash.Sum64()
}

func (m *App) entryTunnelOpen(entry metadata.Entry) bool {
	if m.tunnelSup == nil {
		return false
	}
	if entry.Kind == metadata.EntryWeb {
		return entry.Via != "" && m.tunnelSup.SOCKSOpen(entry.Via)
	}
	return m.tunnelSup.RDPOpen(entry.ID)
}

func (m *App) openSelectedEntry() (tea.Model, tea.Cmd) {
	entry, ok := m.selectedEntry()
	if !ok {
		return m, nil
	}
	return m.openEntry(entry)
}

func (m *App) openEntry(entry metadata.Entry) (tea.Model, tea.Cmd) {
	var via sshconfig.Host
	if entry.Via != "" {
		host, ok := m.findHost(entry.Via)
		if !ok {
			return m, m.setNotice("Jump host " + entry.Via + " is gone. Edit the entry and choose another.")
		}
		if host.SyncSource == "vercel" {
			m.setError(errors.New("Vercel sandboxes can't tunnel Remote Desktop or web pages"))
			return m, nil
		}
		if hostlife.LooksStopped(host, m.hostMetadata()[host.Alias]) {
			return m, m.setNotice("Start " + m.hostLabel(host) + " first")
		}
		via = host
		if _, err := m.tunnelSupervisor(); err != nil {
			m.setError(err)
			return m, nil
		}
	}
	m.status, m.statusError = "Opening "+entry.Label, false
	return m, m.openEntryCmd(entry, via)
}

func (m *App) openEntryCmd(entry metadata.Entry, via sshconfig.Host) tea.Cmd {
	exe := m.bastExecutable()
	passwords := m.paths.PasswordsDir
	cache := tunnel.CacheDir(m.paths.Home)
	timeout := prepareTimeoutForHost(via)
	statusCh := make(chan string, 4)
	resultCh := make(chan entryOpenedMsg, 1)
	go func() {
		defer close(statusCh)
		if entry.Via != "" {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if err := m.ensureSyncedAccess(ctx, via, func(text string) {
				text = strings.TrimSpace(text)
				if text == "" {
					return
				}
				select {
				case statusCh <- text:
				default:
				}
			}); err != nil {
				resultCh <- entryOpenedMsg{err: err}
				return
			}
		}
		switch entry.Kind {
		case metadata.EntryRDP:
			resultCh <- m.launchRDPEntry(entry, via, exe, passwords)
		case metadata.EntryWeb:
			resultCh <- m.launchWebEntry(entry, via, exe, passwords, cache)
		default:
			resultCh <- entryOpenedMsg{err: errors.New("that entry can't be opened")}
		}
	}()
	return pollEntry(statusCh, resultCh)
}

func pollEntry(statusCh <-chan string, resultCh <-chan entryOpenedMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case text, ok := <-statusCh:
			if !ok {
				return <-resultCh
			}
			return entryStatusMsg{text: text, next: pollEntry(statusCh, resultCh)}
		case msg := <-resultCh:
			return msg
		}
	}
}

func (m *App) launchRDPEntry(entry metadata.Entry, via sshconfig.Host, exe, passwords string) entryOpenedMsg {
	session := rdp.FileSession{
		Address: entry.Address, Port: entry.Port, User: entry.User, Domain: entry.Domain,
		Fullscreen: entry.Fullscreen, Admin: entry.Admin,
	}
	if entry.Via == "" {
		cmd, err := startEntryFile(session)
		if err != nil {
			return entryOpenedMsg{err: err}
		}
		if cmd != nil {
			go func() { _ = cmd.Wait() }()
		}
		return entryOpenedMsg{id: entry.ID, notice: "Opened Remote Desktop for " + entry.Label}
	}
	target, err := rdp.DialTarget(entry.Address, entry.Port)
	if err != nil {
		return entryOpenedMsg{err: err}
	}
	port, _, err := startEntryRDP(m, entry.ID, entry.Via, target, func(cmd *exec.Cmd) {
		askpass.Prepare(cmd, exe, via, passwords)
	})
	if err != nil {
		return entryOpenedMsg{err: err}
	}
	session.Address = "127.0.0.1"
	session.Port = strconv.Itoa(port)
	cmd, err := startEntryFile(session)
	if err != nil {
		if m.tunnelSup != nil {
			m.tunnelSup.CloseRDP(entry.ID)
		}
		return entryOpenedMsg{err: err}
	}
	if m.tunnelSup != nil {
		m.tunnelSup.WatchClient(entry.ID, cmd)
	}
	return entryOpenedMsg{id: entry.ID, notice: "Opened Remote Desktop for " + entry.Label}
}

func (m *App) launchWebEntry(entry metadata.Entry, via sshconfig.Host, exe, passwords, cache string) entryOpenedMsg {
	if entry.Via == "" {
		if err := openEntryPage(entry.URL); err != nil {
			return entryOpenedMsg{err: err}
		}
		return entryOpenedMsg{id: entry.ID, notice: "Opened " + entry.Label}
	}
	port, _, err := startEntrySOCKS(m, entry.Via, func(cmd *exec.Cmd) {
		askpass.Prepare(cmd, exe, via, passwords)
	})
	if err != nil {
		return entryOpenedMsg{err: err}
	}
	if err := openProxied(cache, entry.Via, port, entry.URL); err != nil {
		return entryOpenedMsg{err: err}
	}
	return entryOpenedMsg{id: entry.ID, notice: "Opened " + entry.Label}
}

func (m *App) closeSelectedTunnel() (tea.Model, tea.Cmd) {
	entry, ok := m.selectedEntry()
	if !ok || m.tunnelSup == nil {
		return m, nil
	}
	if entry.Kind == metadata.EntryWeb {
		if entry.Via == "" {
			return m, nil
		}
		m.tunnelSup.CloseSOCKS(entry.Via)
	} else {
		m.tunnelSup.CloseRDP(entry.ID)
	}
	return m, m.setNotice("Closed tunnel for " + entry.Label)
}

func (m *App) closeOwnedTunnels() {
	if m.tunnelSup != nil {
		m.tunnelSup.CloseOwned()
	}
}

func (m *App) tunnelSupervisor() (*tunnel.Supervisor, error) {
	if m.tunnelSup != nil {
		return m.tunnelSup, nil
	}
	sup, err := tunnel.Open(filepath.Join(tunnel.CacheDir(m.paths.Home), "tunnels"), m.openSSH.SSH)
	if err != nil {
		return nil, err
	}
	m.tunnelSup = sup
	return sup, nil
}

func (m *App) ensureSyncedAccess(ctx context.Context, host sshconfig.Host, status func(string)) error {
	if !host.Synced || host.SyncID == "" || m.syncer == nil {
		return nil
	}
	var ensure func(context.Context, sshconfig.Host, func(string)) error
	switch host.SyncSource {
	case "gcp":
		ensure = m.syncer.EnsureGCPAccess
	case "aws":
		ensure = m.syncer.EnsureAWSAccess
	case "azure":
		ensure = m.syncer.EnsureAzureAccess
	case "boat":
		ensure = m.syncer.EnsureBoatAccess
	case "upstash":
		ensure = m.syncer.EnsureUpstashAccess
	case "vercel":
		ensure = m.syncer.EnsureVercelAccess
	case "hetzner":
		ensure = m.syncer.EnsureHetznerAccess
	case "bitqate":
		ensure = m.syncer.EnsureBitqateAccess
	case "railway":
		ensure = m.syncer.EnsureRailwayAccess
	case "fly":
		ensure = m.syncer.EnsureFlyAccess
	case "digitalocean":
		ensure = m.syncer.EnsureDigitalOceanAccess
	case "boxd":
		ensure = m.syncer.EnsureBoxdAccess
	}
	if ensure == nil {
		return nil
	}
	return ensure(ctx, host, status)
}

func (m *App) renderEntryRow(s styleSet, row hostRow, selected bool, rowWidth int) string {
	entry := *row.entry
	mark := "  "
	if entry.Hidden {
		mark = "◌ "
	} else if entry.Favorite {
		mark = "◆ "
	}
	indent := strings.Repeat("  ", row.depth)
	icon := ""
	if entry.Icon != "" {
		icon = entry.Icon + " "
	}
	kind := entry.Kind
	if m.entryTunnelOpen(entry) {
		kind += " ●"
	}
	selectedBg := s.selected.GetBackground()
	nameStyle := s.plain
	kindStyle := s.muted
	if selected {
		nameStyle = s.selected
		kindStyle = s.muted.Background(selectedBg)
	}
	if fg, colored := colorForeground(entry.Color); colored {
		nameStyle = nameStyle.Foreground(fg)
	}
	reserved := lipgloss.Width(indent+mark+icon) + lipgloss.Width(kind) + 3
	label := truncate(entry.Label, max(2, rowWidth-reserved))
	line := listGutter(selected, selectedBg)
	if selected {
		line += lipgloss.NewStyle().Background(selectedBg).Render(indent + mark + icon)
	} else {
		line += indent + mark + icon
	}
	line += nameStyle.Render(label) + kindStyle.Render(" "+kind)
	if pad := max(0, rowWidth-lipgloss.Width(line)); pad > 0 {
		if selected {
			line += lipgloss.NewStyle().Background(selectedBg).Render(strings.Repeat(" ", pad))
		} else {
			line += strings.Repeat(" ", pad)
		}
	}
	return line
}

func (m *App) renderEntryDetail(s styleSet, entry metadata.Entry, width int) string {
	var b strings.Builder
	titleStyle := s.active
	if fg, colored := colorForeground(entry.Color); colored {
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(fg)
	}
	icon := ""
	if entry.Icon != "" {
		icon = entry.Icon + " "
	}
	b.WriteString("  " + icon + titleStyle.Render(truncate(entry.Label, max(4, width-3-lipgloss.Width(icon)))) + "\n")
	line := "\n  " + s.title.Render(openAction)
	if m.entryTunnelOpen(entry) {
		line += " " + s.title.Render(closeTunnelAction)
	}
	b.WriteString(line + "\n\n")
	target := entry.URL
	if entry.Kind == metadata.EntryRDP {
		target = entry.Address
		if entry.Port != "" {
			target += ":" + entry.Port
		}
	}
	b.WriteString("  " + s.value.Render(truncate(target, max(4, width-3))) + "\n")
	kind := "Remote Desktop"
	if entry.Kind == metadata.EntryWeb {
		kind = "Web"
	}
	b.WriteString("  " + s.muted.Render(kind) + "\n\n")
	b.WriteString("  " + s.muted.Render("Access") + "\n")
	if entry.Via != "" {
		jump := m.viaLabel(entry.Via)
		if jump == "" {
			jump = entry.Via
		}
		b.WriteString(compactRow(s, "Jump", jump, width))
	}
	if entry.User != "" {
		b.WriteString(compactRow(s, "User", entry.User, width))
	}
	if entry.Domain != "" {
		b.WriteString(compactRow(s, "Domain", entry.Domain, width))
	}
	if entry.Fullscreen {
		b.WriteString(compactRow(s, "Window", "Full screen", width))
	}
	if entry.Admin {
		b.WriteString(compactRow(s, "Session", "Admin", width))
	}
	var about strings.Builder
	if entry.Group != "" {
		about.WriteString(compactRow(s, "Group", entry.Group, width))
	}
	if entry.Environment != "" {
		about.WriteString(compactRow(s, "Env", entry.Environment, width))
	}
	if len(entry.Tags) > 0 {
		about.WriteString(compactRow(s, "Tags", strings.Join(entry.Tags, ", "), width))
	}
	if entry.LastUsedAt != nil {
		about.WriteString(compactRow(s, "Used", usage(metadata.Host{LastUsedAt: entry.LastUsedAt, ConnectionCount: entry.ConnectionCount}), width))
	}
	if entry.Notes != "" {
		about.WriteString(compactRow(s, "Notes", entry.Notes, width))
	}
	if about.Len() > 0 {
		b.WriteString("\n")
		b.WriteString("  " + s.muted.Render("About") + "\n")
		b.WriteString(about.String())
	}
	return b.String()
}

func (m *App) entryOpenBounds(layout panelLayout) (x, y, width int, ok bool) {
	if _, selected := m.selectedEntry(); !selected {
		return 0, 0, 0, false
	}
	x, y, width = m.hostActionButtonBounds(layout, openAction)
	return x, y, width, true
}

func (m *App) entryCloseBounds(layout panelLayout) (x, y, width int, ok bool) {
	entry, selected := m.selectedEntry()
	if !selected || !m.entryTunnelOpen(entry) {
		return 0, 0, 0, false
	}
	openX, openY, openWidth := m.hostActionButtonBounds(layout, openAction)
	width = lipgloss.Width(m.styles().title.Render(closeTunnelAction))
	return openX + openWidth + 1, openY, width, true
}
