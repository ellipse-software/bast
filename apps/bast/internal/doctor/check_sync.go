package doctor

import (
	"os"
	"os/exec"
	"strings"

	boxcloud "bast/internal/cloud/box"
)

func (e Engine) checkSync(r *Report, st runState) {
	if st.store == nil {
		return
	}
	type provider struct {
		name    string
		enabled bool
		cli     string
		err     string
	}
	providers := []provider{
		{"gcp", st.store.GCP().Enabled, "gcloud", st.store.GCP().LastSyncError},
		{"aws", st.store.AWS().Enabled, "aws", st.store.AWS().LastSyncError},
		{"azure", st.store.Azure().Enabled, "az", st.store.Azure().LastSyncError},
		{"box", st.store.Box().Enabled, "box", st.store.Box().LastSyncError},
	}
	for _, p := range providers {
		if !p.enabled {
			continue
		}
		if !providerCLIPresent(p.name, p.cli) {
			title := p.name + " sync is enabled but " + p.cli + " is not on PATH"
			fix := "Install the " + p.cli + " CLI, or run bast sync disable " + p.name + "."
			detail := ""
			if p.name == "box" {
				title = "box sync is enabled but the Box CLI was not found"
				detail = "The ASCII Box installer puts the binary at ~/.ascii/bin/box and a shell function named box. That function is not on PATH, so Bast looks at ~/.ascii/bin/box, ~/.local/bin/box, and BOX_CLI."
				fix = "Install from https://box.ascii.dev/, set BOX_CLI to the binary, or run bast sync disable box."
			}
			r.add(Finding{
				ID: "sync.cli_missing", Severity: SeverityFail, Category: CatSync,
				Title: title, Detail: detail, Fix: fix,
			})
		}
		if strings.TrimSpace(p.err) != "" {
			r.add(Finding{
				ID: "sync.last_error", Severity: SeverityWarn, Category: CatSync,
				Title: p.name + " last sync failed", Detail: p.err,
			})
		}
	}
	up := st.store.Upstash()
	if up.Enabled && !fileExists(e.Paths.UpstashAPIKey) && os.Getenv("UPSTASH_BOX_API_KEY") == "" {
		r.add(Finding{
			ID: "sync.upstash_key_missing", Severity: SeverityFail, Category: CatSync,
			Title: "Upstash sync is enabled but no API key is stored",
			Path:  e.display(e.Paths.UpstashAPIKey),
			Fix:   "bast upstash key --key-file <path>",
		})
	}
	if up.Enabled && strings.TrimSpace(up.LastSyncError) != "" {
		r.add(Finding{
			ID: "sync.last_error", Severity: SeverityWarn, Category: CatSync,
			Title: "upstash last sync failed", Detail: up.LastSyncError,
		})
	}
}

func providerCLIPresent(name, cli string) bool {
	if name == "box" {
		return boxCLIPresent()
	}
	_, err := exec.LookPath(cli)
	return err == nil
}

func boxCLIPresent() bool {
	bin := boxcloud.New().Box
	if bin == "" {
		return false
	}
	if _, err := exec.LookPath(bin); err == nil {
		return true
	}
	info, err := os.Stat(bin)
	return err == nil && !info.IsDir()
}
