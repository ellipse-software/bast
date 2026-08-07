package ui

import (
	"fmt"
	"path/filepath"
	"testing"

	"bast/internal/metadata"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

func benchmarkApp(b *testing.B, hostCount int) *App {
	b.Helper()
	home := b.TempDir()
	store, err := metadata.Open(filepath.Join(home, "state.json"))
	if err != nil {
		b.Fatal(err)
	}
	m := &App{paths: paths.ForHome(home), metadata: store, width: 100, height: 30, dark: true}
	m.hosts = make([]sshconfig.Host, hostCount)
	hostMetadata := make(map[string]metadata.Host, hostCount)
	for i := range hostCount {
		alias := fmt.Sprintf("host-%04d", i)
		m.hosts[i] = sshconfig.Host{Alias: alias, Resolved: sshconfig.Resolved{HostName: alias + ".example"}}
		hostMetadata[alias] = metadata.Host{
			Label:       fmt.Sprintf("Server %04d", hostCount-i),
			Group:       fmt.Sprintf("Region %02d/Service %02d", i%20, i%100),
			Environment: "production",
			Tags:        []string{"web", "primary"},
		}
	}
	if err := m.metadata.UpdateHosts(func(hosts map[string]metadata.Host) {
		for alias, host := range hostMetadata {
			hosts[alias] = host
		}
	}); err != nil {
		b.Fatal(err)
	}
	m.hostMeta, m.hostMetaRevision = m.metadata.HostsSnapshot()
	return m
}

func BenchmarkHostRows1000(b *testing.B) {
	m := benchmarkApp(b, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = m.hostRows()
	}
}

func BenchmarkSortHosts1000(b *testing.B) {
	m := benchmarkApp(b, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m.sortHosts()
	}
}

func BenchmarkRenderHosts200(b *testing.B) {
	m := benchmarkApp(b, 200)
	m.width, m.height = 120, 40
	m.nerdFont = true
	_ = m.hostRows()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = m.render()
	}
}

func BenchmarkRenderTabSwitch(b *testing.B) {
	m := benchmarkApp(b, 200)
	m.width, m.height = 120, 40
	_ = m.hostRows()
	sections := []section{hostsSection, keysSection, syncSection, filesSection}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		m.section = sections[i%len(sections)]
		_ = m.render()
	}
}
