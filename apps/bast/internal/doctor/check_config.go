package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"bast/internal/sshconfig"
)

func (e Engine) checkConfig(r *Report, st runState) {
	ins := st.ins
	if len(ins.Files) == 1 && ins.Files[0].Missing {
		r.add(Finding{
			ID: "ssh_config.missing", Severity: SeverityInfo, Category: CatSSHConfig,
			Title: "No ~/.ssh/config yet", Path: e.display(e.Paths.MainConfig),
			Detail: "Bast will create one and add Include ~/.ssh/bast/config when you add a managed host.",
			Fix:    "Run bast doctor --fix to create the Bast include now.", Fixable: true,
		})
		return
	}
	for _, file := range ins.Files {
		if file.Missing {
			continue
		}
		if file.BOM {
			r.add(Finding{
				ID: "ssh_config.bom", Severity: SeverityWarn, Category: CatSSHConfig,
				Title: "UTF-8 BOM at start of config", Path: e.display(file.Path),
				Detail: "A byte-order mark can make the first directive fail to parse.",
			})
		}
		if file.CRLF && runtime.GOOS != "windows" {
			r.add(Finding{
				ID: "ssh_config.crlf", Severity: SeverityWarn, Category: CatSSHConfig,
				Title: "CRLF line endings", Path: e.display(file.Path),
				Detail: "Unix OpenSSH expects LF line endings.",
			})
		}
	}
	for _, err := range ins.ParseErrors {
		sev := SeverityFail
		if err.Code == "parse" {
			sev = SeverityWarn
		}
		r.add(Finding{
			ID: "ssh_config." + err.Code, Severity: sev, Category: CatSSHConfig,
			Title: err.Message, Path: e.display(err.Path), Line: err.Line, Detail: err.Message,
		})
	}
	e.checkBastInclude(r, ins)
	e.checkSyncIncludes(r, ins, st)
	e.checkEmptyIncludes(r, ins)
	e.checkDuplicateAliases(r, ins)
	e.checkMatchAndWildcards(r, ins)
	e.checkHosts(r, ins, st)
}

func (e Engine) checkBastInclude(r *Report, ins sshconfig.Inspection) {
	var top, scoped []sshconfig.InspectInclude
	for _, inc := range ins.Includes {
		if !e.includeIs(inc, e.Paths.ManagedConfig) {
			continue
		}
		if inc.TopLevel && filepath.Clean(inc.Source) == filepath.Clean(e.Paths.MainConfig) {
			top = append(top, inc)
		} else {
			scoped = append(scoped, inc)
		}
	}
	managedHasHosts := false
	if b, err := os.ReadFile(e.Paths.ManagedConfig); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) >= 2 && strings.EqualFold(fields[0], "host") {
				managedHasHosts = true
				break
			}
		}
	}
	switch {
	case len(top) == 0 && len(scoped) > 0:
		inc := scoped[0]
		r.add(Finding{
			ID: "ssh_config.include_not_toplevel", Severity: SeverityFail, Category: CatSSHConfig,
			Title: "Include ~/.ssh/bast/config is inside a Host or Match block",
			Path:  e.display(inc.Source), Line: inc.Line, Host: inc.Scope,
			Detail:  "OpenSSH treats that Include as part of the preceding Host/Match, so Bast-managed hosts never apply.",
			Fix:     "Move Include ~/.ssh/bast/config to the top of ~/.ssh/config, before any Host or Match block.",
			Fixable: true,
		})
	case len(top) == 0 && managedHasHosts:
		r.add(Finding{
			ID: "ssh_config.include_missing", Severity: SeverityFail, Category: CatSSHConfig,
			Title: "Bast Include is missing from ~/.ssh/config", Path: e.display(e.Paths.MainConfig),
			Detail:  "Managed hosts in ~/.ssh/bast/config are not visible to OpenSSH.",
			Fix:     "Add Include ~/.ssh/bast/config at the top of ~/.ssh/config.",
			Fixable: true,
		})
	case len(top) == 0:
		r.add(Finding{
			ID: "ssh_config.include_missing", Severity: SeverityInfo, Category: CatSSHConfig,
			Title: "Bast Include is not in ~/.ssh/config yet", Path: e.display(e.Paths.MainConfig),
			Detail:  "Bast prepends Include ~/.ssh/bast/config on first managed write.",
			Fix:     "Run bast doctor --fix to add it now.",
			Fixable: true,
		})
	default:
		if len(top) > 1 {
			r.add(Finding{
				ID: "ssh_config.include_duplicate", Severity: SeverityWarn, Category: CatSSHConfig,
				Title: "Bast Include appears more than once", Path: e.display(top[1].Source), Line: top[1].Line,
				Detail: "Duplicate Include lines are harmless but noisy.",
			})
		} else {
			r.add(Finding{
				ID: "ssh_config.include_ok", Severity: SeverityOK, Category: CatSSHConfig,
				Title: "Include ~/.ssh/bast/config is at the top of ~/.ssh/config",
				Path:  e.display(top[0].Source), Line: top[0].Line,
			})
		}
		if len(scoped) > 0 {
			inc := scoped[0]
			r.add(Finding{
				ID: "ssh_config.include_scoped_duplicate", Severity: SeverityWarn, Category: CatSSHConfig,
				Title: "A second Bast Include sits inside a Host or Match block",
				Path:  e.display(inc.Source), Line: inc.Line, Host: inc.Scope,
				Detail: "The top-level Include is enough. The Host-scoped copy is confusing and does not replace it.",
			})
		}
	}
}

