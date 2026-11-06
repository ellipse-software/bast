package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"bast/internal/cloud"
	"bast/internal/metadata"
)

func TestSyncListOrderMatchesDescriptors(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.width = 120
	m.height = 40
	body := stripANSI(m.renderSync(m.styles()))
	last := -1
	for _, d := range cloud.Descriptors() {
		at := strings.Index(body, d.GroupRoot)
		if at < 0 {
			t.Fatalf("missing %s\n%s", d.GroupRoot, body)
		}
		if at < last {
			t.Fatalf("%s out of order", d.GroupRoot)
		}
		last = at
	}
	if strings.Contains(body, "┌") {
		t.Fatalf("sync list should not draw tiles:\n%s", body)
	}
}

func TestSyncSettingsRowsAreUniform(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	gcp := m.syncSettingRows(cloud.GCP)
	if len(gcp) == 0 || gcp[0].label != "Login" {
		t.Fatalf("gcp rows = %+v", gcp)
	}
	for _, label := range []string{"Enabled", "Auto-sync", "Sync now"} {
		if !rowHas(gcp, label) {
			t.Fatalf("gcp missing %s: %+v", label, gcp)
		}
	}
	aws := m.syncSettingRows(cloud.AWS)
	if rowHas(aws, "Login") || rowHas(aws, "New token") {
		t.Fatalf("aws should not offer a login shortcut: %+v", aws)
	}
	if !rowHas(aws, "Enabled") || !rowHas(aws, "Auto-sync") || !rowHas(aws, "Sync now") {
		t.Fatalf("aws rows = %+v", aws)
	}
	upstash := m.syncSettingRows(cloud.Upstash)
	if !rowHas(upstash, "New token") || rowHas(upstash, "Login") {
		t.Fatalf("upstash rows = %+v", upstash)
	}
}

func TestSignedInBoatHidesLogin(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Enabled: true, AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	m.syncStatus.Boat.Authenticated = true
	m.syncStatus.Boat.Login = "hi@ted.ac"
	rows := m.syncSettingRows(cloud.Boat)
	if rowHas(rows, "Login") {
		t.Fatalf("signed-in boat still offers login: %+v", rows)
	}
	snap := m.providerSnapshot(cloud.Boat)
	if word, _ := providerListStatus(snap, false); word != "on" {
		t.Fatalf("status = %q", word)
	}

	m.syncStatus.Boat.Authenticated = false
	m.syncStatus.Boat.Login = ""
	rows = m.syncSettingRows(cloud.Boat)
	if !rowHas(rows, "Login") {
		t.Fatalf("logged-out boat should offer login: %+v", rows)
	}
}

func TestSyncSectionsSplitActions(t *testing.T) {
	m := testApp(t)
	m.width = 80
	m.height = 40
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	m.syncStatus.Boat.Authenticated = true
	m.syncStatus.Boat.Login = "hi@ted.ac"
	detail := stripANSI(m.renderSyncDetail(m.styles(), cloud.Boat, 40, 40))
	settingsAt := strings.Index(detail, "Settings ")
	lifecycleAt := strings.Index(detail, "Lifecycle ")
	enabledAt := strings.Index(detail, "Enabled")
	sandboxAt := strings.Index(detail, "New sandbox")
	if settingsAt < 0 || lifecycleAt < 0 || enabledAt < 0 || sandboxAt < 0 {
		t.Fatalf("sections missing:\n%s", detail)
	}
	if settingsAt > enabledAt || lifecycleAt > sandboxAt || enabledAt > sandboxAt {
		t.Fatalf("section order:\n%s", detail)
	}
	if !strings.Contains(detail, "─") {
		t.Fatalf("section rule missing:\n%s", detail)
	}
	gcp := stripANSI(m.renderSyncDetail(m.styles(), cloud.GCP, 40, 40))
	if !strings.Contains(gcp, "Settings ") || !strings.Contains(gcp, "Lifecycle ") {
		t.Fatalf("gcp should use the same sections:\n%s", gcp)
	}
	visual := m.renderSyncDetail(m.styles(), cloud.Boat, 40, 40)
	lines, rowLine := m.syncDetailLines(cloud.Boat, 40)
	if got := strings.Count(visual, "\n") + 1; got != len(lines) {
		t.Fatalf("detail lines = %d, hit map = %d", got, len(lines))
	}
	rows := m.syncSettingRows(cloud.Boat)
	if len(rowLine) != len(rows) {
		t.Fatalf("clickable rows = %d, settings = %d", len(rowLine), len(rows))
	}
}

