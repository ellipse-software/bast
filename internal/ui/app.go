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
	value       string
	placeholder string
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
	name string
	err  error
}

type clearStatusMsg uint64

type App struct {
	paths    paths.Paths
	config   sshconfig.Manager
	openSSH  openssh.Client
	keyring  keys.Manager
	metadata *metadata.Store

	section     section
	hosts       []sshconfig.Host
	keys        []keys.Key
	cursor      int
	search      string
	form        *form
	help        bool
	showHidden  bool
	loading     bool
	status      string
	statusError bool
	statusID    uint64
	width       int
	height      int
	dark        bool
}

func New(p paths.Paths, client openssh.Client) (*App, error) {
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
		openSSH:  client,
		keyring:  keys.Manager{Paths: p, SSHKeygen: client.SSHKeygen, SSHAdd: client.SSHAdd},
		metadata: store,
		loading:  true,
		dark:     true,
	}, nil
}

func (m *App) Init() tea.Cmd {
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
		m.clampCursor()
		return m, nil
	case processDoneMsg:
		m.loading = true
		if msg.err != nil {
			m.status, m.statusError = msg.name+": "+msg.err.Error(), true
		} else {
			m.status, m.statusError = msg.name+" completed", false
		}
		return m, m.loadCmd()
	case clearStatusMsg:
		if uint64(msg) == m.statusID && m.status == "Hidden hosts concealed" {
			m.status = ""
		}
		return m, nil
	case tea.MouseClickMsg:
		return m.updateMouse(msg)
	case tea.PasteMsg:
		if m.form != nil {
			return m.updateFormPaste(msg)
		}
		return m, nil
	case tea.KeyPressMsg:
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
