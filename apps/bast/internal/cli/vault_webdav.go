package cli

import (
	"context"
	"os"
	"strings"
	"time"

	"bast/internal/vault"
)

func (r *Runner) vaultLoginWebDAV(directory, username, mode string) error {
	if r.NoInput || !r.interactive() {
		return fail("interactive_required", "WebDAV login requires an interactive terminal for the vault encryption passphrase")
	}
	switch vault.MergeMode(strings.ToLower(strings.TrimSpace(mode))) {
	case vault.MergeModeMerge, vault.MergeModeReplaceLocal, vault.MergeModeReplaceRemote:
	default:
		return usagef("mode must be merge, replace_local, or replace_remote")
	}
	var err error
	if username == "" {
		username, err = r.prompt("WebDAV username", "", true)
		if err != nil {
			return err
		}
	}
	password := os.Getenv("BAST_WEBDAV_PASSWORD")
	if password == "" {
		password, err = r.readPassphrase("WebDAV password or app password")
		if err != nil {
			return err
		}
	}
	cfg := &vault.WebDAVConfig{URL: strings.TrimSpace(directory), Username: username, Password: password}
	if err := cfg.Validate(); err != nil {
		return err
	}
	session := vault.Session{Backend: "webdav", WebDAV: cfg}
	client := r.vaultClient(session)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.CheckWebDAV(ctx); err != nil {
		return err
	}
	passphrase, err := r.readPassphrase("Vault encryption passphrase (never sent to WebDAV)")
	if err != nil {
		return err
	}
	confirm, err := r.readPassphrase("Confirm vault encryption passphrase")
	if err != nil {
		return err
	}
	if passphrase == "" || passphrase != confirm {
		return fail("passphrase_mismatch", "passphrases did not match or were empty")
	}
	return r.vaultLink(session, passphrase, mode)
}
