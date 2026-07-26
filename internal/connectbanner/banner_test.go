package connectbanner

import (
	"bytes"
	"strings"
	"testing"
)

func TestBannerAndStatusFormatting(t *testing.T) {
	if !strings.Contains(Banner, " BAST ") || !strings.Contains(Banner, "Connecting to server") {
		t.Fatalf("banner missing expected content: %q", Banner)
	}
	var buf bytes.Buffer
	Write(&buf)
	Status(&buf)("Checking GCP instance access…")
	got := buf.String()
	if !strings.HasPrefix(got, Banner) {
		t.Fatalf("missing banner:\n%s", got)
	}
	if !strings.Contains(got, "\x1b[38;2;107;114;128m Checking GCP instance access…\x1b[0m\r\n") {
		t.Fatalf("status should be muted with a leading space:\n%q", got)
	}
}
