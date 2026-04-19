package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kestrelportfolio/kestrel-cli/internal/config"
)

// Version is set at build time via -ldflags. For now it's a default.
var Version = "dev"

// Client is the HTTP client for the Kestrel Portfolio API.
// Think of it like a configured Faraday connection in Ruby —
// it knows the base URL and auth token and handles the HTTP plumbing.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates an API client from the loaded config.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		BaseURL: cfg.BaseURL,
		Token:   cfg.Token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Get performs a GET request to the given path (relative to BaseURL).
// params is a map of query parameters (like {page: "2"}).
// Returns the parsed Envelope or an error.
func (c *Client) Get(path string, params map[string]string) (*Envelope, error) {
	reqURL, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return nil, fmt.Errorf("building URL for %s: %w", path, err)
	}

	// Add query parameters
	if len(params) > 0 {
		u, err := url.Parse(reqURL)
		if err != nil {
			return nil, fmt.Errorf("parsing URL: %w", err)
		}
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		reqURL = u.String()
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "kestrel-cli/"+Version)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var envelope Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parsing response JSON: %w", err)
	}

	if !envelope.OK {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    envelope.Error,
			Code:       envelope.Code,
		}
	}

	return &envelope, nil
}

// Post sends a POST with a JSON-encoded body and returns the parsed envelope.
// Use a nil body for endpoints that expect an empty POST (e.g. /abandon).
func (c *Client) Post(path string, body any) (*Envelope, error) {
	return c.doJSON("POST", path, body)
}

// Patch sends a PATCH with a JSON-encoded body.
func (c *Client) Patch(path string, body any) (*Envelope, error) {
	return c.doJSON("PATCH", path, body)
}

// Delete sends a DELETE. Returns a nil envelope on 204 No Content.
// Some delete endpoints return 200 with a data payload (e.g. cascade counts
// from DELETE /documents/:id) — the envelope is populated in that case.
func (c *Client) Delete(path string) (*Envelope, error) {
	return c.doJSON("DELETE", path, nil)
}

// Upload sends a POST with a raw body (not JSON-encoded). Used for the
// Basecamp-style file upload at POST /documents?name=<filename>. The caller
// is responsible for query parameters and Content-Type.
//
// In Ruby/Net::HTTP terms this is like `req.body = file.read` with
// `req['Content-Type'] = "application/pdf"`.
func (c *Client) Upload(path string, body io.Reader, contentType string, contentLength int64) (*Envelope, error) {
	reqURL, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return nil, fmt.Errorf("building URL for %s: %w", path, err)
	}

	req, err := http.NewRequest("POST", reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "kestrel-cli/"+Version)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = contentLength

	return c.sendAndDecode(req)
}

// doJSON is the shared path for POST/PATCH/DELETE with optional JSON body.
func (c *Client) doJSON(method, path string, body any) (*Envelope, error) {
	reqURL, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return nil, fmt.Errorf("building URL for %s: %w", path, err)
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, reqURL, reader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "kestrel-cli/"+Version)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.sendAndDecode(req)
}

// sendAndDecode executes a request, decodes the standard or validation envelope,
// and returns an APIError on any non-success.
func (c *Client) sendAndDecode(req *http.Request) (*Envelope, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request to %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	// 204 No Content — success with no body. Nothing to decode.
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var envelope Envelope
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("parsing response JSON (status %d): %w", resp.StatusCode, err)
		}
	}

	if !envelope.OK {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    envelope.Error,
			Code:       envelope.Code,
			Errors:     envelope.Errors,
		}
	}

	return &envelope, nil
}

// GetRedirect performs a GET that does NOT follow redirects.
// For download endpoints that respond with a 302 to a short-lived signed URL,
// it returns the Location header value. On any non-redirect response, it tries
// to decode the standard error envelope and returns an APIError.
//
// In Ruby/Net::HTTP terms, this is like calling get with follow_redirects: false
// and reading response['Location'] yourself.
func (c *Client) GetRedirect(path string) (string, error) {
	reqURL, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return "", fmt.Errorf("building URL for %s: %w", path, err)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "kestrel-cli/"+Version)
	req.Header.Set("Accept", "application/json")

	// One-off client that short-circuits any redirect — we want the 302, not its target.
	noRedirect := &http.Client{
		Timeout: c.HTTPClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("making request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return "", fmt.Errorf("got %d but no Location header", resp.StatusCode)
		}
		return loc, nil
	}

	// Any other status — try to decode the standard error envelope.
	body, _ := io.ReadAll(resp.Body)
	var check struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(body, &check); err == nil && !check.OK {
		return "", &APIError{
			StatusCode: resp.StatusCode,
			Message:    check.Error,
			Code:       check.Code,
		}
	}
	return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
}

// GetRaw performs a GET request and returns the raw response bytes.
// Useful when you want to pass the entire JSON response through to --json output.
func (c *Client) GetRaw(path string, params map[string]string) ([]byte, error) {
	reqURL, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return nil, fmt.Errorf("building URL for %s: %w", path, err)
	}

	if len(params) > 0 {
		u, err := url.Parse(reqURL)
		if err != nil {
			return nil, fmt.Errorf("parsing URL: %w", err)
		}
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		reqURL = u.String()
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", "kestrel-cli/"+Version)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// Check for API errors even in raw mode
	var check struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(body, &check); err == nil && !check.OK {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    check.Error,
			Code:       check.Code,
		}
	}

	return body, nil
}
