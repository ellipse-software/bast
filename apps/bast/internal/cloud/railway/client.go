package railway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ProviderName   = "railway"
	DefaultBaseURL = "https://backboard.railway.com/graphql/v2"
	APIKeyEnv      = "RAILWAY_API_TOKEN"
	BaseURLEnv     = "RAILWAY_API_URL"
	SSHHost        = "ssh.railway.com"
	DefaultImage   = "ubuntu:24.04"
	DefaultStart   = "sleep infinity"
	IdentityName   = "railway"
)

type Client struct {
	BaseURL      string
	TokenFile    string
	APIToken     string // test override; production uses env then TokenFile
	HTTP         *http.Client
	PollWait     time.Duration
	IdentityFile string
	SSHKeygen    string
	ManagedKeys  string
}

type AccountStatus struct {
	Authenticated bool
	Name          string
	Email         string
	Error         string
}

type gqlError struct {
	Message string `json:"message"`
}

type gqlEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

type edgeList[T any] struct {
	Edges []struct {
		Node T `json:"node"`
	} `json:"edges"`
	PageInfo *struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
}

func nodes[T any](list edgeList[T]) []T {
	out := make([]T, 0, len(list.Edges))
	for _, edge := range list.Edges {
		out = append(out, edge.Node)
	}
	return out
}

func New(tokenFile string) *Client {
	return &Client{TokenFile: tokenFile}
}

func (c *Client) baseURL() string {
	if u := strings.TrimSpace(c.BaseURL); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := strings.TrimSpace(os.Getenv(BaseURLEnv)); u != "" {
		return strings.TrimRight(u, "/")
	}
	return DefaultBaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 45 * time.Second}
}

func (c *Client) pollEvery() time.Duration {
	if c.PollWait > 0 {
		return c.PollWait
	}
	return 2 * time.Second
}

func (c *Client) identityPath() string {
	if p := strings.TrimSpace(c.IdentityFile); p != "" {
		return p
	}
	if dir := strings.TrimSpace(c.ManagedKeys); dir != "" {
		return filepath.Join(dir, IdentityName)
	}
	return ""
}

func (c *Client) HasToken() bool {
	_, err := c.ResolveToken()
	return err == nil
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	if !c.HasToken() {
		return AccountStatus{Error: "no API token; connect on the Sync tab or set " + APIKeyEnv}, nil
	}
	if project, err := c.projectToken(ctx); err != nil {
		return AccountStatus{Error: err.Error()}, err
	} else if project {
		return AccountStatus{Authenticated: false, Error: "Railway project tokens only cover one environment; create an account or workspace token at railway.com/account/tokens"}, nil
	}
	var data struct {
		Me *struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"me"`
	}
	if err := c.graphql(ctx, `query { me { name email } }`, nil, &data); err != nil {
		if railwayAuthFailure(err) {
			return AccountStatus{Authenticated: false, Error: err.Error()}, nil
		}
		// Workspace tokens cannot query `me` ("Not Authorized") but can list projects.
		projects, listErr := c.ListProjects(ctx)
		if listErr != nil {
			if railwayAuthFailure(listErr) {
				return AccountStatus{Authenticated: false, Error: listErr.Error()}, nil
			}
			return AccountStatus{Error: listErr.Error()}, listErr
		}
		return AccountStatus{Authenticated: true, Name: fmt.Sprintf("%d projects", len(projects))}, nil
	}
	if data.Me == nil {
		return AccountStatus{Authenticated: false, Error: "token did not return an account"}, nil
	}
	return AccountStatus{Authenticated: true, Name: strings.TrimSpace(data.Me.Name), Email: strings.TrimSpace(data.Me.Email)}, nil
}

// projectToken reports whether the configured token authenticates as a
// Railway project token. Probe failures are not fatal: account and workspace
// tokens are rejected by this header and continue through Bearer auth.
func (c *Client) projectToken(ctx context.Context) (bool, error) {
	token, err := c.ResolveToken()
	if err != nil {
		return false, err
	}
	var data struct {
		ProjectToken *struct {
			ProjectID string `json:"projectId"`
		} `json:"projectToken"`
	}
	if err := c.graphqlWithHeader(ctx, "Project-Access-Token", token, token, `query { projectToken { projectId environmentId } }`, nil, &data); err != nil {
		return false, nil
	}
	return data.ProjectToken != nil && strings.TrimSpace(data.ProjectToken.ProjectID) != "", nil
}

func railwayAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	// GraphQL "Not Authorized" is a permission error (workspace tokens on `me`),
	// not proof the token itself is bad.
	if strings.Contains(lower, "not authorized") && !strings.Contains(lower, "401") {
		return false
	}
	return strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized")
}

func (c *Client) graphql(ctx context.Context, query string, variables map[string]any, out any) error {
	token, err := c.ResolveToken()
	if err != nil {
		return err
	}
	return c.graphqlWithHeader(ctx, "Authorization", "Bearer "+token, token, query, variables, out)
}

func (c *Client) graphqlWithHeader(ctx context.Context, header, value, redact string, query string, variables map[string]any, out any) error {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set(header, value)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("railway: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("railway: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp.StatusCode, data, redact)
	}
	var env gqlEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("railway: decode response: %w", err)
	}
	if len(env.Errors) > 0 {
		msgs := make([]string, 0, len(env.Errors))
		for _, item := range env.Errors {
			if msg := strings.TrimSpace(item.Message); msg != "" {
				msgs = append(msgs, msg)
			}
		}
		if len(msgs) == 0 {
			msgs = append(msgs, "graphql error")
		}
		joined := strings.Join(msgs, "; ")
		joined = strings.ReplaceAll(joined, redact, "[redacted]")
		lower := strings.ToLower(joined)
		if strings.Contains(lower, "already registered") {
			return fmt.Errorf("this SSH key is already registered to a different Railway account or workspace; delete ~/.ssh/bast/keys/railway and ~/.ssh/bast/keys/railway.pub, then sync again")
		}
		return fmt.Errorf("railway: %s", joined)
	}
	if out == nil || len(bytes.TrimSpace(env.Data)) == 0 || string(bytes.TrimSpace(env.Data)) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("railway: decode data: %w", err)
	}
	return nil
}

func (c *Client) apiError(status int, body []byte, token string) error {
	msg := strings.TrimSpace(string(body))
	var env gqlEnvelope
	if json.Unmarshal(body, &env) == nil && len(env.Errors) > 0 {
		msgs := make([]string, 0, len(env.Errors))
		for _, item := range env.Errors {
			if m := strings.TrimSpace(item.Message); m != "" {
				msgs = append(msgs, m)
			}
		}
		if len(msgs) > 0 {
			msg = strings.Join(msgs, "; ")
		}
	}
	msg = strings.ReplaceAll(msg, token, "[redacted]")
	if msg == "" {
		msg = http.StatusText(status)
	}
	if status == http.StatusUnauthorized {
		return fmt.Errorf("railway API 401: invalid or missing API token")
	}
	return fmt.Errorf("railway API %d: %s", status, msg)
}
