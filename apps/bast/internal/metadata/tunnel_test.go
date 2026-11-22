package metadata

import (
	"path/filepath"
	"sync"
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

func TestTunnelWritesDoNotDropAStaleSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := Tunnel{Name: "a", Host: "h", Mode: "socks", Local: "1080"}
	b := Tunnel{Name: "b", Host: "h", Mode: "socks", Local: "1081"}
	if err := first.PutTunnel(a, ""); err != nil {
		t.Fatal(err)
	}
	if err := second.PutTunnel(b, ""); err != nil {
		t.Fatal(err)
	}
	if err := second.SetHost("box", Host{Notes: "keep"}); err != nil {
		t.Fatal(err)
	}
	got, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Tunnel("a"); !ok {
		t.Fatal("stale writer dropped tunnel a")
	}
	if _, ok := got.Tunnel("b"); !ok {
		t.Fatal("stale writer dropped tunnel b")
	}
	if got.Host("box").Notes != "keep" {
		t.Fatal("host save was lost")
	}
}

func TestParallelTunnelPutsKeepBoth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	left, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := left.PutTunnel(Tunnel{Name: "left", Host: "h", Mode: "socks", Local: "1080"}, ""); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := right.PutTunnel(Tunnel{Name: "right", Host: "h", Mode: "socks", Local: "1081"}, ""); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	got, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Tunnel("left"); !ok {
		t.Fatal("missing left")
	}
	if _, ok := got.Tunnel("right"); !ok {
		t.Fatal("missing right")
	}
}
