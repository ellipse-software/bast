package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/atotto/clipboard"

	"bast/internal/askpass"
	"bast/internal/metadata"
)

func (r *Runner) keys(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast keys <list|show|generate|import|promote|comment|export|install|passphrase|public|copy|delete>")
		return nil
	}
	switch args[0] {
	case "list":
		return r.keyList(args[1:])
	case "show":
		return r.keyShow(args[1:])
	case "generate":
		return r.keyGenerate(args[1:])
	case "import":
		return r.keyImport(args[1:])
	case "promote":
		return r.keyPromote(args[1:])
	case "comment":
		return r.keyComment(args[1:])
	case "export":
		return r.keyExport(args[1:])
	case "install":
		return r.keyInstall(args[1:])
	case "passphrase":
		return r.keyPassphrase(args[1:])
	case "public":
		return r.keyPublic(args[1:])
	case "copy":
		return r.keyCopy(args[1:])
	case "delete":
		return r.keyDelete(args[1:])
	default:
		return usagef("unknown keys command %q", args[0])
	}
}

func (r *Runner) keyList(args []string) error {
	fs := newFlagSet("keys list")
	search := fs.String("search", "", "filter keys")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast keys list [--search text]")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	q := strings.ToLower(*search)
	filtered := records[:0]
	for _, key := range records {
		if q == "" || strings.Contains(strings.ToLower(strings.Join([]string{key.Name, key.Algorithm, key.Fingerprint, key.Comment, strings.Join(key.References, " ")}, " ")), q) {
			filtered = append(filtered, key)
		}
	}
	if r.JSON {
		return r.success(filtered, "")
	}
	printKeyTable(r.Out, filtered)
	return nil
}

func (r *Runner) keyShow(args []string) error {
	fs := newFlagSet("keys show")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys show <name>")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	if r.JSON {
		return r.success(key, "")
	}
	fmt.Fprintf(r.Out, "Name: %s\nAlgorithm: %s\nFingerprint: %s\nComment: %s\nPrivate key: %s\nPublic key: %s\nManaged: %t\nIn agent: %t\nReferenced by: %s\n", key.Name, key.Algorithm, key.Fingerprint, key.Comment, key.PrivatePath, key.PublicPath, key.Managed, key.InAgent, strings.Join(key.References, ", "))
	return nil
}

func (r *Runner) keyGenerate(args []string) error {
	fs := newFlagSet("keys generate")
	algorithm := fs.String("algorithm", "ed25519", "ed25519 or rsa")
	noPassphrase := fs.Bool("no-passphrase", false, "create without a passphrase")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() > 1 {
		return usagef("usage: bast keys generate [name] [--algorithm ed25519|rsa] [--no-passphrase]")
	}
	name := ""
	if fs.NArg() == 1 {
		name = fs.Arg(0)
	}
	var err error
	if name == "" {
		name, err = r.prompt("Name", "", true)
		if err != nil {
			return err
		}
	}
	if r.JSON && !*noPassphrase {
		return fail("interactive_required", "JSON key generation requires --no-passphrase")
	}
	cmd, path, err := r.keyring.GenerateCommand(name, strings.ToLower(*algorithm))
	if err != nil {
		return err
	}
	if *noPassphrase {
		cmd.Args = append(cmd.Args, "-N", "")
	}
	if err := r.runProcess(cmd, !*noPassphrase); err != nil {
		return err
	}
	return r.success(map[string]string{"name": name, "privatePath": path, "publicPath": path + ".pub"}, "Key generated: "+name)
}

func (r *Runner) keyImport(args []string) error {
	fs := newFlagSet("keys import")
	private := fs.String("private", "", "private key path or - for stdin")
	public := fs.String("public", "", "public key path or - for stdin")
	comment := fs.String("comment", "", "public-key comment")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() > 1 {
		return usagef("usage: bast keys import [name] --private path|- [--public path|-] [--comment text]")
	}
	name := ""
	if fs.NArg() == 1 {
		name = fs.Arg(0)
	}
	var err error
	if name == "" {
		name, err = r.prompt("Name", "", true)
		if err != nil {
			return err
		}
	}
	if *private == "" {
		*private, err = r.prompt("Private key path", "", true)
		if err != nil {
			return err
		}
	}
	if *private == "-" && *public == "-" {
		return usagef("private and public keys cannot both be read from stdin")
	}
	privateSource := *private
	publicSource := *public
	if *private == "-" {
		content, readErr := io.ReadAll(r.In)
		if readErr != nil {
			return readErr
		}
		privateSource = string(content)
	}
	if *public == "-" {
		content, readErr := io.ReadAll(r.In)
		if readErr != nil {
			return readErr
		}
		publicSource = string(content)
	}
	if err := r.keyring.Import(privateSource, publicSource, name, *comment); err != nil {
		return err
	}
	return r.success(map[string]string{"name": name}, "Key imported: "+name)
}

