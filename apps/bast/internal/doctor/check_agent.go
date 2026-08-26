package doctor

import "runtime"

func (e Engine) checkAgent(r *Report, st runState) {
	ag := st.agent
	if !ag.checked {
		return
	}
	switch {
	case ag.present && ag.count > 0:
		title := "ssh-agent is running"
		if ag.count == 1 {
			title = "ssh-agent is running with 1 key"
		} else {
			title = "ssh-agent is running with keys"
		}
		r.add(Finding{ID: "agent.ok", Severity: SeverityOK, Category: CatAgent, Title: title})
		if ag.count >= 6 {
			r.add(Finding{
				ID: "agent.many_keys", Severity: SeverityWarn, Category: CatAgent,
				Title:  "ssh-agent holds many keys",
				Detail: "Servers often allow only a few attempts. Set IdentitiesOnly yes on hosts that pin an IdentityFile.",
			})
		}
	case ag.present && ag.empty:
		r.add(Finding{
			ID: "agent.empty", Severity: SeverityInfo, Category: CatAgent,
			Title: "ssh-agent is running with no keys",
			Fix:   "ssh-add --apple-use-keychain ~/.ssh/id_ed25519  (macOS) or ssh-add <key>",
		})
	default:
		detail := ag.err
		if detail == "" {
			detail = "Passphrase-protected keys will prompt on every connect."
		}
		fix := "eval \"$(ssh-agent -s)\" then ssh-add"
		if runtime.GOOS == "windows" {
			fix = "Start the OpenSSH Authentication Agent service, then ssh-add."
		}
		r.add(Finding{
			ID: "agent.missing", Severity: SeverityWarn, Category: CatAgent,
			Title: "ssh-agent is not running", Detail: detail, Fix: fix,
		})
	}
}
