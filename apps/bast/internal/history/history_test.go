package history

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"bast/internal/metadata"
	"bast/internal/sshconfig"
)

func TestParseSSH(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    metadata.HistorySuggestion
		ok      bool
	}{
		{name: "destination", command: "ssh ubuntu@example.com", want: metadata.HistorySuggestion{Alias: "ubuntu-example.com", Target: "example.com", HostName: "example.com", User: "ubuntu"}, ok: true},
		{name: "options", command: `ssh -p2222 -i ~/.ssh/work -J jump -l root example.com uptime`, want: metadata.HistorySuggestion{Alias: "root-example.com", Target: "example.com", HostName: "example.com", User: "root", Port: "2222", IdentityFile: "~/.ssh/work", ProxyJump: "jump"}, ok: true},
		{name: "config options", command: `command ssh -o User=deploy -oHostName=10.0.0.4 -o "Port 2200" prod`, want: metadata.HistorySuggestion{Alias: "deploy-10.0.0.4", Target: "prod", HostName: "10.0.0.4", User: "deploy", Port: "2200"}, ok: true},
		{name: "quoted", command: `exec -- /usr/bin/ssh "dev@example.com" && echo done`, want: metadata.HistorySuggestion{Alias: "dev-example.com", Target: "example.com", HostName: "example.com", User: "dev"}, ok: true},
		{name: "relative identity omitted", command: `ssh -i keys/work.pem host`, want: metadata.HistorySuggestion{Alias: "host", Target: "host", HostName: "host"}, ok: true},
		{name: "environment identity", command: `ssh -i '${HOME}/.ssh/work' host`, want: metadata.HistorySuggestion{Alias: "host", Target: "host", HostName: "host", IdentityFile: "${HOME}/.ssh/work"}, ok: true},
		{name: "custom config rejected", command: "ssh -F other.conf prod", ok: false},
		{name: "proxy command rejected", command: `ssh -o ProxyCommand="nc gateway %h %p" prod`, ok: false},
		{name: "config query rejected", command: "ssh -G prod", ok: false},
		{name: "expansion rejected", command: `ssh "$TARGET"`, ok: false},
		{name: "nested command rejected", command: "cd /tmp && ssh prod", ok: false},
		{name: "not ssh", command: "scp file prod:/tmp", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseSSH(test.command)
			if ok != test.ok {
				t.Fatalf("parseSSH ok = %v, want %v: %+v", ok, test.ok, got)
			}
			if !ok {
				return
			}
			got.ID = ""
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseSSH = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestWalkRecords(t *testing.T) {
	tests := []struct {
		name   string
		format string
		input  string
		want   []record
	}{
		{name: "zsh extended", format: "zsh", input: ": 1700000000:2;ssh one\nssh two\n", want: []record{{command: "ssh one", seenAt: 1700000000}, {command: "ssh two"}}},
		{name: "bash timestamps", format: "bash", input: "#1700000000\nssh one\n#1700000001\nssh two\n", want: []record{{command: "ssh one", seenAt: 1700000000}, {command: "ssh two", seenAt: 1700000001}}},
		{name: "bash multiline", format: "bash", input: "#1700000000\necho one \\\n  two\n#1700000001\nssh next\n", want: []record{{command: "echo one \\\n  two", seenAt: 1700000000}, {command: "ssh next", seenAt: 1700000001}}},
		{name: "bash plain", format: "bash", input: "ls\nssh one\n", want: []record{{command: "ls"}, {command: "ssh one"}}},
		{name: "oversized record", format: "bash", input: "ssh before\n" + strings.Repeat("x", maxRecordLen+1) + "\nssh after\n", want: []record{{command: "ssh before"}, {command: "ssh after"}}},
		{name: "fish", format: "fish", input: "- cmd: ssh one\n  when: 1700000000\n- cmd: ssh two\n  when: 1700000001\n", want: []record{{command: "ssh one", seenAt: 1700000000}, {command: "ssh two", seenAt: 1700000001}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []record
			if err := walkRecords(strings.NewReader(test.input), test.format, func(item record) error {
				got = append(got, item)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("records = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestScanPersistsPendingAndReadsOnlyNewRecords(t *testing.T) {
	home := t.TempDir()
	zsh := filepath.Join(home, ".zsh_history")
	writeFile(t, zsh, ": 1700000000:0;ssh ubuntu@example.com\nssh root@example.com\nssh existing\n")
	getenv := func(string) string { return "" }
	existing := []sshconfig.Host{{Alias: "existing"}}

	first, errs := Scan(home, getenv, metadata.HistoryImport{}, existing)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(first.Pending) != 2 {
		t.Fatalf("pending = %+v", first.Pending)
	}
	if first.Pending[0].Alias != "root-example.com" || first.Pending[1].Alias != "ubuntu-example.com" {
		t.Fatalf("unexpected ordering or aliases: %+v", first.Pending)
	}

	first.Pending = first.Pending[1:]
	second, errs := Scan(home, getenv, first, existing)
	if len(errs) != 0 || len(second.Pending) != 1 || second.Pending[0].User != "ubuntu" {
		t.Fatalf("old dismissed entry returned: pending=%+v errors=%v", second.Pending, errs)
	}

	appendFile(t, zsh, "ssh root@example.com\n")
	third, errs := Scan(home, getenv, second, existing)
	if len(errs) != 0 || len(third.Pending) != 2 {
		t.Fatalf("new occurrence was not suggested: pending=%+v errors=%v", third.Pending, errs)
	}
}

func TestScanRecoversAfterHistoryRewriteWithoutReplaying(t *testing.T) {
	home := t.TempDir()
	zsh := filepath.Join(home, ".zsh_history")
	var initial strings.Builder
	for i := 0; i < 12; i++ {
		initial.WriteString("echo ")
		initial.WriteString(string(rune('a' + i)))
		initial.WriteByte('\n')
	}
	initial.WriteString("ssh old.example.com\n")
	writeFile(t, zsh, initial.String())
	first, errs := Scan(home, func(string) string { return "" }, metadata.HistoryImport{}, nil)
	if len(errs) != 0 || len(first.Pending) != 1 {
		t.Fatalf("initial scan: pending=%+v errors=%v", first.Pending, errs)
	}
	first.Pending = nil

	lines := strings.Split(strings.TrimSpace(initial.String()), "\n")
	rewritten := strings.Join(lines[4:], "\n") + "\nssh new.example.com\n"
	writeFile(t, zsh, rewritten)
	second, errs := Scan(home, func(string) string { return "" }, first, nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(second.Pending) != 1 || second.Pending[0].HostName != "new.example.com" {
		t.Fatalf("rewrite pending = %+v", second.Pending)
	}

	writeFile(t, zsh, "ssh old.example.com\nssh unrelated.example.com\n")
	second.Pending = nil
	third, errs := Scan(home, func(string) string { return "" }, second, nil)
	if len(errs) != 0 || len(third.Pending) != 0 {
		t.Fatalf("unanchored rewrite replayed entries: pending=%+v errors=%v", third.Pending, errs)
	}
}

func TestScanAdvancesPastOversizedRecord(t *testing.T) {
	home := t.TempDir()
	bash := filepath.Join(home, ".bash_history")
	writeFile(t, bash, "ssh before.example.com\n"+strings.Repeat("x", maxRecordLen+1)+"\nssh after.example.com\n")

	first, errs := Scan(home, func(string) string { return "" }, metadata.HistoryImport{}, nil)
	if len(errs) != 0 || len(first.Pending) != 2 {
		t.Fatalf("initial scan: pending=%+v errors=%v", first.Pending, errs)
	}
	if first.Sources[bash].Offset != int64(len("ssh before.example.com\n")+maxRecordLen+1+len("\nssh after.example.com\n")) {
		t.Fatalf("checkpoint did not advance: %+v", first.Sources[bash])
	}

	first.Pending = nil
	second, errs := Scan(home, func(string) string { return "" }, first, nil)
	if len(errs) != 0 || len(second.Pending) != 0 {
		t.Fatalf("oversized record was replayed: pending=%+v errors=%v", second.Pending, errs)
	}
}

func TestScanAllStandardHistoriesAndDeduplicatesEndpoints(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".zsh_history"), "ssh dev@shared.example.com\n")
	writeFile(t, filepath.Join(home, ".zhistory"), "ssh zhistory.example.com\n")
	writeFile(t, filepath.Join(home, ".bash_history"), "ssh dev@shared.example.com\nssh bash.example.com\n")
	writeFile(t, filepath.Join(home, ".local", "share", "fish", "fish_history"), "- cmd: ssh fish.example.com\n  when: 1700000000\n")
	state, errs := Scan(home, func(string) string { return "" }, metadata.HistoryImport{}, nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(state.Pending) != 4 {
		t.Fatalf("pending = %+v", state.Pending)
	}
}

func TestSourcePathsIncludeCommonZshLocations(t *testing.T) {
	home := t.TempDir()
	zdotdir := filepath.Join(home, "zsh")
	paths := sourcePaths(home, func(name string) string {
		if name == "ZDOTDIR" {
			return zdotdir
		}
		return ""
	})
	want := []string{
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".zhistory"),
		filepath.Join(zdotdir, ".zsh_history"),
		filepath.Join(zdotdir, ".zhistory"),
	}
	for _, path := range want {
		if !slices.Contains(paths, path) {
			t.Errorf("source paths do not include %q: %v", path, paths)
		}
	}
}

func TestSourcePathsEnvironmentLocations(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		name    string
		env     map[string]string
		want    string
		wantLen int
	}{
		{name: "HISTFILE", env: map[string]string{"HISTFILE": "~/custom/history"}, want: filepath.Join(home, "custom", "history"), wantLen: 5},
		{name: "XDG_DATA_HOME", env: map[string]string{"XDG_DATA_HOME": "~/data"}, want: filepath.Join(home, "data", "fish", "fish_history"), wantLen: 4},
		{name: "fish session", env: map[string]string{"fish_history": "work"}, want: filepath.Join(home, ".local", "share", "fish", "work_history"), wantLen: 4},
		{name: "clean deduplication", env: map[string]string{"HISTFILE": filepath.Join(home, "nested", "..", ".zhistory")}, want: filepath.Join(home, ".zhistory"), wantLen: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := sourcePaths(home, func(name string) string { return test.env[name] })
			if !slices.Contains(paths, test.want) {
				t.Fatalf("source paths do not include %q: %v", test.want, paths)
			}
			if len(paths) != test.wantLen {
				t.Fatalf("source path count = %d, want %d: %v", len(paths), test.wantLen, paths)
			}
		})
	}
}

func TestScanKeepsNewestPendingSuggestions(t *testing.T) {
	previous := metadata.HistoryImport{Pending: make([]metadata.HistorySuggestion, maxPending+5)}
	for i := range previous.Pending {
		previous.Pending[i] = metadata.HistorySuggestion{
			ID:       fmt.Sprintf("suggestion-%03d", i),
			Target:   fmt.Sprintf("host-%03d.example.com", i),
			HostName: fmt.Sprintf("host-%03d.example.com", i),
			SeenAt:   int64(i),
		}
	}

	state, errs := Scan(t.TempDir(), func(string) string { return "" }, previous, nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(state.Pending) != maxPending {
		t.Fatalf("pending count = %d, want %d", len(state.Pending), maxPending)
	}
	if state.Pending[0].ID != "suggestion-204" || state.Pending[maxPending-1].ID != "suggestion-005" {
		t.Fatalf("pending bounds = %q ... %q", state.Pending[0].ID, state.Pending[maxPending-1].ID)
	}
}

func TestScanLargeHistoryRetainsOnlyUniqueSSHDestinations(t *testing.T) {
	home := t.TempDir()
	var content strings.Builder
	for i := 0; i < 25_000; i++ {
		content.WriteString("echo ordinary-command\n")
	}
	content.WriteString("ssh ops@large.example.com\nssh ops@large.example.com\n")
	writeFile(t, filepath.Join(home, ".bash_history"), content.String())
	state, errs := Scan(home, func(string) string { return "" }, metadata.HistoryImport{}, nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(state.Pending) != 1 || state.Pending[0].Alias != "ops-large.example.com" {
		t.Fatalf("pending = %+v", state.Pending)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
