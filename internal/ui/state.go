package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

const noticeDuration = 4 * time.Second

const maxGroupDepth = 5

const mobileBreakpoint = 60

type panelLayout struct {
	mobile       bool
	listWidth    int
	detailWidth  int
	bodyHeight   int
	listHeight   int
	detailHeight int
	listTop      int
	detailTop    int
}

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

func (m *App) isMobileLayout() bool {
	return m.terminalWidth() < mobileBreakpoint
}

func (m *App) panelLayout() panelLayout {
	width := m.terminalWidth()
	bodyHeight := max(1, m.terminalHeight()-3)
	const contentTop = 2

	if !m.isMobileLayout() {
		listWidth := min(36, max(12, width/3))
		if listWidth > width-10 {
			listWidth = max(8, width-10)
		}
		detailWidth := max(1, width-listWidth-1)
		return panelLayout{
			mobile:       false,
			listWidth:    listWidth,
			detailWidth:  detailWidth,
			bodyHeight:   bodyHeight,
			listHeight:   bodyHeight,
			detailHeight: bodyHeight,
			listTop:      contentTop,
			detailTop:    contentTop,
		}
	}

	listHeight := max(6, bodyHeight/2)
	if listHeight > bodyHeight-4 {
		listHeight = max(4, bodyHeight-4)
	}
	detailHeight := max(1, bodyHeight-listHeight-1)
	return panelLayout{
		mobile:       true,
		listWidth:    width,
		detailWidth:  width,
		bodyHeight:   bodyHeight,
		listHeight:   listHeight,
		detailHeight: detailHeight,
		listTop:      contentTop,
		detailTop:    contentTop + listHeight + 1,
	}
}

func (m *App) columnDimensions() (listWidth, detailWidth, bodyHeight int) {
	layout := m.panelLayout()
	return layout.listWidth, layout.detailWidth, layout.bodyHeight
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
		hay := strings.ToLower(strings.Join([]string{m.hostLabel(h), h.Alias, h.Resolved.HostName, h.Resolved.User, meta.Group, strings.Join(meta.Tags, " "), meta.Environment, meta.Notes}, " "))
		if strings.Contains(hay, q) {
			out = append(out, h)
		}
	}
	return out
}

type hostRow struct {
	group  string
	host   sshconfig.Host
	header bool
	count  int
	depth  int
}

type hostGroup struct {
	path     string
	hosts    []sshconfig.Host
	children []*hostGroup
	count    int
}

func (m *App) hostRows() []hostRow {
	hosts := m.filteredHosts()
	groups := map[string]*hostGroup{}
	ungrouped := []sshconfig.Host{}
	topLevelOrder := []string{}
	seenTopLevel := map[string]bool{}
	for _, host := range hosts {
		parts := groupPathParts(m.metadata.Host(host.Alias).Group)
		if len(parts) == 0 {
			if !seenTopLevel[""] {
				seenTopLevel[""] = true
				topLevelOrder = append(topLevelOrder, "")
			}
			ungrouped = append(ungrouped, host)
			continue
		}
		if !seenTopLevel[parts[0]] {
			seenTopLevel[parts[0]] = true
			topLevelOrder = append(topLevelOrder, parts[0])
		}
		path := ""
		var parent *hostGroup
		for _, part := range parts {
			if path == "" {
				path = part
			} else {
				path += "/" + part
			}
			node := groups[path]
			if node == nil {
				node = &hostGroup{path: path}
				groups[path] = node
				if parent != nil {
					parent.children = append(parent.children, node)
				}
			}
			node.count++
			parent = node
		}
		parent.hosts = append(parent.hosts, host)
	}
	rows := make([]hostRow, 0, len(hosts)+len(groups))
	for _, group := range topLevelOrder {
		if group == "" {
			for _, host := range ungrouped {
				rows = append(rows, hostRow{host: host})
			}
			continue
		}
		rows = m.appendGroupRows(rows, groups[group], 0)
	}
	return rows
}

func (m *App) appendGroupRows(rows []hostRow, group *hostGroup, depth int) []hostRow {
	rows = append(rows, hostRow{group: group.path, header: true, count: group.count, depth: depth})
	if m.collapsedGroups[group.path] && m.searchText() == "" {
		return rows
	}
	for _, host := range group.hosts {
		rows = append(rows, hostRow{group: group.path, host: host, depth: depth + 1})
	}
	for _, child := range group.children {
		rows = m.appendGroupRows(rows, child, depth+1)
	}
	return rows
}

func normalizeGroupPath(group string) (string, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return "", nil
	}
	parts := strings.Split(group, "/")
	if len(parts) > maxGroupDepth {
		return "", fmt.Errorf("groups can be at most %d levels deep", maxGroupDepth)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return "", fmt.Errorf("group levels cannot be empty")
		}
	}
	return strings.Join(parts, "/"), nil
}

func groupPathParts(group string) []string {
	normalized, err := normalizeGroupPath(group)
	if err != nil {
		group = strings.TrimSpace(group)
		if group == "" {
			return nil
		}
		return []string{group}
	}
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "/")
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
		references := append([]string(nil), k.References...)
		for _, alias := range k.References {
			if host, ok := m.findHost(alias); ok {
				references = append(references, m.hostLabel(host))
			}
		}
		if strings.Contains(strings.ToLower(k.Name+" "+k.Fingerprint+" "+k.Algorithm+" "+strings.Join(references, " ")), q) {
			out = append(out, k)
		}
	}
	return out
}
func (m *App) selectedHost() (sshconfig.Host, bool) {
	rows := m.hostRows()
	if m.cursor >= 0 && m.cursor < len(rows) && !rows[m.cursor].header {
		return rows[m.cursor].host, true
	}
	return sshconfig.Host{}, false
}

