package hetzner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	ProviderName   = "hetzner"
	DefaultBaseURL = "https://api.hetzner.cloud/v1"
	APIKeyEnv      = "HCLOUD_TOKEN"
	ContextEnv     = "HCLOUD_CONTEXT"
	EndpointEnv    = "HCLOUD_ENDPOINT"
	ConfigEnv      = "HCLOUD_CONFIG"
	maxBody        = 4 << 20
)

type Client struct {
	BaseURL    string
	KeyFile    string
	TokenDir   string
	APIKey     string // test override; production uses env, hcloud config, then TokenDir
	HTTP       *http.Client
	PollWait   time.Duration
	Home       string
	ConfigPath string
	Getenv     func(string) string
}

func New(keyFile, tokenDir, home string) *Client {
	return &Client{KeyFile: keyFile, TokenDir: tokenDir, Home: home}
}

func (c *Client) getenv(key string) string {
	if c.Getenv != nil {
		return c.Getenv(key)
	}
	return os.Getenv(key)
}

func (c *Client) baseURL() string {
	if u := strings.TrimSpace(c.BaseURL); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := strings.TrimSpace(c.getenv(EndpointEnv)); u != "" {
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

type AccountStatus struct {
	HasToken      bool
	Authenticated bool
	Contexts      []string
	Error         string
}

func (c *Client) HasToken() bool {
	contexts, err := c.TokenContexts()
	return err == nil && len(contexts) > 0
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	contexts, err := c.TokenContexts()
	if err != nil {
		return AccountStatus{Error: err.Error()}, nil
	}
	if len(contexts) == 0 {
		return AccountStatus{Error: "no API token; connect on the Sync tab, set " + APIKeyEnv + ", or run bast hetzner key"}, nil
	}
	names := make([]string, 0, len(contexts))
	for _, item := range contexts {
		names = append(names, item.Name)
	}
	status := AccountStatus{HasToken: true, Contexts: names}
	_, _, probeErr := c.listServersPage(ctx, contexts[0].Token, 1)
	if probeErr != nil {
		msg := probeErr.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "invalid") {
			status.Error = msg
			return status, nil
		}
		status.Error = msg
		return status, probeErr
	}
	status.Authenticated = true
	return status, nil
}

func (c *Client) doJSON(ctx context.Context, token, method, path string, query url.Values, body any, out any) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("no Hetzner API token")
	}
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	endpoint := c.baseURL() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
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
		return fmt.Errorf("hetzner: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("hetzner: read response: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return c.retryAfter429(ctx, token, method, path, query, body, out, resp, data)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp.StatusCode, data, token)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("hetzner: decode response: %w", err)
	}
	return nil
}

func (c *Client) retryAfter429(ctx context.Context, token, method, path string, query url.Values, body, out any, resp *http.Response, data []byte) error {
	if c.getenv("HETZNER_NO_RETRY") == "1" {
		return c.apiError(http.StatusTooManyRequests, data, token)
	}
	wait := c.pollEvery()
	if raw := strings.TrimSpace(resp.Header.Get("RateLimit-Reset")); raw != "" {
		if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
			until := time.Unix(unix, 0)
			if delay := time.Until(until); delay > 0 && delay < 30*time.Second {
				wait = delay
			}
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
	}
	reqErr := c.doJSONOnce(ctx, token, method, path, query, body, out)
	return reqErr
}

func (c *Client) doJSONOnce(ctx context.Context, token, method, path string, query url.Values, body any, out any) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	endpoint := c.baseURL() + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
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
		return fmt.Errorf("hetzner: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("hetzner: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp.StatusCode, data, token)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("hetzner: decode response: %w", err)
	}
	return nil
}

func (c *Client) apiError(status int, body []byte, token string) error {
	msg := strings.TrimSpace(string(body))
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if parsed.Error.Message != "" {
			msg = parsed.Error.Message
		} else if parsed.Message != "" {
			msg = parsed.Message
		}
	}
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("hetzner API 401: invalid or missing API token")
	case http.StatusForbidden:
		return fmt.Errorf("hetzner API 403: %s; start, stop, and restart need a Read & Write API token (Hetzner Console → Security → API Tokens)", msg)
	default:
		return fmt.Errorf("hetzner API %d: %s", status, msg)
	}
}

