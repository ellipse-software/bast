package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestFilterIdleMouseMotion(t *testing.T) {
	m := testApp(t)
	msg := tea.MouseMotionMsg(tea.Mouse{X: 1, Y: 1})
	if got := FilterIdleMouseMotion(m, msg); got != nil {
		t.Fatal("expected idle mouse motion to be filtered")
	}
	m.scrollbarDragging = true
	if got := FilterIdleMouseMotion(m, msg); got == nil {
		t.Fatal("expected drag mouse motion to pass through")
	}
	key := tea.KeyPressMsg(tea.Key{Text: "j"})
	if got := FilterIdleMouseMotion(m, key); got != key {
		t.Fatal("expected non-mouse messages to pass through")
	}
}

func TestEnterSyncSectionSkipsFreshStatusProbe(t *testing.T) {
	m := testApp(t)
	m.syncStatusAt = time.Now()
	if cmd := m.enterSyncSection(); cmd != nil {
		t.Fatal("expected fresh sync status to skip CLI probe")
	}
	if m.section != syncSection {
		t.Fatalf("section = %v", m.section)
	}
	m.syncStatusAt = time.Now().Add(-time.Minute)
	if cmd := m.enterSyncSection(); cmd == nil {
		t.Fatal("expected stale sync status to probe")
	}
	if !m.syncStatusProbing {
		t.Fatal("expected sync status probe to be marked in-flight")
	}
}

func TestEnterFilesSectionDoesNotListUntilAPaneIsChosen(t *testing.T) {
	m := testApp(t)
	if cmd := m.enterFilesSection(); cmd != nil {
		t.Fatal("opening Files should not list a directory before a pane is chosen")
	}
	if !m.files.panes[0].pickingHost() || !m.files.panes[1].pickingHost() {
		t.Fatal("both panes should open on the host picker")
	}
	m.section = hostsSection
	if cmd := m.enterFilesSection(); cmd != nil {
		t.Fatal("returning to Files should not list a directory")
	}
}
