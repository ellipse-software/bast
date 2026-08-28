package vercel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	ProviderName   = "vercel"
	DefaultBaseURL = "https://api.vercel.com"
	TokenEnv       = "VERCEL_TOKEN"
	TeamEnv        = "VERCEL_TEAM_ID"
	ProjectEnv     = "VERCEL_PROJECT_ID"
	BaseURLEnv     = "VERCEL_API_URL"
	StoppedHost    = "vercel.sandbox.invalid"
	CleanupTimeout = 30 * time.Minute
	cleanupWorkers = 8
)

type Client struct {
	BaseURL    string
	TokenFile  string
	Token      string // test override; production uses env then TokenFile
	TeamID     string
	ProjectID  string
	ProjectIDs []string
	HTTP       *http.Client
	PollWait   time.Duration
}

type Sandbox struct {
	Name             string            `json:"name"`
	Status           string            `json:"status"`
	Persistent       bool              `json:"persistent"`
	Region           string            `json:"region"`
	VCPUs            int               `json:"vcpus"`
	Memory           int               `json:"memory"`
	Runtime          string            `json:"runtime"`
	Image            string            `json:"image"`
	Timeout          int64             `json:"timeout"`
	CurrentSessionID string            `json:"currentSessionId"`
	CurrentSnapshot  string            `json:"currentSnapshotId"`
	CWD              string            `json:"cwd"`
	Tags             map[string]string `json:"tags"`
	CreatedAt        int64             `json:"createdAt"`
	UpdatedAt        int64             `json:"updatedAt"`
	ExpiresAt        int64             `json:"expiresAt"`
	ProjectID        string            `json:"projectId,omitempty"`
}

type Session struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CWD       string `json:"cwd"`
	Region    string `json:"region"`
	ProjectID string `json:"projectId,omitempty"`
}

type sandboxListResponse struct {
	Sandboxes  []Sandbox `json:"sandboxes"`
	Pagination struct {
		Count int     `json:"count"`
		Next  *string `json:"next"`
	} `json:"pagination"`
}

type sandboxSessionResponse struct {
	Sandbox Sandbox `json:"sandbox"`
	Session Session `json:"session"`
	Resumed bool    `json:"resumed"`
}

type InteractiveSession struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type AccountStatus struct {
	Authenticated bool
	SandboxCount  int
	Error         string
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
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) pollEvery() time.Duration {
	if c.PollWait > 0 {
		return c.PollWait
	}
	return 2 * time.Second
}

func (c *Client) HasToken() bool {
	_, err := c.ResolveToken()
	return err == nil
}

func (c *Client) ResolveTeam() string {
	if team := strings.TrimSpace(c.TeamID); team != "" {
		return team
	}
	return strings.TrimSpace(os.Getenv(TeamEnv))
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	if !c.HasToken() {
		return AccountStatus{Error: "no access token; connect on the Sync tab or set " + TokenEnv}, nil
	}
	if c.ResolveTeam() == "" {
		return AccountStatus{Error: "team is required"}, nil
	}
	if len(c.ResolveProjects()) == 0 {
		return AccountStatus{Error: "project is required"}, nil
	}
	sandboxes, err := c.List(ctx)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "invalid") {
			return AccountStatus{Authenticated: false, Error: msg}, nil
		}
		return AccountStatus{Error: msg}, err
	}
	return AccountStatus{Authenticated: true, SandboxCount: len(sandboxes)}, nil
}

