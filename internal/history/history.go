package history

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

const (
	anchorCount  = 8
	anchorBytes  = 4096
	maxPending   = 200
	maxRecordLen = 1024 * 1024
)

type record struct {
	command string
	seenAt  int64
}

// Scan reads new records from every standard shell history source. It returns
// an updated snapshot; callers decide when that snapshot is durably stored.
func Scan(home string, getenv func(string) string, previous metadata.HistoryImport, hosts []sshconfig.Host) (metadata.HistoryImport, []error) {
	next := metadata.HistoryImport{
		Sources: make(map[string]metadata.HistorySource, len(previous.Sources)),
		Pending: append([]metadata.HistorySuggestion(nil), previous.Pending...),
	}
	for path, source := range previous.Sources {
		source.Anchors = append([]string(nil), source.Anchors...)
		next.Sources[path] = source
	}
	pending := make(map[string]metadata.HistorySuggestion, len(next.Pending))
	for _, suggestion := range next.Pending {
		pending[suggestion.ID] = suggestion
	}

	aliases, endpoints := existingHosts(hosts)
	var scanErrors []error
	for _, path := range sourcePaths(home, getenv) {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			scanErrors = append(scanErrors, fmt.Errorf("read %s history: %w", shellName(path), err))
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		checkpoint := next.Sources[path]
		updated, suggestions, err := scanSource(path, info, checkpoint)
		if err != nil {
			scanErrors = append(scanErrors, fmt.Errorf("read %s history: %w", shellName(path), err))
			continue
		}
		next.Sources[path] = updated
		for _, suggestion := range suggestions {
			if aliases[strings.ToLower(suggestion.Target)] || endpoints[endpointKey(suggestion.User, suggestion.HostName, suggestion.Port)] {
				continue
			}
			if current, ok := pending[suggestion.ID]; !ok || suggestion.SeenAt >= current.SeenAt {
				pending[suggestion.ID] = suggestion
			}
		}
	}

	next.Pending = next.Pending[:0]
	for _, suggestion := range pending {
		if aliases[strings.ToLower(suggestion.Target)] || endpoints[endpointKey(suggestion.User, suggestion.HostName, suggestion.Port)] {
			continue
		}
		next.Pending = append(next.Pending, suggestion)
	}
	sort.Slice(next.Pending, func(i, j int) bool {
		if next.Pending[i].SeenAt != next.Pending[j].SeenAt {
			return next.Pending[i].SeenAt > next.Pending[j].SeenAt
		}
		return next.Pending[i].ID < next.Pending[j].ID
	})
	if len(next.Pending) > maxPending {
		next.Pending = next.Pending[:maxPending]
	}
	assignAliases(next.Pending, aliases)
	return next, scanErrors
}

