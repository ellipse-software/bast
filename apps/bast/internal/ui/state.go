package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud"
	boxcloud "bast/internal/cloud/box"
	cloudsync "bast/internal/cloud/sync"
	upstashcloud "bast/internal/cloud/upstash"
	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/platform"
	"bast/internal/sshconfig"
)

const noticeDuration = 4 * time.Second

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
		listWidth := max(1, (width-1)/2)
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
	return m.filteredHostsWithMetadata(m.hostMetadata())
}

func (m *App) hostMetadata() map[string]metadata.Host {
	if m.hostMeta == nil || m.hostMetaRevision != m.metadata.HostRevision() {
		m.refreshHostMetadata()
	}
	return m.hostMeta
}

func (m *App) refreshHostMetadata() map[string]metadata.Host {
	m.hostMeta, m.hostMetaRevision = m.metadata.HostsSnapshot()
	return m.hostMeta
}

func (m *App) filteredHostsWithMetadata(hostMetadata map[string]metadata.Host) []sshconfig.Host {
	q := strings.ToLower(m.searchText())
	out := make([]sshconfig.Host, 0, len(m.hosts))
	for _, h := range m.hosts {
		meta := hostMetadata[h.Alias]
		if meta.Hidden && !m.showHidden {
			continue
		}
		if !m.showHidden && q == "" && hostLooksStopped(h, meta) {
			continue
		}
		if q == "" {
			out = append(out, h)
			continue
		}
		hay := strings.ToLower(strings.Join([]string{hostLabel(h, meta), h.Alias, h.Resolved.HostName, h.Resolved.User, meta.Group, strings.Join(meta.Tags, " "), meta.Environment, meta.Notes}, " "))
		if strings.Contains(hay, q) {
			out = append(out, h)
		}
	}
	return out
}

type hostRow struct {
	group         string
	label         string
	host          sshconfig.Host
	suggestion    *metadata.HistorySuggestion
	historyHeader bool
	header        bool
	count         int
	depth         int
}

type hostGroup struct {
	path     string
	hosts    []sshconfig.Host
	children []*hostGroup
	count    int
}

