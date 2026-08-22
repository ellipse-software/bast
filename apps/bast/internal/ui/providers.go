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
	default:
		return false
	}
}

func (m *App) shouldInjectProviderRoot(kind cloud.Kind) bool {
	if _, ok := cloud.DescriptorForKind(kind); !ok {
		return false
	}
	if cloud.CapabilitiesFor(kind).Create && m.providerEnabled(kind) {
		return true
	}
	for _, host := range m.hosts {
		if host.Synced && host.SyncSource == string(kind) {
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
		if kind == cloud.Box {
			return " New box "
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

func (m *App) runProviderGroupPrimary(kind cloud.Kind) (tea.Model, tea.Cmd) {
	if cloud.CapabilitiesFor(kind).Create {
		return m.runProviderGroupCreate(kind)
	}
	return m.syncProviderFromHosts(kind)
}

func (m *App) runProviderGroupCreate(kind cloud.Kind) (tea.Model, tea.Cmd) {
	if kind != cloud.Box {
		return m, m.setNotice("Create is not available for this provider yet")
	}
	if m.syncingProviders["box"] {
		return m, m.setNotice("Box operation already in progress")
	}
	m.openBoxNewForm()
	return m, nil
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
