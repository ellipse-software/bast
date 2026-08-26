package doctor

import (
	"context"
	"os"
	"runtime"
	"strings"

	"bast/internal/updater"
)

func (e Engine) checkSuggest(ctx context.Context, r *Report, st runState, opt Options) {
	literals := st.hosts
	var noIdentitiesOnly, noKeepalive, ungrouped, external int
	useKeychain := false
	addKeys := false
	for _, h := range st.ins.Hosts {
		if yesValue(h.UseKeychain) {
			useKeychain = true
		}
		if yesValue(h.AddKeysToAgent) {
			addKeys = true
		}
	}
	for _, h := range literals {
		if len(h.IdentityFiles) > 0 && !yesValue(h.IdentitiesOnly) {
			noIdentitiesOnly++
		}
		if h.ServerAliveInterval == "" && h.HostName != "" && h.HostName != "localhost" && h.HostName != "127.0.0.1" {
			noKeepalive++
		}
		if !h.Managed && !h.Synced {
			external++
		}
	}
	if st.store != nil {
		for _, h := range literals {
			if strings.TrimSpace(st.store.Host(h.Alias).Group) == "" && !h.Synced {
				ungrouped++
			}
		}
	}
	if noIdentitiesOnly > 0 {
		r.add(Finding{
			ID: "suggest.identities_only", Severity: SeverityInfo, Category: CatSuggest,
			Title:  "Hosts with IdentityFile do not set IdentitiesOnly",
			Detail: "IdentitiesOnly yes stops ssh-agent from offering extra keys that cause \"Too many authentication failures\".",
			Fix:    "Edit the host in Bast and add IdentitiesOnly yes, or set it on Host *.",
		})
	}
	if noKeepalive > 2 {
		r.add(Finding{
			ID: "suggest.keepalive", Severity: SeverityInfo, Category: CatSuggest,
			Title:  "No ServerAliveInterval on remote hosts",
			Detail: "A 30-second keepalive drops idle NAT mappings less often.",
			Fix:    "Add ServerAliveInterval 30 to Host * or to individual hosts.",
		})
	}
	if runtime.GOOS == "darwin" && (!useKeychain || !addKeys) {
		r.add(Finding{
			ID: "suggest.macos_keychain", Severity: SeverityInfo, Category: CatSuggest,
			Title:  "macOS keychain is not referenced in SSH config",
			Detail: "UseKeychain yes and AddKeysToAgent yes store passphrases in the login keychain.",
			Fix:    "Add UseKeychain yes and AddKeysToAgent yes under Host * in ~/.ssh/config.",
		})
	}
	if ungrouped > 8 {
		r.add(Finding{
			ID: "suggest.groups", Severity: SeverityInfo, Category: CatSuggest,
			Title:  "Many hosts have no group",
			Detail: "Groups keep the picker scannable. Move hosts with m in the TUI, or bast hosts edit --group.",
		})
	}
	if external > 0 {
		r.add(Finding{
			ID: "suggest.promote", Severity: SeverityInfo, Category: CatSuggest,
			Title:  "External hosts can be promoted into Bast-managed config",
			Detail: "Promotion copies the host into ~/.ssh/bast/config so Bast can edit connection settings. Press p in the TUI.",
		})
	}
	e.checkUpdate(ctx, r, opt)
}

func (e Engine) checkUpdate(ctx context.Context, r *Report, opt Options) {
	if e.Version == "" || e.Version == "dev" {
		return
	}
	client := updaterClient(opt)
	var latest string
	var err error
	switch {
	case updater.IsStable(e.Version):
		latest, err = updater.Check(ctx, client, e.Version)
	case updater.IsNightly(e.Version):
		latest, err = updater.CheckNightly(ctx, client, e.Version)
	default:
		return
	}
	if err != nil || latest == "" {
		return
	}
	exe, _ := os.Executable()
	r.add(Finding{
		ID: "suggest.update", Severity: SeverityInfo, Category: CatSuggest,
		Title: "A newer Bast is available (" + latest + ")",
		Fix:   updater.Suggestion(exe),
	})
}
