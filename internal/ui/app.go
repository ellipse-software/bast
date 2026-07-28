package ui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud/sync"
	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
)

type section int

const (
	hostsSection section = iota
	keysSection
	syncSection
)

type field struct {
	label       string
	description string
	value       string
	placeholder string
	optional    bool
	hidden      bool
	section     string
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
	screen           string
	hubIndex         int
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

type discoveredMsg struct {
	hosts []sshconfig.Host
	err   error
}

type syncDoneMsg struct {
	provider string
	result   sync.Result
	err      error
}

type syncStatusMsg struct {
	status sync.Status
	err    error
}

type processDoneMsg struct {
	name       string
	err        error
	sshSession bool
}

type clearStatusMsg uint64

type reportResultMsg struct {
	err error
}

type updateAvailableMsg struct {
	version    string
	suggestion string
}

type hostRowsCache struct {
	hostGeneration     uint64
	metadataRevision   uint64
	collapseGeneration uint64
	search             string
	showHidden         bool
	hostSignature      uint64
	rows               []hostRow
}

type App struct {
	paths            paths.Paths
	config           sshconfig.Manager
	openSSH          openssh.Client
	keyring          keys.Manager
	metadata         *metadata.Store
	syncer           *sync.Engine
	hostMeta         map[string]metadata.Host
	hostMetaRevision uint64
	hostGeneration   uint64
	collapseRevision uint64
	hostRowsCache    hostRowsCache

	section           section
	hosts             []sshconfig.Host
	keys              []keys.Key
	cursor            int
	search            string
	form              *form
	help              bool
	helpOffset        int
	credits           bool
	showHidden        bool
	loading           bool
	enriching         bool
	syncingProviders  map[string]bool
	status            string
	statusError       bool
	statusID          uint64
	width             int
	height            int
	dark              bool
	nerdFont          bool
	version           string
	latestVersion     string
	updateSuggestion  string
	collapsedGroups   map[string]bool
	scrollbarDragging bool
	syncStatus        sync.Status
	syncProvider      string
	syncCursor        int

	selectAfterLoadSection section
	selectAfterLoadName    string
	selectAfterLoadGroup   bool
}

func New(p paths.Paths, client openssh.Client, version string) (*App, error) {
	store, err := metadata.Open(p.StateFile)
	if err != nil {
		return nil, err
	}
	app := &App{
		paths: p,
		config: sshconfig.Manager{
			Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir,
			ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys,
			SyncGCPConfig: p.SyncGCPConfig, SyncAWSConfig: p.SyncAWSConfig, SyncAzureConfig: p.SyncAzureConfig,
		},
		openSSH:          client,
		keyring:          keys.Manager{Paths: p, SSHKeygen: client.SSHKeygen, SSHAdd: client.SSHAdd},
		metadata:         store,
		syncer:           sync.New(p, store),
		loading:          true,
		dark:             true,
		nerdFont:         detectNerdFont(os.Getenv),
		version:          version,
		collapsedGroups:  collapsedGroupsFromPrefs(store.Preferences().CollapsedGroups),
		syncingProviders: map[string]bool{},
	}
	app.hostMeta, app.hostMetaRevision = store.HostsSnapshot()
	return app, nil
}

func (m *App) providerSyncing(provider string) bool {
	return m.syncingProviders[provider]
}

func (m *App) anySyncing() bool {
	return len(m.syncingProviders) > 0
}

func (m *App) Init() tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	// First frame: discover hosts only. Theme probe, update check, and AutoSync
	// start after hosts are painted so startup never waits on the network or CLIs.
	return m.loadCmd()
}

func (m *App) autoSyncCmds() tea.Cmd {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	var autoSyncCmds []tea.Cmd
	if m.metadata.GCP().Enabled && m.metadata.GCP().AutoSync && !m.syncingProviders["gcp"] {
		m.syncingProviders["gcp"] = true
		autoSyncCmds = append(autoSyncCmds, m.syncGCPCmd())
	}
	if m.metadata.AWS().Enabled && m.metadata.AWS().AutoSync && !m.syncingProviders["aws"] {
		m.syncingProviders["aws"] = true
		autoSyncCmds = append(autoSyncCmds, m.syncAWSCmd())
	}
	if m.metadata.Azure().Enabled && m.metadata.Azure().AutoSync && !m.syncingProviders["azure"] {
		m.syncingProviders["azure"] = true
		autoSyncCmds = append(autoSyncCmds, m.syncAzureCmd())
	}
	if len(autoSyncCmds) == 0 {
		return nil
	}
	return tea.Sequence(autoSyncCmds...)
}

