package ui

import (
	"testing"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func TestForwardUsesOpenWithHostTunnels(t *testing.T) {
	m := testApp(t)
	host := sshconfig.Host{Alias: "alpha"}
	if m.hostHasForwards(host) || m.forwardOpen("alpha") {
		t.Fatal("empty host offered a forward")
	}
	if err := m.metadata.PutTunnel(metadata.Tunnel{Name: "manual", Host: "alpha", Mode: "local", Local: "8080", Remote: "localhost:80"}, ""); err != nil {
		t.Fatal(err)
	}
	if m.hostHasForwards(host) || m.forwardOpen("alpha") {
		t.Fatal("a manual tunnel should not use Forward or Close")
	}
	if err := m.metadata.PutTunnel(metadata.Tunnel{Name: "web", Host: "alpha", Mode: "local", Local: "9090", Remote: "localhost:90", OpenWithHost: true}, ""); err != nil {
		t.Fatal(err)
	}
	if !m.hostHasForwards(host) || m.forwardOpen("alpha") {
		t.Fatal("an idle open-with-host tunnel should offer Forward only")
	}
}