func (r *Runner) keyPromote(args []string) error {
	fs := newFlagSet("keys promote")
	name := fs.String("name", "", "managed key name (defaults to current name)")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys promote <key> [--name managed-name]")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	if key.Managed {
		return fail("already_managed", "key is already Bast managed")
	}
	managedName := *name
	if managedName == "" {
		managedName = key.Name
	}
	if err := r.keyring.Promote(key.raw, managedName); err != nil {
		return err
	}
	return r.success(map[string]string{"name": managedName, "source": key.Name}, "Key promoted: "+managedName)
}

func (r *Runner) keyComment(args []string) error {
	fs := newFlagSet("keys comment")
	var comment optionalString
	fs.Var(&comment, "comment", "new comment")
	clear := fs.Bool("clear-comment", false, "remove the comment")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys comment <name> (--comment text|--clear-comment)")
	}
	if comment.set && *clear {
		return usagef("--comment and --clear-comment cannot be combined")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	value := comment.value
	if !comment.set && !*clear {
		if r.NoInput || !r.interactive() {
			return usagef("--comment or --clear-comment is required")
		}
		value, err = r.prompt("Comment", key.Comment, false)
		if err != nil {
			return err
		}
	}
	if err := r.keyring.SetComment(key.raw, value); err != nil {
		return err
	}
	return r.success(map[string]string{"name": key.Name, "comment": value}, "Key comment saved: "+key.Name)
}

func (r *Runner) keyExport(args []string) error {
	fs := newFlagSet("keys export")
	directory := fs.String("directory", "", "existing destination directory")
	yes := fs.Bool("yes", false, "acknowledge private-key export")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys export <name> --directory path [--yes]")
	}
	var err error
	if *directory == "" {
		*directory, err = r.prompt("Directory", "", true)
		if err != nil {
			return err
		}
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	if err := r.confirm("EXPORT", *yes); err != nil {
		return err
	}
	if err := r.keyring.Export(key.raw, *directory); err != nil {
		return err
	}
	return r.success(map[string]string{"name": key.Name, "directory": *directory}, "Key exported: "+key.Name)
}

func (r *Runner) keyInstall(args []string) error {
	fs := newFlagSet("keys install")
	hostRef := fs.String("host", "", "target host")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys install <name> --host host")
	}
	var err error
	if *hostRef == "" {
		*hostRef, err = r.prompt("Host", "", true)
		if err != nil {
			return err
		}
	}
	hosts, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	host, err := r.findHost(*hostRef, hosts)
	if err != nil {
		return err
	}
	public, err := r.keyring.PublicText(key.raw)
	if err != nil {
		return err
	}
	cmd, err := r.OpenSSH.InstallPublicKeyCommand(host.Alias, public)
	if err != nil {
		return err
	}
	if askpass.Needed(host.raw, r.Paths.PasswordsDir) {
		r.prepareSSH(cmd, host.raw)
	} else if r.JSON && len(cmd.Args) >= 2 {
		cmd.Args = append([]string{cmd.Args[0], "-o", "BatchMode=yes"}, cmd.Args[1:]...)
	}
	if err := r.runProcess(cmd, false); err != nil {
		return err
	}
	return r.success(map[string]string{"name": key.Name, "host": host.Alias}, "Public key installed: "+key.Name+" on "+host.Alias)
}

func (r *Runner) keyPassphrase(args []string) error {
	fs := newFlagSet("keys passphrase")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys passphrase <name>")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	cmd, err := r.keyring.PassphraseCommand(key.raw)
	if err != nil {
		return err
	}
	if err := r.runProcess(cmd, true); err != nil {
		return err
	}
	return r.success(map[string]string{"name": key.Name}, "Passphrase changed: "+key.Name)
}

func (r *Runner) keyPublic(args []string) error {
	fs := newFlagSet("keys public")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys public <name>")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	public, err := r.keyring.PublicText(key.raw)
	if err != nil {
		return err
	}
	if r.JSON {
		return r.success(map[string]string{"name": key.Name, "publicKey": public}, "")
	}
	fmt.Fprintln(r.Out, public)
	return nil
}

func (r *Runner) keyCopy(args []string) error {
	fs := newFlagSet("keys copy")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys copy <name>")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	public, err := r.keyring.PublicText(key.raw)
	if err != nil {
		return err
	}
	if err := clipboard.WriteAll(public); err != nil {
		return err
	}
	return r.success(map[string]string{"name": key.Name}, "Public key copied: "+key.Name)
}

func (r *Runner) keyDelete(args []string) error {
	fs := newFlagSet("keys delete")
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast keys delete <name> [--yes]")
	}
	_, records, err := r.snapshot()
	if err != nil {
		return err
	}
	key, err := findKey(fs.Arg(0), records)
	if err != nil {
		return err
	}
	if err := r.confirm(key.Name, *yes); err != nil {
		return err
	}
	if err := r.keyring.Delete(key.raw, key.Name); err != nil {
		return err
	}
	if key.Managed {
		if id := metadata.VaultKeyTombstoneID(key.Fingerprint, key.Name); id != "" {
			if err := r.store.RecordKeyTombstone(id); err != nil {
				return err
			}
		}
	}
	return r.success(map[string]string{"name": key.Name}, "Key permanently deleted: "+key.Name)
}
