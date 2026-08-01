package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"bast/internal/telemetry"
	"bast/internal/vault"
)

func (r *Runner) trackVaultResult(okEvent, failEvent string, err error) {
	if err == nil {
		telemetry.Track(okEvent, r.Version)
		return
	}
	var ce *commandError
	if errors.As(err, &ce) && ce.code == "usage" {
		return
	}
	telemetry.Track(failEvent, r.Version)
}

func (r *Runner) trackVaultCall(okEvent, failEvent string, fn func() error) error {
	err := fn()
	r.trackVaultResult(okEvent, failEvent, err)
	return err
}

func (r *Runner) vault(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast vault <login|status|push|pull|logout|passphrase>")
		return nil
	}
	switch args[0] {
	case "login":
		return r.trackVaultCall("vault_link", "vault_link_fail", func() error { return r.vaultLogin(args[1:]) })
	case "status":
		return r.vaultStatus(args[1:])
	case "push":
		return r.trackVaultCall("vault_push", "vault_push_fail", func() error { return r.vaultPush(args[1:]) })
	case "pull":
		return r.trackVaultCall("vault_pull", "vault_pull_fail", func() error { return r.vaultPull(args[1:]) })
	case "logout":
		err := r.vaultLogout(args[1:])
		if err == nil {
			telemetry.Track("vault_logout", r.Version)
		}
		return err
	case "passphrase":
		return r.vaultPassphrase(args[1:])
	default:
		return usagef("unknown vault command %q", args[0])
	}
}

func (r *Runner) vaultSessionPath() string {
	return vault.SessionPath(r.Paths.StateFile)
}

func (r *Runner) vaultClient(session vault.Session) *vault.Client {
	return &vault.Client{BaseURL: vault.EffectiveAPIBase(session.APIBase), Token: session.Token}
}

func (r *Runner) vaultLogin(args []string) error {
	fs := newFlagSet("vault login")
	emailFlag := fs.String("email", "", "account email")
	apiBase := fs.String("api", "", "vault API base URL")
	mode := fs.String("mode", "merge", "first-link merge mode: merge|replace_local|replace_remote")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vault login [--email address] [--api url] [--mode merge|replace_local|replace_remote]")
	}
	email := strings.TrimSpace(*emailFlag)
	var err error
	if email == "" {
		email, err = r.prompt("Email", "", true)
		if err != nil {
			return err
		}
	}
	email = strings.ToLower(strings.TrimSpace(email))
	base := vault.NormalizeAPIBase(*apiBase)
	if base == "" {
		base = vault.EffectiveAPIBase("")
	}
	client := &vault.Client{BaseURL: base}
	{
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		err := client.StartOTP(ctx, email)
		cancel()
		if err != nil {
			return err
		}
	}
	code, err := r.prompt("Code from email", "", true)
	if err != nil {
		return err
	}
	var verified vault.OTPVerifyResponse
	{
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		verified, err = client.VerifyOTP(ctx, email, strings.TrimSpace(code))
		cancel()
		if err != nil {
			return err
		}
	}
	passphrase, err := r.readPassphrase("Vault passphrase (never sent to Bast servers)")
	if err != nil {
		return err
	}
	if passphrase == "" {
		return fail("passphrase_required", "vault passphrase is required")
	}
	confirm, err := r.readPassphrase("Confirm vault passphrase")
	if err != nil {
		return err
	}
	if passphrase != confirm {
		return fail("passphrase_mismatch", "passphrases did not match")
	}
	session := vault.Session{
		Email:    verified.Email,
		Token:    verified.Token,
		UserID:   verified.UserID,
		DeviceID: verified.DeviceID,
		APIBase:  base,
	}
	if session.Email == "" {
		session.Email = email
	}
	client.Token = session.Token

	mergeMode := vault.MergeMode(strings.ToLower(strings.TrimSpace(*mode)))
	switch mergeMode {
	case vault.MergeModeMerge, vault.MergeModeReplaceLocal, vault.MergeModeReplaceRemote:
	default:
		return usagef("mode must be merge, replace_local, or replace_remote")
	}

	// Persist auth before merge/apply/push so a later failure cannot leave
	// hosts/keys applied while status still says not linked.
	if err := vault.SaveSession(r.vaultSessionPath(), session); err != nil {
		return err
	}
	if err := vault.SavePassphrase(vault.PassphrasePath(r.Paths.StateFile), passphrase); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return err
	}
	packer := vault.Packer{Paths: r.Paths, Config: r.config, Keyring: r.keyring, Store: r.store}
	localDoc, err := packer.Pack()
	if err != nil {
		return err
	}
	var merged vault.Document
	if len(remoteGet.Ciphertext) == 0 {
		merged = localDoc
	} else {
		remoteDoc, decErr := vault.Decrypt(remoteGet.Ciphertext, passphrase)
		if decErr != nil {
			return decErr
		}
		result := vault.Merge(localDoc, remoteDoc, mergeMode)
		if len(result.Conflicts) > 0 && mergeMode == vault.MergeModeMerge {
			return fail("vault_conflict", fmt.Sprintf("vault has %d conflicts; resolve with --mode replace_local or replace_remote", len(result.Conflicts)))
		}
		merged = result.Document
		if !r.JSON {
			fmt.Fprintf(r.Out, "Linked · local %d hosts / %d keys · remote %d hosts / %d keys · conflicts %d\n",
				result.Summary.LocalHosts, result.Summary.LocalKeys, result.Summary.RemoteHosts, result.Summary.RemoteKeys, result.Summary.Conflicts)
		}
		applier := vault.Applier{Paths: r.Paths, Config: r.config, Store: r.store}
		if err := applier.Apply(merged); err != nil {
			return err
		}
	}
	blob, err := vault.Encrypt(merged, passphrase)
	if err != nil {
		return err
	}
	meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
	if err != nil {
		return err
	}
	session.Revision = meta.Revision
	if err := vault.SaveSession(r.vaultSessionPath(), session); err != nil {
		return err
	}
	return r.success(map[string]any{
		"email":    session.Email,
		"revision": session.Revision,
	}, "Vault linked: "+session.Email)
}

