package cli

import (
	"sort"
	"strings"
)

type candidate struct {
	value string
	desc  string
}

type completeResult struct {
	candidates []candidate
	directive  directive
}

func (r *Runner) completeTokens(tokens []string) completeResult {
	toComplete := ""
	prefix := tokens
	if len(tokens) > 0 {
		toComplete = tokens[len(tokens)-1]
		prefix = tokens[:len(tokens)-1]
	}

	cur := completionRoot()
	var positionals []string
	var pending *flagSpec
	seenFlags := map[string]int{}

	for _, tok := range prefix {
		if pending != nil {
			pending = nil
			continue
		}
		if isGlobalFlagToken(tok) {
			name := strings.TrimLeft(tok, "-")
			seenFlags[name]++
			continue
		}
		name, _, hasEq, isFlag := splitFlag(tok)
		if isFlag {
			if f, ok := findFlag(cur, name); ok {
				seenFlags[f.name]++
				if !f.boolean && !hasEq {
					copyF := f
					pending = &copyF
				}
				continue
			}
			if f, ok := findGlobalFlag(name); ok {
				seenFlags[f.name]++
				continue
			}
			continue
		}
		if child, ok := findChild(cur, tok); ok {
			cur = child
			positionals = nil
			seenFlags = map[string]int{}
			continue
		}
		positionals = append(positionals, tok)
	}

	if name, val, hasEq, isFlag := splitFlag(toComplete); isFlag && hasEq {
		f, ok := findFlag(cur, name)
		if !ok {
			f, ok = findGlobalFlag(name)
		}
		if !ok || f.boolean {
			return completeResult{directive: directiveNoFiles}
		}
		result := r.completeKind(f.kind, f.enum, val, false)
		if result.directive != directiveNoFiles {
			return result
		}
		prefixed := make([]candidate, 0, len(result.candidates))
		flagTok := flagToken(name)
		for _, c := range result.candidates {
			prefixed = append(prefixed, candidate{value: flagTok + "=" + c.value, desc: c.desc})
		}
		return finishComplete(prefixed, directiveNoFiles)
	}

	if pending != nil {
		return r.completeKind(pending.kind, pending.enum, toComplete, false)
	}

	if strings.HasPrefix(toComplete, "-") {
		return finishComplete(r.completeFlags(cur, seenFlags, toComplete), directiveNoFiles)
	}

	var cands []candidate
	if len(positionals) == 0 {
		for _, child := range cur.children {
			for _, name := range child.names() {
				if prefixMatch(toComplete, name) {
					cands = append(cands, candidate{value: name, desc: child.desc})
				}
			}
		}
	}
	if idx := len(positionals); idx < len(cur.args) {
		arg := cur.args[idx]
		more := r.completeKind(arg.kind, arg.enum, toComplete, arg.includeHidden)
		if more.directive != directiveNoFiles && len(cands) == 0 {
			return more
		}
		cands = append(cands, more.candidates...)
	}
	return finishComplete(cands, directiveNoFiles)
}

func (r *Runner) completeFlags(cur specNode, seen map[string]int, toComplete string) []candidate {
	var cands []candidate
	add := func(f flagSpec) {
		if !f.repeatable && seen[f.name] > 0 {
			return
		}
		tok := flagToken(f.name)
		if prefixMatch(toComplete, tok) {
			cands = append(cands, candidate{value: tok, desc: f.desc})
		}
	}
	for _, f := range globalFlagSpecs {
		add(f)
	}
	for _, f := range cur.flags {
		add(f)
	}
	return cands
}

func (r *Runner) completeKind(kind valueKind, enum []string, toComplete string, includeHidden bool) completeResult {
	switch kind {
	case valueFile:
		return completeResult{directive: directiveFiles}
	case valueDir:
		return completeResult{directive: directiveDirs}
	case valueEnum:
		return finishComplete(enumCandidates(enum, toComplete), directiveNoFiles)
	case valueProvider:
		return finishComplete(enumCandidates(syncProviders, toComplete), directiveNoFiles)
	case valueShell:
		return finishComplete(enumCandidates(completionShells, toComplete), directiveNoFiles)
	case valueHost:
		return finishComplete(r.hostCandidates("", includeHidden, toComplete), directiveNoFiles)
	case valueBoxHost:
		return finishComplete(r.hostCandidates("box", true, toComplete), directiveNoFiles)
	case valueUpstashHost:
		return finishComplete(r.hostCandidates("upstash", true, toComplete), directiveNoFiles)
	case valueKey:
		return finishComplete(r.keyCandidates(toComplete), directiveNoFiles)
	default:
		return completeResult{directive: directiveNoFiles}
	}
}

func (r *Runner) hostCandidates(syncSource string, includeHidden bool, toComplete string) []candidate {
	var cands []candidate
	for _, host := range r.completeHosts() {
		if syncSource != "" && host.SyncSource != syncSource {
			continue
		}
		if host.Hidden && !includeHidden {
			continue
		}
		if prefixMatch(toComplete, host.Alias) {
			desc := "host"
			if host.Label != "" && host.Label != host.Alias {
				desc = host.Label
			}
			cands = append(cands, candidate{value: host.Alias, desc: desc})
		}
		if host.Label != "" && host.Label != host.Alias && prefixMatch(toComplete, host.Label) {
			cands = append(cands, candidate{value: host.Label, desc: host.Alias})
		}
		if syncSource != "" && host.SyncID != "" && prefixMatch(toComplete, host.SyncID) {
			cands = append(cands, candidate{value: host.SyncID, desc: host.Alias})
		}
	}
	return cands
}

func (r *Runner) keyCandidates(toComplete string) []candidate {
	var cands []candidate
	for _, name := range r.completeKeyNames() {
		if prefixMatch(toComplete, name) {
			cands = append(cands, candidate{value: name, desc: "key"})
		}
	}
	return cands
}

func enumCandidates(values []string, toComplete string) []candidate {
	var cands []candidate
	for _, value := range values {
		if prefixMatch(toComplete, value) {
			cands = append(cands, candidate{value: value})
		}
	}
	return cands
}

func finishComplete(cands []candidate, dir directive) completeResult {
	sort.SliceStable(cands, func(i, j int) bool {
		return strings.ToLower(cands[i].value) < strings.ToLower(cands[j].value)
	})
	return completeResult{candidates: cands, directive: dir}
}

func findChild(n specNode, name string) (specNode, bool) {
	for _, child := range n.children {
		for _, candidateName := range child.names() {
			if candidateName == name {
				return child, true
			}
		}
	}
	return specNode{}, false
}

func findFlag(n specNode, name string) (flagSpec, bool) {
	for _, f := range n.flags {
		if f.name == name {
			return f, true
		}
	}
	return flagSpec{}, false
}

func findGlobalFlag(name string) (flagSpec, bool) {
	for _, f := range globalFlagSpecs {
		if f.name == name {
			return f, true
		}
	}
	return flagSpec{}, false
}

func splitFlag(token string) (name, value string, hasEq, isFlag bool) {
	if token == "-" || token == "--" || !strings.HasPrefix(token, "-") {
		return "", "", false, false
	}
	body := strings.TrimLeft(token, "-")
	if body == "" {
		return "", "", false, true
	}
	name, value, hasEq = strings.Cut(body, "=")
	return name, value, hasEq, true
}

func isGlobalFlagToken(token string) bool {
	switch token {
	case "--json", "--no-input", "-h", "--help":
		return true
	default:
		return false
	}
}

func flagToken(name string) string {
	if len(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

func prefixMatch(prefix, value string) bool {
	if prefix == "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix))
}
