package tunnel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Forward modes. Host, user, port and identity come from the host's OpenSSH
// config, so a Spec never repeats them.
const (
	ModeLocal       = "local"
	ModeRemote      = "remote"
	ModeSOCKS       = "socks"
	ModeSOCKSRemote = "socks-remote"
)

// Connection states reported by Status.
const (
	StateConfigured    = "configured"
	StateConnecting    = "connecting"
	StateConnected     = "connected"
	StateReconnecting  = "reconnecting"
	StateDisconnected  = "disconnected"
	StateError         = "error"
	defaultKeepAlive   = 30
	healthyAfter       = 30 * time.Second
	settleTime         = 1500 * time.Millisecond
	maxBackoff         = 60 * time.Second
	maxLogBytes        = 1 << 20
	maxEndpointsPerRun = 256
)

// Spec describes one named tunnel.
//
//	local:        Local = listen [addr:]port or socket, Remote = host:port or socket
//	remote:       Remote = listen [addr:]port or socket on the server, Local = [host:]port or socket
//	socks:        Local = listen [addr:]port
//	socks-remote: Remote = listen [addr:]port on the server
//
// Port lists ("8080,8090") and ranges ("8000-8003") may be used for ports.
// Lists on both sides must expand to the same number of endpoints.
type Spec struct {
	Name      string
	Host      string
	Mode      string
	Local     string
	Remote    string
	Group     string
	KeepAlive int
}

// Validate reports the first problem as "tunnel NAME: field: reason".
func Validate(s Spec) error {
	bad := func(field, why string) error {
		return fmt.Errorf("tunnel %q: %s: %s", s.Name, field, why)
	}
	if s.Name == "" || strings.ContainsAny(s.Name, " \t\r\n\x00/\\*?[]") || strings.HasPrefix(s.Name, "-") {
		return bad("name", "use letters, digits, dots, dashes or underscores")
	}
	if err := validateAlias(s.Host); err != nil {
		return bad("host", "an SSH host alias is required")
	}
	if s.KeepAlive < 0 || s.KeepAlive > 3600 {
		return bad("keep_alive", "use 0-3600 seconds")
	}
	switch s.Mode {
	case ModeLocal, ModeRemote, ModeSOCKS, ModeSOCKSRemote:
	default:
		return bad("mode", "use local, remote, socks or socks-remote")
	}
	needLocal := s.Mode != ModeSOCKSRemote
	needRemote := s.Mode == ModeLocal || s.Mode == ModeRemote || s.Mode == ModeSOCKSRemote
	if needLocal && strings.TrimSpace(s.Local) == "" {
		return bad("local", "required for mode "+s.Mode)
	}
	if needRemote && strings.TrimSpace(s.Remote) == "" {
		return bad("remote", "required for mode "+s.Mode)
	}
	if !needLocal && s.Local != "" {
		return bad("local", "not used by mode "+s.Mode)
	}
	if !needRemote && s.Remote != "" {
		return bad("remote", "not used by mode "+s.Mode)
	}
	if _, err := forwardSpecs(s); err != nil {
		return err
	}
	return nil
}

// Args builds the ssh command line. ClearAllForwardings is not used: OpenSSH
// would also drop the -L/-R/-D flags given here.
func Args(s Spec) ([]string, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}
	forwards, _ := forwardSpecs(s)
	keep := s.KeepAlive
	if keep == 0 {
		keep = defaultKeepAlive
	}
	args := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=" + strconv.Itoa(keep),
		"-o", "ServerAliveCountMax=3",
	}
	if os.Getenv("BAST_DEBUG") != "" {
		args = append(args, "-v")
	}
	flag := map[string]string{ModeLocal: "-L", ModeRemote: "-R", ModeSOCKS: "-D", ModeSOCKSRemote: "-R"}[s.Mode]
	for _, f := range forwards {
		args = append(args, flag, f)
	}
	return append(args, "--", s.Host), nil
}

// ListenAddrs returns the local TCP addresses ssh will bind, for conflict checks.
func ListenAddrs(s Spec) []string {
	if s.Mode != ModeLocal && s.Mode != ModeSOCKS {
		return nil
	}
	items, err := expand(s.Local)
	if err != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		if !isSocket(item) {
			out = append(out, bindAddr(item))
		}
	}
	return out
}

