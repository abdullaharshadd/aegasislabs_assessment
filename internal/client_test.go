```go
package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestClient builds a Client whose baseURL points at the given test server.
func newTestClient(server *httptest.Server) *Client {
	return NewClient(server.URL, server.Client())
}

// jsonBody serialises v and returns it as an io.Reader.
func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("empty baseURL uses DefaultBaseURL", func(t *testing.T) {
		c := NewClient("", nil)
		assert.Equal(t, DefaultBaseURL, c.baseURL)
		assert.Equal(t, http.DefaultClient, c.httpClient)
	})

	t.Run("provided baseURL is kept", func(t *testing.T) {
		c := NewClient("http://example.com", nil)
		assert.Equal(t, "http://example.com", c.baseURL)
	})

	t.Run("provided httpClient is kept", func(t *testing.T) {
		custom := &http.Client{}
		c := NewClient("", custom)
		assert.Same(t, custom, c.httpClient)
	})
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		prompt         string
		serverResponse string
		serverStatus   int
		wantResult     APIResponse
		wantErr        bool
	}{
		{
			name:           "valid prompt returns parsed JSON response",
			prompt:         "What is life?",
			serverResponse: `{"id":1,"prompt":"What is life?","status":"created"}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"id": float64(1), "prompt": "What is life?", "status": "created"},
		},
		{
			name:           "server returns non-JSON body",
			prompt:         "bad server",
			serverResponse: `not json at all`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"error": "Invalid response from the server"},
		},
		{
			name:           "server returns empty JSON object",
			prompt:         "empty",
			serverResponse: `{}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var capturedMethod string
			var capturedPath string
			var capturedBody map[string]any
			var capturedContentType string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedContentType = r.Header.Get("Content-Type")

				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)

				w.WriteHeader(tc.serverStatus)
				_, _ = fmt.Fprint(w, tc.serverResponse)
			}))
			defer server.Close()

			c := newTestClient(server)
			result, err := c.CreatePrompt(context.Background(), tc.prompt)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)

			// Invariants: method=POST, path=/create, body has "prompt" key
			assert.Equal(t, http.MethodPost, capturedMethod)
			assert.Equal(t, "/create", capturedPath)
			assert.Equal(t, "application/json", capturedContentType)
			require.NotNil(t, capturedBody)
			assert.Equal(t, tc.prompt, capturedBody["prompt"])
			assert.Len(t, capturedBody, 1, "body must contain exactly one key: 'prompt'")
		})
	}
}

// ---------------------------------------------------------------------------
// GetResponse
// ---------------------------------------------------------------------------

func TestGetResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		promptIndex    int
		serverResponse string
		serverStatus   int
		wantResult     APIResponse
		wantErr        bool
	}{
		{
			name:           "existing prompt returns parsed JSON",
			promptIndex:    0,
			serverResponse: `{"id":0,"prompt":"What is life?"}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"id": float64(0), "prompt": "What is life?"},
		},
		{
			name:           "non-JSON body returns error object",
			promptIndex:    2,
			serverResponse: `this is not json`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"error": "Invalid response from the server"},
		},
		{
			name:           "index embedded correctly in path",
			promptIndex:    42,
			serverResponse: `{"id":42}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"id": float64(42)},
		},
		{
			name:           "empty JSON object response",
			promptIndex:    99,
			serverResponse: `{}`,
			serverStatus:   http.StatusNotFound,
			wantResult:     APIResponse{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var capturedMethod string
			var capturedPath string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				w.WriteHeader(tc.serverStatus)
				_, _ = fmt.Fprint(w, tc.serverResponse)
			}))
			defer server.Close()

			c := newTestClient(server)
			result, err := c.GetResponse(context.Background(), tc.promptIndex)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)

			// Invariants: method=GET, path=/get/<index>
			assert.Equal(t, http.MethodGet, capturedMethod)
			assert.Equal(t, fmt.Sprintf("/get/%d", tc.promptIndex), capturedPath)
		})
	}
}

// TestGetResponse_NeverRaisesOnInvalidJSON specifically validates the invariant
// that GetResponse always returns an object and never errors on invalid JSON.
func TestGetResponse_NeverRaisesOnInvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{broken json`)
	}))
	defer server.Close()

	c := newTestClient(server)
	result, err := c.GetResponse(context.Background(), 0)

	assert.NoError(t, err, "GetResponse must not return an error on invalid JSON")
	assert.Equal(t, APIResponse{"error": "Invalid response from the server"}, result)
}

// ---------------------------------------------------------------------------
// UpdatePrompt
// ---------------------------------------------------------------------------

func TestUpdatePrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		promptIndex    int
		newPrompt      string
		serverResponse string
		serverStatus   int
		wantResult     APIResponse
		wantErr        bool
	}{
		{
			name:           "valid index and new prompt returns parsed JSON",
			promptIndex:    1,
			newPrompt:      "Who is Goku?",
			serverResponse: `{"id":1,"prompt":"Who is Goku?","status":"updated"}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"id": float64(1), "prompt": "Who is Goku?", "status": "updated"},
		},
		{
			name:           "non-JSON body returns error object",
			promptIndex:    0,
			newPrompt:      "something",
			serverResponse: `not valid json`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"error": "Invalid response from the server"},
		},
		{
			name:           "index embedded correctly in path",
			promptIndex:    7,
			newPrompt:      "Test prompt",
			serverResponse: `{"updated":true}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"updated": true},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var capturedMethod string
			var capturedPath string
			var capturedBody map[string]any
			var capturedContentType string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedContentType = r.Header.Get("Content-Type")

				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)

				w.WriteHeader(tc.serverStatus)
				_, _ = fmt.Fprint(w, tc.serverResponse)
			}))
			defer server.Close()

			c := newTestClient(server)
			result, err := c.UpdatePrompt(context.Background(), tc.promptIndex, tc.newPrompt)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)

			// Invariants: method=PUT, path=/update/<index>, body has "new_prompt" key only
			assert.Equal(t, http.MethodPut, capturedMethod)
			assert.Equal(t, fmt.Sprintf("/update/%d", tc.promptIndex), capturedPath)
			assert.Equal(t, "application/json", capturedContentType)
			require.NotNil(t, capturedBody)
			assert.Equal(t, tc.newPrompt, capturedBody["new_prompt"])
			assert.Len(t, capturedBody, 1, "body must contain exactly one key: 'new_prompt'")
		})
	}
}

// ---------------------------------------------------------------------------
// DeletePrompt
// ---------------------------------------------------------------------------

func TestDeletePrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		promptIndex    int
		serverResponse string
		serverStatus   int
		wantResult     APIResponse
		wantErr        bool
	}{
		{
			name:           "valid index with JSON response returns parsed body",
			promptIndex:    0,
			serverResponse: `{"status":"deleted","id":0}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"status": "deleted", "id": float64(0)},
		},
		{
			name:           "non-JSON body returns error object",
			promptIndex:    3,
			serverResponse: `not valid json`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"error": "Invalid response from the server"},
		},
		{
			name:           "index embedded correctly in path",
			promptIndex:    15,
			serverResponse: `{"ok":true}`,
			serverStatus:   http.StatusOK,
			wantResult:     APIResponse{"ok": true},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var capturedMethod string
			var capturedPath string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				w.WriteHeader(tc.serverStatus)
				_, _ = fmt.Fprint(w, tc.serverResponse)
			}))
			defer server.Close()

			c := newTestClient(server)
			result, err := c.DeletePrompt(context.Background(), tc.promptIndex)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)

			// Invariants: method=DELETE, path=/delete/<index>
			assert.Equal(t, http.MethodDelete, capturedMethod)
			assert.Equal(t, fmt.Sprintf("/delete/%d", tc.promptIndex), capturedPath)
		})
	}
}

// TestDeletePrompt_NeverRaisesOnInvalidJSON specifically validates the invariant
// that DeletePrompt always returns an object and never errors on invalid JSON.
func TestDeletePrompt_NeverRaisesOnInvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{broken json`)
	}))
	defer server.Close()

	c := newTestClient(server)
	result, err := c.DeletePrompt(context.Background(), 0)

	assert.NoError(t, err, "DeletePrompt must not return an error on invalid JSON")
	assert.Equal(t, APIResponse{"error": "Invalid response from the server"}, result)
}

// ---------------------------------------------------------------------------
// RunDemo
// ---------------------------------------------------------------------------

func TestRunDemo(t *testing.T) {
	t.Parallel()

	// responses keyed by "METHOD /path"
	type routeKey = string
	responses := map[routeKey]string{
		"POST /create":    `{"status":"created"}`,
		"GET /get/0":      `{"id":0,"prompt":"What is life?"}`,
		"GET /get/1":      `{"id":1,"prompt":"Who is Goku?"}`,
		"PUT /update/1":   `{"status":"updated"}`,
		"DELETE /delete/0": `{"status":"deleted"}`,
	}

	// Track the order of calls received.
	var calls []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		calls = append(calls, key)
		body, ok := responses[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()

	c := newTestClient(server)
	var out bytes.Buffer

	err := RunDemo(context.Background(), c, &out)
	require.NoError(t, err)

	// Verify 7 lines of output (