package sandboxfake

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const UpstashKey = "box_testkey"

type UpstashBox struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Size      string `json:"size"`
	Runtime   string `json:"runtime"`
	KeepAlive bool   `json:"keep_alive"`
}

type UpstashSnapshot struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	BoxID  string `json:"box_id"`
	Status string `json:"status"`
}

type Upstash struct {
	mu        sync.Mutex
	seq       int
	boxes     map[string]*UpstashBox
	snapshots map[string][]UpstashSnapshot
	ListCalls int
	Server    *httptest.Server
}

func NewUpstash(t *testing.T) *Upstash {
	t.Helper()
	api := &Upstash{boxes: map[string]*UpstashBox{}, snapshots: map[string][]UpstashSnapshot{}}
	api.Server = httptest.NewServer(api)
	t.Cleanup(api.Server.Close)
	return api
}

func (a *Upstash) URL() string { return a.Server.URL }

func (a *Upstash) Put(box UpstashBox) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if box.Size == "" {
		box.Size = "small"
	}
	if box.Runtime == "" {
		box.Runtime = "node"
	}
	cloned := box
	a.boxes[box.ID] = &cloned
}

func (a *Upstash) Get(id string) *UpstashBox {
	a.mu.Lock()
	defer a.mu.Unlock()
	box := a.boxes[id]
	if box == nil {
		return nil
	}
	copy := *box
	return &copy
}

func (a *Upstash) IDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids := make([]string, 0, len(a.boxes))
	for id := range a.boxes {
		ids = append(ids, id)
	}
	return ids
}

func (a *Upstash) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Box-Api-Key") != UpstashKey {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
		return
	}
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/v2/box":
		a.serveList(w)
	case r.Method == http.MethodPost && path == "/v2/box":
		a.serveCreate(w, r)
	case r.Method == http.MethodPost && path == "/v2/box/from-snapshot":
		a.serveFromSnapshot(w, r)
	case strings.HasSuffix(path, "/pause") && r.Method == http.MethodPost:
		a.mutateBox(w, strings.TrimSuffix(strings.TrimPrefix(path, "/v2/box/"), "/pause"), func(box *UpstashBox) int {
			if box.KeepAlive {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "keep-alive"})
				return http.StatusBadRequest
			}
			box.Status = "paused"
			return http.StatusOK
		})
	case strings.HasSuffix(path, "/resume") && r.Method == http.MethodPost:
		a.mutateBox(w, strings.TrimSuffix(strings.TrimPrefix(path, "/v2/box/"), "/resume"), func(box *UpstashBox) int {
			box.Status = "idle"
			return http.StatusOK
		})
	case strings.HasSuffix(path, "/snapshots") && r.Method == http.MethodPost:
		a.serveCreateSnapshot(w, strings.TrimSuffix(strings.TrimPrefix(path, "/v2/box/"), "/snapshots"))
	case strings.HasSuffix(path, "/snapshots") && r.Method == http.MethodGet:
		a.serveListSnapshots(w, strings.TrimSuffix(strings.TrimPrefix(path, "/v2/box/"), "/snapshots"))
	case strings.HasPrefix(path, "/v2/box/") && r.Method == http.MethodGet:
		a.serveGet(w, strings.TrimPrefix(path, "/v2/box/"))
	case strings.HasPrefix(path, "/v2/box/") && r.Method == http.MethodDelete:
		a.serveDelete(w, strings.TrimPrefix(path, "/v2/box/"))
	default:
		http.NotFound(w, r)
	}
}

func (a *Upstash) serveList(w http.ResponseWriter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ListCalls++
	list := make([]UpstashBox, 0, len(a.boxes))
	for _, box := range a.boxes {
		list = append(list, *box)
	}
	_ = json.NewEncoder(w).Encode(list)
}

func (a *Upstash) serveCreate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	id := "current-box-" + strconv.Itoa(a.seq)
	name, _ := body["name"].(string)
	if name == "" {
		name = id
	}
	runtime, _ := body["runtime"].(string)
	if runtime == "" {
		runtime = "node"
	}
	size, _ := body["size"].(string)
	if size == "" {
		size = "small"
	}
	keep, _ := body["keep_alive"].(bool)
	box := &UpstashBox{ID: id, Name: name, Status: "idle", Size: size, Runtime: runtime, KeepAlive: keep}
	a.boxes[id] = box
	_ = json.NewEncoder(w).Encode(box)
}

func (a *Upstash) serveGet(w http.ResponseWriter, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	box := a.boxes[id]
	if box == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(box)
}

func (a *Upstash) mutateBox(w http.ResponseWriter, id string, fn func(*UpstashBox) int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	box := a.boxes[id]
	if box == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	status := fn(box)
	if status != http.StatusOK {
		w.WriteHeader(status)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *Upstash) serveDelete(w http.ResponseWriter, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.boxes[id]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	delete(a.boxes, id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Upstash) serveCreateSnapshot(w http.ResponseWriter, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.boxes[id] == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	a.seq++
	snap := UpstashSnapshot{ID: "snap-" + strconv.Itoa(a.seq), Name: "bast-fork", BoxID: id, Status: "ready"}
	a.snapshots[id] = append(a.snapshots[id], snap)
	_ = json.NewEncoder(w).Encode(snap)
}

func (a *Upstash) serveListSnapshots(w http.ResponseWriter, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"snapshots": a.snapshots[id]})
}

func (a *Upstash) serveFromSnapshot(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	id := "fork-box-" + strconv.Itoa(a.seq)
	runtime, _ := body["runtime"].(string)
	size, _ := body["size"].(string)
	keep, _ := body["keep_alive"].(bool)
	box := &UpstashBox{ID: id, Name: id, Status: "idle", Size: size, Runtime: runtime, KeepAlive: keep}
	a.boxes[id] = box
	_ = json.NewEncoder(w).Encode(box)
}
