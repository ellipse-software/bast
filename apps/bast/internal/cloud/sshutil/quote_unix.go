//go:build !windows

package sshutil

import "strings"

// ShellQuote quotes one ProxyCommand argument for POSIX shell evaluation.
func ShellQuote(value string) string {
	if safeProxyArgument(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func WithEnvironment(command, name, value string) string {
	return "env " + name + "=" + ShellQuote(value) + " " + command
}
