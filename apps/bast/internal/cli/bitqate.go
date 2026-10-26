package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	bitqatecloud "bast/internal/cloud/bitqate"
	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) bitqateCmd(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast bitqate <list|start|stop|restart|key>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "list":
		return r.bitqateList(engine, args[1:])
	case "start":
		return r.bitqateStart(engine, args[1:])
	case "stop":
		return r.bitqateStop(engine, args[1:])
	case "restart":
		return r.bitqateRestart(engine, args[1:])
	case "key":
		return r.bitqateKey(engine, args[1:])
	default:
		return usagef("unknown bitqate command %q", args[0])
	}
}

func (r *Runner) bitqateList(engine *sync.Engine, args []string) error {
	fs := newFlagSet("bitqate list")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast bitqate list")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	discovery, err := engine.Bitqate.Discover(ctx, bitqatecloud.DiscoverConfig{
		RegionFilter:   r.store.Bitqate().RegionFilter,
		DefaultSSHUser: r.store.Bitqate().DefaultSSHUser,
		DefaultSSHPort: r.store.Bitqate().DefaultSSHPort,
		Home:           r.Paths.Home,
		ManagedKeys:    r.Paths.ManagedKeys,
	})
	if err != nil {
		telemetry.Track("bitqate_list_fail", r.Version)
		return fail("bitqate_list", err.Error())
	}
	telemetry.Track("bitqate_list", r.Version)
	if r.JSON {
		return r.success(map[string]any{"provider": bitqatecloud.ProviderName, "vms": discovery.Instances}, "")
	}
	if len(discovery.Instances) == 0 {
		fmt.Fprintln(r.Out, "No BitQate VMs")
		return nil
	}
	for _, inst := range discovery.Instances {
		fingerprint := inst.HostKeyFingerprint
		if fingerprint == "" {
			fingerprint = "-"
		}
		region := inst.Region
		if region == "" {
			region = "-"
		}
		fmt.Fprintf(r.Out, "%s\t%s\t%s\t%s\t%s\n", inst.Name, inst.State, inst.HostName, region, fingerprint)
	}
	return nil
}

func (r *Runner) bitqateStart(engine *sync.Engine, args []string) error {
	fs := newFlagSet("bitqate start")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast bitqate start <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBitqateSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("bitqate_start", err.Error())
	}
	result, err := engine.StartBitqate(ctx, syncID)
	if err != nil {
		telemetry.Track("bitqate_start_fail", r.Version)
		return fail("bitqate_start", err.Error())
	}
	telemetry.Track("bitqate_start", r.Version)
	return r.success(result, fmt.Sprintf("Started BitQate VM (%d synced)", result.Count))
}

func (r *Runner) bitqateStop(engine *sync.Engine, args []string) error {
	fs := newFlagSet("bitqate stop")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast bitqate stop <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBitqateSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("bitqate_stop", err.Error())
	}
	result, err := engine.StopBitqate(ctx, syncID)
	if err != nil {
		telemetry.Track("bitqate_stop_fail", r.Version)
		return fail("bitqate_stop", err.Error())
	}
	telemetry.Track("bitqate_stop", r.Version)
	return r.success(result, fmt.Sprintf("Stopped BitQate VM (%d synced)", result.Count))
}

func (r *Runner) bitqateRestart(engine *sync.Engine, args []string) error {
	fs := newFlagSet("bitqate restart")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast bitqate restart <host|id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	syncID, err := engine.ResolveBitqateSyncID(ctx, fs.Arg(0))
	if err != nil {
		return fail("bitqate_restart", err.Error())
	}
	result, err := engine.RestartBitqate(ctx, syncID)
	if err != nil {
		telemetry.Track("bitqate_restart_fail", r.Version)
		return fail("bitqate_restart", err.Error())
	}
	telemetry.Track("bitqate_restart", r.Version)
	return r.success(result, fmt.Sprintf("Restarted BitQate VM (%d synced)", result.Count))
}

func (r *Runner) bitqateKey(engine *sync.Engine, args []string) error {
	fs := newFlagSet("bitqate key")
	keyFile := fs.String("token-file", "", "Read the API token from a file")
	clear := fs.Bool("clear", false, "Remove the stored token")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast bitqate key [--token-file path] | bast bitqate key --clear")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if *clear {
		result, err := engine.DeleteBitqateToken(ctx)
		if err != nil {
			telemetry.Track("bitqate_key_fail", r.Version)
			return fail("bitqate_key", err.Error())
		}
		telemetry.Track("bitqate_key", r.Version)
		return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "cleared": true}, fmt.Sprintf("Removed BitQate token (%d synced)", result.Count))
	}
	key := ""
	if strings.TrimSpace(*keyFile) != "" {
		data, err := os.ReadFile(*keyFile)
		if err != nil {
			return fail("bitqate_key", err.Error())
		}
		key = strings.TrimSpace(string(data))
	} else if env := strings.TrimSpace(os.Getenv(bitqatecloud.APIKeyEnv)); env != "" {
		key = env
	} else {
		secret, err := r.readSecret("BitQate API token")
		if err != nil {
			return err
		}
		key = strings.TrimSpace(secret)
	}
	if key == "" {
		return fail("bitqate_key", "API token is required")
	}
	result, err := engine.SaveBitqateKey(ctx, key)
	if err != nil {
		telemetry.Track("bitqate_key_fail", r.Version)
		return fail("bitqate_key", err.Error())
	}
	telemetry.Track("bitqate_key", r.Version)
	return r.success(map[string]any{"provider": result.Provider, "count": result.Count, "stored": true}, fmt.Sprintf("Stored BitQate token (%d synced)", result.Count))
}
