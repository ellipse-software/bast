package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
)

type section int

const (
	hostsSection section = iota
	keysSection
)

type field struct {
	label       string
	description string
	value       string
	placeholder string
	optional    bool
	hidden      bool
	options     []fieldOption
	selected    int
	customValue string
}

type fieldOption struct {
	label  string
	value  string
	custom bool
}

type form struct {
	title            string
	action           string
	fields           []field
	index            int
	revealed         int
	pastedPrivateKey string
	pastedPublicKey  string
	selecting        bool
	input            textinput.Model
}

type loadedMsg struct {
	hosts []sshconfig.Host
	keys  []keys.Key
	err   error
}

type processDoneMsg struct {
	name     string
	err      error
	exitBast bool
}

type clearStatusMsg uint64

type updateAvailableMsg struct {
	version    string
	suggestion string
}

type App struct {
	paths    paths.Paths
	config   sshconfig.Manager
	openSSH  openssh.Client
	keyring  keys.Manager
	metadata *metadata.Store

	section          section
	hosts            []sshconfig.Host
	keys             []keys.Key
	cursor           int
	search           string
	form             *form
	help             bool
	credits          bool
	showHidden       bool
	loading          bool
	status           string
	statusError      bool
	statusID         uint64
	width            int
	height           int
	dark             bool
	version          string
	latestVersion    string
	updateSuggestion string
	collapsedGroups  map[string]bool

	selectAfterLoadSection section
	selectAfterLoadName    string
	selectAfterLoadGroup   bool
}

func New(p paths.Paths, client openssh.Client, version string) (*App, error) {
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		return nil, err
	}
	return &App{
		paths: p,
		config: sshconfig.Manager{
			Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir,
			ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys,
		},
		openSSH:         client,
		keyring:         keys.Manager{Paths: p, SSHKeygen: client.SSHKeygen, SSHAdd: client.SSHAdd},
		metadata:        store,
		loading:         true,
		dark:            true,
		version:         version,
		collapsedGroups: map[string]bool{},
	}, nil
}

func (m *App) Init() tea.Cmd {
	if updateCmd := m.checkForUpdateCmd(); updateCmd != nil {
		return tea.Batch(m.loadCmd(), tea.RequestBackgroundColor, updateCmd)
	}
	return tea.Batch(m.loadCmd(), tea.RequestBackgroundColor)
}

func (m *App) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		return m, nil
	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.hosts, m.keys = msg.hosts, msg.keys
		m.sortHosts()
		m.selectAfterLoad()
		return m, nil
	case processDoneMsg:
		if msg.err == nil && msg.exitBast {
			return m, tea.Quit
		}
		m.loading = true
		if msg.err != nil {
			m.statusID++
			m.status, m.statusError = msg.name+": "+msg.err.Error(), true
			return m, m.loadCmd()
		}
		return m, tea.Batch(m.loadCmd(), m.setNotice(msg.name+" completed"))
	case clearStatusMsg:
		if uint64(msg) == m.statusID && !m.statusError {
			m.status = ""
		}
		return m, nil
	case updateAvailableMsg:
		m.latestVersion, m.updateSuggestion = msg.version, msg.suggestion
		return m, nil
	case tea.MouseClickMsg:
		return m.updateMouse(msg)
	case tea.PasteMsg:
		if m.form != nil {
			return m.updateFormPaste(msg)
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.statusError && m.status != "" {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "enter", "esc":
				m.statusID++
				m.status, m.statusError = "", false
			}
			return m, nil
		}
		if m.form != nil {
			return m.updateForm(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m *App) View() tea.View {
	content := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "Bast — SSH picker"
	return view
}

var _ tea.Model = (*App)(nil)
