//go:build !windows

package sshutil

import "testing"

func TestPOSIXProxyCommandQuotesBackslashes(t *testing.T) {
	if got := ShellQuote(`service\account.json`); got != `'service\account.json'` {
		t.Fatalf("quoted path = %q", got)
	}
}
