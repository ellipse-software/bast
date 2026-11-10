package cli

import (
	"context"
	"flag"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"bast/internal/askpass"
	"path/filepath"

	"bast/internal/cloud/sync"
	"bast/internal/connectbanner"
	"bast/internal/hostlife"
	"bast/internal/metadata"
	"bast/internal/page"
	"bast/internal/rdp"
	"bast/internal/sshconfig"
	"bast/internal/tunnel"
)

func (r *Runner) rdp(args []string) error {
	return r.entries(metadata.EntryRDP, args)
}

func (r *Runner) web(args []string) error {
	return r.entries(metadata.EntryWeb, args)
}

func (r *Runner) entries(kind string, args []string) error {
	name := "rdp"
	if kind == metadata.EntryWeb {
		name = "web"
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintf(r.Out, "Usage: bast %s <list|add|edit|delete|open|close>\n", name)
		return nil
	}
	switch args[0] {
	case "list":
		return r.entryList(kind, args[1:])
	case "add":
		return r.entryAdd(kind, args[1:])
	case "edit":
		return r.entryEdit(kind, args[1:])
	case "delete":
		return r.entryDelete(kind, args[1:])
	case "open":
		return r.entryOpen(kind, args[1:])
	case "close":
		return r.entryClose(kind, args[1:])
	default:
		return usagef("unknown %s command %q", name, args[0])
	}
}

func (r *Runner) entryList(kind string, args []string) error {
	fs := newFlagSet("list")
	search := fs.String("search", "", "filter entries")
	all := fs.Bool("all", false, "include hidden entries")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast %s list [--search text] [--all]", kindCommand(kind))
	}
	entries := r.filterEntries(kind, *search, *all)
	if r.JSON {
		return r.success(entries, "")
	}
	if len(entries) == 0 {
		return nil
	}
	for _, entry := range entries {
		target := entry.URL
		if entry.Kind == metadata.EntryRDP {
			target = entry.Address
			if entry.Port != "" {
				target += ":" + entry.Port
			}
		}
		fmt.Fprintf(r.Out, "%s\t%s\t%s\n", entry.Label, target, entry.Via)
	}
	return nil
}

func (r *Runner) entryAdd(kind string, args []string) error {
	fs, opts := newEntryFlags(kindCommand(kind) + " add")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	label := strings.TrimSpace(opts.labelFlag)
	if fs.NArg() == 1 {
		if label != "" {
			return usagef("pass the label once")
		}
		label = fs.Arg(0)
	} else if fs.NArg() > 1 {
		return usagef("usage: bast %s add [label] [options]", kindCommand(kind))
	}
	if label == "" {
		return usagef("label is required")
	}
	entry := metadata.Entry{
		Kind: kind, Label: label, Group: opts.group, Environment: opts.environment,
		Icon: opts.icon, Color: opts.color, Notes: opts.notes, Via: opts.via,
		Tags: splitTags(opts.tags), Address: opts.address, Port: opts.port,
		User: opts.user, Domain: opts.domain, URL: opts.url,
		Fullscreen: opts.fullscreen, Admin: opts.admin,
	}
	if kind == metadata.EntryRDP && strings.TrimSpace(entry.Address) == "" {
		return usagef("--address is required")
	}
	if kind == metadata.EntryWeb && strings.TrimSpace(entry.URL) == "" {
		return usagef("--url is required")
	}
	saved, err := r.store.PutEntry(entry)
	if err != nil {
		return fail("invalid_entry", err.Error())
	}
	if r.JSON {
		return r.success(saved, "")
	}
	fmt.Fprintln(r.Out, saved.Label)
	return nil
}

