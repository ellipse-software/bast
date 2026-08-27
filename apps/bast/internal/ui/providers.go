package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud"
	"bast/internal/sshconfig"
)

func (m *App) selectedProviderRoot() (cloud.Kind, bool) {
	group, ok := m.selectedGroupHeader()
	if !ok || !cloud.IsProviderRoot(group) {
		return "", false
	}
	return cloud.KindForGroup(group)
}

func (m *App) providerEnabled(kind cloud.Kind) bool {
	switch kind {
	case cloud.GCP:
		return m.metadata.GCP().Enabled
	case cloud.AWS:
		return m.metadata.AWS().Enabled
	case cloud.Azure:
		return m.metadata.Azure().Enabled
	case cloud.Box:
		return m.metadata.Box().Enabled
	case cloud.Upstash:
		return m.metadata.Upstash().Enabled
	case cloud.Vercel:
		return m.metadata.Vercel().Enabled
	case cloud.Hetzner:
		return m.metadata.Hetzner().Enabled
	default:
		return false
	}
}

func (m *App) shouldInjectProviderRoot(kind cloud.Kind) bool {
	if _, ok := cloud.DescriptorForKind(kind); !ok {
		return false
	}
	// Visible hosts already create the group. An empty header is only useful
	// when `.` is revealing stopped or user-hidden hosts that would otherwise
	// have no parent row. Create still lives on the Sync tab.
	if !m.showHidden || m.searchText() != "" {
		return false
	}
	hostMetadata := m.hostMetadata()
	for _, host := range m.hosts {
		if !host.Synced || host.SyncSource != string(kind) {
			continue
		}
		meta := hostMetadata[host.Alias]
		if meta.Hidden || hostLooksStopped(host, meta) {
			return true
		}
	}
	return false
}

func (m *App) providerGroupStats(group string) (running, stopped int) {
	hostMetadata := m.hostMetadata()
	for _, host := range m.hosts {
		g := strings.TrimSpace(hostMetadata[host.Alias].Group)
		if g != group && !strings.HasPrefix(g, group+"/") {
			continue
		}
		if hostLooksStopped(host, hostMetadata[host.Alias]) {
			stopped++
		} else {
			running++
		}
	}
	return running, stopped
}

func (m *App) providerGroupPrimaryAction(kind cloud.Kind) string {
	if cloud.CapabilitiesFor(kind).Create {
		if kind == cloud.Box || kind == cloud.Upstash {
			return " New box "
		}
		if kind == cloud.Vercel {
			return " New sandbox "
		}
		return " New "
	}
	return " Sync now "
}

func (m *App) hostLifecycleKind(host sshconfig.Host) (cloud.Kind, bool) {
	if !host.Synced {
		return "", false
	}
	kind, ok := cloud.KindForSource(host.SyncSource)
	if !ok {
		return "", false
	}
	return kind, true
}

func (m *App) hostHasCapability(host sshconfig.Host, check func(cloud.Capabilities) bool) bool {
	kind, ok := m.hostLifecycleKind(host)
	if !ok {
		return false
	}
	return check(cloud.CapabilitiesFor(kind))
}

func (m *App) stopSyncedHost(host sshconfig.Host) (tea.Model, tea.Cmd) {
	if !m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Stop }) {
		return m, nil
	}
	if m.syncingProviders[host.SyncSource] {
		return m, m.setNotice("Operation already in progress")
	}
	if m.hostLooksStopped(host) {
		return m, m.setNotice("Already stopped")
	}
	switch host.SyncSource {
	case "upstash":
		m.openUpstashStopForm(host)
	case "box":
		m.openBoxStopForm(host)
	case "vercel":
		m.openVercelStopForm(host)
	case "hetzner":
		m.openHetznerStopForm(host)
	default:
		return m, nil
	}
	return m, nil
}

func (m *App) restartSyncedHost(host sshconfig.Host) (tea.Model, tea.Cmd) {
	if !m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Restart }) {
		return m, nil
	}
	if m.syncingProviders[host.SyncSource] {
		return m, m.setNotice("Operation already in progress")
	}
	if m.hostLooksStopped(host) {
		return m, m.setNotice("Start the server before restarting")
	}
	if host.SyncSource == "hetzner" {
		m.openHetznerRestartForm(host)
		return m, nil
	}
	return m, nil
}

func (m *App) forkSyncedHost(host sshconfig.Host) (tea.Model, tea.Cmd) {
	if !m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Fork }) {
		return m, nil
	}
	if m.syncingProviders[host.SyncSource] {
		return m, m.setNotice("Operation already in progress")
	}
	switch host.SyncSource {
	case "upstash":
		m.openUpstashForkForm(host)
	case "box":
		m.openBoxForkForm(host)
	case "vercel":
		m.openVercelForkForm(host)
	default:
		return m, nil
	}
	return m, nil
}

func (m *App) deleteSyncedHost(host sshconfig.Host) bool {
	if !m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Delete }) {
		return false
	}
	switch host.SyncSource {
	case "upstash":
		m.openUpstashDeleteForm(host)
	case "vercel":
		m.openVercelDeleteForm(host)
	default:
		return false
	}
	return true
}

func (m *App) resumeSyncedHost(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if !m.hostLooksStopped(host) || !m.hostHasCapability(host, func(c cloud.Capabilities) bool { return c.Start }) {
		return nil
	}
	switch host.SyncSource {
	case "upstash":
		return m.resumeSelectedUpstash(host, thenConnect)
	case "box":
		return m.resumeSelectedBox(host, thenConnect)
	case "vercel":
		return m.resumeSelectedVercel(host, thenConnect)
	case "hetzner":
		return m.startSelectedHetzner(host, thenConnect)
	default:
		return nil
	}
}

func (m *App) runProviderGroupPrimary(kind cloud.Kind) (tea.Model, tea.Cmd) {
	if cloud.CapabilitiesFor(kind).Create {
		return m.runProviderGroupCreate(kind)
	}
	return m.syncProviderFromHosts(kind)
}

func (m *App) runProviderGroupCreate(kind cloud.Kind) (tea.Model, tea.Cmd) {
	switch kind {
	case cloud.Box:
		if m.syncingProviders["box"] {
			return m, m.setNotice("Box operation already in progress")
		}
		m.openBoxNewForm()
		return m, nil
	case cloud.Upstash:
		if m.syncingProviders["upstash"] {
			return m, m.setNotice("Upstash operation already in progress")
		}
		if !m.upstashHasKey() {
			m.openUpstashKeyForm()
			return m, nil
		}
		m.openUpstashNewForm()
		return m, nil
	case cloud.Vercel:
		if m.syncingProviders["vercel"] {
			return m, m.setNotice("Vercel operation already in progress")
		}
		if !m.vercelReady() {
			m.openVercelTokenForm()
			return m, nil
		}
		m.openVercelNewForm()
		return m, nil
	default:
		return m, m.setNotice("Create is not available for this provider yet")
	}
}

func (m *App) syncProviderFromHosts(kind cloud.Kind) (tea.Model, tea.Cmd) {
	if m.anySyncing() {
		return m, m.setNotice("Sync already in progress")
	}
	previous := m.syncProvider
	m.syncProvider = string(kind)
	model, cmd := m.runSyncAction("sync")
	m.syncProvider = previous
	return model, cmd
}
