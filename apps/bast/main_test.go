package main

import "testing"

func TestBuildVersionPrefersLinkerValue(t *testing.T) {
	original := version
	version = "v1.2.3"
	t.Cleanup(func() { version = original })
	if got := buildVersion(); got != "v1.2.3" {
		t.Fatalf("buildVersion() = %q", got)
	}
}

func TestBuildVersionFallsBackWhenLinkerValueIsEmpty(t *testing.T) {
	original := version
	version = ""
	t.Cleanup(func() { version = original })
	if got := buildVersion(); got == "" {
		t.Fatal("buildVersion() returned an empty version")
	}
}
