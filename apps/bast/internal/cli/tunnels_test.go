package cli

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestTunnelAddListEditRemove(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	run := func(args ...string) (string, string, error) { return runTestCLI(t, home, client, args...) }

	if _, _, err := run("tunnel", "add", "prod-db", "--host", "bastion", "--mode", "local", "--local", "5432", "--remote", "db:5432", "--group", "prod"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("tunnel", "add", "prod-web", "--host", "bastion", "--mode", "socks", "--local", "1080", "--group", "prod"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("tunnel", "add", "dev", "--host", "bastion", "--mode", "socks", "--local", "1081"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("tunnel", "add", "dev", "--host", "bastion", "--mode", "socks", "--local", "1"); err == nil {
		t.Fatal("duplicate tunnel accepted")
	}

	out, _, err := run("tunnel", "list")
	if err != nil || strings.Count(out, "\n") != 3 || !strings.Contains(out, "configured") {
		t.Fatalf("list = %q err=%v", out, err)
	}
	out, _, err = run("tunnel", "l", "--group", "prod")
	if err != nil || strings.Count(out, "\n") != 2 || strings.Contains(out, "dev\t") {
		t.Fatalf("group list = %q err=%v", out, err)
	}

	_, errOut, err := run("tunnel", "add", "bad", "--host", "bastion", "--mode", "local", "--local", "5432")
	if err == nil || !strings.Contains(errOut, `tunnel "bad": remote:`) {
		t.Fatalf("validation error = %q err=%v", errOut, err)
	}

	if _, _, err := run("tunnel", "edit", "dev", "--local", "1090"); err != nil {
		t.Fatal(err)
	}
	out, _, _ = run("--json", "tunnel", "list")
	if !strings.Contains(out, `"local":"1090"`) {
		t.Fatalf("edit not stored: %s", out)
	}
	if _, _, err := run("tunnel", "edit", "dev", "--open-with-host"); err != nil {
		t.Fatal(err)
	}
	out, _, _ = run("--json", "tunnel", "list")
	if !strings.Contains(out, `"name":"dev"`) || !strings.Contains(out, `"openWithHost":true`) {
		t.Fatalf("open with host not stored: %s", out)
	}
	if _, _, err := run("tunnel", "edit", "dev", "--clear-open-with-host"); err != nil {
		t.Fatal(err)
	}
	out, _, _ = run("--json", "tunnel", "list")
	if strings.Contains(out, `"openWithHost"`) {
		t.Fatalf("clear left openWithHost: %s", out)
	}
	if _, _, err := run("tunnel", "edit", "dev", "--open-with-host=false"); err != nil {
		t.Fatal(err)
	}
	out, _, _ = run("--json", "tunnel", "list")
	if strings.Contains(out, `"openWithHost"`) {
		t.Fatalf("explicit false stored openWithHost: %s", out)
	}

	// Closing tunnels that are not running is safe, by name, glob, and group.
	for _, sel := range [][]string{{"dev"}, {"prod-*"}, {"--group", "prod"}, {"--all"}} {
		args := append([]string{"tunnel", "close"}, sel...)
		if out, _, err := run(args...); err != nil || !strings.Contains(out, "not running") {
			t.Fatalf("close %v = %q err=%v", sel, out, err)
		}
	}
	if _, errOut, err := run("tunnel", "close", "nope-*"); err == nil || !strings.Contains(errOut, "no tunnel matches") {
		t.Fatalf("unknown glob: %q %v", errOut, err)
	}
	if _, _, err := run("tunnel", "close"); err == nil {
		t.Fatal("close without a selector should fail")
	}

	if _, _, err := run("tunnel", "remove", "prod-*"); err != nil {
		t.Fatal(err)
	}
	out, _, _ = run("tunnel", "list")
	if strings.Count(out, "\n") != 1 || !strings.HasPrefix(out, "dev\t") {
		t.Fatalf("after remove: %q", out)
	}
}

func TestTunnelOpenReportsPortConflict(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	if _, _, err := runTestCLI(t, home, client, "hosts", "add", "bastion", "--hostname", "b.example"); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	if _, _, err := runTestCLI(t, home, client, "tunnel", "add", "busy", "--host", "bastion", "--mode", "socks", "--local", port); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := runTestCLI(t, home, client, "tunnel", "open", "busy")
	if err == nil || !strings.Contains(errOut, "already in use") {
		t.Fatalf("open = %q err=%v", errOut, err)
	}
	_, errOut, err = runTestCLI(t, home, client, "tunnel", "add", "ghost", "--host", "missing", "--mode", "socks", "--local", "1")
	if err != nil {
		t.Fatal(err)
	}
	_, errOut, err = runTestCLI(t, home, client, "tunnel", "open", "ghost")
	if err == nil || !strings.Contains(errOut, "not in your SSH config") {
		t.Fatalf("missing host: %q err=%v", errOut, err)
	}
	out, errOut, err := runTestCLI(t, home, client, "--json", "tunnel", "open", "ghost")
	if err == nil || strings.Contains(out, `"ok":true`) || !strings.Contains(errOut, `"ok":false`) || !strings.Contains(errOut, "not in your SSH config") {
		t.Fatalf("json open out=%q errOut=%q err=%v", out, errOut, err)
	}
}

func TestTunnelCompletion(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	if _, _, err := runTestCLI(t, home, client, "tunnel", "add", "prod-db", "--host", "b", "--mode", "socks", "--local", "1080", "--group", "prod"); err != nil {
		t.Fatal(err)
	}
	values, _ := runComplete(t, home, "tunnel", "open", "")
	if !contains(values, "prod-db") {
		t.Fatalf("names = %v", values)
	}
	values, _ = runComplete(t, home, "tunnel", "open", "--group", "")
	if !contains(values, "prod") {
		t.Fatalf("groups = %v", values)
	}
	values, _ = runComplete(t, home, "tunnel", "open", "prod-db", "")
	if !contains(values, "prod-db") {
		t.Fatalf("second name = %v", values)
	}
	values, _ = runComplete(t, home, "tunnel", "close", "prod-db", "prod-")
	if !contains(values, "prod-db") {
		t.Fatalf("close names = %v", values)
	}
	values, _ = runComplete(t, home, "tunnel", "")
	if !contains(values, "open") || !contains(values, "l") {
		t.Fatalf("subcommands = %v", values)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want || strings.HasPrefix(v, want+"\t") {
			return true
		}
	}
	return false
}
