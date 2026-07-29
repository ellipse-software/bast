package ui

import "strings"

func detectNerdFont(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("BAST_NERD_FONT"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}

	if strings.TrimSpace(getenv("WEZTERM_PANE")) != "" ||
		strings.TrimSpace(getenv("KITTY_WINDOW_ID")) != "" ||
		strings.TrimSpace(getenv("WT_SESSION")) != "" {
		return true
	}
	termProgram := strings.ToLower(strings.TrimSpace(getenv("TERM_PROGRAM")))
	lcTerminal := strings.ToLower(strings.TrimSpace(getenv("LC_TERMINAL")))
	term := strings.ToLower(strings.TrimSpace(getenv("TERM")))
	switch termProgram {
	case "wezterm", "kitty", "iterm.app", "alacritty", "ghostty", "warpterminal", "hyper":
		return true
	}
	if lcTerminal == "iterm2" || strings.Contains(term, "kitty") {
		return true
	}
	return false
}