func (r *Runner) entryEdit(kind string, args []string) error {
	fs, opts := newEntryFlags(kindCommand(kind) + " edit")
	clearVia := fs.Bool("clear-via", false, "remove the jump host")
	clearUser := fs.Bool("clear-user", false, "remove the username")
	clearDomain := fs.Bool("clear-domain", false, "remove the domain")
	clearPort := fs.Bool("clear-port", false, "use the default port")
	noFullscreen := fs.Bool("no-fullscreen", false, "open in a window")
	noAdmin := fs.Bool("no-admin", false, "use a normal session")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast %s edit <label> [options]", kindCommand(kind))
	}
	entry, err := r.store.EntryByLabel(kind, fs.Arg(0))
	if err != nil {
		return fail("not_found", err.Error())
	}
	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	if seen["label"] {
		entry.Label = opts.labelFlag
	}
	if seen["group"] {
		entry.Group = opts.group
	}
	if seen["tags"] {
		entry.Tags = splitTags(opts.tags)
	}
	if seen["environment"] {
		entry.Environment = opts.environment
	}
	if seen["icon"] {
		entry.Icon = opts.icon
	}
	if seen["color"] {
		entry.Color = opts.color
	}
	if seen["notes"] {
		entry.Notes = opts.notes
	}
	if seen["via"] && *clearVia {
		return usagef("pass either --via or --clear-via")
	}
	if *clearVia {
		entry.Via = ""
	} else if seen["via"] {
		entry.Via = opts.via
	}
	if kind == metadata.EntryRDP {
		if seen["address"] {
			entry.Address = opts.address
		}
		if seen["port"] && *clearPort {
			return usagef("pass either --port or --clear-port")
		}
		if *clearPort {
			entry.Port = ""
		} else if seen["port"] {
			entry.Port = opts.port
		}
		if seen["user"] && *clearUser {
			return usagef("pass either --user or --clear-user")
		}
		if *clearUser {
			entry.User = ""
		} else if seen["user"] {
			entry.User = opts.user
		}
		if seen["domain"] && *clearDomain {
			return usagef("pass either --domain or --clear-domain")
		}
		if *clearDomain {
			entry.Domain = ""
		} else if seen["domain"] {
			entry.Domain = opts.domain
		}
		if opts.fullscreen && *noFullscreen {
			return usagef("pass either --fullscreen or --no-fullscreen")
		}
		if opts.fullscreen {
			entry.Fullscreen = true
		}
		if *noFullscreen {
			entry.Fullscreen = false
		}
		if opts.admin && *noAdmin {
			return usagef("pass either --admin or --no-admin")
		}
		if opts.admin {
			entry.Admin = true
		}
		if *noAdmin {
			entry.Admin = false
		}
	} else if seen["url"] {
		entry.URL = opts.url
	}
	saved, err := r.store.PutEntry(entry)
	if err != nil {
		return fail("invalid_entry", err.Error())
	}
	if r.JSON {
		return r.success(saved, "")
	}
	fmt.Fprintln(r.Out, saved.Label)
	return nil
}

func (r *Runner) entryDelete(kind string, args []string) error {
	fs := newFlagSet("delete")
	yes := fs.Bool("yes", false, "skip confirmation")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast %s delete <label> [--yes]", kindCommand(kind))
	}
	entry, err := r.store.EntryByLabel(kind, fs.Arg(0))
	if err != nil {
		return fail("not_found", err.Error())
	}
	if err := r.confirm(entry.Label, *yes); err != nil {
		return err
	}
	r.closeEntryTunnel(entry)
	if err := r.store.DeleteEntry(entry.ID); err != nil {
		return err
	}
	if r.JSON {
		return r.success(map[string]string{"id": entry.ID, "label": entry.Label}, "")
	}
	fmt.Fprintln(r.Out, "Deleted "+entry.Label)
	return nil
}

func (r *Runner) entryOpen(kind string, args []string) error {
	fs := newFlagSet("open")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast %s open <label>", kindCommand(kind))
	}
	entry, err := r.store.EntryByLabel(kind, fs.Arg(0))
	if err != nil {
		return fail("not_found", err.Error())
	}
	return r.openStoredEntry(entry)
}

func (r *Runner) entryClose(kind string, args []string) error {
	fs := newFlagSet("close")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast %s close <label>", kindCommand(kind))
	}
	entry, err := r.store.EntryByLabel(kind, fs.Arg(0))
	if err != nil {
		return fail("not_found", err.Error())
	}
	if entry.Via == "" && entry.Kind == metadata.EntryWeb {
		return fail("no_tunnel", entry.Label+" has no tunnel")
	}
	r.closeEntryTunnel(entry)
	if r.JSON {
		return r.success(map[string]string{"id": entry.ID}, "")
	}
	fmt.Fprintln(r.Out, "Closed tunnel for "+entry.Label)
	return nil
}

