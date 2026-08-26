package doctor

import (
	"os"
	"path/filepath"
	"runtime"

	"bast/internal/metadata"
)

func (e Engine) checkMetadata(r *Report, st runState) {
	if st.storeErr != nil {
		r.add(Finding{
			ID: "metadata.unreadable", Severity: SeverityFail, Category: CatMetadata,
			Title: "state.json could not be read", Path: e.display(e.Paths.StateFile),
			Detail: st.storeErr.Error(),
		})
		return
	}
	if st.store == nil {
		return
	}
	if !fileExists(e.Paths.StateFile) {
		return
	}
	hosts := st.store.Hosts()
	known := map[string]bool{}
	for _, h := range st.hosts {
		known[h.Alias] = true
	}
	orphans := 0
	for alias, meta := range hosts {
		if !known[alias] && (meta.Label != "" || meta.Group != "" || meta.Notes != "" || meta.Favorite || meta.Hidden) {
			orphans++
		}
		if meta.Group != "" {
			if _, err := metadata.NormalizeGroupPath(meta.Group); err != nil {
				r.add(Finding{
					ID: "metadata.invalid_group", Severity: SeverityWarn, Category: CatMetadata,
					Title: "Host \"" + alias + "\" has an invalid group path", Host: alias,
					Detail: err.Error(),
				})
			}
		}
	}
	if orphans > 0 {
		r.add(Finding{
			ID: "metadata.orphans", Severity: SeverityInfo, Category: CatMetadata,
			Title:  "Metadata remains for hosts that are no longer in SSH config",
			Path:   e.display(e.Paths.StateFile),
			Detail: "Groups, notes, and favorites for deleted aliases stay in state.json.",
		})
	}
	if runtime.GOOS == "darwin" {
		if configDir, err := os.UserConfigDir(); err == nil {
			legacy := filepath.Join(configDir, "bast", "state.json")
			if fileExists(legacy) && filepath.Clean(legacy) != filepath.Clean(e.Paths.StateFile) {
				r.add(Finding{
					ID: "metadata.legacy_state", Severity: SeverityInfo, Category: CatMetadata,
					Title: "Legacy state file is still present", Path: e.display(legacy),
					Detail: "Bast now uses ~/.config/bast/state.json.",
				})
			}
		}
	}
}
