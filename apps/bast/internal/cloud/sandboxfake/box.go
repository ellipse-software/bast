package sandboxfake

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type BoxRec struct {
	ID       string
	Name     string
	State    string
	IP       string
	Type     string
	Snapshot bool
}

type BoxSnap struct {
	ID    string
	BoxID string
	Kind  string
	Name  string
}

type Box struct {
	mu        sync.Mutex
	seq       int
	Boxes     map[string]*BoxRec
	Snapshots []BoxSnap
	Named     []BoxSnap
}

func NewBox() *Box {
	return &Box{Boxes: map[string]*BoxRec{}}
}

func (b *Box) Put(rec BoxRec) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if rec.Type == "" {
		rec.Type = "default"
	}
	cloned := rec
	b.Boxes[rec.ID] = &cloned
}

func (b *Box) Get(id string) *BoxRec {
	b.mu.Lock()
	defer b.mu.Unlock()
	rec := b.Boxes[id]
	if rec == nil {
		return nil
	}
	copy := *rec
	return &copy
}

func (b *Box) Runner() func(ctx context.Context, args []string, env []string) ([]byte, error) {
	return func(_ context.Context, args []string, _ []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "--version"):
			return []byte("box 1.0.0"), nil
		case containsArg(args, "status"):
			return []byte(`{"ok":true,"user":{"login":"octocat","email":"o@example.com"}}`), nil
		case containsArg(args, "list"):
			return b.listJSON()
		case containsArg(args, "new"):
			return b.create()
		case containsArg(args, "info"):
			return b.info(args)
		case containsArg(args, "stop"):
			return b.stop(args)
		case containsArg(args, "resume"):
			return b.resume(args)
		case containsArg(args, "fork"):
			return b.fork(args)
		case containsArg(args, "snapshots"):
			return b.listSnapshots(args)
		case containsArg(args, "snapshot") && containsArg(args, "delete"):
			return b.deleteSnapshot(args)
		case containsArg(args, "snapshot") && containsArg(args, "rm"):
			return b.removeNamedSnapshot(args)
		case containsArg(args, "delete"):
			return b.deleteBox(args)
		default:
			return nil, fmt.Errorf("unexpected box command %s", joined)
		}
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func (b *Box) listJSON() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	boxes := make([]map[string]any, 0, len(b.Boxes))
	for _, rec := range b.Boxes {
		var ip any
		if rec.IP != "" {
			ip = rec.IP
		}
		boxes = append(boxes, map[string]any{
			"id": rec.ID, "name": rec.Name, "state": rec.State, "ip": ip,
			"snapshotAvailable": rec.Snapshot, "type": rec.Type,
		})
	}
	return json.Marshal(map[string]any{"boxes": boxes})
}

func (b *Box) create() ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := "bx_new" + strconv.Itoa(b.seq)
	b.Boxes[id] = &BoxRec{ID: id, Name: "created-" + strconv.Itoa(b.seq), State: "idle", IP: "203.0.113.20", Type: "default"}
	return json.Marshal(map[string]any{"event": "ready", "id": id, "box": map[string]any{"id": id}})
}

func (b *Box) info(args []string) ([]byte, error) {
	id := argAfter(args, "info")
	b.mu.Lock()
	defer b.mu.Unlock()
	rec := b.Boxes[id]
	if rec == nil {
		return nil, fmt.Errorf("box info: not found")
	}
	var ip any
	if rec.IP != "" {
		ip = rec.IP
	}
	return json.Marshal(map[string]any{"box": map[string]any{
		"id": rec.ID, "name": rec.Name, "state": rec.State, "ip": ip,
		"snapshotAvailable": rec.Snapshot, "type": rec.Type,
	}})
}

func (b *Box) stop(args []string) ([]byte, error) {
	id := argAfter(args, "stop")
	b.mu.Lock()
	defer b.mu.Unlock()
	rec := b.Boxes[id]
	if rec == nil {
		return nil, fmt.Errorf("box stop: not found")
	}
	rec.State = "stopped"
	rec.IP = ""
	rec.Snapshot = true
	return json.Marshal(map[string]any{"ok": true, "id": id})
}

func (b *Box) resume(args []string) ([]byte, error) {
	id := argAfter(args, "resume")
	b.mu.Lock()
	defer b.mu.Unlock()
	rec := b.Boxes[id]
	if rec == nil {
		return nil, fmt.Errorf("box resume: not found")
	}
	rec.State = "idle"
	rec.IP = "203.0.113.21"
	return json.Marshal(map[string]any{"ok": true, "id": id})
}

func (b *Box) fork(args []string) ([]byte, error) {
	id := argAfter(args, "fork")
	b.mu.Lock()
	defer b.mu.Unlock()
	src := b.Boxes[id]
	if src == nil {
		return nil, fmt.Errorf("box fork: not found")
	}
	if !src.Snapshot {
		return nil, fmt.Errorf("box %s has no snapshot yet", id)
	}
	b.seq++
	newID := "bx_fork" + strconv.Itoa(b.seq)
	b.Boxes[newID] = &BoxRec{ID: newID, Name: src.Name + "-fork", State: "idle", IP: "203.0.113.22", Type: src.Type, Snapshot: false}
	return json.Marshal(map[string]any{"ok": true, "id": newID})
}

func (b *Box) deleteBox(args []string) ([]byte, error) {
	id := argAfter(args, "delete")
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Boxes[id] == nil {
		return nil, fmt.Errorf("box delete: not found")
	}
	delete(b.Boxes, id)
	kept := b.Snapshots[:0]
	for _, snap := range b.Snapshots {
		if snap.BoxID != id {
			kept = append(kept, snap)
		}
	}
	b.Snapshots = kept
	return json.Marshal(map[string]any{"ok": true, "id": id})
}

func (b *Box) listSnapshots(args []string) ([]byte, error) {
	id := argAfter(args, "snapshots")
	b.mu.Lock()
	defer b.mu.Unlock()
	snaps := make([]map[string]any, 0)
	for _, snap := range b.Snapshots {
		if id != "" && snap.BoxID != id {
			continue
		}
		snaps = append(snaps, map[string]any{
			"id": snap.ID, "boxId": snap.BoxID, "kind": snap.Kind, "status": "completed",
		})
	}
	named := make([]map[string]any, 0, len(b.Named))
	for _, snap := range b.Named {
		if id != "" && snap.BoxID != id {
			continue
		}
		named = append(named, map[string]any{"id": snap.ID, "boxId": snap.BoxID, "name": snap.Name, "kind": "named"})
	}
	return json.Marshal(map[string]any{"snapshots": snaps, "named": named})
}

func (b *Box) deleteSnapshot(args []string) ([]byte, error) {
	id := argAfter(args, "delete")
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, snap := range b.Snapshots {
		if snap.ID == id {
			b.Snapshots = append(b.Snapshots[:i], b.Snapshots[i+1:]...)
			return json.Marshal(map[string]any{"ok": true, "id": id})
		}
	}
	return nil, fmt.Errorf("box snapshot delete: not found")
}

func (b *Box) removeNamedSnapshot(args []string) ([]byte, error) {
	name := argAfter(args, "rm")
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, snap := range b.Named {
		if snap.Name == name {
			b.Named = append(b.Named[:i], b.Named[i+1:]...)
			return json.Marshal(map[string]any{"ok": true, "id": snap.ID})
		}
	}
	return nil, fmt.Errorf("box snapshot rm: not found")
}

func argAfter(args []string, command string) string {
	for i, arg := range args {
		if arg == command && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			return args[i+1]
		}
	}
	return ""
}
