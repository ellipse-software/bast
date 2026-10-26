package upstash

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

type Instance struct {
	SyncID    string
	Name      string
	State     string
	HostName  string
	User      string
	Running   bool
	KeepAlive bool
	Size      string
	Runtime   string
	Tags      []string
}

type Discovery struct {
	Instances []Instance
	Warnings  []string
	Complete  bool
}

func (c *Client) Discover(ctx context.Context, _ struct{}) (Discovery, error) {
	if !c.HasKey() {
		return Discovery{}, fmt.Errorf("no API key; connect on the Sync tab or set " + APIKeyEnv)
	}
	boxes, err := c.List(ctx)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "401") || strings.Contains(lower, "invalid") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "api key") {
			return Discovery{}, fmt.Errorf("%s", msg)
		}
		return Discovery{}, err
	}
	instances := make([]Instance, 0, len(boxes))
	for _, box := range boxes {
		if inst, ok := instanceFromBox(box, c.SSHHost()); ok {
			instances = append(instances, inst)
		}
	}
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Running != instances[j].Running {
			return instances[i].Running
		}
		return instances[i].Name < instances[j].Name
	})
	return Discovery{Instances: instances, Complete: true}, nil
}

func instanceFromBox(box BoxData, sshHost string) (Instance, bool) {
	id := strings.TrimSpace(box.ID)
	if id == "" {
		return Instance{}, false
	}
	state := normalizeState(box.Status)
	if state == "deleted" {
		return Instance{}, false
	}
	name := strings.TrimSpace(box.Name)
	if name == "" {
		name = id
	}
	running := isRunningState(box.Status)
	tags := []string{"state:" + state}
	if size := strings.TrimSpace(box.Size); size != "" {
		tags = append(tags, "size:"+size)
	}
	if runtime := strings.TrimSpace(box.Runtime); runtime != "" {
		tags = append(tags, "runtime:"+runtime)
	}
	if box.KeepAlive {
		tags = append(tags, "keepalive")
	}
	return Instance{
		SyncID:    id,
		Name:      name,
		State:     state,
		HostName:  sshHost,
		User:      id,
		Running:   running,
		KeepAlive: box.KeepAlive,
		Size:      strings.TrimSpace(box.Size),
		Runtime:   strings.TrimSpace(box.Runtime),
		Tags:      tags,
	}, true
}

func normalizeState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "idle", "running", "ready":
		return "running"
	case "paused":
		return "paused"
	case "creating":
		return "creating"
	case "error":
		return "error"
	case "deleted":
		return "deleted"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func isRunningState(status string) bool {
	switch normalizeState(status) {
	case "running", "creating":
		return true
	default:
		return false
	}
}

func isReadyState(status string) bool {
	return normalizeState(status) == "running"
}

func IsStoppedState(status string) bool {
	return normalizeState(status) == "paused"
}

func HostLooksStopped(tags []string) bool {
	return IsStoppedState(StateFromTags(tags))
}

func KeepAliveFromTags(tags []string) bool {
	for _, tag := range tags {
		if tag == "keepalive" {
			return true
		}
	}
	return false
}

func StateFromTags(tags []string) string {
	for _, tag := range tags {
		if state, ok := strings.CutPrefix(tag, "state:"); ok {
			return state
		}
	}
	return ""
}

func ParseSyncID(syncID string) (string, error) {
	id := strings.TrimSpace(syncID)
	if id == "" {
		return "", fmt.Errorf("invalid Upstash Box id %q", syncID)
	}
	if strings.ContainsAny(id, "/\\ \t\r\n") {
		return "", fmt.Errorf("invalid Upstash Box id %q", syncID)
	}
	for _, r := range id {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return "", fmt.Errorf("invalid Upstash Box id %q", syncID)
		}
	}
	if len(id) < 2 || len(id) > 80 {
		return "", fmt.Errorf("invalid Upstash Box id %q", syncID)
	}
	return id, nil
}
