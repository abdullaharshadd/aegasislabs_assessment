// Package client provides an HTTP client for exercising a CRUD REST API
// that manages prompts (create, get, update, delete).
//
// MIGRATION_NOTE: The original Python script used a hardcoded BASE_URL and a
// __main__ guard. In this Go port, the client is implemented as a reusable type
// with a constructor, the base URL is externalized via the PROMPT_API_BASE_URL
// environment variable, and the script entrypoint lives in package main
// (see cmd/client/main.go below or move main into its own package).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultBaseURL is used when no base URL is supplied. It mirrors the original
// hardcoded value but should be overridden via configuration in production.
const DefaultBaseURL = "http://127.0.0.1:5000"

// HTTPDoer abstracts the subset of *http.Client used by Client so that callers
// can inject a custom transport or mock implementation in tests.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is an HTTP client for the prompt CRUD API.
type Client struct {
	baseURL string
	http    HTTPDoer
}

// CreateRequest is the payload sent when creating a prompt.
type CreateRequest struct {
	Prompt string `json:"prompt"`
}

// UpdateRequest is the payload sent when updating an existing prompt.
type UpdateRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// Response represents a decoded JSON response from the API. The original API
// contract is implicit, so the response is modeled as a generic map.
//
// MIGRATION_NOTE: The Python code returned raw decoded JSON (a dict). Because
// the API schema is not formally defined, we decode into a generic map here.
// Replace this with a concrete struct once the API contract is known.
type Response map[string]interface{}

// NewClient constructs a Client. If baseURL is empty, DefaultBaseURL is used.
// If httpClient is nil, http.DefaultClient is used.
func NewClient(baseURL string, httpClient HTTPDoer) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, http: httpClient}
}

// CreatePrompt creates a new prompt and returns the decoded API response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (Response, error) {
	return c.do(ctx, http.MethodPost, "/create", CreateRequest{Prompt: prompt})
}

// GetResponse retrieves the prompt response at the given index.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (Response, error) {
	return c.do(ctx, http.MethodGet, fmt.Sprintf("/get/%d", promptIndex), nil)
}

// UpdatePrompt updates the prompt at the given index with newPrompt.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (Response, error) {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/update/%d", promptIndex), UpdateRequest{NewPrompt: newPrompt})
}

// DeletePrompt deletes the prompt at the given index.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (Response, error) {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/delete/%d", promptIndex), nil)
}

// do performs an HTTP request against the API and decodes the JSON response.
//
// MIGRATION_NOTE: The original Python code only handled JSONDecodeError on the
// get/delete calls. Per the migration notes, error handling is standardized
// here so every call returns a structured error response when the server
// returns a non-JSON body, rather than panicking or returning inconsistent
// results.
func (c *Client) do(ctx context.Context, method, endpoint string, payload interface{}) (Response, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encoding request payload: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing %s %s: %w", method, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		// Mirror the Python fallback behavior: return a structured error
		// payload instead of failing hard when the server returns non-JSON.
		return Response{"error": "Invalid response from the server"}, nil
	}
	return decoded, nil
}
