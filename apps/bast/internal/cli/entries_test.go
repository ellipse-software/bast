package cli

import (
	"strings"
	"testing"
)

func TestRDPEntryAddListAndRejectsUserinfo(t *testing.T) {
	home := t.TempDir()
	client := fakeOpenSSH(t)
	out, errOut, err := runTestCLI(t, home, client, "--json", "rdp", "add", "DC01", "--address", "10.0.0.5", "--user", `CORP\alice`, "--group", "Work")
	if err != nil || errOut != "" || !strings.Contains(out, `"label":"DC01"`) || !strings.Contains(out, `"domain":"CORP"`) {
		t.Fatalf("add output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, errOut, err = runTestCLI(t, home, client, "--json", "web", "add", "Firewall", "--url", "https://root:secret@192.168.0.1/")
	if err == nil || !strings.Contains(errOut, "username or password") {
		t.Fatalf("userinfo output=%q stderr=%q err=%v", out, errOut, err)
	}
	out, errOut, err = runTestCLI(t, home, client, "--json", "rdp", "list")
	if err != nil || errOut != "" || !strings.Contains(out, "DC01") {
		t.Fatalf("list output=%q stderr=%q err=%v", out, errOut, err)
	}
}
