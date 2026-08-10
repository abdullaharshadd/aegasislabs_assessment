// Package client provides an HTTP client for consuming the prompts REST API.
//
// This file is the idiomatic Go migration of the original client.py script.
// It is an API *client*, not a server: it consumes the endpoints
//
//	POST   /create
//	GET    /get/{index}
//	PUT    /update/{index}
//	DELETE /delete/{index}
//
// which are defined by a separate server (originally a Flask app on port 5000).
//
// MIGRATION_NOTE: The instruction to "register ALL HTTP routes" does not apply
// cleanly here — this source file never defines or serves routes; it only calls
// them. The route paths are preserved verbatim as the client's request paths
// (see the Client methods below). The actual server-side route registration
// belongs in the migration of the separate server file.
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

// defaultBaseURL is the fallback API base URL when PROMPTS_API_BASE_URL is unset.
//
// MIGRATION_NOTE: The original BASE_URL was hardcoded. It is now sourced from
// the PROMPTS_API_BASE_URL environment variable, falling back to this default.
const defaultBaseURL = "http://127.0.0.1:5000"

// errInvalidResponse mirrors the Python client's {"error": "Invalid response ..."}
// behavior for responses whose bodies are not valid JSON.
const invalidResponseMessage = "Invalid response from the server"

// HTTPDoer is the minimal interface the Client needs from an HTTP client.
// Using an interface (rather than *http.Client) makes the Client testable
// with a stubbed transport.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Response represents a decoded JSON response body from the API.
//
// The upstream API returns arbitrary JSON objects, so this is a generic map.
// For responses that cannot be decoded as JSON, methods return a Response
// containing an "error" key, matching the original Python behavior.
type Response map[string]any

// Client is a client for the prompts REST API.
type Client struct {
	baseURL    string
	httpClient HTTPDoer
}

// Option configures a Client via the functional-options pattern.
type Option func(*Client)

// WithBaseURL overrides the API base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(doer HTTPDoer) Option {
	return func(c *Client) {
		c.httpClient = doer
	}
}

// NewClient constructs a Client. The base URL is taken from the
// PROMPTS_API_BASE_URL environment variable, then any supplied options.
func NewClient(opts ...Option) *Client {
	baseURL := os.Getenv("PROMPTS_API_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CreatePrompt sends POST /create with the given prompt and returns the decoded response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	body := map[string]string{"prompt": prompt}
	return c.doJSON(ctx, http.MethodPost, "/create", body, true)
}

// GetResponse sends GET /get/{index} and returns the decoded response.
//
// Matching the original client, a non-JSON body yields a Response containing
// an "error" key rather than a hard error.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	path := fmt.Sprintf("/get/%d", promptIndex)
	return c.doJSON(ctx, http.MethodGet, path, nil, false)
}

// UpdatePrompt sends PUT /update/{index} with the new prompt and returns the decoded response.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	path := fmt.Sprintf("/update/%d", promptIndex)
	body := map[string]string{"new_prompt": newPrompt}
	return c.doJSON(ctx, http.MethodPut, path, body, true)
}

// DeletePrompt sends DELETE /delete/{index} and returns the decoded response.
//
// Matching the original client, a non-JSON body yields a Response containing
// an "error" key rather than a hard error.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	path := fmt.Sprintf("/delete/%d", promptIndex)
	return c.doJSON(ctx, http.MethodDelete, path, nil, false)
}

// doJSON performs an HTTP request and decodes the JSON response.
//
// When strictJSON is true (POST/PUT in the original), a JSON decode failure is
// returned as an error. When false (GET/DELETE in the original), a decode
// failure returns a Response with an "error" key, replicating the Python
// JSONDecodeError handling.
func (c *Client) doJSON(ctx context.Context, method, path string, body any, strictJSON bool) (Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body for %s %s: %w", method, path, err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("building request for %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body for %s %s: %w", method, path, err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		if strictJSON {
			return nil, fmt.Errorf("decoding JSON response for %s %s: %w", method, path, err)
		}
		return Response{"error": invalidResponseMessage}, nil
	}

	return decoded, nil
}
