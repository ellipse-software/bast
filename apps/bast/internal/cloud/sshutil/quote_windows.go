//go:build windows

package sshutil

import "strings"

// ShellQuote quotes one ProxyCommand argument for cmd.exe, which is the shell
// used by Win32 OpenSSH for ProxyCommand expansion.
func ShellQuote(value string) string {
	if safeProxyArgument(value) {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func WithEnvironment(command, name, value string) string {
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `set "` + name + `=` + value + `"&& ` + command
}
