package ui

import "testing"

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
