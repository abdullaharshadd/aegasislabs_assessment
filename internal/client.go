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

// errInvalidResponse mirrors the Python client's fallback body returned when the
// server response cannot be decoded as JSON.
//
// MIGRATION_NOTE: The Python client returned {"error": "Invalid response from
// the server"} on a JSONDecodeError. Here we return the same map so callers
// observe identical behavior. The decision to fall back is body-content driven
// (a failed JSON decode), not Content-Type header driven, matching the source.
func errInvalidResponse() map[string]any {
	return map[string]any{"error": "Invalid response from the server"}
}

// Client is an HTTP client for the prompt CRUD API.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient constructs a Client targeting the given base URL. If baseURL is
// empty, DefaultBaseURL is used. If httpClient is nil, http.DefaultClient is used.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, http: httpClient}
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// response body as a generic JSON object. It returns (nil, error) only on
// transport-level or request-construction failures. When the response body
// cannot be decoded as JSON it returns the invalid-response fallback map with a
// nil error, matching the Python client's JSONDecodeError handling.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any) (map[string]any, error) {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
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
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		// Body-content driven fallback, matching the Python JSONDecodeError path.
		return errInvalidResponse(), nil
	}
	return out, nil
}

// CreatePrompt sends a POST /create request to store a new prompt and returns
// the decoded JSON response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", map[string]any{"prompt": prompt})
}

// GetResponse sends a GET /get/{index} request and returns the decoded JSON
// response. If the server returns a non-JSON body, the invalid-response
// fallback is returned with a nil error.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt sends a PUT /update/{index} request to replace an existing
// prompt and returns the decoded JSON response.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), map[string]any{"new_prompt": newPrompt})
}

// DeletePrompt sends a DELETE /delete/{index} request and returns the decoded
// JSON response. If the server returns a non-JSON body, the invalid-response
// fallback is returned with a nil error.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// RunClientDemo exercises each CRUD operation against the API, mirroring the
// Python script's main() routine. It prints each response to stdout and returns
// the first transport-level error encountered, if any.
//
// MIGRATION_NOTE: The Python __main__ guard ran main() when executed as a
// script. In Go, package internal cannot host an executable entrypoint;
// RunClientDemo is exported so a cmd/ binary (or a test) can invoke it. Wire it
// up from a cmd/<name>/main.go if a standalone demo binary is desired.
func RunClientDemo(ctx context.Context, c *Client) error {
	const (
		prompt1 = "What is life?"
		prompt2 = "What is the capital of Pakistan?"
	)

	createResponse1, err := c.CreatePrompt(ctx, prompt1)
	if err != nil {
		return fmt.Errorf("create prompt 1: %w", err)
	}
	createResponse2, err := c.CreatePrompt(ctx, prompt2)
	if err != nil {
		return fmt.Errorf("create prompt 2: %w", err)
	}
	fmt.Println(createResponse1)
	fmt.Println(createResponse2)

	response, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get response index 0: %w", err)
	}
	fmt.Println(response)

	updateResponse, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return fmt.Errorf("update prompt index 1: %w", err)
	}
	fmt.Println(updateResponse)

	responseAfterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return fmt.Errorf("get response after update: %w", err)
	}
	fmt.Println(responseAfterUpdate)

	deleteResponse, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return fmt.Errorf("delete prompt index 0: %w", err)
	}
	fmt.Println(deleteResponse)

	responseAfterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return fmt.Errorf("get response after delete: %w", err)
	}
	fmt.Println(responseAfterDelete)

	return nil
}
