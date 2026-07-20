// Package client provides an HTTP client for consuming the prompts REST API.
//
// The API server exposes CRUD operations on prompts identified by a 0-based
// positional index. This client wraps those endpoints and returns decoded JSON
// responses.
//
// MIGRATION_NOTE: The original client.py was a plain Python script using the
// 'requests' library (no Django/server code). BASE_URL was hardcoded; here it
// is configurable via the PROMPTS_API_BASE_URL environment variable with a
// sensible default. The original main() was a manual demo runner and is
// preserved as a small runnable example under a separate cmd binary or, as
// here, an exported RunDemo function. Consider replacing it with proper
// integration tests.
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

// defaultBaseURL is used when PROMPTS_API_BASE_URL is not set.
//
// MIGRATION_NOTE: In client.py this was the hardcoded BASE_URL constant.
const defaultBaseURL = "http://127.0.0.1:5000"

// Doer abstracts the subset of *http.Client used by Client, allowing callers
// to inject a custom implementation (e.g. for testing).
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client consumes the prompts REST API.
type Client struct {
	baseURL string
	http    Doer
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the base URL of the target API server.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(d Doer) Option {
	return func(c *Client) {
		c.http = d
	}
}

// NewClient constructs a Client. If no base URL option is provided, it uses the
// PROMPTS_API_BASE_URL environment variable, falling back to defaultBaseURL.
func NewClient(opts ...Option) *Client {
	base := os.Getenv("PROMPTS_API_BASE_URL")
	if base == "" {
		base = defaultBaseURL
	}

	c := &Client{
		baseURL: base,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Response represents a decoded JSON response from the API server.
//
// MIGRATION_NOTE: The Python client returned untyped dicts (response.json()).
// Because the server's response schema is not defined in this file, we decode
// into a generic map. If the schema becomes known, replace this with a typed
// struct.
type Response map[string]any

// errInvalidResponse mirrors the Python client's graceful handling of
// non-JSON server responses by returning {"error": "..."} instead of failing.
func errInvalidResponse() Response {
	return Response{"error": "Invalid response from the server"}
}

// doJSON sends the request, reads the body, and decodes it as JSON. On a JSON
// decode error it returns errInvalidResponse rather than an error, matching the
// original client's try/except behavior. Transport-level failures still return
// an error.
func (c *Client) doJSON(req *http.Request) (Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request %s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out Response
	if err := json.Unmarshal(body, &out); err != nil {
		// Mirror Python's JSONDecodeError handling: return a graceful error map.
		return errInvalidResponse(), nil
	}
	return out, nil
}

// newJSONRequest builds an HTTP request with an optional JSON body.
func (c *Client) newJSONRequest(ctx context.Context, method, path string, payload any) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// CreatePrompt sends a new prompt to the server via POST /create.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/create", map[string]string{"prompt": prompt})
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

// GetResponse fetches the prompt at the given 0-based index via GET /get/{index}.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	req, err := c.newJSONRequest(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

// UpdatePrompt replaces the prompt at the given index via PUT /update/{index}.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	req, err := c.newJSONRequest(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), map[string]string{"new_prompt": newPrompt})
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

// DeletePrompt removes the prompt at the given index via DELETE /delete/{index}.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	req, err := c.newJSONRequest(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
	if err != nil {
		return nil, err
	}
	return c.doJSON(req)
}

// RunDemo exercises each endpoint in sequence, mirroring the original Python
// main() manual test runner. It writes results to the provided writer.
//
// MIGRATION_NOTE: This replaces the `if __name__ == "__main__": main()` script
// entry point. In production this logic belongs in a cmd/ binary or in
// integration tests rather than in the library package.
func RunDemo(ctx context.Context, c *Client, out io.Writer) error {
	printResp := func(label string, r Response) {
		fmt.Fprintf(out, "%s: %v\n", label, r)
	}

	// Test CreatePrompt.
	createResp1, err := c.CreatePrompt(ctx, "What is life?")
	if err != nil {
		return fmt.Errorf("create prompt 1: %w", err)
	}
	printResp("create1", createResp1)

	createResp2, err := c.CreatePrompt(ctx, "What is the capital of Pakistan?")
	if err != nil {
		return fmt.Errorf("create prompt 2: %w", err)
	}
	printResp("create2", createResp2)

	// Test GetResponse.
	getResp, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0: %w", err)
	}
	printResp("get0", getResp)

	// Test UpdatePrompt.
	updateResp, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return fmt.Errorf("update prompt 1: %w", err)
	}
	printResp("update1", updateResp)

	// Test GetResponse after update.
	afterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get prompt 1 after update: %w", err)
	}
	printResp("get1AfterUpdate", afterUpdate)

	// Test DeletePrompt.
	deleteResp, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete prompt 0: %w", err)
	}
	printResp("delete0", deleteResp)

	// Test GetResponse after delete.
	afterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0 after delete: %w", err)
	}
	printResp("get0AfterDelete", afterDelete)

	return nil
}
