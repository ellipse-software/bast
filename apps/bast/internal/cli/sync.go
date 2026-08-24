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
		fmt.Fprintln(r.Out, "Usage: bast sync <gcp|aws|azure|box|upstash|fly|status|disable>")
		return nil
	}
	engine := sync.New(r.Paths, r.store)
	switch args[0] {
	case "gcp":
		return r.syncGCP(engine, args[1:])
	case "aws":
		return r.syncAWS(engine, args[1:])
	case "azure":
		return r.syncAzure(engine, args[1:])
	case "box":
		return r.syncBox(engine, args[1:])
	case "upstash":
		return r.syncUpstash(engine, args[1:])
	case "fly":
		return r.syncFly(engine, args[1:])
	case "status":
		return r.syncStatus(engine, args[1:])
	case "disable":
		return r.syncDisable(engine, args[1:])
	default:
		return usagef("unknown sync command %q", args[0])
	}
}

func (r *Runner) syncBox(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync box")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast sync box")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := engine.SyncBox(ctx)
	if err != nil {
		telemetry.Track("sync_box_fail", r.Version)
		return fail("sync_failed", err.Error())
	}
	telemetry.Track("sync_box", r.Version)
	msg := fmt.Sprintf("Synced %d boxes", result.Count)
	if result.Error != "" {
		msg += "\nWarning: " + result.Error
	}
	return r.success(result, msg)
}

func (r *Runner) syncUpstash(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync upstash")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast sync upstash")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := engine.SyncUpstash(ctx)
	if err != nil {
		telemetry.Track("sync_upstash_fail", r.Version)
		return fail("sync_failed", err.Error())
	}
	telemetry.Track("sync_upstash", r.Version)
	msg := fmt.Sprintf("Synced %d Upstash boxes", result.Count)
	if result.Error != "" {
		msg += "\nWarning: " + result.Error
	}
	return r.success(result, msg)
}

func (r *Runner) syncFly(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync fly")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast sync fly")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := engine.SyncFly(ctx)
	if err != nil {
		telemetry.Track("sync_fly_fail", r.Version)
		return fail("sync_failed", err.Error())
	}
	telemetry.Track("sync_fly", r.Version)
	msg := fmt.Sprintf("Synced %d Fly Machines", result.Count)
	if result.Error != "" {
		msg += "\nWarning: " + result.Error
	}
	return r.success(result, msg)
}

