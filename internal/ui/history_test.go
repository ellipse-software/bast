package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
)

func historyTestApp(t *testing.T, suggestions ...metadata.HistorySuggestion) *App {
	t.Helper()
	p := paths.ForHome(t.TempDir())
	app, err := New(p, openssh.Default(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	state := metadata.HistoryImport{Pending: append([]metadata.HistorySuggestion(nil), suggestions...)}
	if err := app.metadata.SetHistoryImport(state); err != nil {
		t.Fatal(err)
	}
	app.historySuggestions = state.Pending
	if len(suggestions) > 0 {
		app.cursor = 1
	}
	app.loading = false
	app.width, app.height = 80, 24
	return app
}

func testHistorySuggestion() metadata.HistorySuggestion {
	return metadata.HistorySuggestion{
		ID: "suggestion-1", Alias: "ubuntu-example.com", Target: "example.com",
		HostName: "example.com", User: "ubuntu", Port: "2222", Source: "zsh",
	}
}

func TestHistorySuggestionsFollowHostsAndUseSelectedDetail(t *testing.T) {
	m := testApp(t)
	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	rows := m.hostListRows()
	if len(rows) != 4 || !rows[2].historyHeader || rows[3].suggestion == nil {
		t.Fatalf("host list rows = %+v", rows)
	}
	m.cursor = 3
	view := m.renderHosts(m.styles())
	for _, text := range []string{"ubuntu-example.com", "ubuntu@example.com:2222", "From zsh history", addAction} {
		if !strings.Contains(view, text) {
			t.Fatalf("history detail is missing %q:\n%s", text, view)
		}
	}
	if footer := m.renderFooter(m.styles()); !strings.Contains(footer, "add") || !strings.Contains(footer, "dismiss") {
		t.Fatalf("history footer = %q", footer)
	}
}

func TestHistorySuggestionsParticipateInSearch(t *testing.T) {
	m := testApp(t)
	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	m.search = "ubuntu"
	rows := m.hostListRows()
	if len(rows) != 2 || !rows[0].historyHeader || rows[1].suggestion == nil {
		t.Fatalf("filtered rows = %+v", rows)
	}
	m.search = "missing"
	if rows := m.hostListRows(); len(rows) != 0 {
		t.Fatalf("non-matching rows = %+v", rows)
	}
}

func TestSuggestedGroupCanCollapseAndExpand(t *testing.T) {
	m := historyTestApp(t, testHistorySuggestion())
	m.cursor = 0
	if !m.historySuggestionsHeaderSelected() {
		t.Fatal("suggested group header is not selectable")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !m.historySuggestionsCollapsed || len(m.hostListRows()) != 1 {
		t.Fatalf("suggested group did not collapse: rows=%+v", m.hostListRows())
	}
	m.Update(press("space"))
	if m.historySuggestionsCollapsed || len(m.hostListRows()) != 2 {
		t.Fatalf("suggested group did not expand: rows=%+v", m.hostListRows())
	}
}

func TestDismissHistorySuggestionPersists(t *testing.T) {
	suggestion := testHistorySuggestion()
	m := historyTestApp(t, suggestion)
	_, cmd := m.Update(press("x"))
	if cmd == nil {
		t.Fatal("dismiss did not return a notice command")
	}
	if len(m.historySuggestions) != 0 || len(m.metadata.HistoryImport().Pending) != 0 {
		t.Fatalf("suggestion was not dismissed: local=%+v stored=%+v", m.historySuggestions, m.metadata.HistoryImport().Pending)
	}
}

func TestEnterImportsHistorySuggestion(t *testing.T) {
	suggestion := testHistorySuggestion()
	m := historyTestApp(t, suggestion)
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil || m.historyImporting != suggestion.ID {
		t.Fatalf("import did not start: id=%q cmd=%v", m.historyImporting, cmd)
	}
	if _, duplicate := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})); duplicate != nil {
		t.Fatal("duplicate history import was not blocked")
	}
	message := cmd()
	done, ok := message.(historyImportDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("import result = %#v", message)
	}
	m.Update(done)
	hosts, err := m.config.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Alias != suggestion.Alias || hosts[0].Resolved.HostName != suggestion.HostName {
		t.Fatalf("imported hosts = %+v", hosts)
	}
	if len(m.metadata.HistoryImport().Pending) != 0 || len(m.historySuggestions) != 0 {
		t.Fatal("accepted suggestion remained pending")
	}
}

func TestHistoryImportRollsBackSSHBlockWhenStateWriteFails(t *testing.T) {
	suggestion := testHistorySuggestion()
	m := historyTestApp(t, suggestion)
	stateDir := filepath.Dir(m.paths.StateFile)
	if err := os.Remove(m.paths.StateFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stateDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	result := cmd().(historyImportDoneMsg)
	if result.err == nil {
		t.Fatal("history import unexpectedly succeeded")
	}
	hosts, err := m.config.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 0 {
		t.Fatalf("failed import left an SSH host behind: %+v", hosts)
	}
}

func TestReviewHistorySuggestionPrefillsExistingHostForm(t *testing.T) {
	suggestion := testHistorySuggestion()
	suggestion.IdentityFile = "~/.ssh/work"
	suggestion.ProxyJump = "jump"
	m := historyTestApp(t, suggestion)
	m.Update(press("e"))
	if m.form == nil || m.form.action != "history_host_add" {
		t.Fatalf("review form = %+v", m.form)
	}
	for label, want := range map[string]string{
		"Label": suggestion.Alias, "Hostname": suggestion.HostName, "User": suggestion.User,
		"Port": suggestion.Port, "History suggestion": suggestion.ID,
	} {
		if got := formFieldByLabel(m, label).value; got != want {
			t.Fatalf("%s = %q, want %q", label, got, want)
		}
	}
	if got := formFieldByLabel(m, "Identity file").value; got != suggestion.IdentityFile {
		t.Fatalf("identity = %q", got)
	}
}

func TestHistoryAddButtonIsClickable(t *testing.T) {
	m := historyTestApp(t, testHistorySuggestion())
	layout := m.panelLayout()
	buttonX, buttonY, _ := m.hostActionButtonBounds(layout, addAction)
	_, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: buttonX, Y: buttonY, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("history Add button did not trigger import")
	}
}

func TestHistorySuggestionRendersInMobileLayout(t *testing.T) {
	m := historyTestApp(t, testHistorySuggestion())
	m.width, m.height = 40, 20
	view := m.renderHosts(m.styles())
	if !strings.Contains(view, "ubuntu-example.com") || !strings.Contains(view, addAction) {
		t.Fatalf("mobile history suggestion is incomplete:\n%s", view)
	}
}

func TestHistoryScanCommandPersistsNewSuggestions(t *testing.T) {
	m := historyTestApp(t)
	historyPath := filepath.Join(m.paths.Home, ".zsh_history")
	if err := writeHistoryFixture(historyPath, "ssh deploy@example.com\n"); err != nil {
		t.Fatal(err)
	}
	message := m.historyScanCmd(nil)()
	loaded, ok := message.(historyLoadedMsg)
	if !ok || loaded.err != nil || len(loaded.suggestions) != 1 {
		t.Fatalf("scan result = %#v", message)
	}
	m.Update(loaded)
	if len(m.historySuggestions) != 1 || len(m.metadata.HistoryImport().Pending) != 1 {
		t.Fatal("scanned suggestion was not loaded and persisted")
	}
	if m.historyScanCmd(nil) != nil {
		t.Fatal("history scan started more than once")
	}
}

func writeHistoryFixture(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
