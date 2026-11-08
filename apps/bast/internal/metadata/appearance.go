package metadata

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// ANSIColor is one theme colour. Code is the 0-15 ANSI index lipgloss turns
// into a theme-resolved SGR colour. Gray is bright black (8), not ANSI black.
type ANSIColor struct {
	Token string
	Label string
	Code  int
}

// ANSIPalette is the colour picker, in display order.
func ANSIPalette() []ANSIColor {
	return []ANSIColor{
		{Token: "red", Label: "Red", Code: 1},
		{Token: "green", Label: "Green", Code: 2},
		{Token: "yellow", Label: "Yellow", Code: 3},
		{Token: "blue", Label: "Blue", Code: 4},
		{Token: "magenta", Label: "Magenta", Code: 5},
		{Token: "cyan", Label: "Cyan", Code: 6},
		{Token: "white", Label: "White", Code: 7},
		{Token: "gray", Label: "Gray", Code: 8},
	}
}

// ANSIColorTokens returns the stored tokens, for CLI completion.
func ANSIColorTokens() []string {
	palette := ANSIPalette()
	out := make([]string, len(palette))
	for i, color := range palette {
		out[i] = color.Token
	}
	return out
}

// GroupAppearance is the icon and colour for one group path.
type GroupAppearance struct {
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

func (g GroupAppearance) empty() bool {
	return g.Icon == "" && g.Color == ""
}

// NormalizeIcon accepts an empty value or one emoji or flag. The result is
// one grapheme cluster, one or two columns wide.
func NormalizeIcon(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("icon cannot contain control characters")
		}
	}
	if graphemeCount(raw) != 1 {
		return "", fmt.Errorf("icon must be a single emoji or flag")
	}
	width := ansi.StringWidth(raw)
	if width < 1 || width > 2 {
		return "", fmt.Errorf("icon must be one or two columns wide")
	}
	return raw, nil
}

// NormalizeColor accepts an empty value, an ANSI palette token, or #RGB / #RRGGBB.
// Tokens are stored lowercase. Hex is stored as uppercase #RRGGBB.
func NormalizeColor(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	for _, color := range ANSIPalette() {
		if strings.EqualFold(raw, color.Token) || strings.EqualFold(raw, color.Label) {
			return color.Token, nil
		}
	}
	if strings.HasPrefix(raw, "#") {
		hex, ok := normalizeHexColor(raw)
		if !ok {
			return "", fmt.Errorf("colour must be a theme name or hex value")
		}
		return hex, nil
	}
	return "", fmt.Errorf("colour must be a theme name or hex value")
}

// ANSICode reports the 0-15 index for a stored palette token.
func ANSICode(token string) (int, bool) {
	for _, color := range ANSIPalette() {
		if color.Token == token {
			return color.Code, true
		}
	}
	return 0, false
}

func normalizeHexColor(raw string) (string, bool) {
	hex := strings.TrimPrefix(strings.TrimSpace(raw), "#")
	switch len(hex) {
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 6:
	default:
		return "", false
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "", false
		}
	}
	return "#" + strings.ToUpper(hex), true
}

func graphemeCount(s string) int {
	n := 0
	rest := s
	for rest != "" {
		cluster, _ := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		if cluster == "" || len(cluster) > len(rest) {
			return n + 1
		}
		n++
		rest = rest[len(cluster):]
	}
	return n
}
