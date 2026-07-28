package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"bast/internal/cli"
	azurecloud "bast/internal/cloud/azure"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/telemetry"
	"bast/internal/ui"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if code, ok := cli.ExitCode(err); ok {
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, "bast:", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "__azure-bastion-proxy" {
		options, err := azurecloud.ParseProxyOptions(args[1:])
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return azurecloud.RunBastionProxy(ctx, options, os.Stdin, os.Stdout, os.Stderr)
	}
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}
	if len(args) == 1 && (args[0] == "-v" || args[0] == "--version") {
		fmt.Println("bast", buildVersion())
		return nil
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		cli.PrintHelp(os.Stdout)
		return nil
	}
	if len(args) == 2 && jsonOutput && (args[0] == "-v" || args[0] == "--version" || args[1] == "-v" || args[1] == "--version") {
		fmt.Printf("{\"ok\":true,\"data\":{\"version\":%q}}\n", buildVersion())
		return nil
	}

	p, err := paths.Default()
	if err != nil {
		return err
	}
	client := openssh.Default()
	if cli.IsInvocation(args) {
		if len(args) == 1 && args[0] == "tui" {
			args = nil
		} else {
			runner, err := cli.New(p, client, os.Stdin, os.Stdout, os.Stderr)
			if err != nil {
				return err
			}
			runner.Version = buildVersion()
			return runner.Run(args)
		}
	}
	if err := client.Check(); err != nil {
		return err
	}
	if len(args) > 1 {
		return errors.New("usage: bast [label]")
	}
	if len(args) == 1 {
		runner, err := cli.New(p, client, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			return err
		}
		runner.Version = buildVersion()
		return runner.Run([]string{"connect", args[0]})
	}
	telemetry.Track("tui_open", buildVersion())
	model, err := ui.New(p, client, version)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(model).Run()
	return err
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
