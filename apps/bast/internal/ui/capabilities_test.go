package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestDetectNerdFont(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "wezterm pane", env: map[string]string{"WEZTERM_PANE": "1"}, want: true},
		{name: "wezterm program", env: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: true},
		{name: "kitty window", env: map[string]string{"KITTY_WINDOW_ID": "1"}, want: true},
		{name: "kitty term", env: map[string]string{"TERM": "xterm-kitty"}, want: true},
		{name: "iterm", env: map[string]string{"TERM_PROGRAM": "iTerm.app"}, want: true},
		{name: "iterm lc", env: map[string]string{"LC_TERMINAL": "iTerm2"}, want: true},
		{name: "alacritty", env: map[string]string{"TERM_PROGRAM": "Alacritty"}, want: true},
		{name: "ghostty", env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: true},
		{name: "windows terminal", env: map[string]string{"WT_SESSION": "1"}, want: true},
		{name: "unknown terminal", env: map[string]string{"TERM": "xterm-256color"}},
		{name: "forced on", env: map[string]string{"BAST_NERD_FONT": "1"}, want: true},
		{name: "forced off", env: map[string]string{"BAST_NERD_FONT": "0", "WEZTERM_PANE": "1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := detectNerdFont(func(key string) string { return test.env[key] })
			if got != test.want {
				t.Fatalf("detectNerdFont() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestCloudGroupNamesUseNerdFontIconsWhenEnabled(t *testing.T) {
	tests := []struct {
		name  string
		group string
		icon  string
	}{
		{name: "GCP", group: "Google Cloud", icon: "\ue7f1"},
		{name: "AWS", group: "Amazon EC2", icon: "\ue7ad"},
		{name: "Azure", group: "Microsoft Azure", icon: "\ue754"},
		{name: "Box", group: "Box", icon: "\uf1b2"},
		{name: "Upstash", group: "Upstash", icon: "\uf1b2"},
		{name: "Vercel", group: "Vercel", icon: "\u25b2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withIcon := renderManagedGroupName(test.group, lipgloss.NewStyle(), true)
			withoutIcon := renderManagedGroupName(test.group, lipgloss.NewStyle(), false)
			if !strings.Contains(withIcon, test.icon) {
				t.Fatalf("Nerd Font icon missing from %q", withIcon)
			}
			if strings.Contains(withoutIcon, test.icon) {
				t.Fatalf("Nerd Font icon rendered while disabled: %q", withoutIcon)
			}
		})
	}
}