func forwardSpecs(s Spec) ([]string, error) {
	bad := func(field string, err error) error {
		return fmt.Errorf("tunnel %q: %s: %w", s.Name, field, err)
	}
	var local, remote []string
	var err error
	if s.Local != "" {
		if local, err = expand(s.Local); err != nil {
			return nil, bad("local", err)
		}
	}
	if s.Remote != "" {
		if remote, err = expand(s.Remote); err != nil {
			return nil, bad("remote", err)
		}
	}
	switch s.Mode {
	case ModeSOCKS:
		out := make([]string, len(local))
		for i, l := range local {
			if isSocket(l) {
				return nil, bad("local", errors.New("a SOCKS proxy needs a TCP port"))
			}
			out[i] = bindAddr(l)
		}
		return out, nil
	case ModeSOCKSRemote:
		out := make([]string, len(remote))
		for i, r := range remote {
			if isSocket(r) {
				return nil, bad("remote", errors.New("a SOCKS proxy needs a TCP port"))
			}
			out[i] = r
		}
		return out, nil
	}
	if len(local) != len(remote) {
		return nil, bad("remote", fmt.Errorf("%d local endpoint(s) but %d remote; they must match", len(local), len(remote)))
	}
	out := make([]string, len(local))
	for i := range local {
		l, r := local[i], remote[i]
		if s.Mode == ModeLocal {
			if !isSocket(l) {
				l = bindAddr(l)
			}
			if !isSocket(r) {
				if _, _, err := net.SplitHostPort(r); err != nil {
					return nil, bad("remote", errors.New("use host:port or a socket path"))
				}
			}
			out[i] = l + ":" + r
			continue
		}
		if !isSocket(l) && !strings.Contains(strings.Trim(l, "[]"), ":") && !strings.Contains(l, ".") && l != "localhost" {
			l = "localhost:" + l
		} else if !isSocket(l) {
			if _, _, err := net.SplitHostPort(l); err != nil {
				l = "localhost:" + l
			}
		}
		out[i] = r + ":" + l
	}
	return out, nil
}

// expand turns "8000-8002,9000" or "host:80-82" into single endpoints.
func expand(list string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if err := checkEndpoint(part); err != nil {
			return nil, err
		}
		if isSocket(part) {
			out = append(out, part)
			continue
		}
		host, port := "", part
		if i := strings.LastIndex(part, ":"); i >= 0 && !strings.HasSuffix(part, "]") {
			host, port = part[:i], part[i+1:]
		}
		lo, hi, err := portRange(port)
		if err != nil {
			return nil, err
		}
		if len(out)+hi-lo+1 > maxEndpointsPerRun {
			return nil, errors.New("too many ports")
		}
		for p := lo; p <= hi; p++ {
			if host == "" {
				out = append(out, strconv.Itoa(p))
			} else {
				out = append(out, host+":"+strconv.Itoa(p))
			}
		}
	}
	return out, nil
}

func portRange(s string) (int, int, error) {
	loS, hiS, isRange := strings.Cut(s, "-")
	lo, err := strconv.Atoi(loS)
	hi := lo
	if err == nil && isRange {
		hi, err = strconv.Atoi(hiS)
	}
	if err != nil || lo < 1 || hi > 65535 || hi < lo {
		return 0, 0, fmt.Errorf("%q is not a valid port or range", s)
	}
	return lo, hi, nil
}

func checkEndpoint(s string) error {
	if s == "" || strings.HasPrefix(s, "-") {
		return errors.New("empty or invalid endpoint")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == ' ' || r == ';' || r == '"' || r == '\'' {
			return fmt.Errorf("%q has characters that can't be used", s)
		}
	}
	return nil
}

func isSocket(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "~/")
}

func bindAddr(s string) string {
	if strings.Contains(s, ":") {
		return s
	}
	return "127.0.0.1:" + s
}

// Match returns the names selected by any of the glob patterns. A pattern
// without wildcards must match exactly. Unknown exact names are returned in
// missing so callers can report them.
func Match(patterns, names []string) (matched, missing []string) {
	seen := map[string]bool{}
	for _, p := range patterns {
		hit := false
		for _, n := range names {
			if ok, _ := path.Match(p, n); ok {
				hit = true
				if !seen[n] {
					seen[n] = true
					matched = append(matched, n)
				}
			}
		}
		if !hit {
			missing = append(missing, p)
		}
	}
	sort.Strings(matched)
	return matched, missing
}

// ErrorKind names why ssh failed, so callers can retry or stop.
type ErrorKind string

const (
	KindAuth    ErrorKind = "auth"
	KindHostKey ErrorKind = "hostkey"
	KindDNS     ErrorKind = "dns"
	KindPort    ErrorKind = "port"
	KindNetwork ErrorKind = "network"
	KindOther   ErrorKind = "other"
)