func TestNewTokenOpensTheConsole(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.width = 120
	m.height = 40
	var opened string
	prev := openBrowser
	openBrowser = func(raw string) error {
		opened = raw
		return nil
	}
	t.Cleanup(func() { openBrowser = prev })
	for i, d := range cloud.Descriptors() {
		if d.Kind == cloud.Upstash {
			m.syncCursor = i
			break
		}
	}
	m.updateSyncKeys("l")
	if m.syncProvider != "upstash" {
		t.Fatalf("provider = %q", m.syncProvider)
	}
	m.updateSyncKeys("enter")
	if opened != "https://console.upstash.com" {
		t.Fatalf("opened %q", opened)
	}
	if m.form == nil || m.form.action != "provider_token:upstash" {
		t.Fatalf("form = %#v", m.form)
	}
}

func TestDisabledProviderShowsOff(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Disabled: true}); err != nil {
		t.Fatal(err)
	}
	body := m.renderSync(m.styles())
	if strings.Contains(body, "sticky") || strings.Contains(body, "auto-connect") || strings.Contains(body, "no auto-connect") {
		t.Fatalf("opt-out copy leaked:\n%s", body)
	}
	if !strings.Contains(body, "off") {
		t.Fatalf("expected off:\n%s", body)
	}
}

func TestSyncListJKAndEnterSettings(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.width = 100
	m.height = 30
	m.syncCursor = 0
	m.updateSyncKeys("j")
	if m.syncCursor != 1 || m.syncProvider != "" {
		t.Fatalf("j cursor=%d provider=%q", m.syncCursor, m.syncProvider)
	}
	m.updateSyncKeys("k")
	if m.syncCursor != 0 {
		t.Fatalf("k cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("l")
	if m.syncProvider != string(cloud.Descriptors()[0].Kind) {
		t.Fatalf("l provider=%q", m.syncProvider)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 1 {
		t.Fatalf("settings j cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("h")
	if m.syncProvider != "" || m.syncCursor != 0 {
		t.Fatalf("h provider=%q cursor=%d", m.syncProvider, m.syncCursor)
	}
}

func TestSyncMissingSetupShowsOff(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	m.section = syncSection
	m.syncStatus.GCP.GCloudError = "gcloud CLI not found; install Google Cloud SDK and run gcloud auth login"
	m.syncStatus.Boat.BoatCLIError = "not logged in; run boat login"
	m.syncStatus.Upstash.Error = "no API token; connect on the Sync tab"
	body := m.renderSync(m.styles())
	plain := stripANSI(body)
	for _, phrase := range []string{"not found", "not logged in", "no API token", "error"} {
		if strings.Contains(plain, phrase) {
			t.Fatalf("missing setup rendered %q:\n%s", phrase, plain)
		}
	}
	if strings.Contains(body, "38;2;239;68;68") {
		t.Fatalf("missing setup should not be red:\n%s", body)
	}
	if !strings.Contains(plain, "off") {
		t.Fatalf("missing setup should show off:\n%s", plain)
	}
}

func TestSyncDetailShowsTheFullError(t *testing.T) {
	m := testApp(t)
	m.width = 80
	m.height = 40
	m.section = syncSection
	const msg = "could not find Azure CLI; install Azure CLI 2.62 or later and run az login"
	m.syncStatus.Azure.AzureCLIError = msg
	for i, d := range cloud.Descriptors() {
		if d.Kind == cloud.Azure {
			m.syncCursor = i
			break
		}
	}
	snap := m.providerSnapshot(cloud.Azure)
	if snap.Error != msg {
		t.Fatalf("error = %q", snap.Error)
	}
	word, count := providerListStatus(snap, false)
	if word != "off" || count != "" {
		t.Fatalf("missing CLI status = %q %q", word, count)
	}
	raw := m.renderSyncDetail(m.styles(), cloud.Azure, 40, 40)
	if strings.Contains(stripANSI(raw), "Azure CLI") || strings.Contains(raw, "38;2;239;68;68") {
		t.Fatalf("off provider should not show an error:\n%s", stripANSI(raw))
	}

	if err := m.metadata.SetAzure(metadata.AzureIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	const failure = "the subscription lookup failed because the token was rejected by the management endpoint"
	m.syncStatus.Azure.AzureCLIError = failure
	snap = m.providerSnapshot(cloud.Azure)
	word, _ = providerListStatus(snap, false)
	if word != "error" {
		t.Fatalf("failure status = %q", word)
	}
	detail := strings.Join(strings.Fields(stripANSI(m.renderSyncDetail(m.styles(), cloud.Azure, 36, 40))), " ")
	if !strings.Contains(detail, failure) {
		t.Fatalf("full failure missing:\n%s", detail)
	}
	visual := m.renderSyncDetail(m.styles(), cloud.Azure, 36, 40)
	lines, _ := m.syncDetailLines(cloud.Azure, 36)
	if got := strings.Count(visual, "\n") + 1; got != len(lines) {
		t.Fatalf("detail lines = %d, hit map = %d", got, len(lines))
	}
}

func TestSyncTabSwapsPaneFocus(t *testing.T) {
	m := testApp(t)
	m.section = syncSection
	m.syncCursor = 1
	if b, ok := m.matchBinding("tab"); !ok || b.ID != ActionSyncTab {
		t.Fatalf("tab binding = %+v ok=%v", b, ok)
	}
	m.updateSyncKeys("tab")
	if m.syncProvider != string(cloud.Descriptors()[1].Kind) || m.syncCursor != 0 {
		t.Fatalf("tab into settings provider=%q cursor=%d", m.syncProvider, m.syncCursor)
	}
	m.updateSyncKeys("j")
	if m.syncCursor != 1 {
		t.Fatalf("settings cursor=%d", m.syncCursor)
	}
	m.updateSyncKeys("tab")
	if m.syncProvider != "" || m.syncCursor != 1 {
		t.Fatalf("tab back to list provider=%q cursor=%d", m.syncProvider, m.syncCursor)
	}
}

func TestSyncListCountStaysOn(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	m.section = syncSection
	if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, LastInstanceCount: 4}); err != nil {
		t.Fatal(err)
	}
	d, ok := cloud.DescriptorForKind(cloud.GCP)
	if !ok {
		t.Fatal("missing gcp descriptor")
	}
	row := m.renderSyncListRow(m.styles(), d, "on", "(4)", 40, false)
	plain := stripANSI(row)
	if !strings.Contains(plain, "on (4)") {
		t.Fatalf("row = %q", plain)
	}
	green := strings.Index(row, "38;2;16;185;129")
	countAt := strings.Index(row, "(4)")
	if green < 0 || countAt < green || !strings.Contains(row[green:countAt], "m") {
		t.Fatalf("on should be green and the count separate:\n%s", row)
	}

	body := stripANSI(m.renderSync(m.styles()))
	if !strings.Contains(body, "on (4)") {
		t.Fatalf("list missing count:\n%s", body)
	}

	if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, LastInstanceCount: 4, LastSyncError: "access denied"}); err != nil {
		t.Fatal(err)
	}
	snap := m.providerSnapshot(cloud.GCP)
	word, count := providerListStatus(snap, false)
	if word != "error" || count != "" {
		t.Fatalf("failure status = %q %q", word, count)
	}

	if err := m.metadata.SetGCP(metadata.GCPIntegration{LastInstanceCount: 2}); err != nil {
		t.Fatal(err)
	}
	snap = m.providerSnapshot(cloud.GCP)
	word, count = providerListStatus(snap, false)
	if word != "off" || count != "" {
		t.Fatalf("disabled status = %q %q", word, count)
	}
}

func TestSyncSettingSelectionFillsTheRow(t *testing.T) {
	s := testApp(t).styles()
	const width = 40
	login := renderSyncSettingLine(s, syncSettingRow{label: "Login"}, width, true)
	if got := lipgloss.Width(login); got != width {
		t.Fatalf("login highlight width = %d", got)
	}
	enabled := renderSyncSettingLine(s, syncSettingRow{label: "Enabled", value: "on"}, width, true)
	if got := lipgloss.Width(enabled); got != width {
		t.Fatalf("enabled highlight width = %d", got)
	}
	if !strings.Contains(login, "48;2;31;41;55") {
		t.Fatalf("selection background missing:\n%s", login)
	}
}

func TestSyncListUsesBrandColor(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	m.section = syncSection
	body := m.renderSync(m.styles())
	for _, color := range []string{"38;2;66;133;244", "38;2;255;153;0", "38;2;0;233;163"} {
		if !strings.Contains(body, color) {
			t.Fatalf("missing brand color %s\n%s", color, body)
		}
	}
}

func TestSyncPageHidesLongIdentifiers(t *testing.T) {
	m := testApp(t)
	m.width = 120
	m.height = 40
	m.section = syncSection
	const team = "team_0123456789abcdef"
	const project = "prj_0123456789abcdef"
	if err := m.metadata.SetVercel(metadata.VercelIntegration{
		Enabled: true, TeamID: team, ProjectID: project,
	}); err != nil {
		t.Fatal(err)
	}
	m.syncStatus.Vercel.HasToken = true
	for i, d := range cloud.Descriptors() {
		if d.Kind == cloud.Vercel {
			m.syncCursor = i
			break
		}
	}
	body := m.renderSync(m.styles())
	if strings.Contains(body, team) || strings.Contains(body, project) {
		t.Fatalf("long ids should stay off the sync page:\n%s", body)
	}
	rows := m.syncSettingRows(cloud.Vercel)
	if !rowHasValue(rows, "Team", "on") || !rowHasValue(rows, "Projects", "on") || !rowHasValue(rows, "Enabled", "on") {
		t.Fatalf("vercel rows = %+v", rows)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func rowHas(rows []syncSettingRow, label string) bool {
	for _, row := range rows {
		if row.label == label {
			return true
		}
	}
	return false
}

func rowHasValue(rows []syncSettingRow, label, value string) bool {
	for _, row := range rows {
		if row.label == label && row.value == value {
			return true
		}
	}
	return false
}
