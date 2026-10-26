package vercel

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

const (
	extendEvery     = 4 * time.Minute
	extendBy        = 5 * time.Minute
	maxConnectedFor = 5 * time.Hour
)

type ShellOptions struct {
	Name      string
	ProjectID string
	TeamID    string
}

type startFrame struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	CWD     string   `json:"cwd,omitempty"`
	Cols    int      `json:"cols"`
	Rows    int      `json:"rows"`
}

type controlFrame struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Code *int   `json:"code,omitempty"`
}

func ParseShellOptions(args []string) (ShellOptions, error) {
	fs := flag.NewFlagSet("vercel-sandbox-shell", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var options ShellOptions
	fs.StringVar(&options.Name, "name", "", "")
	fs.StringVar(&options.ProjectID, "project", "", "")
	fs.StringVar(&options.TeamID, "team", "", "")
	if err := fs.Parse(args); err != nil {
		return ShellOptions{}, err
	}
	if fs.NArg() != 0 || strings.TrimSpace(options.Name) == "" {
		return ShellOptions{}, errors.New("invalid Vercel sandbox shell arguments")
	}
	return options, nil
}

func InteractiveURL(rawURL, token string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	token = strings.TrimSpace(token)
	if rawURL == "" || token == "" {
		return "", fmt.Errorf("interactive url and token are required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("interactive url: %w", err)
	}
	query := parsed.Query()
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func EncodeStartFrame(command string, args []string, env []string, cwd string, cols, rows int) ([]byte, error) {
	if command == "" {
		command = "/bin/bash"
	}
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return json.Marshal(startFrame{
		Type:    "start",
		Command: command,
		Args:    args,
		Env:     env,
		CWD:     cwd,
		Cols:    cols,
		Rows:    rows,
	})
}

func EncodeResizeFrame(cols, rows int) ([]byte, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return json.Marshal(controlFrame{Type: "resize", Cols: cols, Rows: rows})
}

func RunShell(ctx context.Context, client *Client, options ShellOptions, in io.Reader, out, errOut io.Writer) error {
	if client == nil {
		return fmt.Errorf("vercel client is required")
	}
	client.ProjectID = strings.TrimSpace(options.ProjectID)
	if team := strings.TrimSpace(options.TeamID); team != "" {
		client.TeamID = team
	}
	if err := client.PersistResolvedToken(); err != nil {
		return err
	}
	syncID := SyncID(client.ProjectID, options.Name)
	info, err := client.Get(ctx, syncID, false)
	if err != nil {
		return err
	}
	if !isReadyState(info.Sandbox.Status) {
		if err := client.Resume(ctx, syncID); err != nil {
			return err
		}
		info, err = client.Get(ctx, syncID, false)
		if err != nil {
			return err
		}
	}
	sessionID := strings.TrimSpace(info.Sandbox.CurrentSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(info.Session.ID)
	}
	if sessionID == "" {
		return fmt.Errorf("vercel sandbox %s has no running session", options.Name)
	}
	interactive, err := client.OpenInteractive(ctx, sessionID)
	if err != nil {
		return err
	}
	dialURL, err := InteractiveURL(interactive.URL, interactive.Token)
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 20 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, dialURL, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
		}
		if status > 0 {
			return fmt.Errorf("vercel sandbox websocket %d: %v; check the access token and project", status, err)
		}
		return fmt.Errorf("vercel sandbox websocket: %w; check the access token and project", err)
	}
	defer conn.Close()
	var writeMu sync.Mutex
	writeMessage := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(messageType, data)
	}

	cols, rows := terminalSize(out)
	cwd := strings.TrimSpace(info.Sandbox.CWD)
	if cwd == "" {
		cwd = strings.TrimSpace(info.Session.CWD)
	}
	start, err := EncodeStartFrame("/bin/bash", []string{"-i"}, []string{"TERM=xterm-256color"}, cwd, cols, rows)
	if err != nil {
		return err
	}
	if err := writeMessage(websocket.TextMessage, start); err != nil {
		return fmt.Errorf("vercel sandbox start: %w", err)
	}

	extendCtx, stopExtend := context.WithCancel(ctx)
	defer stopExtend()
	go extendTimeoutLoop(extendCtx, client, sessionID)

	restore, err := makeRaw(in)
	if err == nil && restore != nil {
		defer restore()
	}

	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, 32*1024)
		state := stdinAfterNL
		for {
			n, readErr := in.Read(buf)
			if n > 0 {
				forward, disconnect, next := processStdinBytes(state, buf[:n])
				state = next
				if len(forward) > 0 {
					if writeErr := writeMessage(websocket.BinaryMessage, forward); writeErr != nil {
						errCh <- writeErr
						return
					}
				}
				if disconnect {
					_ = writeMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					errCh <- errLocalDisconnect
					return
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					_ = writeMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					errCh <- nil
					return
				}
				errCh <- readErr
				return
			}
		}
	}()
	go func() {
		for {
			messageType, data, readErr := conn.ReadMessage()
			if readErr != nil {
				errCh <- readErr
				return
			}
			switch messageType {
			case websocket.BinaryMessage:
				if _, writeErr := out.Write(data); writeErr != nil {
					errCh <- writeErr
					return
				}
			case websocket.TextMessage:
				var frame controlFrame
				if json.Unmarshal(data, &frame) == nil && frame.Type == "exit" {
					errCh <- nil
					return
				}
				if _, writeErr := out.Write(data); writeErr != nil {
					errCh <- writeErr
					return
				}
			}
		}
	}()
	watchResize(ctx, writeMessage, out)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		if isSessionClose(err) {
			return nil
		}
		return err
	}
}