func (e Engine) checkSyncIncludes(r *Report, ins sshconfig.Inspection, st runState) {
	targets := []struct {
		path string
		name string
	}{
		{e.Paths.SyncGCPConfig, "gcp"},
		{e.Paths.SyncAWSConfig, "aws"},
		{e.Paths.SyncAzureConfig, "azure"},
		{e.Paths.SyncBoxConfig, "box"},
		{e.Paths.SyncUpstashConfig, "upstash"},
	}
	enabled := map[string]bool{}
	if st.store != nil {
		enabled["gcp"] = st.store.GCP().Enabled
		enabled["aws"] = st.store.AWS().Enabled
		enabled["azure"] = st.store.Azure().Enabled
		enabled["box"] = st.store.Box().Enabled
		enabled["upstash"] = st.store.Upstash().Enabled
	}
	for _, t := range targets {
		var found []sshconfig.InspectInclude
		for _, inc := range ins.Includes {
			if e.includeIs(inc, t.path) {
				found = append(found, inc)
			}
		}
		if len(found) == 0 {
			continue
		}
		leading := false
		for _, inc := range found {
			if inc.TopLevel && filepath.Clean(inc.Source) == filepath.Clean(e.Paths.ManagedConfig) {
				leading = true
			}
		}
		if !leading {
			inc := found[0]
			r.add(Finding{
				ID: "ssh_config.sync_include_not_leading", Severity: SeverityFail, Category: CatSSHConfig,
				Title: "Sync Include for " + t.name + " is not at the start of ~/.ssh/bast/config",
				Path:  e.display(inc.Source), Line: inc.Line,
				Detail: "Include must come before any Host or Match block or synced hosts never apply.",
			})
		}
		if st.store != nil && !enabled[t.name] {
			inc := found[0]
			r.add(Finding{
				ID: "sync.include_orphaned", Severity: SeverityWarn, Category: CatSync,
				Title: t.name + " sync Include is present while sync is disabled",
				Path:  e.display(inc.Source), Line: inc.Line,
				Detail: "Disconnecting a provider should remove its Include. Disable again from bast sync disable " + t.name + ".",
			})
		}
	}
}

func (e Engine) checkEmptyIncludes(r *Report, ins sshconfig.Inspection) {
	for _, inc := range ins.Includes {
		if inc.Empty && !inc.Cycle && !inc.Deep {
			r.add(Finding{
				ID: "ssh_config.include_empty", Severity: SeverityWarn, Category: CatSSHConfig,
				Title: "Include matched no files", Path: e.display(inc.Source), Line: inc.Line,
				Detail: "Pattern " + inc.Pattern + " expanded to " + e.display(inc.Expanded) + ".",
			})
		}
	}
}

