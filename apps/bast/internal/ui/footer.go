package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// browseFooterHint returns contextual key chords for the current browse mode.
// Keep these short: a few primary actions for the selection, then "?".
func (m *App) browseFooterHint(maxWidth int) string {
	if m.section == filesSection {
		if status := m.filesFooterStatus(); status != "" {
			return fitFooterHint(splitFooterHint(status), maxWidth)
		}
	}
	return fitFooterHint(m.catalogFooterParts(), maxWidth)
}

func splitFooterHint(hint string) []string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return []string{"?"}
	}
	parts := strings.Split(hint, " · ")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"?"}
	}
	return out
}

// fitFooterHint joins chords with " · ", dropping trailing actions when the
// terminal is too narrow. The first chord and a trailing "?" are always kept.
func fitFooterHint(parts []string, maxWidth int) string {
	parts = append([]string(nil), parts...)
	if len(parts) == 0 {
		return "?"
	}
	if parts[len(parts)-1] != "?" {
		parts = append(parts, "?")
	}
	join := func(items []string) string {
		return strings.Join(items, " · ")
	}
	if maxWidth <= 0 || lipgloss.Width(join(parts)) <= maxWidth {
		return join(parts)
	}
	for len(parts) > 2 {
		// Drop the second-to-last action (keep primary + "?").
		parts = append(parts[:len(parts)-2], parts[len(parts)-1])
		if lipgloss.Width(join(parts)) <= maxWidth {
			return join(parts)
		}
	}
	if lipgloss.Width(parts[0]) <= maxWidth {
		return parts[0]
	}
	return "?"
}
