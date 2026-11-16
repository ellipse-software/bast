package rdp

import (
	"strings"
	"testing"
)

func TestRenderFileCarriesSettingsAndNoPassword(t *testing.T) {
	body, err := RenderFile(FileSession{
		Address: "10.0.0.5", User: `CORP\alice`, Admin: true, Fullscreen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"full address:s:10.0.0.5",
		"server port:i:3389",
		"username:s:alice",
		"domain:s:CORP",
		"prompt for credentials:i:1",
		"administrative session:i:1",
		"screen mode id:i:2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %q", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "password") {
		t.Fatalf("password leaked into %q", body)
	}

	tunneled, err := RenderFile(FileSession{Address: "127.0.0.1", Port: "43123"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tunneled, "full address:s:127.0.0.1") || !strings.Contains(tunneled, "server port:i:43123") || strings.Contains(tunneled, "10.0.0.5") {
		t.Fatalf("tunneled file = %q", tunneled)
	}

	v6, err := RenderFile(FileSession{Address: "2001:db8::1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v6, "full address:s:2001:db8::1") || strings.Contains(v6, "[2001:db8::1]") {
		t.Fatalf("ipv6 file = %q", v6)
	}
}

func TestDialTargetRejectsInjection(t *testing.T) {
	for _, address := range []string{"10.0.0.5 -o ProxyCommand=x", "-evil", "bad;rm"} {
		if _, err := DialTarget(address, "3389"); err == nil {
			t.Fatalf("accepted %q", address)
		}
	}
	got, err := DialTarget("10.0.0.5", "")
	if err != nil || got != "10.0.0.5:3389" {
		t.Fatalf("target = %q err=%v", got, err)
	}
	got, err = DialTarget("2001:db8::5", "3389")
	if err != nil || got != "[2001:db8::5]:3389" {
		t.Fatalf("ipv6 target = %q err=%v", got, err)
	}
}
