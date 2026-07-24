package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"bast/internal/cli"
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
		telemetry.Track("direct_connect", buildVersion())
		runner, err := cli.New(p, client, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			return err
		}
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
