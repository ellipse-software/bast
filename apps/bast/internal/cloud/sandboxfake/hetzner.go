package sandboxfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const HetznerToken = "hetzner_test_token"

type HetznerServer struct {
	ID       int
	Name     string
	Status   string
	IPv4     string
	Location string
}

type Hetzner struct {
	mu      sync.Mutex
	seq     int
	servers map[int]*HetznerServer
	Server  *httptest.Server
}

func NewHetzner(t *testing.T) *Hetzner {
	t.Helper()
	api := &Hetzner{servers: map[int]*HetznerServer{}}
	api.Server = httptest.NewServer(api)
	t.Cleanup(api.Server.Close)
	return api
}

func (a *Hetzner) URL() string { return a.Server.URL }

func (a *Hetzner) Put(srv HetznerServer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if srv.Location == "" {
		srv.Location = "fsn1"
	}
	if srv.IPv4 == "" {
		srv.IPv4 = "203.0.113.10"
	}
	cloned := srv
	a.servers[srv.ID] = &cloned
}

func (a *Hetzner) Get(id int) *HetznerServer {
	a.mu.Lock()
	defer a.mu.Unlock()
	srv := a.servers[id]
	if srv == nil {
		return nil
	}
	copy := *srv
	return &copy
}

func (a *Hetzner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+HetznerToken {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "unauthorized"}})
		return
	}
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/servers":
		a.serveList(w)
	case r.Method == http.MethodGet && path == "/ssh_keys":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ssh_keys": []any{},
			"meta":     map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil, "last_page": 1}},
		})
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/servers/"):
		a.serveGet(w, path)
	case r.Method == http.MethodPost && strings.Contains(path, "/actions/"):
		a.serveAction(w, path)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/actions/"):
		_ = json.NewEncoder(w).Encode(map[string]any{"action": map[string]any{"id": 1, "status": "success"}})
	default:
		http.NotFound(w, r)
	}
}

func (a *Hetzner) serveList(w http.ResponseWriter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	list := make([]map[string]any, 0, len(a.servers))
	for _, srv := range a.servers {
		list = append(list, hetznerJSON(srv))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"servers": list,
		"meta":    map[string]any{"pagination": map[string]any{"page": 1, "next_page": nil, "last_page": 1}},
	})
}

func (a *Hetzner) serveGet(w http.ResponseWriter, path string) {
	id, err := strconv.Atoi(strings.TrimPrefix(path, "/servers/"))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	srv := a.servers[id]
	if srv == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"server": hetznerJSON(srv)})
}

func (a *Hetzner) serveAction(w http.ResponseWriter, path string) {
	rest := strings.TrimPrefix(path, "/servers/")
	idStr, command, ok := strings.Cut(rest, "/actions/")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	srv := a.servers[id]
	if srv == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch command {
	case "poweron":
		srv.Status = "running"
	case "shutdown", "poweroff":
		srv.Status = "off"
	case "reboot", "reset":
		srv.Status = "running"
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	a.seq++
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"action": map[string]any{"id": a.seq, "status": "success"}})
}

func hetznerJSON(srv *HetznerServer) map[string]any {
	return map[string]any{
		"id": srv.ID, "name": srv.Name, "status": srv.Status,
		"public_net":  map[string]any{"ipv4": map[string]any{"ip": srv.IPv4, "blocked": false}, "ipv6": nil},
		"private_net": []any{},
		"server_type": map[string]any{"name": "cx22"},
		"datacenter":  map[string]any{"name": srv.Location + "-dc", "location": map[string]any{"name": srv.Location}},
		"image":       map[string]any{"name": "ubuntu-24.04", "os_flavor": "ubuntu"},
		"ssh_keys":    []any{},
		"labels":      map[string]any{},
	}
}
