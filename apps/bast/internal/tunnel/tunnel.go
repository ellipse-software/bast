// Package tunnel starts OpenSSH local and dynamic forwards and keeps them
// alive until they are closed. Bast does not implement SSH itself.
package tunnel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const readyTimeout = 15 * time.Second

type record struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Via       string `json:"via"`
	PID       int    `json:"pid"`
	Owner     int    `json:"owner,omitempty"`
	LocalPort int    `json:"localPort"`
}

type session struct {
	key       string
	fileID    string
	kind      string
	via       string
	pid       int
	localPort int
	cmd       *exec.Cmd
	owned     bool
	clients   int
	stderr    *limitedWriter
}

// Supervisor tracks forwards started by this process and forwards left running
// with no owner. A crashed owner is reaped on Open. A forward with no owner,
// including every host port forward, stays up until it is closed.
type Supervisor struct {
	dir    string
	ssh    string
	mu     sync.Mutex
	rdp    map[string]*session
	socks  map[string]*session
	config map[string]*session
}

// CacheDir is the per-user directory for tunnel records and browser profiles.
// A home that is not the current user, such as a test temporary directory,
// stays under that home so a test cannot reap the owner's tunnels.
func CacheDir(home string) string {
	if cache, err := os.UserCacheDir(); err == nil && cache != "" && home != "" {
		if current, homeErr := os.UserHomeDir(); homeErr == nil && filepath.Clean(home) == filepath.Clean(current) {
			return filepath.Join(cache, "bast")
		}
	}
	return filepath.Join(home, ".cache", "bast")
}

func Open(dir, ssh string) (*Supervisor, error) {
	if strings.TrimSpace(ssh) == "" {
		ssh = "ssh"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Supervisor{dir: dir, ssh: ssh, rdp: map[string]*session{}, socks: map[string]*session{}, config: map[string]*session{}}
	if err := s.withLock(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.syncFromDiskLocked()
	}); err != nil {
		return nil, err
	}
	return s, nil
}

// RDP starts or reuses a local forward for one entry. target is host:port
// as produced by rdp.DialTarget. The returned port is on 127.0.0.1.
func (s *Supervisor) RDP(id, via, target string, configure func(*exec.Cmd)) (int, bool, error) {
	return s.start("rdp", id, id, via, target, configure)
}

// SOCKS starts or reuses one dynamic forward per jump alias.
func (s *Supervisor) SOCKS(via string, configure func(*exec.Cmd)) (int, bool, error) {
	return s.start("socks", socksFileID(via), via, via, "", configure)
}

func (s *Supervisor) RDPOpen(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.rdp[id]
	return sess != nil && pidAlive(sess.pid)
}

func (s *Supervisor) SOCKSOpen(via string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.socks[via]
	return sess != nil && pidAlive(sess.pid)
}

func (s *Supervisor) CloseRDP(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopKeyLocked("rdp", id)
}

func (s *Supervisor) CloseSOCKS(via string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopKeyLocked("socks", via)
}

