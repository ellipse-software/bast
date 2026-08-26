package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"bast/internal/askpass"
	"bast/internal/cli"
	azurecloud "bast/internal/cloud/azure"
	upstashcloud "bast/internal/cloud/upstash"
	vercelcloud "bast/internal/cloud/vercel"
	"bast/internal/hostpass"
	"bast/internal/metadata"
	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/telemetry"
	"bast/internal/ui"
)

var version = "dev"

func main() {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		stack := string(debug.Stack())
		fmt.Fprintf(os.Stderr, "bast: panic: %v\n%s", recovered, stack)
		if telemetry.Enabled() && term.IsTerminal(os.Stdin.Fd()) {
			telemetry.OfferReport(os.Stdin, os.Stderr, telemetry.Report{
				Message: fmt.Sprint(recovered),
				Version: buildVersion(),
				Stack:   stack,
				Context: "panic",
			})
		}
		os.Exit(2)
	}()

	if err := run(os.Args[1:]); err != nil {
		if code, ok := cli.ExitCode(err); ok {
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, "bast:", err)
		if telemetry.Enabled() && term.IsTerminal(os.Stdin.Fd()) {
			telemetry.OfferReport(os.Stdin, os.Stderr, telemetry.Report{
				Message: err.Error(),
				Version: buildVersion(),
				Context: "cli",
			})
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if askpass.IsRequest() {
		if term.IsTerminal(os.Stdout.Fd()) {
			return errors.New("askpass refused: stdout is a terminal")
		}
		p, err := paths.Default()
		if err != nil {
			return err
		}
		prompt := ""
		if len(args) > 0 {
			prompt = args[0]
		}
		switch askpass.Kind() {
		case askpass.KindHost:
			return hostpass.Print(os.Stdout, p.PasswordsDir, askpass.HostID(), prompt, askpass.ExpectedHost(), askpass.ExpectedAlias())
		default:
			return upstashcloud.PrintAPIKey(os.Stdout, p.UpstashAPIKey)
		}
	}
	if len(args) > 0 && args[0] == "__azure-bastion-proxy" {
		options, err := azurecloud.ParseProxyOptions(args[1:])
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return azurecloud.RunBastionProxy(ctx, options, os.Stdin, os.Stdout, os.Stderr)
	}
	if len(args) > 0 && args[0] == "__vercel-sandbox-shell" {
		options, err := vercelcloud.ParseShellOptions(args[1:])
		if err != nil {
			return err
		}
		p, err := paths.Default()
		if err != nil {
			return err
		}
		client := vercelcloud.New(p.VercelToken)
		if store, storeErr := metadata.Open(p.StateFile); storeErr == nil {
			integration := store.Vercel()
			if options.TeamID == "" {
				options.TeamID = integration.TeamID
			}
			if options.ProjectID == "" {
				options.ProjectID = integration.ProjectID
			}
		}
		if strings.TrimSpace(options.ProjectID) == "" {
			return errors.New("vercel project is required")
		}
		client.TeamID = options.TeamID
		client.ProjectID = options.ProjectID
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return vercelcloud.RunShell(ctx, client, options, os.Stdin, os.Stdout, os.Stderr)
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
	positional := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--json" && arg != "--no-input" {
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 {
		return errors.New("usage: bast [label]")
	}
	if len(positional) == 1 {
		runner, err := cli.New(p, client, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			return err
		}
		runner.Version = buildVersion()
		return runner.Run(append([]string{"connect"}, args...))
	}
	telemetry.Track("tui_open", buildVersion())
	model, err := ui.New(p, client, buildVersion())
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(model, tea.WithFilter(ui.FilterIdleMouseMotion)).Run()
	return err
}

func buildVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
