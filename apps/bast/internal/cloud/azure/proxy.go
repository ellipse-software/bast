package azure

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type ProxyOptions struct {
	Subscription         string
	BastionResourceGroup string
	BastionName          string
	TargetResourceID     string
	ResourcePort         int
}

func ParseProxyOptions(args []string) (ProxyOptions, error) {
	fs := flag.NewFlagSet("azure-bastion-proxy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var options ProxyOptions
	fs.StringVar(&options.Subscription, "subscription", "", "")
	fs.StringVar(&options.BastionResourceGroup, "bastion-group", "", "")
	fs.StringVar(&options.BastionName, "bastion", "", "")
	fs.StringVar(&options.TargetResourceID, "target", "", "")
	fs.IntVar(&options.ResourcePort, "resource-port", 22, "")
	if err := fs.Parse(args); err != nil {
		return ProxyOptions{}, err
	}
	if fs.NArg() != 0 || options.Subscription == "" || options.BastionResourceGroup == "" ||
		options.BastionName == "" || options.TargetResourceID == "" || options.ResourcePort < 1 || options.ResourcePort > 65535 {
		return ProxyOptions{}, errors.New("invalid Azure Bastion proxy arguments")
	}
	return options, nil
}

func RunBastionProxy(ctx context.Context, options ProxyOptions, in io.Reader, out, errOut io.Writer) error {
	if err := New().CheckExtension(ctx, "bastion"); err != nil {
		return err
	}
	// The probe releases the port before az binds it, so another process can win
	// that race. An intermittent bind failure is therefore safe to retry.
	localPort, err := availableLocalPort(ctx)
	if err != nil {
		return fmt.Errorf("choose Azure Bastion tunnel port: %w", err)
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	args := BastionTunnelArgs(options, localPort)
	cmd := exec.CommandContext(childCtx, "az", args...)
	cmd.Stderr = errOut
	cmd.Env = append(os.Environ(), "AZURE_CORE_ONLY_SHOW_ERRORS=true")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Azure Bastion tunnel: %w", err)
	}
	waited := make(chan error, 1)
	go func() {
		waited <- cmd.Wait()
		close(waited)
	}()

	conn, err := waitForTunnel(childCtx, localPort, waited, 15*time.Second)
	if err != nil {
		cancel()
		select {
		case <-waited:
		case <-time.After(time.Second):
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

func BastionTunnelArgs(options ProxyOptions, localPort int) []string {
	return []string{
		"network", "bastion", "tunnel",
		"--name", options.BastionName,
		"--resource-group", options.BastionResourceGroup,
		"--target-resource-id", options.TargetResourceID,
		"--resource-port", strconv.Itoa(options.ResourcePort),
		"--port", strconv.Itoa(localPort),
		"--subscription", options.Subscription,
		"--only-show-errors",
	}
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
				return nil, errors.New("tunnel through Azure Bastion exited before accepting connections")
			}
			return nil, fmt.Errorf("tunnel through Azure Bastion failed: %w", err)
		case <-deadline.C:
			return nil, errors.New("timed out waiting for Azure Bastion tunnel")
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
