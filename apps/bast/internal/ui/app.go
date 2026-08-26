package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"bast/internal/cloud/sync"
	"bast/internal/doctor"
	"bast/internal/history"
	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
	"bast/internal/vault"
)

type section int

const (
	hostsSection section = iota
	keysSection
	vaultSection
	syncSection
	filesSection
)

type field struct {
	label       string
	description string
	value       string
	placeholder string
	optional    bool
	hidden      bool
	secret      bool
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
	validationError  string
	input            textinput.Model
}

type loadedMsg struct {
	hosts            []sshconfig.Host
	keys             []keys.Key
	enrichmentErrors int
	err              error
}

type discoveredMsg struct {
	hosts []sshconfig.Host
	err   error
}

type syncDoneMsg struct {
	provider   string
	result     sync.Result
	err        error
	skipped    bool
	focusAlias string // when set, jump to Hosts with this alias selected
	notice     string
	opGen      uint64
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

type hostSaveHintTickMsg uint64

type reportResultMsg struct {
	err error
}

type updateAvailableMsg struct {
	version    string
	suggestion string
}

type historyLoadedMsg struct {
	suggestions []metadata.HistorySuggestion
	warnings    int
	err         error
}

type historyImportDoneMsg struct {
	id    string
	alias string
	err   error
}

type hostRowsCache struct {
	hostGeneration     uint64
	metadataRevision   uint64
	collapseGeneration uint64
	search             string
	showHidden         bool
	hostSignature      uint64
	boxEnabled         bool
	rows               []hostRow
}

type hostListRowsCache struct {
	hostGeneration     uint64
	metadataRevision   uint64
	collapseGeneration uint64
	search             string
	showHidden         bool
	boxEnabled         bool
	hostSignature      uint64
	historyCollapsed   bool
	suggestionSig      uint64
	rows               []hostRow
}

type styleCache struct {
	ready  bool
	dark   bool
	styles styleSet
}

type App struct {
	paths                       paths.Paths
	config                      sshconfig.Manager
	openSSH                     openssh.Client
	keyring                     keys.Manager
	metadata                    *metadata.Store
	syncer                      *sync.Engine
	hostMeta                    map[string]metadata.Host
	hostMetaRevision            uint64
	hostGeneration              uint64
	collapseRevision            uint64
	hostRowsCache               hostRowsCache
	hostListRowsCache           hostListRowsCache
	styleCache                  styleCache
	historySuggestions          []metadata.HistorySuggestion
	historyScanStarted          bool
	historyImporting            string
	historySuggestionsCollapsed bool

	section             section
	hosts               []sshconfig.Host
	keys                []keys.Key
	cursor              int
	search              string
	form                *form
	help                bool
	helpOffset          int
	credits             bool
	onboarding          bool
	onboardingTracked   bool
	onboardingReplay    bool
	onboardingPending   bool
	doctor              bool
	doctorOffset        int
	doctorLoading       bool
	doctorReport        doctor.Report
	showHidden          bool
	loading             bool
	enriching           bool
	autoSyncStarted     bool
	syncingProviders    map[string]bool
	syncOpGen           map[string]uint64
	status              string
	statusError         bool
	statusID            uint64
	hostSaveHintID      uint64
	hostSaveHintEnter   bool
	width               int
	height              int
	dark                bool
	nerdFont            bool
	version             string
	latestVersion       string
	updateSuggestion    string
	collapsedGroups     map[string]bool
	scrollbarDragging   bool
	syncStatus          sync.Status
	syncStatusAt        time.Time
	syncStatusProbing   bool
	syncProvider        string
	syncCursor          int
	vaultPassphrase     string
	vaultSession        *vault.Session
	vaultSessionChecked bool   // true after a session cache read or explicit set
	vaultAPIBase        string // remembered server URL (including unlinked preference)
	vaultStatus         string
	vaultLastSync       string
	vaultDirty          bool
	vaultBusy           string
	vaultBusyHoldLoad   bool
	vaultPushID         uint64
	vaultOpGen          uint64
	vaultRemoteRetry    bool // one-shot auto-recovery after ErrRemoteUpdated
	vaultConflict       *vaultConflictState
	syncBusy            string          // full-screen Sync overlay for long ops (e.g. box create)
	syncActivity        string          // footer label while a provider op runs (e.g. "resuming…")
	boxConnectAfter     string          // after box resume+reload, SSH into this alias
	syncInvCollapsed    map[string]bool // session-only status-group collapse on provider pages

	files filesState

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
			SyncGCPConfig: p.SyncGCPConfig, SyncAWSConfig: p.SyncAWSConfig, SyncAzureConfig: p.SyncAzureConfig, SyncBoxConfig: p.SyncBoxConfig, SyncUpstashConfig: p.SyncUpstashConfig, SyncVercelConfig: p.SyncVercelConfig,
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
		syncOpGen:        map[string]uint64{},
	}
	app.hostMeta, app.hostMetaRevision = store.HostsSnapshot()
	app.historySuggestions = store.HistoryImport().Pending
	app.onboardingPending = store.ShouldOnboard()
	if pass, err := vault.LoadPassphrase(vault.PassphrasePath(p.StateFile)); err == nil && pass != "" {
		app.vaultPassphrase = pass
	}
	app.refreshVaultSessionCache()
	return app, nil
}