func (e Engine) checkDuplicateAliases(r *Report, ins sshconfig.Inspection) {
	index := map[string][]sshconfig.InspectHost{}
	for _, h := range ins.Hosts {
		if h.Wildcard {
			continue
		}
		index[h.Alias] = append(index[h.Alias], h)
	}
	names := make([]string, 0, len(index))
	for alias := range index {
		names = append(names, alias)
	}
	sort.Strings(names)
	for _, alias := range names {
		copies := index[alias]
		if len(copies) < 2 {
			continue
		}
		later := copies[1]
		r.add(Finding{
			ID: "ssh_config.duplicate_alias", Severity: SeverityWarn, Category: CatSSHConfig,
			Title: "Host \"" + alias + "\" is defined more than once",
			Path:  e.display(later.Source), Line: later.Line, Host: alias,
			Detail: "Bast lists the first copy only (" + e.display(copies[0].Source) + "). Later blocks still apply in OpenSSH for unset keywords and extra IdentityFile lines.",
		})
	}
}

func (e Engine) checkMatchAndWildcards(r *Report, ins sshconfig.Inspection) {
	if len(ins.Matches) > 0 {
		m := ins.Matches[0]
		r.add(Finding{
			ID: "ssh_config.match_blocks", Severity: SeverityInfo, Category: CatSSHConfig,
			Title: "Match blocks are present", Path: e.display(m.Source), Line: m.Line,
			Detail: "OpenSSH applies Match rules. Bast's host picker does not list them as hosts.",
		})
	}
	var wild []sshconfig.InspectHost
	for _, h := range ins.Hosts {
		if h.Wildcard {
			wild = append(wild, h)
		}
	}
	if len(wild) > 0 {
		h := wild[0]
		r.add(Finding{
			ID: "ssh_config.wildcard_hosts", Severity: SeverityInfo, Category: CatSSHConfig,
			Title: "Wildcard Host patterns are present", Path: e.display(h.Source), Line: h.Line,
			Detail: "Patterns like Host * still apply in OpenSSH. Bast only lists literal aliases in the picker.",
		})
	}
}

func (e Engine) checkHosts(r *Report, ins sshconfig.Inspection, st runState) {
	literals := st.hosts
	if literals == nil {
		literals = e.literalHosts(ins)
	}
	known := map[string]bool{}
	for _, h := range literals {
		known[h.Alias] = true
	}
	tooMany := 0
	for _, h := range literals {
		e.checkOneHost(r, h, known, st, &tooMany)
	}
}