// Config starts ssh -N for alias so the host's own LocalForward, RemoteForward,
// and DynamicForward lines stay up without an interactive session. The process
// has no owner: quitting Bast leaves it running, and every Bast window shares it.
func (s *Supervisor) Config(alias string, configure func(*exec.Cmd)) (bool, error) {
	if err := validateAlias(alias); err != nil {
		return false, err
	}
	var reused bool
	err := s.withLock(func() error {
		s.mu.Lock()
		if err := s.syncFromDiskLocked(); err != nil {
			s.mu.Unlock()
			return err
		}
		if existing := s.config[alias]; existing != nil && pidAlive(existing.pid) {
			s.mu.Unlock()
			reused = true
			return nil
		}
		s.mu.Unlock()

		args, err := ConfigForwardArgs(alias)
		if err != nil {
			return err
		}
		fileID := configFileID(alias)
		logFile, err := os.OpenFile(s.logPath(fileID), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
		if err != nil {
			return err
		}
		cmd := exec.Command(s.ssh, args...)
		cmd.Stdin = nil
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		detachBackground(cmd)
		if configure != nil {
			configure(cmd)
		}
		if err := cmd.Start(); err != nil {
			logFile.Close()
			return err
		}
		logFile.Close()
		sess := &session{
			key: alias, fileID: fileID, kind: "config", via: alias,
			pid: cmd.Process.Pid, cmd: cmd, owned: false,
		}
		if err := s.writeRecord(sess, 0); err != nil {
			killProcess(sess.pid)
			_ = cmd.Wait()
			s.removeLog(fileID)
			return err
		}
		exited := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(exited)
			s.mu.Lock()
			defer s.mu.Unlock()
			current := s.config[alias]
			if current == nil || current.pid != sess.pid {
				return
			}
			delete(s.config, alias)
			s.removeRecord(fileID)
			s.removeLog(fileID)
		}()
		if err := waitAlive(exited, 2*time.Second); err != nil {
			message := s.readLog(fileID)
			killProcess(sess.pid)
			<-exited
			s.removeRecord(fileID)
			s.removeLog(fileID)
			return errors.New(FailureMessage(alias, message))
		}
		s.mu.Lock()
		s.config[alias] = sess
		s.mu.Unlock()
		return nil
	})
	return reused, err
}

func (s *Supervisor) ConfigOpen(alias string) bool {
	var open bool
	_ = s.withLock(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.syncFromDiskLocked(); err != nil {
			sess := s.config[alias]
			open = sess != nil && pidAlive(sess.pid)
			return nil
		}
		sess := s.config[alias]
		open = sess != nil && pidAlive(sess.pid)
		return nil
	})
	return open
}

func (s *Supervisor) CloseConfig(alias string) {
	_ = s.withLock(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		_ = s.syncFromDiskLocked()
		s.stopKeyLocked("config", alias)
		return nil
	})
}

func (s *Supervisor) ConfigAliases() []string {
	var out []string
	err := s.withLock(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.syncFromDiskLocked(); err != nil {
			return err
		}
		out = s.configAliasesLocked()
		return nil
	})
	if err != nil {
		s.mu.Lock()
		out = s.configAliasesLocked()
		s.mu.Unlock()
	}
	return out
}

func (s *Supervisor) configAliasesLocked() []string {
	out := make([]string, 0, len(s.config))
	for alias, sess := range s.config {
		if pidAlive(sess.pid) {
			out = append(out, alias)
		}
	}
	sort.Strings(out)
	return out
}

// CloseOwned stops forwards this process started. Host port forwards and other
// unowned forwards stay up.
func (s *Supervisor) CloseOwned() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.rdp {
		if sess.owned {
			s.stopLocked(sess)
			delete(s.rdp, id)
		}
	}
	for via, sess := range s.socks {
		if sess.owned {
			s.stopLocked(sess)
			delete(s.socks, via)
		}
	}
	for alias, sess := range s.config {
		if sess.owned {
			s.stopLocked(sess)
			delete(s.config, alias)
		}
	}
}

// CloseAll stops every forward this supervisor knows about, including host
// port forwards that have no owner.
func (s *Supervisor) CloseAll() {
	_ = s.withLock(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		_ = s.syncFromDiskLocked()
		s.stopEveryLocked()
		return nil
	})
}

func (s *Supervisor) stopEveryLocked() {
	for id, sess := range s.rdp {
		s.stopLocked(sess)
		delete(s.rdp, id)
	}
	for via, sess := range s.socks {
		s.stopLocked(sess)
		delete(s.socks, via)
	}
	for alias, sess := range s.config {
		s.stopLocked(sess)
		delete(s.config, alias)
	}
}

// Release keeps the forward running after this process exits.
func (s *Supervisor) ReleaseRDP(id string) {
	s.release("rdp", id)
}

func (s *Supervisor) ReleaseSOCKS(via string) {
	s.release("socks", via)
}

