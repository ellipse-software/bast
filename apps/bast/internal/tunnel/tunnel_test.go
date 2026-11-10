package tunnel

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestForwardArgs(t *testing.T) {
	args, err := LocalForwardArgs(43111, "[2001:db8::5]:3389", "bastion")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"ExitOnForwardFailure=yes", "ClearAllForwardings=yes", "-L 127.0.0.1:43111:[2001:db8::5]:3389", "-- bastion"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	if _, err := LocalForwardArgs(1, "10.0.0.5;rm", "bastion"); err == nil {
		t.Fatal("hostile target reached argv")
	}
	if _, err := DynamicForwardArgs(1080, "-bastion"); err == nil {
		t.Fatal("hostile alias reached argv")
	}
	configArgs, err := ConfigForwardArgs("bastion")
	if err != nil {
		t.Fatal(err)
	}
	configJoined := strings.Join(configArgs, " ")
	if !strings.Contains(configJoined, "-N") || !strings.Contains(configJoined, "ExitOnForwardFailure=yes") || strings.Contains(configJoined, "ClearAllForwardings") || strings.Contains(configJoined, "-L") {
		t.Fatalf("config args = %q", configJoined)
	}
}

func TestFailureMessageHidesSecrets(t *testing.T) {
	if got := FailureMessage("bastion", "Host key verification failed.\n"); !strings.Contains(got, "Connect to bastion once") {
		t.Fatalf("message = %q", got)
	}
	if got := FailureMessage("bastion", "Permission denied, please try again.\nsecret=hunter2"); strings.Contains(got, "hunter2") || !strings.Contains(got, "Unlock the key") {
		t.Fatalf("message = %q", got)
	}
}

func TestTunnelFixture(t *testing.T) {
	if os.Getenv("BAST_TUNNEL_FIXTURE") != "1" {
		return
	}
	if err := runTunnelFixture(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runTunnelFixture(args []string) error {
	if os.Getenv("BAST_TUNNEL_MODE") == "fail" {
		return fmt.Errorf("Host key verification failed.")
	}
	var forward string
	for i, arg := range args {
		if (arg == "-L" || arg == "-D") && i+1 < len(args) {
			forward = args[i+1]
		}
	}
	if err := noteTunnelStart(); err != nil {
		return err
	}
	parts := strings.Split(forward, ":")
	if len(parts) < 2 {
		time.Sleep(time.Hour)
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+parts[1])
	if err != nil {
		return err
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return nil
		}
		_ = conn.Close()
	}
}

func TestSupervisorReusesAndRejectsBeforeExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	script := filepath.Join(dir, "ssh")
	body := "#!/bin/sh\nexec " + shellQuote(os.Args[0]) + " -test.run=^TestTunnelFixture$ -- \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	sup, err := Open(filepath.Join(dir, "tunnels"), script)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sup.CloseOwned)
	if _, _, err := sup.RDP("entry", "bastion", "10.0.0.5;rm", nil); err == nil {
		t.Fatal("hostile target started ssh")
	}
	if _, err := os.Stat(count); err == nil {
		t.Fatal("ssh ran for a hostile target")
	}
	t.Setenv("BAST_TUNNEL_FIXTURE", "1")
	t.Setenv("BAST_TUNNEL_COUNT", count)
	port, reused, err := sup.RDP("entry", "bastion", "10.0.0.5:3389", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reused || port < 1 {
		t.Fatalf("port=%d reused=%v", port, reused)
	}
	again, reused, err := sup.RDP("entry", "bastion", "10.0.0.5:3389", nil)
	if err != nil || !reused || again != port {
		t.Fatalf("again=%d reused=%v err=%v", again, reused, err)
	}
	time.Sleep(50 * time.Millisecond)
	got, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1" {
		t.Fatalf("ssh starts = %q", got)
	}
	sup.CloseRDP("entry")
	if sup.RDPOpen("entry") {
		t.Fatal("tunnel stayed open")
	}
	socksPort, reused, err := sup.SOCKS("bastion", nil)
	if err != nil || reused || socksPort < 1 {
		t.Fatalf("socks port=%d reused=%v err=%v", socksPort, reused, err)
	}
	againSocks, reused, err := sup.SOCKS("bastion", nil)
	if err != nil || !reused || againSocks != socksPort {
		t.Fatalf("socks again=%d reused=%v err=%v", againSocks, reused, err)
	}
	time.Sleep(50 * time.Millisecond)
	got, err = os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "11" {
		t.Fatalf("ssh starts after socks reuse = %q", got)
	}
	reused, err = sup.Config("bastion", nil)
	if err != nil || reused {
		t.Fatalf("config reused=%v err=%v", reused, err)
	}
	if reused, err = sup.Config("bastion", nil); err != nil || !reused || !sup.ConfigOpen("bastion") {
		t.Fatalf("config again reused=%v open=%v err=%v", reused, sup.ConfigOpen("bastion"), err)
	}
	sup.CloseConfig("bastion")
	if sup.ConfigOpen("bastion") {
		t.Fatal("config forward stayed open")
	}
}

func TestSupervisorReportsHostKeyFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	body := "#!/bin/sh\nexec " + shellQuote(os.Args[0]) + " -test.run=^TestTunnelFixture$ -- \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	sup, err := Open(filepath.Join(dir, "tunnels"), script)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAST_TUNNEL_FIXTURE", "1")
	t.Setenv("BAST_TUNNEL_MODE", "fail")
	_, _, err = sup.RDP("entry", "bastion", "10.0.0.5:3389", nil)
	if err == nil || !strings.Contains(err.Error(), "Connect to bastion once") {
		t.Fatalf("err = %v", err)
	}
}

func noteTunnelStart() error {
	path := os.Getenv("BAST_TUNNEL_COUNT")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte("1"))
	_ = f.Close()
	return err
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
