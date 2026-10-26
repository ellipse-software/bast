package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	railwaycloud "bast/internal/cloud/railway"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) railwayCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast railway <new|stop|resume|delete|key>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.railwayNew(engine, args[1:])
	case "stop":
		return r.railwayStop(engine, args[1:])
	case "resume":
		return r.railwayResume(engine, args[1:])
	case "delete":
		return r.railwayDelete(engine, args[1:])
	case "key":
		return r.railwayKey(engine, args[1:])
	default:
		return usagef("unknown railway command %q", args[0])
	}
}

func (r *Runner) railwayNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("railway new")
	name := fs.String("name", "", "Service name")
	image := fs.String("image", railwaycloud.DefaultImage, "Docker image")
	start := fs.String("start-command", railwaycloud.DefaultStart, "Container start command")
	project := fs.String("project", "", "Existing project id or name")
	newProject := fs.String("new-project", "", "Create a new project with this name")
	environment := fs.String("environment", "", "Environment id (defaults to production)")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast railway new --name name [--image ubuntu:24.04] [--start-command command] [--project id|name | --new-project name] [--environment id]")
	}
	if strings.TrimSpace(*name) == "" {
		return usagef("service name is required")
	}
	if strings.TrimSpace(*project) == "" && strings.TrimSpace(*newProject) == "" {
		return usagef("pass --project or --new-project")
	}
	opts := railwaycloud.CreateOpts{
		Name:          *name,
		Image:         *image,
		StartCommand:  *start,
		EnvironmentID: strings.TrimSpace(*environment),
		NewProject:    strings.TrimSpace(*newProject),
	}
	if opts.NewProject == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		resolved, err := engine.Railway.ResolveProject(ctx, *project)
		cancel()
		if err != nil {
			return fail("railway_new", err.Error())
		}
		opts.ProjectID = resolved.ID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, alias, err := engine.NewRailway(ctx, opts)
	if err != nil {
		if alias == "" {
			telemetry.Track("railway_new_fail", r.Version)
			return fail("railway_new", err.Error())
		}
		telemetry.Track("railway_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("railway_new", r.Version)
	msg := fmt.Sprintf("Created Railway service (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) railwayStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("railway stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast railway stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveRailwaySyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("railway_stop", err.Error())
	}
	result, err := engine.StopRailway(ctx, syncID)
	if err != nil {
		telemetry.Track("railway_stop_fail", r.Version)
		return fail("railway_stop", err.Error())
	}
	telemetry.Track("railway_stop", r.Version)
	return r.success(result, fmt.Sprintf("Stopped Railway service (%d synced)", result.Count))
}

func (r *Runner) railwayResume(engine *sync.Engine, args []string) error {
	fs := newFlagSet("railway resume")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast railway resume <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveRailwaySyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("railway_resume", err.Error())
	}
	result, err := engine.ResumeRailway(ctx, syncID)
	if err != nil {
		telemetry.Track("railway_resume_fail", r.Version)
		return fail("railway_resume", err.Error())
	}
	telemetry.Track("railway_resume", r.Version)
	return r.success(result, fmt.Sprintf("Resumed Railway service (%d synced)", result.Count))
}

func (r *Runner) railwayDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("railway delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast railway delete <host|id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("railway_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveRailwaySyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("railway_delete", err.Error())
	}
	result, err := engine.DeleteRailway(ctx, syncID)
	if err != nil {
		telemetry.Track("railway_delete_fail", r.Version)
		return fail("railway_delete", err.Error())
	}
	telemetry.Track("railway_delete", r.Version)
	return r.success(result, fmt.Sprintf("Deleted Railway service (%d synced)", result.Count))
}

func (r *Runner) railwayKey(engine *sync.Engine, args []string) error {
	fs := newFlagSet("railway key")
	keyFile := fs.String("key-file", "", "Read the API token from a file")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast railway key [--key-file path]")
	}
	key := ""
	if strings.TrimSpace(*keyFile) != "" {
		data, err := os.ReadFile(*keyFile)
		if err != nil {
			return fail("railway_key", err.Error())
		}
		key = strings.TrimSpace(string(data))
	} else if env := strings.TrimSpace(os.Getenv(railwaycloud.APIKeyEnv)); env != "" {
		key = env
	} else {
		secret, err := r.readSecret("Railway API token")
		if err != nil {
			return err
		}
		key = strings.TrimSpace(secret)
	}
	if key == "" {
		return fail("railway_key", "API token is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := engine.SaveRailwayToken(ctx, key)
	if err != nil {
		telemetry.Track("railway_key_fail", r.Version)
		return fail("railway_key", err.Error())
	}
	telemetry.Track("railway_key", r.Version)
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "stored": true}, fmt.Sprintf("Stored Railway API token (%d synced)", result.Count))
}
