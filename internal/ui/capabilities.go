package ui

import "strings"

func detectNerdFont(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("BAST_NERD_FONT"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}

	if strings.TrimSpace(getenv("WEZTERM_PANE")) != "" || strings.TrimSpace(getenv("KITTY_WINDOW_ID")) != "" {
		return true
	}
	termProgram := strings.ToLower(strings.TrimSpace(getenv("TERM_PROGRAM")))
	term := strings.ToLower(strings.TrimSpace(getenv("TERM")))
	return termProgram == "wezterm" || termProgram == "kitty" || strings.Contains(term, "kitty")
}
