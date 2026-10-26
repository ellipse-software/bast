package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	keymodel "bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func TestFitFooterHintDropsTrailingActions(t *testing.T) {
	parts := []string{"enter", "e edit", "F files", "?"}
	wide := fitFooterHint(parts, 80)
	if wide != "enter · e edit · F files · ?" {
		t.Fatalf("wide = %q", wide)
	}
	narrow := fitFooterHint(parts, 12)
	if lipgloss.Width(narrow) > 12 {
		t.Fatalf("narrow too wide: %q (%d)", narrow, lipgloss.Width(narrow))
	}
	if !strings.Contains(narrow, "enter") && narrow != "?" {
		t.Fatalf("narrow should keep primary action: %q", narrow)
	}
}

func TestContextualFootersBySection(t *testing.T) {
	m := testApp(t)
	m.width = 100

	m.section = hostsSection
	m.hosts = nil
	if got := m.browseFooterHint(80); got != "a add · ?" {
		t.Fatalf("empty hosts = %q", got)
	}

	m.hosts = []sshconfig.Host{{Alias: "alpha", Managed: true}}
	m.cursor = 0
	if got := m.browseFooterHint(80); !strings.Contains(got, "enter") || !strings.Contains(got, "F files") {
		t.Fatalf("host = %q", got)
	}

	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = 0
	if got := m.browseFooterHint(80); !strings.Contains(got, "␣") || !strings.Contains(got, "e rename") {
		t.Fatalf("group = %q", got)
	}

	m.section = keysSection
	m.keys = nil
	if got := m.browseFooterHint(80); got != "g generate · i import · ?" {
		t.Fatalf("empty keys = %q", got)
	}
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub", PrivatePath: "/tmp/work"}}
	m.cursor = 0
	if got := m.browseFooterHint(80); !strings.Contains(got, "a add") || strings.Contains(got, "u add") {
		t.Fatalf("selected key = %q", got)
	}

	m.section = vaultSection
	m.syncCursor = -1
	if got := m.browseFooterHint(80); !strings.Contains(got, "enter link") {
		t.Fatalf("unlinked vault = %q", got)
	}

	m.section = syncSection
	m.syncProvider = ""
	if got := m.browseFooterHint(80); !strings.Contains(got, "enter open") {
		t.Fatalf("sync root = %q", got)
	}
	m.syncProvider = "gcp"
	if got := m.browseFooterHint(80); !strings.Contains(got, "esc back") {
		t.Fatalf("sync provider = %q", got)
	}

	m.section = filesSection
	m.initFilesState()
	if got := m.browseFooterHint(80); !strings.Contains(got, "tab") || !strings.Contains(got, "esc back") {
		t.Fatalf("files = %q", got)
	}
}
