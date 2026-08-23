package upstash

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
	ProviderName   = "upstash"
	DefaultBaseURL = "https://us-east-1.box.upstash.com"
	APIKeyEnv      = "UPSTASH_BOX_API_KEY"
	BaseURLEnv     = "UPSTASH_BOX_BASE_URL"
)

type Client struct {
	BaseURL  string
	KeyFile  string
	APIKey   string // test override; production uses env then KeyFile
	HTTP     *http.Client
	PollWait time.Duration
}

type BoxData struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Labels    []string `json:"labels"`
	Size      string   `json:"size"`
	Runtime   string   `json:"runtime"`
	Status    string   `json:"status"`
	KeepAlive bool     `json:"keep_alive"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

type Snapshot struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BoxID     string `json:"box_id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

type AccountStatus struct {
	Authenticated bool
	BoxCount      int
	Error         string
}

func New(keyFile string) *Client {
	return &Client{KeyFile: keyFile}
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

func (c *Client) SSHHost() string {
	raw := c.baseURL()
	u, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(u.Host) == "" {
		return "us-east-1.box.upstash.com"
	}
	return u.Host
}

func (c *Client) HasKey() bool {
	_, err := c.ResolveKey()
	return err == nil
}

func (c *Client) Account(ctx context.Context) (AccountStatus, error) {
	if !c.HasKey() {
		return AccountStatus{Error: "no API key; connect on the Sync tab or set " + APIKeyEnv}, nil
	}
	boxes, err := c.List(ctx)
	if err != nil {
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "401") || strings.Contains(lower, "invalid") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "api key") {
			return AccountStatus{Authenticated: false, Error: msg}, nil
		}
		return AccountStatus{Error: msg}, err
	}
	return AccountStatus{Authenticated: true, BoxCount: len(boxes)}, nil
}

func (c *Client) List(ctx context.Context) ([]BoxData, error) {
	var boxes []BoxData
	if err := c.doJSON(ctx, http.MethodGet, "/v2/box", nil, &boxes); err != nil {
		return nil, err
	}
	return boxes, nil
}

func (c *Client) Get(ctx context.Context, id string) (BoxData, error) {
	id, err := ParseSyncID(id)
	if err != nil {
		return BoxData{}, err
	}
	var box BoxData
	if err := c.doJSON(ctx, http.MethodGet, "/v2/box/"+url.PathEscape(id), nil, &box); err != nil {
		return BoxData{}, err
	}
	if strings.TrimSpace(box.ID) == "" {
		return BoxData{}, fmt.Errorf("upstash box %s info was incomplete", id)
	}
	return box, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	key, err := c.ResolveKey()
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
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL()+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("X-Box-Api-Key", key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("upstash box: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("upstash box: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.apiError(resp.StatusCode, data, key)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("upstash box: decode response: %w", err)
	}
	return nil
}

func (c *Client) apiError(status int, body []byte, key string) error {
	msg := strings.TrimSpace(string(body))
	var parsed struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if parsed.Error != "" {
			msg = parsed.Error
		} else if parsed.Message != "" {
			msg = parsed.Message
		}
	}
	msg = strings.ReplaceAll(msg, key, "[redacted]")
	if msg == "" {
		msg = http.StatusText(status)
	}
	if status == http.StatusUnauthorized {
		return fmt.Errorf("upstash box API 401: invalid or missing API key")
	}
	return fmt.Errorf("upstash box API %d: %s", status, msg)
}
