package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	keymodel "bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

func TestNewEnablesOnboardingOnFreshStore(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	app, err := New(p, openssh.Default(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !app.onboardingPending || app.onboarding {
		t.Fatal("first TUI should wait for host discovery before showing the chooser")
	}
}

func TestOnboardingShowsOnlyWhenNoHosts(t *testing.T) {
	m := testApp(t)
	m.onboardingPending = true
	m.hosts = nil
	m.decideOnboarding()
	if !m.onboarding || m.onboardingPending {
		t.Fatal("empty map should show the start chooser")
	}

	m = testApp(t)
	m.onboardingPending = true
	m.decideOnboarding()
	if m.onboarding || m.onboardingPending || m.metadata.ShouldOnboard() {
		t.Fatal("existing hosts should skip the chooser and persist dismissal")
	}
}

func TestNewSkipsOnboardingWhenStateExists(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(p.StateFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.StateFile, []byte(`{"version":7,"hosts":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := New(p, openssh.Default(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if app.onboarding || app.onboardingPending {
		t.Fatal("existing Bast state should not show onboarding")
	}
}

func TestOnboardingEnvSkipsCensus(t *testing.T) {
	t.Setenv("BAST_NO_ONBOARDING", "1")
	p := paths.ForHome(t.TempDir())
	app, err := New(p, openssh.Default(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if app.onboarding || app.onboardingPending {
		t.Fatal("BAST_NO_ONBOARDING should skip the chooser")
	}
}

func TestOnboardingChooserIncludesWelcomeFactsAndActions(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.loading = false
	m.hosts = nil
	m.keys = testOnboardingKeys()
	view := m.renderOnboarding(m.styles())
	for _, want := range []string{
		"Welcome to Bast",
		onboardingCopy,
		"2 keys on this machine",
		"add a host",
		"vault · other computers",
		"sync · cloud VMs",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("empty chooser missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "skip") || strings.Contains(view, "continue") {
		t.Fatalf("chooser should not list skip/continue:\n%s", view)
	}

	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	view = m.renderOnboarding(m.styles())
	if !strings.Contains(view, "1 from zsh history") || !strings.Contains(view, "history") {
		t.Fatalf("history destination:\n%s", view)
	}

	m.hosts = []sshconfig.Host{{Alias: "alpha"}, {Alias: "beta"}}
	m.keys = m.keys[:1]
	view = m.renderOnboarding(m.styles())
	if !strings.Contains(view, "2 hosts") || !strings.Contains(view, "1 key") || !strings.Contains(view, "hosts") {
		t.Fatalf("populated chooser:\n%s", view)
	}
	if strings.Contains(view, "add a host") {
		t.Fatalf("populated chooser should not list add:\n%s", view)
	}
	for _, banned := range []string{"Press", "Click", "Step", "skip", "continue"} {
		if strings.Contains(view, banned) {
			t.Fatalf("chooser contains %q:\n%s", banned, view)
		}
	}
}

func testOnboardingKeys() []keymodel.Key {
	return []keymodel.Key{
		{Name: "id_ed25519", PublicPath: "/tmp/id_ed25519.pub"},
		{Name: "work", PublicPath: "/tmp/work.pub"},
	}
}

func TestOnboardingSealFitsNarrowTerminals(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.loading = false
	m.hosts = []sshconfig.Host{{Alias: "alpha"}}
	m.keys = testOnboardingKeys()
	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	for _, width := range []int{40, 60, 100} {
		m.width = width
		view := m.renderOnboarding(m.styles())
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("line wider than %d (%d): %q", width, lipgloss.Width(line), line)
			}
		}
	}
}

func TestOnboardingEscSkipsWithoutSelectingHistory(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.loading = false
	m.hosts = nil
	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.onboarding {
		t.Fatal("esc should dismiss onboarding")
	}
	if m.metadata.ShouldOnboard() {
		t.Fatal("esc should persist dismissal")
	}
	if _, ok := m.selectedHistorySuggestion(); ok {
		t.Fatal("esc should not select a history suggestion")
	}
}

func TestOnboardingEnterLandsOnHistoryWhenNoHosts(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.loading = false
	m.hosts = nil
	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.onboarding {
		t.Fatal("enter should dismiss onboarding")
	}
	if _, ok := m.selectedHistorySuggestion(); !ok {
		t.Fatalf("enter should land on the first suggestion, cursor=%d rows=%+v", m.cursor, m.hostListRows())
	}
}

func TestOnboardingEnterLandsOnExistingHosts(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.loading = false
	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.onboarding || m.section != hostsSection {
		t.Fatalf("enter should continue to hosts: onboarding=%v section=%v", m.onboarding, m.section)
	}
	if _, ok := m.selectedHost(); !ok {
		t.Fatal("enter should keep the first configured host selected")
	}
}

func TestOnboardingQuitDoesNotDismiss(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	_, cmd := m.Update(press("q"))
	requireQuit(t, cmd)
	if !m.onboarding {
		t.Fatal("q should leave onboarding eligible")
	}
	if m.metadata.Onboarding().DismissedAt != nil {
		t.Fatal("q should not persist dismissal")
	}
}

func TestOnboardingJumpsAndAdd(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.Update(press("3"))
	if m.onboarding || m.section != vaultSection {
		t.Fatalf("3 should open vault: onboarding=%v section=%v", m.onboarding, m.section)
	}

	m = testApp(t)
	m.onboarding = true
	m.Update(press("4"))
	if m.onboarding || m.section != syncSection {
		t.Fatalf("4 should open sync: onboarding=%v section=%v", m.onboarding, m.section)
	}

	m = testApp(t)
	m.onboarding = true
	m.Update(press("a"))
	if m.onboarding || m.form == nil || m.form.action != "host_add" {
		t.Fatalf("a should open add host, form=%#v onboarding=%v", m.form, m.onboarding)
	}

	m = testApp(t)
	m.onboarding = true
	m.Update(press("?"))
	if m.onboarding || !m.help {
		t.Fatalf("? should open help: onboarding=%v help=%v", m.onboarding, m.help)
	}
}

func TestOnboardingHeaderTabClickJumps(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	x := lipgloss.Width(headerTitle) + 2
	for i, label := range headerTabLabels {
		if headerTabSections[i] == vaultSection {
			m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: 0, Button: tea.MouseLeft}))
			if m.onboarding || m.section != vaultSection {
				t.Fatalf("click %q should open vault: onboarding=%v section=%v", label, m.onboarding, m.section)
			}
			return
		}
		x += lipgloss.Width(label) + lipgloss.Width(headerTabSpacing)
	}
	t.Fatal("vault tab not found")
}

func TestAboutCanReplayOnboarding(t *testing.T) {
	m := testApp(t)
	m.Update(press("v"))
	if !m.credits {
		t.Fatal("v should open about")
	}
	m.Update(press("o"))
	if !m.onboarding || m.credits || !m.onboardingReplay {
		t.Fatalf("o should replay census: onboarding=%v credits=%v replay=%v", m.onboarding, m.credits, m.onboardingReplay)
	}
	view := m.render()
	if !strings.Contains(view, "Welcome to Bast") || !strings.Contains(view, "vault · other computers") {
		t.Fatalf("replay chooser:\n%s", view)
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.onboarding || m.onboardingReplay {
		t.Fatal("esc should close the replayed census")
	}
	if m.metadata.Onboarding().DismissedAt != nil {
		t.Fatal("replaying from about should not persist first-run dismissal")
	}
	if !m.metadata.ShouldOnboard() {
		t.Fatal("fresh store should stay eligible after a replay")
	}
}

func TestOnboardingFooterOmitsActions(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.loading = false
	m.hosts = nil
	footer := m.renderFooter(m.styles())
	for _, text := range []string{"add", "continue", "vault", "skip", "Press"} {
		if strings.Contains(footer, text) {
			t.Fatalf("onboarding footer should be empty, got %q", footer)
		}
	}
}

func TestOnboardingErrorStillWins(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.status, m.statusError = "failed", true
	view := m.render()
	if !strings.Contains(view, "Action failed") || strings.Contains(view, "Welcome to Bast") {
		t.Fatalf("error should replace the chooser:\n%s", view)
	}
}

func TestEmptyKeysUsesSeal(t *testing.T) {
	m := testApp(t)
	m.section = keysSection
	m.keys = nil
	m.loading = false
	m.enriching = false
	view := m.renderKeys(m.styles())
	if !strings.Contains(view, "No keys yet") || !strings.Contains(view, "OpenSSH · ssh-agent") {
		t.Fatalf("empty keys:\n%s", view)
	}
	if strings.Contains(view, "Press") {
		t.Fatalf("empty keys should not narrate the key:\n%s", view)
	}
}
