package sshconfig

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Inspection is a diagnostic walk of SSH config. Unlike Discover, it keeps
// duplicate aliases, wildcard Host lines, Match blocks, and include scoping.
type Inspection struct {
	Files       []InspectFile
	Includes    []InspectInclude
	Hosts       []InspectHost
	Matches     []InspectMatch
	ParseErrors []InspectError
}

type InspectFile struct {
	Path    string
	Missing bool
	CRLF    bool
	BOM     bool
}

type InspectInclude struct {
	Source   string
	Line     int
	Pattern  string
	Expanded string
	Matches  []string
	TopLevel bool
	Scope    string // Host alias or Match spec when not top-level
	Depth    int
	Cycle    bool
	Deep     bool
	Empty    bool
}

type InspectHost struct {
	Alias               string
	Source              string
	Line                int
	Wildcard            bool
	HostName            string
	User                string
	Port                string
	IdentityFiles       []string
	RawIdentityFiles    []string
	IdentitiesOnly      string
	ProxyJump           string
	ForwardAgent        string
	CertificateFile     string
	ServerAliveInterval string
	ControlPath         string
	ControlMaster       string
	UseKeychain         string
	AddKeysToAgent      string
	Managed             bool
	ManagedID           string
	Synced              bool
	SyncSource          string
	SyncID              string
}

type InspectMatch struct {
	Source string
	Line   int
	Spec   string
}

type InspectError struct {
	Path    string
	Line    int
	Code    string
	Message string
}

// Inspect walks the main SSH config and every Include without stopping on the
// first cycle or parse problem. Discovery behavior is unchanged.
func (m Manager) Inspect() Inspection {
	var ins Inspection
	if _, err := os.Stat(m.MainConfig); os.IsNotExist(err) {
		ins.Files = append(ins.Files, InspectFile{Path: m.MainConfig, Missing: true})
		return ins
	} else if err != nil {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: m.MainConfig, Code: "read", Message: err.Error()})
		return ins
	}
	stack := map[string]bool{}
	m.inspectFile(m.MainConfig, 0, stack, &ins)
	return ins
}

func (m Manager) inspectFile(pathName string, depth int, stack map[string]bool, ins *Inspection) {
	abs, err := filepath.Abs(pathName)
	if err != nil {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: pathName, Code: "path", Message: err.Error()})
		return
	}
	if depth > 8 {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Code: "include_depth", Message: "SSH config include depth exceeded"})
		return
	}
	if stack[abs] {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Code: "include_cycle", Message: "cyclic SSH config include"})
		return
	}
	b, err := os.ReadFile(abs)
	if os.IsNotExist(err) {
		ins.Files = append(ins.Files, InspectFile{Path: abs, Missing: true})
		return
	}
	if err != nil {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Code: "read", Message: err.Error()})
		return
	}
	file := InspectFile{Path: abs, BOM: bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}), CRLF: bytes.Contains(b, []byte("\r\n"))}
	ins.Files = append(ins.Files, file)
	if file.BOM {
		b = b[3:]
	}
	stack[abs] = true
	defer delete(stack, abs)

	inBlock := false
	scope := ""
	var active []int
	managedID := ""
	managedOpenLine := 0
	syncSource, syncID := "", ""
	syncOpenLine := 0
	baseDir := filepath.Dir(abs)
	includeBase := filepath.Dir(m.MainConfig)
	if includeBase == "" {
		includeBase = baseDir
	}

	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		rawLine := scanner.Text()
		raw := strings.TrimSpace(rawLine)
		if strings.HasPrefix(raw, markerPrefix) {
			if managedID != "" {
				ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: managedOpenLine, Code: "marker_unclosed", Message: "Bast managed block was not closed before the next # bast:id="})
			}
			managedID = strings.TrimSpace(strings.TrimPrefix(raw, markerPrefix))
			managedOpenLine = lineNo
			continue
		}
		if raw == markerEnd {
			managedID = ""
			managedOpenLine = 0
			continue
		}
		if strings.HasPrefix(raw, syncMarkerPrefix) {
			if raw == syncMarkerEnd {
				syncSource, syncID = "", ""
				syncOpenLine = 0
			} else {
				if syncID != "" {
					ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: syncOpenLine, Code: "marker_unclosed", Message: "Bast sync block was not closed before the next # bast:sync: marker"})
				}
				rest := strings.TrimPrefix(raw, syncMarkerPrefix)
				if source, id, ok := strings.Cut(rest, "="); ok {
					syncSource = strings.TrimSpace(source)
					syncID = strings.TrimSpace(id)
					syncOpenLine = lineNo
				}
			}
			continue
		}
		parsed, ferr := fields(raw)
		if ferr != nil {
			ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: lineNo, Code: "parse", Message: ferr.Error()})
			continue
		}
		if len(parsed) == 0 {
			continue
		}
		switch strings.ToLower(parsed[0]) {
		case "include":
			for _, pattern := range parsed[1:] {
				expanded := expandPath(pattern, m.Home, includeBase)
				inc := InspectInclude{
					Source: abs, Line: lineNo, Pattern: pattern, Expanded: expanded,
					TopLevel: !inBlock, Scope: scope, Depth: depth,
				}
				matches, globErr := filepath.Glob(expanded)
				if globErr != nil {
					inc.Empty = true
					ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: lineNo, Code: "include_glob", Message: globErr.Error()})
					ins.Includes = append(ins.Includes, inc)
					continue
				}
				inc.Matches = append([]string(nil), matches...)
				if len(matches) == 0 {
					inc.Empty = true
				}
				ins.Includes = append(ins.Includes, inc)
				idx := len(ins.Includes) - 1
				for _, match := range matches {
					matchAbs, _ := filepath.Abs(match)
					if matchAbs == "" {
						matchAbs = match
					}
					if stack[matchAbs] {
						ins.Includes[idx].Cycle = true
						ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: lineNo, Code: "include_cycle", Message: "cyclic SSH config include at " + matchAbs})
						continue
					}
					if depth+1 > 8 {
						ins.Includes[idx].Deep = true
						ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: lineNo, Code: "include_depth", Message: "SSH config include depth exceeded at " + matchAbs})
						continue
					}
					m.inspectFile(match, depth+1, stack, ins)
				}
			}
		case "host":
			inBlock = true
			active = active[:0]
			aliases := parsed[1:]
			if len(aliases) > 0 {
				scope = aliases[0]
			}
			for _, alias := range aliases {
				h := InspectHost{
					Alias: alias, Source: abs, Line: lineNo,
					Wildcard: !selectableAlias(alias),
					Managed:  abs == m.ManagedConfig && managedID != "", ManagedID: managedID,
					Synced: syncID != "", SyncSource: syncSource, SyncID: syncID,
				}
				ins.Hosts = append(ins.Hosts, h)
				active = append(active, len(ins.Hosts)-1)
			}
		case "match":
			inBlock = true
			active = nil
			spec := strings.Join(parsed[1:], " ")
			scope = spec
			ins.Matches = append(ins.Matches, InspectMatch{Source: abs, Line: lineNo, Spec: spec})
		default:
			if len(active) == 0 || len(parsed) < 2 {
				continue
			}
			applyInspectDirective(ins.Hosts, active, parsed, m.Home, baseDir)
		}
	}
	if err := scanner.Err(); err != nil {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Code: "scan", Message: err.Error()})
	}
	if managedID != "" {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: managedOpenLine, Code: "marker_unclosed", Message: "Bast managed block is missing # bast:end"})
	}
	if syncID != "" {
		ins.ParseErrors = append(ins.ParseErrors, InspectError{Path: abs, Line: syncOpenLine, Code: "marker_unclosed", Message: "Bast sync block is missing # bast:sync:end"})
	}
}

