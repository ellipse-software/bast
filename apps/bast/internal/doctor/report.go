package doctor

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

type Severity string

const (
	SeverityFail Severity = "fail"
	SeverityWarn Severity = "warn"
	SeverityInfo Severity = "info"
	SeverityOK   Severity = "ok"
)

type Category string

const (
	CatEnv         Category = "env"
	CatPermissions Category = "permissions"
	CatSSHConfig   Category = "ssh_config"
	CatKeys        Category = "keys"
	CatAgent       Category = "agent"
	CatKnownHosts  Category = "known_hosts"
	CatMetadata    Category = "metadata"
	CatVault       Category = "vault"
	CatSync        Category = "sync"
	CatSuggest     Category = "suggest"
	CatProbe       Category = "probe"
)

var categoryOrder = []Category{
	CatEnv, CatPermissions, CatSSHConfig, CatKeys, CatAgent, CatKnownHosts,
	CatMetadata, CatVault, CatSync, CatSuggest, CatProbe,
}

var categoryTitles = map[Category]string{
	CatEnv:         "OpenSSH",
	CatPermissions: "Permissions",
	CatSSHConfig:   "SSH config",
	CatKeys:        "Keys",
	CatAgent:       "Agent",
	CatKnownHosts:  "Known hosts",
	CatMetadata:    "Metadata",
	CatVault:       "Vault",
	CatSync:        "Sync",
	CatSuggest:     "Suggestions",
	CatProbe:       "Connectivity",
}

