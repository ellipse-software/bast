package doctor

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
)

func (e Engine) checkKnownHosts(ctx context.Context, r *Report, st runState) {
	_ = ctx
	path := filepath.Join(e.Paths.SSHDir, "known_hosts")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	dupesByType := map[string]int{}
	hostsSeen := map[string]int{}
	hashed := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.HasPrefix(fields[0], "@") {
			if len(fields) < 3 {
				continue
			}
			fields = fields[1:]
		}
		hostField, keyType := fields[0], fields[1]
		if strings.HasPrefix(hostField, "|1|") {
			hashed++
			continue
		}
		for _, host := range strings.Split(hostField, ",") {
			host = strings.TrimPrefix(host, "[")
			host = strings.TrimSuffix(host, "]")
			if i := strings.LastIndex(host, "]:"); i >= 0 {
				host = strings.TrimPrefix(host[:i], "[")
			}
			hostsSeen[host]++
			dupesByType[host+" "+keyType]++
		}
	}
	dupes := 0
	var sample string
	for hostKey, n := range dupesByType {
		if n > 1 {
			dupes++
			if sample == "" {
				sample = hostKey
			}
		}
	}
	if dupes > 0 {
		r.add(Finding{
			ID: "known_hosts.duplicates", Severity: SeverityWarn, Category: CatKnownHosts,
			Title: "known_hosts has duplicate host entries", Path: e.display(path),
			Detail: "Example: " + sample + ". Remove stale lines with bast hosts known-host remove <host>.",
		})
	}
	unseen := 0
	for _, h := range st.hosts {
		name := h.HostName
		if name == "" {
			name = h.Alias
		}
		if hostsSeen[name] == 0 && hashed == 0 {
			unseen++
		}
	}
	if unseen > 3 {
		r.add(Finding{
			ID: "known_hosts.unseen", Severity: SeverityInfo, Category: CatKnownHosts,
			Title: "Several hosts have no known_hosts entry", Path: e.display(path),
			Detail: "First connect will prompt to trust the host key.",
		})
	}
}
