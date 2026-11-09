package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"bast/internal/metadata"
)

func listGutter(selected bool, selectedBg color.Color) string {
	if selected {
		return lipgloss.NewStyle().Background(selectedBg).Render(" ")
	}
	return " "
}

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
	line := listGutter(selected, selectedBg) + prefixStyle.Render(rawPrefix)
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
	line := listGutter(selected, selectedBg) + indentStyle.Render(indent) + markStyle.Render(mark)
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

const (
	colorSwatchInner = 6
	colorGridGap     = 2
	colorGridMaxCols = 5
)

func colorGridColumns(width int) int {
	cell := colorSwatchInner + 2
	available := width - 4
	if available < cell {
		return 1
	}
	cols := (available + colorGridGap) / (cell + colorGridGap)
	if cols > colorGridMaxCols {
		return colorGridMaxCols
	}
	if cols < 1 {
		return 1
	}
	return cols
}

func moveColorGrid(index, count, cols int, key string) (int, bool) {
	if count <= 0 || cols < 1 || index < 0 || index >= count {
		return index, false
	}
	col := index % cols
	switch key {
	case "left", "h":
		if col == 0 {
			return index, true
		}
		return index - 1, true
	case "right", "l":
		if col >= cols-1 || index+1 >= count {
			return index, true
		}
		return index + 1, true
	case "up", "k":
		if index < cols {
			return index, true
		}
		return index - cols, true
	case "down", "j":
		if index+cols >= count {
			return index, true
		}
		return index + cols, true
	default:
		return index, false
	}
}

func colorGridLabel(option fieldOption) string {
	if option.custom {
		return "Custom"
	}
	return option.label
}

func padCenter(text string, width int) string {
	if lipgloss.Width(text) > width {
		text = text[:width]
	}
	gap := width - lipgloss.Width(text)
	left := gap / 2
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", gap-left)
}

func colorSwatch(s styleSet, option fieldOption, customHex string, selected bool) string {
	inner := strings.Repeat(" ", colorSwatchInner)
	switch {
	case option.custom && strings.TrimSpace(customHex) != "":
		if fg, ok := colorForeground(customHex); ok {
			inner = lipgloss.NewStyle().Background(fg).Render(inner)
		}
	case option.custom:
		inner = s.muted.Render("  #   ")
	default:
		if code, ok := metadata.ANSICode(option.value); ok {
			inner = lipgloss.NewStyle().Background(lipgloss.Color(strconv.Itoa(code))).Render(inner)
		}
	}
	bracket := s.muted
	if selected {
		bracket = s.active
	}
	return bracket.Render("[") + inner + bracket.Render("]")
}

func renderColorGrid(s styleSet, options []fieldOption, selected int, customHex string, width int) []string {
	if len(options) == 0 {
		return nil
	}
	if selected < 0 || selected >= len(options) {
		selected = 0
	}
	cols := colorGridColumns(width)
	cell := colorSwatchInner + 2
	var lines []string
	for start := 0; start < len(options); start += cols {
		end := min(start+cols, len(options))
		var swatches, labels strings.Builder
		for i := start; i < end; i++ {
			if i > start {
				swatches.WriteString(strings.Repeat(" ", colorGridGap))
				labels.WriteString(strings.Repeat(" ", colorGridGap))
			}
			swatches.WriteString(colorSwatch(s, options[i], customHex, i == selected))
			label := padCenter(colorGridLabel(options[i]), cell)
			if i == selected {
				labels.WriteString(s.selected.Render(label))
			} else {
				labels.WriteString(s.muted.Render(label))
			}
		}
		lines = append(lines, swatches.String(), labels.String())
	}
	return lines
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

func customColorReturnHint(s styleSet, item field, input string) string {
	hint := customChoiceEscapeHint(item, input)
	if hint == "" || item.label != "Color" {
		return ""
	}
	return "    " + s.muted.Render(hint)
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
