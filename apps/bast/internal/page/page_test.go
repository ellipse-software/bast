package page

import (
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	got, err := ParseURL("HTTPS://192.168.0.1/admin")
	if err != nil || got != "https://192.168.0.1/admin" {
		t.Fatalf("url = %q err=%v", got, err)
	}
	for _, raw := range []string{"javascript:alert(1)", "https://user:pass@192.168.0.1/", "file:///etc/passwd", "https://", ""} {
		if _, err := ParseURL(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestLaunchSpecUsesIsolatedProxy(t *testing.T) {
	name, args, prefs := LaunchSpec(Browser{Name: "chrome", Path: "/bin/chrome"}, 43210, "/tmp/profile", "https://192.168.0.1/")
	if name != "/bin/chrome" || prefs != "" {
		t.Fatalf("name=%q prefs=%q", name, prefs)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--proxy-server=socks5://127.0.0.1:43210") || !strings.Contains(joined, "--user-data-dir=/tmp/profile") || !strings.Contains(joined, "https://192.168.0.1/") {
		t.Fatalf("args = %q", joined)
	}
	_, _, prefs = LaunchSpec(Browser{Name: "firefox", Path: "/bin/firefox"}, 43210, "/tmp/profile", "https://192.168.0.1/")
	if !strings.Contains(prefs, `network.proxy.socks_port", 43210`) || !strings.Contains(prefs, "socks_remote_dns") {
		t.Fatalf("prefs = %q", prefs)
	}
}
