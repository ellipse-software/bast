package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/x/term"

	"bast/internal/keys"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
	"bast/internal/telemetry"
)

const help = `Bast: native SSH picker, key manager, and CLI

Usage:
  bast                         Open the TUI
  bast <label>                 Connect directly using a host label
  bast tui                     Open the TUI explicitly
  bast update                  Update script-installed copies of Bast
  bast connect <host>          Connect using an alias or display label
  bast hosts <command>         Manage SSH hosts
  bast keys <command>          Manage SSH keys
  bast sync <command>          Sync cloud VMs into Bast
  bast box <command>           Create and manage ASCII Box sandboxes
  bast upstash <command>       Create and manage Upstash Box sandboxes
  bast vault <command>         Sync Bast-managed config via encrypted vault
  bast completion <shell>      Print a shell completion script

Host commands:
  list, show, add, edit, delete, promote, favorite, unfavorite, hide, show-hidden,
  sort, known-host

Key commands:
  list, show, generate, import, promote, comment, export, install, passphrase,
  public, copy, delete

Sync commands:
  gcp, aws, azure, box, upstash, status, disable

Box commands:
  new, fork, stop, resume

Upstash commands:
  new, fork, stop, resume, delete, key

Vault commands:
  login, status, push, pull, logout, passphrase

Global options:
  --json                      Emit structured JSON
  --no-input                  Never prompt for missing input

Run "bast hosts <command> --help", "bast keys <command> --help", "bast sync <command> --help",
"bast box <command> --help", "bast upstash <command> --help", "bast vault <command> --help",
or "bast completion --help" for details.
`

func PrintHelp(out io.Writer) { fmt.Fprint(out, help) }

type Runner struct {
	Paths   paths.Paths
	OpenSSH openssh.Client
	Version string
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	JSON    bool
	NoInput bool

	config  sshconfig.Manager
	keyring keys.Manager
	store   *metadata.Store
	reader  *bufio.Reader
}

type reportedError struct{ code int }

func (e reportedError) Error() string { return "CLI error already reported" }
func ExitCode(err error) (int, bool) {
	var reported reportedError
	ok := errors.As(err, &reported)
	return reported.code, ok
}

type commandError struct {
	code    string
	message string
	exit    int
}

func (e *commandError) Error() string { return e.message }

func usagef(format string, args ...any) error {
	return &commandError{code: "usage", message: fmt.Sprintf(format, args...), exit: 2}
}
func fail(code, message string) error { return &commandError{code: code, message: message, exit: 1} }

func New(p paths.Paths, client openssh.Client, in io.Reader, out, errOut io.Writer) (*Runner, error) {
	return &Runner{
		Paths: p, OpenSSH: client, Version: "dev", In: in, Out: out, Err: errOut,
		config:  sshconfig.Manager{Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir, ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys, SyncGCPConfig: p.SyncGCPConfig, SyncAWSConfig: p.SyncAWSConfig, SyncAzureConfig: p.SyncAzureConfig, SyncBoxConfig: p.SyncBoxConfig, SyncUpstashConfig: p.SyncUpstashConfig},
		keyring: keys.Manager{Paths: p, SSHKeygen: client.SSHKeygen, SSHAdd: client.SSHAdd},
		reader:  bufio.NewReader(in),
	}, nil
}

func IsCommand(arg string) bool {
	switch arg {
	case "tui", "update", "connect", "hosts", "keys", "sync", "box", "upstash", "vault", "completion", "__complete":
		return true
	}
	return false
}

func IsInvocation(args []string) bool {
	globalOnly := false
	for _, arg := range args {
		if arg == "--json" || arg == "--no-input" {
			globalOnly = true
			continue
		}
		return IsCommand(arg) || arg == "-h" || arg == "--help"
	}
	return globalOnly
}

