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

// Client is a thin HTTP client for the prompt CRUD REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a Client pointing at the given base URL. If baseURL is
// empty, DefaultBaseURL is used. If httpClient is nil, http.DefaultClient is used.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// APIResponse holds a decoded JSON response body. Since the upstream API returns
// arbitrary JSON objects, the payload is kept as a generic map. When the server
// returns a non-JSON body (mirroring the Python JSONDecodeError handling), the
// result carries {"error": "Invalid response from the server"}.
type APIResponse map[string]any

// post issues a POST request with a JSON body. Content-Type is set explicitly
// because Go's net/http does not infer it the way Python's requests json= kwarg does.
func (c *Client) post(ctx context.Context, endpoint string, body any) (APIResponse, error) {
	return c.doWithBody(ctx, http.MethodPost, endpoint, body)
}

// put issues a PUT request with a JSON body. Content-Type is set explicitly for
// the same reason as post.
func (c *Client) put(ctx context.Context, endpoint string, body any) (APIResponse, error) {
	return c.doWithBody(ctx, http.MethodPut, endpoint, body)
}

// get issues a bodyless GET request. Content-Type is intentionally omitted to
// prove the endpoint does not 415 on a bodyless request.
func (c *Client) get(ctx context.Context, endpoint string) (APIResponse, error) {
	return c.doBodyless(ctx, http.MethodGet, endpoint)
}

// del issues a bodyless DELETE request. Content-Type is intentionally omitted.
func (c *Client) del(ctx context.Context, endpoint string) (APIResponse, error) {
	return c.doBodyless(ctx, http.MethodDelete, endpoint)
}

func (c *Client) doWithBody(ctx context.Context, method, endpoint string, body any) (APIResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.do(req)
}

func (c *Client) doBodyless(ctx context.Context, method, endpoint string) (APIResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	// Content-Type intentionally omitted for bodyless requests.
	return c.do(req)
}

// do executes the request and decodes the JSON response. A body that is not
// valid JSON is reported as APIResponse{"error": "Invalid response from the
// server"}, mirroring the Python client's JSONDecodeError fallback.
func (c *Client) do(req *http.Request) (APIResponse, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute %s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var result APIResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return APIResponse{"error": "Invalid response from the server"}, nil
	}
	return result, nil
}

// CreatePrompt sends a new prompt to the /create endpoint.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (APIResponse, error) {
	return c.post(ctx, "/create", map[string]any{"prompt": prompt})
}

// GetResponse fetches the prompt at the given index via /get/{index}.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (APIResponse, error) {
	return c.get(ctx, fmt.Sprintf("/get/%d", promptIndex))
}

// UpdatePrompt updates the prompt at the given index via /update/{index}.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (APIResponse, error) {
	return c.put(ctx, fmt.Sprintf("/update/%d", promptIndex), map[string]any{"new_prompt": newPrompt})
}

// DeletePrompt deletes the prompt at the given index via /delete/{index}.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (APIResponse, error) {
	return c.del(ctx, fmt.Sprintf("/delete/%d", promptIndex))
}

// RunDemo reproduces the Python script's __main__ sequence: create two prompts,
// read, update, re-read, delete, and re-read. Results are written to out (each
// APIResponse rendered as JSON), and the first error encountered is returned.
//
// MIGRATION_NOTE: the Python script printed to stdout and had no error
// propagation. Here errors are returned so callers can decide how to handle
// them; a demo caller may simply log them. This function replaces the
// `if __name__ == "__main__": main()` guard, which has no Go equivalent.
func RunDemo(ctx context.Context, c *Client, out io.Writer) error {
	printResponse := func(resp APIResponse) error {
		encoded, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
		if _, err := fmt.Fprintln(out, string(encoded)); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
		return nil
	}

	// Test CreatePrompt.
	createResp1, err := c.CreatePrompt(ctx, "What is life?")
	if err != nil {
		return err
	}
	if err := printResponse(createResp1); err != nil {
		return err
	}

	createResp2, err := c.CreatePrompt(ctx, "What is the capital of Pakistan?")
	if err != nil {
		return err
	}
	if err := printResponse(createResp2); err != nil {
		return err
	}

	// Test GetResponse.
	getResp, err := c.GetResponse(ctx, 0)
	if err != nil {
		return err
	}
	if err := printResponse(getResp); err != nil {
		return err
	}

	// Test UpdatePrompt.
	updateResp, err := c.UpdatePrompt(ctx, 1, "Who is Goku?")
	if err != nil {
		return err
	}
	if err := printResponse(updateResp); err != nil {
		return err
	}

	// Test GetResponse after update.
	afterUpdate, err := c.GetResponse(ctx, 1)
	if err != nil {
		return err
	}
	if err := printResponse(afterUpdate); err != nil {
		return err
	}

	// Test DeletePrompt.
	deleteResp, err := c.DeletePrompt(ctx, 0)
	if err != nil {
		return err
	}
	if err := printResponse(deleteResp); err != nil {
		return err
	}

	// Test GetResponse after delete.
	afterDelete, err := c.GetResponse(ctx, 0)
	if err != nil {
		return err
	}
	return printResponse(afterDelete)
}
