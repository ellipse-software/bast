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

const (
	VercelToken   = "vercel_test_token"
	VercelTeam    = "team_1"
	VercelProject = "prj_1"
)

type VercelSandbox struct {
	Name             string
	Status           string
	Persistent       bool
	VCPUs            int
	Runtime          string
	CurrentSessionID string
	CurrentSnapshot  string
	Project          string
}

type Vercel struct {
	mu            sync.Mutex
	seq           int
	sandboxes     map[string]*VercelSandbox
	ListCalls     int
	AllowTeamWide bool
	Server        *httptest.Server
}

func NewVercel(t *testing.T) *Vercel {
	t.Helper()
	api := &Vercel{sandboxes: map[string]*VercelSandbox{}}
	api.Server = httptest.NewServer(api)
	t.Cleanup(api.Server.Close)
	return api
}

func (a *Vercel) URL() string { return a.Server.URL }

func (a *Vercel) Get(name string) *VercelSandbox {
	a.mu.Lock()
	defer a.mu.Unlock()
	box := a.sandboxes[name]
	if box == nil {
		return nil
	}
	copy := *box
	return &copy
}

func (a *Vercel) Put(box VercelSandbox) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if box.Runtime == "" {
		box.Runtime = "node24"
	}
	if box.VCPUs == 0 {
		box.VCPUs = 2
	}
	if box.CurrentSessionID == "" && box.Status == "running" {
		box.CurrentSessionID = "sbx_" + box.Name
	}
	if box.Project == "" {
		box.Project = VercelProject
	}
	cloned := box
	a.sandboxes[box.Name] = &cloned
}

func (a *Vercel) Names() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	names := make([]string, 0, len(a.sandboxes))
	for name := range a.sandboxes {
		names = append(names, name)
	}
	return names
}

func (a *Vercel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+VercelToken {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "unauthorized"}})
		return
	}
	if r.URL.Query().Get("teamId") != VercelTeam {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "team required"}})
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v2/sandboxes":
		a.serveList(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/v2/sandboxes":
		a.serveCreate(w, r)
	case strings.HasPrefix(r.URL.Path, "/v2/sandboxes/sessions/") && strings.HasSuffix(r.URL.Path, "/stop") && r.Method == http.MethodPost:
		a.serveStop(w, r)
	case strings.HasSuffix(r.URL.Path, "/fork") && r.Method == http.MethodPost:
		a.serveFork(w, r)
	case strings.HasPrefix(r.URL.Path, "/v2/sandboxes/") && r.Method == http.MethodGet:
		a.serveGet(w, r)
	case strings.HasPrefix(r.URL.Path, "/v2/sandboxes/") && r.Method == http.MethodDelete:
		a.serveDelete(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (a *Vercel) serveList(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ListCalls++
	project := r.URL.Query().Get("project")
	if project == "" && !a.AllowTeamWide {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "project required"}})
		return
	}
	list := make([]map[string]any, 0, len(a.sandboxes))
	for _, box := range a.sandboxes {
		if project != "" {
			want := box.Project
			if want == "" {
				want = VercelProject
			}
			if want != project {
				continue
			}
		}
		list = append(list, vercelJSON(box))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sandboxes": list, "pagination": map[string]any{"count": len(list)}})
}

func (a *Vercel) serveCreate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	name, _ := body["name"].(string)
	if name == "" {
		name = "sandbox-" + strconv.Itoa(a.seq)
	}
	persistent := true
	if v, ok := body["persistent"].(bool); ok {
		persistent = v
	}
	vcpus := 2
	if resources, ok := body["resources"].(map[string]any); ok {
		if n, ok := resources["vcpus"].(float64); ok {
			vcpus = int(n)
		}
	}
	box := &VercelSandbox{
		Name: name, Status: "running", Persistent: persistent, VCPUs: vcpus, Runtime: "node24",
		CurrentSessionID: "sbx_" + name,
	}
	a.sandboxes[name] = box
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sandbox": vercelJSON(box),
		"session": map[string]any{"id": box.CurrentSessionID, "status": "running"},
	})
}

func (a *Vercel) serveGet(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v2/sandboxes/")
	a.mu.Lock()
	defer a.mu.Unlock()
	box := a.sandboxes[name]
	if box == nil {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("resume") == "true" {
		box.Status = "running"
		if box.CurrentSessionID == "" {
			box.CurrentSessionID = "sbx_" + box.Name
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sandbox": vercelJSON(box),
		"session": map[string]any{"id": box.CurrentSessionID, "status": box.Status},
	})
}

func (a *Vercel) serveStop(w http.ResponseWriter, r *http.Request) {
	session := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/sandboxes/sessions/"), "/stop")
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, box := range a.sandboxes {
		if box.CurrentSessionID == session {
			box.Status = "stopped"
			if box.Persistent {
				box.CurrentSnapshot = "snap_" + box.Name
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	http.NotFound(w, r)
}

func (a *Vercel) serveFork(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/sandboxes/"), "/fork")
	var body map[string]any
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
	a.mu.Lock()
	defer a.mu.Unlock()
	src := a.sandboxes[source]
	if src == nil {
		http.NotFound(w, r)
		return
	}
	name, _ := body["name"].(string)
	if name == "" {
		name = source + "-fork"
	}
	box := &VercelSandbox{
		Name: name, Status: "running", Persistent: src.Persistent, VCPUs: src.VCPUs, Runtime: src.Runtime,
		CurrentSessionID: "sbx_" + name,
	}
	a.sandboxes[name] = box
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sandbox": vercelJSON(box),
		"session": map[string]any{"id": box.CurrentSessionID, "status": "running"},
	})
}

func (a *Vercel) serveDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v2/sandboxes/")
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.sandboxes[name]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(a.sandboxes, name)
	w.WriteHeader(http.StatusNoContent)
}

func vercelJSON(box *VercelSandbox) map[string]any {
	project := box.Project
	if project == "" {
		project = VercelProject
	}
	return map[string]any{
		"name": box.Name, "status": box.Status, "persistent": box.Persistent,
		"vcpus": box.VCPUs, "runtime": box.Runtime,
		"currentSessionId": box.CurrentSessionID, "currentSnapshotId": box.CurrentSnapshot,
		"projectId": project,
	}
}