func (m *App) hostRows() []hostRow {
	hostMetadata := m.hostMetadata()
	search := m.searchText()
	hostSignature := hostListSignature(m.hosts)
	cache := m.hostRowsCache
	if cache.rows != nil &&
		cache.hostGeneration == m.hostGeneration &&
		cache.metadataRevision == m.hostMetaRevision &&
		cache.collapseGeneration == m.collapseRevision &&
		cache.search == search &&
		cache.showHidden == m.showHidden &&
		cache.boxEnabled == m.metadata.Box().Enabled &&
		cache.hostSignature == hostSignature {
		return cache.rows
	}
	hosts := m.filteredHostsWithMetadata(hostMetadata)
	groups := map[string]*hostGroup{}
	ungrouped := []sshconfig.Host{}
	topLevelOrder := []string{}
	seenTopLevel := map[string]bool{}
	for _, host := range hosts {
		parts := groupPathParts(hostMetadata[host.Alias].Group)
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
	for _, d := range cloud.Descriptors() {
		if seenTopLevel[d.GroupRoot] {
			continue
		}
		if !m.shouldInjectProviderRoot(d.Kind) {
			continue
		}
		seenTopLevel[d.GroupRoot] = true
		topLevelOrder = append(topLevelOrder, d.GroupRoot)
		groups[d.GroupRoot] = &hostGroup{path: d.GroupRoot}
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
	m.hostRowsCache = hostRowsCache{
		hostGeneration: m.hostGeneration, metadataRevision: m.hostMetaRevision,
		collapseGeneration: m.collapseRevision, search: search,
		showHidden: m.showHidden, boxEnabled: m.metadata.Box().Enabled,
		hostSignature: hostSignature, rows: rows,
	}
	return rows
}

func (m *App) hostListRows() []hostRow {
	base := m.hostRows()
	search := m.searchText()
	suggestionSig := historySuggestionSignature(m.historySuggestions)
	cache := m.hostListRowsCache
	if cache.rows != nil &&
		cache.hostGeneration == m.hostGeneration &&
		cache.metadataRevision == m.hostMetaRevision &&
		cache.collapseGeneration == m.collapseRevision &&
		cache.search == search &&
		cache.showHidden == m.showHidden &&
		cache.boxEnabled == m.hostRowsCache.boxEnabled &&
		cache.hostSignature == m.hostRowsCache.hostSignature &&
		cache.historyCollapsed == m.historySuggestionsCollapsed &&
		cache.suggestionSig == suggestionSig {
		return cache.rows
	}
	suggestions := m.visibleHistorySuggestions()
	rows := base
	if len(suggestions) > 0 {
		rows = make([]hostRow, 0, len(base)+1+len(suggestions))
		rows = append(rows, base...)
		rows = append(rows, hostRow{historyHeader: true, header: true, label: "(Suggested)", count: len(suggestions)})
		if !(m.historySuggestionsCollapsed && search == "") {
			for i := range suggestions {
				suggestion := suggestions[i]
				rows = append(rows, hostRow{suggestion: &suggestion, depth: 1})
			}
		}
	}
	m.hostListRowsCache = hostListRowsCache{
		hostGeneration: m.hostGeneration, metadataRevision: m.hostMetaRevision,
		collapseGeneration: m.collapseRevision, search: search,
		showHidden: m.showHidden, boxEnabled: m.hostRowsCache.boxEnabled,
		hostSignature:    m.hostRowsCache.hostSignature,
		historyCollapsed: m.historySuggestionsCollapsed, suggestionSig: suggestionSig, rows: rows,
	}
	return rows
}

func historySuggestionSignature(suggestions []metadata.HistorySuggestion) uint64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	signature := offset
	for _, suggestion := range suggestions {
		for i := range len(suggestion.ID) {
			signature = (signature ^ uint64(suggestion.ID[i])) * prime
		}
		signature = (signature ^ 0xff) * prime
	}
	return signature ^ uint64(len(suggestions))
}

func hostListSignature(hosts []sshconfig.Host) uint64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	signature := offset
	writeString := func(value string) {
		for i := range len(value) {
			signature = (signature ^ uint64(value[i])) * prime
		}
		signature = (signature ^ 0xff) * prime
	}
	for _, host := range hosts {
		writeString(host.Alias)
		writeString(host.Source)
		writeString(host.ManagedID)
		writeString(host.SyncSource)
		writeString(host.SyncID)
		writeString(host.Resolved.HostName)
		writeString(host.Resolved.User)
		writeString(host.Resolved.Port)
		writeString(host.Resolved.IdentitiesOnly)
		writeString(host.Resolved.PubkeyAuthentication)
		writeString(host.Resolved.PasswordAuthentication)
		writeString(host.Resolved.PreferredAuthentications)
		writeString(host.Resolved.ProxyJump)
		for _, identity := range host.Resolved.IdentityFiles {
			writeString(identity)
		}
		flags := uint64(0)
		if host.Managed {
			flags |= 1
		}
		if host.Synced {
			flags |= 2
		}
		if host.KnownHost {
			flags |= 4
		}
		signature = (signature ^ flags ^ uint64(host.Line)) * prime
	}
	return signature
}

