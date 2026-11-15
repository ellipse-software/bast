package metadata

import (
	"path/filepath"
	"testing"
)

func TestTunnelStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := Tunnel{Name: "prod-db", Host: "bastion", Mode: "local", Local: "5432", Remote: "db:5432", Group: "prod"}
	if err := s.PutTunnel(a, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTunnel(a, ""); err == nil {
		t.Fatal("duplicate name accepted")
	}
	renamed := a
	renamed.Name = "prod-pg"
	if err := s.PutTunnel(renamed, "prod-db"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Tunnels()
	if len(got) != 1 || got[0].Name != "prod-pg" || got[0].Group != "prod" {
		t.Fatalf("tunnels = %+v", got)
	}
	if err := reopened.DeleteTunnel("prod-pg"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteTunnel("prod-pg"); err == nil {
		t.Fatal("deleting a missing tunnel succeeded")
	}
	if len(reopened.Tunnels()) != 0 {
		t.Fatal("tunnel survived delete")
	}
}
