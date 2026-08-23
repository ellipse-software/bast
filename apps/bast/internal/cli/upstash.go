package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"bast/internal/cloud/sync"
	upstashcloud "bast/internal/cloud/upstash"
	"bast/internal/telemetry"

	"github.com/charmbracelet/x/term"
)

func (r *Runner) upstashCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast upstash <new|fork|stop|resume|delete|key>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "new":
		return r.upstashNew(engine, args[1:])
	case "fork":
		return r.upstashFork(engine, args[1:])
	case "stop":
		return r.upstashStop(engine, args[1:])
	case "resume":
		return r.upstashResume(engine, args[1:])
	case "delete":
		return r.upstashDelete(engine, args[1:])
	case "key":
		return r.upstashKey(engine, args[1:])
	default:
		return usagef("unknown upstash command %q", args[0])
	}
}

func (r *Runner) upstashNew(engine *sync.Engine, args []string) error {
	fs := newFlagSet("upstash new")
	name := fs.String("name", "", "Optional box name")
	runtime := fs.String("runtime", "node", "Runtime: node, python, golang, ruby, rust, or *-alpine")
	size := fs.String("size", "small", "Size: small, medium, or large")
	keepAlive := fs.Bool("keep-alive", false, "Keep the box on between sessions")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast upstash new [--name name] [--runtime node|python|golang|ruby|rust] [--size small|medium|large] [--keep-alive]")
	}
	if err := validateUpstashRuntime(*runtime); err != nil {
		return usagef("%v", err)
	}
	if err := validateUpstashSize(*size); err != nil {
		return usagef("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, alias, err := engine.NewUpstash(ctx, upstashcloud.CreateOpts{
		Name: *name, Runtime: *runtime, Size: *size, KeepAlive: *keepAlive,
	})
	if err != nil {
		if alias == "" {
			telemetry.Track("upstash_new_fail", r.Version)
			return fail("upstash_new", err.Error())
		}
		telemetry.Track("upstash_new", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Created %s (sync incomplete)", alias))
	}
	telemetry.Track("upstash_new", r.Version)
	msg := fmt.Sprintf("Created Upstash box (%d synced)", result.Count)
	if alias != "" {
		msg = fmt.Sprintf("Created %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) upstashFork(engine *sync.Engine, args []string) error {
	fs := newFlagSet("upstash fork")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast upstash fork <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveUpstashSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("upstash_fork", err.Error())
	}
	result, alias, err := engine.ForkUpstash(ctx, syncID)
	if err != nil {
		if alias == "" {
			telemetry.Track("upstash_fork_fail", r.Version)
			return fail("upstash_fork", err.Error())
		}
		telemetry.Track("upstash_fork", r.Version)
		fmt.Fprintf(r.Err, "bast: warning: %v\n", err)
		return r.success(map[string]any{
			"provider": result.Provider, "count": result.Count, "alias": alias, "warning": err.Error(),
		}, fmt.Sprintf("Forked to %s (sync incomplete)", alias))
	}
	telemetry.Track("upstash_fork", r.Version)
	msg := "Forked Upstash box"
	if alias != "" {
		msg = fmt.Sprintf("Forked to %s", alias)
	}
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "alias": alias}, msg)
}

func (r *Runner) upstashStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("upstash stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast upstash stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveUpstashSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("upstash_stop", err.Error())
	}
	result, err := engine.StopUpstash(ctx, syncID)
	if err != nil {
		telemetry.Track("upstash_stop_fail", r.Version)
		return fail("upstash_stop", err.Error())
	}
	telemetry.Track("upstash_stop", r.Version)
	return r.success(result, fmt.Sprintf("Paused Upstash box (%d synced)", result.Count))
}

func (r *Runner) upstashResume(engine *sync.Engine, args []string) error {
	fs := newFlagSet("upstash resume")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast upstash resume <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveUpstashSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("upstash_resume", err.Error())
	}
	result, err := engine.ResumeUpstash(ctx, syncID)
	if err != nil {
		telemetry.Track("upstash_resume_fail", r.Version)
		return fail("upstash_resume", err.Error())
	}
	telemetry.Track("upstash_resume", r.Version)
	return r.success(result, fmt.Sprintf("Resumed Upstash box (%d synced)", result.Count))
}

func (r *Runner) upstashDelete(engine *sync.Engine, args []string) error {
	fs := newFlagSet("upstash delete")
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast upstash delete <host|id> [--yes]")
	}
	if !*yes {
		confirm, err := r.prompt("Type delete to confirm", "", true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(confirm) != "delete" {
			return fail("upstash_delete", "confirmation did not match")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveUpstashSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("upstash_delete", err.Error())
	}
	result, err := engine.DeleteUpstash(ctx, syncID)
	if err != nil {
		telemetry.Track("upstash_delete_fail", r.Version)
		return fail("upstash_delete", err.Error())
	}
	telemetry.Track("upstash_delete", r.Version)
	return r.success(result, fmt.Sprintf("Deleted Upstash box (%d synced)", result.Count))
}

func (r *Runner) upstashKey(engine *sync.Engine, args []string) error {
	fs := newFlagSet("upstash key")
	keyFile := fs.String("key-file", "", "Read the API key from a file")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast upstash key [--key-file path]")
	}
	key := ""
	if strings.TrimSpace(*keyFile) != "" {
		data, err := os.ReadFile(*keyFile)
		if err != nil {
			return fail("upstash_key", err.Error())
		}
		key = strings.TrimSpace(string(data))
	} else if env := strings.TrimSpace(os.Getenv(upstashcloud.APIKeyEnv)); env != "" {
		key = env
	} else {
		secret, err := r.readSecret("Upstash Box API key")
		if err != nil {
			return err
		}
		key = strings.TrimSpace(secret)
	}
	if key == "" {
		return fail("upstash_key", "API key is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := engine.SaveUpstashKey(ctx, key)
	if err != nil {
		telemetry.Track("upstash_key_fail", r.Version)
		return fail("upstash_key", err.Error())
	}
	telemetry.Track("upstash_key", r.Version)
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "stored": true}, fmt.Sprintf("Stored Upstash API key (%d synced)", result.Count))
}

func (r *Runner) readSecret(label string) (string, error) {
	if r.NoInput || !r.interactive() {
		return "", fail("input_required", label+" is required")
	}
	in, ok := r.In.(*os.File)
	if !ok {
		return "", fail("input_required", label+" requires a terminal")
	}
	fmt.Fprintf(r.Err, "%s: ", label)
	secret, err := term.ReadPassword(in.Fd())
	fmt.Fprintln(r.Err)
	if err != nil {
		return "", err
	}
	return string(secret), nil
}

func validateUpstashRuntime(runtime string) error {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "node", "python", "golang", "ruby", "rust",
		"node-alpine", "python-alpine", "golang-alpine", "ruby-alpine", "rust-alpine":
		return nil
	default:
		return fmt.Errorf("runtime must be node, python, golang, ruby, rust, or an *-alpine variant")
	}
}

func validateUpstashSize(size string) error {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "small", "medium", "large":
		return nil
	default:
		return fmt.Errorf("size must be small, medium, or large")
	}
}