func sourcePaths(home string, getenv func(string) string) []string {
	paths := []string{
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".zhistory"),
		filepath.Join(home, ".bash_history"),
	}
	if histfile := strings.TrimSpace(getenv("HISTFILE")); histfile != "" {
		paths = append(paths, expandHome(histfile, home))
	}
	if zdotdir := strings.TrimSpace(getenv("ZDOTDIR")); zdotdir != "" {
		zshHome := expandHome(zdotdir, home)
		paths = append(paths, filepath.Join(zshHome, ".zsh_history"), filepath.Join(zshHome, ".zhistory"))
	}
	dataHome := strings.TrimSpace(getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	} else {
		dataHome = expandHome(dataHome, home)
	}
	fishSession := strings.TrimSpace(getenv("fish_history"))
	if fishSession == "" || fishSession == "default" {
		fishSession = "fish"
	}
	if !strings.ContainsAny(fishSession, `/\`) {
		paths = append(paths, filepath.Join(dataHome, "fish", fishSession+"_history"))
	}

	seen := map[string]bool{}
	unique := paths[:0]
	for _, path := range paths {
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			unique = append(unique, path)
		}
	}
	return unique
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func scanSource(path string, info os.FileInfo, checkpoint metadata.HistorySource) (metadata.HistorySource, []metadata.HistorySuggestion, error) {
	file, err := os.Open(path)
	if err != nil {
		return checkpoint, nil, err
	}
	defer file.Close()
	size := info.Size()
	format, err := detectFormat(file, path, size)
	if err != nil {
		return checkpoint, nil, err
	}

	start := int64(0)
	skipRecords := int64(0)
	anchorsComplete := false
	emit := checkpoint.Offset == 0 && checkpoint.TailHash == "" && len(checkpoint.Anchors) == 0
	anchors := []string(nil)
	if !emit && checkpoint.Offset <= size && checkpoint.TailHash != "" {
		valid, hashErr := matchesTail(file, checkpoint.Offset, checkpoint.TailHash)
		if hashErr != nil {
			return checkpoint, nil, hashErr
		}
		if valid {
			start, emit, anchors = checkpoint.Offset, true, append([]string(nil), checkpoint.Anchors...)
		}
	}

	if !emit && len(checkpoint.Anchors) > 0 {
		matchAfter, lastAnchors, walkErr := locateAnchors(io.NewSectionReader(file, 0, size), format, checkpoint.Anchors)
		if walkErr != nil {
			return checkpoint, nil, walkErr
		}
		anchors = lastAnchors
		anchorsComplete = true
		if matchAfter >= 0 {
			start = 0
			skipRecords = matchAfter
			emit = true
		}
	}

	var suggestions []metadata.HistorySuggestion
	var newAnchors []string
	if emit {
		reader := io.NewSectionReader(file, start, size-start)
		index := int64(0)
		err = walkRecords(reader, format, func(item record) error {
			index++
			if index <= skipRecords {
				return nil
			}
			newAnchors = appendAnchor(newAnchors, recordHash(item.command))
			suggestion, ok := parseSSH(item.command)
			if !ok {
				return nil
			}
			suggestion.Source = format
			if item.seenAt != 0 {
				suggestion.SeenAt = item.seenAt * 1_000_000_000
			} else {
				suggestion.SeenAt = info.ModTime().UnixNano() + index
			}
			suggestions = append(suggestions, suggestion)
			return nil
		})
		if err != nil {
			return checkpoint, nil, err
		}
		if !anchorsComplete {
			anchors = appendAnchors(anchors, newAnchors)
		}
	}
	if len(anchors) == 0 {
		var walkErr error
		anchors, walkErr = lastRecordAnchors(io.NewSectionReader(file, 0, size), format)
		if walkErr != nil {
			return checkpoint, nil, walkErr
		}
	}
	tail, err := tailHash(file, size)
	if err != nil {
		return checkpoint, nil, err
	}
	return metadata.HistorySource{Offset: size, TailHash: tail, Anchors: anchors}, suggestions, nil
}

func detectFormat(file *os.File, path string, size int64) (string, error) {
	name := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(name, "zsh"):
		return "zsh", nil
	case strings.Contains(name, "bash"):
		return "bash", nil
	case strings.HasSuffix(name, "_history") && strings.Contains(filepath.ToSlash(path), "/fish/"):
		return "fish", nil
	}
	peek := make([]byte, min(int64(8192), size))
	if _, err := file.ReadAt(peek, 0); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	text := string(peek)
	if strings.Contains(text, "\n- cmd: ") || strings.HasPrefix(text, "- cmd: ") {
		return "fish", nil
	}
	if strings.HasPrefix(strings.TrimSpace(text), ": ") {
		return "zsh", nil
	}
	return "bash", nil
}

func shellName(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(path, "fish"):
		return "fish"
	case strings.Contains(path, "bash"):
		return "bash"
	default:
		return "zsh"
	}
}

func matchesTail(file *os.File, offset int64, want string) (bool, error) {
	got, err := tailHash(file, offset)
	return got == want, err
}

func tailHash(file *os.File, offset int64) (string, error) {
	start := max(int64(0), offset-anchorBytes)
	data := make([]byte, offset-start)
	if len(data) > 0 {
		if _, err := file.ReadAt(data, start); err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func locateAnchors(reader io.Reader, format string, want []string) (int64, []string, error) {
	window := make([]string, 0, len(want))
	lastMatch := int64(-1)
	var records int64
	last := []string(nil)
	err := walkRecords(reader, format, func(item record) error {
		records++
		hash := recordHash(item.command)
		last = appendAnchor(last, hash)
		window = append(window, hash)
		if len(window) > len(want) {
			window = window[1:]
		}
		if len(window) == len(want) && slices.Equal(window, want) {
			lastMatch = records
		}
		return nil
	})
	return lastMatch, last, err
}

func lastRecordAnchors(reader io.Reader, format string) ([]string, error) {
	var anchors []string
	err := walkRecords(reader, format, func(item record) error {
		anchors = appendAnchor(anchors, recordHash(item.command))
		return nil
	})
	return anchors, err
}

func appendAnchor(anchors []string, hash string) []string {
	anchors = append(anchors, hash)
	if len(anchors) > anchorCount {
		anchors = anchors[len(anchors)-anchorCount:]
	}
	return anchors
}

func appendAnchors(current, added []string) []string {
	for _, hash := range added {
		current = appendAnchor(current, hash)
	}
	return current
}

func recordHash(command string) string {
	hash := sha256.Sum256([]byte(command))
	return hex.EncodeToString(hash[:])
}

func walkRecords(reader io.Reader, format string, visit func(record) error) error {
	call := func(command string, seenAt int64) error {
		command = strings.TrimSpace(command)
		if command == "" {
			return nil
		}
		return visit(record{command: command, seenAt: seenAt})
	}
	walkLines := func(handle func(line string, oversized bool) error) error {
		buffered := bufio.NewReaderSize(reader, 64*1024)
		var line strings.Builder
		oversized := false
		for {
			fragment, more, err := buffered.ReadLine()
			if errors.Is(err, io.EOF) && len(fragment) == 0 && line.Len() == 0 && !oversized {
				return nil
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			if !oversized {
				if line.Len()+len(fragment) > maxRecordLen {
					line.Reset()
					oversized = true
				} else {
					_, _ = line.Write(fragment)
				}
			}
			if !more {
				if err := handle(line.String(), oversized); err != nil {
					return err
				}
				line.Reset()
				oversized = false
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
		}
	}

	switch format {
	case "fish":
		command := ""
		var seenAt int64
		if err := walkLines(func(line string, oversized bool) error {
			if oversized {
				err := call(command, seenAt)
				command, seenAt = "", 0
				return err
			}
			if strings.HasPrefix(line, "- cmd: ") {
				if err := call(command, seenAt); err != nil {
					return err
				}
				command, seenAt = unescapeFish(strings.TrimPrefix(line, "- cmd: ")), 0
			} else if command != "" && strings.HasPrefix(line, "  when: ") {
				seenAt, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "  when: ")), 10, 64)
			}
			return nil
		}); err != nil {
			return err
		}
		if err := call(command, seenAt); err != nil {
			return err
		}
	case "zsh":
		return walkLines(func(line string, oversized bool) error {
			if oversized {
				return nil
			}
			command, seenAt := parseZshRecord(line)
			return call(command, seenAt)
		})
	default:
		var command strings.Builder
		var seenAt int64
		timestamped := false
		discard := false
		flush := func() error {
			var err error
			if !discard {
				err = call(command.String(), seenAt)
			}
			command.Reset()
			discard = false
			return err
		}
		if err := walkLines(func(line string, oversized bool) error {
			if timestamp, ok := bashTimestamp(line); ok {
				if err := flush(); err != nil {
					return err
				}
				seenAt, timestamped = timestamp, true
				return nil
			}
			if oversized {
				if timestamped {
					command.Reset()
					discard = true
				}
				return nil
			}
			if timestamped {
				if discard {
					return nil
				}
				length := command.Len() + len(line)
				if command.Len() > 0 {
					length++
				}
				if length > maxRecordLen {
					command.Reset()
					discard = true
					return nil
				}
				if command.Len() > 0 {
					_ = command.WriteByte('\n')
				}
				_, _ = command.WriteString(line)
			} else if err := call(line, 0); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
		if err := flush(); err != nil {
			return err
		}
	}
	return nil
}

func parseZshRecord(line string) (string, int64) {
	if !strings.HasPrefix(line, ": ") {
		return line, 0
	}
	rest := strings.TrimPrefix(line, ": ")
	firstColon := strings.IndexByte(rest, ':')
	semicolon := strings.IndexByte(rest, ';')
	if firstColon < 1 || semicolon < firstColon {
		return line, 0
	}
	timestamp, err := strconv.ParseInt(rest[:firstColon], 10, 64)
	if err != nil {
		return line, 0
	}
	return rest[semicolon+1:], timestamp
}

func bashTimestamp(line string) (int64, bool) {
	if len(line) < 2 || line[0] != '#' {
		return 0, false
	}
	value, err := strconv.ParseInt(line[1:], 10, 64)
	return value, err == nil
}

func unescapeFish(value string) string {
	quoted := `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	if unquoted, err := strconv.Unquote(quoted); err == nil {
		return unquoted
	}
	return strings.ReplaceAll(strings.ReplaceAll(value, `\n`, "\n"), `\\`, `\`)
}

func parseSSH(command string) (metadata.HistorySuggestion, bool) {
	words, ok := shellWords(command)
	if !ok || len(words) < 2 {
		return metadata.HistorySuggestion{}, false
	}
	if words[0] == "command" || words[0] == "exec" {
		words = words[1:]
		if len(words) > 0 && words[0] == "--" {
			words = words[1:]
		}
	}
	if len(words) < 2 || filepath.Base(words[0]) != "ssh" {
		return metadata.HistorySuggestion{}, false
	}

	var user, hostname, port, identityFile, proxyJump, target string
	for i := 1; i < len(words); i++ {
		word := words[i]
		if word == "--" {
			i++
			if i >= len(words) {
				return metadata.HistorySuggestion{}, false
			}
			target = words[i]
			break
		}
		if !strings.HasPrefix(word, "-") || word == "-" {
			target = word
			break
		}
		name, value, attached := sshOption(word)
		switch name {
		case "l", "p", "i", "J", "o":
			if !attached {
				i++
				if i >= len(words) {
					return metadata.HistorySuggestion{}, false
				}
				value = words[i]
			}
			switch name {
			case "l":
				user = value
			case "p":
				port = value
			case "i":
				if safeIdentityPath(value) {
					identityFile = value
				}
			case "J":
				proxyJump = value
			case "o":
				key, optionValue := sshConfigOption(value)
				switch strings.ToLower(key) {
				case "hostname":
					hostname = optionValue
				case "user":
					user = optionValue
				case "port":
					port = optionValue
				case "identityfile":
					if safeIdentityPath(optionValue) {
						identityFile = optionValue
					}
				case "proxyjump":
					proxyJump = optionValue
				case "addressfamily", "batchmode", "compression", "connecttimeout", "forwardagent", "ipqos", "loglevel", "requesttty", "serveralivecountmax", "serveraliveinterval", "stricthostkeychecking", "userknownhostsfile", "localforward", "remoteforward", "dynamicforward":
					// These affect a single invocation but not the imported endpoint.
				default:
					return metadata.HistorySuggestion{}, false
				}
			}
		case "F":
			return metadata.HistorySuggestion{}, false
		case "B", "b", "c", "D", "E", "e", "I", "L", "m", "P", "R", "S", "W", "w":
			if !attached {
				i++
				if i >= len(words) {
					return metadata.HistorySuggestion{}, false
				}
			}
		case "G", "O", "Q", "V":
			return metadata.HistorySuggestion{}, false
		case "4", "6", "A", "a", "C", "f", "g", "K", "k", "M", "N", "n", "q", "s", "T", "t", "v", "X", "x", "Y", "y":
		default:
			return metadata.HistorySuggestion{}, false
		}
	}
	if target == "" || hasUnsafeShellChars(target) {
		return metadata.HistorySuggestion{}, false
	}
	targetHost := target
	if at := strings.LastIndex(target, "@"); at >= 0 {
		if user == "" {
			user = target[:at]
		}
		targetHost = target[at+1:]
	}
	targetHost = strings.TrimSuffix(strings.TrimPrefix(targetHost, "["), "]")
	if hostname == "" {
		hostname = targetHost
	}
	if hostname == "" || hasUnsafeShellChars(hostname) || hasUnsafeShellChars(user) || hasUnsafeShellChars(proxyJump) {
		return metadata.HistorySuggestion{}, false
	}
	input := sshconfig.HostInput{Alias: baseAlias(user, hostname), HostName: hostname, User: user, Port: port, IdentityFile: identityFile, ProxyJump: proxyJump}
	if err := sshconfig.Validate(input); err != nil {
		return metadata.HistorySuggestion{}, false
	}
	id := endpointID(user, hostname, port)
	return metadata.HistorySuggestion{
		ID: id, Alias: input.Alias, Target: targetHost, HostName: hostname, User: user,
		Port: port, IdentityFile: identityFile, ProxyJump: proxyJump,
	}, true
}

func sshOption(word string) (name, value string, attached bool) {
	trimmed := strings.TrimPrefix(word, "-")
	if trimmed == "" {
		return "", "", false
	}
	name = trimmed[:1]
	if len(trimmed) > 1 {
		return name, trimmed[1:], true
	}
	return name, "", false
}

func sshConfigOption(option string) (string, string) {
	if key, value, ok := strings.Cut(option, "="); ok {
		return strings.TrimSpace(key), strings.TrimSpace(value)
	}
	parts := strings.Fields(option)
	if len(parts) >= 2 {
		return parts[0], strings.Join(parts[1:], " ")
	}
	return option, ""
}

func safeIdentityPath(path string) bool {
	if path == "" || strings.ContainsAny(path, "\r\n\x00`;|&<>") {
		return false
	}
	if strings.Contains(path, "$") {
		end := strings.IndexByte(path, '}')
		if !strings.HasPrefix(path, "${") || end < 3 || strings.Contains(path[end+1:], "$") {
			return false
		}
		for _, char := range path[2:end] {
			if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' {
				return false
			}
		}
	}
	return filepath.IsAbs(path) || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "${") || strings.HasPrefix(path, "%")
}

