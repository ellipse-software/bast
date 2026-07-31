package ui

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/metadata"
)

type groupPickerChoice struct {
	label  string
	value  string
	create bool
	score  int
}

func isGroupAssignmentForm(f *form) bool {
	return f != nil && f.action == "group_assign"
}

func (m *App) updateGroupAssignmentForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := m.form
	choices := m.groupPickerChoices()
	key := msg.String()
	switch key {
	case "esc":
		m.form = nil
		return m, nil
	case "up", "shift+tab", "down", "tab":
		if len(choices) > 0 {
			direction := 1
			if key == "up" || key == "shift+tab" {
				direction = -1
			}
			item := &f.fields[f.index]
			item.selected = (item.selected + direction + len(choices)) % len(choices)
		}
		return m, nil
	case "enter":
		if len(choices) == 0 {
			return m, nil
		}
		selected := min(f.fields[f.index].selected, len(choices)-1)
		f.fields[f.index].value = choices[selected].value
		return m.submitForm()
	}

	return m.updateGroupAssignmentInput(msg)
}

func (m *App) updateGroupAssignmentInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.form
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() == before {
		return m, cmd
	}
	m.resetGroupPickerSelection()
	f.validationError = ""
	return m, cmd
}

func (m *App) groupPickerChoices() []groupPickerChoice {
	f := m.form
	item := f.fieldByLabel("Group")
	if item == nil {
		return nil
	}
	query := strings.TrimSpace(f.input.Value())
	if query == "" {
		choices := make([]groupPickerChoice, 0, len(item.options))
		for _, option := range item.options {
			choices = append(choices, groupPickerChoice{label: option.label, value: option.value})
		}
		return choices
	}

	normalized, err := metadata.NormalizeGroupPath(query)
	matchQuery := query
	if err == nil {
		matchQuery = normalized
	}

	choices := make([]groupPickerChoice, 0, len(item.options)+1)
	exact := false
	for _, option := range item.options {
		if option.value == "" {
			continue
		}
		if err == nil && strings.EqualFold(option.value, normalized) {
			exact = true
		}
		score, ok := fuzzyGroupScore(option.value, matchQuery)
		if ok {
			choices = append(choices, groupPickerChoice{label: option.label, value: option.value, score: score})
		}
	}
	sort.SliceStable(choices, func(i, j int) bool {
		left, right := choices[i], choices[j]
		if left.score != right.score {
			return left.score > right.score
		}
		leftLength, rightLength := len([]rune(left.value)), len([]rune(right.value))
		if leftLength != rightLength {
			return leftLength < rightLength
		}
		return strings.ToLower(left.value) < strings.ToLower(right.value)
	})

	if !exact {
		create := groupPickerChoice{
			label:  fmt.Sprintf("Create %q", query),
			value:  query,
			create: true,
		}
		choices = slices.Insert(choices, min(1, len(choices)), create)
	}
	return choices
}

func (m *App) resetGroupPickerSelection() {
	item := m.form.fieldByLabel("Group")
	if item == nil {
		return
	}
	item.selected = 0
	if strings.TrimSpace(m.form.input.Value()) != "" {
		return
	}
	current := groupAssignmentCurrentGroup(m.form)
	for i, option := range item.options {
		if option.value == current {
			item.selected = i
			return
		}
	}
}

func groupAssignmentCurrentGroup(f *form) string {
	if field := f.fieldByLabel("Current group"); field != nil {
		return field.value
	}
	return ""
}

func fuzzyGroupScore(candidate, query string) (int, bool) {
	candidateLower := strings.ToLower(candidate)
	queryLower := strings.ToLower(strings.TrimSpace(query))
	candidateRunes := []rune(candidateLower)
	queryRunes := []rune(queryLower)
	if len(queryRunes) == 0 {
		return 0, true
	}

	const (
		noMatch            = -1 << 30
		baseMatchScore     = 10
		groupBoundaryBonus = 12
		consecutiveBonus   = 8
		exactMatchBonus    = 300
		prefixMatchBonus   = 150
		substringBonus     = 100
	)
	previousScores := make([]int, len(candidateRunes))
	for i := range previousScores {
		previousScores[i] = noMatch
	}
	for queryIndex, wanted := range queryRunes {
		currentScores := make([]int, len(candidateRunes))
		for i := range currentScores {
			currentScores[i] = noMatch
		}
		matched := false
		for candidateIndex, candidateRune := range candidateRunes {
			if candidateRune != wanted {
				continue
			}
			matchScore := baseMatchScore
			if candidateIndex == 0 || candidateRunes[candidateIndex-1] == '/' || candidateRunes[candidateIndex-1] == ' ' || candidateRunes[candidateIndex-1] == '-' {
				matchScore += groupBoundaryBonus
			}
			if queryIndex == 0 {
				currentScores[candidateIndex] = matchScore - candidateIndex
				matched = true
				continue
			}
			for previousIndex := 0; previousIndex < candidateIndex; previousIndex++ {
				if previousScores[previousIndex] == noMatch {
					continue
				}
				score := previousScores[previousIndex] + matchScore - (candidateIndex - previousIndex - 1)
				if candidateIndex == previousIndex+1 {
					score += consecutiveBonus
				}
				currentScores[candidateIndex] = max(currentScores[candidateIndex], score)
			}
			matched = matched || currentScores[candidateIndex] != noMatch
		}
		if !matched {
			return 0, false
		}
		previousScores = currentScores
	}

	bestScore := noMatch
	for _, score := range previousScores {
		bestScore = max(bestScore, score)
	}
	switch {
	case candidateLower == queryLower:
		bestScore += exactMatchBonus
	case strings.HasPrefix(candidateLower, queryLower):
		bestScore += prefixMatchBonus
	case strings.Contains(candidateLower, queryLower):
		bestScore += substringBonus
	}
	bestScore -= len(candidateRunes) - len(queryRunes)
	return bestScore, true
}

func (m *App) renderGroupAssignmentForm(s styleSet) string {
	f := m.form
	item := f.fieldByLabel("Group")
	choices := m.groupPickerChoices()
	if item.selected >= len(choices) {
		item.selected = max(0, len(choices)-1)
	}

	var b strings.Builder
	b.WriteString("\n  " + s.active.Render(f.title) + "\n\n")
	b.WriteString("  " + s.active.Render("› Group") + "\n")
	b.WriteString("    " + s.muted.Render("Search existing groups or type a new path") + "\n")
	b.WriteString("    " + f.input.View() + "\n")

	current := groupAssignmentCurrentGroup(f)
	currentLabel := current
	if currentLabel == "" {
		currentLabel = "No group"
	}
	b.WriteString("    " + s.muted.Render("Current: "+currentLabel) + "\n\n")

	rows := min(7, len(choices))
	start := scrollStart(item.selected, len(choices), rows)
	for i := start; i < min(len(choices), start+rows); i++ {
		choice := choices[i]
		label := choice.label
		if choice.value == current && !choice.create {
			label += "  current"
		}
		if choice.create {
			label = "+ " + label
		}
		if i == item.selected {
			b.WriteString("    " + s.selected.Render("› "+label) + "\n")
		} else {
			b.WriteString("    " + s.muted.Render("  "+label) + "\n")
		}
	}
	if f.validationError != "" {
		b.WriteString("\n    " + s.error.Render("✕ "+f.validationError) + "\n")
	}
	return b.String()
}

func groupAssignmentHint() string {
	return "Type to filter • ↑/↓ choose • Enter assign/create • Esc cancel"
}