func (m *App) selectedGroup() (string, bool) {
	rows := m.hostRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return "", false
	}
	group := rows[m.cursor].group
	return group, group != ""
}

func (m *App) selectedGroupHeader() (string, bool) {
	rows := m.hostRows()
	if m.cursor < 0 || m.cursor >= len(rows) || !rows[m.cursor].header {
		return "", false
	}
	return rows[m.cursor].group, true
}

func groupShortName(path string) string {
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		return path[slash+1:]
	}
	return path
}

func replaceGroupPrefix(group, oldPrefix, newPrefix string) string {
	if group == oldPrefix {
		return newPrefix
	}
	if strings.HasPrefix(group, oldPrefix+"/") {
		return newPrefix + group[len(oldPrefix):]
	}
	return group
}

func (m *App) renameGroup(oldPath, newSegment string) (string, error) {
	newSegment = strings.TrimSpace(newSegment)
	if newSegment == "" {
		return "", fmt.Errorf("group name cannot be empty")
	}
	parts := groupPathParts(oldPath)
	parent := strings.Join(parts[:len(parts)-1], "/")
	newPath := newSegment
	if parent != "" {
		newPath = parent + "/" + newSegment
	}
	normalized, err := normalizeGroupPath(newPath)
	if err != nil {
		return "", err
	}
	if normalized == oldPath {
		return normalized, nil
	}
	for _, host := range m.hosts {
		meta := m.metadata.Host(host.Alias)
		existing, err := normalizeGroupPath(meta.Group)
		if err != nil || existing == "" {
			continue
		}
		if existing != oldPath && !strings.HasPrefix(existing, oldPath+"/") {
			continue
		}
		meta.Group = replaceGroupPrefix(existing, oldPath, normalized)
		if err := m.metadata.SetHost(host.Alias, meta); err != nil {
			return "", err
		}
	}
	if m.collapsedGroups != nil {
		updated := map[string]bool{}
		for path, collapsed := range m.collapsedGroups {
			updated[replaceGroupPrefix(path, oldPath, normalized)] = collapsed
		}
		m.collapsedGroups = updated
	}
	return normalized, nil
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
		return len(m.hostRows())
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

func (m *App) selectAfterLoad() {
	name := m.selectAfterLoadName
	if name == "" {
		m.clampCursor()
		return
	}
	section := m.selectAfterLoadSection
	selectGroup := m.selectAfterLoadGroup
	previousSearch := m.search
	m.search = ""
	index := -1
	if section == hostsSection {
		for i, row := range m.hostRows() {
			if (selectGroup && row.header && row.group == name) || (!selectGroup && !row.header && row.host.Alias == name) {
				index = i
				break
			}
		}
	} else {
		for i, key := range m.keys {
			if key.Name == name {
				index = i
				break
			}
		}
	}
	m.selectAfterLoadName = ""
	m.selectAfterLoadGroup = false
	if index < 0 {
		m.search = previousSearch
		m.clampCursor()
		return
	}
	m.section = section
	m.search = ""
	m.cursor = index
	m.clampCursor()
}

func (m *App) groupExists(group string) bool {
	for _, host := range m.hosts {
		existing, err := normalizeGroupPath(m.metadata.Host(host.Alias).Group)
		if err == nil && (existing == group || strings.HasPrefix(existing, group+"/")) {
			return true
		}
	}
	return false
}

func (m *App) toggleSelectedGroup() tea.Cmd {
	if m.searchText() != "" {
		return m.setNotice("Clear the search filter to collapse groups")
	}
	group, ok := m.selectedGroup()
	if !ok {
		return nil
	}
	if m.collapsedGroups == nil {
		m.collapsedGroups = map[string]bool{}
	}
	m.collapsedGroups[group] = !m.collapsedGroups[group]
	for i, row := range m.hostRows() {
		if row.header && row.group == group {
			m.cursor = i
			break
		}
	}
	return nil
}
func (m *App) searchText() string { return strings.TrimPrefix(m.search, "\x00") }
func (m *App) setError(err error) {
	m.statusID++
	m.status, m.statusError = err.Error(), true
}

func (m *App) setNotice(message string) tea.Cmd {
	m.statusID++
	m.status, m.statusError = message, false
	statusID := m.statusID
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg {
		return clearStatusMsg(statusID)
	})
}

func (m *App) cycleSort() tea.Cmd {
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
		return nil
	}
	m.sortHosts()
	m.cursor = 0
	display := next
	if display == "alias" {
		display = "label"
	}
	return m.setNotice("Sort: " + display)
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
			return strings.ToLower(m.hostLabel(m.hosts[i])) < strings.ToLower(m.hostLabel(m.hosts[j]))
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
		return strings.ToLower(m.hostLabel(m.hosts[i])) < strings.ToLower(m.hostLabel(m.hosts[j]))
	})
}

func (m *App) hostLabel(host sshconfig.Host) string {
	if label := strings.TrimSpace(m.metadata.Host(host.Alias).Label); label != "" {
		return label
	}
	return host.Alias
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
func hostIdentity(h sshconfig.Host) string {
	if passwordOnly(h.Resolved) {
		return "password only"
	}
	return joinOr(h.Resolved.IdentityFiles, "agent/defaults")
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