func applyInspectDirective(hosts []InspectHost, active []int, parsed []string, home, baseDir string) {
	key := strings.ToLower(parsed[0])
	value := parsed[1]
	joined := strings.Join(parsed[1:], " ")
	for _, idx := range active {
		h := &hosts[idx]
		switch key {
		case "hostname":
			if h.HostName == "" {
				h.HostName = value
			}
		case "user":
			if h.User == "" {
				h.User = value
			}
		case "port":
			if h.Port == "" {
				h.Port = value
			}
		case "identityfile":
			h.IdentityFiles = append(h.IdentityFiles, expandPath(value, home, baseDir))
			h.RawIdentityFiles = append(h.RawIdentityFiles, value)
		case "identitiesonly":
			if h.IdentitiesOnly == "" {
				h.IdentitiesOnly = value
			}
		case "proxyjump":
			if h.ProxyJump == "" {
				h.ProxyJump = value
			}
		case "forwardagent":
			if h.ForwardAgent == "" {
				h.ForwardAgent = value
			}
		case "certificatefile":
			if h.CertificateFile == "" {
				h.CertificateFile = expandPath(value, home, baseDir)
			}
		case "serveraliveinterval":
			if h.ServerAliveInterval == "" {
				h.ServerAliveInterval = value
			}
		case "controlpath":
			if h.ControlPath == "" {
				h.ControlPath = joined
			}
		case "controlmaster":
			if h.ControlMaster == "" {
				h.ControlMaster = value
			}
		case "usekeychain":
			if h.UseKeychain == "" {
				h.UseKeychain = value
			}
		case "addkeystoagent":
			if h.AddKeysToAgent == "" {
				h.AddKeysToAgent = value
			}
		}
	}
}

func includeTargets(inc InspectInclude, target string) bool {
	if target == "" {
		return false
	}
	want := cleanPath(target)
	if cleanPath(inc.Expanded) == want {
		return true
	}
	for _, match := range inc.Matches {
		if cleanPath(match) == want {
			return true
		}
	}
	return false
}

// HostPatternMatch reports whether an SSH Host pattern matches name.
func HostPatternMatch(pattern, name string) bool {
	if pattern == "" || strings.HasPrefix(pattern, "!") {
		return false
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}
