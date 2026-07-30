package metadata

import (
	"strings"
	"testing"
)

func TestSplitLabelPath(t *testing.T) {
	tests := []struct {
		raw     string
		group   string
		label   string
		wantErr string
	}{
		{raw: "", group: "", label: ""},
		{raw: "web", group: "", label: "web"},
		{raw: "  web  ", group: "", label: "web"},
		{raw: "abc/test", group: "abc", label: "test"},
		{raw: "Work/Production/web", group: "Work/Production", label: "web"},
		{raw: " one / two / three ", group: "one/two", label: "three"},
		{raw: "one/two/three/four/five/leaf", group: "one/two/three/four/five", label: "leaf"},
		{raw: "abc/", wantErr: "label required after /"},
		{raw: "/web", wantErr: "group levels cannot be empty"},
		{raw: "a//b", wantErr: "group levels cannot be empty"},
		{raw: "one/two/three/four/five/six/leaf", wantErr: "at most 5 levels"},
	}
	for _, test := range tests {
		group, label, err := SplitLabelPath(test.raw)
		if test.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("SplitLabelPath(%q) err = %v, want containing %q", test.raw, err, test.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("SplitLabelPath(%q) unexpected err: %v", test.raw, err)
		}
		if group != test.group || label != test.label {
			t.Fatalf("SplitLabelPath(%q) = (%q, %q), want (%q, %q)", test.raw, group, label, test.group, test.label)
		}
	}
}

func TestJoinAndLeafHelpers(t *testing.T) {
	if got := JoinLabelPath("Work/Production", "web"); got != "Work/Production/web" {
		t.Fatalf("JoinLabelPath = %q", got)
	}
	if got := JoinLabelPath("Work/Production", ""); got != "Work/Production/" {
		t.Fatalf("JoinLabelPath trailing = %q", got)
	}
	if got := JoinLabelPath("", "web"); got != "web" {
		t.Fatalf("JoinLabelPath leaf only = %q", got)
	}
	if got := LabelLeaf("Work/Production/web"); got != "web" {
		t.Fatalf("LabelLeaf = %q", got)
	}
	if got := LabelLeaf("Work/Production/"); got != "" {
		t.Fatalf("LabelLeaf empty leaf = %q", got)
	}
	if got := LabelGroup("Work/Production/web"); got != "Work/Production" {
		t.Fatalf("LabelGroup = %q", got)
	}
	if got := LabelGroup("Work/Production/"); got != "Work/Production" {
		t.Fatalf("LabelGroup trailing = %q", got)
	}
	if got := LabelGroup("web"); got != "" {
		t.Fatalf("LabelGroup plain = %q", got)
	}
}
