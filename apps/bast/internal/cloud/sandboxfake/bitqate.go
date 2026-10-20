package sandboxfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const BitqateToken = "bitqate_test_token"

type BitqateVM struct {
	ID             string
	Name           string
	Status         string
	IPv4           string
	Region         string
	OS             string
	Fingerprint    string
	AuthorizedKey  string
	ReadOnlyTokens map[string]bool
}

type Bitqate struct {
	mu              sync.Mutex
	vms             map[string]*BitqateVM
	Server          *httptest.Server
	RateHit         bool
	Conflict        bool
	ReadOnlyTokens  map[string]bool
	PowerOnlyTokens map[string]bool
	fail            *bitqateFail
}

func NewBitqate(t *testing.T) *Bitqate {
	t.Helper()
	api := &Bitqate{vms: map[string]*BitqateVM{}}
	api.Server = httptest.NewServer(api)
	t.Cleanup(api.Server.Close)
	return api
}

func (a *Bitqate) URL() string { return a.Server.URL }

func (a *Bitqate) Put(vm BitqateVM) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cloned := vm
	a.vms[vm.ID] = &cloned
}

func (a *Bitqate) Get(id string) *BitqateVM {
	a.mu.Lock()
	defer a.mu.Unlock()
	vm := a.vms[id]
	if vm == nil {
		return nil
	}
	copy := *vm
	return &copy
}

func (a *Bitqate) Fail(status int, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fail = &bitqateFail{status: status, message: message}
}

func (a *Bitqate) failErr() *bitqateFail {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fail
}

type bitqateFail struct {
	status  int
	message string
}

func (a *Bitqate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	rateHit := a.RateHit
	a.RateHit = false
	a.mu.Unlock()
	if rateHit {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "rate limited"})
		return
	}
	if a.failErr() != nil {
		f := a.failErr()
		w.WriteHeader(f.status)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": f.message})
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" || (token != BitqateToken && !a.isToken(token)) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Unauthorized"})
		return
	}
	path := r.URL.Path
	if r.Method == http.MethodGet && a.isPowerOnly(token) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Token is missing the vms:read scope"})
		return
	}
	switch {
	case r.Method == http.MethodGet && path == "/vms":
		a.serveList(w)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/vms/"):
		a.serveGet(w, path)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/vms/"):
		a.servePower(w, r, path, token)
	default:
		http.NotFound(w, r)
	}
}

func (a *Bitqate) isToken(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.ReadOnlyTokens[token]; ok {
		return true
	}
	_, ok := a.PowerOnlyTokens[token]
	return ok
}

func bitqateJSON(vm *BitqateVM) map[string]any {
	var region, os any
	if vm.Region != "" {
		region = map[string]any{"id": vm.Region, "name": vm.Region}
	}
	if vm.OS != "" {
		os = map[string]any{"id": vm.OS, "name": vm.OS}
	}
	return map[string]any{
		"id": vm.ID, "name": vm.Name, "status": vm.Status, "ipv4": vm.IPv4,
		"os": os, "region": region,
		"resources": map[string]any{"memory_mb": 4096, "cpu_cores": 2, "disk_gb": 80},
		"ssh": map[string]any{
			"host_key_fingerprint": vm.Fingerprint,
			"authorized_key":       vm.AuthorizedKey,
		},
		"created_at": "2025-01-01T00:00:00Z", "expires_at": nil,
	}
}

func (a *Bitqate) serveList(w http.ResponseWriter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	list := make([]map[string]any, 0, len(a.vms))
	for _, vm := range a.vms {
		list = append(list, bitqateJSON(vm))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "vms": list})
}

func (a *Bitqate) serveGet(w http.ResponseWriter, path string) {
	id := strings.TrimPrefix(path, "/vms/")
	a.mu.Lock()
	defer a.mu.Unlock()
	vm := a.vms[id]
	if vm == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "VM not found"})
		return
	}
	out := bitqateJSON(vm)
	out["ipv6_addresses"] = []any{map[string]any{"address": "2001:db8::1", "cidr": 64}}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "vm": out})
}

func (a *Bitqate) isPowerOnly(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.PowerOnlyTokens[token]
	return ok
}

func (a *Bitqate) servePower(w http.ResponseWriter, r *http.Request, path, token string) {
	if a.ReadOnlyTokens[token] {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "Token is missing the vms:power scope"})
		return
	}
	rest := strings.TrimPrefix(path, "/vms/")
	id, action, ok := strings.Cut(rest, "/")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	vm := a.vms[id]
	if vm == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "VM not found"})
		return
	}
	if a.Conflict {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "VM is busy"})
		return
	}
	if vm.Status == "suspended" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "VM is suspended"})
		return
	}
	switch action {
	case "start":
		if vm.Status != "stopped" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		vm.Status = "running"
	case "stop":
		vm.Status = "stopped"
	case "restart":
		if vm.Status != "running" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		vm.Status = "running"
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "VM " + action + " accepted"})
}