func (r *Runner) Run(args []string) error {
	if cmd, rest, ok := takeCompletionCommand(args); ok {
		if cmd == "__complete" {
			return r.completeQuery(rest)
		}
		return r.report(r.completion(rest))
	}
	args = r.globalFlags(args)
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		PrintHelp(r.Out)
		return nil
	}
	if len(args) >= 2 && (args[len(args)-1] == "-h" || args[len(args)-1] == "--help") {
		command := "--help"
		if len(args) >= 3 {
			command = args[1]
		}
		fmt.Fprintln(r.Out, commandUsage(args[0], command))
		return nil
	}
	var err error
	if args[0] == "update" {
		err = r.update(args[1:])
	} else {
		store, openErr := metadata.Open(r.Paths.StateFile)
		err = openErr
		if err == nil {
			r.store = store
			err = r.OpenSSH.Check()
		}
		if err == nil {
			switch args[0] {
			case "connect":
				err = r.connect(args[1:])
			case "hosts":
				err = r.hosts(args[1:])
			case "keys":
				err = r.keys(args[1:])
			case "sync":
				err = r.sync(args[1:])
			case "box":
				err = r.boxCmd(args[1:])
			case "upstash":
				err = r.upstashCmd(args[1:])
			case "vault":
				err = r.vault(args[1:])
			default:
				err = usagef("unknown command %q", args[0])
			}
		}
	}
	return r.report(err)
}

func (r *Runner) report(err error) error {
	if err == nil {
		return nil
	}
	ce := &commandError{code: "operation_failed", message: err.Error(), exit: 1}
	var typed *commandError
	if errors.As(err, &typed) {
		ce = typed
	}
	if r.JSON {
		_ = json.NewEncoder(r.Err).Encode(map[string]any{"ok": false, "error": map[string]string{"code": ce.code, "message": ce.message}})
	} else {
		fmt.Fprintln(r.Err, "bast:", ce.message)
		if !r.NoInput && telemetry.Enabled() {
			if in, ok := r.In.(*os.File); ok && term.IsTerminal(in.Fd()) {
				telemetry.OfferReport(r.In, r.Err, telemetry.Report{
					Message: ce.message,
					Version: r.Version,
					Code:    ce.code,
					Context: "cli",
				})
			}
		}
	}
	return reportedError{code: ce.exit}
}