// WatchClient closes an owned RDP forward after every watched client exits.
// macOS and Linux openers return before the client exits, so they pass nil.
func (s *Supervisor) WatchClient(id string, cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	s.mu.Lock()
	if sess := s.rdp[id]; sess != nil {
		sess.clients++
	}
	s.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		sess := s.rdp[id]
		if sess == nil {
			return
		}
		sess.clients--
		if sess.clients > 0 || !sess.owned {
			return
		}
		s.stopLocked(sess)
		delete(s.rdp, id)
	}()
}

func (s *Supervisor) release(kind, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.lookupLocked(kind, key)
	if sess == nil {
		return
	}
	sess.owned = false
	_ = s.writeRecord(sess, 0)
}

func (s *Supervisor) start(kind, fileID, key, via, target string, configure func(*exec.Cmd)) (int, bool, error) {
	if err := validateAlias(via); err != nil {
		return 0, false, err
	}
	if kind == "rdp" {
		if err := validateTarget(target); err != nil {
			return 0, false, err
		}
	}
	s.mu.Lock()
	if existing := s.lookupLocked(kind, key); existing != nil && pidAlive(existing.pid) && portOpen(existing.localPort) {
		port := existing.localPort
		s.mu.Unlock()
		return port, true, nil
	} else if existing != nil {
		s.stopLocked(existing)
		s.deleteLocked(kind, key)
	}
	s.mu.Unlock()

	var last error
	for attempt := 0; attempt < 5; attempt++ {
		port, err := listenPort()
		if err != nil {
			return 0, false, err
		}
		args, err := forwardArgs(kind, port, target, via)
		if err != nil {
			return 0, false, err
		}
		cmd := exec.Command(s.ssh, args...)
		buf := &limitedWriter{max: 4096}
		cmd.Stdout = io.Discard
		cmd.Stderr = buf
		detach(cmd)
		if configure != nil {
			configure(cmd)
		}
		if err := cmd.Start(); err != nil {
			return 0, false, err
		}
		sess := &session{
			key: key, fileID: fileID, kind: kind, via: via,
			pid: cmd.Process.Pid, localPort: port, cmd: cmd, owned: true, stderr: buf,
		}
		if err := s.writeRecord(sess, os.Getpid()); err != nil {
			killProcess(sess.pid)
			_ = cmd.Wait()
			return 0, false, err
		}
		exited := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(exited)
			s.mu.Lock()
			defer s.mu.Unlock()
			current := s.lookupLocked(kind, key)
			if current == nil || current.pid != sess.pid {
				return
			}
			s.deleteLocked(kind, key)
			s.removeRecord(fileID)
		}()
		if err := waitReady(exited, port, readyTimeout); err != nil {
			killProcess(sess.pid)
			<-exited
			s.removeRecord(fileID)
			last = errors.New(FailureMessage(via, buf.string()))
			if attempt == 4 || !strings.Contains(strings.ToLower(buf.string()), "address already in use") {
				return 0, false, last
			}
			continue
		}
		s.mu.Lock()
		s.storeLocked(sess)
		s.mu.Unlock()
		return port, false, nil
	}
	if last == nil {
		last = errors.New(FailureMessage(via, ""))
	}
	return 0, false, last
}

func (s *Supervisor) lookupLocked(kind, key string) *session {
	switch kind {
	case "socks":
		return s.socks[key]
	case "config":
		return s.config[key]
	default:
		return s.rdp[key]
	}
}

func (s *Supervisor) storeLocked(sess *session) {
	switch sess.kind {
	case "socks":
		s.socks[sess.key] = sess
	case "config":
		s.config[sess.key] = sess
	default:
		s.rdp[sess.key] = sess
	}
}

func (s *Supervisor) deleteLocked(kind, key string) {
	switch kind {
	case "socks":
		delete(s.socks, key)
	case "config":
		delete(s.config, key)
	default:
		delete(s.rdp, key)
	}
}

