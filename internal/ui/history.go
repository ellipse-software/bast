package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func (m *App) visibleHistorySuggestions() []metadata.HistorySuggestion {
	query := strings.ToLower(m.searchText())
	visible := make([]metadata.HistorySuggestion, 0, len(m.historySuggestions))
	for _, suggestion := range m.historySuggestions {
		if m.historySuggestionExists(suggestion) {
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

func (m *App) historySuggestionExists(suggestion metadata.HistorySuggestion) bool {
	port := suggestion.Port
	if port == "" {
		port = "22"
	}
	for _, host := range m.hosts {
		if strings.EqualFold(host.Alias, suggestion.Target) {
			return true
		}
		hostname := host.Resolved.HostName
		if hostname == "" {
			hostname = host.Alias
		}
		hostPort := host.Resolved.Port
		if hostPort == "" {
			hostPort = "22"
		}
		if strings.EqualFold(hostname, suggestion.HostName) &&
			strings.EqualFold(host.Resolved.User, suggestion.User) && hostPort == port {
			return true
		}
	}
	return false
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
		added, err := m.config.Add(input)
		if err != nil {
			return historyImportDoneMsg{id: suggestion.ID, alias: alias, err: fmt.Errorf("import history host: %w", err)}
		}
		if err := m.metadata.AcceptHistorySuggestion(alias, metadata.Host{}, suggestion.ID); err != nil {
			rollbackErr := m.config.Delete(added.ManagedID)
			if rollbackErr != nil {
				err = fmt.Errorf("save imported host: %w; rollback failed: %v", err, rollbackErr)
			} else {
				err = fmt.Errorf("save imported host: %w", err)
			}
			return historyImportDoneMsg{id: suggestion.ID, alias: alias, err: err}
		}
		return historyImportDoneMsg{id: suggestion.ID, alias: alias}
	}
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
