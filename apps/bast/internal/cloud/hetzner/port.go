package hetzner

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"
)

var alternateSSHPorts = []int{2022, 2222}

var probeSSHPort = probeSSH

func detectSSHPort(ctx context.Context, host, configured string) string {
	preferred := 22
	if n, err := strconv.Atoi(strings.TrimSpace(configured)); err == nil && n > 0 && n <= 65535 {
		preferred = n
	}
	switch probeSSHPort(ctx, host, preferred) {
	case probeSSHBanner:
		if preferred == 22 {
			return ""
		}
		return strconv.Itoa(preferred)
	case probeRefused, probeUnknown, probeOther:
		if preferred != 22 && probeSSHPort(ctx, host, 22) == probeSSHBanner {
			return ""
		}
		for _, port := range alternateSSHPorts {
			if port == preferred {
				continue
			}
			if probeSSHPort(ctx, host, port) == probeSSHBanner {
				return strconv.Itoa(port)
			}
		}
	}
	if preferred == 22 {
		return ""
	}
	return strconv.Itoa(preferred)
}

type probeResult int

const (
	probeUnknown probeResult = iota
	probeSSHBanner
	probeRefused
	probeOther
)

func probeSSH(ctx context.Context, host string, port int) probeResult {
	dialer := net.Dialer{Timeout: time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "connection refused") || strings.Contains(msg, "connect: refused") {
			return probeRefused
		}
		return probeUnknown
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(800 * time.Millisecond))
	buf := make([]byte, 64)
	n, _ := conn.Read(buf)
	if n >= 7 && strings.HasPrefix(string(buf[:n]), "SSH-2.0") {
		return probeSSHBanner
	}
	return probeOther
}