type Finding struct {
	ID       string   `json:"id"`
	Severity Severity `json:"severity"`
	Category Category `json:"category"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail,omitempty"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Host     string   `json:"host,omitempty"`
	Fix      string   `json:"fix,omitempty"`
	Fixable  bool     `json:"fixable"`
}

type Summary struct {
	Fail int `json:"fail"`
	Warn int `json:"warn"`
	Info int `json:"info"`
	OK   int `json:"ok"`
}

type Report struct {
	Healthy  bool      `json:"healthy"`
	Summary  Summary   `json:"summary"`
	Findings []Finding `json:"findings"`
	Fixed    []string  `json:"fixed,omitempty"`
}

func (r *Report) add(f Finding) {
	r.Findings = append(r.Findings, f)
}

func (r *Report) finalize() {
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	r.Summary = Summary{}
	for _, f := range r.Findings {
		switch f.Severity {
		case SeverityFail:
			r.Summary.Fail++
		case SeverityWarn:
			r.Summary.Warn++
		case SeverityInfo:
			r.Summary.Info++
		case SeverityOK:
			r.Summary.OK++
		}
	}
	r.Healthy = r.Summary.Fail == 0
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Category != b.Category {
			return categoryIndex(a.Category) < categoryIndex(b.Category)
		}
		if sa, sb := severityRank(a.Severity), severityRank(b.Severity); sa != sb {
			return sa < sb
		}
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.ID < b.ID
	})
}

func (r *Report) filter(categories []string) {
	if len(categories) == 0 {
		return
	}
	allow := map[Category]bool{}
	for _, c := range categories {
		allow[Category(c)] = true
	}
	out := r.Findings[:0]
	for _, f := range r.Findings {
		if allow[f.Category] {
			out = append(out, f)
		}
	}
	r.Findings = out
}

func (r Report) HasFail() bool { return r.Summary.Fail > 0 }

func categoryIndex(c Category) int {
	for i, item := range categoryOrder {
		if item == c {
			return i
		}
	}
	return len(categoryOrder)
}

func severityRank(s Severity) int {
	switch s {
	case SeverityFail:
		return 0
	case SeverityWarn:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}

func ValidCategory(name string) bool {
	_, ok := categoryTitles[Category(name)]
	return ok
}

func CategoryTitle(c Category) string {
	if title, ok := categoryTitles[c]; ok {
		return title
	}
	return string(c)
}

type formatStyles struct {
	chip   lipgloss.Style
	active lipgloss.Style
	muted  lipgloss.Style
	fail   lipgloss.Style
	ok     lipgloss.Style
	warn   lipgloss.Style
	text   lipgloss.Style
}

func tuiStyles() formatStyles {
	primary := lipgloss.Color("#8B5CF6")
	muted := lipgloss.Color("#6B7280")
	return formatStyles{
		chip:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(primary),
		active: lipgloss.NewStyle().Bold(true).Foreground(primary),
		muted:  lipgloss.NewStyle().Foreground(muted),
		fail:   lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")),
		ok:     lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")),
		warn:   lipgloss.NewStyle().Bold(true),
		text:   lipgloss.NewStyle(),
	}
}

func Format(w io.Writer, r Report) error {
	_, err := lipgloss.Fprint(w, render(r, outputWidth(w)))
	return err
}

func render(r Report, width int) string {
	s := tuiStyles()
	indent := 8
	wrapAt := max(24, width-indent-2)
	var b strings.Builder
	b.WriteString(s.chip.Render(" BAST "))
	b.WriteString("  ")
	b.WriteString(s.muted.Render("doctor"))
	b.WriteByte('\n')
	if len(r.Fixed) > 0 {
		b.WriteByte('\n')
		b.WriteString(s.ok.Render("Repaired"))
		b.WriteByte('\n')
		for _, item := range r.Fixed {
			b.WriteString(s.muted.Render("  " + item))
			b.WriteByte('\n')
		}
	}
	current := Category("")
	for _, f := range r.Findings {
		if f.Category != current {
			b.WriteByte('\n')
			current = f.Category
			b.WriteString(s.active.Render(CategoryTitle(current)))
			b.WriteString("\n\n")
		}
		sevStyle, titleStyle := severityStyles(s, f.Severity)
		b.WriteString("  ")
		b.WriteString(sevStyle.Render(padSev(f.Severity)))
		b.WriteString("  ")
		b.WriteString(titleStyle.Render(f.Title))
		b.WriteByte('\n')
		if loc := location(f); loc != "" {
			b.WriteString(s.muted.Render(strings.Repeat(" ", indent) + loc))
			b.WriteByte('\n')
		}
		if f.Detail != "" {
			for _, line := range wrapWords(f.Detail, wrapAt) {
				b.WriteString(s.muted.Render(strings.Repeat(" ", indent) + line))
				b.WriteByte('\n')
			}
		}
		if f.Fix != "" && f.Severity != SeverityOK {
			text := f.Fix
			if f.Fixable {
				text = "Fix: " + f.Fix
			}
			for _, line := range wrapWords(text, wrapAt) {
				b.WriteString(s.muted.Render(strings.Repeat(" ", indent) + line))
				b.WriteByte('\n')
			}
		}
	}
	b.WriteByte('\n')
	b.WriteString(renderSummary(s, r))
	b.WriteByte('\n')
	return b.String()
}

func severityStyles(s formatStyles, sev Severity) (label, title lipgloss.Style) {
	switch sev {
	case SeverityFail:
		return s.fail, s.fail
	case SeverityOK:
		return s.ok, s.ok
	case SeverityWarn:
		return s.warn, s.text
	default:
		return s.muted, s.muted
	}
}

func padSev(sev Severity) string {
	label := string(sev)
	if n := 4 - len(label); n > 0 {
		label += strings.Repeat(" ", n)
	}
	return label
}

func renderSummary(s formatStyles, r Report) string {
	if r.Summary.Fail == 0 && r.Summary.Warn == 0 && r.Summary.Info == 0 {
		return s.ok.Render("No issues found")
	}
	var parts []string
	if r.Summary.Fail > 0 {
		parts = append(parts, s.fail.Render(fmt.Sprintf("%d failed", r.Summary.Fail)))
	}
	if r.Summary.Warn > 0 {
		parts = append(parts, s.warn.Render(fmt.Sprintf("%d warnings", r.Summary.Warn)))
	}
	if r.Summary.Info > 0 {
		parts = append(parts, s.muted.Render(fmt.Sprintf("%d suggestions", r.Summary.Info)))
	}
	return strings.Join(parts, s.muted.Render(", "))
}

func outputWidth(w io.Writer) int {
	const fallback = 80
	file, ok := w.(*os.File)
	if !ok {
		return fallback
	}
	cols, _, err := term.GetSize(file.Fd())
	if err != nil || cols < 48 {
		return fallback
	}
	return cols
}

func location(f Finding) string {
	if f.Path == "" {
		return ""
	}
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.Path, f.Line)
	}
	return f.Path
}

func wrapWords(text string, width int) []string {
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
