package sshutil

import "strings"

func safeProxyArgument(value string) bool {
	return value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("-_=./:@%+", r))
	}) < 0
}
