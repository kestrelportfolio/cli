package api

import (
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
