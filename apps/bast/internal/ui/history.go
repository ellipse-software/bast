package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func (m *App) visibleHistorySuggestions() []metadata.HistorySuggestion {
	if len(m.historySuggestions) == 0 {
		return nil
	}
	query := strings.ToLower(m.searchText())
	aliases := make(map[string]bool, len(m.hosts))
	destinations := make(map[string]bool, len(m.hosts))
	for _, host := range m.hosts {
		aliases[strings.ToLower(host.Alias)] = true
		hostname := host.Resolved.HostName
		if hostname == "" {
			hostname = host.Alias
		}
		port := host.Resolved.Port
		if port == "" {
			port = "22"
		}
		destinations[historyDestination(hostname, host.Resolved.User, port)] = true
	}
	visible := make([]metadata.HistorySuggestion, 0, len(m.historySuggestions))
	for _, suggestion := range m.historySuggestions {
		port := suggestion.Port
		if port == "" {
			port = "22"
		}
		if aliases[strings.ToLower(suggestion.Target)] || destinations[historyDestination(suggestion.HostName, suggestion.User, port)] {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{
				suggestion.Alias, suggestion.Target, suggestion.HostName,
				suggestion.User, suggestion.Port, suggestion.Source,
			}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		visible = append(visible, suggestion)
	}
	return visible
}

func historyDestination(hostname, user, port string) string {
	return strings.ToLower(hostname) + "\x00" + strings.ToLower(user) + "\x00" + port
}

func (m *App) selectedHistorySuggestion() (metadata.HistorySuggestion, bool) {
	rows := m.hostListRows()
	if m.cursor < 0 || m.cursor >= len(rows) || rows[m.cursor].suggestion == nil {
		return metadata.HistorySuggestion{}, false
	}
	return *rows[m.cursor].suggestion, true
}

func (m *App) historySuggestionsHeaderSelected() bool {
	rows := m.hostListRows()
	return m.cursor >= 0 && m.cursor < len(rows) && rows[m.cursor].historyHeader
}

func (m *App) toggleHistorySuggestions() tea.Cmd {
	if !m.historySuggestionsHeaderSelected() {
		return nil
	}
	if m.searchText() != "" {
		return m.setNotice("Clear the search filter to collapse suggestions")
	}
	m.historySuggestionsCollapsed = !m.historySuggestionsCollapsed
	m.clampCursor()
	return nil
}

func (m *App) removeHistorySuggestion(id string) {
	for i, suggestion := range m.historySuggestions {
		if suggestion.ID == id {
			m.historySuggestions = append(m.historySuggestions[:i], m.historySuggestions[i+1:]...)
			break
		}
	}
	m.clampCursor()
}

func (m *App) dismissSelectedHistorySuggestion() tea.Cmd {
	suggestion, ok := m.selectedHistorySuggestion()
	if !ok {
		return nil
	}
	if err := m.metadata.DismissHistorySuggestion(suggestion.ID); err != nil {
		m.setError(err)
		return nil
	}
	m.removeHistorySuggestion(suggestion.ID)
	return m.setNotice("Suggestion dismissed")
}

func (m *App) importSelectedHistorySuggestion() (tea.Model, tea.Cmd) {
	suggestion, ok := m.selectedHistorySuggestion()
	if !ok || m.historyImporting != "" {
		return m, nil
	}
	alias := m.availableHistoryAlias(suggestion.Alias)
	input := historyHostInput(suggestion, alias)
	m.historyImporting = suggestion.ID
	return m, func() tea.Msg {
		_, err := m.addHistoryHost(input, metadata.Host{}, suggestion.ID)
		return historyImportDoneMsg{id: suggestion.ID, alias: alias, err: err}
	}
}

func (m *App) addHistoryHost(input sshconfig.HostInput, meta metadata.Host, suggestionID string) (sshconfig.Host, error) {
	added, err := m.config.Add(input)
	if err != nil {
		return sshconfig.Host{}, fmt.Errorf("add history host: %w", err)
	}
	if err := m.metadata.AcceptHistorySuggestion(input.Alias, meta, suggestionID); err != nil {
		if rollbackErr := m.config.Delete(added.ManagedID); rollbackErr != nil {
			return sshconfig.Host{}, fmt.Errorf("save history host: %w; rollback failed: %v", err, rollbackErr)
		}
		return sshconfig.Host{}, fmt.Errorf("save history host: %w", err)
	}
	return added, nil
}

func historyHostInput(suggestion metadata.HistorySuggestion, alias string) sshconfig.HostInput {
	input := sshconfig.HostInput{
		Alias: alias, HostName: suggestion.HostName, User: suggestion.User,
		Port: suggestion.Port, IdentityFile: suggestion.IdentityFile, ProxyJump: suggestion.ProxyJump,
	}
	if input.IdentityFile != "" {
		input.ExtraOptions = []string{"IdentitiesOnly yes"}
	}
	return input
}

func (m *App) availableHistoryAlias(preferred string) string {
	if _, exists := m.findHost(preferred); !exists {
		return preferred
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", preferred, suffix)
		if _, exists := m.findHost(candidate); !exists {
			return candidate
		}
	}
}

func (m *App) openHistoryHostForm() {
	suggestion, ok := m.selectedHistorySuggestion()
	if !ok {
		return
	}
	alias := m.availableHistoryAlias(suggestion.Alias)
	fields := hostFormFields(m, metadataHostValues{label: alias}, hostConnectionValues{
		includeConnection: true,
		hostname:          suggestion.HostName,
		user:              suggestion.User,
		port:              suggestion.Port,
		identity:          suggestion.IdentityFile,
		advanced:          sshconfig.AdvancedSettings{ProxyJump: suggestion.ProxyJump},
	}, []field{{label: "History suggestion", value: suggestion.ID, hidden: true}})
	m.openHostForm("Add host from history", "history_host_add", fields)
}