func (s *Supervisor) stopKeyLocked(kind, key string) {
	sess := s.lookupLocked(kind, key)
	if sess == nil {
		return
	}
	s.stopLocked(sess)
	s.deleteLocked(kind, key)
}

func (s *Supervisor) stopLocked(sess *session) {
	killProcess(sess.pid)
	s.removeRecord(sess.fileID)
	s.removeLog(sess.fileID)
}

func forwardArgs(kind string, port int, target, alias string) ([]string, error) {
	if kind == "socks" {
		return DynamicForwardArgs(port, alias)
	}
	return LocalForwardArgs(port, target, alias)
}

func LocalForwardArgs(localPort int, target, alias string) ([]string, error) {
	if err := validatePort(localPort); err != nil {
		return nil, err
	}
	if err := validateTarget(target); err != nil {
		return nil, err
	}
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	forward := fmt.Sprintf("127.0.0.1:%d:%s", localPort, target)
	return baseArgs("-L", forward, alias), nil
}

// ConfigForwardArgs runs the host's configured forwards without a shell.
// ClearAllForwardings is intentionally absent so LocalForward, RemoteForward,
// and DynamicForward in that host's SSH config stay in effect.
func ConfigForwardArgs(alias string) ([]string, error) {
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	return []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"--", alias,
	}, nil
}

func DynamicForwardArgs(localPort int, alias string) ([]string, error) {
	if err := validatePort(localPort); err != nil {
		return nil, err
	}
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	forward := fmt.Sprintf("127.0.0.1:%d", localPort)
	return baseArgs("-D", forward, alias), nil
}

func baseArgs(flag, value, alias string) []string {
	return []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ClearAllForwardings=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		flag, value,
		"--", alias,
	}
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("that local port can't be used")
	}
	return nil
}

func validateAlias(alias string) error {
	if alias == "" || strings.HasPrefix(alias, "-") || strings.ContainsAny(alias, " \t\r\n\x00") {
		return errors.New("that jump host can't be used")
	}
	return nil
}

func validateTarget(target string) error {
	if target == "" || strings.HasPrefix(target, "-") {
		return errors.New("that address can't be opened in Remote Desktop")
	}
	for _, r := range target {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == ':', r == '-', r == '[', r == ']':
		default:
			return errors.New("that address can't be opened in Remote Desktop")
		}
	}
	return nil
}

// FailureMessage turns ssh stderr into a short notice. It does not echo secrets.
func FailureMessage(via, stderr string) string {
	text := strings.ToLower(stderr)
	switch {
	case strings.Contains(text, "host key verification failed"), strings.Contains(text, "authenticity of host"):
		return fmt.Sprintf("Connect to %s once so its key is known, then try again.", via)
	case strings.Contains(text, "passphrase"), strings.Contains(text, "permission denied"), strings.Contains(text, "password"):
		return fmt.Sprintf("Unlock the key for %s, or save its password on that host, then try again.", via)
	default:
		line := firstLine(stderr)
		if line == "" {
			return fmt.Sprintf("Couldn't open a tunnel through %s.", via)
		}
		return fmt.Sprintf("Couldn't open a tunnel through %s. %s", via, line)
	}
}

func firstLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 180 {
			line = line[:180]
		}
		return line
	}
	return ""
}

func listenPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port == 0 {
		return 0, errors.New("that local port can't be used")
	}
	return addr.Port, nil
}

func waitAlive(exited <-chan struct{}, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-exited:
		return errors.New("exited")
	case <-timer.C:
		return nil
	}
}

func waitReady(exited <-chan struct{}, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			return errors.New("exited")
		default:
		}
		if portOpen(port) {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return errors.New("timed out")
}

func portOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func socksFileID(via string) string {
	sum := sha256.Sum256([]byte(via))
	return "socks-" + hex.EncodeToString(sum[:8])
}

func configFileID(alias string) string {
	sum := sha256.Sum256([]byte("config:" + alias))
	return "cfg-" + hex.EncodeToString(sum[:8])
}

