// Package client provides an HTTP client for consuming the prompts REST API.
//
// The remote server exposes the following endpoints:
//
//	POST   /create           create a new prompt
//	GET    /get/{index}      retrieve a prompt by index
//	PUT    /update/{index}   update an existing prompt
//	DELETE /delete/{index}   delete a prompt by index
//
// MIGRATION_NOTE: The original Python file (client.py) is a standalone script
// that consumes a REST API, not a server. It has been migrated into a reusable
// Client type plus a RunDemo function that mirrors the original main().
//
// MIGRATION_NOTE: The API uses integer indices as identifiers rather than
// stable IDs. This indexing scheme is preserved here. Review whether the
// server migration should adopt stable IDs instead.
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

// defaultBaseURL is used when the PROMPTS_API_BASE_URL environment variable
// is not set.
//
// MIGRATION_NOTE: BASE_URL was hardcoded in the Python source. It is now
// externalized to the PROMPTS_API_BASE_URL environment variable, falling back
// to the original default for local development.
const defaultBaseURL = "http://127.0.0.1:5000"

// Response represents a decoded JSON response from the prompts API.
//
// The API returns arbitrary JSON objects, so a generic map is used. On a
// non-JSON response, an {"error": "Invalid response from the server"} value
// is returned to mirror the original graceful degradation behaviour.
type Response map[string]any

// invalidResponse mirrors the Python client's fallback for non-JSON bodies.
func invalidResponse() Response {
	return Response{"error": "Invalid response from the server"}
}

// Client is an HTTP client for the prompts REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the base URL used for all requests.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient overrides the underlying *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// NewClient constructs a Client. The base URL is resolved from the
// PROMPTS_API_BASE_URL environment variable, defaulting to defaultBaseURL.
// Options may override the base URL or HTTP client.
func NewClient(opts ...Option) *Client {
	baseURL := os.Getenv("PROMPTS_API_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// response into a Response. A non-JSON response body yields invalidResponse()
// rather than an error, matching the original client's behaviour.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) (Response, error) {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
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
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		// Mirrors the Python client's JSONDecodeError handling: a non-JSON
		// body is not treated as a hard failure.
		return invalidResponse(), nil
	}

	return out, nil
}

// CreatePrompt creates a new prompt via POST /create.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", map[string]string{"prompt": prompt})
}

// GetResponse retrieves a prompt by index via GET /get/{index}.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt updates an existing prompt via PUT /update/{index}.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex),
		map[string]string{"new_prompt": newPrompt})
}

// DeletePrompt deletes a prompt by index via DELETE /delete/{index}.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// RunDemo exercises the client against the configured API, mirroring the
// original Python main() function. It prints each response to stdout.
func RunDemo(ctx context.Context, c *Client) error {
	// Test CreatePrompt.
	const (
		prompt1 = "What is life?"
		prompt2 = "What is the capital of Pakistan?"
	)

	createResp1, err := c.CreatePrompt(ctx, prompt1)
	if err != nil {
		return fmt.Errorf("create prompt 1: %w", err)
	}
	createResp2, err := c.CreatePrompt(ctx, prompt2)
	if err != nil {
		return fmt.Errorf("create prompt 2: %w", err)
	}
	fmt.Println(createResp1)
	fmt.Println(createResp2)

	// Test GetResponse.
	resp, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0: %w", err)
	}
	fmt.Println(resp)

	// Test UpdatePrompt.
	updateResp, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return fmt.Errorf("update prompt 1: %w", err)
	}
	fmt.Println(updateResp)

	// Test GetResponse after update.
	respAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get prompt 1 after update: %w", err)
	}
	fmt.Println(respAfterUpdate)

	// Test DeletePrompt.
	deleteResp, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete prompt 0: %w", err)
	}
	fmt.Println(deleteResp)

	// Test GetResponse after delete.
	respAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0 after delete: %w", err)
	}
	fmt.Println(respAfterDelete)

	return nil
}
