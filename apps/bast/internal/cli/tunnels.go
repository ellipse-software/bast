package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"bast/internal/askpass"
	"bast/internal/metadata"
	"bast/internal/tunnel"
)

// tunnels manages named tunnels: forwards stored in Bast metadata that run
// through a host from the OpenSSH config. Each runs as its own background
// `bast tunnel run`, so closing the terminal does not close the tunnel.
func (r *Runner) tunnels(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, commandUsage("tunnel", "--help"))
		return nil
	}
	switch args[0] {
	case "list", "l":
		return r.tunnelList(args[1:])
	case "open":
		return r.tunnelOpenClose(true, args[1:])
	case "close":
		return r.tunnelOpenClose(false, args[1:])
	case "add":
		return r.tunnelSave(false, args[1:])
	case "edit":
		return r.tunnelSave(true, args[1:])
	case "remove", "delete":
		return r.tunnelRemove(args[1:])
	case "logs":
		return r.tunnelLogs(args[1:])
	case "run":
		return r.tunnelRun(args[1:])
	default:
		return usagef("unknown tunnel command %q", args[0])
	}
}

func (r *Runner) tunnelManager() (*tunnel.Manager, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fail("tunnel", err.Error())
	}
	m, err := tunnel.NewManager(tunnel.CacheDir(r.Paths.Home), exe)
	if err != nil {
		return nil, fail("tunnel", err.Error())
	}
	return m, nil
}

func specOf(t metadata.Tunnel) tunnel.Spec {
	return tunnel.Spec{Name: t.Name, Host: t.Host, Mode: t.Mode, Local: t.Local, Remote: t.Remote, Group: t.Group, KeepAlive: t.KeepAlive}
}

type tunnelRow struct {
	Name   string `json:"name"`
	Group  string `json:"group"`
	Host   string `json:"host"`
	Mode   string `json:"mode"`
	Local  string `json:"local"`
	Remote string `json:"remote"`
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
}

// interleaved parses flags that may come before or after positional names.
func interleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func (r *Runner) tunnelList(args []string) error {
	fs := newFlagSet("tunnel list")
	group := fs.String("group", "", "only this group")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return usagef("usage: bast tunnel list [--group name]")
	}
	mgr, err := r.tunnelManager()
	if err != nil {
		return err
	}
	rows := []tunnelRow{}
	for _, t := range r.store.Tunnels() {
		if *group != "" && t.Group != *group {
			continue
		}
		st := mgr.Status(t.Name)
		rows = append(rows, tunnelRow{t.Name, t.Group, t.Host, t.Mode, t.Local, t.Remote, st.State, st.Error})
	}
	if r.JSON {
		return r.success(rows, "")
	}
	for _, row := range rows {
		fmt.Fprintf(r.Out, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", row.Name, row.Group, row.Mode, row.Local, row.Remote, row.Host, row.State)
	}
	return nil
}

// selectTunnels resolves --all, --group and names or globs to stored tunnels.
func (r *Runner) selectTunnels(all bool, group string, patterns []string) ([]metadata.Tunnel, error) {
	if !all && group == "" && len(patterns) == 0 {
		return nil, usagef("name a tunnel, a glob like 'prod-*', or pass --all or --group")
	}
	stored := r.store.Tunnels()
	byName := map[string]metadata.Tunnel{}
	var names []string
	for _, t := range stored {
		byName[t.Name] = t
		if group == "" || t.Group == group {
			names = append(names, t.Name)
		}
	}
	chosen := names
	if !all && len(patterns) > 0 {
		var missing []string
		chosen, missing = tunnel.Match(patterns, names)
		if len(missing) > 0 {
			return nil, fail("not_found", "no tunnel matches "+strings.Join(missing, ", "))
		}
	} else if !all && group != "" && len(names) == 0 {
		return nil, fail("not_found", "no tunnels in group "+group)
	}
	out := make([]metadata.Tunnel, 0, len(chosen))
	for _, n := range chosen {
		out = append(out, byName[n])
	}
	return out, nil
}

func (r *Runner) tunnelOpenClose(open bool, args []string) error {
	fs := newFlagSet("tunnel")
	all := fs.Bool("all", false, "every tunnel")
	group := fs.String("group", "", "tunnels in this group")
	patterns, err := interleaved(fs, args)
	if err != nil {
		return usagef("%v", err)
	}
	picked, err := r.selectTunnels(*all, *group, patterns)
	if err != nil {
		return err
	}
	mgr, err := r.tunnelManager()
	if err != nil {
		return err
	}
	type result struct {
		Name   string `json:"name"`
		Result string `json:"result"`
	}
	var results []result
	var failed []string
	for _, t := range picked {
		if !open {
			was := mgr.Running(t.Name)
			mgr.Stop(t.Name)
			if was {
				results = append(results, result{t.Name, "closed"})
			} else {
				results = append(results, result{t.Name, "not running"})
			}
			continue
		}
		if err := r.ensureTunnelHost(t); err != nil {
			failed = append(failed, err.Error())
			continue
		}
		switch err := mgr.Start(specOf(t)); {
		case err == tunnel.ErrRunning:
			results = append(results, result{t.Name, "already running"})
		case err != nil:
			failed = append(failed, err.Error())
		default:
			results = append(results, result{t.Name, "opened"})
		}
	}
	if !r.JSON {
		for _, res := range results {
			fmt.Fprintf(r.Out, "%s: %s\n", res.Name, res.Result)
		}
	}
	if len(failed) > 0 {
		msg := strings.Join(failed, "\n")
		if r.JSON {
			return &commandError{
				code: "tunnel", message: msg, exit: 1,
				data: map[string]any{"results": results, "failed": failed},
			}
		}
		return fail("tunnel", msg)
	}
	if r.JSON {
		return r.success(results, "")
	}
	return nil
}

