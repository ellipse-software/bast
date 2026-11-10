package ui

import (
	"strings"
	"testing"

	"bast/internal/sshconfig"
)

func TestForwardLinesDescribeEachKind(t *testing.T) {
	lines := forwardLines(sshconfig.AdvancedSettings{
		LocalForwards:  []string{"3005 localhost:3005"},
		RemoteForwards: []string{"9000 localhost:3000"},
		DynamicForward: "1080",
	})
	got := strings.Join(lines, " | ")
	want := "3005 → localhost:3005 | remote 9000 → localhost:3000 | socks 1080"
	if got != want {
		t.Fatalf("lines = %q", got)
	}
}
