//go:build !windows

package files

import (
	"fmt"
	"os"
	"os/exec"
)

// ShellCommand returns an interactive local shell started in dir.
func ShellCommand(dir string) (*exec.Cmd, error) {
	dir, err := CleanLocal(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	shell := os.Getenv("BAST_SHELL")
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-l")
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd, nil
}
