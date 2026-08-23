package files

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/pkg/sftp"

	"bast/internal/openssh"
)

// Session is an OpenSSH-backed SFTP connection for one host alias.
type Session struct {
	Alias  string
	client *sftp.Client
	cmd    *exec.Cmd
	stderr bytes.Buffer
	mu     sync.Mutex
	closed bool
}

// OpenSession starts ssh -s sftp for alias using the given OpenSSH client.
// If ctx is cancelled before the handshake finishes, the SSH process is killed.
func OpenSession(ctx context.Context, openSSH openssh.Client, alias string) (*Session, error) {
	return OpenSessionPrepared(ctx, openSSH, alias, nil)
}

// OpenSessionPrepared starts ssh -s sftp. prepare may swap BatchMode off and set askpass env.
func OpenSessionPrepared(ctx context.Context, openSSH openssh.Client, alias string, prepare func(*exec.Cmd) error) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var cmd *exec.Cmd
	var err error
	if prepare != nil {
		cmd, err = openSSH.SFTPCommandInteractive(alias)
	} else {
		cmd, err = openSSH.SFTPCommand(alias)
	}
	if err != nil {
		return nil, err
	}
	if prepare != nil {
		if err := prepare(cmd); err != nil {
			return nil, err
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	session := &Session{Alias: alias, cmd: cmd}
	cmd.Stderr = &session.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sftp: %w", err)
	}

	type result struct {
		client *sftp.Client
		err    error
	}
	done := make(chan result, 1)
	go func() {
		client, err := sftp.NewClientPipe(stdout, stdin)
		done <- result{client: client, err: err}
	}()

	select {
	case <-ctx.Done():
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		// Wait for the handshake goroutine before Wait so StdoutPipe is not
		// closed under an in-flight NewClientPipe reader, and so a client that
		// raced past cancellation is closed instead of leaked.
		res := <-done
		if res.client != nil {
			_ = res.client.Close()
		}
		_ = cmd.Wait()
		return nil, ctx.Err()
	case res := <-done:
		if res.err != nil {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			detail := strings.TrimSpace(session.stderr.String())
			if detail != "" {
				return nil, fmt.Errorf("sftp handshake: %w (%s)", res.err, detail)
			}
			return nil, fmt.Errorf("sftp handshake: %w; unlock keys with ssh-add or connect once from Hosts to accept the host key", res.err)
		}
		session.client = res.client
		return session, nil
	}
}

// Client returns the underlying SFTP client.
func (s *Session) Client() *sftp.Client {
	return s.client
}

// Close tears down the SFTP client and SSH process.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var first error
	if s.client != nil {
		if err := s.client.Close(); err != nil && first == nil {
			first = err
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		if err := s.cmd.Wait(); err != nil && first == nil {
			if _, ok := err.(*exec.ExitError); !ok {
				first = err
			}
		}
	}
	return first
}

// Stderr returns captured SSH stderr (useful after failed connect).
func (s *Session) Stderr() string {
	return strings.TrimSpace(s.stderr.String())
}

// Home returns the remote working directory for the session.
func (s *Session) Home() (string, error) {
	wd, err := s.client.Getwd()
	if err != nil {
		return "", err
	}
	return CleanRemote(wd)
}
