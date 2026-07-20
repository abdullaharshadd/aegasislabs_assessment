// Package client provides an HTTP client for consuming the prompt CRUD REST API.
//
// MIGRATION_NOTE: The original client.py was a standalone HTTP client script that
// consumes an external REST API (POST /create, GET /get/{index}, PUT /update/{index},
// DELETE /delete/{index}). It is NOT a server and defines no routes of its own. The
// "register all HTTP routes" requirement does not apply here because these paths are
// consumed, not served — they live on a separate Flask server (default port 5000).
// Each endpoint is instead modeled as a client method below.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// defaultBaseURL is used when the PROMPT_API_BASE_URL environment variable is unset.
//
// MIGRATION_NOTE: The original hardcoded BASE_URL (http://127.0.0.1:5000) is now
// externalized to an environment variable, falling back to this default.
const defaultBaseURL = "http://127.0.0.1:5000"

// defaultTimeout bounds each HTTP request. The original Python code had no timeout.
const defaultTimeout = 10 * time.Second

// Doer abstracts the HTTP client so callers may inject custom transports or mocks.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is a client for the prompt CRUD REST API.
type Client struct {
	baseURL string
	http    Doer
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client (useful for tests).
func WithHTTPClient(d Doer) Option {
	return func(c *Client) {
		c.http = d
	}
}

// WithBaseURL overrides the base URL of the target API server.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// NewClient constructs a Client. The base URL is read from the
// PROMPT_API_BASE_URL environment variable, defaulting to defaultBaseURL.
func NewClient(opts ...Option) *Client {
	baseURL := os.Getenv("PROMPT_API_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	c := &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: defaultTimeout},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// APIResponse is the decoded JSON body of an API response.
//
// MIGRATION_NOTE: The Python code returned untyped dicts (response.json()).
// Since the server's response schema is not defined here, we decode into a
// generic map. Replace with a concrete struct once the API contract is known.
type APIResponse map[string]any

// CreatePrompt creates a new prompt via POST /create.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (APIResponse, error) {
	body := map[string]string{"prompt": prompt}
	return c.doJSON(ctx, http.MethodPost, "/create", body)
}

// GetResponse fetches a prompt by index via GET /get/{index}.
//
// MIGRATION_NOTE: prompt_index is a positional list index on the server side,
// not a stable resource ID. Preserved as-is; consider migrating to real IDs.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (APIResponse, error) {
	path := fmt.Sprintf("/get/%d", promptIndex)
	return c.doJSON(ctx, http.MethodGet, path, nil)
}

// UpdatePrompt updates a prompt by index via PUT /update/{index}.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (APIResponse, error) {
	path := fmt.Sprintf("/update/%d", promptIndex)
	body := map[string]string{"new_prompt": newPrompt}
	return c.doJSON(ctx, http.MethodPut, path, body)
}

// DeletePrompt deletes a prompt by index via DELETE /delete/{index}.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (APIResponse, error) {
	path := fmt.Sprintf("/delete/%d", promptIndex)
	return c.doJSON(ctx, http.MethodDelete, path, nil)
}

// doJSON performs an HTTP request, optionally encoding body as JSON, and decodes
// the JSON response into an APIResponse.
//
// MIGRATION_NOTE: The Python code caught requests.exceptions.JSONDecodeError and
// returned {"error": "Invalid response from the server"}. Here, a decode failure
// returns that same sentinel payload alongside the decode error so callers can
// inspect both.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) (APIResponse, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out APIResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		// Mirror the Python fallback for invalid JSON responses.
		return APIResponse{"error": "Invalid response from the server"},
			fmt.Errorf("decoding response from %s %s: %w", method, path, err)
	}

	return out, nil
}