func (m *App) appendGroupRows(rows []hostRow, group *hostGroup, depth int) []hostRow {
	label := groupShortName(group.path)
	keepProviderRoot := depth == 0 && cloudsync.IsSyncedGroup(group.path)
	for !keepProviderRoot && len(group.hosts) == 0 && len(group.children) == 1 {
		group = group.children[0]
		label += "/" + groupShortName(group.path)
	}
	rows = append(rows, hostRow{group: group.path, label: label, header: true, count: group.count, depth: depth})
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
	return metadata.NormalizeGroupPath(group)
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
	hostMetadata := m.hostMetadata()
	for _, host := range m.hosts {
		if hostMetadata[host.Alias].Hidden {
			return true
		}
		if hostLooksStopped(host, hostMetadata[host.Alias]) {
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
	out := make([]keys.Key, 0, len(m.keys))
	hostMetadata := m.hostMetadata()
	hostLabels := make(map[string]string, len(m.hosts))
	for _, host := range m.hosts {
		hostLabels[host.Alias] = hostLabel(host, hostMetadata[host.Alias])
	}
	for _, k := range m.keys {
		references := append([]string(nil), k.References...)
		for _, alias := range k.References {
			if label, ok := hostLabels[alias]; ok {
				references = append(references, label)
			}
		}
		if strings.Contains(strings.ToLower(k.Name+" "+k.Fingerprint+" "+k.Algorithm+" "+strings.Join(references, " ")), q) {
			out = append(out, k)
		}
	}
	return out
}
func (m *App) selectedHost() (sshconfig.Host, bool) {
	rows := m.hostListRows()
	if m.cursor >= 0 && m.cursor < len(rows) && !rows[m.cursor].header && rows[m.cursor].suggestion == nil {
		return rows[m.cursor].host, true
	}
	return sshconfig.Host{}, false
}

func (m *App) selectedGroup() (string, bool) {
	rows := m.hostListRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return "", false
	}
	group := rows[m.cursor].group
	return group, group != ""
}

func (m *App) selectedGroupHeader() (string, bool) {
	rows := m.hostListRows()
	if m.cursor < 0 || m.cursor >= len(rows) || !rows[m.cursor].header || rows[m.cursor].historyHeader {
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
	parent := ""
	if len(parts) > 0 {
		parent = strings.Join(parts[:len(parts)-1], "/")
	}
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
	if m.groupExists(normalized) {
		return "", fmt.Errorf("group %q already exists; choose a different name to avoid merging hosts", normalized)
	}
	aliases := make(map[string]bool, len(m.hosts))
	for _, host := range m.hosts {
		aliases[host.Alias] = true
	}
	if err := m.metadata.UpdateHosts(func(hosts map[string]metadata.Host) {
		for alias, meta := range hosts {
			if !aliases[alias] {
				continue
			}
			existing, err := normalizeGroupPath(meta.Group)
			if err != nil || existing == "" || (existing != oldPath && !strings.HasPrefix(existing, oldPath+"/")) {
				continue
			}
			meta.Group = replaceGroupPrefix(existing, oldPath, normalized)
			hosts[alias] = meta
		}
	}); err != nil {
		return "", err
	}
	if m.collapsedGroups != nil {
		updated := map[string]bool{}
		for path, collapsed := range m.collapsedGroups {
			updated[replaceGroupPrefix(path, oldPath, normalized)] = collapsed
		}
		m.collapsedGroups = updated
		m.collapseRevision++
		if err := m.persistCollapsedGroups(); err != nil {
			m.setError(err)
		}
	}
	m.refreshHostMetadata()
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
	switch m.section {
	case hostsSection:
		return len(m.hostListRows())
	case keysSection:
		return len(m.filteredKeys())
	case filesSection:
		return 0
	default:
		return len(m.syncMenuItems())
	}
}
func (m *App) clampCursor() {
	if n := m.itemCount(); n == 0 {
		m.cursor = 0
	} else if m.cursor >= n {
		m.cursor = n - 1
	} else if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *App) hostCursorKey() (kind, name string) {
	rows := m.hostListRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return "", ""
	}
	row := rows[m.cursor]
	switch {
	case row.historyHeader:
		return "historyHeader", ""
	case row.suggestion != nil:
		return "suggestion", row.suggestion.ID
	case row.header:
		return "group", row.group
	default:
		return "host", row.host.Alias
	}
}

func (m *App) restoreHostCursor(kind, name string) {
	prev := m.cursor
	rows := m.hostListRows()
	for i, row := range rows {
		switch kind {
		case "host":
			if !row.header && row.suggestion == nil && row.host.Alias == name {
				m.cursor = i
				return
			}
		case "group":
			if row.header && !row.historyHeader && row.group == name {
				m.cursor = i
				return
			}
		case "suggestion":
			if row.suggestion != nil && row.suggestion.ID == name {
				m.cursor = i
				return
			}
		case "historyHeader":
			if row.historyHeader {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = prev
	m.clampCursor()
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
		revealGroup := name
		if !selectGroup {
			revealGroup = m.metadata.Host(name).Group
		}
		if revealGroup != "" {
			next, _, changed := m.collapsedGroupsRevealing(revealGroup)
			if changed {
				m.collapsedGroups = next
				m.collapseRevision++
				_ = m.persistCollapsedGroups()
			}
		}
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
	metadataByHost := m.hostMetadata()
	for _, host := range m.hosts {
		existing, err := normalizeGroupPath(metadataByHost[host.Alias].Group)
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
	if m.collapsedGroups[group] {
		delete(m.collapsedGroups, group)
	} else {
		m.collapsedGroups[group] = true
	}
	m.collapseRevision++
	for i, row := range m.hostRows() {
		if row.header && row.group == group {
			m.cursor = i
			break
		}
	}
	if err := m.persistCollapsedGroups(); err != nil {
		m.setError(err)
	}
	return nil
}

func (m *App) collapseAllGroups() tea.Cmd {
	if m.searchText() != "" {
		return m.setNotice("Clear the search filter to collapse groups")
	}
	m.collapsedGroups = map[string]bool{}
	for _, row := range m.hostRows() {
		if row.header {
			m.collapsedGroups[row.group] = true
		}
	}
	m.collapseRevision++
	m.clampCursor()
	if err := m.persistCollapsedGroups(); err != nil {
		m.setError(err)
		return nil
	}
	return nil
}

func (m *App) expandAllGroups() tea.Cmd {
	m.collapsedGroups = map[string]bool{}
	m.collapseRevision++
	if err := m.persistCollapsedGroups(); err != nil {
		m.setError(err)
		return nil
	}
	return nil
}

func (m *App) collapsedGroupsRevealing(group string) (map[string]bool, []string, bool) {
	next := make(map[string]bool, len(m.collapsedGroups))
	for path, collapsed := range m.collapsedGroups {
		next[path] = collapsed
	}
	parts := groupPathParts(group)
	path := ""
	changed := false
	for _, part := range parts {
		if path == "" {
			path = part
		} else {
			path += "/" + part
		}
		if next[path] {
			delete(next, path)
			changed = true
		}
	}
	collapsedPaths := make([]string, 0, len(next))
	for path, collapsed := range next {
		if collapsed {
			collapsedPaths = append(collapsedPaths, path)
		}
	}
	return next, collapsedPaths, changed
}

func (m *App) persistCollapsedGroups() error {
	live := map[string]bool{}
	metadataByHost := m.hostMetadata()
	for _, host := range m.hosts {
		group := strings.TrimSpace(metadataByHost[host.Alias].Group)
		if group == "" {
			continue
		}
		parts := groupPathParts(group)
		for i := 1; i <= len(parts); i++ {
			live[strings.Join(parts[:i], "/")] = true
		}
	}
	groups := make([]string, 0, len(m.collapsedGroups))
	cleaned := map[string]bool{}
	for path, collapsed := range m.collapsedGroups {
		if collapsed && live[path] {
			groups = append(groups, path)
			cleaned[path] = true
		}
	}
	m.collapsedGroups = cleaned
	return m.metadata.SetCollapsedGroups(groups)
}

func collapsedGroupsFromPrefs(groups []string) map[string]bool {
	out := make(map[string]bool, len(groups))
	for _, group := range groups {
		if group != "" {
			out[group] = true
		}
	}
	return out
}

func (m *App) collapseActionLabel() string {
	group, ok := m.selectedGroup()
	if !ok {
		return "collapse"
	}
	if m.collapsedGroups[group] {
		return "expand"
	}
	return "collapse"
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
	hostMetadata := m.hostMetadata()
	sort.SliceStable(m.hosts, func(i, j int) bool {
		a, b := hostMetadata[m.hosts[i].Alias], hostMetadata[m.hosts[j].Alias]
		switch order {
		case "alias":
			return strings.ToLower(hostLabel(m.hosts[i], a)) < strings.ToLower(hostLabel(m.hosts[j], b))
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
		return strings.ToLower(hostLabel(m.hosts[i], a)) < strings.ToLower(hostLabel(m.hosts[j], b))
	})
	m.hostGeneration++
}

func (m *App) hostLabel(host sshconfig.Host) string {
	return hostLabel(host, m.hostMetadata()[host.Alias])
}

func hostLabel(host sshconfig.Host, meta metadata.Host) string {
	if label := strings.TrimSpace(meta.Label); label != "" {
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

func (m *App) hostLooksStopped(host sshconfig.Host) bool {
	return hostLooksStopped(host, m.hostMetadata()[host.Alias])
}

func hostLooksStopped(host sshconfig.Host, meta metadata.Host) bool {
	if !host.Synced {
		return false
	}
	kind, ok := cloud.KindForSource(host.SyncSource)
	if !ok || !cloud.CapabilitiesFor(kind).Stop {
		return false
	}
	if kind == cloud.Box {
		return boxcloud.HostLooksStopped(host.Resolved.HostName, meta.Tags)
	}
	if kind == cloud.Upstash {
		return upstashcloud.HostLooksStopped(meta.Tags)
	}
	return false
}
func hostIdentity(h sshconfig.Host, passwordStored bool) string {
	if passwordOnly(h.Resolved) {
		if passwordStored {
			return "password · saved"
		}
		return "password"
	}
	return joinOr(h.Resolved.IdentityFiles, "agent/defaults")
}

func hostAuthSummary(h sshconfig.Host, passwordStored bool) string {
	if !h.Synced {
		return hostIdentity(h, passwordStored)
	}
	if passwordOnly(h.Resolved) {
		if passwordStored {
			return "password · saved"
		}
		return "password"
	}
	if len(h.Resolved.IdentityFiles) > 0 {
		return h.Resolved.IdentityFiles[0]
	}
	return "SSH access ensured on connect"
}

func hostStatusLine(h sshconfig.Host, meta metadata.Host) string {
	parts := make([]string, 0, 4)
	switch {
	case meta.Hidden:
		parts = append(parts, "◌ hidden")
	case meta.Favorite:
		parts = append(parts, "◆ favorite")
	}
	switch {
	case h.Synced && h.SyncSource == "gcp":
		parts = append(parts, "GCP synced")
	case h.Synced && h.SyncSource == "aws":
		parts = append(parts, "AWS synced")
	case h.Synced && h.SyncSource == "azure":
		parts = append(parts, "Azure synced")
	case h.Synced && h.SyncSource == "box":
		if hostLooksStopped(h, meta) {
			parts = append(parts, "Box stopped")
		} else {
			parts = append(parts, "Box synced")
		}
	case h.Synced && h.SyncSource == "upstash":
		if hostLooksStopped(h, meta) {
			parts = append(parts, "Upstash paused")
		} else {
			parts = append(parts, "Upstash synced")
		}
	case h.Synced:
		parts = append(parts, h.SyncSource+" synced")
	case h.Managed:
		parts = append(parts, "Bast managed")
	default:
		parts = append(parts, "external")
	}
	if h.KnownHost {
		parts = append(parts, "known host")
	} else {
		parts = append(parts, "not in known_hosts")
	}
	return strings.Join(parts, " · ")
}

func syncedAuthSummary(h sshconfig.Host) string {
	user := strings.TrimSpace(h.Resolved.User)
	key := ""
	if len(h.Resolved.IdentityFiles) > 0 {
		key = h.Resolved.IdentityFiles[0]
	}
	switch {
	case user == "" && key == "":
		return "SSH access ensured on connect"
	case user == "":
		return "user unknown · " + key
	case key == "":
		return user + " · agent/defaults"
	default:
		return user + " · " + key
	}
}
func usage(h metadata.Host) string {
	if h.LastUsedAt == nil {
		return "never"
	}
	return h.LastUsedAt.Local().Format("2006-01-02 15:04") + fmt.Sprintf(" (%d)", h.ConnectionCount)
}
func noneValue(s string) string {
	if s == "" || s == "none" {
		return "-"
	}
	return s
}
func emptyIfNone(s string) string {
	if s == "none" || s == "-" {
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
		return "-"
	}
	return platform.HomeRelative(path, home)
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
