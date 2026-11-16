package ui

import (
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"bast/internal/metadata"
	"bast/internal/rdp"
	"bast/internal/sshconfig"
)

func TestEntryOpensWithoutSSHAndStaysOutOfFiles(t *testing.T) {
	m := testApp(t)
	saved, err := m.metadata.PutEntry(metadata.Entry{
		Kind: metadata.EntryRDP, Label: "DC01", Address: "10.0.0.5", User: `CORP\alice`,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for i, row := range m.hostListRows() {
		if row.entry != nil && row.entry.ID == saved.ID {
			found = true
			m.cursor = i
		}
	}
	if !found {
		t.Fatal("entry missing from the host list")
	}
	for _, row := range m.hostRows() {
		if row.entry != nil || strings.Contains(row.host.Alias, "DC01") {
			t.Fatalf("files source includes the entry: %+v", row)
		}
	}

	var got rdp.FileSession
	var fileCalls, checkboxCalls int
	prevFile := startEntryFile
	prevRDP := launchRDP
	startEntryFile = func(session rdp.FileSession) (*exec.Cmd, error) {
		fileCalls++
		got = session
		return nil, nil
	}
	launchRDP = func(rdp.Session) error {
		checkboxCalls++
		return nil
	}
	t.Cleanup(func() {
		startEntryFile = prevFile
		launchRDP = prevRDP
	})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("enter did not open the entry")
	}
	msg := cmd()
	opened, ok := msg.(entryOpenedMsg)
	if !ok || opened.err != nil {
		t.Fatalf("open result = %#v", msg)
	}
	if fileCalls != 1 || checkboxCalls != 0 || got.Address != "10.0.0.5" || got.User != "alice" || got.Domain != "CORP" {
		t.Fatalf("session = %+v file=%d checkbox=%d", got, fileCalls, checkboxCalls)
	}

	selectHostAlias(t, m, "alpha")
	m.Update(ctrlO())
	if fileCalls != 1 || checkboxCalls != 0 {
		t.Fatalf("ctrl+o on a host without Remote Desktop should do nothing: file=%d checkbox=%d", fileCalls, checkboxCalls)
	}
}

func TestEntrySortsWithHostsAndRefusesBadJumps(t *testing.T) {
	m := testApp(t)
	if err := m.metadata.SetHost("alpha", metadata.Host{Group: "Work"}); err != nil {
		t.Fatal(err)
	}
	saved, err := m.metadata.PutEntry(metadata.Entry{
		Kind: metadata.EntryRDP, Label: "Zebra", Address: "10.0.0.5", Group: "Work", Favorite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Favorite {
		t.Fatal("favorite was not stored")
	}
	var labels []string
	for _, row := range m.hostListRows() {
		if row.header || row.entry == nil && row.host.Alias == "" {
			continue
		}
		if row.group != "Work" && row.entry == nil && row.host.Alias != "alpha" {
			continue
		}
		if row.entry != nil {
			labels = append(labels, "entry:"+row.entry.Label)
			continue
		}
		if row.host.Alias == "alpha" {
			labels = append(labels, "host:alpha")
		}
	}
	if len(labels) < 2 || labels[0] != "entry:Zebra" || labels[1] != "host:alpha" {
		t.Fatalf("sort = %v", labels)
	}

	m.openEntryForm(metadata.EntryRDP, metadata.Entry{})
	via := m.form.fieldByLabel("Via")
	if via == nil || via.options[0].label != "Direct" || via.options[1].value != "alpha" {
		t.Fatalf("via options = %+v", via)
	}
	if !strings.Contains(m.renderForm(m.styles()), "Basics") || !strings.Contains(m.renderForm(m.styles()), "Sign-in") {
		t.Fatal("entry form is missing its sections")
	}
	m.form = nil

	m.hosts = append(m.hosts, sshconfig.Host{Alias: "sandbox", Synced: true, SyncSource: "vercel", SyncID: "sb"})
	tunneled, err := m.metadata.PutEntry(metadata.Entry{Kind: metadata.EntryWeb, Label: "Panel", URL: "https://192.168.0.1/", Via: "sandbox"})
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := m.openEntry(tunneled)
	if cmd != nil || !m.statusError || !strings.Contains(m.status, "Vercel") {
		t.Fatalf("vercel status=%q err=%v cmd=%v", m.status, m.statusError, cmd != nil)
	}

	missing := tunneled
	missing.Via = "gone"
	_, cmd = m.openEntry(missing)
	if !strings.Contains(m.status, "gone") {
		t.Fatalf("missing jump status=%q", m.status)
	}

	m.hosts = append(m.hosts, sshconfig.Host{Alias: "down", Synced: true, SyncSource: "hetzner", SyncID: "9"})
	if err := m.metadata.SetHost("down", metadata.Host{Tags: []string{"state:off"}}); err != nil {
		t.Fatal(err)
	}
	stopped := tunneled
	stopped.Via = "down"
	_, _ = m.openEntry(stopped)
	if !strings.Contains(m.status, "Start down first") {
		t.Fatalf("stopped status=%q", m.status)
	}

	m.initFilesState()
	for _, row := range m.filesPickerRows(m.filesFocusedPane()) {
		if row.row.entry != nil {
			t.Fatalf("files picker includes an entry: %+v", row.row.entry)
		}
	}
}

func TestEntryStatusReachesTheStatusLine(t *testing.T) {
	statusCh := make(chan string, 1)
	resultCh := make(chan entryOpenedMsg, 1)
	statusCh <- "Publishing SSH key"
	msg := pollEntry(statusCh, resultCh)()
	update, ok := msg.(entryStatusMsg)
	if !ok || update.text != "Publishing SSH key" || update.next == nil {
		t.Fatalf("status = %#v", msg)
	}
	resultCh <- entryOpenedMsg{notice: "Opened DC01"}
	close(statusCh)
	done := update.next()
	if opened, ok := done.(entryOpenedMsg); !ok || opened.notice != "Opened DC01" {
		t.Fatalf("done = %#v", done)
	}
}
