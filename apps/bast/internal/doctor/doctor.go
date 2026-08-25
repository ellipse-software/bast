package doctor

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/platform"
	"bast/internal/sshconfig"
)

type Options struct {
	Fix        bool
	Probe      bool
	Categories []string
	HTTP       *http.Client
}

type Engine struct {
	Paths   paths.Paths
	OpenSSH openssh.Client
	Config  sshconfig.Manager
	Keyring keys.Manager
	Version string
}

type runState struct {
	ins      sshconfig.Inspection
	hosts    []effectiveHost
	store    *metadata.Store
	storeErr error
	keys     []keys.Key
	agent    agentState
}

type agentState struct {
	checked bool
	present bool
	empty   bool
	count   int
	err     string
}

type effectiveHost struct {
	Alias               string
	Source              string
	Line                int
	HostName            string
	User                string
	Port                string
	IdentityFiles       []string
	RawIdentityFiles    []string
	IdentitiesOnly      string
	ProxyJump           string
	ForwardAgent        string
	CertificateFile     string
	ServerAliveInterval string
	ControlPath         string
	ControlMaster       string
	UseKeychain         string
	AddKeysToAgent      string
	Managed             bool
	Synced              bool
	Literal             sshconfig.InspectHost
}

func New(p paths.Paths, client openssh.Client, version string) Engine {
	return Engine{
		Paths:   p,
		OpenSSH: client,
		Config: sshconfig.Manager{
			Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir,
			ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys,
			SyncGCPConfig: p.SyncGCPConfig, SyncAWSConfig: p.SyncAWSConfig,
			SyncAzureConfig: p.SyncAzureConfig, SyncBoxConfig: p.SyncBoxConfig,
			SyncUpstashConfig: p.SyncUpstashConfig,
		},
		Keyring: keys.Manager{Paths: p, SSHKeygen: client.SSHKeygen, SSHAdd: client.SSHAdd},
		Version: version,
	}
}

func (e Engine) Run(ctx context.Context, opt Options) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	st := e.collect(ctx)
	r := e.diagnose(ctx, opt, st)
	if opt.Fix {
		fixed := e.applyFixes(r)
		st = e.collect(ctx)
		r = e.diagnose(ctx, Options{Probe: opt.Probe, Categories: opt.Categories, HTTP: opt.HTTP}, st)
		r.Fixed = fixed
		r.finalize()
	}
	return r
}

func (e Engine) collect(ctx context.Context) runState {
	st := runState{ins: e.Config.Inspect()}
	st.hosts = e.literalHosts(st.ins)
	st.store, st.storeErr = metadata.Open(e.Paths.StateFile)
	referenced := map[string][]string{}
	for _, h := range st.hosts {
		for _, id := range h.IdentityFiles {
			referenced[id] = append(referenced[id], h.Alias)
		}
	}
	keyCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if list, err := e.Keyring.Discover(keyCtx, referenced); err == nil {
		st.keys = list
	}
	st.agent = e.inspectAgent(ctx)
	return st
}

func (e Engine) diagnose(ctx context.Context, opt Options, st runState) Report {
	var r Report
	e.checkEnv(ctx, &r)
	e.checkPerm(&r, st)
	e.checkConfig(&r, st)
	e.checkKeys(ctx, &r, st)
	e.checkAgent(&r, st)
	e.checkKnownHosts(ctx, &r, st)
	e.checkMetadata(&r, st)
	e.checkVault(&r)
	e.checkSync(&r, st)
	e.checkSuggest(ctx, &r, st, opt)
	if opt.Probe {
		e.checkProbe(ctx, &r, st)
	}
	r.filter(opt.Categories)
	r.finalize()
	return r
}

func (e Engine) display(path string) string {
	if path == "" {
		return path
	}
	return platform.HomeRelative(path, e.Paths.Home)
}

func (e Engine) literalHosts(ins sshconfig.Inspection) []effectiveHost {
	seen := map[string]bool{}
	var out []effectiveHost
	for _, h := range ins.Hosts {
		if h.Wildcard || !selectable(h.Alias) {
			continue
		}
		if seen[h.Alias] {
			continue
		}
		seen[h.Alias] = true
		out = append(out, e.effective(ins, h))
	}
	return out
}

func selectable(alias string) bool {
	return alias != "" && !strings.HasPrefix(alias, "!") && !strings.ContainsAny(alias, `*?%#\"' `+"\t\r\n\x00")
}

func (e Engine) effective(ins sshconfig.Inspection, first sshconfig.InspectHost) effectiveHost {
	out := effectiveHost{
		Alias:   first.Alias,
		Source:  first.Source,
		Line:    first.Line,
		Managed: first.Managed,
		Synced:  first.Synced,
		Literal: first,
	}
	for _, block := range ins.Hosts {
		if !sshconfig.HostPatternMatch(block.Alias, first.Alias) {
			continue
		}
		if out.HostName == "" {
			out.HostName = block.HostName
		}
		if out.User == "" {
			out.User = block.User
		}
		if out.Port == "" {
			out.Port = block.Port
		}
		out.IdentityFiles = append(out.IdentityFiles, block.IdentityFiles...)
		out.RawIdentityFiles = append(out.RawIdentityFiles, block.RawIdentityFiles...)
		if out.IdentitiesOnly == "" {
			out.IdentitiesOnly = block.IdentitiesOnly
		}
		if out.ProxyJump == "" {
			out.ProxyJump = block.ProxyJump
		}
		if out.ForwardAgent == "" {
			out.ForwardAgent = block.ForwardAgent
		}
		if out.CertificateFile == "" {
			out.CertificateFile = block.CertificateFile
		}
		if out.ServerAliveInterval == "" {
			out.ServerAliveInterval = block.ServerAliveInterval
		}
		if out.ControlPath == "" {
			out.ControlPath = block.ControlPath
		}
		if out.ControlMaster == "" {
			out.ControlMaster = block.ControlMaster
		}
		if out.UseKeychain == "" {
			out.UseKeychain = block.UseKeychain
		}
		if out.AddKeysToAgent == "" {
			out.AddKeysToAgent = block.AddKeysToAgent
		}
	}
	return out
}

func existingFiles(paths []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func defaultIdentityFiles(home string) []string {
	dir := filepath.Join(home, ".ssh")
	names := []string{"id_rsa", "id_ecdsa", "id_ecdsa_sk", "id_ed25519", "id_ed25519_sk", "id_xmss", "id_dsa"}
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

func yesValue(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "yes" || v == "true"
}

func firstHop(proxy string) string {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" || strings.EqualFold(proxy, "none") {
		return ""
	}
	first, _, _ := strings.Cut(proxy, ",")
	first = strings.TrimSpace(first)
	if i := strings.LastIndex(first, "@"); i >= 0 {
		first = first[i+1:]
	}
	if strings.HasPrefix(first, "[") {
		if end := strings.Index(first, "]"); end > 0 {
			return first[1:end]
		}
	}
	if host, _, ok := strings.Cut(first, ":"); ok && !strings.Contains(first, "::") {
		return host
	}
	return first
}

func looksLikeHostname(value string) bool {
	if value == "" {
		return false
	}
	if strings.Contains(value, ".") || strings.Contains(value, ":") {
		return true
	}
	if strings.EqualFold(value, "localhost") {
		return true
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func updaterClient(opt Options) *http.Client {
	if opt.HTTP != nil {
		return opt.HTTP
	}
	return &http.Client{Timeout: 2 * time.Second}
}
