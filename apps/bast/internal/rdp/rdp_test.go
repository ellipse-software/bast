package rdp

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandPerOS(t *testing.T) {
	session := Session{Address: "rdp.example"}
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "windows", name: "mstsc", args: []string{"/v:rdp.example"}},
		{goos: "darwin", name: "open", args: []string{"rdp://full%20address=s:rdp.example"}},
		{goos: "linux", name: "xdg-open", args: []string{"rdp://rdp.example"}},
		{goos: "freebsd", name: "xdg-open", args: []string{"rdp://rdp.example"}},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			name, args, err := Command(test.goos, session)
			if err != nil {
				t.Fatal(err)
			}
			if name != test.name || strings.Join(args, "\x00") != strings.Join(test.args, "\x00") {
				t.Fatalf("command = %s %q, want %s %q", name, args, test.name, test.args)
			}
			if name == "sh" || name == "cmd" || name == "bash" || strings.Contains(strings.Join(args, " "), " -c ") {
				t.Fatalf("command was built as a shell string: %s %q", name, args)
			}
		})
	}
}

func TestCommandCanonicalizesAddressAndReservesCredentials(t *testing.T) {
	name, args, err := Command("darwin", Session{
		Address: "2001:db8::1",
		User:    "alice",
		Domain:  "CORP",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "open" || len(args) != 1 || args[0] != "rdp://full%20address=s:[2001:db8::1]&username=s:alice&domain=s:CORP" {
		t.Fatalf("darwin command = %s %q", name, args)
	}

	_, args, err = Command("linux", Session{Address: "[2001:db8::1]:3389", User: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 || args[0] != "rdp://full%20address=s:[2001:db8::1]:3389&username=s:alice" {
		t.Fatalf("linux credentials changed the short URL incorrectly: %q", args)
	}

	_, args, err = Command("windows", Session{Address: "10.0.0.8:3389", User: "alice", Domain: "CORP"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 || args[0] != "/v:10.0.0.8:3389" {
		t.Fatalf("windows command = %q", args)
	}

	_, args, err = Command("linux", Session{Address: "FILE_SERVER"})
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "rdp://FILE_SERVER" {
		t.Fatalf("hostname = %q", args[0])
	}

	// An unbracketed IPv6 value is an address, not an address plus a port.
	_, args, err = Command("windows", Session{Address: "2001:db8::1:3389"})
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "/v:[2001:db8::1:3389]" {
		t.Fatalf("bare IPv6 = %q", args[0])
	}
}

func TestCommandRejectsInjectionAndBadAddresses(t *testing.T) {
	rejected := []string{
		"",
		"   ",
		"example.com;id",
		"example.com && id",
		"example.com|id",
		"`id`",
		"$(id)",
		"example.com\nid",
		"exam\rple.com",
		"-oProxyCommand=evil",
		"example.com > out",
		"example.com'bad",
		"example.com\"bad",
		"example.com$(bad)",
		"example.com%00",
		"host name",
		"example.com:0",
		"example.com:65536",
		"example.com:03389",
		"example.com/path",
		"..",
		".",
		"-host",
	}
	for _, address := range rejected {
		t.Run(address, func(t *testing.T) {
			name, args, err := Command("linux", Session{Address: address})
			if err == nil || name != "" || args != nil {
				t.Fatalf("accepted %q as %s %q (%v)", address, name, args, err)
			}
			if strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), address) && strings.ContainsAny(address, "\r\n;&|`$") {
				t.Fatalf("error leaked the address or a process lookup: %v", err)
			}
		})
	}
	if _, _, err := Command("darwin", Session{Address: "ok.example", User: "alice&admin"}); err == nil {
		t.Fatal("accepted a username that could extend the URL")
	}
	if _, _, err := Command("windows", Session{Address: "ok.example", Domain: "CORP;calc"}); err == nil {
		t.Fatal("accepted a domain that could extend the command")
	}
}

func TestLaunchUsesArgvAndFriendlyErrors(t *testing.T) {
	origLook, origStart, origCombined := lookPathFn, startProcessFn, combinedFn
	t.Cleanup(func() {
		lookPathFn, startProcessFn, combinedFn = origLook, origStart, origCombined
	})

	t.Run("windows", func(t *testing.T) {
		var gotName string
		var gotArgs []string
		looked := false
		lookPathFn = func(name string) (string, error) {
			looked = true
			if name != "mstsc" {
				t.Fatalf("looked up %s", name)
			}
			return name, nil
		}
		startProcessFn = func(name string, args ...string) error {
			gotName, gotArgs = name, append([]string(nil), args...)
			return nil
		}
		combinedFn = func(string, ...string) ([]byte, error) {
			t.Fatal("windows launch waited on combined output")
			return nil, nil
		}
		if err := launch("windows", Session{Address: "rdp.example"}); err != nil {
			t.Fatal(err)
		}
		if !looked || gotName != "mstsc" || len(gotArgs) != 1 || gotArgs[0] != "/v:rdp.example" {
			t.Fatalf("started %s %q", gotName, gotArgs)
		}
	})

	t.Run("windows missing", func(t *testing.T) {
		lookPathFn = func(string) (string, error) {
			return "", exec.ErrNotFound
		}
		startProcessFn = func(string, ...string) error {
			t.Fatal("started a missing client")
			return nil
		}
		err := launch("windows", Session{Address: "rdp.example"})
		if err == nil || strings.Contains(err.Error(), "executable file not found") || !strings.Contains(err.Error(), "mstsc") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("darwin", func(t *testing.T) {
		var gotName string
		var gotArgs []string
		combinedFn = func(name string, args ...string) ([]byte, error) {
			gotName, gotArgs = name, append([]string(nil), args...)
			return nil, nil
		}
		if err := launch("darwin", Session{Address: "rdp.example"}); err != nil {
			t.Fatal(err)
		}
		if gotName != "open" || len(gotArgs) != 1 || gotArgs[0] != "rdp://full%20address=s:rdp.example" {
			t.Fatalf("opened %s %q", gotName, gotArgs)
		}
	})

	t.Run("linux missing handler", func(t *testing.T) {
		combinedFn = func(string, ...string) ([]byte, error) {
			return []byte("executable file not found"), errors.New("exit status 2")
		}
		err := launch("linux", Session{Address: "rdp.example"})
		if err == nil || strings.Contains(err.Error(), "executable file not found") || !strings.Contains(err.Error(), "xdg-utils") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid does not exec", func(t *testing.T) {
		startProcessFn = func(string, ...string) error {
			t.Fatal("started from an invalid address")
			return nil
		}
		combinedFn = func(string, ...string) ([]byte, error) {
			t.Fatal("opened from an invalid address")
			return nil, nil
		}
		if err := launch("linux", Session{Address: "bad;rm"}); err == nil {
			t.Fatal("expected a validation error")
		}
	})
}

func TestEscapeRDPValue(t *testing.T) {
	if got := escapeRDPValue("alice"); got != "alice" {
		t.Fatalf("plain = %q", got)
	}
	if got := escapeRDPValue("a b&="); got != "a%20b%26%3D" {
		t.Fatalf("escaped = %q", got)
	}
}
