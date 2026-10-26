package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	hetznercloud "bast/internal/cloud/hetzner"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) hetznerCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast hetzner <start|stop|restart|key>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "start":
		return r.hetznerStart(engine, args[1:])
	case "stop":
		return r.hetznerStop(engine, args[1:])
	case "restart":
		return r.hetznerRestart(engine, args[1:])
	case "key":
		return r.hetznerKey(engine, args[1:])
	default:
		return usagef("unknown hetzner command %q", args[0])
	}
}

func (r *Runner) hetznerStart(engine *sync.Engine, args []string) error {
	fs := newFlagSet("hetzner start")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hetzner start <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveHetznerSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("hetzner_start", err.Error())
	}
	result, err := engine.StartHetzner(ctx, syncID)
	if err != nil {
		telemetry.Track("hetzner_start_fail", r.Version)
		return fail("hetzner_start", err.Error())
	}
	telemetry.Track("hetzner_start", r.Version)
	return r.success(result, fmt.Sprintf("Started Hetzner server (%d synced)", result.Count))
}

func (r *Runner) hetznerStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("hetzner stop")
	force := fs.Bool("force", false, "Hard poweroff instead of ACPI shutdown")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hetzner stop <host|id> [--force]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveHetznerSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("hetzner_stop", err.Error())
	}
	result, err := engine.StopHetzner(ctx, syncID, *force)
	if err != nil {
		telemetry.Track("hetzner_stop_fail", r.Version)
		return fail("hetzner_stop", err.Error())
	}
	telemetry.Track("hetzner_stop", r.Version)
	return r.success(result, fmt.Sprintf("Stopped Hetzner server (%d synced)", result.Count))
}

func (r *Runner) hetznerRestart(engine *sync.Engine, args []string) error {
	fs := newFlagSet("hetzner restart")
	force := fs.Bool("force", false, "Hard reset instead of ACPI reboot")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast hetzner restart <host|id> [--force]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveHetznerSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("hetzner_restart", err.Error())
	}
	result, err := engine.RestartHetzner(ctx, syncID, *force)
	if err != nil {
		telemetry.Track("hetzner_restart_fail", r.Version)
		return fail("hetzner_restart", err.Error())
	}
	telemetry.Track("hetzner_restart", r.Version)
	return r.success(result, fmt.Sprintf("Restarted Hetzner server (%d synced)", result.Count))
}

func (r *Runner) hetznerKey(engine *sync.Engine, args []string) error {
	fs := newFlagSet("hetzner key")
	name := fs.String("name", "default", "Project name for this token (one token per Hetzner Cloud project)")
	keyFile := fs.String("key-file", "", "Read the API token from a file")
	remove := fs.String("remove", "", "Remove the stored token with this project name")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast hetzner key [--name project] [--key-file path] | --remove project")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if strings.TrimSpace(*remove) != "" {
		result, err := engine.DeleteHetznerToken(ctx, *remove)
		if err != nil {
			telemetry.Track("hetzner_key_fail", r.Version)
			return fail("hetzner_key", err.Error())
		}
		telemetry.Track("hetzner_key", r.Version)
		return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "removed": *remove}, fmt.Sprintf("Removed Hetzner token %s (%d synced)", *remove, result.Count))
	}
	key := ""
	if strings.TrimSpace(*keyFile) != "" {
		data, err := os.ReadFile(*keyFile)
		if err != nil {
			return fail("hetzner_key", err.Error())
		}
		key = strings.TrimSpace(string(data))
	} else if env := strings.TrimSpace(os.Getenv(hetznercloud.APIKeyEnv)); env != "" && strings.TrimSpace(*name) == "default" {
		key = env
	} else {
		secret, err := r.readSecret("Hetzner API token")
		if err != nil {
			return err
		}
		key = strings.TrimSpace(secret)
	}
	if key == "" {
		return fail("hetzner_key", "API token is required")
	}
	result, err := engine.SaveHetznerKey(ctx, *name, key)
	if err != nil {
		telemetry.Track("hetzner_key_fail", r.Version)
		return fail("hetzner_key", err.Error())
	}
	telemetry.Track("hetzner_key", r.Version)
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "name": *name, "stored": true}, fmt.Sprintf("Stored Hetzner token %s (%d synced)", *name, result.Count))
}
