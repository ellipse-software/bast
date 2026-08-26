package vercel

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type Instance struct {
	SyncID     string
	Name       string
	ProjectID  string
	State      string
	HostName   string
	Running    bool
	Persistent bool
	VCPUs      int
	Runtime    string
	SessionID  string
	CWD        string
	Tags       []string
}

type Discovery struct {
	Instances    []Instance
	Unrestorable []string
	Warnings     []string
	Complete     bool
}

func (c *Client) Discover(ctx context.Context, _ struct{}) (Discovery, error) {
	account, err := c.Account(ctx)
	if err != nil {
		return Discovery{}, err
	}
	if !account.Authenticated {
		msg := account.Error
		if msg == "" {
			msg = "not authenticated; connect on the Sync tab or set " + TokenEnv
		}
		return Discovery{}, fmt.Errorf("%s", msg)
	}
	sandboxes, err := c.List(ctx)
	if err != nil {
		return Discovery{}, err
	}
	project := c.ResolveProject()
	instances := make([]Instance, 0, len(sandboxes))
	var unrestorable []string
	for _, box := range sandboxes {
		box, drop := c.confirmUnrestorable(ctx, box)
		if drop {
			if name := strings.TrimSpace(box.Name); name != "" {
				unrestorable = append(unrestorable, name)
			}
			continue
		}
		if inst, ok := instanceFromSandbox(box, project); ok {
			instances = append(instances, inst)
		}
	}
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Running != instances[j].Running {
			return instances[i].Running
		}
		return instances[i].Name < instances[j].Name
	})
	sort.Strings(unrestorable)
	return Discovery{Instances: instances, Unrestorable: unrestorable, Complete: true}, nil
}

func (c *Client) Unrestorable(ctx context.Context) ([]string, error) {
	sandboxes, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, box := range sandboxes {
		box, drop := c.confirmUnrestorable(ctx, box)
		if !drop {
			continue
		}
		if name := strings.TrimSpace(box.Name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (c *Client) CleanupUnrestorable(ctx context.Context) ([]string, error) {
	sandboxes, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	var deleted []string
	var first error
	for _, box := range sandboxes {
		box, drop := c.confirmUnrestorable(ctx, box)
		if !drop {
			continue
		}
		name := strings.TrimSpace(box.Name)
		if name == "" {
			continue
		}
		if err := c.Delete(ctx, SyncID(c.ResolveProject(), name)); err != nil && !isAPINotFound(err) {
			if first == nil {
				first = fmt.Errorf("delete %s: %w", name, err)
			}
			continue
		}
		deleted = append(deleted, name)
	}
	sort.Strings(deleted)
	return deleted, first
}

func (c *Client) confirmUnrestorable(ctx context.Context, box Sandbox) (Sandbox, bool) {
	if !unrestorableOffline(box) {
		return box, false
	}
	name := strings.TrimSpace(box.Name)
	if name == "" {
		return box, false
	}
	if box.Persistent && strings.TrimSpace(box.CurrentSnapshot) == "" {
		info, err := c.Get(ctx, SyncID(c.ResolveProject(), name), false)
		if err != nil {
			return box, false
		}
		box = info.Sandbox
		if !unrestorableOffline(box) {
			return box, false
		}
	}
	return box, true
}

func unrestorableOffline(box Sandbox) bool {
	if strings.TrimSpace(box.CurrentSnapshot) != "" {
		return false
	}
	switch normalizeState(box.Status) {
	case "stopped", "failed", "aborted":
		return true
	default:
		return false
	}
}

func isAPINotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "API 404")
}

func instanceFromSandbox(box Sandbox, projectID string) (Instance, bool) {
	name := strings.TrimSpace(box.Name)
	if name == "" {
		return Instance{}, false
	}
	state := normalizeState(box.Status)
	if state == "failed" || state == "aborted" {
		return Instance{}, false
	}
	if state == "stopped" && strings.TrimSpace(box.CurrentSnapshot) == "" {
		return Instance{}, false
	}
	running := isRunningState(box.Status)
	tags := []string{"state:" + state}
	if box.VCPUs > 0 {
		tags = append(tags, fmt.Sprintf("vcpus:%d", box.VCPUs))
	}
	if runtime := strings.TrimSpace(box.Runtime); runtime != "" {
		tags = append(tags, "runtime:"+runtime)
	}
	if box.Persistent {
		tags = append(tags, "persistent")
	}
	return Instance{
		SyncID:     SyncID(projectID, name),
		Name:       name,
		ProjectID:  projectID,
		State:      state,
		HostName:   StoppedHost,
		Running:    running,
		Persistent: box.Persistent,
		VCPUs:      box.VCPUs,
		Runtime:    strings.TrimSpace(box.Runtime),
		SessionID:  strings.TrimSpace(box.CurrentSessionID),
		CWD:        strings.TrimSpace(box.CWD),
		Tags:       tags,
	}, true
}

func normalizeState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "running":
		return "running"
	case "stopping", "snapshotting":
		return "stopping"
	case "stopped":
		return "stopped"
	case "failed":
		return "failed"
	case "aborted":
		return "aborted"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func isRunningState(status string) bool {
	switch normalizeState(status) {
	case "running":
		return true
	default:
		return false
	}
}

func isReadyState(status string) bool {
	return strings.ToLower(strings.TrimSpace(status)) == "running"
}

func IsStoppedState(status string) bool {
	switch normalizeState(status) {
	case "stopped", "stopping":
		return true
	default:
		return false
	}
}

func HostLooksStopped(tags []string) bool {
	return IsStoppedState(StateFromTags(tags))
}

func StateFromTags(tags []string) string {
	for _, tag := range tags {
		if state, ok := strings.CutPrefix(tag, "state:"); ok {
			return state
		}
	}
	return ""
}

func SyncID(projectID, name string) string {
	projectID = strings.TrimSpace(projectID)
	name = strings.TrimSpace(name)
	if projectID == "" {
		return name
	}
	return projectID + "/" + name
}

func ScopedName(syncID, fallbackProject string) (projectID, name string, err error) {
	projectID, name, err = ParseSyncID(syncID)
	if err != nil {
		return "", "", err
	}
	if projectID == "" {
		projectID = strings.TrimSpace(fallbackProject)
	}
	return projectID, name, nil
}

func ParseSyncID(syncID string) (projectID, name string, err error) {
	id := strings.TrimSpace(syncID)
	if id == "" {
		return "", "", fmt.Errorf("invalid Vercel sandbox id %q", syncID)
	}
	if strings.ContainsAny(id, "\\\t\r\n ") {
		return "", "", fmt.Errorf("invalid Vercel sandbox id %q", syncID)
	}
	projectID, name, ok := strings.Cut(id, "/")
	if !ok {
		name = projectID
		projectID = ""
	}
	if err := validateIDPart(projectID, true); err != nil {
		return "", "", fmt.Errorf("invalid Vercel sandbox id %q", syncID)
	}
	if err := validateIDPart(name, false); err != nil {
		return "", "", fmt.Errorf("invalid Vercel sandbox id %q", syncID)
	}
	return projectID, name, nil
}

func validateIDPart(value string, optional bool) error {
	if value == "" {
		if optional {
			return nil
		}
		return fmt.Errorf("empty")
	}
	if len(value) > 128 {
		return fmt.Errorf("too long")
	}
	for _, r := range value {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return fmt.Errorf("invalid char")
		}
	}
	return nil
}
