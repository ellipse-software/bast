package ui

import (
	"strings"
	"testing"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func TestDisabledFeaturesHideTabsAndRenumber(t *testing.T) {
	m := testApp(t)
	m.noVault = true
	m.noSync = true
	tabs := stripANSI(m.renderTabs(m.styles()))
	if strings.Contains(tabs, "Vault") || strings.Contains(tabs, "Sync") {
		t.Fatalf("hidden tabs still rendered: %q", tabs)
	}
	for _, want := range []string{"[1] Hosts", "[2] Keys", "[3] Files"} {
		if !strings.Contains(tabs, want) {
			t.Fatalf("tabs = %q, missing %q", tabs, want)
		}
	}
	m.Update(press("3"))
	if m.section != filesSection {
		t.Fatalf("3 should open Files, got %v", m.section)
	}
	m.Update(press("4"))
	if m.section != filesSection {
		t.Fatalf("4 should do nothing when Files is 3, got %v", m.section)
	}
	m.Update(press("5"))
	if m.section != filesSection {
		t.Fatalf("5 should not open a hidden Sync, got %v", m.section)
	}

	m = testApp(t)
	m.noVault = true
	m.Update(press("3"))
	if m.section != syncSection {
		t.Fatalf("3 should open Sync when Vault is hidden, got %v", m.section)
	}
	m.Update(press("4"))
	if m.section != filesSection {
		t.Fatalf("4 should open Files when Vault is hidden, got %v", m.section)
	}
	if _, ok := m.tabAtX(0); ok {
		t.Fatal("the title is not a tab")
	}
}

func TestDisabledFeaturesOmitOnboardingRows(t *testing.T) {
	m := testApp(t)
	m.noSync = true
	m.noVault = true
	m.onboarding = true
	view := m.render()
	if strings.Contains(view, "vault ·") || strings.Contains(view, "sync ·") {
		t.Fatalf("onboarding still offers a hidden feature:\n%s", view)
	}
	m.Update(press("3"))
	if m.section != filesSection || m.onboarding {
		t.Fatalf("onboarding 3 should open Files: onboarding=%v section=%v", m.onboarding, m.section)
	}
}

func TestSyncOffSkipsAutoSyncAndProviderActions(t *testing.T) {
	m := testApp(t)
	m.noSync = true
	if err := m.metadata.SetGCP(metadata.GCPIntegration{Enabled: true, AutoSync: true}); err != nil {
		t.Fatal(err)
	}
	if cmd := m.autoSyncCmds(); cmd != nil || m.syncingProviders["gcp"] {
		t.Fatalf("auto-sync ran with BAST_NO_SYNC: cmd=%v providers=%v", cmd != nil, m.syncingProviders)
	}

	m.hosts = []sshconfig.Host{{
		Alias: "boat_live", Synced: true, SyncSource: "boat",
		Resolved: sshconfig.Resolved{HostName: "203.0.113.10", User: "user"},
	}}
	if err := m.metadata.SetBoat(metadata.BoatIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.metadata.SetHost("boat_live", metadata.Host{Label: "live", Group: "Boat", Tags: []string{"state:idle"}}); err != nil {
		t.Fatal(err)
	}
	m.collapsedGroups = map[string]bool{}
	m.cursor = 0
	detail := m.renderGroupDetail(m.styles(), m.hostRows()[0], 60)
	if strings.Contains(detail, "New sandbox") || strings.Contains(detail, "Sync now") {
		t.Fatalf("provider group still offers a sync action:\n%s", detail)
	}
	m.updateKeys(press("n"))
	if m.form != nil {
		t.Fatalf("n opened a form while sync is off: %#v", m.form)
	}
	if !strings.Contains(m.status, "BAST_NO_SYNC") {
		t.Fatalf("status = %q", m.status)
	}

	local := testApp(t)
	local.noSync = true
	local.updateKeys(press("s"))
	if strings.Contains(local.status, "BAST_NO_SYNC") {
		t.Fatalf("sort on a local host was treated as sync: %q", local.status)
	}
}
