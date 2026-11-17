package vault

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const webDAVFilename = "bast-vault.json"

var errWebDAVWeakETag = errors.New("WebDAV must provide a strong ETag for safe synchronization")

// WebDAVConfig is a local secret, stored with the session rather than in Document.
// URL names an existing collection; Bast does not create or list directories.
type WebDAVConfig struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (cfg WebDAVConfig) Validate() error {
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("WebDAV URL must be an absolute https:// directory URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("WebDAV URL must not contain credentials, query parameters, or a fragment")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("WebDAV requires HTTPS (HTTP is only allowed on loopback for local testing)")
		}
	}
	if strings.TrimSpace(cfg.Username) == "" || strings.Contains(cfg.Username, ":") || cfg.Password == "" {
		return errors.New("WebDAV username and password are required; username must not contain a colon")
	}
	return nil
}

func (c *Client) webDAVRequest(ctx context.Context, method, filename string, body []byte, headers http.Header) (*http.Response, error) {
	if c.WebDAV == nil {
		return nil, errors.New("WebDAV is not configured")
	}
	if err := c.WebDAV.Validate(); err != nil {
		return nil, err
	}
	u, _ := url.Parse(strings.TrimRight(c.WebDAV.URL, "/") + "/")
	target := u.ResolveReference(&url.URL{Path: filename})
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.SetBasicAuth(c.WebDAV.Username, c.WebDAV.Password)
	req.Header.Set("Content-Type", "application/json")
	// Conditional writes must refer to the stored representation. Compression
	// middleware can weaken or rewrite ETags, so request the original bytes.
	req.Header.Set("Accept-Encoding", "identity")
	// Never send credentials or replay a conditional upload at a redirected URL.
	client := *c.http()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && resp.StatusCode != http.StatusNotModified {
		resp.Body.Close()
		return nil, errors.New("WebDAV redirected the request; configure the final directory URL")
	}
	return resp, nil
}

func strongWebDAVETag(value string) bool {
	// Preserve the opaque, quoted value, including backslashes. Weak validators
	// cannot protect If-Match writes and must never be turned into strong ones.
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	for _, b := range []byte(value[1 : len(value)-1]) {
		if b < 0x21 || b == '"' || b == 0x7f {
			return false
		}
	}
	return true
}

func webDAVError(resp *http.Response) error {
	// Do not reflect remote response bodies, which may contain credentials or HTML.
	return fmt.Errorf("WebDAV: HTTP %d", resp.StatusCode)
}

func (c *Client) webDAVGet(ctx context.Context, revision string) (VaultGetResult, error) {
	return c.webDAVGetFile(ctx, webDAVFilename, revision)
}

func (c *Client) webDAVGetFile(ctx context.Context, filename, revision string) (VaultGetResult, error) {
	// Apache may temporarily mark a just-written file's ETag weak until its
	// modification time precedes the current second. Wait for a strong validator
	// without stripping W/ or weakening the write precondition.
	for attempt := 0; ; attempt++ {
		result, err := c.webDAVReadFile(ctx, filename, revision)
		if !errors.Is(err, errWebDAVWeakETag) || attempt == 2 {
			return result, err
		}
		timer := time.NewTimer(1100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return VaultGetResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) webDAVReadFile(ctx context.Context, filename, revision string) (VaultGetResult, error) {
	headers := make(http.Header)
	if revision != "" {
		if !strongWebDAVETag(revision) {
			return VaultGetResult{}, errors.New("WebDAV revision must be a strong ETag")
		}
		headers.Set("If-None-Match", revision)
	}
	resp, err := c.webDAVRequest(ctx, http.MethodGet, filename, nil, headers)
	if err != nil {
		return VaultGetResult{}, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return VaultGetResult{}, nil
	case http.StatusNotModified:
		if revision == "" {
			return VaultGetResult{}, errors.New("WebDAV returned 304 without a conditional request")
		}
		return VaultGetResult{NotModified: true, Meta: VaultMeta{Revision: revision}}, nil
	case http.StatusOK:
	default:
		return VaultGetResult{}, webDAVError(resp)
	}
	etag := resp.Header.Get("ETag")
	if !strongWebDAVETag(etag) {
		if strings.HasPrefix(etag, "W/") {
			return VaultGetResult{}, errWebDAVWeakETag
		}
		return VaultGetResult{}, errors.New("WebDAV must provide a strong ETag for safe synchronization")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxVaultBytes+1))
	if err != nil {
		return VaultGetResult{}, err
	}
	if len(data) == 0 || len(data) > MaxVaultBytes {
		return VaultGetResult{}, errors.New("WebDAV vault must be non-empty and at most 1 MiB")
	}
	var updated int64
	if modified, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		updated = modified.Unix()
	}
	return VaultGetResult{Ciphertext: data, Meta: VaultMeta{
		Revision: etag, Size: int64(len(data)), UpdatedAt: updated,
	}}, nil
}