func commandUsage(resource, command string) string {
	usage := map[string]string{
		"update --help": "Usage: bast update",
		"hosts --help": `Usage: bast hosts <command>

Commands: list, show, add, edit, delete, promote, favorite, unfavorite, hide,
          show-hidden, sort, known-host`,
		"hosts list": "Usage: bast hosts list [--search text] [--sort smart|label|recent|group] [--all]",
		"hosts show": "Usage: bast hosts show <host>",
		"hosts add": `Usage: bast hosts add [label] --hostname host [options]

Connection: --user, --port, --identity, --password-only, --proxy-jump
Advanced: --forward-agent, --startup-command, --request-tty, --set-env,
          --local-forward, --remote-forward, --dynamic-forward, --compression,
          --keepalive, --ssh-option
Metadata: label paths like Work/api set the group; or --group, --tag, --environment, --color, --notes`,
		"hosts edit": `Usage: bast hosts edit <host> [options]

Connection: --label, --hostname, --user, --port, --identity, --password-only,
            --proxy-jump
Advanced: --forward-agent, --startup-command, --request-tty, --set-env,
          --local-forward, --remote-forward, --dynamic-forward, --compression,
          --keepalive, --ssh-option
Metadata: --label paths like Work/api set the group; or --group, --tag,
          --environment, --color, --notes
Repeat list options to provide multiple values. Use the corresponding --clear-*
option to restore a default or remove values.`,
		"hosts delete":  "Usage: bast hosts delete <host> [--yes]",
		"hosts promote": "Usage: bast hosts promote <host>",
		"keys --help": `Usage: bast keys <command>

Commands: list, show, generate, import, promote, comment, export, install,
          passphrase, public, copy, delete`,
		"keys list":         "Usage: bast keys list [--search text]",
		"keys show":         "Usage: bast keys show <name>",
		"keys generate":     "Usage: bast keys generate [name] [--algorithm ed25519|rsa] [--no-passphrase]",
		"keys import":       "Usage: bast keys import [name] --private path|- [--public path|-] [--comment text]",
		"keys promote":      "Usage: bast keys promote <key> [--name managed-name]",
		"keys comment":      "Usage: bast keys comment <name> (--comment text|--clear-comment)",
		"keys export":       "Usage: bast keys export <name> --directory path [--yes]",
		"keys install":      "Usage: bast keys install <name> --host host",
		"keys passphrase":   "Usage: bast keys passphrase <name>",
		"keys public":       "Usage: bast keys public <name>",
		"keys copy":         "Usage: bast keys copy <name>",
		"keys delete":       "Usage: bast keys delete <name> [--yes]",
		"connect --help":    "Usage: bast connect <host>",
		"sync gcp":          "Usage: bast sync gcp",
		"sync aws":          "Usage: bast sync aws",
		"sync azure":        "Usage: bast sync azure",
		"sync box":          "Usage: bast sync box",
		"sync upstash":      "Usage: bast sync upstash",
		"sync status":       "Usage: bast sync status",
		"sync disable":      "Usage: bast sync disable <gcp|aws|azure|box|upstash>",
		"sync --help":       "Usage: bast sync <gcp|aws|azure|box|upstash|status|disable>",
		"box --help":        "Usage: bast box <new|fork|stop|resume>",
		"box new":           "Usage: bast box new [--type small|default|large] [--ttl seconds | --no-auto-stop] [--no-env]",
		"box fork":          "Usage: bast box fork <host|id> [--type small|default|large] [--no-env]",
		"box stop":          "Usage: bast box stop <host|id>",
		"box resume":        "Usage: bast box resume <host|id> [--type small|default|large] [--no-env]",
		"upstash --help":    "Usage: bast upstash <new|fork|stop|resume|delete|key>",
		"upstash new":       "Usage: bast upstash new [--name name] [--runtime node|python|golang|ruby|rust] [--size small|medium|large] [--keep-alive]",
		"upstash fork":      "Usage: bast upstash fork <host|id>",
		"upstash stop":      "Usage: bast upstash stop <host|id>",
		"upstash resume":    "Usage: bast upstash resume <host|id>",
		"upstash delete":    "Usage: bast upstash delete <host|id> [--yes]",
		"upstash key":       "Usage: bast upstash key [--key-file path]",
		"vault --help":      "Usage: bast vault <login|status|push|pull|logout|passphrase>",
		"vault login":       "Usage: bast vault login [--email address] [--api url] [--accept-terms] [--mode merge|replace_local|replace_remote]",
		"vault status":      "Usage: bast vault status",
		"vault push":        "Usage: bast vault push",
		"vault pull":        "Usage: bast vault pull [--mode merge|replace_local|replace_remote]",
		"vault logout":      "Usage: bast vault logout",
		"vault passphrase":  "Usage: bast vault passphrase [--force]",
		"completion --help": strings.TrimSuffix(completionUsage, "\n"),
	}
	if value := usage[resource+" "+command]; value != "" {
		return value
	}
	return "Usage: bast " + resource + " " + command + " [options]"
}

func (r *Runner) globalFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--json":
			r.JSON, r.NoInput = true, true
		case "--no-input":
			r.NoInput = true
		default:
			out = append(out, arg)
		}
	}
	return out
}

func (r *Runner) success(data any, message string) error {
	if r.JSON {
		return json.NewEncoder(r.Out).Encode(map[string]any{"ok": true, "data": data})
	}
	if message != "" {
		fmt.Fprintln(r.Out, message)
	}
	return nil
}

