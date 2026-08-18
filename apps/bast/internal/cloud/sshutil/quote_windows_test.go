//go:build windows

package sshutil

import "testing"

func TestWindowsProxyCommandQuoting(t *testing.T) {
	if got := ShellQuote(`C:\Program Files\Bast\bast.exe`); got != `"C:\Program Files\Bast\bast.exe"` {
		t.Fatalf("quoted executable = %q", got)
	}
	if got := WithEnvironment("gcloud compute", "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE", `C:\Users\Ted Brine\key.json`); got != `set "CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE=C:\Users\Ted Brine\key.json"&& gcloud compute` {
		t.Fatalf("environment command = %q", got)
	}
}
