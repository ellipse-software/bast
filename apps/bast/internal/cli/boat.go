package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	boatcloud "bast/internal/cloud/boat"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) boatCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast boat <new|fork|stop|resume|delete|snapshots|snapshot>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.boatNew(engine, args[1:])
	case "fork":
		return r.boatFork(engine, args[1:])
	case "stop":
		return r.boatStop(engine, args[1:])
	case "resume":
		return r.boatResume(engine, args[1:])
	case "delete":
		return r.boatDelete(engine, args[1:])
	case "snapshots":
		return r.boatSnapshots(engine, args[1:])
	case "snapshot":
		return r.boatSnapshot(engine, args[1:])
	default:
		return usagef("unknown boat command %q", args[0])
	}
}

func (r *Runner) boatNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat new")
	boatType := fs.String("type", "default", "Machine size: small, default, or large")
	ttl := fs.Int("ttl", 0, "Auto-stop TTL in seconds (0 = Boat default)")
	noAutoStop := fs.Bool("no-auto-stop", false, "Disable automatic stop")
	noEnv := fs.Bool("no-env", false, "Create a no-env sandbox")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast boat new [--type small|default|large] [--ttl seconds | --no-auto-stop] [--no-env]")
	}
	if *ttl < 0 {
		return usagef("--ttl must be zero or a positive number of seconds")
	}
	if *ttl > 0 && *noAutoStop {
		return usagef("--ttl and --no-auto-stop cannot be used together")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, alias, err := engine.NewBoat(ctx, boatcloud.NewOpts{
		Type: *boatType, TTLSeconds: *ttl, NoAutoStop: *noAutoStop, NoEnv: *noEnv,
	})
	if err != nil {
		if alias == "" {
			telemetry.Track("boat_new_fail", r.Version)
			return fail("boat_new", err.Error())
		}
		telemetry.Track("boat_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("boat_new", r.Version)
	msg := fmt.Sprintf("Created sandbox (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) boatFork(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat fork")
	boatType := fs.String("type", "", "Machine size for the fork")
	noEnv := fs.Bool("no-env", false, "Fork as no-env")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boat fork <host|id> [--type small|default|large] [--no-env]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoatSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("boat_fork", err.Error())
	}
	result, alias, err := engine.ForkBoat(ctx, syncID, boatcloud.ForkOpts{Type: *boatType, NoEnv: *noEnv})
	if err != nil {
		if alias == "" {
			telemetry.Track("boat_fork_fail", r.Version)
			return fail("boat_fork", err.Error())
		}
		telemetry.Track("boat_fork", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Forked to %s (sync incomplete)", alias))
	}
	telemetry.Track("boat_fork", r.Version)
	msg := "Forked sandbox"
	if alias != "" {
		msg = fmt.Sprintf("Forked to %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) boatStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boat stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoatSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("boat_stop", err.Error())
	}
	result, err := engine.StopBoat(ctx, syncID)
	if err != nil {
		telemetry.Track("boat_stop_fail", r.Version)
		return fail("boat_stop", err.Error())
	}
	telemetry.Track("boat_stop", r.Version)
	return r.success(result, fmt.Sprintf("Stopped sandbox (%d synced)", result.Count))
}

func (r *Runner) boatResume(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat resume")
	boatType := fs.String("type", "", "Machine size on resume")
	noEnv := fs.Bool("no-env", false, "Resume as no-env")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boat resume <host|id> [--type small|default|large] [--no-env]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoatSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("boat_resume", err.Error())
	}
	result, err := engine.ResumeBoat(ctx, syncID, boatcloud.ResumeOpts{Type: *boatType, NoEnv: *noEnv})
	if err != nil {
		if result.Provider == "" {
			telemetry.Track("boat_resume_fail", r.Version)
			return fail("boat_resume", err.Error())
		}
		telemetry.Track("boat_resume", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "warning": err.Error(),
		}, "Resumed sandbox (sync incomplete)")
	}
	telemetry.Track("boat_resume", r.Version)
	return r.success(result, fmt.Sprintf("Resumed sandbox (%d synced)", result.Count))
}

func (r *Runner) boatDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boat delete <host|id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("boat_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBoatSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("boat_delete", err.Error())
	}
	result, err := engine.DeleteBoat(ctx, syncID)
	if err != nil {
		telemetry.Track("boat_delete_fail", r.Version)
		return fail("boat_delete", err.Error())
	}
	telemetry.Track("boat_delete", r.Version)
	return r.success(result, fmt.Sprintf("Deleted sandbox (%d synced)", result.Count))
}

func (r *Runner) boatSnapshots(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat snapshots")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() > 1 {
		return usagef("usage: bast boat snapshots [host|id]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	boatID := ""
	if fs.NArg() == 1 {
		id, err := engine.ResolveBoatSyncID(ctx, fs.Arg(0))
		if err != nil {
			return fail("boat_snapshots", err.Error())
		}
		boatID = id
	}
	list, err := engine.ListBoatSnapshots(ctx, boatID)
	if err != nil {
		return fail("boat_snapshots", err.Error())
	}
	rows := make([]map[string]any, 0, len(list.Named)+len(list.Snapshots))
	for _, snap := range list.Named {
		rows = append(rows, map[string]any{
			"id": snap.SnapshotID, "sandboxId": snap.SourceSandboxID, "kind": "named", "name": snap.Name, "created": snap.CreatedAt,
		})
	}
	for _, snap := range list.Snapshots {
		rows = append(rows, map[string]any{
			"id": snap.ID, "sandboxId": snap.SandboxID, "kind": snap.Kind, "created": snap.CreatedAt,
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
			name = snap.SnapshotID
		}
		fmt.Fprintf(r.Out, "%s  %s  named  %s\n", snap.SourceSandboxID, name, snap.CreatedAt)
	}
	for _, snap := range list.Snapshots {
		fmt.Fprintf(r.Out, "%s  %s  %s  %s\n", snap.SandboxID, snap.ID, snap.Kind, snap.CreatedAt)
	}
	return nil
}

func (r *Runner) boatSnapshot(engine *sync.Engine, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return usagef("usage: bast boat snapshot delete <snapshot-id> [--yes]\n       bast boat snapshot rm <name>")
	}
	switch args[0] {
	case "delete":
		return r.boatSnapshotDelete(engine, args[1:])
	case "rm":
		return r.boatSnapshotRm(engine, args[1:])
	default:
		return usagef("unknown boat snapshot command %q", args[0])
	}
}

func (r *Runner) boatSnapshotDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat snapshot delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boat snapshot delete <snapshot-id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("boat_snapshot_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := engine.DeleteBoatSnapshot(ctx, fs.Arg(0)); err != nil {
		return fail("boat_snapshot_delete", err.Error())
	}
	return r.success(map[string]any{"deleted": fs.Arg(0)}, "Deleted snapshot "+fs.Arg(0))
}

func (r *Runner) boatSnapshotRm(engine *sync.Engine, args []string) error {
	fs := newFlagSet("boat snapshot rm")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast boat snapshot rm <name>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := engine.RemoveBoatNamedSnapshot(ctx, fs.Arg(0)); err != nil {
		return fail("boat_snapshot_rm", err.Error())
	}
	return r.success(map[string]any{"removed": fs.Arg(0)}, "Removed named snapshot "+fs.Arg(0))
}
