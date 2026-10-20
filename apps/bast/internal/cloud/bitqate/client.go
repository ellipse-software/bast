package bitqate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bast/internal/platform"
)

const (
	ProviderName   = "bitqate"
	DefaultBaseURL = "https://api.bitqate.com/v1"
	APIKeyEnv      = "BITQATE_TOKEN"
	BaseURLEnv     = "BITQATE_API_URL"
	maxBody        = 4 << 20
)

type Client struct {
	BaseURL  string
	KeyFile  string
	APIKey   string
	HTTP     *http.Client
	PollWait time.Duration
	Getenv   func(string) string
}

func New(keyFile string) *Client {
	return &Client{KeyFile: keyFile}
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
	if u := strings.TrimSpace(c.getenv(BaseURLEnv)); u != "" {
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
	Error         string
}

func (c *Client) HasToken() bool {
	_, err := c.ResolveToken()
	return err == nil
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	if !c.HasToken() {
		return AccountStatus{Error: "no API token; connect on the Sync tab, set " + APIKeyEnv + ", or run bast bitqate key"}, nil
	}
	if _, err := c.listVMs(ctx); err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "invalid") {
			return AccountStatus{HasToken: true, Error: msg}, nil
		}
		return AccountStatus{HasToken: true, Error: msg}, err
	}
	return AccountStatus{HasToken: true, Authenticated: true}, nil
}

func (c *Client) ResolveToken() (string, error) {
	if key := strings.TrimSpace(c.APIKey); key != "" {
		return key, nil
	}
	if key := strings.TrimSpace(c.getenv(APIKeyEnv)); key != "" {
		return key, nil
	}
	return ReadTokenFile(c.KeyFile)
}

func ReadTokenFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("no BitQate API token; connect on the Sync tab, set %s, or run bast bitqate key", APIKeyEnv)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no BitQate API token; connect on the Sync tab, set %s, or run bast bitqate key", APIKeyEnv)
		}
		return "", fmt.Errorf("read BitQate API token: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("BitQate API token file is empty")
	}
	return key, nil
}

func WriteTokenFile(path, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("BitQate API token is required")
	}
	if strings.ContainsAny(key, "\r\n") {
		return fmt.Errorf("BitQate API token must be a single line")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := platform.SecurePath(dir, 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(key+"\n"), 0600); err != nil {
		return fmt.Errorf("write BitQate API token: %w", err)
	}
	if err := platform.ReplaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return platform.SecurePath(path, 0600)
}

func (c *Client) SaveToken(key string) error {
	if err := WriteTokenFile(c.KeyFile, key); err != nil {
		return err
	}
	c.APIKey = ""
	return nil
}

func (c *Client) DeleteToken() error {
	if err := os.Remove(c.KeyFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove BitQate API token: %w", err)
	}
	c.APIKey = ""
	return nil
}

type apiNamed struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type apiVM struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Status string    `json:"status"`
	IPv4   string    `json:"ipv4"`
	OS     *apiNamed `json:"os"`
	Region *apiNamed `json:"region"`
	SSH    struct {
		HostKeyFingerprint string `json:"host_key_fingerprint"`
		AuthorizedKey      string `json:"authorized_key"`
	} `json:"ssh"`
}

func (c *Client) doJSON(ctx context.Context, method, path string, out any) error {
	token, err := c.ResolveToken()
	if err != nil {
		return err
	}
	return c.doJSONOnce(ctx, token, method, path, out, true)
}

func (c *Client) doJSONOnce(ctx context.Context, token, method, path string, out any, allowRetry bool) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("bitqate: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("bitqate: read response: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests && allowRetry {
		return c.retryAfter429(ctx, token, method, path, out, resp, data)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp.StatusCode, data, token)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("bitqate: decode response: %w", err)
	}
	return nil
}

func (c *Client) retryAfter429(ctx context.Context, token, method, path string, out any, resp *http.Response, data []byte) error {
	if c.getenv("BITQATE_NO_RETRY") == "1" {
		return c.apiError(http.StatusTooManyRequests, data, token)
	}
	wait := c.pollEvery()
	if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil {
			if delay := time.Duration(secs) * time.Second; delay > 0 && delay < 30*time.Second {
				wait = delay
			}
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
	}
	return c.doJSONOnce(ctx, token, method, path, out, false)
}

func (c *Client) apiError(status int, body []byte, token string) error {
	msg := strings.TrimSpace(string(body))
	var parsed struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Message != "" {
		msg = parsed.Message
	}
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("bitqate API 401: invalid or missing API token")
	case http.StatusForbidden:
		return fmt.Errorf("bitqate API 403: %s; mint a token with the required scopes in the BitQate dashboard → Settings → API", msg)
	default:
		return fmt.Errorf("bitqate API %d: %s", status, msg)
	}
}

func (c *Client) listVMs(ctx context.Context) ([]apiVM, error) {
	var out struct {
		VMs []apiVM `json:"vms"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/vms", &out); err != nil {
		return nil, err
	}
	return out.VMs, nil
}

func (c *Client) getVM(ctx context.Context, id string) (apiVM, error) {
	var out struct {
		VM apiVM `json:"vm"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/vms/"+url.PathEscape(id), &out); err != nil {
		return apiVM{}, err
	}
	if strings.TrimSpace(out.VM.ID) == "" {
		return apiVM{}, fmt.Errorf("bitqate VM %s info was incomplete", id)
	}
	return out.VM, nil
}

func (c *Client) vmAction(ctx context.Context, id, action string) error {
	var out struct {
		Message string `json:"message"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/vms/"+url.PathEscape(id)+"/"+action, &out); err != nil {
		return err
	}
	return nil
}
