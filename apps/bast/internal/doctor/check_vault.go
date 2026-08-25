package doctor

import (
	"encoding/json"
	"os"

	"bast/internal/platform"
	"bast/internal/vault"
)

func platformPOSIX() bool { return platform.SupportsPOSIXPermissions() }

func (e Engine) checkVault(r *Report) {
	sessionPath := vault.SessionPath(e.Paths.StateFile)
	passPath := vault.PassphrasePath(e.Paths.StateFile)
	if fileExists(sessionPath) {
		b, err := os.ReadFile(sessionPath)
		if err != nil {
			r.add(Finding{
				ID: "vault.session_unreadable", Severity: SeverityWarn, Category: CatVault,
				Title: "Vault session file could not be read", Path: e.display(sessionPath),
				Detail: err.Error(),
			})
		} else if !json.Valid(b) {
			r.add(Finding{
				ID: "vault.session_unreadable", Severity: SeverityWarn, Category: CatVault,
				Title: "Vault session file is not valid JSON", Path: e.display(sessionPath),
			})
		}
	}
	if fileExists(passPath) && platformPOSIX() {
		info, err := os.Stat(passPath)
		if err == nil && info.Mode().Perm()&0o077 != 0 {
			r.add(Finding{
				ID: "vault.passphrase_mode", Severity: SeverityFail, Category: CatVault,
				Title: "Vault passphrase file mode is too open", Path: e.display(passPath),
				Detail:  "Anyone who can read this file can decrypt the vault.",
				Fix:     "chmod 600 " + e.display(passPath),
				Fixable: true,
			})
		}
	}
}
