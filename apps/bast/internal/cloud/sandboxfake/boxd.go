package sandboxfake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// BoxdOrgID and BoxdOrgName are the org the fake boxd CLI is logged in to.
const (
	BoxdOrgID   = "org-0000aaaa"
	BoxdOrgName = "acme"
)

type BoxdComputer struct {
	Region string
	Name   string
	Status string
	Port   string
}

// Boxd is an in-process stand-in for the boxd CLI and the SSH entries it
// writes, for tests that drive the shipped client through its Runner.
// BoxdAccountOrg is an extra org on the fake signed-in account.
type BoxdAccountOrg struct {
	ID     string
	Name   string
	Active bool
}

type Boxd struct {
	mu        sync.Mutex
	seq       int
	Computers map[string]*BoxdComputer
	Calls     [][]string
	// unavailable regions are left out of a `--region all` list, which names
	// them on stderr and still succeeds, like the CLI.
	unavailable map[string]bool
	// ExtraOrgs are added to the signed-in account beside the default org.
	ExtraOrgs []BoxdAccountOrg
	// BootStatus is the status a created or forked computer comes up in.
	// Empty means running.
	BootStatus string
}

func NewBoxd() *Boxd {
	return &Boxd{Computers: map[string]*BoxdComputer{}, unavailable: map[string]bool{}}
}

// SetUnavailable makes regions unreadable for later lists; no regions clears it.
func (b *Boxd) SetUnavailable(regions ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.unavailable = map[string]bool{}
	for _, region := range regions {
		b.unavailable[region] = true
	}
}

// Remove deletes a computer outside Bast, as another client would.
func (b *Boxd) Remove(region, name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.Computers, boxdKey(region, name))
}

func boxdKey(region, name string) string { return region + "/" + name }

func (b *Boxd) bootStatus() string {
	if b.BootStatus == "" {
		return "running"
	}
	return b.BootStatus
}

func (b *Boxd) Put(c BoxdComputer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c.Port == "" {
		b.seq++
		c.Port = fmt.Sprintf("%d", 20000+b.seq)
	}
	copy := c
	b.Computers[boxdKey(c.Region, c.Name)] = &copy
}

func (b *Boxd) Get(region, name string) *BoxdComputer {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.Computers[boxdKey(region, name)]
	if c == nil {
		return nil
	}
	copy := *c
	return &copy
}

func boxdURL(c *BoxdComputer) string {
	if c.Region == "fra" {
		return c.Name + ".boxd.sh"
	}
	return c.Name + "." + c.Region + ".boxd.sh"
}

// ResolveSSH answers like `ssh -G` does for the entries the boxd CLI keeps.
func (b *Boxd) ResolveSSH() func(ctx context.Context, host string) (string, string, string, bool) {
	return func(_ context.Context, host string) (port, user, identity string, ok bool) {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, c := range b.Computers {
			if boxdURL(c) == host {
				return c.Port, "boxd", "/home/test/.config/boxd/id_ed25519_test", true
			}
		}
		return "22", "", "", false
	}
}

func boxdFlag(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func (b *Boxd) Runner() func(ctx context.Context, args []string, env []string) ([]byte, []byte, error) {
	return func(_ context.Context, args []string, _ []string) ([]byte, []byte, error) {
		b.mu.Lock()
		defer b.mu.Unlock()
		args = args[1:]
		b.Calls = append(b.Calls, append([]string(nil), args...))
		var stderr strings.Builder
		if len(args) >= 2 && args[0] == "machine" && args[1] == "list" && boxdFlag(args, "--region") == "all" {
			for region := range b.unavailable {
				fmt.Fprintf(&stderr, "█ %s: unavailable (can't be reached right now: connection refused)\n", region)
			}
		}
		out, err := b.respond(args)
		return out, []byte(stderr.String()), err
	}
}

func (b *Boxd) respond(args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("boxd: no command")
	}
	switch args[0] {
	case "version":
		return []byte("boxd 0.3.3\n"), nil
	case "auth":
		orgs := []map[string]any{{
			"active": true, "id": BoxdOrgID, "is_admin": true, "name": BoxdOrgName,
		}}
		for _, extra := range b.ExtraOrgs {
			orgs = append(orgs, map[string]any{"active": extra.Active, "id": extra.ID, "name": extra.Name})
		}
		return json.Marshal(map[string]any{
			"active_org": BoxdOrgName, "display_name": "tester", "orgs": orgs, "user_id": "gh-tester",
		})
	}
	if args[0] != "machine" || len(args) < 2 {
		return nil, fmt.Errorf("boxd: unexpected command %v", args)
	}
	if org := boxdFlag(args, "--org"); org != BoxdOrgID {
		return nil, fmt.Errorf("boxd: org '%s' not found", org)
	}
	region := boxdFlag(args, "--region")
	switch args[1] {
	case "list":
		rows := []map[string]string{}
		for _, c := range b.Computers {
			if (region == "all" && !b.unavailable[c.Region]) || region == "" || region == c.Region {
				rows = append(rows, map[string]string{"region": c.Region, "name": c.Name, "status": c.Status, "size": "2x8G", "url": boxdURL(c)})
			}
		}
		return json.Marshal(rows)
	case "new":
		if boxdFlag(args, "--vcpu") != "" && boxdFlag(args, "--from-snapshot") != "" {
			return nil, fmt.Errorf("boxd: --vcpu/--memory only apply to fresh machines; forks and snapshot restores keep their source's size")
		}
		if region == "" {
			region = "fra"
		}
		name := args[2]
		if strings.HasPrefix(name, "-") {
			b.seq++
			name = fmt.Sprintf("auto-%d", b.seq)
		}
		b.seq++
		c := &BoxdComputer{Region: region, Name: name, Status: b.bootStatus(), Port: fmt.Sprintf("%d", 20000+b.seq)}
		b.Computers[boxdKey(region, name)] = c
		return json.Marshal(map[string]string{"id": "id-" + name, "name": name, "url": boxdURL(c), "boot": "6ms"})
	}
	c := b.Computers[boxdKey(region, args[2])]
	if c == nil {
		return nil, fmt.Errorf("boxd: no machine %q in %s", args[2], region)
	}
	switch args[1] {
	case "get":
		return json.Marshal(map[string]any{"name": c.Name, "id": "id-" + c.Name, "status": c.Status, "url": boxdURL(c), "ssh_disabled": false})
	case "fork":
		name := c.Name + "-fork"
		if len(args) > 3 && !strings.HasPrefix(args[3], "-") {
			name = args[3]
		}
		b.seq++
		fork := &BoxdComputer{Region: c.Region, Name: name, Status: b.bootStatus(), Port: fmt.Sprintf("%d", 20000+b.seq)}
		b.Computers[boxdKey(c.Region, name)] = fork
		return json.Marshal(map[string]string{"id": "id-" + name, "name": name, "url": boxdURL(fork), "boot": "starting"})
	case "stop":
		c.Status = "stopped"
	case "start", "wake", "resume", "reboot":
		c.Status = "running"
	case "remove":
		delete(b.Computers, boxdKey(region, args[2]))
		return []byte(`{"name":"` + args[2] + `","status":"destroyed"}`), nil
	default:
		return nil, fmt.Errorf("boxd: unexpected command %v", args)
	}
	return []byte(`{"name":"` + c.Name + `","status":"` + c.Status + `"}`), nil
}