func (r *Runner) syncAzure(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync azure")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("usage: bast sync azure")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := engine.SyncAzure(ctx)
	if err != nil {
		telemetry.Track("sync_azure_fail", r.Version)
		return fail("sync_failed", err.Error())
	}
	telemetry.Track("sync_azure", r.Version)
	msg := fmt.Sprintf("Synced %d Azure VMs", result.Count)
	if result.Error != "" {
		msg += "\nWarning: " + result.Error
	}
	return r.success(result, msg)
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
	msg := fmt.Sprintf("Synced %d AWS instances", result.Count)
	if result.Error != "" {
		msg += "\nWarning: " + result.Error
	}
	return r.success(result, msg)
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, ran, autoSyncErr := engine.MaybeAutoConnectBox(ctx)
	if autoSyncErr == nil && ran {
		telemetry.Track("sync_box_auto", r.Version)
	}
	_, upstashRan, upstashAutoErr := engine.MaybeAutoConnectUpstash(ctx)
	if upstashAutoErr == nil && upstashRan {
		telemetry.Track("sync_upstash_auto", r.Version)
	}
	_, flyRan, flyAutoErr := engine.MaybeAutoConnectFly(ctx)
	if flyAutoErr == nil && flyRan {
		telemetry.Track("sync_fly_auto", r.Version)
	}
	status, err := engine.Status(ctx)
	if err != nil {
		return fail("sync_status", err.Error())
	}
	if autoSyncErr != nil {
		status.Box.LastSyncError = autoSyncErr.Error()
	}
	if upstashAutoErr != nil {
		status.Upstash.LastSyncError = upstashAutoErr.Error()
	}
	if flyAutoErr != nil {
		status.Fly.LastSyncError = flyAutoErr.Error()
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
	azure := status.Azure
	fmt.Fprintln(r.Out, "Azure")
	fmt.Fprintf(r.Out, "  Enabled: %t\n", azure.Enabled)
	fmt.Fprintf(r.Out, "  Auto-sync: %t\n", azure.AutoSync)
	if azure.AzureCLIError != "" {
		fmt.Fprintf(r.Out, "  az: %s\n", azure.AzureCLIError)
	} else if len(azure.Subscriptions) > 0 {
		fmt.Fprintf(r.Out, "  Subscriptions: %s\n", strings.Join(azure.Subscriptions, ", "))
	} else {
		fmt.Fprintln(r.Out, "  Subscriptions: none")
	}
	if azure.SSHExtensionError != "" {
		fmt.Fprintf(r.Out, "  ssh extension: %s\n", azure.SSHExtensionError)
	}
	if azure.BastionExtensionError != "" {
		fmt.Fprintf(r.Out, "  bastion extension: %s\n", azure.BastionExtensionError)
	}
	if len(azure.SubscriptionFilter) > 0 {
		fmt.Fprintf(r.Out, "  Subscription filter: %s\n", strings.Join(azure.SubscriptionFilter, ", "))
	}
	if len(azure.ResourceGroupFilter) > 0 {
		fmt.Fprintf(r.Out, "  Resource group filter: %s\n", strings.Join(azure.ResourceGroupFilter, ", "))
	}
	if azure.DefaultSSHUser != "" {
		fmt.Fprintf(r.Out, "  Default SSH user: %s\n", azure.DefaultSSHUser)
	}
	if azure.LastSyncAt != nil {
		fmt.Fprintf(r.Out, "  Last sync: %s (%d VMs)\n", azure.LastSyncAt.Local().Format(time.RFC3339), azure.LastInstanceCount)
	} else {
		fmt.Fprintln(r.Out, "  Last sync: never")
	}
	if azure.LastSyncError != "" {
		fmt.Fprintf(r.Out, "  Last error: %s\n", azure.LastSyncError)
	}
	box := status.Box
	fmt.Fprintln(r.Out, "Box")
	fmt.Fprintf(r.Out, "  Enabled: %t\n", box.Enabled)
	fmt.Fprintf(r.Out, "  Auto-sync: %t\n", box.AutoSync)
	if box.Disabled {
		fmt.Fprintln(r.Out, "  Disabled: true (sticky; will not auto-connect)")
	}
	if box.BoxCLIError != "" {
		fmt.Fprintf(r.Out, "  box: %s\n", box.BoxCLIError)
	} else if box.Authenticated {
		login := box.Login
		if login == "" {
			login = "authenticated"
		}
		fmt.Fprintf(r.Out, "  Account: %s\n", login)
		if box.Plan != "" {
			fmt.Fprintf(r.Out, "  Plan: %s\n", box.Plan)
		}
	} else {
		fmt.Fprintln(r.Out, "  Account: not logged in")
	}
	if box.LastSyncAt != nil {
		fmt.Fprintf(r.Out, "  Last sync: %s (%d boxes)\n", box.LastSyncAt.Local().Format(time.RFC3339), box.LastInstanceCount)
	} else {
		fmt.Fprintln(r.Out, "  Last sync: never")
	}
	if box.LastSyncError != "" {
		fmt.Fprintf(r.Out, "  Last error: %s\n", box.LastSyncError)
	}
	upstash := status.Upstash
	fmt.Fprintln(r.Out, "Upstash")
	fmt.Fprintf(r.Out, "  Enabled: %t\n", upstash.Enabled)
	fmt.Fprintf(r.Out, "  Auto-sync: %t\n", upstash.AutoSync)
	if upstash.Disabled {
		fmt.Fprintln(r.Out, "  Disabled: true (sticky; will not auto-connect)")
	}
	if upstash.Error != "" {
		fmt.Fprintf(r.Out, "  API: %s\n", upstash.Error)
	} else if upstash.Authenticated {
		fmt.Fprintln(r.Out, "  Account: authenticated")
	} else if upstash.HasKey {
		fmt.Fprintln(r.Out, "  Account: key stored")
	} else {
		fmt.Fprintln(r.Out, "  Account: no API key")
	}
	if upstash.LastSyncAt != nil {
		fmt.Fprintf(r.Out, "  Last sync: %s (%d boxes)\n", upstash.LastSyncAt.Local().Format(time.RFC3339), upstash.LastInstanceCount)
	} else {
		fmt.Fprintln(r.Out, "  Last sync: never")
	}
	if upstash.LastSyncError != "" {
		fmt.Fprintf(r.Out, "  Last error: %s\n", upstash.LastSyncError)
	}
	fly := status.Fly
	fmt.Fprintln(r.Out, "Fly")
	fmt.Fprintf(r.Out, "  Enabled: %t\n", fly.Enabled)
	fmt.Fprintf(r.Out, "  Auto-sync: %t\n", fly.AutoSync)
	if fly.Disabled {
		fmt.Fprintln(r.Out, "  Disabled: true (sticky; will not auto-connect)")
	}
	if fly.FlyCLIError != "" {
		fmt.Fprintf(r.Out, "  fly: %s\n", fly.FlyCLIError)
	} else if fly.Authenticated {
		login := fly.Login
		if login == "" {
			login = "authenticated"
		}
		fmt.Fprintf(r.Out, "  Account: %s\n", login)
	} else {
		fmt.Fprintln(r.Out, "  Account: not logged in")
	}
	if len(fly.OrgFilter) > 0 {
		fmt.Fprintf(r.Out, "  Org filter: %s\n", strings.Join(fly.OrgFilter, ", "))
	}
	if len(fly.AppFilter) > 0 {
		fmt.Fprintf(r.Out, "  App filter: %s\n", strings.Join(fly.AppFilter, ", "))
	}
	if fly.DefaultSSHUser != "" {
		fmt.Fprintf(r.Out, "  Default SSH user: %s\n", fly.DefaultSSHUser)
	}
	if fly.LastSyncAt != nil {
		fmt.Fprintf(r.Out, "  Last sync: %s (%d machines)\n", fly.LastSyncAt.Local().Format(time.RFC3339), fly.LastInstanceCount)
	} else {
		fmt.Fprintln(r.Out, "  Last sync: never")
	}
	if fly.LastSyncError != "" {
		fmt.Fprintf(r.Out, "  Last error: %s\n", fly.LastSyncError)
	}
	return nil
}

func (r *Runner) syncDisable(engine *sync.Engine, args []string) error {
	fs := newFlagSet("sync disable")
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 1 {
		return usagef("usage: bast sync disable <gcp|aws|azure|box|upstash|fly>")
	}
	provider := fs.Arg(0)
	if provider != "gcp" && provider != "aws" && provider != "azure" && provider != "box" && provider != "upstash" && provider != "fly" {
		return usagef("unknown sync provider %q", provider)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	switch provider {
	case "gcp":
		err = engine.DisableGCP(ctx)
	case "aws":
		err = engine.DisableAWS(ctx)
	case "azure":
		err = engine.DisableAzure(ctx)
	case "box":
		err = engine.DisableBox(ctx)
	case "upstash":
		err = engine.DisableUpstash(ctx)
	case "fly":
		err = engine.DisableFly(ctx)
	}
	if err != nil {
		return fail("sync_disable", err.Error())
	}
	telemetry.Track("sync_"+provider+"_disable", r.Version)
	return r.success(map[string]any{"provider": provider, "enabled": false}, strings.ToUpper(provider)+" sync disabled")
}