func (r *Runner) vaultStatus(args []string) error {
	fs := newFlagSet("vault status")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vault status")
	}
	session, err := vault.LoadSession(r.vaultSessionPath())
	if err != nil {
		if os.IsNotExist(err) {
			return r.success(map[string]any{"linked": false}, "Vault not linked")
		}
		return err
	}
	return r.success(map[string]any{
		"linked":   true,
		"email":    session.Email,
		"revision": session.Revision,
		"apiBase":  vault.EffectiveAPIBase(session.APIBase),
	}, fmt.Sprintf("Vault linked · %s · %s · revision %s", session.Email, vault.EffectiveAPIBase(session.APIBase), emptyDefault(session.Revision)))
}

func (r *Runner) vaultPush(args []string) error {
	fs := newFlagSet("vault push")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vault push")
	}
	session, passphrase, client, err := r.vaultReady()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return err
	}
	if len(remoteGet.Ciphertext) == 0 && session.Revision != "" {
		return fail("vault_missing", "remote vault missing; run bast vault passphrase --force to replace it")
	}
	if err := vault.VerifyPassphrase(remoteGet.Ciphertext, passphrase); err != nil {
		return err
	}
	prev := vault.Document{}
	var remoteDoc vault.Document
	haveRemote := len(remoteGet.Ciphertext) > 0
	if haveRemote {
		doc, decErr := vault.Decrypt(remoteGet.Ciphertext, passphrase)
		if decErr != nil {
			_ = vault.ClearPassphrase(vault.PassphrasePath(r.Paths.StateFile))
			return decErr
		}
		remoteDoc = doc
		prev = doc
	}
	packer := vault.Packer{Paths: r.Paths, Config: r.config, Keyring: r.keyring, Store: r.store, Previous: prev}
	localDoc, err := packer.Pack()
	if err != nil {
		return err
	}
	doc := localDoc
	if haveRemote {
		result := vault.Merge(localDoc, remoteDoc, vault.MergeModeMerge)
		if len(result.Conflicts) > 0 {
			return fail("vault_conflict", fmt.Sprintf("vault has %d conflicts; resolve with bast vault pull --mode replace_local or replace_remote, then push", len(result.Conflicts)))
		}
		doc = result.Document
		applier := vault.Applier{Paths: r.Paths, Config: r.config, Store: r.store}
		if err := applier.Apply(doc); err != nil {
			return err
		}
	}
	doc.Revision = remoteGet.Meta.Revision
	if doc.Revision == "" {
		doc.Revision = session.Revision
	}
	blob, err := vault.Encrypt(doc, passphrase)
	if err != nil {
		return err
	}
	ifMatch := remoteGet.Meta.Revision
	if ifMatch == "" {
		ifMatch = session.Revision
	}
	meta, err := client.PutVault(ctx, blob, ifMatch)
	if err != nil {
		return err
	}
	session.Revision = meta.Revision
	if err := vault.SaveSession(r.vaultSessionPath(), session); err != nil {
		return err
	}
	return r.success(map[string]any{"revision": meta.Revision}, "Vault pushed")
}