// Classify sorts ssh stderr into a failure kind.
func Classify(stderr string) ErrorKind {
	t := strings.ToLower(stderr)
	switch {
	case strings.Contains(t, "host key verification failed"), strings.Contains(t, "remote host identification has changed"):
		return KindHostKey
	case strings.Contains(t, "permission denied"), strings.Contains(t, "too many authentication failures"), strings.Contains(t, "no more authentication methods"):
		return KindAuth
	case strings.Contains(t, "could not resolve hostname"), strings.Contains(t, "name or service not known"), strings.Contains(t, "nodename nor servname"):
		return KindDNS
	case strings.Contains(t, "address already in use"), strings.Contains(t, "cannot listen to port"), strings.Contains(t, "remote port forwarding failed"), strings.Contains(t, "could not request local forwarding"):
		return KindPort
	case strings.Contains(t, "connection refused"), strings.Contains(t, "timed out"), strings.Contains(t, "connection reset"), strings.Contains(t, "broken pipe"), strings.Contains(t, "network is unreachable"), strings.Contains(t, "no route to host"), strings.Contains(t, "closed by remote host"):
		return KindNetwork
	default:
		return KindOther
	}
}

// retryable says whether another attempt can help. Auth, host-key and port
// problems need the user, so the runner stops and reports them.
func retryable(k ErrorKind) bool {
	return k != KindAuth && k != KindHostKey && k != KindPort
}

// Status is the on-disk view of one tunnel's runner.
type Status struct {
	Name     string    `json:"name"`
	PID      int       `json:"pid"`
	State    string    `json:"state"`
	Kind     ErrorKind `json:"kind,omitempty"`
	Error    string    `json:"error,omitempty"`
	Attempts int       `json:"attempts,omitempty"`
	Since    time.Time `json:"since"`
}

// Manager starts and stops per-tunnel runner processes. Each runner is a
// detached `bast tunnel run NAME`, so the CLI and TUI can exit while tunnels
// stay up, and one runner dying never touches another.
type Manager struct {
	Dir string // directory for state and logs
	Exe string // bast executable that provides `tunnel run`
}

func NewManager(cacheDir, exe string) (*Manager, error) {
	dir := filepath.Join(cacheDir, "tunnels", "named")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Manager{Dir: dir, Exe: exe}, nil
}

func (m *Manager) id(name string) string {
	sum := sha256.Sum256([]byte("named:" + name))
	return "t-" + hex.EncodeToString(sum[:8])
}

func (m *Manager) statePath(name string) string { return filepath.Join(m.Dir, m.id(name)+".json") }
func (m *Manager) LogPath(name string) string   { return filepath.Join(m.Dir, m.id(name)+".log") }

func (m *Manager) lock(fn func() error) error {
	f, err := os.OpenFile(filepath.Join(m.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)
	return fn()
}

// Status reads name's state. A runner that died without recording an error
// reads as disconnected and its stale file is removed. A terminal error stays
// on disk until the next start or an explicit close, so a later poll still
// shows why the tunnel stopped.
func (m *Manager) Status(name string) Status {
	b, err := os.ReadFile(m.statePath(name))
	if err != nil {
		return Status{Name: name, State: StateConfigured}
	}
	var st Status
	if json.Unmarshal(b, &st) != nil || st.Name != name {
		return Status{Name: name, State: StateConfigured}
	}
	if pidAlive(st.PID) {
		return st
	}
	if st.State == StateError {
		return st
	}
	_ = os.Remove(m.statePath(name))
	return Status{Name: name, State: StateDisconnected, Since: st.Since}
}

func (m *Manager) Running(name string) bool {
	st := m.Status(name)
	return pidAlive(st.PID) && st.State != StateError
}

// Start launches a runner. It is idempotent: a tunnel that is already running
// is left alone and reported with ErrRunning. Local ports are checked first so
// a conflict is reported as a conflict, not as an ssh failure.
func (m *Manager) Start(spec Spec) error {
	if err := Validate(spec); err != nil {
		return err
	}
	return m.lock(func() error {
		if m.Running(spec.Name) {
			return ErrRunning
		}
		for _, addr := range ListenAddrs(spec) {
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("tunnel %q: local %s is already in use", spec.Name, addr)
			}
			_ = ln.Close()
		}
		// A previous terminal error must not be reported as the result of this attempt.
		_ = os.Remove(m.statePath(spec.Name))
		cmd := exec.Command(m.Exe, "tunnel", "run", spec.Name)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
		detachBackground(cmd)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			st := m.Status(spec.Name)
			if st.State == StateError {
				return runnerFailure(spec.Name, st)
			}
			if st.State != StateConfigured && st.State != StateDisconnected && pidAlive(st.PID) {
				return nil
			}
			time.Sleep(25 * time.Millisecond)
		}
		killProcess(cmd.Process.Pid)
		return fmt.Errorf("tunnel %q: runner did not start", spec.Name)
	})
}