func (m *App) providerSyncing(provider string) bool {
	return m.syncingProviders[provider]
}

func (m *App) anySyncing() bool {
	return len(m.syncingProviders) > 0
}

func (m *App) beginProviderOp(provider string) uint64 {
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	if m.syncOpGen == nil {
		m.syncOpGen = map[string]uint64{}
	}
	m.syncOpGen[provider]++
	m.syncingProviders[provider] = true
	return m.syncOpGen[provider]
}

func (m *App) providerOpGen(provider string) uint64 {
	if m.syncOpGen == nil {
		return 0
	}
	return m.syncOpGen[provider]
}

func (m *App) staleProviderOp(msg syncDoneMsg) bool {
	if msg.opGen == 0 {
		return false
	}
	return m.providerOpGen(msg.provider) != msg.opGen
}

func (m *App) syncCompletionNotice(provider string, count int) string {
	type providerCount struct {
		id      string
		name    string
		enabled bool
		count   int
	}
	gcp := m.metadata.GCP()
	aws := m.metadata.AWS()
	azure := m.metadata.Azure()
	box := m.metadata.Box()
	upstash := m.metadata.Upstash()
	vercel := m.metadata.Vercel()
	providers := []providerCount{
		{id: "gcp", name: "GCP", enabled: gcp.Enabled, count: gcp.LastInstanceCount},
		{id: "aws", name: "AWS", enabled: aws.Enabled, count: aws.LastInstanceCount},
		{id: "azure", name: "Azure", enabled: azure.Enabled, count: azure.LastInstanceCount},
		{id: "box", name: "Box", enabled: box.Enabled, count: box.LastInstanceCount},
		{id: "upstash", name: "Upstash", enabled: upstash.Enabled, count: upstash.LastInstanceCount},
		{id: "vercel", name: "Vercel", enabled: vercel.Enabled, count: vercel.LastInstanceCount},
	}
	parts := make([]string, 0, len(providers))
	for _, item := range providers {
		if !item.enabled {
			continue
		}
		if item.id == provider {
			item.count = count
		}
		parts = append(parts, fmt.Sprintf("%s %d", item.name, item.count))
	}
	if len(parts) > 1 {
		return strings.Join(parts, " · ")
	}
	return fmt.Sprintf("Synced %d %s instances", count, strings.ToUpper(provider))
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
	if m.autoSyncStarted {
		return nil
	}
	m.autoSyncStarted = true
	if m.syncingProviders == nil {
		m.syncingProviders = map[string]bool{}
	}
	var autoSyncCmds []tea.Cmd
	if m.metadata.GCP().Enabled && m.metadata.GCP().AutoSync && !m.syncingProviders["gcp"] {
		m.beginProviderOp("gcp")
		autoSyncCmds = append(autoSyncCmds, m.syncGCPCmd())
	}
	if m.metadata.AWS().Enabled && m.metadata.AWS().AutoSync && !m.syncingProviders["aws"] {
		m.beginProviderOp("aws")
		autoSyncCmds = append(autoSyncCmds, m.syncAWSCmd())
	}
	if m.metadata.Azure().Enabled && m.metadata.Azure().AutoSync && !m.syncingProviders["azure"] {
		m.beginProviderOp("azure")
		autoSyncCmds = append(autoSyncCmds, m.syncAzureCmd())
	}
	if box := m.metadata.Box(); !box.Disabled && !m.syncingProviders["box"] {
		if box.Enabled && box.AutoSync {
			m.beginProviderOp("box")
			autoSyncCmds = append(autoSyncCmds, m.syncBoxCmd())
		} else if !box.Enabled {
			m.beginProviderOp("box")
			autoSyncCmds = append(autoSyncCmds, m.autoConnectBoxCmd())
		}
	}
	if upstash := m.metadata.Upstash(); !upstash.Disabled && !m.syncingProviders["upstash"] {
		if upstash.Enabled && upstash.AutoSync {
			m.beginProviderOp("upstash")
			autoSyncCmds = append(autoSyncCmds, m.syncUpstashCmd())
		} else if !upstash.Enabled && m.upstashHasKey() {
			m.beginProviderOp("upstash")
			autoSyncCmds = append(autoSyncCmds, m.autoConnectUpstashCmd())
		}
	}
	if vercel := m.metadata.Vercel(); !vercel.Disabled && !m.syncingProviders["vercel"] {
		if vercel.Enabled && vercel.AutoSync {
			m.beginProviderOp("vercel")
			autoSyncCmds = append(autoSyncCmds, m.syncVercelCmd())
		} else if !vercel.Enabled && m.vercelReady() {
			m.beginProviderOp("vercel")
			autoSyncCmds = append(autoSyncCmds, m.autoConnectVercelCmd())
		}
	}
	if pull := m.vaultPullCmd(false); pull != nil {
		autoSyncCmds = append(autoSyncCmds, pull)
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

func (m *App) historyScanCmd(hosts []sshconfig.Host) tea.Cmd {
	if m.historyScanStarted {
		return nil
	}
	m.historyScanStarted = true
	hostSnapshot := append([]sshconfig.Host(nil), hosts...)
	return func() tea.Msg {
		var warningCount int
		for range 3 {
			previous, revision := m.metadata.HistoryImportSnapshot()
			next, warnings := history.Scan(m.paths.Home, os.Getenv, previous, hostSnapshot)
			warningCount = len(warnings)
			committed, ok, err := m.metadata.CommitHistoryImport(revision, next)
			if err != nil {
				return historyLoadedMsg{suggestions: previous.Pending, warnings: warningCount, err: err}
			}
			if ok {
				return historyLoadedMsg{suggestions: committed.Pending, warnings: warningCount}
			}
		}
		return historyLoadedMsg{suggestions: m.metadata.HistoryImport().Pending, warnings: warningCount}
	}
}

func (m *App) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.help {
			m.clampHelpOffset()
		}
		if m.doctor {
			m.clampDoctorOffset()
		}
		return m, nil
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		return m, nil
	case loadedMsg:
		m.loading = false
		m.enriching = false
		if m.vaultBusyHoldLoad {
			m.vaultBusyHoldLoad = false
			m.clearVaultBusy()
		}
		if msg.hosts != nil {
			m.hosts = msg.hosts
			m.sortHosts()
		}
		if msg.err != nil {
			m.boxConnectAfter = ""
			m.setError(msg.err)
			return m, m.autoSyncCmds()
		}
		m.keys = msg.keys
		m.selectAfterLoad()
		if cmd := m.connectAfterBoxResume(); cmd != nil {
			cmds := []tea.Cmd{cmd}
			if msg.enrichmentErrors > 0 {
				cmds = append(cmds, m.setNotice(fmt.Sprintf("%d host details could not be resolved", msg.enrichmentErrors)))
			}
			return m, tea.Batch(cmds...)
		}
		if msg.enrichmentErrors > 0 {
			return m, tea.Batch(m.autoSyncCmds(), m.setNotice(fmt.Sprintf("%d host details could not be resolved", msg.enrichmentErrors)))
		}
		return m, m.autoSyncCmds()
	case doctorDoneMsg:
		if !m.doctor {
			return m, nil
		}
		m.doctorLoading = false
		m.doctorReport = msg.report
		m.clampDoctorOffset()
		return m, nil
	case discoveredMsg:
		if msg.err != nil {
			m.loading = false
			m.enriching = false
			m.onboardingPending = false
			m.setError(msg.err)
			cmds := []tea.Cmd{tea.RequestBackgroundColor}
			if cmd := m.postPaintCmds(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			if cmd := m.historyScanCmd(m.hosts); cmd != nil {
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
		m.decideOnboarding()
		cmds := []tea.Cmd{m.enrichCmd(m.hosts), tea.RequestBackgroundColor}
		if cmd := m.postPaintCmds(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.historyScanCmd(m.hosts); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case historyLoadedMsg:
		if msg.err == nil {
			m.historySuggestions = msg.suggestions
			m.clampCursor()
		}
		if msg.err != nil {
			return m, m.setNotice("Couldn't save shell history scan")
		}
		if msg.warnings > 0 {
			return m, m.setNotice("Some shell history couldn't be read")
		}
		return m, nil
	case historyImportDoneMsg:
		if m.historyImporting == msg.id {
			m.historyImporting = ""
		}
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.removeHistorySuggestion(msg.id)
		m.selectAfterLoadSection, m.selectAfterLoadName, m.selectAfterLoadGroup = hostsSection, msg.alias, false
		m.loading = true
		m.enriching = false
		return m, tea.Batch(m.loadCmd(), m.setNotice("Host added"))
	case reportResultMsg:
		if msg.err != nil {
			return m, m.setNotice("Couldn't send report")
		}
		return m, m.setNotice("Report sent")
	case syncDoneMsg:
		if m.staleProviderOp(msg) {
			return m, nil
		}
		delete(m.syncingProviders, msg.provider)
		m.clearSyncBusy()
		m.syncActivity = ""
		m.clampSyncCursor(m.syncMenuItems())
		if msg.skipped {
			m.boxConnectAfter = ""
			return m, m.syncStatusCmd()
		}
		if msg.err != nil {
			m.boxConnectAfter = ""
			telemetry.Track("sync_"+msg.provider+"_fail", m.version)
			m.setError(msg.err)
			return m, m.syncStatusCmd()
		}
		label := strings.ToUpper(msg.provider)
		notice := m.syncCompletionNotice(msg.provider, msg.result.Count)
		if msg.provider == "vercel" {
			if n := len(m.metadata.Vercel().Unrestorable); n > 0 && msg.notice == "" {
				notice += fmt.Sprintf(" · %d unrestorable", n)
			}
		}
		connectAfter := m.boxConnectAfter
		if msg.result.Error == "disabled" {
			m.boxConnectAfter = ""
			notice = label + " sync disconnected"
			telemetry.Track("sync_"+msg.provider+"_disable", m.version)
		} else if msg.notice != "" {
			notice = msg.notice
			telemetry.Track("sync_"+msg.provider, m.version)
		} else if msg.focusAlias != "" {
			notice = "Created " + msg.focusAlias
			m.selectAfterLoadSection = hostsSection
			m.selectAfterLoadName = msg.focusAlias
			m.selectAfterLoadGroup = false
			m.section = hostsSection
			m.syncProvider = ""
			telemetry.Track("sync_"+msg.provider, m.version)
		} else if connectAfter != "" {
			m.selectAfterLoadSection = hostsSection
			m.selectAfterLoadName = connectAfter
			m.selectAfterLoadGroup = false
			m.section = hostsSection
			telemetry.Track("sync_"+msg.provider, m.version)
			m.loading = true
			m.enriching = false
			return m, tea.Batch(m.loadCmd(), m.syncStatusCmd())
		} else if strings.HasPrefix(msg.result.Error, "created ") {
			// Legacy create notice path; prefer focusAlias for navigation.
			notice = strings.TrimPrefix(msg.result.Error, "created ")
			if notice != "" {
				notice = "Created " + notice
			} else {
				notice = "Box created"
			}
			telemetry.Track("sync_"+msg.provider, m.version)
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
		m.syncStatusProbing = false
		if msg.err == nil {
			m.syncStatus = msg.status
			m.syncStatusAt = time.Now()
		}
		return m, nil
	case vaultOTPStartedMsg:
		m.clearVaultBusy()
		if msg.err != nil {
			telemetry.Track("vault_link_fail", m.version)
			m.setError(msg.err)
			return m, nil
		}
		m.openVaultCodeForm(msg.email, msg.apiBase)
		return m, m.setNotice("Code sent to " + msg.email)
	case vaultOTPVerifiedMsg:
		m.clearVaultBusy()
		if msg.err != nil {
			telemetry.Track("vault_link_fail", m.version)
			m.setError(msg.err)
			return m, nil
		}
		m.openVaultPassphraseForm(msg.email, msg.token, msg.userID, msg.deviceID, msg.apiBase, true)
		return m, nil
	case vaultUnlockedMsg:
		m.clearVaultBusy()
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.rememberVaultPassphrase(msg.passphrase)
		telemetry.Track("vault_unlock", m.version)
		if msg.next == "vault_sync" {
			return m.runVaultAction(msg.next)
		}
		return m, m.setNotice("Vault unlocked")
	case vaultPullMsg:
		if msg.opGen != 0 && msg.opGen != m.vaultOpGen {
			return m, nil
		}
		if msg.session != nil {
			m.setVaultSession(msg.session)
		}
		if msg.passphrase != "" {
			m.rememberVaultPassphrase(msg.passphrase)
		}
		if msg.err != nil {
			m.vaultBusyHoldLoad = false
			m.clearVaultBusy()
			if msg.badPassphrase {
				m.forgetVaultPassphrase()
			}
			m.vaultStatus = msg.err.Error()
			if msg.linked {
				telemetry.Track("vault_link_fail", m.version)
			} else if msg.thenPush {
				telemetry.Track("vault_sync_fail", m.version)
			}
			if !msg.interactive {
				return m, nil
			}
			m.setError(msg.err)
			return m, nil
		}
		if msg.revision != "" {
			m.vaultLastSync = time.Now().Local().Format("15:04:05")
			if m.vaultSession != nil {
				m.vaultSession.Revision = msg.revision
			}
		}
		m.vaultStatus = ""
		if msg.linked {
			telemetry.Track("vault_link", m.version)
		}
		if msg.thenPush {
			m.vaultBusyHoldLoad = false
			m.keepVaultBusy("Syncing vault…")
			m.vaultStatus = "syncing…"
			var cmds []tea.Cmd
			if msg.changed {
				cmds = append(cmds, m.loadCmd())
			}
			cmds = append(cmds, m.vaultPushCmdOpts(true))
			return m, tea.Batch(cmds...)
		}
		if msg.changed {
			// Keep the Sync/Vault body blocked through host reload so linking
			// does not flash the vault menu between network work and apply.
			if m.vaultBusy == "" {
				m.vaultBusy = "Updating…"
			}
			m.vaultBusyHoldLoad = true
			m.loading = true
			notice := msg.notice
			if notice == "" {
				notice = "Vault pulled"
			}
			return m, tea.Batch(m.loadCmd(), m.setNotice(notice))
		}
		m.vaultBusyHoldLoad = false
		m.clearVaultBusy()
		if msg.notice != "" {
			return m, m.setNotice(msg.notice)
		}
		return m, nil
	case vaultConflictMsg:
		if msg.opGen != 0 && msg.opGen != m.vaultOpGen {
			return m, nil
		}
		m.clearVaultBusy()
		if msg.session != nil {
			m.setVaultSession(msg.session)
		}
		if msg.forLink && msg.passphrase != "" {
			m.rememberVaultPassphrase(msg.passphrase)
		}
		m.vaultStatus = fmt.Sprintf("%d sync conflicts · choose keep local or keep remote", msg.count)
		m.openVaultConflictForm(msg)
		return m, nil
	case vaultPushMsg:
		if msg.opGen != 0 && msg.opGen != m.vaultOpGen {
			return m, nil
		}
		m.clearVaultBusy()
		if msg.session != nil {
			m.setVaultSession(msg.session)
		}
		if msg.err != nil {
			if msg.badPassphrase {
				m.forgetVaultPassphrase()
			}
			if errors.Is(msg.err, vault.ErrRemoteUpdated) {
				m.vaultStatus = "Remote vault changed"
				if m.vaultRemoteRetry {
					// Already attempted pull+push once; stop so Sync cannot hang forever.
					m.vaultRemoteRetry = false
					if msg.synced {
						telemetry.Track("vault_sync_fail", m.version)
					}
					return m, m.setNotice("Remote vault changed · Sync now again")
				}
				m.beginVaultBusy("Syncing vault…")
				m.vaultRemoteRetry = true
				return m, tea.Batch(
					m.setNotice("Remote vault changed · syncing"),
					m.vaultPullCmdOpts(true, true),
				)
			}
			m.vaultRemoteRetry = false
			m.vaultStatus = msg.err.Error()
			if msg.synced {
				telemetry.Track("vault_sync_fail", m.version)
				return m, m.setNotice("Vault sync failed")
			}
			return m, m.setNotice("Vault push failed")
		}
		m.vaultRemoteRetry = false
		if msg.passphrase != "" && (msg.resetPassphrase || msg.rotatePassphrase) {
			m.rememberVaultPassphrase(msg.passphrase)
		}
		m.vaultDirty = false
		m.vaultLastSync = time.Now().Local().Format("15:04:05")
		m.vaultStatus = ""
		notice := "Vault pushed"
		if msg.resetPassphrase {
			notice = "Vault passphrase reset · remote replaced"
			telemetry.Track("vault_passphrase_reset", m.version)
		} else if msg.rotatePassphrase {
			notice = "Vault passphrase rotated"
			telemetry.Track("vault_passphrase_rotate", m.version)
		} else if msg.synced {
			notice = "Vault synced"
			telemetry.Track("vault_sync", m.version)
		}
		if msg.applied {
			m.loading = true
			return m, tea.Batch(m.loadCmd(), m.setNotice(notice))
		}
		return m, m.setNotice(notice)
	case vaultPushDebounceMsg:
		if msg.id != m.vaultPushID || !m.vaultDirty {
			return m, nil
		}
		return m, m.vaultPushCmd()
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
	case filesListMsg:
		return m, m.handleFilesListMsg(msg)
	case filesConnectMsg:
		return m, m.handleFilesConnectMsg(msg)
	case filesTransferDoneMsg:
		return m, m.handleFilesTransferDone(msg)
	case filesTransferProgressMsg:
		return m, m.handleFilesTransferProgress(msg)
	case filesOpDoneMsg:
		return m, m.handleFilesOpDone(msg)
	case filesPreviewMsg:
		return m, m.handleFilesPreviewMsg(msg)
	case clearStatusMsg:
		if uint64(msg) == m.statusID && !m.statusError {
			m.status = ""
		}
		return m, nil
	case hostSaveHintTickMsg:
		if uint64(msg) != m.hostSaveHintID || m.form == nil || !isHostForm(m.form) {
			return m, nil
		}
		m.hostSaveHintEnter = !m.hostSaveHintEnter
		return m, m.hostSaveHintTickCmd()
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
		if m.section == filesSection {
			m.initFilesState()
			pane := m.filesFocusedPane()
			if pane.pathEdit {
				return m.updateFilesPathInputMsg(pane, msg)
			}
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
		if msg.String() == "esc" && m.vaultBusy != "" && m.form == nil {
			m.cancelVaultOp()
			return m, m.setNotice("Vault cancelled")
		}
		if m.form != nil {
			return m.updateForm(msg)
		}
		if m.onboarding {
			return m.updateOnboarding(msg.String())
		}
		model, cmd := m.updateKeys(msg)
		if m.form != nil && isHostForm(m.form) {
			cmd = tea.Batch(cmd, m.hostSaveHintTickCmd())
		}
		return model, cmd
	}
	return m, nil
}

func (m *App) View() tea.View {
	content := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	if m.form != nil && !isHostForm(m.form) {
		view.MouseMode = tea.MouseModeNone
	}
	view.WindowTitle = "Bast: SSH picker"
	return view
}

var _ tea.Model = (*App)(nil)
