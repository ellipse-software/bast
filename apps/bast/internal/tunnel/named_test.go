package tunnel

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func argsOf(t *testing.T, s Spec) string {
	t.Helper()
	a, err := Args(s)
	if err != nil {
		t.Fatalf("%+v: %v", s, err)
	}
	return strings.Join(a, " ")
}

func TestArgsEveryMode(t *testing.T) {
	cases := []struct {
		spec Spec
		want []string
	}{
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "8080", Remote: "db:5432"}, []string{"-L 127.0.0.1:8080:db:5432", "ServerAliveInterval=30", "-- h"}},
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "0.0.0.0:8080", Remote: "db:5432", KeepAlive: 10}, []string{"-L 0.0.0.0:8080:db:5432", "ServerAliveInterval=10"}},
		{Spec{Name: "a", Host: "h", Mode: ModeRemote, Local: "3000", Remote: "9000"}, []string{"-R 9000:localhost:3000"}},
		{Spec{Name: "a", Host: "h", Mode: ModeSOCKS, Local: "1080"}, []string{"-D 127.0.0.1:1080"}},
		{Spec{Name: "a", Host: "h", Mode: ModeSOCKSRemote, Remote: "1080"}, []string{"-R 1080"}},
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "/tmp/l.sock", Remote: "/run/r.sock"}, []string{"-L /tmp/l.sock:/run/r.sock"}},
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "8080", Remote: "/run/db.sock"}, []string{"-L 127.0.0.1:8080:/run/db.sock"}},
		{Spec{Name: "a", Host: "h", Mode: ModeRemote, Local: "/tmp/app.sock", Remote: "/run/app.sock"}, []string{"-R /run/app.sock:/tmp/app.sock"}},
	}
	for _, c := range cases {
		got := argsOf(t, c.spec)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %q in %q", c.spec.Mode, w, got)
			}
		}
		if strings.Contains(got, "ClearAllForwardings") {
			t.Errorf("ClearAllForwardings would drop the forwards: %q", got)
		}
	}
}

func TestArgsRangesAndLists(t *testing.T) {
	got := argsOf(t, Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "8000-8002", Remote: "db:9000-9002"})
	for _, w := range []string{"-L 127.0.0.1:8000:db:9000", "-L 127.0.0.1:8001:db:9001", "-L 127.0.0.1:8002:db:9002"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
	got = argsOf(t, Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "80,81", Remote: "a:1,b:2"})
	if !strings.Contains(got, "127.0.0.1:80:a:1") || !strings.Contains(got, "127.0.0.1:81:b:2") {
		t.Errorf("list: %q", got)
	}
}

func TestValidateNamesTunnelAndField(t *testing.T) {
	cases := []struct {
		spec  Spec
		field string
	}{
		{Spec{Name: "", Host: "h", Mode: ModeSOCKS, Local: "1"}, "name"},
		{Spec{Name: "a b", Host: "h", Mode: ModeSOCKS, Local: "1"}, "name"},
		{Spec{Name: "a", Host: "-oProxyCommand=x", Mode: ModeSOCKS, Local: "1"}, "host"},
		{Spec{Name: "a", Host: "h", Mode: "bogus"}, "mode"},
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "8080"}, "remote"},
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "99999", Remote: "x:1"}, "local"},
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "80-81", Remote: "x:1"}, "remote"},
		{Spec{Name: "a", Host: "h", Mode: ModeLocal, Local: "80", Remote: "nohostport"}, "remote"},
		{Spec{Name: "a", Host: "h", Mode: ModeSOCKS, Local: "1080", Remote: "x:1"}, "remote"},
		{Spec{Name: "a", Host: "h", Mode: ModeSOCKS, Local: "/tmp/s"}, "local"},
		{Spec{Name: "a", Host: "h", Mode: ModeSOCKS, Local: "1;rm"}, "local"},
		{Spec{Name: "a", Host: "h", Mode: ModeSOCKS, Local: "1", KeepAlive: -1}, "keep_alive"},
	}
	for _, c := range cases {
		err := Validate(c.spec)
		if err == nil {
			t.Errorf("%+v: expected error", c.spec)
			continue
		}
		if !strings.Contains(err.Error(), "tunnel \""+c.spec.Name+"\"") || !strings.Contains(err.Error(), c.field+":") {
			t.Errorf("%+v: error %q should name tunnel and %s", c.spec, err, c.field)
		}
	}
}

