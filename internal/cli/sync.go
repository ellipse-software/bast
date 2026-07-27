package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bast/internal/cloud/sync"
	"bast/internal/telemetry"
)

func (r *Runner) sync(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(r.Out, "Usage: bast sync <gcp|status|disable>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "gcp":
		return r.syncGCP(engine, args[1:])
	case "status":
		return r.syncStatus(engine, args[1:])
	case "disable":
		return r.syncDisable(engine, args[1:])
	default:
		return usagef("unknown sync command %q", args[0])
	}
}

func (r *Runner) syncGCP(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync gcp")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast sync gcp")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := engine.SyncGCP(ctx)
	if err != nil {
		telemetry.Track("sync_gcp_fail", r.Version)
		return fail("sync_failed", err.Error())
	}
	telemetry.Track("sync_gcp", r.Version)
	msg := fmt.Sprintf("Synced %d GCP instances", result.Count)
	if result.Error != "" {
		msg += "\nWarning: " + result.Error
	}
	return r.success(result, msg)
}

func (r *Runner) syncStatus(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync status")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast sync status")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := engine.Status(ctx)
	if err != nil {
		return fail("sync_status", err.Error())
	}
	if r.JSON {
		return r.success(status, "")
	}
	gcp := status.GCP
	fmt.Fprintln(r.Out, "GCP")
	fmt.Fprintf(r.Out, "  Enabled: %t\n", gcp.Enabled)
	fmt.Fprintf(r.Out, "  Auto-sync: %t\n", gcp.AutoSync)
	if gcp.GCloudError != "" {
		fmt.Fprintf(r.Out, "  gcloud: %s\n", gcp.GCloudError)
	} else if len(gcp.Accounts) > 0 {
		fmt.Fprintf(r.Out, "  Accounts: %s\n", strings.Join(gcp.Accounts, ", "))
	} else {
		fmt.Fprintln(r.Out, "  Accounts: none")
	}
	if len(gcp.ServiceAccounts) > 0 {
		fmt.Fprintf(r.Out, "  Service accounts: %s\n", strings.Join(gcp.ServiceAccounts, ", "))
	}
	if len(gcp.ProjectFilter) > 0 {
		fmt.Fprintf(r.Out, "  Project filter: %s\n", strings.Join(gcp.ProjectFilter, ", "))
	}
	if gcp.DefaultSSHUser != "" {
		fmt.Fprintf(r.Out, "  Default SSH user: %s\n", gcp.DefaultSSHUser)
	}
	if gcp.LastSyncAt != nil {
		fmt.Fprintf(r.Out, "  Last sync: %s (%d instances)\n", gcp.LastSyncAt.Local().Format(time.RFC3339), gcp.LastInstanceCount)
	} else {
		fmt.Fprintln(r.Out, "  Last sync: never")
	}
	if gcp.LastSyncError != "" {
		fmt.Fprintf(r.Out, "  Last error: %s\n", gcp.LastSyncError)
	}
	return nil
}

func (r *Runner) syncDisable(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync disable")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast sync disable <gcp>")
	}
	provider := fs.Arg(0)
	if provider != "gcp" {
		return usagef("unknown sync provider %q", provider)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := engine.DisableGCP(ctx); err != nil {
		return fail("sync_disable", err.Error())
	}
	telemetry.Track("sync_gcp_disable", r.Version)
	return r.success(map[string]any{"provider": "gcp", "enabled": false}, "GCP sync disabled")
}
