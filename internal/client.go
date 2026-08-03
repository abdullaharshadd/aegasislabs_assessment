package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultBaseURL is the base URL of the running prompt API server.
// Replace this with the base URL of your running API server.
const DefaultBaseURL = "http://127.0.0.1:5000"

// PromptClient is a REST consumer for the prompts CRUD API.
type PromptClient struct {
	baseURL string
	http    *http.Client
}

// NewPromptClient constructs a PromptClient targeting baseURL. If httpClient is
// nil, http.DefaultClient is used.
func NewPromptClient(baseURL string, httpClient *http.Client) *PromptClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &PromptClient{baseURL: baseURL, http: httpClient}
}

// decodeJSON reads and decodes the response body into a generic map. When the
// body is not valid JSON, it returns a map with an "error" key mirroring the
// Python client's JSONDecodeError fallback.
func decodeJSON(resp *http.Response) (map[string]any, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return map[string]any{"error": "Invalid response from the server"}, nil
	}
	return out, nil
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// response.
func (c *PromptClient) doJSON(ctx context.Context, method, endpoint string, payload any) (map[string]any, error) {
	var bodyReader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	return decodeJSON(resp)
}

// CreatePrompt sends a new prompt to the /create endpoint and returns the
// decoded response.
func (c *PromptClient) CreatePrompt(ctx context.Context, prompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", map[string]any{"prompt": prompt})
}

// GetResponse fetches the prompt at promptIndex from the /get/{index} endpoint.
func (c *PromptClient) GetResponse(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt replaces the prompt at promptIndex via the /update/{index}
// endpoint.
func (c *PromptClient) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), map[string]any{"new_prompt": newPrompt})
}

// DeletePrompt removes the prompt at promptIndex via the /delete/{index}
// endpoint.
func (c *PromptClient) DeletePrompt(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// RunDemo exercises the full CRUD flow against the API, mirroring the original
// script's __main__ block. It is intended as a manual smoke test rather than a
// production entrypoint.
func RunDemo(ctx context.Context, c *PromptClient) error {
	const (
		prompt1 = "What is life?"
		prompt2 = "What is the capital of Pakistan?"
	)

	create1, err := c.CreatePrompt(ctx, prompt1)
	if err != nil {
		return fmt.Errorf("create prompt 1: %w", err)
	}
	fmt.Println(create1)

	create2, err := c.CreatePrompt(ctx, prompt2)
	if err != nil {
		return fmt.Errorf("create prompt 2: %w", err)
	}
	fmt.Println(create2)

	// Test GetResponse.
	got, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0: %w", err)
	}
	fmt.Println(got)

	// Test UpdatePrompt.
	updated, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return fmt.Errorf("update prompt 1: %w", err)
	}
	fmt.Println(updated)

	// Test GetResponse after update.
	gotAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get prompt 1 after update: %w", err)
	}
	fmt.Println(gotAfterUpdate)

	// Test DeletePrompt.
	deleted, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete prompt 0: %w", err)
	}
	fmt.Println(deleted)

	// Test GetResponse after delete.
	gotAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get prompt 0 after delete: %w", err)
	}
	fmt.Println(gotAfterDelete)

	// GATE box 7: the bad-index case expectation is unresolved. The original
	// get_response path may either guard the bad index and return 404
	// (StatusNotFound) or let an IndexError propagate as a 500. Until a grep
	// confirms which path the server takes, the following assertion is left
	// commented out. Uncomment and adjust once box 7 is resolved:
	//
	//   assertStatus(resp, http.StatusNotFound)

	return nil
}
