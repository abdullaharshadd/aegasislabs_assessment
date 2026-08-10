// Package client provides an HTTP client for a CRUD REST API that manages prompts.
//
// MIGRATION_NOTE: The original file was labeled as Django code but is actually a
// standalone Python HTTP client (using the requests library) that consumes a
// REST API, likely served by Flask on port 5000. This Go migration preserves the
// business logic while adding idiomatic Go patterns: context propagation,
// explicit error handling, timeouts, and a constructor-based client.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// defaultBaseURL is used when the PROMPT_API_BASE_URL environment variable is unset.
//
// MIGRATION_NOTE: The original hardcoded BASE_URL is now sourced from an
// environment variable, per the migration notes recommending configurable base URLs.
const defaultBaseURL = "http://127.0.0.1:5000"

// defaultTimeout bounds every HTTP request.
//
// MIGRATION_NOTE: The original Python client configured no timeout. A timeout is
// added here for production readiness. Retry and authentication remain TODO.
const defaultTimeout = 10 * time.Second

// Response represents a decoded JSON response from the prompt API.
//
// MIGRATION_NOTE: The Python code returned arbitrary decoded JSON (dicts). Since
// the server contract is not strongly typed, we decode into a generic map. When
// the concrete API schema is known, replace this with typed structs.
type Response map[string]any

// Client is an HTTP client for the prompt CRUD REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the default *http.Client used by the Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithBaseURL overrides the base URL used by the Client.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// NewClient constructs a Client. The base URL defaults to the PROMPT_API_BASE_URL
// environment variable, falling back to defaultBaseURL if unset.
func NewClient(opts ...Option) *Client {
	baseURL := os.Getenv("PROMPT_API_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// JSON response into a Response.
//
// MIGRATION_NOTE: The original Python code only handled JSONDecodeError on GET
// and DELETE. This helper applies consistent decode-error handling to every
// verb, addressing the inconsistency flagged in the migration notes.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any) (Response, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("build %s request for %s: %w", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform %s request to %s: %w", method, url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body from %s: %w", url, err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		// MIGRATION_NOTE: Mirrors the Python client's fallback of returning
		// {"error": "Invalid response from the server"} rather than propagating
		// the decode error. We return it as a Response with a nil error to
		// preserve that behavior.
		return Response{"error": "Invalid response from the server"}, nil
	}

	return decoded, nil
}

// CreatePrompt creates a new prompt and returns the server's decoded response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	body := map[string]string{"prompt": prompt}
	return c.doJSON(ctx, http.MethodPost, "/create", body)
}

// GetResponse retrieves the prompt at the given positional index.
//
// MIGRATION_NOTE: The API uses integer positional indices rather than stable
// resource IDs. This contract is preserved as-is; consider migrating to
// ID-based lookups on the server side.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	endpoint := fmt.Sprintf("/get/%d", promptIndex)
	return c.doJSON(ctx, http.MethodGet, endpoint, nil)
}

// UpdatePrompt updates the prompt at the given positional index.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	endpoint := fmt.Sprintf("/update/%d", promptIndex)
	body := map[string]string{"new_prompt": newPrompt}
	return c.doJSON(ctx, http.MethodPut, endpoint, body)
}

// DeletePrompt deletes the prompt at the given positional index.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	endpoint := fmt.Sprintf("/delete/%d", promptIndex)
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil)
}

// Run exercises the CRUD API, preserving the demonstration flow of the original
// Python main() function.
//
// MIGRATION_NOTE: The Python main() printed responses and swallowed errors. Here
// each error is returned so callers can decide how to handle failures, while
// successful responses are logged to preserve the demonstration output.
func Run(ctx context.Context, c *Client) error {
	prompt1 := "What is life?"
	prompt2 := "What is the capital of Pakistan?"

	createResponse1, err := c.CreatePrompt(ctx, prompt1)
	if err != nil {
		return fmt.Errorf("create prompt 1: %w", err)
	}
	log.Printf("%v", createResponse1)

	createResponse2, err := c.CreatePrompt(ctx, prompt2)
	if err != nil {
		return fmt.Errorf("create prompt 2: %w", err)
	}
	log.Printf("%v", createResponse2)

	// Test GetResponse.
	response, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0: %w", err)
	}
	log.Printf("%v", response)

	// Test UpdatePrompt.
	newPrompt := "Who is Goku?"
	updateResponse, err := c.UpdatePrompt(ctx, 1, newPrompt)
	if err != nil {
		return fmt.Errorf("update prompt 1: %w", err)
	}
	log.Printf("%v", updateResponse)

	// Test GetResponse after update.
	responseAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get prompt 1 after update: %w", err)
	}
	log.Printf("%v", responseAfterUpdate)

	// Test DeletePrompt.
	deleteResponse, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete prompt 0: %w", err)
	}
	log.Printf("%v", deleteResponse)

	// Test GetResponse after delete.
	responseAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0 after delete: %w", err)
	}
	log.Printf("%v", responseAfterDelete)

	return nil
}

// MIGRATION_NOTE: The Python `if __name__ == "__main__": main()` entrypoint maps
// to a Go `func main()` in a `package main` binary under cmd/. Because this file
// lives in an internal library package, the runnable entrypoint below is
// commented out. Move it to cmd/client/main.go to build an executable:
//
//	package main
//
//	func main() {
//		ctx := context.Background()
//		c := client.NewClient()
//		if err := client.Run(ctx, c); err != nil {
//			log.Fatalf("client run failed: %v", err)
//		}
//	}
