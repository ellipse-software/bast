package ui

import (
	"strings"
	"testing"

	keymodel "bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func sheet(m *App) string {
	return stripANSI(strings.Join(m.paletteContent(m.styles(), 160), "\n"))
}

func TestContextualSheetBySection(t *testing.T) {
	m := testApp(t)
	m.width = 100

	m.section = hostsSection
	m.hosts = nil
	got := sheet(m)
	if !strings.Contains(got, "add") || !strings.Contains(got, "desktop or web") {
		t.Fatalf("empty hosts = %q", got)
	}
	if strings.Contains(got, "connect") {
		t.Fatalf("empty hosts should not offer connect: %q", got)
	}

	m.hosts = []sshconfig.Host{{Alias: "alpha", Managed: true}}
	m.cursor = 0
	got = sheet(m)
	if !strings.Contains(got, "connect") || !strings.Contains(got, "files") {
		t.Fatalf("host = %q", got)
	}

	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	m.sortHosts()
	m.cursor = 0
	got = sheet(m)
	if !strings.Contains(got, "␣") || !strings.Contains(got, "edit") {
		t.Fatalf("group = %q", got)
	}

	m.section = keysSection
	m.keys = nil
	got = sheet(m)
	if !strings.Contains(got, "generate") || !strings.Contains(got, "import") {
		t.Fatalf("empty keys = %q", got)
	}
	if strings.Contains(got, "add") && strings.Contains(got, "n") == false {
		t.Fatalf("empty keys = %q", got)
	}
	m.keys = []keymodel.Key{{Name: "work", PublicPath: "/tmp/work.pub", PrivatePath: "/tmp/work"}}
	m.cursor = 0
	got = sheet(m)
	if !strings.Contains(got, "add") {
		t.Fatalf("selected key = %q", got)
	}

	m.section = vaultSection
	m.syncCursor = -1
	got = sheet(m)
	if !strings.Contains(got, "link") {
		t.Fatalf("unlinked vault = %q", got)
	}

	m.section = syncSection
	m.syncProvider = ""
	got = sheet(m)
	if !strings.Contains(got, "open") {
		t.Fatalf("sync root = %q", got)
	}
	m.syncProvider = "gcp"
	got = sheet(m)
	if !strings.Contains(got, "back") {
		t.Fatalf("sync provider = %q", got)
	}

	m.section = filesSection
	m.initFilesState()
	got = sheet(m)
	if !strings.Contains(got, "connect") || !strings.Contains(got, "search") {
		t.Fatalf("files picker = %q", got)
	}
	m.files.panes[m.files.focus].kind = filesPaneLocal
	got = sheet(m)
	if !strings.Contains(got, "pane") || !strings.Contains(got, "hosts") {
		t.Fatalf("files browse = %q", got)
	}
}
