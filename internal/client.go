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

// invalidResponseError is the fallback payload returned when the server's
// response body cannot be decoded as JSON (e.g. a non-JSON 404/500 page).
//
// MIGRATION_NOTE: The Python client caught requests.exceptions.JSONDecodeError
// on GET /get/{index} and DELETE /delete/{index} and returned this exact
// {"error": "Invalid response from the server"} map. We preserve that behavior
// as a load-bearing fallback rather than propagating the decode error, because
// callers depend on the map shape.
func invalidResponseError() map[string]any {
	return map[string]any{"error": "Invalid response from the server"}
}

// Client is an HTTP consumer of the prompt CRUD REST API.
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient constructs a Client targeting the given base URL. If httpClient is
// nil, http.DefaultClient is used. If baseURL is empty, DefaultBaseURL is used.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, hc: httpClient}
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// response body into a generic map. It never returns a transport-level error
// as a JSON-decode fallback; only genuine network/construction failures return
// a non-nil error.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		// MIGRATION_NOTE: mirrors the Python JSONDecodeError fallback. A non-JSON
		// response (404/500 HTML page etc.) yields the invalid-response map
		// instead of an error. The HTTP 200 "invalid index" quirk is preserved
		// because we do not inspect resp.StatusCode at all here — whatever JSON
		// the server returns (including a 200 with an error field) is passed
		// straight through to the caller.
		return invalidResponseError(), nil
	}
	return decoded, nil
}

// CreatePrompt sends POST /create with the given prompt text and returns the
// server's decoded JSON response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", map[string]any{"prompt": prompt})
}

// GetResponse sends GET /get/{promptIndex} and returns the server's decoded
// JSON response, or the invalid-response fallback map if the body is not JSON.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt sends PUT /update/{promptIndex} with the new prompt text and
// returns the server's decoded JSON response.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), map[string]any{"new_prompt": newPrompt})
}

// DeletePrompt sends DELETE /delete/{promptIndex} and returns the server's
// decoded JSON response, or the invalid-response fallback map if the body is
// not JSON.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// RunDemo exercises the full CRUD flow against the API, mirroring the original
// script's __main__ block. It writes each response to the provided writer.
//
// MIGRATION_NOTE: The Python script printed to stdout from a top-level main().
// Here the demo is a reusable exported function taking an io.Writer so it can
// be driven from a cmd/ entrypoint or a test. Errors are returned rather than
// panicking, per idiomatic Go.
func (c *Client) RunDemo(ctx context.Context, out io.Writer) error {
	printResp := func(label string, resp map[string]any) {
		fmt.Fprintf(out, "%s: %v\n", label, resp)
	}

	prompt1 := "What is life?"
	prompt2 := "What is the capital of Pakistan?"

	createResp1, err := c.CreatePrompt(ctx, prompt1)
	if err != nil {
		return fmt.Errorf("create prompt 1: %w", err)
	}
	printResp("create1", createResp1)

	createResp2, err := c.CreatePrompt(ctx, prompt2)
	if err != nil {
		return fmt.Errorf("create prompt 2: %w", err)
	}
	printResp("create2", createResp2)

	// Test get
	resp, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get index 0: %w", err)
	}
	printResp("get", resp)

	// Test update
	updateResp, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return fmt.Errorf("update index 1: %w", err)
	}
	printResp("update", updateResp)

	// Test get after update
	respAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get index 1 after update: %w", err)
	}
	printResp("getAfterUpdate", respAfterUpdate)

	// Test delete
	deleteResp, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete index 0: %w", err)
	}
	printResp("delete", deleteResp)

	// Test get after delete
	respAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get index 0 after delete: %w", err)
	}
	printResp("getAfterDelete", respAfterDelete)

	return nil
}
