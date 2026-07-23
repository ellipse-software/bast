package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func (m *App) terminalWidth() int {
	if m.width > 0 {
		return m.width
	}
	return 80
}

func (m *App) terminalHeight() int {
	if m.height > 0 {
		return m.height
	}
	return 24
}

func (m *App) columnDimensions() (listWidth, detailWidth, bodyHeight int) {
	width := m.terminalWidth()
	bodyHeight = max(1, m.terminalHeight()-3)
	listWidth = min(36, max(12, width/3))
	if listWidth > width-10 {
		listWidth = max(8, width-10)
	}
	detailWidth = max(1, width-listWidth-1)
	return listWidth, detailWidth, bodyHeight
}

func (m *App) filteredHosts() []sshconfig.Host {
	q := strings.ToLower(m.searchText())
	out := []sshconfig.Host{}
	for _, h := range m.hosts {
		meta := m.metadata.Host(h.Alias)
		if meta.Hidden && !m.showHidden {
			continue
		}
		if q == "" {
			out = append(out, h)
			continue
		}
		hay := strings.ToLower(strings.Join([]string{h.Alias, h.Resolved.HostName, h.Resolved.User, meta.Group, strings.Join(meta.Tags, " "), meta.Environment, meta.Notes}, " "))
		if strings.Contains(hay, q) {
			out = append(out, h)
		}
	}
	return out
}

func (m *App) hasHiddenHosts() bool {
	for _, host := range m.hosts {
		if m.metadata.Host(host.Alias).Hidden {
			return true
		}
	}
	return false
}
func (m *App) filteredKeys() []keys.Key {
	q := strings.ToLower(m.searchText())
	if q == "" {
		return m.keys
	}
	out := []keys.Key{}
	for _, k := range m.keys {
		if strings.Contains(strings.ToLower(k.Name+" "+k.Fingerprint+" "+k.Algorithm+" "+strings.Join(k.References, " ")), q) {
			out = append(out, k)
		}
	}
	return out
}
func (m *App) selectedHost() (sshconfig.Host, bool) {
	items := m.filteredHosts()
	if m.cursor >= 0 && m.cursor < len(items) {
		return items[m.cursor], true
	}
	return sshconfig.Host{}, false
}
func (m *App) selectedKey() (keys.Key, bool) {
	items := m.filteredKeys()
	if m.cursor >= 0 && m.cursor < len(items) {
		return items[m.cursor], true
	}
	return keys.Key{}, false
}
func (m *App) findHost(alias string) (sshconfig.Host, bool) {
	for _, h := range m.hosts {
		if h.Alias == alias {
			return h, true
		}
	}
	return sshconfig.Host{}, false
}
func (m *App) findKey(name string) (keys.Key, bool) {
	for _, k := range m.keys {
		if k.Name == name {
			return k, true
		}
	}
	return keys.Key{}, false
}
func (m *App) itemCount() int {
	if m.section == hostsSection {
		return len(m.filteredHosts())
	}
	return len(m.filteredKeys())
}
func (m *App) clampCursor() {
	if n := m.itemCount(); n == 0 {
		m.cursor = 0
	} else if m.cursor >= n {
		m.cursor = n - 1
	}
}
func (m *App) searchText() string { return strings.TrimPrefix(m.search, "\x00") }
func (m *App) setError(err error) { m.status, m.statusError = err.Error(), true }

func (m *App) cycleSort() {
	orders := []string{"smart", "alias", "recent", "group"}
	current := m.metadata.Preferences().Sort
	next := orders[0]
	for i, v := range orders {
		if v == current {
			next = orders[(i+1)%len(orders)]
			break
		}
	}
	if err := m.metadata.SetSort(next); err != nil {
		m.setError(err)
		return
	}
	m.sortHosts()
	m.cursor = 0
	display := next
	if display == "alias" {
		display = "label"
	}
	m.status, m.statusError = "Sort: "+display, false
}
func (m *App) sortHosts() {
	order := m.metadata.Preferences().Sort
	if order == "" {
		order = "smart"
	}
	sort.SliceStable(m.hosts, func(i, j int) bool {
		a, b := m.metadata.Host(m.hosts[i].Alias), m.metadata.Host(m.hosts[j].Alias)
		switch order {
		case "alias":
			return strings.ToLower(m.hosts[i].Alias) < strings.ToLower(m.hosts[j].Alias)
		case "recent":
			return later(a.LastUsedAt, b.LastUsedAt)
		case "group":
			if a.Group != b.Group {
				return strings.ToLower(a.Group) < strings.ToLower(b.Group)
			}
		}
		if a.Favorite != b.Favorite {
			return a.Favorite
		}
		if (a.LastUsedAt != nil) != (b.LastUsedAt != nil) {
			return a.LastUsedAt != nil
		}
		if a.LastUsedAt != nil && !a.LastUsedAt.Equal(*b.LastUsedAt) {
			return a.LastUsedAt.After(*b.LastUsedAt)
		}
		return strings.ToLower(m.hosts[i].Alias) < strings.ToLower(m.hosts[j].Alias)
	})
}

func destination(h sshconfig.Host) string {
	user := ""
	if h.Resolved.User != "" {
		user = h.Resolved.User + "@"
	}
	port := ""
	if h.Resolved.Port != "" && h.Resolved.Port != "22" {
		port = ":" + h.Resolved.Port
	}
	return user + h.Resolved.HostName + port
}
func usage(h metadata.Host) string {
	if h.LastUsedAt == nil {
		return "never"
	}
	return h.LastUsedAt.Local().Format("2006-01-02 15:04") + fmt.Sprintf(" (%d)", h.ConnectionCount)
}
func noneValue(s string) string {
	if s == "" || s == "none" {
		return "—"
	}
	return s
}
func emptyIfNone(s string) string {
	if s == "none" || s == "—" {
		return ""
	}
	return s
}
func joinOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
func shortPath(path, home string) string {
	if path == "" {
		return "—"
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + rel
	}
	return path
}
func truncate(s string, width int) string {
	r := []rune(s)
	if width < 2 {
		return ""
	}
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
func scrollStart(cursor, total, height int) int {
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	if start+height > total {
		start = max(0, total-height)
	}
	return start
}
func later(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}
