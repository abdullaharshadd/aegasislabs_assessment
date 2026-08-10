// Package client provides an HTTP client for consuming the prompts CRUD REST API.
//
// MIGRATION_NOTE: The original client.py was a standalone script that consumed an
// external CRUD API using the `requests` library. It has been migrated into a
// reusable Client type with (T, error) return values and context propagation.
// The original module-level BASE_URL default (http://127.0.0.1:5000) is preserved
// as DefaultBaseURL, but the base URL is now injectable via NewClient.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the base URL of the prompts API when none is supplied.
//
// MIGRATION_NOTE: The source hard-coded the local API server address. In an
// idiomatic setup this should come from configuration (see internal/config),
// but the literal default is kept here to preserve behaviour.
const DefaultBaseURL = "http://127.0.0.1:5000"

// Client is an HTTP client for the prompts CRUD API.
type Client struct {
	baseURL string
	http    *http.Client
}

// PromptResponse represents a decoded JSON response from the prompts API.
//
// MIGRATION_NOTE: The Python client returned raw dicts of arbitrary shape
// (response.json()). Since the exact server contract is not statically known
// from this file, responses are decoded into a generic map. The Error field is
// populated locally when the server returns a non-JSON body, mirroring the
// original try/except requests.exceptions.JSONDecodeError fallback.
type PromptResponse map[string]any

// NewClient constructs a Client targeting the given base URL. If baseURL is
// empty, DefaultBaseURL is used. A nil httpClient falls back to a sensible
// default with a timeout.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL: baseURL,
		http:    httpClient,
	}
}

// CreatePrompt sends a new prompt to the API's /create endpoint and returns the
// decoded response.
func (c *Client) CreatePrompt(ctx context.Context, prompt string) (PromptResponse, error) {
	body := map[string]string{"prompt": prompt}
	return c.doJSON(ctx, http.MethodPost, "/create", body, false)
}

// GetResponse fetches the prompt/response at the given index from the API's
// /get/{index} endpoint.
//
// MIGRATION_NOTE: The Python version swallowed JSONDecodeError and returned
// {"error": "Invalid response from the server"}. That fallback is preserved by
// passing tolerateNonJSON=true, so callers still receive a PromptResponse
// (with an "error" key) rather than a hard error on a non-JSON body.
func (c *Client) GetResponse(ctx context.Context, promptIndex int) (PromptResponse, error) {
	endpoint := fmt.Sprintf("/get/%d", promptIndex)
	return c.doJSON(ctx, http.MethodGet, endpoint, nil, true)
}

// UpdatePrompt replaces the prompt at the given index via the API's
// /update/{index} endpoint.
func (c *Client) UpdatePrompt(ctx context.Context, promptIndex int, newPrompt string) (PromptResponse, error) {
	endpoint := fmt.Sprintf("/update/%d", promptIndex)
	body := map[string]string{"new_prompt": newPrompt}
	return c.doJSON(ctx, http.MethodPut, endpoint, body, false)
}

// DeletePrompt removes the prompt at the given index via the API's
// /delete/{index} endpoint.
//
// MIGRATION_NOTE: Like GetResponse, the original tolerated non-JSON responses
// and returned an error map; that behaviour is preserved via tolerateNonJSON.
func (c *Client) DeletePrompt(ctx context.Context, promptIndex int) (PromptResponse, error) {
	endpoint := fmt.Sprintf("/delete/%d", promptIndex)
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil, true)
}

// doJSON performs an HTTP request against the base URL, optionally encoding the
// given body as JSON, and decodes the response body into a PromptResponse.
//
// When tolerateNonJSON is true, a body that fails JSON decoding is not treated
// as a fatal error; instead a PromptResponse containing an "error" key is
// returned, matching the source's try/except behaviour.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body any, tolerateNonJSON bool) (PromptResponse, error) {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body for %s: %w", endpoint, err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", endpoint, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing %s %s: %w", method, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body for %s: %w", endpoint, err)
	}

	var out PromptResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		if tolerateNonJSON {
			return PromptResponse{"error": "Invalid response from the server"}, nil
		}
		return nil, fmt.Errorf("decoding response for %s (status %d): %w", endpoint, resp.StatusCode, err)
	}

	return out, nil
}