func (r *Runner) openStoredEntry(entry metadata.Entry) error {
	var via sshconfig.Host
	if entry.Via != "" {
		hosts, _, err := r.snapshot()
		if err != nil {
			return err
		}
		record, err := r.findHost(entry.Via, hosts)
		if err != nil {
			return fail("not_found", "Jump host "+entry.Via+" is gone. Edit the entry and choose another.")
		}
		if record.SyncSource == "vercel" {
			return fail("vercel_tunnel", "Vercel sandboxes can't tunnel Remote Desktop or web pages")
		}
		if hostlife.LooksStopped(record.raw, record.meta) {
			return fail("jump_stopped", "Start "+record.Label+" first")
		}
		if err := r.ensureJump(record); err != nil {
			return err
		}
		via = record.raw
	}
	switch entry.Kind {
	case metadata.EntryRDP:
		return r.openRDPEntry(entry, via)
	case metadata.EntryWeb:
		return r.openWebEntry(entry, via)
	default:
		return fail("invalid_entry", "that entry can't be opened")
	}
}

func (r *Runner) openRDPEntry(entry metadata.Entry, via sshconfig.Host) error {
	session := rdp.FileSession{
		Address: entry.Address, Port: entry.Port, User: entry.User, Domain: entry.Domain,
		Fullscreen: entry.Fullscreen, Admin: entry.Admin,
	}
	if entry.Via == "" {
		if _, err := rdp.StartFile(session); err != nil {
			return fail("rdp_open", err.Error())
		}
		_ = r.store.RecordEntryUse(entry.ID)
		return r.opened(entry)
	}
	sup, err := r.entryTunnels()
	if err != nil {
		return err
	}
	target, err := rdp.DialTarget(entry.Address, entry.Port)
	if err != nil {
		return fail("invalid_entry", err.Error())
	}
	port, _, err := sup.RDP(entry.ID, entry.Via, target, func(cmd *exec.Cmd) {
		askpass.Prepare(cmd, "", via, r.Paths.PasswordsDir)
	})
	if err != nil {
		return fail("tunnel", err.Error())
	}
	session.Address = "127.0.0.1"
	session.Port = fmt.Sprintf("%d", port)
	if _, err := rdp.StartFile(session); err != nil {
		sup.CloseRDP(entry.ID)
		return fail("rdp_open", err.Error())
	}
	sup.ReleaseRDP(entry.ID)
	_ = r.store.RecordEntryUse(entry.ID)
	return r.opened(entry)
}

func (r *Runner) openWebEntry(entry metadata.Entry, via sshconfig.Host) error {
	if entry.Via == "" {
		if err := page.Open(entry.URL); err != nil {
			return fail("web_open", err.Error())
		}
		_ = r.store.RecordEntryUse(entry.ID)
		return r.opened(entry)
	}
	sup, err := r.entryTunnels()
	if err != nil {
		return err
	}
	port, _, err := sup.SOCKS(entry.Via, func(cmd *exec.Cmd) {
		askpass.Prepare(cmd, "", via, r.Paths.PasswordsDir)
	})
	if err != nil {
		return fail("tunnel", err.Error())
	}
	if err := page.OpenProxied(tunnel.CacheDir(r.Paths.Home), entry.Via, port, entry.URL); err != nil {
		return fail("web_open", err.Error())
	}
	sup.ReleaseSOCKS(entry.Via)
	_ = r.store.RecordEntryUse(entry.ID)
	return r.opened(entry)
}

func (r *Runner) opened(entry metadata.Entry) error {
	if r.JSON {
		return r.success(entry, "")
	}
	fmt.Fprintln(r.Out, "Opened "+entry.Label)
	return nil
}

