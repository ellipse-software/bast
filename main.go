package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"bast/internal/openssh"
	"bast/internal/paths"
	"bast/internal/sshconfig"
	"bast/internal/ui"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bast:", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 1 {
		return errors.New("usage: bast [label]")
	}
	if len(args) == 1 {
		switch args[0] {
		case "-h", "--help":
			fmt.Println("Bast — native SSH picker and key manager\n\nUsage:\n  bast          Open the TUI\n  bast <label>  Connect directly using a host label\n  bast --help\n  bast --version")
			return nil
		case "-v", "--version":
			fmt.Println("bast", buildVersion())
			return nil
		}
	}

	p, err := paths.Default()
	if err != nil {
		return err
	}
	client := openssh.Default()
	if err := client.Check(); err != nil {
		return err
	}
	if len(args) == 1 {
		return directConnect(p, client, args[0])
	}
	model, err := ui.New(p, client)
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

func directConnect(p paths.Paths, client openssh.Client, alias string) error {
	manager := sshconfig.Manager{
		Home: p.Home, MainConfig: p.MainConfig, ManagedDir: p.ManagedDir,
		ManagedConfig: p.ManagedConfig, ManagedKeys: p.ManagedKeys,
	}
	hosts, err := manager.Discover()
	if err != nil {
		return err
	}
	found := false
	for _, host := range hosts {
		if host.Alias == alias {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown host label %q", alias)
	}
	cmd, err := client.SSHCommand(alias)
	if err != nil {
		return err
	}
	return cmd.Run()
}
