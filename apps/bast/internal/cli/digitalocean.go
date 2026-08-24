package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	docloud "bast/internal/cloud/digitalocean"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) digitalOceanCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast digitalocean <new|fork|stop|resume|delete>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.digitalOceanNew(engine, args[1:])
	case "fork":
		return r.digitalOceanFork(engine, args[1:])
	case "stop":
		return r.digitalOceanStop(engine, args[1:])
	case "resume":
		return r.digitalOceanResume(engine, args[1:])
	case "delete":
		return r.digitalOceanDelete(engine, args[1:])
	default:
		return usagef("unknown digitalocean command %q", args[0])
	}
}

func (r *Runner) digitalOceanNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("digitalocean new")
	region := fs.String("region", docloud.DefaultNewOpts().Region, "Region slug")
	size := fs.String("size", docloud.DefaultNewOpts().Size, "Size slug")
	image := fs.String("image", docloud.DefaultNewOpts().Image, "Image slug")
	contextName := fs.String("context", "", "doctl auth context")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast digitalocean new <name> [--region nyc3] [--size s-1vcpu-1gb] [--image ubuntu-24-04-x64] [--context name]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, alias, err := engine.NewDigitalOcean(ctx, docloud.NewOpts{
		Name: fs.Arg(0), Region: *region, Size: *size, Image: *image, Context: *contextName,
	})
	if err != nil {
		if alias == "" {
			telemetry.Track("digitalocean_new_fail", r.Version)
			return fail("digitalocean_new", err.Error())
		}
		telemetry.Track("digitalocean_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("digitalocean_new", r.Version)
	msg := fmt.Sprintf("Created droplet (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) digitalOceanFork(engine *sync.Engine, args []string) error {
	fs := newFlagSet("digitalocean fork")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast digitalocean fork <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveDigitalOceanSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("digitalocean_fork", err.Error())
	}
	result, alias, err := engine.ForkDigitalOcean(ctx, syncID)
	if err != nil {
		if alias == "" {
			telemetry.Track("digitalocean_fork_fail", r.Version)
			return fail("digitalocean_fork", err.Error())
		}
		telemetry.Track("digitalocean_fork", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Forked to %s (sync incomplete)", alias))
	}
	telemetry.Track("digitalocean_fork", r.Version)
	msg := "Forked droplet"
	if alias != "" {
		msg = fmt.Sprintf("Forked to %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) digitalOceanStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("digitalocean stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast digitalocean stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveDigitalOceanSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("digitalocean_stop", err.Error())
	}
	result, err := engine.StopDigitalOcean(ctx, syncID)
	if err != nil {
		telemetry.Track("digitalocean_stop_fail", r.Version)
		return fail("digitalocean_stop", err.Error())
	}
	telemetry.Track("digitalocean_stop", r.Version)
	return r.success(result, fmt.Sprintf("Powered off droplet (%d synced)", result.Count))
}

func (r *Runner) digitalOceanResume(engine *sync.Engine, args []string) error {
	fs := newFlagSet("digitalocean resume")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast digitalocean resume <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveDigitalOceanSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("digitalocean_resume", err.Error())
	}
	result, err := engine.ResumeDigitalOcean(ctx, syncID)
	if err != nil {
		telemetry.Track("digitalocean_resume_fail", r.Version)
		return fail("digitalocean_resume", err.Error())
	}
	telemetry.Track("digitalocean_resume", r.Version)
	return r.success(result, fmt.Sprintf("Powered on droplet (%d synced)", result.Count))
}

func (r *Runner) digitalOceanDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("digitalocean delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast digitalocean delete <host|id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("digitalocean_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveDigitalOceanSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("digitalocean_delete", err.Error())
	}
	result, err := engine.DeleteDigitalOcean(ctx, syncID)
	if err != nil {
		telemetry.Track("digitalocean_delete_fail", r.Version)
		return fail("digitalocean_delete", err.Error())
	}
	telemetry.Track("digitalocean_delete", r.Version)
	return r.success(result, fmt.Sprintf("Deleted droplet (%d synced)", result.Count))
}