func (m *App) postPaintCmds() tea.Cmd {
	if updateCmd := m.checkForUpdateCmd(); updateCmd != nil {
		return updateCmd
	}
	return nil
}

func (m *App) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.help {
			m.clampHelpOffset()
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		return m, nil
	case loadedMsg:
		m.loading = false
		m.enriching = false
		if msg.hosts != nil {
			m.hosts = msg.hosts
			m.sortHosts()
		}
		if msg.err != nil {
			m.setError(msg.err)
			return m, m.autoSyncCmds()
		}
		m.keys = msg.keys
		m.selectAfterLoad()
		return m, m.autoSyncCmds()
	case discoveredMsg:
		if msg.err != nil {
			m.loading = false
			m.enriching = false
			m.setError(msg.err)
			cmds := []tea.Cmd{tea.RequestBackgroundColor}
			if cmd := m.postPaintCmds(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		previous := make(map[string]sshconfig.Host, len(m.hosts))
		for _, host := range m.hosts {
			previous[host.Alias] = host
		}
		for i := range msg.hosts {
			if host, ok := previous[msg.hosts[i].Alias]; ok {
				if msg.hosts[i].Resolved.HostName == "" {
					msg.hosts[i].Resolved = host.Resolved
				}
				if host.KnownHost {
					msg.hosts[i].KnownHost = true
				}
			}
		}
		m.hosts = msg.hosts
		m.sortHosts()
		// Hosts are usable from config parse; clear loading now and enrich quietly.
		m.loading = false
		m.enriching = true
		cmds := []tea.Cmd{m.enrichCmd(m.hosts), tea.RequestBackgroundColor}
		if cmd := m.postPaintCmds(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case reportResultMsg:
		if msg.err != nil {
			return m, m.setNotice("Couldn't send report")
		}
		return m, m.setNotice("Report sent")
	case syncDoneMsg:
		delete(m.syncingProviders, msg.provider)
		if msg.err != nil {
			telemetry.Track("sync_"+msg.provider+"_fail", m.version)
			m.setError(msg.err)
			return m, m.syncStatusCmd()
		}
		label := strings.ToUpper(msg.provider)
		notice := fmt.Sprintf("Synced %d %s instances", msg.result.Count, label)
		if msg.result.Error == "disabled" {
			notice = label + " sync disconnected"
			telemetry.Track("sync_"+msg.provider+"_disable", m.version)
		} else {
			if msg.result.Error != "" {
				notice += " (with warnings)"
			}
			telemetry.Track("sync_"+msg.provider, m.version)
		}
		m.loading = true
		m.enriching = false
		return m, tea.Batch(m.loadCmd(), m.syncStatusCmd(), m.setNotice(notice))
	case syncStatusMsg:
		if msg.err == nil {
			m.syncStatus = msg.status
		}
		return m, nil
	case processDoneMsg:
		m.loading = true
		m.enriching = false
		if msg.sshSession {
			m.statusID++
			m.status, m.statusError = "", false
			return m, m.loadCmd()
		}
		if msg.err != nil {
			m.statusID++
			m.status, m.statusError = msg.name+": "+openssh.FormatError(msg.err), true
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
	case tea.MouseMotionMsg:
		return m.updateMouseMotion(msg)
	case tea.MouseReleaseMsg:
		m.scrollbarDragging = false
		return m, nil
	case tea.MouseWheelMsg:
		return m.updateMouseWheel(msg)
	case tea.PasteMsg:
		if m.form != nil {
			return m.updateFormPaste(msg)
		}
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.statusError && m.status != "" {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "space":
				if telemetry.Enabled() {
					report := telemetry.Report{
						Message: m.status,
						Version: m.version,
						Context: "tui",
					}
					m.statusID++
					m.status, m.statusError = "Sending report…", false
					return m, func() tea.Msg {
						return reportResultMsg{err: telemetry.ReportError(report)}
					}
				}
				m.statusID++
				m.status, m.statusError = "", false
				return m, nil
			case "enter", "esc", "backspace", "ctrl+h":
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
