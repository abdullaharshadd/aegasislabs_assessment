// Package client provides an HTTP client for consuming the prompt CRUD REST API.
//
// MIGRATION_NOTE: The source (client.py) is a standalone HTTP *client* script, not a
// server. There are no server-side routes to register here; this file consumes an
// external API exposing the following endpoints:
//
//	POST   /create
//	GET    /get/{index}
//	PUT    /update/{index}
//	DELETE /delete/{index}
//
// MIGRATION_NOTE: BASE_URL was hardcoded to http://127.0.0.1:5000 in Python. It is
// externalized here to the PROMPT_API_BASE_URL environment variable with the same
// default, per the migration guidance.
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

// defaultBaseURL mirrors the hardcoded Flask default from the Python source.
const defaultBaseURL = "http://127.0.0.1:5000"

// baseURLEnvVar is the environment variable used to override the API base URL.
const baseURLEnvVar = "PROMPT_API_BASE_URL"

// APIResponse represents a decoded JSON response from the prompt API.
//
// MIGRATION_NOTE: The Python code returned dynamic dicts (response.json()). Since
// the server contract is not defined in this file, responses are modeled as a
// generic map to preserve arbitrary JSON shapes. Callers that know the concrete
// schema should decode into a typed struct instead.
type APIResponse map[string]interface{}

// Client is an HTTP client for the prompt CRUD API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client. It follows the functional options pattern.
type Option func(*Client)

// WithBaseURL overrides the API base URL.
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
// PROMPT_API_BASE_URL environment variable, falling back to the Flask default,
// unless overridden via WithBaseURL.
func NewClient(opts ...Option) *Client {
	baseURL := os.Getenv(baseURLEnvVar)
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

// doJSON performs an HTTP request with an optional JSON body and decodes the
// JSON response body into an APIResponse.
//
// MIGRATION_NOTE: The Python get/delete helpers swallowed JSONDecodeError and
// returned {"error": "Invalid response from the server"}. That behavior is
// preserved here: a JSON decode failure yields that sentinel response and a nil
// error, matching the original client contract. Create/update did not guard
// against decode errors in Python, but applying the same guard uniformly is
// safer and does not change successful-path behavior.
func (c *Client) doJSON(ctx context.Context, method, path string, body interface{}) (APIResponse, error) {
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
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out APIResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		// Mirror the Python JSONDecodeError fallback.
		return APIResponse{"error": "Invalid response from the server"}, nil
	}

	return out, nil
}

// CreatePrompt sends POST /create with the given prompt text and returns the
// decoded API response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (APIResponse, error) {
	body := map[string]string{"prompt": prompt}
	return c.doJSON(ctx, http.MethodPost, "/create", body)
}

// GetResponse sends GET /get/{index} and returns the decoded API response.
//
// MIGRATION_NOTE: promptIndex is a 0-based positional index into the server's
// ordered prompt collection, matching the original indexing semantics.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (APIResponse, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt sends PUT /update/{index} with the new prompt text and returns
// the decoded API response.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (APIResponse, error) {
	body := map[string]string{"new_prompt": newPrompt}
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), body)
}

// DeletePrompt sends DELETE /delete/{index} and returns the decoded API response.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (APIResponse, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// Run exercises the client against the API, mirroring the Python main() test
// harness. It is exported so a thin cmd/ main package can invoke it.
//
// MIGRATION_NOTE: The Python __main__ guard maps to a cmd/ entrypoint in Go.
// This function reproduces the original test sequence: create two prompts, get,
// update, get-after-update, delete, and get-after-delete. Output is written to
// stdout as in the original (print statements).
func Run(ctx context.Context, c *Client) error {
	prompt1 := "What is life?"
	prompt2 := "What is the capital of Pakistan?"

	createResp1, err := c.CreatePrompt(ctx, prompt1)
	if err != nil {
		return fmt.Errorf("create prompt1: %w", err)
	}
	createResp2, err := c.CreatePrompt(ctx, prompt2)
	if err != nil {
		return fmt.Errorf("create prompt2: %w", err)
	}
	fmt.Println(createResp1)
	fmt.Println(createResp2)

	// Get index 0.
	resp, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get index 0: %w", err)
	}
	fmt.Println(resp)

	// Update index 1.
	updateResp, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return fmt.Errorf("update index 1: %w", err)
	}
	fmt.Println(updateResp)

	// Get index 1 after update.
	respAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get index 1 after update: %w", err)
	}
	fmt.Println(respAfterUpdate)

	// Delete index 0.
	deleteResp, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete index 0: %w", err)
	}
	fmt.Println(deleteResp)

	// Get index 0 after delete.
	respAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get index 0 after delete: %w", err)
	}
	fmt.Println(respAfterDelete)

	return nil
}
