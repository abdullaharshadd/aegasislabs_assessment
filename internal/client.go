package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultBaseURL is the base URL of the running API server.
// Replace this with the base URL of your running API server.
const DefaultBaseURL = "http://127.0.0.1:5000"

// invalidResponse is the fallback payload the client synthesizes when the
// server returns a body that cannot be decoded as JSON.
//
// MIGRATION_NOTE: This preserves the original Python behavior where a
// JSONDecodeError (e.g. the server returning a 500 due to a missing/placeholder
// OpenAI key) is swallowed and replaced with {"error": "Invalid response from
// the server"}. This matches the reference app's out-of-the-box observable
// behavior and is intentionally kept.
func invalidResponse() map[string]any {
	return map[string]any{"error": "Invalid response from the server"}
}

// Client is a thin REST consumer for the prompt CRUD API.
type Client struct {
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL used by the Client.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient overrides the underlying *http.Client used for requests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.http = hc
	}
}

// NewClient constructs a Client with sensible defaults, overridable via options.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		http:    http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// response body into a generic map. If the response body cannot be decoded as
// JSON, it returns the synthesized invalidResponse payload (matching the
// original Python client's JSONDecodeError fallback) rather than an error.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		// MIGRATION_NOTE: mirror Python's JSONDecodeError fallback.
		return invalidResponse(), nil
	}
	return out, nil
}

// CreatePrompt sends a new prompt to the API and returns the decoded response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", map[string]any{"prompt": prompt})
}

// GetResponse fetches the prompt/response at the given index.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt updates the prompt at the given index with a new prompt value.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), map[string]any{"new_prompt": newPrompt})
}

// DeletePrompt removes the prompt at the given index.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// RunDemo exercises the full CRUD flow against the API, mirroring the behavior
// of the original Python script's main() function. Results are printed to the
// provided writer.
//
// MIGRATION_NOTE: The original file was a standalone script with an
// `if __name__ == "__main__"` entrypoint. Since this package is `internal` and
// the module already has a server entrypoint in cmd/server, the demo logic is
// exposed as an exported function rather than its own main(). Wire it into a
// dedicated cmd/ binary if you want to run it directly.
func RunDemo(ctx context.Context, c *Client, w io.Writer) error {
	printResult := func(v map[string]any) error {
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode result: %w", err)
		}
		if _, err := fmt.Fprintln(w, string(encoded)); err != nil {
			return fmt.Errorf("write result: %w", err)
		}
		return nil
	}

	// Test CreatePrompt
	createResponse1, err := c.CreatePrompt(ctx, "What is life?")
	if err != nil {
		return err
	}
	createResponse2, err := c.CreatePrompt(ctx, "What is the capital of Pakistan?")
	if err != nil {
		return err
	}
	if err := printResult(createResponse1); err != nil {
		return err
	}
	if err := printResult(createResponse2); err != nil {
		return err
	}

	// Test GetResponse
	response, err := c.GetResponse(ctx, 0)
	if err != nil {
		return err
	}
	if err := printResult(response); err != nil {
		return err
	}

	// Test UpdatePrompt
	updateResponse, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return err
	}
	if err := printResult(updateResponse); err != nil {
		return err
	}

	// Test GetResponse after update
	responseAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return err
	}
	if err := printResult(responseAfterUpdate); err != nil {
		return err
	}

	// Test DeletePrompt
	deleteResponse, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return err
	}
	if err := printResult(deleteResponse); err != nil {
		return err
	}

	// Test GetResponse after delete
	responseAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return err
	}
	if err := printResult(responseAfterDelete); err != nil {
		return err
	}

	return nil
}
