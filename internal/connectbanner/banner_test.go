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
	if !strings.Contains(ContinuePrompt, "Press any key to continue back to Bast") {
		t.Fatalf("continue prompt missing expected content: %q", ContinuePrompt)
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

func TestWaitToContinueWritesPromptAndReadsKey(t *testing.T) {
	var out bytes.Buffer
	in := strings.NewReader("x")
	WaitToContinue(in, &out)
	if got := out.String(); got != ContinuePrompt {
		t.Fatalf("output = %q, want %q", got, ContinuePrompt)
	}
	if in.Len() != 0 {
		t.Fatal("WaitToContinue did not consume a key")
	}
}

func TestWaitToContinueNilIO(t *testing.T) {
	WaitToContinue(nil, nil)
}

