// Package client provides an HTTP client for the prompt CRUD REST API.
//
// MIGRATION_NOTE: The original client.py was a standalone Python script that
// exercised a CRUD REST API using the requests library. It is NOT a Django
// server file. This migration models it as an idiomatic Go HTTP client package
// with a constructor (NewClient), context propagation, explicit error handling,
// and configurable timeouts/base URL.
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

// defaultBaseURL is used when no base URL is supplied.
//
// MIGRATION_NOTE: The original hardcoded BASE_URL has been externalized.
// Prefer setting the PROMPT_API_BASE_URL environment variable or passing an
// explicit base URL to NewClient.
const defaultBaseURL = "http://127.0.0.1:5000"

// defaultTimeout bounds every outgoing request. The original Python client set
// no timeout; adding one is recommended for production use.
const defaultTimeout = 30 * time.Second

// Response represents a decoded JSON response from the API server. Because the
// server's response shape is not strongly typed in the original script, this is
// modeled as a generic map. The Error field is populated when the server
// returns a non-JSON body (mirroring the Python JSONDecodeError handling).
type Response map[string]interface{}

// invalidResponse mirrors the Python {"error": "Invalid response from the server"}
// fallback returned when the response body is not valid JSON.
func invalidResponse() Response {
	return Response{"error": "Invalid response from the server"}
}

// Client is an HTTP client for the prompt CRUD REST API.
type Client struct {
	baseURL string
	http    *http.Client
}

// Option configures a Client. It follows the functional options pattern.
type Option func(*Client)

// WithBaseURL overrides the base URL used for all requests.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = baseURL
		}
	}
}

// WithHTTPClient overrides the underlying *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// NewClient constructs a Client. The base URL is resolved from (in order):
// any WithBaseURL option, the PROMPT_API_BASE_URL environment variable, then
// the default base URL.
func NewClient(opts ...Option) *Client {
	baseURL := defaultBaseURL
	if env := os.Getenv("PROMPT_API_BASE_URL"); env != "" {
		baseURL = env
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

// CreatePrompt creates a new prompt and returns the decoded server response.
//
// MIGRATION_NOTE: The original Python code did not guard against JSON decode
// errors on create/update, so a decode failure here is surfaced as an error
// rather than swallowed.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	body := map[string]string{"prompt": prompt}
	return c.doJSON(ctx, http.MethodPost, "/create", body, false)
}

// GetResponse retrieves the prompt at the given index. If the server returns a
// non-JSON body, it returns the invalid-response fallback rather than an error,
// preserving the original Python behavior.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	endpoint := fmt.Sprintf("/get/%d", promptIndex)
	return c.doJSON(ctx, http.MethodGet, endpoint, nil, true)
}

// UpdatePrompt updates the prompt at the given index with a new value.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	endpoint := fmt.Sprintf("/update/%d", promptIndex)
	body := map[string]string{"new_prompt": newPrompt}
	return c.doJSON(ctx, http.MethodPut, endpoint, body, false)
}

// DeletePrompt deletes the prompt at the given index. If the server returns a
// non-JSON body, it returns the invalid-response fallback rather than an error,
// preserving the original Python behavior.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	endpoint := fmt.Sprintf("/delete/%d", promptIndex)
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil, true)
}

// doJSON performs an HTTP request and decodes the JSON response.
//
// When tolerateNonJSON is true, a JSON decode failure returns the
// invalid-response fallback (matching the Python JSONDecodeError handling on
// GET and DELETE). When false, a decode failure is returned as an error.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body interface{}, tolerateNonJSON bool) (Response, error) {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		if tolerateNonJSON {
			return invalidResponse(), nil
		}
		return nil, fmt.Errorf("decoding JSON response (status %d): %w", resp.StatusCode, err)
	}

	return out, nil
}