func hasUnsafeShellChars(value string) bool {
	return strings.ContainsAny(value, "\r\n\x00`$;|&<>")
}

func shellWords(command string) ([]string, bool) {
	var words []string
	var word strings.Builder
	quote := rune(0)
	escaped := false
	started := false
	flush := func() {
		if started {
			words = append(words, word.String())
			word.Reset()
			started = false
		}
	}
	for _, char := range command {
		if escaped {
			word.WriteRune(char)
			started, escaped = true, false
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else if char == '\\' && quote == '"' {
				escaped = true
			} else {
				word.WriteRune(char)
				started = true
			}
			continue
		}
		switch {
		case char == '\\':
			escaped, started = true, true
		case char == '\'' || char == '"':
			quote, started = char, true
		case unicode.IsSpace(char):
			flush()
		case char == '#' && !started:
			flush()
			return words, true
		case strings.ContainsRune(";|&", char):
			flush()
			return words, true
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, false
	}
	flush()
	return words, true
}

func endpointID(user, hostname, port string) string {
	hash := sha256.Sum256([]byte(endpointKey(user, hostname, port)))
	return hex.EncodeToString(hash[:])
}

func endpointKey(user, hostname, port string) string {
	if port == "" {
		port = "22"
	}
	return strings.ToLower(strings.TrimSpace(user)) + "\x00" + strings.ToLower(strings.TrimSpace(hostname)) + "\x00" + port
}

func existingHosts(hosts []sshconfig.Host) (map[string]bool, map[string]bool) {
	aliases := make(map[string]bool, len(hosts))
	endpoints := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		aliases[strings.ToLower(host.Alias)] = true
		hostname := host.Resolved.HostName
		if hostname == "" {
			hostname = host.Alias
		}
		endpoints[endpointKey(host.Resolved.User, hostname, host.Resolved.Port)] = true
	}
	return aliases, endpoints
}

func assignAliases(suggestions []metadata.HistorySuggestion, existing map[string]bool) {
	used := make(map[string]bool, len(existing)+len(suggestions))
	for alias := range existing {
		used[alias] = true
	}
	for i := range suggestions {
		base := baseAlias(suggestions[i].User, suggestions[i].HostName)
		alias := base
		for suffix := 2; used[strings.ToLower(alias)]; suffix++ {
			alias = fmt.Sprintf("%s-%d", base, suffix)
		}
		suggestions[i].Alias = alias
		used[strings.ToLower(alias)] = true
	}
}

func baseAlias(user, hostname string) string {
	value := strings.ToLower(hostname)
	if user != "" {
		value = strings.ToLower(user) + "-" + value
	}
	var alias strings.Builder
	lastDash := false
	for _, char := range value {
		allowed := unicode.IsLetter(char) || unicode.IsDigit(char) || char == '.' || char == '_' || char == '-'
		if allowed {
			alias.WriteRune(char)
			lastDash = char == '-'
		} else if !lastDash {
			alias.WriteByte('-')
			lastDash = true
		}
	}
	clean := strings.Trim(alias.String(), ".-_")
	if clean == "" {
		return "host"
	}
	return clean
}
