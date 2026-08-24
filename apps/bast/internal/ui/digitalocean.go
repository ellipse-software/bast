package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	docloud "bast/internal/cloud/digitalocean"
	"bast/internal/sshconfig"
)

func (m *App) resumeSelectedDigitalOcean(host sshconfig.Host, thenConnect bool) tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncingProviders["digitalocean"] {
		return m.setNotice("DigitalOcean operation already in progress")
	}
	if !m.hostLooksStopped(host) {
		return m.setNotice("Droplet is already running")
	}
	if strings.TrimSpace(host.SyncID) == "" {
		return m.setNotice("DigitalOcean sync id missing; sync first")
	}
	opGen := m.beginProviderOp("digitalocean")
	m.syncActivity = "powering on…"
	if thenConnect {
		m.boxConnectAfter = host.Alias
	} else {
		m.boxConnectAfter = ""
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := m.syncer.ResumeDigitalOcean(ctx, host.SyncID)
		return syncDoneMsg{provider: "digitalocean", result: result, err: err, opGen: opGen}
	}
}

func (m *App) openDigitalOceanNewForm() {
	defaults := docloud.DefaultNewOpts()
	m.openForm("New Droplet", "digitalocean_new", []field{
		{label: "Name", placeholder: "web-01"},
		{
			label: "Region", value: defaults.Region, selected: 1,
			options: []fieldOption{
				{label: "nyc1", value: "nyc1"},
				{label: "nyc3", value: "nyc3"},
				{label: "sfo3", value: "sfo3"},
				{label: "ams3", value: "ams3"},
				{label: "sgp1", value: "sgp1"},
				{label: "lon1", value: "lon1"},
				{label: "fra1", value: "fra1"},
				{label: "tor1", value: "tor1"},
				{label: "blr1", value: "blr1"},
				{label: "syd1", value: "syd1"},
			},
		},
		{
			label: "Size", value: defaults.Size, selected: 0,
			options: []fieldOption{
				{label: "s-1vcpu-1gb", value: "s-1vcpu-1gb"},
				{label: "s-1vcpu-2gb", value: "s-1vcpu-2gb"},
				{label: "s-2vcpu-2gb", value: "s-2vcpu-2gb"},
				{label: "s-2vcpu-4gb", value: "s-2vcpu-4gb"},
				{label: "s-4vcpu-8gb", value: "s-4vcpu-8gb"},
			},
		},
		{
			label: "Image", value: defaults.Image, selected: 0,
			options: []fieldOption{
				{label: "ubuntu-24-04-x64", value: "ubuntu-24-04-x64"},
				{label: "ubuntu-22-04-x64", value: "ubuntu-22-04-x64"},
				{label: "debian-12-x64", value: "debian-12-x64"},
				{label: "fedora-coreos-stable", value: "fedora-coreos-stable"},
			},
		},
		{label: "Context", description: "Blank uses the current doctl context", optional: true, placeholder: "default"},
	})
}

func (m *App) openDigitalOceanStopForm(host sshconfig.Host) {
	if m.hostLooksStopped(host) {
		m.setError(errString("droplet is already powered off"))
		return
	}
	m.openForm("Stop Droplet", "digitalocean_stop", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type stop to confirm", description: "Powers off. Disk still bills until you delete", placeholder: "stop"},
	})
}

func (m *App) openDigitalOceanForkForm(host sshconfig.Host) {
	m.openForm("Fork Droplet", "digitalocean_fork", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type fork to confirm", description: "Snapshots this Droplet and creates a new one from it", placeholder: "fork"},
	})
}

func (m *App) openDigitalOceanDeleteForm(host sshconfig.Host) {
	label := m.hostLabel(host)
	m.openForm("Delete Droplet: "+label, "digitalocean_delete", []field{
		{label: "SyncID", value: host.SyncID, hidden: true},
		{label: "Type delete to confirm", description: "Permanently destroys the Droplet; snapshots are kept", placeholder: "delete"},
	})
}