func (c *Client) List(ctx context.Context) ([]Sandbox, error) {
	projects := c.ResolveProjects()
	if len(projects) == 0 {
		return nil, fmt.Errorf("vercel project is required; set %s to one or more project IDs (comma-separated)", ProjectEnv)
	}
	var all []Sandbox
	seen := map[string]bool{}
	for _, project := range projects {
		page, err := c.listProject(ctx, project)
		if err != nil {
			return all, err
		}
		for _, box := range page {
			key := SyncID(project, box.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			if strings.TrimSpace(box.ProjectID) == "" {
				box.ProjectID = project
			}
			all = append(all, box)
		}
	}
	return all, nil
}

func (c *Client) listProject(ctx context.Context, project string) ([]Sandbox, error) {
	var all []Sandbox
	cursor := ""
	for page := 0; page < 100; page++ {
		query := url.Values{}
		if project != "" {
			query.Set("project", project)
		}
		query.Set("limit", "50")
		query.Set("sortBy", "createdAt")
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var raw sandboxListResponse
		if err := c.doJSON(ctx, http.MethodGet, "/v2/sandboxes", query, nil, &raw); err != nil {
			return nil, err
		}
		for _, box := range raw.Sandboxes {
			if project != "" && strings.TrimSpace(box.ProjectID) == "" {
				box.ProjectID = project
			}
			all = append(all, box)
		}
		if raw.Pagination.Next == nil || strings.TrimSpace(*raw.Pagination.Next) == "" {
			return all, nil
		}
		cursor = strings.TrimSpace(*raw.Pagination.Next)
	}
	return all, fmt.Errorf("vercel sandbox list: too many pages")
}

func (c *Client) Get(ctx context.Context, syncID string, resume bool) (sandboxSessionResponse, error) {
	project, name, err := c.parseScopedID(syncID)
	if err != nil {
		return sandboxSessionResponse{}, err
	}
	query := url.Values{}
	if project != "" {
		query.Set("projectId", project)
	}
	if resume {
		query.Set("resume", "true")
	}
	var raw sandboxSessionResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v2/sandboxes/"+url.PathEscape(name), query, nil, &raw); err != nil {
		return sandboxSessionResponse{}, err
	}
	if strings.TrimSpace(raw.Sandbox.Name) == "" {
		return sandboxSessionResponse{}, fmt.Errorf("vercel sandbox %s info was incomplete", name)
	}
	if strings.TrimSpace(raw.Sandbox.ProjectID) == "" {
		if sessionProject := strings.TrimSpace(raw.Session.ProjectID); sessionProject != "" {
			raw.Sandbox.ProjectID = sessionProject
		} else if project != "" {
			raw.Sandbox.ProjectID = project
		}
	}
	return raw, nil
}

func (c *Client) OpenInteractive(ctx context.Context, sessionID string) (InteractiveSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return InteractiveSession{}, fmt.Errorf("session id is required")
	}
	var raw InteractiveSession
	if err := c.doJSON(ctx, http.MethodPost, "/v2/sandboxes/sessions/"+url.PathEscape(sessionID)+"/interactive", nil, map[string]any{}, &raw); err != nil {
		return InteractiveSession{}, err
	}
	if strings.TrimSpace(raw.URL) == "" || strings.TrimSpace(raw.Token) == "" {
		return InteractiveSession{}, fmt.Errorf("vercel interactive session: missing url or token")
	}
	return raw, nil
}

func (c *Client) ExtendTimeout(ctx context.Context, sessionID string, duration time.Duration) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if duration <= 0 {
		duration = 5 * time.Minute
	}
	return c.doJSON(ctx, http.MethodPost, "/v2/sandboxes/sessions/"+url.PathEscape(sessionID)+"/extend-timeout", nil, map[string]any{
		"duration": duration.Milliseconds(),
	}, nil)
}

func (c *Client) requireProject() (string, error) {
	if c.ResolveTeam() == "" {
		return "", fmt.Errorf("vercel team is required")
	}
	project := c.ResolveProject()
	if project == "" {
		return "", fmt.Errorf("vercel project is required to create a sandbox")
	}
	return project, nil
}

func (c *Client) parseScopedID(syncID string) (project, name string, err error) {
	project, name, err = ParseSyncID(syncID)
	if err != nil {
		return "", "", err
	}
	if project == "" {
		project = c.ResolveProject()
	}
	if c.ResolveTeam() == "" {
		return "", "", fmt.Errorf("vercel team is required")
	}
	return project, name, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	token, err := c.ResolveToken()
	if err != nil {
		return err
	}
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	endpoint, err := url.Parse(c.baseURL() + path)
	if err != nil {
		return err
	}
	q := endpoint.Query()
	if team := c.ResolveTeam(); team != "" {
		q.Set("teamId", team)
	}
	for key, values := range query {
		for _, value := range values {
			q.Set(key, value)
		}
	}
	endpoint.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("vercel sandbox: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("vercel sandbox: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp.StatusCode, data, token)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("vercel sandbox: decode response: %w", err)
	}
	return nil
}

func (c *Client) apiError(status int, body []byte, token string) error {
	msg := strings.TrimSpace(string(body))
	var parsed struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
		Code    string          `json:"code"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if parsed.Message != "" {
			msg = parsed.Message
		} else if len(parsed.Error) > 0 {
			var asString string
			if json.Unmarshal(parsed.Error, &asString) == nil && asString != "" {
				msg = asString
			} else {
				var asObj struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				}
				if json.Unmarshal(parsed.Error, &asObj) == nil && asObj.Message != "" {
					msg = asObj.Message
				}
			}
		}
	}
	msg = strings.ReplaceAll(msg, token, "[redacted]")
	if msg == "" {
		msg = http.StatusText(status)
	}
	if status == http.StatusUnauthorized {
		return fmt.Errorf("vercel sandbox API 401: invalid or missing access token")
	}
	if status == http.StatusPaymentRequired {
		return fmt.Errorf("vercel sandbox API 402: %s", msg)
	}
	return fmt.Errorf("vercel sandbox API %d: %s", status, msg)
}
