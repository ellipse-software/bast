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
		fmt.Fprintln(r.Out, "Usage: bast sync <gcp|aws|status|disable>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "gcp":
		return r.syncGCP(engine, args[1:])
	case "aws":
		return r.syncAWS(engine, args[1:])
	case "status":
		return r.syncStatus(engine, args[1:])
	case "disable":
		return r.syncDisable(engine, args[1:])
	default:
		return usagef("unknown sync command %q", args[0])
	}
}

func (r *Runner) syncAWS(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync aws")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast sync aws")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := engine.SyncAWS(ctx)
	if err != nil {
		telemetry.Track("sync_aws_fail", r.Version)
		return fail("sync_failed", err.Error())
	}
	telemetry.Track("sync_aws", r.Version)
	return r.success(result, fmt.Sprintf("Synced %d AWS instances", result.Count))
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
	aws := status.AWS
	fmt.Fprintln(r.Out, "AWS")
	fmt.Fprintf(r.Out, "  Enabled: %t\n", aws.Enabled)
	fmt.Fprintf(r.Out, "  Auto-sync: %t\n", aws.AutoSync)
	if aws.AWSCLIError != "" {
		fmt.Fprintf(r.Out, "  aws: %s\n", aws.AWSCLIError)
	} else if len(aws.Profiles) > 0 {
		fmt.Fprintf(r.Out, "  Profiles: %s\n", strings.Join(aws.Profiles, ", "))
	} else {
		fmt.Fprintln(r.Out, "  Profiles: none")
	}
	if len(aws.ProfileFilter) > 0 {
		fmt.Fprintf(r.Out, "  Profile filter: %s\n", strings.Join(aws.ProfileFilter, ", "))
	}
	if len(aws.RegionFilter) > 0 {
		fmt.Fprintf(r.Out, "  Region filter: %s\n", strings.Join(aws.RegionFilter, ", "))
	}
	if aws.DefaultSSHUser != "" {
		fmt.Fprintf(r.Out, "  Default SSH user: %s\n", aws.DefaultSSHUser)
	}
	if aws.LastSyncAt != nil {
		fmt.Fprintf(r.Out, "  Last sync: %s (%d instances)\n", aws.LastSyncAt.Local().Format(time.RFC3339), aws.LastInstanceCount)
	} else {
		fmt.Fprintln(r.Out, "  Last sync: never")
	}
	if aws.LastSyncError != "" {
		fmt.Fprintf(r.Out, "  Last error: %s\n", aws.LastSyncError)
	}
	return nil
}

func (r *Runner) syncDisable(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync disable")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast sync disable <gcp|aws>")
	}
	provider := fs.Arg(0)
	if provider != "gcp" && provider != "aws" {
		return usagef("unknown sync provider %q", provider)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	if provider == "gcp" {
		err = engine.DisableGCP(ctx)
	} else {
		err = engine.DisableAWS(ctx)
	}
	if err != nil {
		return fail("sync_disable", err.Error())
	}
	telemetry.Track("sync_"+provider+"_disable", r.Version)
	return r.success(map[string]any{"provider": provider, "enabled": false}, strings.ToUpper(provider)+" sync disabled")
}
