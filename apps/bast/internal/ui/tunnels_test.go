package ui

import (
	"strings"
	"testing"

	"bast/internal/metadata"
)

func TestTunnelsTabIsSixthAndLists(t *testing.T) {
	m := testApp(t)
	tabs := stripANSI(m.renderTabs(m.styles()))
	if !strings.Contains(tabs, "[6] Tunnels") || strings.Index(tabs, "Files") > strings.Index(tabs, "Tunnels") {
		t.Fatalf("tabs = %q", tabs)
	}
	m.Update(press("6"))
	if m.section != tunnelsSection {
		t.Fatalf("6 opened section %v", m.section)
	}
	if !strings.Contains(stripANSI(m.render()), "No tunnels yet") {
		t.Fatal("empty state missing")
	}

	if err := m.metadata.PutTunnel(metadata.Tunnel{Name: "pg", Host: "alpha", Mode: "local", Local: "5432", Remote: "db:5432", Group: "prod"}, ""); err != nil {
		t.Fatal(err)
	}
	view := stripANSI(m.render())
	for _, want := range []string{"pg", "alpha", "db:5432", "configured"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestTunnelFormSaveValidatesAndStores(t *testing.T) {
	m := testApp(t)
	m.Update(press("6"))
	m.Update(press("a"))
	if m.form == nil || m.form.action != "tunnel_save" {
		t.Fatalf("a did not open the tunnel form: %+v", m.form)
	}
	values := map[string]string{"Name": "pg", "Host": "alpha", "Mode": "local", "Local": "5432", "Remote": ""}
	m.submitTunnelSave(values)
	if m.form == nil || !strings.Contains(m.form.validationError, "remote") {
		t.Fatalf("missing remote not reported: %+v", m.form)
	}
	values["Remote"] = "db:5432"
	m.submitTunnelSave(values)
	if m.form != nil {
		t.Fatalf("form stayed open: %s", m.form.validationError)
	}
	if got, ok := m.metadata.Tunnel("pg"); !ok || got.Remote != "db:5432" {
		t.Fatalf("stored = %+v ok=%v", got, ok)
	}

	m.Update(press("d"))
	if m.form == nil || m.form.action != "tunnel_delete" {
		t.Fatal("d did not open delete")
	}
	m.submitTunnelDelete(map[string]string{"Name": "pg", "Confirmation": "pg", "Type the name to confirm": "pg"})
	if _, ok := m.metadata.Tunnel("pg"); ok {
		t.Fatal("tunnel not deleted")
	}
}

func TestTunnelEditKeepsHostMissingFromThePicker(t *testing.T) {
	m := testApp(t)
	kept := m.tunnelHostField("retired")
	if kept.value != "retired" || kept.options[kept.selected].value != "retired" {
		t.Fatalf("host field = %+v", kept)
	}
	if fresh := m.tunnelHostField(""); fresh.value != "alpha" {
		t.Fatalf("new tunnel host = %q", fresh.value)
	}
}

func TestTunnelActionReportsManagerInitError(t *testing.T) {
	m := testApp(t)
	m.section = tunnelsSection
	m.tunnelMgr = nil
	m.tunnelMgrErr = errTunnelRunner
	if err := m.metadata.PutTunnel(metadata.Tunnel{Name: "pg", Host: "alpha", Mode: "socks", Local: "1080"}, ""); err != nil {
		t.Fatal(err)
	}
	cmd := m.toggleSelectedTunnel()
	if cmd == nil {
		t.Fatal("toggle did nothing")
	}
	msg, ok := cmd().(tunnelResultMsg)
	if !ok || msg.err == nil || msg.err.Error() != errTunnelRunner.Error() {
		t.Fatalf("toggle result = %#v", msg)
	}
	if !strings.Contains(stripANSI(m.render()), errTunnelRunner.Error()) {
		t.Fatal("runner error not shown")
	}
}

var errTunnelRunner = tunnelRunnerError("tunnels directory is a file")

type tunnelRunnerError string

func (e tunnelRunnerError) Error() string { return string(e) }
