package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"bast/internal/doctor"
)

type doctorDoneMsg struct {
	report doctor.Report
}

func (m *App) openDoctor() tea.Cmd {
	m.help, m.credits, m.helpOffset = false, false, 0
	m.onboarding = false
	m.doctor, m.doctorOffset, m.doctorLoading = true, 0, true
	m.doctorReport = doctor.Report{}
	return m.doctorCmd()
}

func (m *App) doctorCmd() tea.Cmd {
	p, client, version := m.paths, m.openSSH, m.version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		return doctorDoneMsg{report: doctor.New(p, client, version).Run(ctx, doctor.Options{})}
	}
}

func (m *App) closeDoctor() {
	m.doctor, m.doctorLoading, m.doctorOffset = false, false, 0
}

func (m *App) doctorLines(s styleSet) []string {
	width := m.helpContentWidth()
	lines := []string{"", s.active.Render("Doctor"), ""}
	if m.doctorLoading {
		return append(lines, s.muted.Render("Checking SSH setup…"))
	}
	if len(m.doctorReport.Fixed) > 0 {
		lines = append(lines, s.success.Render("Repaired"))
		for _, item := range m.doctorReport.Fixed {
			lines = append(lines, s.muted.Render("  "+item))
		}
		lines = append(lines, "")
	}
	current := doctor.Category("")
	for _, f := range m.doctorReport.Findings {
		if f.Category != current {
			if current != "" {
				lines = append(lines, "")
			}
			current = f.Category
			lines = append(lines, s.active.Render(doctor.CategoryTitle(current)), "")
		}
		sev := s.muted
		switch f.Severity {
		case doctor.SeverityFail:
			sev = s.error
		case doctor.SeverityWarn:
			sev = s.value
		case doctor.SeverityOK:
			sev = s.success
		}
		title := f.Title
		if f.Path != "" {
			loc := f.Path
			if f.Line > 0 {
				loc += ":" + strconv.Itoa(f.Line)
			}
			title += " (" + loc + ")"
		}
		lines = append(lines, sev.Render(padRight(string(f.Severity), 5)+" "+truncate(title, max(8, width-8))))
		if f.Detail != "" {
			for _, line := range wrapPlain(f.Detail, max(12, width-6)) {
				lines = append(lines, s.muted.Render("      "+line))
			}
		}
		if f.Fix != "" && f.Severity != doctor.SeverityOK {
			for _, line := range wrapPlain(f.Fix, max(12, width-6)) {
				lines = append(lines, s.muted.Render("      "+line))
			}
		}
	}
	if !m.doctorLoading {
		lines = append(lines, "", s.muted.Render(doctorSummary(m.doctorReport)))
	}
	return lines
}

func doctorSummary(r doctor.Report) string {
	if r.Summary.Fail == 0 && r.Summary.Warn == 0 && r.Summary.Info == 0 {
		return "No issues found"
	}
	var parts []string
	if r.Summary.Fail > 0 {
		parts = append(parts, strconv.Itoa(r.Summary.Fail)+" failed")
	}
	if r.Summary.Warn > 0 {
		parts = append(parts, strconv.Itoa(r.Summary.Warn)+" warnings")
	}
	if r.Summary.Info > 0 {
		parts = append(parts, strconv.Itoa(r.Summary.Info)+" suggestions")
	}
	return strings.Join(parts, ", ") + " · bast doctor --fix repairs a few of these"
}

func (m *App) renderDoctor(s styleSet) string {
	lines := m.doctorLines(s)
	bodyHeight := m.helpBodyHeight()
	offset := min(max(0, m.doctorOffset), max(0, len(lines)-bodyHeight))
	end := min(len(lines), offset+bodyHeight)
	content := lipgloss.NewStyle().
		MarginLeft(2).
		Render(strings.Join(lines[offset:end], "\n"))
	return lipgloss.Place(m.terminalWidth(), bodyHeight, lipgloss.Left, lipgloss.Top, content)
}

func (m *App) maxDoctorOffset() int {
	return max(0, len(m.doctorLines(m.styles()))-m.helpBodyHeight())
}

func (m *App) clampDoctorOffset() {
	if m.doctorOffset < 0 {
		m.doctorOffset = 0
	}
	if maxOffset := m.maxDoctorOffset(); m.doctorOffset > maxOffset {
		m.doctorOffset = maxOffset
	}
}

func (m *App) scrollDoctor(delta int) {
	m.doctorOffset += delta
	m.clampDoctorOffset()
}

func (m *App) doctorCanScroll() bool {
	return m.maxDoctorOffset() > 0
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func wrapPlain(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	words := strings.Fields(text)
	var lines []string
	var current strings.Builder
	for _, word := range words {
		if current.Len() == 0 {
			current.WriteString(word)
			continue
		}
		if current.Len()+1+len(word) > width {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(word)
			continue
		}
		current.WriteByte(' ')
		current.WriteString(word)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}
