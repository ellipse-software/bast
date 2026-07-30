package sshutil

import (
	"fmt"
	"regexp"
	"strings"
)

var unsafeAliasChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// UniqueAlias adds a numeric suffix when base is already in use.
func UniqueAlias(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

// SanitizeAliasPart converts a provider value into a bounded SSH alias segment.
func SanitizeAliasPart(value string) string {
	value = unsafeAliasChars.ReplaceAllString(strings.TrimSpace(value), "_")
	value = strings.Trim(value, "_")
	if len(value) > 48 {
		value = value[:48]
	}
	return value
}

// ProxyLiteral escapes percent signs that OpenSSH would otherwise expand.
func ProxyLiteral(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}

// ShellQuote quotes one ProxyCommand argument for POSIX shell evaluation.
func ShellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("-_=./:@%+", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
