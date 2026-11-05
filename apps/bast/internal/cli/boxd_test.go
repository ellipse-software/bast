package cli

import (
	"strings"
	"testing"

	"bast/internal/openssh"
)

func TestBoxdCommandsTakeFlagsAfterTheHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BOXD_CLI", "/nonexistent/boxd")
	for _, args := range [][]string{
		{"boxd", "fork", "boxd_fra_web", "--name", "copy"},
		{"boxd", "delete", "boxd_fra_web", "--yes"},
		{"boxd", "stop", "boxd_fra_web"},
		{"boxd", "restart", "boxd_fra_web"},
	} {
		_, errOut, err := runTestCLI(t, home, openssh.Client{}, args...)
		if err == nil {
			t.Fatalf("%v: expected an unknown-host error", args)
		}
		if msg := err.Error() + errOut; strings.Contains(msg, "usage:") || !strings.Contains(msg, "not found") {
			t.Fatalf("%v: %q", args, msg)
		}
	}
}
