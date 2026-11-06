package ui

import (
	"strings"
	"testing"
)

func TestBoxdNewFormDefaultsToTheOrgSize(t *testing.T) {
	m := testApp(t)
	m.openBoxdNewForm()
	for _, item := range m.form.fields {
		if item.label != "Size" {
			continue
		}
		if option := item.options[item.selected]; option.label != "Org default" || option.value != "" {
			t.Fatalf("size defaults to %+v; a snapshot keeps its size and a fresh computer takes the org's", option)
		}
		return
	}
	t.Fatal("no Size field")
}

func TestBoxdNewFormKeepsASnapshotSizeMismatchOpen(t *testing.T) {
	m := testApp(t)
	m.openBoxdNewForm()
	if cmd := m.submitBoxdForm("boxd_new", map[string]string{"Size": "2", "Snapshot": "golden"}); cmd != nil {
		t.Fatal("expected the form to stay open")
	}
	if m.form == nil || !strings.Contains(m.form.validationError, "snapshot") {
		t.Fatalf("form = %#v", m.form)
	}
}