func (s *Supervisor) syncFromDiskLocked() error {
	files, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	our := os.Getpid()
	live := map[string]struct{}{}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.dir, file.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rec record
		if err := json.Unmarshal(body, &rec); err != nil || rec.PID <= 0 || rec.ID == "" {
			_ = os.Remove(path)
			continue
		}
		if !pidAlive(rec.PID) {
			_ = os.Remove(path)
			s.removeLog(rec.ID)
			continue
		}
		// Host port forwards stay up after every Bast window exits. Other
		// tunnels still belong to the process that started them.
		if rec.Kind != "config" && rec.Owner != 0 && rec.Owner != our {
			if !pidAlive(rec.Owner) {
				killProcess(rec.PID)
				_ = os.Remove(path)
				s.removeLog(rec.ID)
			}
			continue
		}
		live[rec.ID] = struct{}{}
		s.adoptLocked(rec)
	}
	s.forgetStaleLocked(live)
	return nil
}

func (s *Supervisor) adoptLocked(rec record) {
	current := s.lookupLocked(rec.Kind, adoptKey(rec))
	if current != nil && current.pid == rec.PID {
		return
	}
	if current != nil && current.cmd != nil && pidAlive(current.pid) {
		return
	}
	sess := &session{
		fileID: rec.ID, kind: rec.Kind, via: rec.Via,
		pid: rec.PID, localPort: rec.LocalPort, owned: false,
	}
	switch rec.Kind {
	case "socks":
		sess.key = rec.Via
	case "config":
		sess.key = rec.Via
	default:
		sess.key = rec.ID
	}
	s.storeLocked(sess)
}

func adoptKey(rec record) string {
	if rec.Kind == "socks" || rec.Kind == "config" {
		return rec.Via
	}
	return rec.ID
}

func (s *Supervisor) forgetStaleLocked(live map[string]struct{}) {
	drop := func(sess *session) bool {
		if !pidAlive(sess.pid) {
			return true
		}
		_, ok := live[sess.fileID]
		return !ok && sess.cmd == nil
	}
	for id, sess := range s.rdp {
		if drop(sess) {
			delete(s.rdp, id)
		}
	}
	for via, sess := range s.socks {
		if drop(sess) {
			delete(s.socks, via)
		}
	}
	for alias, sess := range s.config {
		if drop(sess) {
			delete(s.config, alias)
		}
	}
}

func (s *Supervisor) withLock(fn func() error) error {
	f, err := os.OpenFile(filepath.Join(s.dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
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

func (s *Supervisor) logPath(fileID string) string {
	return filepath.Join(s.dir, fileID+".err")
}

func (s *Supervisor) readLog(fileID string) string {
	body, err := os.ReadFile(s.logPath(fileID))
	if err != nil || len(body) == 0 {
		return ""
	}
	if len(body) > 4096 {
		body = body[:4096]
	}
	return string(body)
}

func (s *Supervisor) removeLog(fileID string) {
	if fileID == "" || strings.ContainsAny(fileID, `/\`) {
		return
	}
	_ = os.Remove(s.logPath(fileID))
}

func (s *Supervisor) writeRecord(sess *session, owner int) error {
	body, err := json.Marshal(record{
		ID: sess.fileID, Kind: sess.kind, Via: sess.via,
		PID: sess.pid, Owner: owner, LocalPort: sess.localPort,
	})
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, sess.fileID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Supervisor) removeRecord(fileID string) {
	if fileID == "" || strings.ContainsAny(fileID, `/\`) {
		return
	}
	_ = os.Remove(filepath.Join(s.dir, fileID+".json"))
}

type limitedWriter struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) < w.max {
		room := w.max - len(w.buf)
		if len(p) < room {
			room = len(p)
		}
		w.buf = append(w.buf, p[:room]...)
	}
	return len(p), nil
}

func (w *limitedWriter) string() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}
