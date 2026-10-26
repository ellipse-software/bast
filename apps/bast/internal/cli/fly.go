package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	flycloud "bast/internal/cloud/fly"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) flyCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast fly <new|fork|stop|resume|delete>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.flyNew(engine, args[1:])
	case "fork":
		return r.flyFork(engine, args[1:])
	case "stop":
		return r.flyStop(engine, args[1:])
	case "resume":
		return r.flyResume(engine, args[1:])
	case "delete":
		return r.flyDelete(engine, args[1:])
	default:
		return usagef("unknown fly command %q", args[0])
	}
}

func (r *Runner) flyNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("fly new")
	app := fs.String("app", "", "Existing Fly app")
	image := fs.String("image", "", "Docker image")
	org := fs.String("org", "", "Fly organization slug")
	region := fs.String("region", "", "Region code")
	size := fs.String("size", "", "VM size, for example shared-cpu-1x")
	name := fs.String("name", "", "Optional machine name")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast fly new --app app --image image [--org org] [--region region] [--size size] [--name name]")
	}
	if strings.TrimSpace(*app) == "" || strings.TrimSpace(*image) == "" {
		return usagef("--app and --image are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, alias, err := engine.NewFly(ctx, flycloud.CreateOpts{
		App: *app, Image: *image, Org: *org, Region: *region, Size: *size, Name: *name,
	})
	if err != nil {
		if alias == "" {
			telemetry.Track("fly_new_fail", r.Version)
			return fail("fly_new", err.Error())
		}
		telemetry.Track("fly_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("fly_new", r.Version)
	msg := fmt.Sprintf("Created Fly Machine (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) flyFork(engine *sync.Engine, args []string) error {
	fs := newFlagSet("fly fork")
	region := fs.String("region", "", "Region for the clone")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast fly fork <host|id> [--region region]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveFlySyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("fly_fork", err.Error())
	}
	result, alias, err := engine.ForkFly(ctx, syncID, flycloud.ForkOpts{Region: *region})
	if err != nil {
		if alias == "" {
			telemetry.Track("fly_fork_fail", r.Version)
			return fail("fly_fork", err.Error())
		}
		telemetry.Track("fly_fork", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Forked to %s (sync incomplete)", alias))
	}
	telemetry.Track("fly_fork", r.Version)
	msg := "Forked Fly Machine"
	if alias != "" {
		msg = fmt.Sprintf("Forked to %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) flyStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("fly stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast fly stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveFlySyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("fly_stop", err.Error())
	}
	result, err := engine.StopFly(ctx, syncID)
	if err != nil {
		telemetry.Track("fly_stop_fail", r.Version)
		return fail("fly_stop", err.Error())
	}
	telemetry.Track("fly_stop", r.Version)
	return r.success(result, fmt.Sprintf("Stopped Fly Machine (%d synced)", result.Count))
}

func (r *Runner) flyResume(engine *sync.Engine, args []string) error {
	fs := newFlagSet("fly resume")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast fly resume <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveFlySyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("fly_resume", err.Error())
	}
	result, err := engine.ResumeFly(ctx, syncID)
	if err != nil {
		telemetry.Track("fly_resume_fail", r.Version)
		return fail("fly_resume", err.Error())
	}
	telemetry.Track("fly_resume", r.Version)
	return r.success(result, fmt.Sprintf("Started Fly Machine (%d synced)", result.Count))
}

func (r *Runner) flyDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("fly delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast fly delete <host|id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("fly_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveFlySyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("fly_delete", err.Error())
	}
	result, err := engine.DeleteFly(ctx, syncID)
	if err != nil {
		telemetry.Track("fly_delete_fail", r.Version)
		return fail("fly_delete", err.Error())
	}
	telemetry.Track("fly_delete", r.Version)
	return r.success(result, fmt.Sprintf("Destroyed Fly Machine (%d synced)", result.Count))
}