func (e Engine) checkOneHost(r *Report, h effectiveHost, known map[string]bool, st runState, tooMany *int) {
	if h.HostName == "" && !looksLikeHostname(h.Alias) {
		r.add(Finding{
			ID: "ssh_config.hostname_missing", Severity: SeverityWarn, Category: CatSSHConfig,
			Title: "Host \"" + h.Alias + "\" has no HostName", Path: e.display(h.Source), Line: h.Line, Host: h.Alias,
			Detail: "OpenSSH will use the alias as the DNS name.",
			Fix:    "Set HostName, or edit the host in Bast.",
		})
	}
	if len(h.IdentityFiles) > 0 {
		for i, id := range h.IdentityFiles {
			raw := id
			if i < len(h.RawIdentityFiles) {
				raw = h.RawIdentityFiles[i]
			}
			if strings.HasSuffix(raw, ".pub") || strings.HasSuffix(id, ".pub") {
				r.add(Finding{
					ID: "ssh_config.identity_is_public", Severity: SeverityFail, Category: CatSSHConfig,
					Title: "IdentityFile for \"" + h.Alias + "\" points at a .pub file",
					Path:  e.display(id), Host: h.Alias,
					Detail: "IdentityFile must be the private key.",
					Fix:    "Point IdentityFile at the private key (no .pub suffix).",
				})
			}
			if !fileExists(id) {
				r.add(Finding{
					ID: "ssh_config.identity_missing", Severity: SeverityFail, Category: CatSSHConfig,
					Title: "IdentityFile for \"" + h.Alias + "\" does not exist",
					Path:  e.display(id), Host: h.Alias,
					Detail: "OpenSSH will try a missing key and fail that method.",
					Fix:    "Create the key, or point the host at an existing identity in Bast.",
				})
			}
			if !filepath.IsAbs(raw) && !strings.HasPrefix(raw, "~") {
				r.add(Finding{
					ID: "ssh_config.identity_relative", Severity: SeverityWarn, Category: CatSSHConfig,
					Title: "IdentityFile for \"" + h.Alias + "\" is a relative path",
					Path:  raw, Host: h.Alias,
					Detail: "Relative IdentityFile paths are resolved from the current working directory, not ~/.ssh.",
				})
			}
		}
	}
	if h.CertificateFile != "" && len(h.IdentityFiles) == 0 {
		r.add(Finding{
			ID: "ssh_config.certificate_without_identity", Severity: SeverityWarn, Category: CatSSHConfig,
			Title: "CertificateFile for \"" + h.Alias + "\" has no IdentityFile",
			Path:  e.display(h.CertificateFile), Host: h.Alias,
			Detail: "A certificate needs the matching private key.",
		})
	}
	if hop := firstHop(h.ProxyJump); hop != "" && !known[hop] && !looksLikeHostname(hop) {
		r.add(Finding{
			ID: "ssh_config.proxyjump_unknown", Severity: SeverityWarn, Category: CatSSHConfig,
			Title: "ProxyJump \"" + hop + "\" for \"" + h.Alias + "\" is not a known host",
			Path:  e.display(h.Source), Line: h.Line, Host: h.Alias,
			Detail: "OpenSSH must resolve that jump as an alias or a hostname.",
		})
	}
	if yesValue(h.ForwardAgent) && !st.agent.present {
		r.add(Finding{
			ID: "ssh_config.forward_agent_no_agent", Severity: SeverityWarn, Category: CatSSHConfig,
			Title: "ForwardAgent is yes on \"" + h.Alias + "\" but no agent is running",
			Path:  e.display(h.Source), Line: h.Line, Host: h.Alias,
		})
	}
	if h.ControlPath != "" && h.ControlMaster == "" {
		r.add(Finding{
			ID: "ssh_config.controlpath_without_master", Severity: SeverityInfo, Category: CatSSHConfig,
			Title: "ControlPath is set on \"" + h.Alias + "\" without ControlMaster",
			Path:  e.display(h.Source), Line: h.Line, Host: h.Alias,
		})
	}
	offered := existingFiles(h.IdentityFiles)
	if len(h.IdentityFiles) == 0 {
		offered = existingFiles(defaultIdentityFiles(e.Paths.Home))
	}
	identitiesOnly := yesValue(h.IdentitiesOnly)
	count := len(offered)
	if !identitiesOnly && st.agent.present {
		count += st.agent.count
	}
	if count >= 4 && *tooMany < 8 {
		*tooMany++
		sev := SeverityWarn
		if count >= 6 {
			sev = SeverityFail
		}
		r.add(Finding{
			ID: "ssh_config.too_many_identities", Severity: sev, Category: CatSSHConfig,
			Title: "Host \"" + h.Alias + "\" offers many identities",
			Path:  e.display(h.Source), Line: h.Line, Host: h.Alias,
			Detail: "OpenSSH may hit \"Too many authentication failures\" when it tries every agent key and IdentityFile.",
			Fix:    "Set IdentitiesOnly yes on this host, or in Bast: edit the host and add that SSH flag.",
		})
	}
}

func (e Engine) includeIs(inc sshconfig.InspectInclude, target string) bool {
	if target == "" {
		return false
	}
	want := filepath.Clean(target)
	if filepath.Clean(inc.Expanded) == want {
		return true
	}
	for _, match := range inc.Matches {
		if filepath.Clean(match) == want {
			return true
		}
	}
	rel := e.display(target)
	if inc.Pattern == rel || inc.Pattern == target {
		return true
	}
	if strings.Contains(inc.Pattern, "bast/config") && strings.HasSuffix(want, string(filepath.Separator)+"bast"+string(filepath.Separator)+"config") {
		if !strings.Contains(inc.Pattern, "/sync/") && !strings.Contains(want, string(filepath.Separator)+"sync"+string(filepath.Separator)) {
			return filepath.Base(want) == "config" && filepath.Base(filepath.Dir(want)) == "bast"
		}
	}
	return false
}
