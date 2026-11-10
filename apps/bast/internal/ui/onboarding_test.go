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
		"██████╗",
		"Welcome to Bast",
		"Termius",
		"Thank you for supporting my",
		"project.",
		onboardingCopy,
		"any key to continue",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("empty chooser missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Press") || strings.Contains(view, "Add a host") {
		t.Fatalf("chooser should not list individual keys:\n%s", view)
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

func TestOnboardingAnyKeyLandsOnHistoryWhenNoHosts(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.loading = false
	m.hosts = nil
	m.historySuggestions = []metadata.HistorySuggestion{testHistorySuggestion()}
	m.Update(press("x"))
	if m.onboarding || m.section != hostsSection {
		t.Fatalf("any key should open hosts: onboarding=%v section=%v", m.onboarding, m.section)
	}
	if m.metadata.ShouldOnboard() {
		t.Fatal("continuing should persist dismissal")
	}
	if _, ok := m.selectedHistorySuggestion(); !ok {
		t.Fatalf("any key should land on the first suggestion, cursor=%d rows=%+v", m.cursor, m.hostListRows())
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

func TestOnboardingQuestionMarkContinuesToHosts(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	m.Update(press("?"))
	if m.onboarding || m.palette || m.section != hostsSection {
		t.Fatalf("? should continue to hosts: onboarding=%v palette=%v section=%v", m.onboarding, m.palette, m.section)
	}
}

func TestOnboardingHeaderTabClickJumps(t *testing.T) {
	m := testApp(t)
	m.onboarding = true
	x, ok := 0, false
	dummy := &App{}
	cursor := lipgloss.Width(headerTitle) + 2
	gap := lipgloss.Width(headerTabSpacing)
	for _, sec := range headerTabSections {
		width := lipgloss.Width(dummy.tabChip(sec))
		if sec == vaultSection {
			x, ok = cursor, true
			break
		}
		cursor += width + gap
	}
	if !ok {
		t.Fatal("vault tab not found")
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: 0, Button: tea.MouseLeft}))
	if m.onboarding || m.section != vaultSection {
		t.Fatalf("click vault should open vault: onboarding=%v section=%v", m.onboarding, m.section)
	}
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
	if !strings.Contains(view, "Welcome to Bast") || !strings.Contains(view, "any key to continue") {
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
