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

// Client is an HTTP client for consuming the prompt REST API.
// It performs CRUD operations against the endpoints exposed by the server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client using the functional options pattern.
type Option func(*Client)

// WithBaseURL overrides the default base URL used by the Client.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient overrides the default *http.Client used by the Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// NewClient constructs a Client. Any supplied Options override the defaults
// (DefaultBaseURL and http.DefaultClient).
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// APIResponse represents a decoded JSON response from the prompt API.
//
// MIGRATION_NOTE: The Python client returned the raw decoded JSON dict from
// each endpoint. The server contract uses different top-level keys per
// endpoint ("response" for Get, "message" for Update/Delete, plus "error").
// Rather than a dynamic map, we decode into a struct capturing the known
// keys. Unknown keys are preserved in Extra for forward compatibility.
type APIResponse struct {
	// Response holds the payload returned by the Get endpoint.
	Response string `json:"response,omitempty"`
	// Message holds the payload returned by the Update and Delete endpoints.
	Message string `json:"message,omitempty"`
	// Error holds a server- or client-side error message when present.
	Error string `json:"error,omitempty"`
	// Extra captures any additional JSON keys not modeled above.
	Extra map[string]any `json:"-"`
}

// doJSON performs an HTTP request with an optional JSON body and decodes the
// JSON response body into an APIResponse. When the response body is not valid
// JSON, it returns an APIResponse with Error set, mirroring the Python client's
// JSONDecodeError handling, rather than returning a hard error.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) (*APIResponse, error) {
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
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return decodeResponse(raw)
}

// decodeResponse decodes a raw JSON body into an APIResponse. On invalid JSON
// it returns an APIResponse with Error set to "Invalid response from the
// server", matching the Python client's behaviour on JSONDecodeError.
func decodeResponse(raw []byte) (*APIResponse, error) {
	var out APIResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return &APIResponse{Error: "Invalid response from the server"}, nil
	}

	// Preserve any unmodeled keys in Extra.
	var extra map[string]any
	if err := json.Unmarshal(raw, &extra); err == nil {
		delete(extra, "response")
		delete(extra, "message")
		delete(extra, "error")
		if len(extra) > 0 {
			out.Extra = extra
		}
	}
	return &out, nil
}

// createPromptRequest is the request body for the create endpoint.
type createPromptRequest struct {
	Prompt string `json:"prompt"`
}

// updatePromptRequest is the request body for the update endpoint.
type updatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// CreatePrompt sends a POST /create request creating a new prompt and returns
// the decoded server response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (*APIResponse, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", createPromptRequest{Prompt: prompt})
}

// GetResponse sends a GET /get/{index} request and returns the decoded server
// response for the prompt at the given index.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (*APIResponse, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt sends a PUT /update/{index} request replacing the prompt at the
// given index and returns the decoded server response.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (*APIResponse, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), updatePromptRequest{NewPrompt: newPrompt})
}

// DeletePrompt sends a DELETE /delete/{index} request removing the prompt at
// the given index and returns the decoded server response.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (*APIResponse, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}
