package hostlife

import (
	"testing"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func TestLooksStopped(t *testing.T) {
	host := sshconfig.Host{Synced: true, SyncSource: "hetzner"}
	if !LooksStopped(host, metadata.Host{Tags: []string{"state:off"}}) {
		t.Fatal("stopped hetzner host was accepted")
	}
	if LooksStopped(host, metadata.Host{Tags: []string{"state:running"}}) {
		t.Fatal("running hetzner host was stopped")
	}
	if LooksStopped(sshconfig.Host{Alias: "local"}, metadata.Host{}) {
		t.Fatal("local host was stopped")
	}
}
