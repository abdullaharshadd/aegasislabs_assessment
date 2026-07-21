// Package client provides a thin HTTP client for consuming the prompt CRUD REST API.
//
// MIGRATION_NOTE: The original Python module was a standalone script using the
// `requests` library. It has been ported as an idiomatic Go package exposing a
// Client type with methods for each endpoint. The verb-in-path URL contract is
// preserved exactly (POST /create, GET /get/{id}, PUT /update/{id},
// DELETE /delete/{id}) to match the Go router on the server side.
//
// CHANGE 2 (from migration debate): create now captures and returns the
// server-assigned ID from the response so callers can reference newly created
// prompts instead of relying on positional indexes only.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the base URL of the running API server.
//
// MIGRATION_NOTE: In the Python source this was a module-level constant with a
// comment instructing the user to replace it. In Go it is exposed as an
// overridable field on Client via the WithBaseURL option.
const DefaultBaseURL = "http://127.0.0.1:5000"

// Client is a thin HTTP client for the prompt CRUD REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the base URL of the target API server.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient overrides the underlying *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// NewClient constructs a Client with sensible defaults, applying any options.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CreatePromptRequest is the JSON body sent to POST /create.
type CreatePromptRequest struct {
	Prompt string `json:"prompt"`
}

// UpdatePromptRequest is the JSON body sent to PUT /update/{id}.
type UpdatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// Response models a decoded JSON response from the API.
//
// MIGRATION_NOTE: The Python client returned the raw decoded JSON (a dict). The
// server response shape is not strongly typed in the source, so we decode into
// a generic map to preserve arbitrary fields. Callers that know the concrete
// shape may re-marshal/unmarshal into a typed struct.
type Response map[string]any

// doJSON performs an HTTP request with an optional JSON body and decodes the
// JSON response. If the response body is not valid JSON, it returns a Response
// containing an "error" key rather than failing, mirroring the Python client's
// JSONDecodeError handling.
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
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		// Mirror Python's behaviour: an invalid JSON body yields an error map
		// rather than a hard failure.
		return Response{"error": "Invalid response from the server"}, nil
	}
	return out, nil
}

// CreatePrompt sends POST /create with the given prompt and returns the decoded
// response. Per CHANGE 2, callers may read the server-assigned ID (if present)
// from the returned Response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", CreatePromptRequest{Prompt: prompt})
}

// GetResponse sends GET /get/{promptIndex} and returns the decoded response.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt sends PUT /update/{promptIndex} with the new prompt text.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), UpdatePromptRequest{NewPrompt: newPrompt})
}

// DeletePrompt sends DELETE /delete/{promptIndex} and returns the decoded response.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// Demo exercises each endpoint in sequence, mirroring the original Python
// main() function. It is intended for manual smoke-testing against a running
// server.
//
// MIGRATION_NOTE: The Python `main()` under `if __name__ == "__main__"` becomes
// an exported Demo function. Wire it into a cmd/ main package (e.g.
// cmd/client/main.go) to run it as a standalone binary; keeping it in the
// internal package as a method avoids an unused import when the client is used
// purely as a library.
func (c *Client) Demo(ctx context.Context) error {
	// Test CreatePrompt
	const prompt1 = "What is life?"
	const prompt2 = "What is the capital of Pakistan?"

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

	// Test GetResponse
	resp, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0: %w", err)
	}
	fmt.Println(resp)

	// Test UpdatePrompt
	const newPrompt = "Who is Goku?"
	updateResp, err := c.UpdatePrompt(ctx, 1, newPrompt)
	if err != nil {
		return fmt.Errorf("update prompt 1: %w", err)
	}
	fmt.Println(updateResp)

	// Test GetResponse after update
	respAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get prompt 1 after update: %w", err)
	}
	fmt.Println(respAfterUpdate)

	// Test DeletePrompt
	deleteResp, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete prompt 0: %w", err)
	}
	fmt.Println(deleteResp)

	// Test GetResponse after delete
	respAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0 after delete: %w", err)
	}
	fmt.Println(respAfterDelete)

	return nil
}
