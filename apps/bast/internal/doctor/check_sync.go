package doctor

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	boatcloud "bast/internal/cloud/boat"
	"bast/internal/sshconfig"
)

func (e Engine) checkSync(r *Report, st runState) {
	if err := e.Config.CheckBoatSyncMigration(); err != nil {
		id, title := "sync.boat_migration", "Boat inventory migration could not be checked"
		fix := "Resolve the inventory error and restart Bast."
		var conflict *sshconfig.BoatMigrationConflict
		if errors.As(err, &conflict) {
			id, title = "sync.boat_migration_conflict", "Boat inventories conflict"
			fix = "Back up and reconcile both inventories, then restart Bast. Boat operations are blocked to preserve the files and host metadata."
		}
		r.add(Finding{ID: id, Severity: SeverityFail, Category: CatSync, Title: title, Detail: err.Error(), Fix: fix})
	}
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
		{"boat", st.store.Boat().Enabled, "boat", st.store.Boat().LastSyncError},
	}
	for _, p := range providers {
		if !p.enabled {
			continue
		}
		if !providerCLIPresent(p.name, p.cli) {
			title := p.name + " sync is enabled but " + p.cli + " is not on PATH"
			fix := "Install the " + p.cli + " CLI, or run bast sync disable " + p.name + "."
			detail := ""
			if p.name == "boat" {
				title = "boat sync is enabled but the Boat CLI was not found"
				detail = "The Boat installer puts the binary at ~/.ascii/bin/boat and a shell function named boat. That function is not on PATH, so Bast looks at ~/.ascii/bin/boat, ~/.local/bin/boat, and BOAT_CLI."
				fix = "Install from https://boat.dev/, set BOAT_CLI to the binary, or run bast sync disable boat."
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
	if name == "boat" {
		return boatCLIPresent()
	}
	_, err := exec.LookPath(cli)
	return err == nil
}

func boatCLIPresent() bool {
	bin := boatcloud.New().Boat
	if bin == "" {
		return false
	}
	if _, err := exec.LookPath(bin); err == nil {
		return true
	}
	info, err := os.Stat(bin)
	return err == nil && !info.IsDir()
}
