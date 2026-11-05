package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	boxdcloud "bast/internal/cloud/boxd"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) boxdCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast boxd <new|fork|stop|start|restart|delete>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.boxdNew(engine, args[1:])
	case "fork":
		return r.boxdFork(engine, args[1:])
	case "stop":
		return r.boxdLifecycle(engine, "stop", args[1:])
	case "start", "resume":
		return r.boxdLifecycle(engine, "start", args[1:])
	case "restart":
		return r.boxdLifecycle(engine, "restart", args[1:])
	case "delete":
		return r.boxdDelete(engine, args[1:])
	default:
		return usagef("unknown boxd command %q", args[0])
	}
}

func (r *Runner) boxdNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boxd new")
	name := fs.String("name", "", "Computer name (generated when blank)")
	org := fs.String("org", "", "boxd org name or id")
	region := fs.String("region", "", "Region, for example fra or nyc")
	vcpu := fs.Int("vcpu", 0, "vCPUs: 1, 2, 4, or 8 (default: the org's size; not with --snapshot)")
	snapshot := fs.String("snapshot", "", "Snapshot to create the computer from")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast boxd new [--name name] [--org org] [--region region] [--vcpu n] [--snapshot name]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, alias, err := engine.NewBoxd(ctx, boxdcloud.CreateOpts{
		OrgID: *org, Region: *region, Name: *name, VCPU: *vcpu, Snapshot: *snapshot,
	})
	if err != nil {
		if alias == "" {
			telemetry.Track("boxd_new_fail", r.Version)
			return fail("boxd_new", err.Error())
		}
		telemetry.Track("boxd_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("boxd_new", r.Version)
	msg := fmt.Sprintf("Created boxd computer (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) boxdFork(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boxd fork")
	name := fs.String("name", "", "Name for the copy (generated when blank)")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boxd fork <host|name> [--name name]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoxdSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("boxd_fork", err.Error())
	}
	result, alias, err := engine.ForkBoxd(ctx, syncID, boxdcloud.ForkOpts{Name: *name})
	if err != nil {
		if alias == "" {
			telemetry.Track("boxd_fork_fail", r.Version)
			return fail("boxd_fork", err.Error())
		}
		telemetry.Track("boxd_fork", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Forked to %s (sync incomplete)", alias))
	}
	telemetry.Track("boxd_fork", r.Version)
	msg := "Forked boxd computer"
	if alias != "" {
		msg = fmt.Sprintf("Forked to %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) boxdLifecycle(engine *sync.Engine, verb string, args []string) error {
	fs := newFlagSet("boxd " + verb)
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boxd %s <host|name>", verb)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	code := "boxd_" + verb
	syncID, err := engine.ResolveBoxdSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail(code, err.Error())
	}
	var result sync.Result
	var done string
	switch verb {
	case "stop":
		result, err = engine.StopBoxd(ctx, syncID)
		done = "Stopped"
	case "start":
		result, err = engine.StartBoxd(ctx, syncID)
		done = "Started"
	case "restart":
		result, err = engine.RestartBoxd(ctx, syncID)
		done = "Restarted"
	}
	if err != nil {
		telemetry.Track(code+"_fail", r.Version)
		return fail(code, err.Error())
	}
	telemetry.Track(code, r.Version)
	return r.success(result, fmt.Sprintf("%s boxd computer (%d synced)", done, result.Count))
}

func (r *Runner) boxdDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boxd delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(positionalFirst(args)); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boxd delete <host|name> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("boxd_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoxdSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("boxd_delete", err.Error())
	}
	result, err := engine.DeleteBoxd(ctx, syncID)
	if err != nil {
		telemetry.Track("boxd_delete_fail", r.Version)
		return fail("boxd_delete", err.Error())
	}
	telemetry.Track("boxd_delete", r.Version)
	return r.success(result, fmt.Sprintf("Deleted boxd computer (%d synced)", result.Count))
}