type hostRecord struct {
	Alias           string                     `json:"alias"`
	Label           string                     `json:"label"`
	Hostname        string                     `json:"hostname"`
	User            string                     `json:"user"`
	Port            string                     `json:"port"`
	IdentityFiles   []string                   `json:"identityFiles"`
	Authentication  string                     `json:"authentication"`
	ProxyJump       string                     `json:"proxyJump"`
	Advanced        sshconfig.AdvancedSettings `json:"advanced"`
	Group           string                     `json:"group"`
	Tags            []string                   `json:"tags"`
	Environment     string                     `json:"environment"`
	Color           string                     `json:"color"`
	Notes           string                     `json:"notes"`
	Favorite        bool                       `json:"favorite"`
	Hidden          bool                       `json:"hidden"`
	Managed         bool                       `json:"managed"`
	Synced          bool                       `json:"synced"`
	SyncSource      string                     `json:"syncSource,omitempty"`
	SyncID          string                     `json:"syncId,omitempty"`
	Source          string                     `json:"source"`
	KnownHost       bool                       `json:"knownHost"`
	ResolveError    string                     `json:"resolveError,omitempty"`
	LastUsedAt      *time.Time                 `json:"lastUsedAt,omitempty"`
	ConnectionCount int                        `json:"connectionCount"`
	raw             sshconfig.Host
	meta            metadata.Host
}

type keyRecord struct {
	Name        string   `json:"name"`
	Algorithm   string   `json:"algorithm"`
	Fingerprint string   `json:"fingerprint"`
	Comment     string   `json:"comment"`
	PrivatePath string   `json:"privatePath"`
	PublicPath  string   `json:"publicPath"`
	Managed     bool     `json:"managed"`
	InAgent     bool     `json:"inAgent"`
	References  []string `json:"references"`
	raw         keys.Key
}

