package fly

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"bast/internal/cloud/sshutil"
)

type ProxyOptions struct {
	Org          string
	App          string
	Machine      string
	ResourcePort int
}

func ParseProxyOptions(args []string) (ProxyOptions, error) {
	fs := flag.NewFlagSet("fly-proxy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var options ProxyOptions
	fs.StringVar(&options.Org, "org", "", "")
	fs.StringVar(&options.App, "app", "", "")
	fs.StringVar(&options.Machine, "machine", "", "")
	fs.IntVar(&options.ResourcePort, "resource-port", 22, "")
	if err := fs.Parse(args); err != nil {
		return ProxyOptions{}, err
	}
	if fs.NArg() != 0 || options.Org == "" || options.App == "" || options.Machine == "" ||
		options.ResourcePort < 1 || options.ResourcePort > 65535 {
		return ProxyOptions{}, errors.New("invalid Fly proxy arguments")
	}
	return options, nil
}

func ProxyCommand(options ProxyOptions, bastExecutable string) string {
	if strings.TrimSpace(bastExecutable) == "" {
		bastExecutable = "bast"
	}
	args := []string{
		bastExecutable, "__fly-proxy",
		"--org", options.Org,
		"--app", options.App,
		"--machine", options.Machine,
		"--resource-port", "%p",
	}
	for i := range args {
		if args[i] != "%p" {
			args[i] = sshutil.ProxyLiteral(args[i])
		}
		args[i] = sshutil.ShellQuote(args[i])
	}
	return strings.Join(args, " ")
}

func RunProxy(ctx context.Context, options ProxyOptions, in io.Reader, out, errOut io.Writer) error {
	client := New()
	if err := client.CheckAvailable(ctx); err != nil {
		return err
	}
	localPort, err := availableLocalPort(ctx)
	if err != nil {
		return fmt.Errorf("choose Fly proxy port: %w", err)
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	remote := strconv.Itoa(options.ResourcePort)
	if remote == "" || remote == "0" {
		remote = "22"
	}
	args := []string{
		"proxy", fmt.Sprintf("%d:%s", localPort, remote), InternalHost(options.App, options.Machine),
		"--app", options.App,
		"--org", options.Org,
		"--quiet",
		"--watch-stdin",
	}
	cmd := exec.CommandContext(childCtx, client.bin(), args...)
	cmd.Stderr = errOut
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start fly proxy: %w", err)
	}
	waited := make(chan error, 1)
	go func() {
		waited <- cmd.Wait()
		close(waited)
	}()

	conn, err := waitForTunnel(childCtx, localPort, waited, 20*time.Second)
	if err != nil {
		cancel()
		select {
		case <-waited:
		case <-time.After(time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
		return err
	}
	copyErr := proxyStreams(conn, in, out)
	cancel()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return copyErr
}

func availableLocalPort(ctx context.Context) (int, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForTunnel(ctx context.Context, port int, processExit <-chan error, timeout time.Duration) (net.Conn, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for {
		dialCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
		cancel()
		if err == nil {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-processExit:
			if err == nil {
				return nil, errors.New("fly proxy exited before accepting connections")
			}
			return nil, fmt.Errorf("fly proxy failed: %w", err)
		case <-deadline.C:
			return nil, errors.New("timed out waiting for fly proxy")
		case <-ticker.C:
		}
	}
}

func proxyStreams(conn net.Conn, in io.Reader, out io.Writer) error {
	defer conn.Close()
	type copyResult struct {
		direction string
		err       error
	}
	results := make(chan copyResult, 2)
	go func() {
		_, err := io.Copy(conn, in)
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		results <- copyResult{direction: "input", err: err}
	}()
	go func() {
		_, err := io.Copy(out, conn)
		results <- copyResult{direction: "output", err: err}
	}()

	first := <-results
	if first.direction == "input" && first.err == nil {
		first = <-results
	} else {
		_ = conn.Close()
	}
	if first.err != nil && !errors.Is(first.err, net.ErrClosed) && !strings.Contains(first.err.Error(), "use of closed network connection") {
		return first.err
	}
	return nil
}