func TestMatchGlobs(t *testing.T) {
	names := []string{"prod-db", "prod-web", "dev-db"}
	got, missing := Match([]string{"prod-*", "dev-db"}, names)
	if strings.Join(got, ",") != "dev-db,prod-db,prod-web" || len(missing) != 0 {
		t.Fatalf("got %v missing %v", got, missing)
	}
	if _, missing := Match([]string{"nope*", "x"}, names); len(missing) != 2 {
		t.Fatalf("missing = %v", missing)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]ErrorKind{
		"Permission denied (publickey).":                               KindAuth,
		"ssh: Could not resolve hostname x: Name or service not known": KindDNS,
		"bind [127.0.0.1]:80: Address already in use":                  KindPort,
		"connect to host x port 22: Connection refused":                KindNetwork,
		"Host key verification failed.":                                KindHostKey,
		"something odd":                                                KindOther,
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Errorf("%q = %s, want %s", in, got, want)
		}
	}
}

func fakeSSH(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	path := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitState(t *testing.T, m *Manager, name, state string) Status {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if st := m.Status(name); st.State == state {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never reached %s; now %+v", name, state, m.Status(name))
	return Status{}
}

func TestRunReconnectsDedupesAndStops(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "n")
	// First attempt drops with a network error; later attempts stay up.
	ssh := fakeSSH(t, `n=$(cat "`+counter+`" 2>/dev/null || echo 0); echo $((n+1)) > "`+counter+`"
if [ "$n" = 0 ]; then echo "Connection reset by peer" >&2; exit 255; fi
exec sleep 30
`)
	m, err := NewManager(t.TempDir(), "unused")
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Name: "t1", Host: "h", Mode: ModeSOCKS, Local: "1080"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx, spec, ssh, nil) }()

	st := waitState(t, m, "t1", StateReconnecting)
	if st.Kind != KindNetwork {
		t.Fatalf("kind = %s", st.Kind)
	}
	st = waitState(t, m, "t1", StateConnected)
	if st.Attempts < 2 {
		t.Fatalf("attempts = %d", st.Attempts)
	}
	if err := m.Start(spec); err != ErrRunning {
		t.Fatalf("second start = %v, want ErrRunning", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := m.Status("t1").State; got != StateConfigured {
		t.Fatalf("state after close = %s", got)
	}
	if m.Running("t1") {
		t.Fatal("still running after close")
	}
}

func TestRunStopsOnAuthFailure(t *testing.T) {
	ssh := fakeSSH(t, "echo 'Permission denied (publickey).' >&2\nexit 255\n")
	m, _ := NewManager(t.TempDir(), "unused")
	err := m.Run(context.Background(), Spec{Name: "t2", Host: "h", Mode: ModeSOCKS, Local: "1080"}, ssh, nil)
	if err == nil || !strings.Contains(err.Error(), "auth") {
		t.Fatalf("err = %v", err)
	}
	st := m.Status("t2")
	if st.State != StateError || st.Kind != KindAuth {
		t.Fatalf("status = %+v", st)
	}
}

func TestStartReportsPortConflict(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	m, _ := NewManager(t.TempDir(), "unused")
	err = m.Start(Spec{Name: "c", Host: "h", Mode: ModeLocal, Local: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Remote: "x:1"})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("err = %v", err)
	}
}

func TestStopIsIdempotent(t *testing.T) {
	m, _ := NewManager(t.TempDir(), "unused")
	m.Stop("never-started")
	m.Stop("never-started")
}

func TestStartStopLifecycle(t *testing.T) {
	dir := t.TempDir()
	m, _ := NewManager(dir, "")
	m.Exe = fakeSSH(t, `echo '{"name":"life","pid":'$$',"state":"connected","since":"2026-01-01T00:00:00Z"}' > "$STATE"
exec sleep 30
`)
	t.Setenv("STATE", m.statePath("life"))
	free, err := listenPort()
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{Name: "life", Host: "h", Mode: ModeSOCKS, Local: strconv.Itoa(free)}
	if err := m.Start(spec); err != nil {
		t.Fatal(err)
	}
	if !m.Running("life") {
		t.Fatal("not running after Start")
	}
	if err := m.Start(spec); err != ErrRunning {
		t.Fatalf("duplicate start = %v", err)
	}
	m.Stop("life")
	if m.Running("life") {
		t.Fatal("still running after Stop")
	}
	m.Stop("life")
}
