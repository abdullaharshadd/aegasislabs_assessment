// Package client provides an HTTP client for consuming the prompts REST API.
//
// This is an API *consumer*, not a provider. The routes referenced
// (/create, /get/{index}, /update/{index}, /delete/{index}) are implemented
// by an external server; this client only calls them.
//
// MIGRATION_NOTE: The original Python file used a module-level BASE_URL constant
// hardcoded to http://127.0.0.1:5000. In Go we externalize this to an
// environment variable (PROMPTS_API_BASE_URL) with the same default, and inject
// it through the Client constructor so the client is testable and configurable.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// DefaultBaseURL is used when no base URL is configured explicitly.
//
// MIGRATION_NOTE: preserved from the original hardcoded Python value.
const DefaultBaseURL = "http://127.0.0.1:5000"

// baseURLEnvVar is the environment variable used to override the API base URL.
const baseURLEnvVar = "PROMPTS_API_BASE_URL"

// Response models a decoded JSON payload returned by the prompts API.
//
// The server responses are not strictly typed in the original client, which
// simply returned the decoded JSON map. We preserve that flexibility with a
// generic map while still exposing a helper for the well-known "error" field
// the original code produced on JSON decode failures.
type Response map[string]any

// Doer abstracts the subset of *http.Client used by Client, allowing callers to
// inject a custom or mock HTTP transport in tests.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is a REST client for the prompts API.
type Client struct {
	baseURL string
	http    Doer
}

// Option configures a Client using the functional options pattern.
type Option func(*Client)

// WithBaseURL overrides the API base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = baseURL
		}
	}
}

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(d Doer) Option {
	return func(c *Client) {
		if d != nil {
			c.http = d
		}
	}
}

// NewClient constructs a Client. The base URL defaults to the value of the
// PROMPTS_API_BASE_URL environment variable, falling back to DefaultBaseURL.
// Options may override the base URL and HTTP client.
func NewClient(opts ...Option) *Client {
	baseURL := os.Getenv(baseURLEnvVar)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	c := &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// invalidResponse mirrors the Python client's behaviour of returning
// {"error": "Invalid response from the server"} when the body cannot be parsed
// as JSON.
func invalidResponse() Response {
	return Response{"error": "Invalid response from the server"}
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// response as JSON.
//
// The tolerateDecodeError flag controls whether a JSON decode failure returns
// the sentinel invalidResponse (matching the Python get/delete behaviour) or
// is surfaced as an error (matching the create/update behaviour, which called
// response.json() without a try/except).
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any, tolerateDecodeError bool) (Response, error) {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	fullURL := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, fullURL, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request %s %s: %w", method, fullURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var decoded Response
	if err := json.Unmarshal(raw, &decoded); err != nil {
		if tolerateDecodeError {
			return invalidResponse(), nil
		}
		return nil, fmt.Errorf("decode response from %s %s: %w", method, fullURL, err)
	}

	return decoded, nil
}

// CreatePrompt registers a new prompt with the server.
//
// Route: POST /create
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	body := map[string]string{"prompt": prompt}
	return c.doJSON(ctx, http.MethodPost, "/create", body, false)
}

// GetResponse fetches the prompt (and its response) at the given positional
// index.
//
// MIGRATION_NOTE: promptIndex is a positional array index used by the existing
// server, not a database ID. This is preserved for compatibility.
//
// Route: GET /get/{index}
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	endpoint := fmt.Sprintf("/get/%s", url.PathEscape(fmt.Sprintf("%d", promptIndex)))
	return c.doJSON(ctx, http.MethodGet, endpoint, nil, true)
}

// UpdatePrompt replaces the prompt at the given positional index.
//
// Route: PUT /update/{index}
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	endpoint := fmt.Sprintf("/update/%s", url.PathEscape(fmt.Sprintf("%d", promptIndex)))
	body := map[string]string{"new_prompt": newPrompt}
	return c.doJSON(ctx, http.MethodPut, endpoint, body, false)
}

// DeletePrompt removes the prompt at the given positional index.
//
// Route: DELETE /delete/{index}
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	endpoint := fmt.Sprintf("/delete/%s", url.PathEscape(fmt.Sprintf("%d", promptIndex)))
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil, true)
}
