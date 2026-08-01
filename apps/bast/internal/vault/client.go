package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultAPIBase = "https://bast.sh"
	MaxVaultBytes  = 1 << 20 // 1 MiB
)

// ErrRemoteUpdated means the vault changed since the caller's If-Match revision.
var ErrRemoteUpdated = errors.New("vault was updated elsewhere; pull and merge before pushing")

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

func (c *Client) base() string {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultAPIBase
	}
	return base
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

type OTPStartResponse struct {
	OK bool `json:"ok"`
}

type OTPVerifyResponse struct {
	OK       bool   `json:"ok"`
	Token    string `json:"token"`
	UserID   string `json:"userId"`
	Email    string `json:"email"`
	DeviceID string `json:"deviceId,omitempty"`
}

type VaultMeta struct {
	Revision    string `json:"revision"`
	UpdatedAt   int64  `json:"updatedAt"`
	Size        int64  `json:"size"`
	ContentHash string `json:"contentHash,omitempty"`
}

type VaultGetResult struct {
	NotModified bool
	Meta        VaultMeta
	Ciphertext  []byte
}

func (c *Client) StartOTP(ctx context.Context, email string) error {
	body, _ := json.Marshal(map[string]string{"email": email})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/api/auth/otp/start", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return apiError(resp)
	}
	return nil
}

func (c *Client) VerifyOTP(ctx context.Context, email, code string) (OTPVerifyResponse, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	code = NormalizeOTP(code)
	if code == "" {
		return OTPVerifyResponse{}, errors.New("code must be 6 digits")
	}
	body, _ := json.Marshal(map[string]string{"email": email, "code": code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/api/auth/otp/verify", bytes.NewReader(body))
	if err != nil {
		return OTPVerifyResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return OTPVerifyResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return OTPVerifyResponse{}, apiError(resp)
	}
	var out OTPVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return OTPVerifyResponse{}, err
	}
	if out.Token == "" {
		return OTPVerifyResponse{}, errors.New("server did not return a vault token")
	}
	return out, nil
}

// NormalizeOTP keeps digits only. Returns empty unless exactly 6 digits.
func NormalizeOTP(code string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(code) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if len(digits) != 6 {
		return ""
	}
	return digits
}

func (c *Client) GetVault(ctx context.Context, ifNoneMatch string) (VaultGetResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/api/vault", nil)
	if err != nil {
		return VaultGetResult{}, err
	}
	c.authorize(req)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", quoteETag(ifNoneMatch))
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return VaultGetResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return VaultGetResult{NotModified: true, Meta: VaultMeta{Revision: ifNoneMatch}}, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return VaultGetResult{}, nil
	}
	if resp.StatusCode >= 300 {
		return VaultGetResult{}, apiError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxVaultBytes+1))
	if err != nil {
		return VaultGetResult{}, err
	}
	if len(data) > MaxVaultBytes {
		return VaultGetResult{}, errors.New("vault payload exceeds 1 MiB limit")
	}
	revision := stripETag(resp.Header.Get("ETag"))
	return VaultGetResult{
		Meta: VaultMeta{
			Revision:  revision,
			UpdatedAt: parseInt64(resp.Header.Get("X-Vault-Updated-At")),
			Size:      int64(len(data)),
		},
		Ciphertext: data,
	}, nil
}

func (c *Client) PutVault(ctx context.Context, ciphertext []byte, ifMatch string) (VaultMeta, error) {
	if len(ciphertext) > MaxVaultBytes {
		return VaultMeta{}, errors.New("vault payload exceeds 1 MiB limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.base()+"/api/vault", bytes.NewReader(ciphertext))
	if err != nil {
		return VaultMeta{}, err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		req.Header.Set("If-Match", quoteETag(ifMatch))
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return VaultMeta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return VaultMeta{}, ErrRemoteUpdated
	}
	if resp.StatusCode >= 300 {
		return VaultMeta{}, apiError(resp)
	}
	var meta VaultMeta
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	if meta.Revision == "" {
		meta.Revision = stripETag(resp.Header.Get("ETag"))
	}
	return meta, nil
}

func (c *Client) Logout(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/api/auth/logout", nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusUnauthorized {
		return apiError(resp)
	}
	return nil
}

func (c *Client) authorize(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

func quoteETag(rev string) string {
	rev = strings.TrimSpace(rev)
	if rev == "" {
		return ""
	}
	if strings.HasPrefix(rev, `"`) {
		return rev
	}
	return `"` + rev + `"`
}

func stripETag(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToUpper(value), "W/") {
		value = strings.TrimSpace(value[2:])
	}
	return strings.ReplaceAll(value, `"`, "")
}

func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return fmt.Errorf("vault api: %s", parsed.Error)
	}
	return fmt.Errorf("vault api: HTTP %d", resp.StatusCode)
}

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}