func (r *Runner) load(ctx context.Context) ([]hostRecord, []keyRecord, error) {
	hosts, err := r.config.Discover()
	if err != nil {
		return nil, nil, err
	}
	referenced := map[string][]string{}
	hostRecords := make([]hostRecord, len(hosts))
	type enrichResult struct {
		index      int
		resolved   sshconfig.Resolved
		known      bool
		identities []string
		err        error
	}
	jobs := make(chan int)
	results := make(chan enrichResult, len(hosts))
	var workers sync.WaitGroup
	workerCount := min(8, len(hosts))
	if workerCount == 0 {
		workerCount = 1
	}
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				host := hosts[i]
				res := enrichResult{index: i}
				if host.Resolved.HostName != "" {
					res.resolved = host.Resolved
					res.identities = host.Resolved.IdentityFiles
				} else {
					resolved, resolveErr := r.OpenSSH.Resolve(ctx, host.Alias)
					if resolveErr != nil {
						res.err = resolveErr
						results <- res
						continue
					}
					res.resolved = resolved
					res.identities = resolved.IdentityFiles
				}
				known, _ := r.OpenSSH.Fingerprints(ctx, res.resolved.HostName, res.resolved.Port)
				res.known = known != ""
				results <- res
			}
		}()
	}
	go func() {
		for i := range hosts {
			jobs <- i
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	timedOut := false
	resolveErrors := make([]string, len(hosts))
	for res := range results {
		if res.err != nil {
			if ctx.Err() != nil {
				timedOut = true
			} else {
				resolveErrors[res.index] = res.err.Error()
			}
			continue
		}
		hosts[res.index].Resolved = res.resolved
		hosts[res.index].KnownHost = res.known
		for _, identity := range res.identities {
			referenced[identity] = append(referenced[identity], hosts[res.index].Alias)
		}
	}
	if timedOut || ctx.Err() != nil {
		return nil, nil, fail("timeout", "timed out resolving SSH hosts; retry or reduce host count")
	}

	for i := range hosts {
		meta := r.store.Host(hosts[i].Alias)
		label := strings.TrimSpace(meta.Label)
		if label == "" {
			label = hosts[i].Alias
		}
		resolved := hosts[i].Resolved
		auth := "default"
		if isPasswordOnly(resolved) {
			auth = "password"
		} else if (strings.EqualFold(resolved.IdentitiesOnly, "yes") || strings.EqualFold(resolved.IdentitiesOnly, "true")) && len(resolved.IdentityFiles) > 0 {
			auth = "identity"
		}
		adv, advErr := loadHostAdvanced(r.config, hosts[i], emptyNone(resolved.ProxyJump))
		if advErr != nil {
			return nil, nil, advErr
		}
		hostRecords[i] = hostRecord{
			Alias: hosts[i].Alias, Label: label, Hostname: resolved.HostName, User: resolved.User, Port: resolved.Port,
			IdentityFiles: nonNil(resolved.IdentityFiles), Authentication: auth, ProxyJump: emptyNone(resolved.ProxyJump), Advanced: adv,
			Group: meta.Group,
			Tags:  nonNil(meta.Tags), Environment: meta.Environment, Color: meta.Color, Notes: meta.Notes, Favorite: meta.Favorite,
			Hidden: meta.Hidden, Managed: hosts[i].Managed, Synced: hosts[i].Synced, SyncSource: hosts[i].SyncSource,
			SyncID: hosts[i].SyncID, Source: hosts[i].Source, KnownHost: hosts[i].KnownHost, ResolveError: resolveErrors[i],
			LastUsedAt: meta.LastUsedAt, ConnectionCount: meta.ConnectionCount, raw: hosts[i], meta: meta,
		}
	}
	keyList, err := r.keyring.Discover(ctx, referenced)
	if err != nil {
		return nil, nil, err
	}
	if ctx.Err() != nil {
		return nil, nil, fail("timeout", "timed out resolving SSH hosts; retry or reduce host count")
	}
	keyRecords := make([]keyRecord, 0, len(keyList))
	for _, key := range keyList {
		keyRecords = append(keyRecords, keyRecord{Name: key.Name, Algorithm: key.Algorithm, Fingerprint: key.Fingerprint, Comment: key.Comment, PrivatePath: key.PrivatePath, PublicPath: key.PublicPath, Managed: key.Managed, InAgent: key.InAgent, References: nonNil(key.References), raw: key})
	}
	return hostRecords, keyRecords, nil
}

func (r *Runner) snapshot() ([]hostRecord, []keyRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return r.load(ctx)
}

func (r *Runner) findHost(ref string, hosts []hostRecord) (hostRecord, error) {
	for _, host := range hosts {
		if host.Alias == ref {
			return host, nil
		}
	}
	normalized := sshconfig.NormalizeAlias(ref)
	if normalized != ref {
		for _, host := range hosts {
			if host.Alias == normalized {
				return host, nil
			}
		}
	}
	matches := []hostRecord{}
	for _, host := range hosts {
		if host.Label == ref {
			matches = append(matches, host)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return hostRecord{}, fail("conflict", fmt.Sprintf("host label %q is ambiguous; use an SSH alias", ref))
	}
	return hostRecord{}, fail("not_found", fmt.Sprintf("unknown host %q", ref))
}

func findKey(name string, records []keyRecord) (keyRecord, error) {
	for _, key := range records {
		if key.Name == name {
			return key, nil
		}
	}
	return keyRecord{}, fail("not_found", fmt.Sprintf("unknown key %q", name))
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func isPasswordOnly(resolved sshconfig.Resolved) bool {
	pubkey, password := strings.ToLower(resolved.PubkeyAuthentication), strings.ToLower(resolved.PasswordAuthentication)
	return (pubkey == "no" || pubkey == "false") && (password == "yes" || password == "true")
}
func emptyNone(value string) string {
	if value == "none" {
		return ""
	}
	return value
}
func emptyDefault(value string) string {
	if value == "" {
		return "default"
	}
	return value
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func positionalFirst(args []string) []string {
	if len(args) > 1 && !strings.HasPrefix(args[0], "-") {
		return append(append([]string{}, args[1:]...), args[0])
	}
	return args
}

type stringsFlag []string

func (s *stringsFlag) String() string         { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(value string) error { *s = append(*s, value); return nil }

type optionalString struct {
	value string
	set   bool
}

func (s *optionalString) String() string         { return s.value }
func (s *optionalString) Set(value string) error { s.value, s.set = value, true; return nil }

func (r *Runner) prompt(label, current string, required bool) (string, error) {
	if r.NoInput || !r.interactive() {
		if required {
			return "", fail("input_required", label+" is required")
		}
		return current, nil
	}
	if current == "" {
		fmt.Fprintf(r.Err, "%s: ", label)
	} else {
		fmt.Fprintf(r.Err, "%s [%s]: ", label, current)
	}
	if r.reader == nil {
		r.reader = bufio.NewReader(r.In)
	}
	line, err := r.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = current
	}
	if required && line == "" {
		return "", fail("input_required", label+" is required")
	}
	return line, nil
}

func (r *Runner) interactive() bool {
	file, ok := r.In.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (r *Runner) confirm(target string, yes bool) error {
	if yes {
		return nil
	}
	if r.NoInput || !r.interactive() {
		return fail("confirmation_required", "confirmation required; pass --yes")
	}
	value, err := r.prompt("Type "+target+" to confirm", "", true)
	if err != nil {
		return err
	}
	if value != target {
		return fail("confirmation_failed", "confirmation did not match")
	}
	return nil
}

func (r *Runner) runProcess(cmd *exec.Cmd, interactiveOnly bool) error {
	if r.JSON && interactiveOnly {
		return fail("interactive_required", "this command requires an interactive terminal and does not support --json")
	}
	if cmd.Stdin == nil || cmd.Stdin == os.Stdin {
		cmd.Stdin = r.In
	}
	if r.JSON {
		cmd.Stdout, cmd.Stderr = r.Err, r.Err
	} else {
		cmd.Stdout, cmd.Stderr = r.Out, r.Err
	}
	err := cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
		return &commandError{code: "process_failed", message: openssh.FormatError(err), exit: exitErr.ExitCode()}
	}
	if errors.As(err, &exitErr) {
		return &commandError{code: "process_failed", message: openssh.FormatError(err), exit: 1}
	}
	return err
}

func printHostTable(out io.Writer, hosts []hostRecord) {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ALIAS\tLABEL\tDESTINATION\tGROUP\tFLAGS")
	for _, host := range hosts {
		destination := host.Hostname
		if host.User != "" {
			destination = host.User + "@" + destination
		}
		if host.Port != "" && host.Port != "22" {
			destination += ":" + host.Port
		}
		flags := []string{}
		if host.Favorite {
			flags = append(flags, "favorite")
		}
		if host.Hidden {
			flags = append(flags, "hidden")
		}
		if !host.Managed {
			flags = append(flags, "external")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", host.Alias, host.Label, destination, host.Group, strings.Join(flags, ","))
	}
	_ = w.Flush()
}

func printKeyTable(out io.Writer, records []keyRecord) {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tALGORITHM\tFINGERPRINT\tREFERENCES\tFLAGS")
	for _, key := range records {
		flags := []string{}
		if key.Managed {
			flags = append(flags, "managed")
		}
		if key.InAgent {
			flags = append(flags, "agent")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", key.Name, key.Algorithm, key.Fingerprint, strings.Join(key.References, ","), strings.Join(flags, ","))
	}
	_ = w.Flush()
}

func sortHosts(records []hostRecord, order string) error {
	if order == "" {
		order = "smart"
	}
	if order == "label" {
		order = "alias"
	}
	if order != "smart" && order != "alias" && order != "recent" && order != "group" {
		return usagef("sort must be smart, label, recent, or group")
	}
	sort.SliceStable(records, func(i, j int) bool {
		a, b := records[i], records[j]
		switch order {
		case "alias":
			return strings.ToLower(a.Label) < strings.ToLower(b.Label)
		case "recent":
			return later(a.LastUsedAt, b.LastUsedAt)
		case "group":
			if a.Group != b.Group {
				return strings.ToLower(a.Group) < strings.ToLower(b.Group)
			}
		}
		if a.Favorite != b.Favorite {
			return a.Favorite
		}
		if (a.LastUsedAt != nil) != (b.LastUsedAt != nil) {
			return a.LastUsedAt != nil
		}
		if a.LastUsedAt != nil && !a.LastUsedAt.Equal(*b.LastUsedAt) {
			return a.LastUsedAt.After(*b.LastUsedAt)
		}
		return strings.ToLower(a.Label) < strings.ToLower(b.Label)
	})
	return nil
}

func later(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}