type pagination struct {
	Page         int  `json:"page"`
	PerPage      int  `json:"per_page"`
	PreviousPage *int `json:"previous_page"`
	NextPage     *int `json:"next_page"`
	LastPage     int  `json:"last_page"`
	TotalEntries int  `json:"total_entries"`
}

type listMeta struct {
	Pagination pagination `json:"pagination"`
}

type apiServer struct {
	ID         int               `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	PublicNet  apiPublicNet      `json:"public_net"`
	PrivateNet []apiPrivateNet   `json:"private_net"`
	ServerType apiNamed          `json:"server_type"`
	Datacenter apiDatacenter     `json:"datacenter"`
	Location   apiNamed          `json:"location"`
	Image      *apiImage         `json:"image"`
	Labels     map[string]string `json:"labels"`
	SSHKeys    []sshKeyID        `json:"ssh_keys"`
	Locked     bool              `json:"locked"`
}

type apiNamed struct {
	Name string `json:"name"`
}

type apiDatacenter struct {
	Name     string   `json:"name"`
	Location apiNamed `json:"location"`
}

type apiImage struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	OSFlavor    string `json:"os_flavor"`
}

type apiPublicNet struct {
	IPv4 *apiPrimaryIP `json:"ipv4"`
	IPv6 *apiPrimaryIP `json:"ipv6"`
}

type apiPrimaryIP struct {
	IP      string `json:"ip"`
	Blocked bool   `json:"blocked"`
}

type apiPrivateNet struct {
	IP string `json:"ip"`
}

type sshKeyID struct {
	ID int
}

func (s *sshKeyID) UnmarshalJSON(b []byte) error {
	var id int
	if err := json.Unmarshal(b, &id); err == nil {
		s.ID = id
		return nil
	}
	var obj struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	s.ID = obj.ID
	return nil
}

type apiSSHKey struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

type apiAction struct {
	ID       int    `json:"id"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Error    *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) listServersPage(ctx context.Context, token string, page int) (servers []apiServer, next *int, err error) {
	query := url.Values{}
	query.Set("page", strconv.Itoa(page))
	query.Set("per_page", "50")
	var out struct {
		Servers []apiServer `json:"servers"`
		Meta    listMeta    `json:"meta"`
	}
	if err := c.doJSON(ctx, token, http.MethodGet, "/servers", query, nil, &out); err != nil {
		return nil, nil, err
	}
	return out.Servers, out.Meta.Pagination.NextPage, nil
}

func (c *Client) listServers(ctx context.Context, token string) ([]apiServer, error) {
	var all []apiServer
	page := 1
	for {
		servers, next, err := c.listServersPage(ctx, token, page)
		if err != nil {
			return nil, err
		}
		all = append(all, servers...)
		if next == nil || *next <= page {
			return all, nil
		}
		page = *next
	}
}

func (c *Client) listSSHKeys(ctx context.Context, token string) ([]apiSSHKey, error) {
	var all []apiSSHKey
	page := 1
	for {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", "50")
		var out struct {
			SSHKeys []apiSSHKey `json:"ssh_keys"`
			Meta    listMeta    `json:"meta"`
		}
		if err := c.doJSON(ctx, token, http.MethodGet, "/ssh_keys", query, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.SSHKeys...)
		next := out.Meta.Pagination.NextPage
		if next == nil || *next <= page {
			return all, nil
		}
		page = *next
	}
}

func (c *Client) getServer(ctx context.Context, token string, id int) (apiServer, error) {
	var out struct {
		Server apiServer `json:"server"`
	}
	if err := c.doJSON(ctx, token, http.MethodGet, "/servers/"+strconv.Itoa(id), nil, nil, &out); err != nil {
		return apiServer{}, err
	}
	if out.Server.ID == 0 {
		return apiServer{}, fmt.Errorf("hetzner server %d info was incomplete", id)
	}
	return out.Server, nil
}
