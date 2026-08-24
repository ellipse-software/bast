package railway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	tokenFile := filepath.Join(t.TempDir(), "railway-api-token")
	if err := WriteTokenFile(tokenFile, "rw_testtoken"); err != nil {
		t.Fatal(err)
	}
	return &Client{
		BaseURL:      server.URL,
		TokenFile:    tokenFile,
		PollWait:     time.Millisecond,
		HTTP:         server.Client(),
		IdentityFile: filepath.Join(t.TempDir(), "railway"),
	}
}

func writeGQL(w http.ResponseWriter, data any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func writeGQLErrors(w http.ResponseWriter, messages ...string) {
	errs := make([]map[string]string, 0, len(messages))
	for _, msg := range messages {
		errs = append(errs, map[string]string{"message": msg})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"errors": errs})
}

func decodeGQL(t *testing.T, r *http.Request) gqlRequest {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer rw_testtoken" {
		t.Errorf("Authorization = %q", got)
	}
	var req gqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestListAndDiscover(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeGQL(t, r)
		switch {
		case strings.Contains(req.Query, "me {"):
			writeGQL(w, map[string]any{"me": map[string]any{"name": "Ted", "email": "ted@example.com"}})
		case strings.Contains(req.Query, "projects"):
			writeGQL(w, map[string]any{
				"projects": map[string]any{"edges": []any{
					map[string]any{"node": map[string]any{
						"id": "proj-1", "name": "api",
						"environments": map[string]any{"edges": []any{
							map[string]any{"node": map[string]any{"id": "env-prod", "name": "production"}},
							map[string]any{"node": map[string]any{"id": "env-stg", "name": "staging"}},
						}},
					}},
				}},
			})
		case strings.Contains(req.Query, "serviceInstances"):
			envID, _ := req.Variables["environmentId"].(string)
			name := "web"
			status := "SUCCESS"
			stopped := false
			if envID == "env-stg" {
				name = "web"
				status = "SLEEPING"
				stopped = true
			}
			writeGQL(w, map[string]any{
				"environment": map[string]any{
					"serviceInstances": map[string]any{
						"edges": []any{
							map[string]any{"node": map[string]any{
								"id": "si-1", "serviceId": "svc-web", "serviceName": name,
								"latestDeployment": map[string]any{"id": "dpl-1", "status": status, "deploymentStopped": stopped},
							}},
						},
						"pageInfo": map[string]any{"hasNextPage": false},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	discovery, err := client.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(discovery.Instances))
	}
	if !discovery.Instances[0].Running || discovery.Instances[0].User != "si-1" {
		t.Fatalf("running = %+v", discovery.Instances[0])
	}
	if discovery.Instances[0].SyncID != "proj-1/env-prod/svc-web" {
		t.Fatalf("sync id = %s", discovery.Instances[0].SyncID)
	}
	if GroupPath(discovery.Instances[0]) != "Railway / api / production" {
		t.Fatalf("group = %s", GroupPath(discovery.Instances[0]))
	}
	if AliasFor(discovery.Instances[0]) != "railway_api_web" {
		t.Fatalf("alias = %s", AliasFor(discovery.Instances[0]))
	}
	if discovery.Instances[1].Running || !HostLooksStopped(discovery.Instances[1].Tags) {
		t.Fatalf("sleeping = %+v", discovery.Instances[1])
	}
	if AliasFor(discovery.Instances[1]) != "railway_api_staging_web" {
		t.Fatalf("staging alias = %s", AliasFor(discovery.Instances[1]))
	}
	host := ToSyncHost(discovery.Instances[0], "")
	if host.User != "si-1" || host.HostName != SSHHost || host.SyncSource != ProviderName || !host.IdentitiesOnly {
		t.Fatalf("sync host = %+v", host)
	}
}

func TestCreatePollsUntilReady(t *testing.T) {
	gets := 0
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeGQL(t, r)
		switch {
		case strings.Contains(req.Query, "projectCreate"):
			writeGQL(w, map[string]any{"projectCreate": map[string]any{"id": "proj-new", "name": "sandbox"}})
		case strings.Contains(req.Query, "serviceInstance("):
			gets++
			status := "BUILDING"
			if gets >= 2 {
				status = "SUCCESS"
			}
			writeGQL(w, map[string]any{
				"serviceInstance": map[string]any{
					"id": "si-new", "serviceId": "svc-1", "serviceName": "shell",
					"latestDeployment": map[string]any{"id": "dpl-new", "status": status},
				},
				"project": map[string]any{
					"id": "proj-new", "name": "sandbox",
					"environments": map[string]any{"edges": []any{
						map[string]any{"node": map[string]any{"id": "env-1", "name": "production"}},
					}},
				},
			})
		case strings.Contains(req.Query, "query project"):
			writeGQL(w, map[string]any{"project": map[string]any{
				"id": "proj-new", "name": "sandbox",
				"environments": map[string]any{"edges": []any{
					map[string]any{"node": map[string]any{"id": "env-1", "name": "production"}},
				}},
			}})
		case strings.Contains(req.Query, "serviceCreate"):
			input, _ := req.Variables["input"].(map[string]any)
			source, _ := input["source"].(map[string]any)
			if input["name"] != "shell" || source["image"] != "ubuntu:24.04" {
				t.Errorf("create input = %#v", input)
			}
			writeGQL(w, map[string]any{"serviceCreate": map[string]any{"id": "svc-1", "name": "shell"}})
		case strings.Contains(req.Query, "serviceInstanceUpdate"):
			input, _ := req.Variables["input"].(map[string]any)
			if input["startCommand"] != "sleep infinity" {
				t.Errorf("start = %#v", input)
			}
			writeGQL(w, map[string]any{"serviceInstanceUpdate": true})
		case strings.Contains(req.Query, "serviceInstanceDeployV2"):
			writeGQL(w, map[string]any{"serviceInstanceDeployV2": "dpl-new"})
		default:
			http.NotFound(w, r)
		}
	})
	inst, err := client.Create(context.Background(), CreateOpts{NewProject: "sandbox", Name: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if inst.ServiceID != "svc-1" || inst.User != "si-new" || inst.State != "running" {
		t.Fatalf("inst = %+v", inst)
	}
}

func TestStop(t *testing.T) {
	stopped := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeGQL(t, r)
		if strings.Contains(req.Query, "mutation deploymentStop") {
			stopped = true
			writeGQL(w, map[string]any{"deploymentStop": true})
			return
		}
		status := "SUCCESS"
		if stopped {
			status = "SLEEPING"
		}
		writeGQL(w, map[string]any{
			"serviceInstance": map[string]any{
				"id": "si-1", "serviceId": "svc-web", "serviceName": "web",
				"latestDeployment": map[string]any{"id": "dpl-1", "status": status, "deploymentStopped": stopped},
			},
			"project": map[string]any{
				"id": "proj-1", "name": "api",
				"environments": map[string]any{"edges": []any{
					map[string]any{"node": map[string]any{"id": "env-1", "name": "production"}},
				}},
			},
		})
	})
	if err := client.Stop(context.Background(), "proj-1/env-1/svc-web"); err != nil {
		t.Fatal(err)
	}
}

func TestUnauthorizedRedactsToken(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"errors":[{"message":"unauthorized rw_testtoken"}]}`)
	})
	account, err := client.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.Authenticated || !strings.Contains(account.Error, "401") {
		t.Fatalf("account = %+v", account)
	}
	if strings.Contains(account.Error, "rw_testtoken") {
		t.Fatalf("leaked token: %s", account.Error)
	}
}

func TestResumeRedeploys(t *testing.T) {
	status := "SLEEPING"
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeGQL(t, r)
		switch {
		case strings.Contains(req.Query, "serviceInstanceRedeploy"):
			status = "SUCCESS"
			writeGQL(w, map[string]any{"serviceInstanceRedeploy": true})
		case strings.Contains(req.Query, "serviceInstance("):
			writeGQL(w, map[string]any{
				"serviceInstance": map[string]any{
					"id": "si-1", "serviceId": "svc-web", "serviceName": "web",
					"latestDeployment": map[string]any{"id": "dpl-1", "status": status, "deploymentStopped": status != "SUCCESS"},
				},
				"project": map[string]any{
					"id": "proj-1", "name": "api",
					"environments": map[string]any{"edges": []any{
						map[string]any{"node": map[string]any{"id": "env-1", "name": "production"}},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	if err := client.Resume(context.Background(), "proj-1/env-1/svc-web"); err != nil {
		t.Fatal(err)
	}
}

func TestDelete(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeGQL(t, r)
		if !strings.Contains(req.Query, "serviceDelete") {
			http.NotFound(w, r)
			return
		}
		if req.Variables["id"] != "svc-web" {
			t.Errorf("id = %#v", req.Variables["id"])
		}
		writeGQL(w, map[string]any{"serviceDelete": true})
	})
	if err := client.Delete(context.Background(), "proj-1/env-1/svc-web"); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterSSHKey(t *testing.T) {
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest bast-railway"
	dir := t.TempDir()
	priv := filepath.Join(dir, "railway")
	if err := os.WriteFile(priv, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(priv+".pub", []byte(pub+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	registered := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeGQL(t, r)
		switch {
		case strings.Contains(req.Query, "sshPublicKeys"):
			keys := []any{}
			if registered {
				keys = append(keys, map[string]any{"node": map[string]any{
					"id": "key-1", "name": "bast", "publicKey": pub, "fingerprint": "SHA256:abc",
				}})
			}
			writeGQL(w, map[string]any{"sshPublicKeys": map[string]any{"edges": keys}})
		case strings.Contains(req.Query, "sshPublicKeyCreate"):
			registered = true
			writeGQL(w, map[string]any{"sshPublicKeyCreate": map[string]any{"id": "key-1", "name": "bast"}})
		default:
			http.NotFound(w, r)
		}
	})
	client.IdentityFile = priv
	path, err := client.EnsureIdentity(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != priv || !registered {
		t.Fatalf("path = %s registered = %t", path, registered)
	}
}

func TestWriteAndReadTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := WriteTokenFile(path, " rw_abc \n"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTokenFile(path)
	if err != nil || got != "rw_abc" {
		t.Fatalf("got %q err %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("perm = %o", info.Mode().Perm())
	}
}

func TestParseSyncID(t *testing.T) {
	p, e, s, err := ParseSyncID("proj-1/env-1/svc-1")
	if err != nil || p != "proj-1" || e != "env-1" || s != "svc-1" {
		t.Fatalf("got %s %s %s err %v", p, e, s, err)
	}
	if _, _, _, err := ParseSyncID("../etc/passwd"); err == nil {
		t.Fatal("expected reject")
	}
	if _, _, _, err := ParseSyncID("proj env/svc"); err == nil {
		t.Fatal("expected reject")
	}
	if _, _, _, err := ParseSyncID("only-two/parts"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestAlreadyRegisteredKey(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeGQLErrors(w, "This SSH key is already registered")
	})
	err := client.registerSSHKey(context.Background(), "bast", "ssh-ed25519 AAA")
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("err = %v", err)
	}
}
