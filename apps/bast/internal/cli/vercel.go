package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"bast/internal/cloud/sync"
	vercelcloud "bast/internal/cloud/vercel"
	"bast/internal/telemetry"
)

func (r *Runner) vercelCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast vercel <new|fork|stop|resume|delete|cleanup|token>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.vercelNew(engine, args[1:])
	case "fork":
		return r.vercelFork(engine, args[1:])
	case "stop":
		return r.vercelStop(engine, args[1:])
	case "resume":
		return r.vercelResume(engine, args[1:])
	case "delete":
		return r.vercelDelete(engine, args[1:])
	case "cleanup":
		return r.vercelCleanup(engine, args[1:])
	case "token":
		return r.vercelToken(engine, args[1:])
	default:
		return usagef("unknown vercel command %q", args[0])
	}
}

func (r *Runner) vercelNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("vercel new")
	name := fs.String("name", "", "Optional sandbox name")
	project := fs.String("project", "", "Project ID or name (default: first stored project)")
	vcpus := fs.Int("vcpus", 2, "vCPUs: 1, 2, or 4")
	timeout := fs.String("timeout", "1h", "Session timeout: 15m, 1h, or 5h")
	ephemeral := fs.Bool("ephemeral", false, "Disable filesystem persistence")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vercel new [--name name] [--project id] [--vcpus 1|2|4] [--timeout 15m|1h|5h] [--ephemeral]")
	}
	if err := vercelcloud.ValidateVCPUs(*vcpus); err != nil {
		return usagef("%v", err)
	}
	duration, err := vercelcloud.ParseTimeoutFlag(*timeout)
	if err != nil {
		return usagef("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, alias, err := engine.NewVercel(ctx, vercelcloud.CreateOpts{
		Name: *name, Project: *project, VCPUs: *vcpus, Timeout: duration, Persistent: !*ephemeral,
	})
	if err != nil {
		if alias == "" {
			telemetry.Track("vercel_new_fail", r.Version)
			return fail("vercel_new", err.Error())
		}
		telemetry.Track("vercel_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("vercel_new", r.Version)
	msg := fmt.Sprintf("Created Vercel sandbox (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) vercelFork(engine *sync.Engine, args []string) error {
	fs := newFlagSet("vercel fork")
	name := fs.String("name", "", "Optional name for the fork")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast vercel fork <host|id> [--name name]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveVercelSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("vercel_fork", err.Error())
	}
	result, alias, err := engine.ForkVercel(ctx, syncID, *name)
	if err != nil {
		if alias == "" {
			telemetry.Track("vercel_fork_fail", r.Version)
			return fail("vercel_fork", err.Error())
		}
		telemetry.Track("vercel_fork", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Forked to %s (sync incomplete)", alias))
	}
	telemetry.Track("vercel_fork", r.Version)
	msg := "Forked Vercel sandbox"
	if alias != "" {
		msg = fmt.Sprintf("Forked to %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) vercelStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("vercel stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast vercel stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveVercelSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("vercel_stop", err.Error())
	}
	result, err := engine.StopVercel(ctx, syncID)
	if err != nil {
		telemetry.Track("vercel_stop_fail", r.Version)
		return fail("vercel_stop", err.Error())
	}
	telemetry.Track("vercel_stop", r.Version)
	return r.success(result, fmt.Sprintf("Stopped Vercel sandbox (%d synced)", result.Count))
}

func (r *Runner) vercelResume(engine *sync.Engine, args []string) error {
	fs := newFlagSet("vercel resume")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast vercel resume <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveVercelSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("vercel_resume", err.Error())
	}
	result, err := engine.ResumeVercel(ctx, syncID)
	if err != nil {
		telemetry.Track("vercel_resume_fail", r.Version)
		return fail("vercel_resume", err.Error())
	}
	telemetry.Track("vercel_resume", r.Version)
	return r.success(result, fmt.Sprintf("Resumed Vercel sandbox (%d synced)", result.Count))
}

func (r *Runner) vercelDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("vercel delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast vercel delete <host|id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("vercel_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveVercelSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("vercel_delete", err.Error())
	}
	result, err := engine.DeleteVercel(ctx, syncID)
	if err != nil {
		telemetry.Track("vercel_delete_fail", r.Version)
		return fail("vercel_delete", err.Error())
	}
	telemetry.Track("vercel_delete", r.Version)
	return r.success(result, fmt.Sprintf("Deleted Vercel sandbox (%d synced)", result.Count))
}

func (r *Runner) vercelCleanup(engine *sync.Engine, args []string) error {
	fs := newFlagSet("vercel cleanup")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vercel cleanup [--yes]")
	}
	listCtx, listCancel := context.WithTimeout(context.Background(), 30*time.Second)
	names, err := engine.ListVercelUnrestorable(listCtx)
	listCancel()
	if err != nil {
		return fail("vercel_cleanup", err.Error())
	}
	if len(names) == 0 {
		return r.success(map[string]any{"deleted": []string{}}, "No unrestorable sandboxes")
	}
	if !*yes {
		if len(names) <= 8 {
			fmt.Fprintf(r.Err, "Offline, no snapshot: %s\n", strings.Join(names, ", "))
		} else {
			fmt.Fprintf(r.Err, "%d unrestorable sandboxes (offline, no snapshot)\n", len(names))
		}
		confirm, err := r.prompt("Type cleanup to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "cleanup" {
			return fail("vercel_cleanup", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), vercelcloud.CleanupTimeout)
	defer cancel()
	var lastLine string
	result, deleted, err := engine.CleanupVercel(ctx, func(p vercelcloud.CleanupProgress) {
		if r.JSON {
			return
		}
		line := vercelcloud.FormatCleanupProgress(p)
		if line == lastLine {
			return
		}
		lastLine = line
		if p.Total > 0 && p.Done == p.Total {
			fmt.Fprintf(r.Err, "\r%-40s\n", line)
			lastLine = ""
			return
		}
		fmt.Fprintf(r.Err, "\r%-40s", line)
	})
	if !r.JSON && lastLine != "" {
		fmt.Fprintln(r.Err)
	}
	if err != nil {
		telemetry.Track("vercel_cleanup_fail", r.Version)
		return fail("vercel_cleanup", err.Error())
	}
	telemetry.Track("vercel_cleanup", r.Version)
	msg := "No unrestorable sandboxes"
	if len(deleted) == 1 {
		msg = "Deleted " + deleted[0]
	} else if len(deleted) > 1 {
		msg = fmt.Sprintf("Deleted %d unrestorable sandboxes", len(deleted))
	}
	return r.success(map[string]any{
		"provider": result.Provider, "count": result.Count, "deleted": deleted,
	}, msg)
}

func (r *Runner) vercelToken(engine *sync.Engine, args []string) error {
	fs := newFlagSet("vercel token")
	tokenFile := fs.String("token-file", "", "Read the access token from a file")
	team := fs.String("team", "", "Vercel team ID")
	project := fs.String("project", "", "Vercel project ID or name (comma-separated to add several)")
	remove := fs.String("remove", "", "Remove a stored project ID")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast vercel token [--token-file path] [--team team_id] [--project id[,id...]]\n       bast vercel token --remove project_id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if strings.TrimSpace(*remove) != "" {
		result, err := engine.RemoveVercelProject(ctx, *remove)
		if err != nil {
			telemetry.Track("vercel_token_fail", r.Version)
			return fail("vercel_token", err.Error())
		}
		telemetry.Track("vercel_token", r.Version)
		return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "removed": *remove}, fmt.Sprintf("Removed Vercel project %s (%d synced)", *remove, result.Count))
	}
	projectID := strings.TrimSpace(*project)
	if projectID == "" {
		projectID = strings.TrimSpace(os.Getenv(vercelcloud.ProjectEnv))
	}
	if engine.Vercel.HasToken() && strings.TrimSpace(*tokenFile) == "" && os.Getenv(vercelcloud.TokenEnv) == "" && projectID != "" && strings.TrimSpace(*team) == "" {
		result, err := engine.AddVercelProject(ctx, projectID)
		if err != nil {
			telemetry.Track("vercel_token_fail", r.Version)
			return fail("vercel_token", err.Error())
		}
		telemetry.Track("vercel_token", r.Version)
		return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "added": projectID}, fmt.Sprintf("Added Vercel project %s (%d synced)", projectID, result.Count))
	}
	token := ""
	if strings.TrimSpace(*tokenFile) != "" {
		data, err := os.ReadFile(*tokenFile)
		if err != nil {
			return fail("vercel_token", err.Error())
		}
		token = strings.TrimSpace(string(data))
	} else if env := strings.TrimSpace(os.Getenv(vercelcloud.TokenEnv)); env != "" {
		token = env
	} else {
		secret, err := r.readSecret("Vercel access token")
		if err != nil {
			return err
		}
		token = strings.TrimSpace(secret)
	}
	if token == "" {
		return fail("vercel_token", "access token is required")
	}
	teamID := strings.TrimSpace(*team)
	if teamID == "" {
		teamID = strings.TrimSpace(os.Getenv(vercelcloud.TeamEnv))
	}
	if teamID == "" {
		value, err := r.prompt("Vercel team ID", engine.Store.Vercel().TeamID, true)
		if err != nil {
			return err
		}
		teamID = strings.TrimSpace(value)
	}
	if projectID == "" {
		existing := strings.Join(engine.Store.Vercel().Projects(), ",")
		value, err := r.prompt("Vercel project ID", existing, len(engine.Store.Vercel().Projects()) == 0)
		if err != nil {
			return err
		}
		projectID = strings.TrimSpace(value)
	}
	if teamID == "" {
		return fail("vercel_token", "team is required")
	}
	if projectID == "" && len(engine.Store.Vercel().Projects()) == 0 {
		return fail("vercel_token", "project is required")
	}
	result, err := engine.SaveVercelToken(ctx, token, teamID, projectID)
	if err != nil {
		telemetry.Track("vercel_token_fail", r.Version)
		return fail("vercel_token", err.Error())
	}
	telemetry.Track("vercel_token", r.Version)
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "stored": true}, fmt.Sprintf("Stored Vercel token (%d synced)", result.Count))
}
