package ui

import "strings"

func detectNerdFont(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("BAST_NERD_FONT"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}

	if windowsTerminalSession(getenv) ||
		strings.TrimSpace(getenv("WEZTERM_PANE")) != "" ||
		strings.TrimSpace(getenv("WEZTERM_EXECUTABLE")) != "" ||
		strings.TrimSpace(getenv("WEZTERM_UNIX_SOCKET")) != "" ||
		strings.TrimSpace(getenv("KITTY_WINDOW_ID")) != "" ||
		strings.TrimSpace(getenv("GHOSTTY_RESOURCES_DIR")) != "" ||
		strings.TrimSpace(getenv("ITERM_SESSION_ID")) != "" {
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

func windowsTerminalSession(getenv func(string) string) bool {
	return strings.TrimSpace(getenv("WT_SESSION")) != ""
}

// Windows Terminal gives every UTF-16 code unit a column. A supplementary-plane
// icon is a surrogate pair, so the terminal reserves two cells and the first
// cell is blank. These stand-ins occupy one cell.
func windowsTerminalIcon(icon string) string {
	switch icon {
	case nerdEnterGlyph: // nf-md-keyboard_return
		return "↵"
	case "\U000f0ec8": // nf-md-sail_boat
		return "\uf21a" // nf-fa-ship
	case "\U000f01a7": // nf-md-cube_outline
		return "\ued75" // nf-fa-box
	default:
		return icon
	}
}

const nerdEnterGlyph = "\U000f0311"

func enterGlyph(windowsTerminal bool) string {
	if windowsTerminal {
		return windowsTerminalIcon(nerdEnterGlyph)
	}
	return nerdEnterGlyph
}