func (r *Runner) closeEntryTunnel(entry metadata.Entry) {
	sup, err := r.entryTunnels()
	if err != nil {
		return
	}
	if entry.Kind == metadata.EntryWeb {
		sup.CloseSOCKS(entry.Via)
		return
	}
	sup.CloseRDP(entry.ID)
}

func (r *Runner) entryTunnels() (*tunnel.Supervisor, error) {
	sup, err := tunnel.Open(filepath.Join(tunnel.CacheDir(r.Paths.Home), "tunnels"), r.OpenSSH.SSH)
	if err != nil {
		return nil, fail("tunnel", err.Error())
	}
	return sup, nil
}

func (r *Runner) ensureJump(host hostRecord) error {
	if !host.Synced || host.SyncID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	engine := sync.New(r.Paths, r.store)
	status := connectbanner.Status(r.Out)
	if r.JSON {
		status = func(string) {}
	}
	var err error
	switch host.SyncSource {
	case "gcp":
		err = engine.EnsureGCPAccess(ctx, host.raw, status)
	case "aws":
		err = engine.EnsureAWSAccess(ctx, host.raw, status)
	case "azure":
		err = engine.EnsureAzureAccess(ctx, host.raw, status)
	case "boat":
		err = engine.EnsureBoatAccess(ctx, host.raw, status)
	case "upstash":
		err = engine.EnsureUpstashAccess(ctx, host.raw, status)
	case "hetzner":
		err = engine.EnsureHetznerAccess(ctx, host.raw, status)
	case "bitqate":
		err = engine.EnsureBitqateAccess(ctx, host.raw, status)
	case "railway":
		err = engine.EnsureRailwayAccess(ctx, host.raw, status)
	case "fly":
		err = engine.EnsureFlyAccess(ctx, host.raw, status)
	case "digitalocean":
		err = engine.EnsureDigitalOceanAccess(ctx, host.raw, status)
	case "boxd":
		err = engine.EnsureBoxdAccess(ctx, host.raw, status)
	}
	if err != nil {
		return fail(host.SyncSource+"_access", err.Error())
	}
	return nil
}

func (r *Runner) filterEntries(kind, search string, all bool) []metadata.Entry {
	q := strings.ToLower(strings.TrimSpace(search))
	var out []metadata.Entry
	for _, entry := range r.store.Entries() {
		if entry.Kind != kind {
			continue
		}
		if entry.Hidden && !all {
			continue
		}
		hay := strings.ToLower(strings.Join([]string{entry.Label, entry.Address, entry.URL, entry.Via, entry.Group, entry.Notes}, " "))
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

type entryFlagSet struct {
	labelFlag, group, tags, environment, icon, color, notes string
	via, address, port, user, domain, url                   string
	fullscreen, admin                                       bool
}

func newEntryFlags(name string) (*flag.FlagSet, *entryFlagSet) {
	fs := newFlagSet(name)
	opts := &entryFlagSet{}
	fs.StringVar(&opts.labelFlag, "label", "", "display label")
	fs.StringVar(&opts.group, "group", "", "group path")
	fs.StringVar(&opts.tags, "tags", "", "comma-separated tags")
	fs.StringVar(&opts.environment, "environment", "", "environment name")
	fs.StringVar(&opts.icon, "icon", "", "one emoji or flag")
	fs.StringVar(&opts.color, "color", "", "theme colour or hex")
	fs.StringVar(&opts.notes, "notes", "", "short note")
	fs.StringVar(&opts.via, "via", "", "SSH host to tunnel through")
	fs.StringVar(&opts.address, "address", "", "Remote Desktop address")
	fs.StringVar(&opts.port, "port", "", "Remote Desktop port")
	fs.StringVar(&opts.user, "user", "", "Remote Desktop username")
	fs.StringVar(&opts.domain, "domain", "", "Remote Desktop domain")
	fs.StringVar(&opts.url, "url", "", "http or https URL")
	fs.BoolVar(&opts.fullscreen, "fullscreen", false, "open full screen")
	fs.BoolVar(&opts.admin, "admin", false, "admin session")
	return fs, opts
}

func splitTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func kindCommand(kind string) string {
	if kind == metadata.EntryWeb {
		return "web"
	}
	return "rdp"
}
