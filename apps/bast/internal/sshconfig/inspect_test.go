package sshconfig

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectKeepsDuplicatesWildcardsAndScopedIncludes(t *testing.T) {
	m := testManager(t)
	writeTestFile(t, m.MainConfig, strings.Join([]string{
		"Host work",
		"  HostName work.example",
		"  Include " + filepath.ToSlash(filepath.Join(m.Home, ".ssh", "bast", "config")),
		"Host prod",
		"  HostName first.example",
		"Host prod",
		"  HostName second.example",
		"Host *.wild",
		"  User deploy",
		"Match host extra",
		"  User other",
	}, "\n")+"\n", 0600)
	writeTestFile(t, m.ManagedConfig, "Host managed\n  HostName managed.example\n", 0600)

	ins := m.Inspect()
	if len(ins.ParseErrors) != 0 {
		t.Fatalf("parse errors: %+v", ins.ParseErrors)
	}
	var scoped, topLevel int
	for _, inc := range ins.Includes {
		if includeTargets(inc, m.ManagedConfig) {
			if inc.TopLevel {
				topLevel++
			} else {
				scoped++
				if inc.Scope != "work" {
					t.Fatalf("scoped include scope = %q", inc.Scope)
				}
			}
		}
	}
	if scoped != 1 || topLevel != 0 {
		t.Fatalf("includes scoped=%d topLevel=%d: %+v", scoped, topLevel, ins.Includes)
	}

	var prods, wildcards, managed int
	for _, h := range ins.Hosts {
		switch {
		case h.Alias == "prod":
			prods++
		case h.Wildcard && h.Alias == "*.wild":
			wildcards++
		case h.Alias == "managed":
			managed++
		}
	}
	if prods != 2 {
		t.Fatalf("expected 2 prod occurrences, hosts=%+v", aliases(ins))
	}
	if wildcards != 1 || managed != 1 {
		t.Fatalf("wildcard=%d managed=%d aliases=%v", wildcards, managed, aliases(ins))
	}
	if len(ins.Matches) != 1 || ins.Matches[0].Spec != "host extra" {
		t.Fatalf("matches = %+v", ins.Matches)
	}
}

func TestInspectRecordsCyclesWithoutAborting(t *testing.T) {
	m := testManager(t)
	writeTestFile(t, m.MainConfig, "Include loop.conf\nHost after\n  HostName after.example\n", 0600)
	writeTestFile(t, filepath.Join(filepath.Dir(m.MainConfig), "loop.conf"), "Include config\n", 0600)
	ins := m.Inspect()
	foundCycle := false
	for _, err := range ins.ParseErrors {
		if err.Code == "include_cycle" {
			foundCycle = true
		}
	}
	if !foundCycle {
		t.Fatalf("expected cycle error, got %+v", ins.ParseErrors)
	}
	foundAfter := false
	for _, h := range ins.Hosts {
		if h.Alias == "after" {
			foundAfter = true
		}
	}
	if !foundAfter {
		t.Fatalf("cycle should not hide later hosts: %v", aliases(ins))
	}
}

func TestInspectMissingMainConfig(t *testing.T) {
	m := testManager(t)
	ins := m.Inspect()
	if len(ins.Files) != 1 || !ins.Files[0].Missing {
		t.Fatalf("files = %+v", ins.Files)
	}
}

func TestHostPatternMatch(t *testing.T) {
	if !HostPatternMatch("*", "prod") || !HostPatternMatch("prod", "prod") || HostPatternMatch("prod", "other") {
		t.Fatal("literal and star matching failed")
	}
	if !HostPatternMatch("*.example", "api.example") || HostPatternMatch("*.example", "api") {
		t.Fatal("wildcard matching failed")
	}
}

func aliases(ins Inspection) []string {
	out := make([]string, 0, len(ins.Hosts))
	for _, h := range ins.Hosts {
		out = append(out, h.Alias)
	}
	return out
}