func (r *Runner) vaultPull(args []string) error {
	fs := newFlagSet("vault pull")
	mode := fs.String("mode", "merge", "merge mode: merge|replace_local|replace_remote")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vault pull [--mode merge|replace_local|replace_remote]")
	}
	mergeMode := vault.MergeMode(strings.ToLower(strings.TrimSpace(*mode)))
	switch mergeMode {
	case vault.MergeModeMerge, vault.MergeModeReplaceLocal, vault.MergeModeReplaceRemote:
	default:
		return usagef("mode must be merge, replace_local, or replace_remote")
	}
	session, passphrase, client, err := r.vaultReady()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return err
	}
	if len(remoteGet.Ciphertext) == 0 {
		return r.success(map[string]any{"changed": false}, "No remote vault yet; push first")
	}
	remoteDoc, err := vault.Decrypt(remoteGet.Ciphertext, passphrase)
	if err != nil {
		_ = vault.ClearPassphrase(vault.PassphrasePath(r.Paths.StateFile))
		return err
	}
	if remoteGet.Meta.Revision != "" && remoteGet.Meta.Revision == session.Revision {
		return r.success(map[string]any{"revision": session.Revision, "changed": false}, "Vault already up to date")
	}
	packer := vault.Packer{Paths: r.Paths, Config: r.config, Keyring: r.keyring, Store: r.store}
	localDoc, err := packer.Pack()
	if err != nil {
		return err
	}
	result := vault.Merge(localDoc, remoteDoc, mergeMode)
	if len(result.Conflicts) > 0 && mergeMode == vault.MergeModeMerge {
		return fail("vault_conflict", fmt.Sprintf("vault has %d conflicts; resolve with --mode replace_local or replace_remote", len(result.Conflicts)))
	}
	applier := vault.Applier{Paths: r.Paths, Config: r.config, Store: r.store}
	if err := applier.Apply(result.Document); err != nil {
		return err
	}
	session.Revision = remoteGet.Meta.Revision
	if err := vault.SaveSession(r.vaultSessionPath(), session); err != nil {
		return err
	}
	return r.success(map[string]any{
		"revision":  session.Revision,
		"changed":   true,
		"conflicts": len(result.Conflicts),
		"summary":   result.Summary,
	}, "Vault pulled")
}

func (r *Runner) vaultLogout(args []string) error {
	fs := newFlagSet("vault logout")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vault logout")
	}
	session, err := vault.LoadSession(r.vaultSessionPath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && session.Token != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = r.vaultClient(session).Logout(ctx)
		cancel()
	}
	if err := vault.ClearSession(r.vaultSessionPath()); err != nil {
		return err
	}
	_ = vault.ClearPassphrase(vault.PassphrasePath(r.Paths.StateFile))
	return r.success(map[string]any{"linked": false}, "Vault logged out")
}

func (r *Runner) vaultPassphrase(args []string) error {
	fs := newFlagSet("vault passphrase")
	force := fs.Bool("force", false, "overwrite remote with this machine's vault under a new passphrase (old passphrase not required)")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vault passphrase [--force]")
	}
	if *force {
		if err := r.vaultPassphraseForce(); err != nil {
			return err
		}
		telemetry.Track("vault_passphrase_reset", r.Version)
		return nil
	}
	session, oldPass, client, err := r.vaultReady()
	if err != nil {
		return err
	}
	newPass, err := r.readPassphrase("New vault passphrase")
	if err != nil {
		return err
	}
	confirm, err := r.readPassphrase("Confirm new vault passphrase")
	if err != nil {
		return err
	}
	if newPass == "" || newPass != confirm {
		return fail("passphrase_mismatch", "passphrases did not match")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return err
	}
	var doc vault.Document
	if len(remoteGet.Ciphertext) > 0 {
		doc, err = vault.Decrypt(remoteGet.Ciphertext, oldPass)
		if err != nil {
			return err
		}
	} else {
		packer := vault.Packer{Paths: r.Paths, Config: r.config, Keyring: r.keyring, Store: r.store}
		doc, err = packer.Pack()
		if err != nil {
			return err
		}
	}
	blob, err := vault.Encrypt(doc, newPass)
	if err != nil {
		return err
	}
	meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
	if err != nil {
		return err
	}
	session.Revision = meta.Revision
	if err := vault.SaveSession(r.vaultSessionPath(), session); err != nil {
		return err
	}
	if err := vault.SavePassphrase(vault.PassphrasePath(r.Paths.StateFile), newPass); err != nil {
		return err
	}
	telemetry.Track("vault_passphrase_rotate", r.Version)
	return r.success(map[string]any{"revision": meta.Revision, "force": false}, "Vault passphrase updated")
}