func (r *Runner) ensureTunnelHost(t metadata.Tunnel) error {
	hosts, _, err := r.snapshot()
	if err != nil {
		return err
	}
	record, err := r.findHost(t.Host, hosts)
	if err != nil {
		return fmt.Errorf("tunnel %q: host: %s is not in your SSH config", t.Name, t.Host)
	}
	return r.ensureJump(record)
}

func (r *Runner) tunnelFlags(name string) (*flag.FlagSet, *metadata.Tunnel) {
	fs := newFlagSet(name)
	t := &metadata.Tunnel{}
	fs.StringVar(&t.Host, "host", "", "SSH config host")
	fs.StringVar(&t.Mode, "mode", "", "local, remote, socks or socks-remote")
	fs.StringVar(&t.Local, "local", "", "local endpoint")
	fs.StringVar(&t.Remote, "remote", "", "remote endpoint")
	fs.StringVar(&t.Group, "group", "", "group name")
	fs.IntVar(&t.KeepAlive, "keep-alive", 0, "ServerAliveInterval seconds")
	return fs, t
}

func (r *Runner) tunnelSave(edit bool, args []string) error {
	verb := map[bool]string{false: "add", true: "edit"}[edit]
	fs, in := r.tunnelFlags("tunnel " + verb)
	pos, err := interleaved(fs, args)
	if err != nil || len(pos) != 1 {
		return usagef("usage: bast tunnel %s <name> --host alias --mode local|remote|socks|socks-remote [--local ..] [--remote ..] [--group g] [--keep-alive s]", verb)
	}
	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	t := metadata.Tunnel{Name: pos[0]}
	if edit {
		old, ok := r.store.Tunnel(pos[0])
		if !ok {
			return fail("not_found", fmt.Sprintf("no tunnel named %q", pos[0]))
		}
		t = old
	} else if t.Mode = in.Mode; t.Mode == "" {
		t.Mode = tunnel.ModeLocal
	}
	set := func(flag string, dst *string, v string) {
		if seen[flag] {
			*dst = v
		}
	}
	set("host", &t.Host, in.Host)
	set("mode", &t.Mode, in.Mode)
	set("local", &t.Local, in.Local)
	set("remote", &t.Remote, in.Remote)
	set("group", &t.Group, in.Group)
	if seen["keep-alive"] {
		t.KeepAlive = in.KeepAlive
	}
	if err := tunnel.Validate(specOf(t)); err != nil {
		return fail("invalid_tunnel", err.Error())
	}
	if err := r.store.PutTunnel(t, map[bool]string{true: t.Name}[edit]); err != nil {
		return fail("invalid_tunnel", err.Error())
	}
	if edit {
		if mgr, err := r.tunnelManager(); err == nil && mgr.Running(t.Name) {
			fmt.Fprintln(r.Err, "Saved. Close and open the tunnel to apply the change.")
		}
	}
	if r.JSON {
		return r.success(t, "")
	}
	fmt.Fprintln(r.Out, t.Name)
	return nil
}

func (r *Runner) tunnelRemove(args []string) error {
	fs := newFlagSet("tunnel remove")
	patterns, err := interleaved(fs, args)
	if err != nil || len(patterns) == 0 {
		return usagef("usage: bast tunnel remove <name>...")
	}
	picked, err := r.selectTunnels(false, "", patterns)
	if err != nil {
		return err
	}
	mgr, err := r.tunnelManager()
	if err != nil {
		return err
	}
	for _, t := range picked {
		mgr.Stop(t.Name)
		if err := r.store.DeleteTunnel(t.Name); err != nil {
			return fail("tunnel", err.Error())
		}
		if !r.JSON {
			fmt.Fprintln(r.Out, "removed "+t.Name)
		}
	}
	if r.JSON {
		return r.success(len(picked), "")
	}
	return nil
}

func (r *Runner) tunnelLogs(args []string) error {
	if len(args) != 1 {
		return usagef("usage: bast tunnel logs <name>")
	}
	mgr, err := r.tunnelManager()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(mgr.LogPath(args[0]))
	if err != nil {
		return fail("not_found", "no log for "+args[0])
	}
	if len(body) > 16384 {
		body = body[len(body)-16384:]
	}
	_, err = r.Out.Write(body)
	return err
}

// tunnelRun is the background runner started by `tunnel open`.
func (r *Runner) tunnelRun(args []string) error {
	if len(args) != 1 {
		return usagef("usage: bast tunnel run <name>")
	}
	t, ok := r.store.Tunnel(args[0])
	if !ok {
		return fail("not_found", fmt.Sprintf("no tunnel named %q", args[0]))
	}
	mgr, err := r.tunnelManager()
	if err != nil {
		return err
	}
	var configure func(*exec.Cmd)
	if hosts, _, err := r.snapshot(); err == nil {
		if record, err := r.findHost(t.Host, hosts); err == nil {
			configure = func(cmd *exec.Cmd) { askpass.Prepare(cmd, "", record.raw, r.Paths.PasswordsDir) }
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mgr.Run(ctx, specOf(t), r.OpenSSH.SSH, configure); err != nil {
		return fail("tunnel", err.Error())
	}
	return nil
}