func (c *Client) webDAVPut(ctx context.Context, data []byte, revision string) (VaultMeta, error) {
	return c.webDAVPutFile(ctx, webDAVFilename, data, revision)
}

func (c *Client) webDAVPutFile(ctx context.Context, filename string, data []byte, revision string) (VaultMeta, error) {
	if len(data) == 0 || len(data) > MaxVaultBytes {
		return VaultMeta{}, errors.New("WebDAV vault must be non-empty and at most 1 MiB")
	}
	headers := make(http.Header)
	if revision == "" {
		headers.Set("If-None-Match", "*")
	} else {
		if !strongWebDAVETag(revision) {
			return VaultMeta{}, errors.New("WebDAV revision must be a strong ETag")
		}
		headers.Set("If-Match", revision)
	}
	resp, err := c.webDAVRequest(ctx, http.MethodPut, filename, data, headers)
	if err != nil {
		return VaultMeta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return VaultMeta{}, ErrRemoteUpdated
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return VaultMeta{}, webDAVError(resp)
	}
	if etag := resp.Header.Get("ETag"); strongWebDAVETag(etag) {
		return VaultMeta{Revision: etag, Size: int64(len(data)), UpdatedAt: time.Now().Unix()}, nil
	}
	// Some DAV servers omit ETag on PUT. Bind the following GET's revision to
	// our exact bytes, rather than accidentally accepting another writer's ETag.
	remote, err := c.webDAVGetFile(ctx, filename, "")
	if err != nil {
		return VaultMeta{}, err
	}
	if !bytes.Equal(remote.Ciphertext, data) {
		return VaultMeta{}, ErrRemoteUpdated
	}
	return remote.Meta, nil
}

// CheckWebDAV checks conditional writes against an isolated random resource.
// A server which ignores If-Match must not be used for a shared encrypted vault.
// The existing vault is never modified by this check.
func (c *Client) CheckWebDAV(ctx context.Context) (checkErr error) {
	if c.WebDAV == nil {
		return errors.New("WebDAV is not configured")
	}
	if err := c.WebDAV.Validate(); err != nil {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	filename := ".bast-probe-" + hex.EncodeToString(random[:])
	body := []byte(`{"probe":1}`)
	defer func() {
		// Cleanup gets its own short deadline even when the caller timed out.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		resp, err := c.webDAVRequest(cleanup, http.MethodDelete, filename, nil, nil)
		if err != nil {
			if checkErr == nil {
				checkErr = fmt.Errorf("WebDAV probe cleanup: %w", err)
			}
			return
		}
		defer resp.Body.Close()
		if checkErr == nil && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			checkErr = fmt.Errorf("WebDAV probe cleanup: HTTP %d (delete permission is required)", resp.StatusCode)
		}
	}()
	meta, err := c.webDAVPutFile(ctx, filename, body, "")
	if err != nil {
		return fmt.Errorf("WebDAV capability check: %w", err)
	}
	if _, err := c.webDAVPutFile(ctx, filename, body, ""); !errors.Is(err, ErrRemoteUpdated) {
		return errors.New("WebDAV must reject If-None-Match creation of an existing file with HTTP 412")
	}
	if _, err := c.webDAVPutFile(ctx, filename, body, `"bast-stale-`+hex.EncodeToString(random[:])+`"`); !errors.Is(err, ErrRemoteUpdated) {
		return errors.New("WebDAV must reject stale If-Match writes with HTTP 412")
	}
	// Also verify that valid updates work, rather than a server rejecting all PUTs.
	if _, err := c.webDAVPutFile(ctx, filename, []byte(`{"probe":2}`), meta.Revision); err != nil {
		return fmt.Errorf("WebDAV conditional update check: %w", err)
	}
	return nil
}