func (r *Runner) vaultPassphraseForce() error {
	session, err := vault.LoadSession(r.vaultSessionPath())
	if err != nil {
		if os.IsNotExist(err) {
			return fail("not_linked", "vault not linked; run bast vault login")
		}
		return err
	}
	if !r.JSON {
		fmt.Fprintln(r.Err, "Force reset replaces the remote vault with this machine's managed hosts/keys.")
		fmt.Fprintln(r.Err, "Anything only on other machines (or only in the old ciphertext) will be lost.")
	}
	newPass, err := r.readPassphrase("New vault passphrase")
	if err != nil {
		return err
	}
	confirm, err := r.readPassphrase("Confirm new vault passphrase")
	if err != nil {
		return err
	}
	if newPass == "" || newPass != confirm {
		return fail("passphrase_mismatch", "passphrases did not match")
	}
	client := r.vaultClient(session)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return err
	}
	packer := vault.Packer{Paths: r.Paths, Config: r.config, Keyring: r.keyring, Store: r.store}
	doc, err := packer.Pack()
	if err != nil {
		return err
	}
	blob, err := vault.Encrypt(doc, newPass)
	if err != nil {
		return err
	}
	meta, err := client.PutVault(ctx, blob, remoteGet.Meta.Revision)
	if err != nil {
		return err
	}
	session.Revision = meta.Revision
	if err := vault.SaveSession(r.vaultSessionPath(), session); err != nil {
		return err
	}
	if err := vault.SavePassphrase(vault.PassphrasePath(r.Paths.StateFile), newPass); err != nil {
		return err
	}
	return r.success(map[string]any{"revision": meta.Revision, "force": true}, "Vault passphrase reset · remote replaced from this machine")
}

func (r *Runner) vaultReady() (vault.Session, string, *vault.Client, error) {
	session, err := vault.LoadSession(r.vaultSessionPath())
	if err != nil {
		if os.IsNotExist(err) {
			return vault.Session{}, "", nil, fail("not_linked", "vault not linked; run bast vault login")
		}
		return vault.Session{}, "", nil, err
	}
	passPath := vault.PassphrasePath(r.Paths.StateFile)
	passphrase, err := vault.LoadPassphrase(passPath)
	if err != nil {
		return vault.Session{}, "", nil, err
	}
	if passphrase == "" {
		passphrase, err = r.readPassphrase("Vault passphrase")
		if err != nil {
			return vault.Session{}, "", nil, err
		}
	}
	if passphrase == "" {
		return vault.Session{}, "", nil, fail("passphrase_required", "vault passphrase is required")
	}
	client := r.vaultClient(session)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	remoteGet, err := client.GetVault(ctx, "")
	if err != nil {
		return vault.Session{}, "", nil, err
	}
	if len(remoteGet.Ciphertext) == 0 && session.Revision != "" {
		return vault.Session{}, "", nil, fail("vault_missing", "remote vault missing; run bast vault passphrase --force to replace it")
	}
	if err := vault.VerifyPassphrase(remoteGet.Ciphertext, passphrase); err != nil {
		_ = vault.ClearPassphrase(passPath)
		if !r.NoInput {
			fmt.Fprintln(r.Err, "Saved passphrase rejected; enter the correct vault passphrase.")
			passphrase, err = r.readPassphrase("Vault passphrase")
			if err != nil {
				return vault.Session{}, "", nil, err
			}
			if err := vault.VerifyPassphrase(remoteGet.Ciphertext, passphrase); err != nil {
				return vault.Session{}, "", nil, err
			}
		} else {
			return vault.Session{}, "", nil, err
		}
	}
	if err := vault.SavePassphrase(passPath, passphrase); err != nil {
		return vault.Session{}, "", nil, err
	}
	return session, passphrase, client, nil
}

func (r *Runner) readPassphrase(prompt string) (string, error) {
	if r.NoInput {
		return "", fail("interactive_required", prompt+" requires an interactive terminal")
	}
	fmt.Fprintf(r.Err, "%s: ", prompt)
	in, ok := r.In.(*os.File)
	if !ok {
		line, err := r.prompt(prompt, "", true)
		return line, err
	}
	if !term.IsTerminal(int(in.Fd())) {
		line, err := r.prompt(prompt, "", true)
		return line, err
	}
	secret, err := term.ReadPassword(int(in.Fd()))
	fmt.Fprintln(r.Err)
	if err != nil {
		if err == syscall.EINTR {
			return "", err
		}
		return "", err
	}
	return string(secret), nil
}
