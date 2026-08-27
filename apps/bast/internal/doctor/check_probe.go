package doctor

import (
	"context"
	"net"
	"strconv"
	"time"
)

func (e Engine) checkProbe(ctx context.Context, r *Report, st runState) {
	literals := st.hosts
	deadline, ok := ctx.Deadline()
	if !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		deadline, _ = ctx.Deadline()
	}
	probed := 0
	for _, h := range literals {
		if probed >= 20 || time.Until(deadline) < 200*time.Millisecond {
			break
		}
		host := h.HostName
		if host == "" {
			host = h.Alias
		}
		if host == "" {
			continue
		}
		port := h.Port
		if port == "" {
			port = "22"
		}
		if _, err := strconv.Atoi(port); err != nil {
			continue
		}
		probed++
		pctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
		addrs, err := net.DefaultResolver.LookupHost(pctx, host)
		if err != nil {
			cancel()
			r.add(Finding{
				ID: "probe.dns", Severity: SeverityWarn, Category: CatProbe,
				Title: "HostName \"" + host + "\" did not resolve", Host: h.Alias, Detail: err.Error(),
			})
			continue
		}
		_ = addrs
		var d net.Dialer
		conn, err := d.DialContext(pctx, "tcp", net.JoinHostPort(host, port))
		cancel()
		if err != nil {
			r.add(Finding{
				ID: "probe.tcp", Severity: SeverityWarn, Category: CatProbe,
				Title: "Cannot connect to " + net.JoinHostPort(host, port), Host: h.Alias, Detail: err.Error(),
			})
			continue
		}
		_ = conn.Close()
	}
}
