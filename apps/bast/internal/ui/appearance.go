package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/metadata"
)

func colorForeground(raw string) (color.Color, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if code, ok := metadata.ANSICode(raw); ok {
		return lipgloss.Color(strconv.Itoa(code)), true
	}
	normalized, err := metadata.NormalizeColor(raw)
	if err != nil || !strings.HasPrefix(normalized, "#") {
		return nil, false
	}
	return lipgloss.Color(normalized), true
}

func renderUserGroupRow(s styleSet, appearance metadata.GroupAppearance, userIcon, rawPrefix, name string, count int, selected bool, rowWidth int) (string, bool) {
	fg, colored := colorForeground(appearance.Color)
	if userIcon == "" && !colored {
		return "", false
	}
	selectedBg := s.selected.GetBackground()
	prefixStyle := s.active
	nameStyle := s.active
	countStyle := s.muted
	iconStyle := lipgloss.NewStyle()
	gapStyle := lipgloss.NewStyle()
	if selected {
		prefixStyle = s.selected
		nameStyle = s.selected
		countStyle = s.muted.Background(selectedBg)
		iconStyle = lipgloss.NewStyle().Background(selectedBg)
		gapStyle = lipgloss.NewStyle().Background(selectedBg)
	}
	if colored {
		nameStyle = nameStyle.Foreground(fg)
	}
	line := prefixStyle.Render(rawPrefix)
	if userIcon != "" {
		line += iconStyle.Render(userIcon)
	}
	line += nameStyle.Render(name) + gapStyle.Render(" ") + countStyle.Render(fmt.Sprintf("(%d)", count))
	if selected {
		if pad := max(0, rowWidth-lipgloss.Width(line)); pad > 0 {
			line += lipgloss.NewStyle().Background(selectedBg).Render(strings.Repeat(" ", pad))
		}
	}
	return line, true
}

func renderUserHostRow(s styleSet, indent, mark, userIcon, label string, colored bool, fg color.Color, selected, stopped bool, rowWidth int) string {
	selectedBg := s.selected.GetBackground()
	markStyle := s.plain
	nameStyle := s.plain
	indentStyle := lipgloss.NewStyle()
	iconStyle := lipgloss.NewStyle()
	if selected {
		markStyle = s.selected
		nameStyle = s.selected
		indentStyle = lipgloss.NewStyle().Background(selectedBg)
		iconStyle = lipgloss.NewStyle().Background(selectedBg)
	} else if stopped {
		markStyle = s.muted
		nameStyle = s.muted
	}
	if colored {
		nameStyle = nameStyle.Foreground(fg)
	}
	line := indentStyle.Render(indent) + markStyle.Render(mark)
	if userIcon != "" {
		line += iconStyle.Render(userIcon)
	}
	line += nameStyle.Render(label)
	if selected {
		if pad := max(0, rowWidth-lipgloss.Width(line)); pad > 0 {
			line += lipgloss.NewStyle().Background(selectedBg).Render(strings.Repeat(" ", pad))
		}
	}
	return line
}

func renderSelectOption(s styleSet, label, value string, selected bool) string {
	if code, ok := metadata.ANSICode(value); ok {
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(strconv.Itoa(code)))
		if selected {
			st = st.Bold(true).Background(s.selected.GetBackground())
			return st.Render("› " + label)
		}
		return st.Render("  " + label)
	}
	if selected {
		return s.selected.Render("› " + label)
	}
	return s.muted.Render("  " + label)
}

func renderClosedChoice(s styleSet, label, value string) string {
	if code, ok := metadata.ANSICode(value); ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(strconv.Itoa(code))).Render(label)
	}
	return s.value.Render(label)
}

func focusedColorValue(item field, input string) string {
	if len(item.options) == 0 {
		return strings.TrimSpace(input)
	}
	option := item.options[item.selected]
	if option.custom {
		return strings.TrimSpace(input)
	}
	return option.value
}

func formColorSample(f *form) string {
	if f == nil {
		return "Preview"
	}
	if label := f.fieldByLabel("Label"); label != nil {
		_, name, err := metadata.SplitLabelPath(label.value)
		if err == nil && strings.TrimSpace(name) != "" {
			return name
		}
	}
	if name := f.fieldByLabel("Name"); name != nil && strings.TrimSpace(name.value) != "" {
		return strings.TrimSpace(name.value)
	}
	return "Preview"
}

func colorPreviewLine(s styleSet, sample, color string) string {
	fg, ok := colorForeground(color)
	if !ok {
		return ""
	}
	if strings.TrimSpace(sample) == "" {
		sample = "Preview"
	}
	return "    " + lipgloss.NewStyle().Bold(true).Foreground(fg).Render(sample) + "\n"
}