var errLocalDisconnect = errors.New("local disconnect")

type stdinEscapeState int

const (
	stdinAfterNL stdinEscapeState = iota
	stdinNormal
	stdinTilde
)

// processStdinBytes applies OpenSSH's newline-then-~. local disconnect.
func processStdinBytes(state stdinEscapeState, input []byte) (forward []byte, disconnect bool, next stdinEscapeState) {
	next = state
	for _, b := range input {
		switch next {
		case stdinAfterNL:
			if b == '~' {
				next = stdinTilde
				continue
			}
			forward = append(forward, b)
			if b != '\r' && b != '\n' {
				next = stdinNormal
			}
		case stdinTilde:
			if b == '.' {
				return forward, true, next
			}
			forward = append(forward, '~')
			if b == '~' {
				next = stdinNormal
				continue
			}
			forward = append(forward, b)
			if b == '\r' || b == '\n' {
				next = stdinAfterNL
			} else {
				next = stdinNormal
			}
		default:
			forward = append(forward, b)
			if b == '\r' || b == '\n' {
				next = stdinAfterNL
			}
		}
	}
	return forward, false, next
}

func isSessionClose(err error) bool {
	if err == nil || errors.Is(err, errLocalDisconnect) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, websocket.ErrCloseSent) {
		return true
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "websocket: close") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "broken pipe")
}

func extendTimeoutLoop(ctx context.Context, client *Client, sessionID string) {
	deadline := time.Now().Add(maxConnectedFor)
	ticker := time.NewTicker(extendEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				return
			}
			extendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			_ = client.ExtendTimeout(extendCtx, sessionID, extendBy)
			cancel()
		}
	}
}

func terminalSize(out io.Writer) (cols, rows int) {
	cols, rows = 80, 24
	file, ok := out.(*os.File)
	if !ok {
		return cols, rows
	}
	w, h, err := term.GetSize(int(file.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return cols, rows
	}
	return w, h
}

func makeRaw(in io.Reader) (func(), error) {
	file, ok := in.(*os.File)
	if !ok {
		return nil, errors.New("stdin is not a file")
	}
	if !term.IsTerminal(int(file.Fd())) {
		return nil, errors.New("stdin is not a terminal")
	}
	state, err := term.MakeRaw(int(file.Fd()))
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(int(file.Fd()), state) }, nil
}