var ErrRunning = errors.New("already running")

// Stop ends name's runner and its ssh child. Stopping a tunnel that is not
// running is not an error. The state file is removed first so a runner that
// is mid-restart cannot report itself alive again.
func (m *Manager) Stop(name string) {
	_ = m.lock(func() error {
		st := m.Status(name)
		_ = os.Remove(m.statePath(name))
		// A recorded terminal error keeps the exited runner's pid. Signalling
		// that pid can hit an unrelated process that reused it.
		if pidAlive(st.PID) {
			killProcess(st.PID)
		}
		return nil
	})
}

func runnerFailure(name string, st Status) error {
	msg := st.Error
	if msg == "" {
		msg = string(st.Kind)
	}
	if msg == "" {
		msg = "runner stopped"
	}
	return fmt.Errorf("tunnel %q: %s", name, msg)
}

// Run is the runner body: keep ssh up until ctx ends, reconnecting with
// backoff. It writes state for Manager.Status and logs events, never secrets.
func (m *Manager) Run(ctx context.Context, spec Spec, ssh string, configure func(*exec.Cmd)) error {
	args, err := Args(spec)
	if err != nil {
		return err
	}
	if ssh == "" {
		ssh = "ssh"
	}
	if fi, err := os.Stat(m.LogPath(spec.Name)); err == nil && fi.Size() > maxLogBytes {
		_ = os.Remove(m.LogPath(spec.Name))
	}
	logf, err := os.OpenFile(m.LogPath(spec.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	logLine := func(format string, a ...any) {
		fmt.Fprintf(logf, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, a...))
	}
	st := Status{Name: spec.Name, PID: os.Getpid(), State: StateConnecting, Since: time.Now()}
	set := func(state string, kind ErrorKind, msg string) {
		if state != st.State {
			st.Since = time.Now()
		}
		st.State, st.Kind, st.Error = state, kind, msg
		b, _ := json.Marshal(st)
		tmp := m.statePath(spec.Name) + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, m.statePath(spec.Name))
		}
	}
	defer func() {
		if st.State != StateError {
			_ = os.Remove(m.statePath(spec.Name))
		}
	}()
	backoff := time.Second
	for {
		st.Attempts++
		set(map[bool]string{true: StateConnecting, false: StateReconnecting}[st.Attempts == 1], "", "")
		logLine("connecting via %s (attempt %d)", spec.Host, st.Attempts)
		cmd := exec.Command(ssh, args...)
		buf := &limitedWriter{max: 4096}
		cmd.Stdout = io.Discard
		cmd.Stderr = io.MultiWriter(buf, logf)
		detach(cmd)
		if configure != nil {
			configure(cmd)
		}
		started := time.Now()
		var runErr error
		if err := cmd.Start(); err != nil {
			set(StateError, KindOther, err.Error())
			logLine("start failed: %v", err)
			return err
		}
		exited := make(chan struct{})
		go func() { runErr = cmd.Wait(); close(exited) }()
		select {
		case <-ctx.Done():
			killProcess(cmd.Process.Pid)
			<-exited
			logLine("closed")
			return nil
		case <-exited:
		case <-time.After(settleTime):
			set(StateConnected, "", "")
			logLine("connected")
			select {
			case <-ctx.Done():
				killProcess(cmd.Process.Pid)
				<-exited
				logLine("closed")
				return nil
			case <-exited:
			}
		}
		kind := Classify(buf.string())
		msg := firstLine(buf.string())
		if msg == "" && runErr != nil {
			msg = runErr.Error()
		}
		logLine("disconnected (%s): %s", kind, msg)
		if !retryable(kind) {
			set(StateError, kind, msg)
			return fmt.Errorf("tunnel %q stopped: %s: %s", spec.Name, kind, msg)
		}
		if time.Since(started) > healthyAfter {
			backoff = time.Second
		}
		set(StateReconnecting, kind, msg)
		logLine("retrying in %s", backoff)
		select {
		case <-ctx.Done():
			logLine("closed")
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
}
