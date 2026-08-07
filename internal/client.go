// Package client provides a small HTTP client for the prompts REST API.
//
// It mirrors the behaviour of the original standalone Python client that
// exercised the create/read/update/delete endpoints of a running API server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultBaseURL is the base URL of the target API server.
//
// MIGRATION_NOTE: The original Python module used a package-level BASE_URL
// constant ("http://127.0.0.1:5000"). In Go this is exposed as a configurable
// field on Client with this default; replace it with the address of your
// running API server.
const DefaultBaseURL = "http://127.0.0.1:5000"

// Client is an HTTP client for the prompts REST API.
type Client struct {
	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API server base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient overrides the underlying *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.http = hc
		}
	}
}

// NewClient constructs a Client. By default it targets DefaultBaseURL and uses
// the default *http.Client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		http:    &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Response represents a decoded JSON response from the API server.
//
// MIGRATION_NOTE: The Python client returned whatever JSON shape the server
// produced (an arbitrary dict). We model that faithfully as a generic map so
// that no server field is silently dropped.
type Response map[string]any

// invalidResponse mirrors the Python fallback {"error": "Invalid response
// from the server"} returned when the body cannot be decoded as JSON.
func invalidResponse() Response {
	return Response{"error": "Invalid response from the server"}
}

// doJSON issues an HTTP request with an optional JSON body and decodes the
// JSON response. When fallbackOnDecodeError is true, a body that cannot be
// decoded as JSON yields the invalidResponse fallback instead of an error,
// matching the Python JSONDecodeError handling on GET and DELETE.
//
// MIGRATION_NOTE: The original fallback is body-content driven (a failed JSON
// decode), not Content-Type header driven, so we do not inspect Content-Type.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any, fallbackOnDecodeError bool) (Response, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("build %s %s request: %w", method, endpoint, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s %s response: %w", method, endpoint, err)
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		if fallbackOnDecodeError {
			return invalidResponse(), nil
		}
		return nil, fmt.Errorf("decode %s %s response: %w", method, endpoint, err)
	}
	return out, nil
}

// CreatePrompt sends a new prompt to the /create endpoint and returns the
// server's JSON response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	return c.doJSON(ctx, http.MethodPost, "/create", map[string]string{"prompt": prompt}, false)
}

// GetResponse fetches the prompt at the given index from the /get/{index}
// endpoint. If the server body is not valid JSON, it returns the
// invalidResponse fallback rather than an error.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	return c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil, true)
}

// UpdatePrompt updates the prompt at the given index via the
// /update/{index} endpoint and returns the server's JSON response.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), map[string]string{"new_prompt": newPrompt}, false)
}

// DeletePrompt deletes the prompt at the given index via the
// /delete/{index} endpoint. If the server body is not valid JSON, it returns
// the invalidResponse fallback rather than an error.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil, true)
}
