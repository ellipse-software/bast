package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	boxcloud "bast/internal/cloud/box"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) boxCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast box <new|fork|stop|resume|delete|snapshots|snapshot>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.boxNew(engine, args[1:])
	case "fork":
		return r.boxFork(engine, args[1:])
	case "stop":
		return r.boxStop(engine, args[1:])
	case "resume":
		return r.boxResume(engine, args[1:])
	case "delete":
		return r.boxDelete(engine, args[1:])
	case "snapshots":
		return r.boxSnapshots(engine, args[1:])
	case "snapshot":
		return r.boxSnapshot(engine, args[1:])
	default:
		return usagef("unknown box command %q", args[0])
	}
}

func (r *Runner) boxNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box new")
	boxType := fs.String("type", "default", "Machine size: small, default, or large")
	ttl := fs.Int("ttl", 0, "Auto-stop TTL in seconds (0 = Box default)")
	noAutoStop := fs.Bool("no-auto-stop", false, "Disable automatic stop")
	noEnv := fs.Bool("no-env", false, "Create a no-env box")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast box new [--type small|default|large] [--ttl seconds | --no-auto-stop] [--no-env]")
	}
	if *ttl < 0 {
		return usagef("--ttl must be zero or a positive number of seconds")
	}
	if *ttl > 0 && *noAutoStop {
		return usagef("--ttl and --no-auto-stop cannot be used together")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, alias, err := engine.NewBox(ctx, boxcloud.NewOpts{
		Type: *boxType, TTLSeconds: *ttl, NoAutoStop: *noAutoStop, NoEnv: *noEnv,
	})
	if err != nil {
		if alias == "" {
			telemetry.Track("box_new_fail", r.Version)
			return fail("box_new", err.Error())
		}
		telemetry.Track("box_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("box_new", r.Version)
	msg := fmt.Sprintf("Created box (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) boxFork(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box fork")
	boxType := fs.String("type", "", "Machine size for the fork")
	noEnv := fs.Bool("no-env", false, "Fork as no-env")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast box fork <host|id> [--type small|default|large] [--no-env]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoxSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("box_fork", err.Error())
	}
	result, alias, err := engine.ForkBox(ctx, syncID, boxcloud.ForkOpts{Type: *boxType, NoEnv: *noEnv})
	if err != nil {
		if alias == "" {
			telemetry.Track("box_fork_fail", r.Version)
			return fail("box_fork", err.Error())
		}
		telemetry.Track("box_fork", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Forked to %s (sync incomplete)", alias))
	}
	telemetry.Track("box_fork", r.Version)
	msg := "Forked box"
	if alias != "" {
		msg = fmt.Sprintf("Forked to %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) boxStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast box stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoxSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("box_stop", err.Error())
	}
	result, err := engine.StopBox(ctx, syncID)
	if err != nil {
		telemetry.Track("box_stop_fail", r.Version)
		return fail("box_stop", err.Error())
	}
	telemetry.Track("box_stop", r.Version)
	return r.success(result, fmt.Sprintf("Stopped box (%d synced)", result.Count))
}

func (r *Runner) boxResume(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box resume")
	boxType := fs.String("type", "", "Machine size on resume")
	noEnv := fs.Bool("no-env", false, "Resume as no-env")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast box resume <host|id> [--type small|default|large] [--no-env]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoxSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("box_resume", err.Error())
	}
	result, err := engine.ResumeBox(ctx, syncID, boxcloud.ResumeOpts{Type: *boxType, NoEnv: *noEnv})
	if err != nil {
		if result.Provider == "" {
			telemetry.Track("box_resume_fail", r.Version)
			return fail("box_resume", err.Error())
		}
		telemetry.Track("box_resume", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "warning": err.Error(),
		}, "Resumed box (sync incomplete)")
	}
	telemetry.Track("box_resume", r.Version)
	return r.success(result, fmt.Sprintf("Resumed box (%d synced)", result.Count))
}

func (r *Runner) boxDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast box delete <host|id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("box_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoxSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("box_delete", err.Error())
	}
	result, err := engine.DeleteBox(ctx, syncID)
	if err != nil {
		telemetry.Track("box_delete_fail", r.Version)
		return fail("box_delete", err.Error())
	}
	telemetry.Track("box_delete", r.Version)
	return r.success(result, fmt.Sprintf("Deleted box (%d synced)", result.Count))
}

func (r *Runner) boxSnapshots(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box snapshots")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() > 1 {
		return usagef("usage: bast box snapshots [host|id]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	boxID := ""
	if fs.NArg() == 1 {
		id, err := engine.ResolveBoxSyncID(ctx, fs.Arg(0))
		if err != nil {
			return fail("box_snapshots", err.Error())
		}
		boxID = id
	}
	list, err := engine.ListBoxSnapshots(ctx, boxID)
	if err != nil {
		return fail("box_snapshots", err.Error())
	}
	rows := make([]map[string]any, 0, len(list.Named)+len(list.Snapshots))
	for _, snap := range list.Named {
		rows = append(rows, map[string]any{
			"id": snap.ID, "box": snap.BoxID, "kind": "named", "name": snap.Name, "created": snap.CreatedAt,
		})
	}
	for _, snap := range list.Snapshots {
		rows = append(rows, map[string]any{
			"id": snap.ID, "box": snap.BoxID, "kind": snap.Kind, "created": snap.CreatedAt,
		})
	}
	if r.JSON {
		return r.success(map[string]any{"snapshots": rows, "named": list.Named, "count": len(rows)}, "")
	}
	if len(rows) == 0 {
		fmt.Fprintln(r.Out, "No snapshots")
		return nil
	}
	for _, snap := range list.Named {
		name := snap.Name
		if name == "" {
			name = snap.ID
		}
		fmt.Fprintf(r.Out, "%s  %s  named  %s\n", snap.BoxID, name, snap.CreatedAt)
	}
	for _, snap := range list.Snapshots {
		fmt.Fprintf(r.Out, "%s  %s  %s  %s\n", snap.BoxID, snap.ID, snap.Kind, snap.CreatedAt)
	}
	return nil
}

func (r *Runner) boxSnapshot(engine *sync.Engine, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return usagef("usage: bast box snapshot delete <snapshot-id> [--yes]\n       bast box snapshot rm <name>")
	}
	switch args[0] {
	case "delete":
		return r.boxSnapshotDelete(engine, args[1:])
	case "rm":
		return r.boxSnapshotRm(engine, args[1:])
	default:
		return usagef("unknown box snapshot command %q", args[0])
	}
}

func (r *Runner) boxSnapshotDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box snapshot delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast box snapshot delete <snapshot-id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("box_snapshot_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := engine.DeleteBoxSnapshot(ctx, fs.Arg(0)); err != nil {
		return fail("box_snapshot_delete", err.Error())
	}
	return r.success(map[string]any{"deleted": fs.Arg(0)}, "Deleted snapshot "+fs.Arg(0))
}

func (r *Runner) boxSnapshotRm(engine *sync.Engine, args []string) error {
	fs := newFlagSet("box snapshot rm")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast box snapshot rm <name>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := engine.RemoveBoxNamedSnapshot(ctx, fs.Arg(0)); err != nil {
		return fail("box_snapshot_rm", err.Error())
	}
	return r.success(map[string]any{"removed": fs.Arg(0)}, "Removed named snapshot "+fs.Arg(0))
}
