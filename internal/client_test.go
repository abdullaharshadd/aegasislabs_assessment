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

func newTestClient(t *testing.T, handler http.Handler) (*PromptClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewPromptClient(srv.URL, srv.Client()), srv
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// captureOutput redirects os.Stdout by wrapping fmt.Println calls – because
// RunDemo uses fmt.Println we capture via a pipe only where needed.
// For unit tests we only need to verify side-effects (HTTP calls), not stdout.

// ---------------------------------------------------------------------------
// NewPromptClient
// ---------------------------------------------------------------------------

func TestNewPromptClient(t *testing.T) {
	t.Run("nil http client uses DefaultClient", func(t *testing.T) {
		c := NewPromptClient("http://example.com", nil)
		assert.Equal(t, "http://example.com", c.baseURL)
		assert.Equal(t, http.DefaultClient, c.http)
	})

	t.Run("provided http client is used", func(t *testing.T) {
		custom := &http.Client{}
		c := NewPromptClient("http://example.com", custom)
		assert.Same(t, custom, c.http)
	})
}

// ---------------------------------------------------------------------------
// decodeJSON (tested indirectly through the client methods)
// ---------------------------------------------------------------------------

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "this is not json")
	}))
	defer srv.Close()

	c := NewPromptClient(srv.URL, srv.Client())
	result, err := c.GetResponse(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"error": "Invalid response from the server"}, result)
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	type want struct {
		method      string
		path        string
		requestBody map[string]any
		response    map[string]any
	}

	tests := []struct {
		name           string
		prompt         string
		serverResponse string
		serverStatus   int
		wantResult     map[string]any
		wantErr        bool
	}{
		{
			name:           "valid prompt returns parsed JSON",
			prompt:         "What is life?",
			serverResponse: `{"id": 1, "prompt": "What is life?"}`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"id": float64(1), "prompt": "What is life?"},
		},
		{
			name:           "server returns created status",
			prompt:         "Who are you?",
			serverResponse: `{"id": 2, "prompt": "Who are you?", "status": "created"}`,
			serverStatus:   http.StatusCreated,
			wantResult:     map[string]any{"id": float64(2), "prompt": "Who are you?", "status": "created"},
		},
		{
			name:           "server returns non-JSON body",
			prompt:         "Test",
			serverResponse: `not json`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"error": "Invalid response from the server"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var capturedMethod, capturedPath string
			var capturedBody map[string]any

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path

				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.serverStatus)
				fmt.Fprint(w, tc.serverResponse)
			}))
			defer srv.Close()

			c := NewPromptClient(srv.URL, srv.Client())
			result, err := c.CreatePrompt(context.Background(), tc.prompt)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// verify HTTP contract
			assert.Equal(t, http.MethodPost, capturedMethod)
			assert.Equal(t, "/create", capturedPath)
			assert.Equal(t, map[string]any{"prompt": tc.prompt}, capturedBody)

			// verify result
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func TestCreatePrompt_ContentTypeHeader(t *testing.T) {
	var capturedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok": true}`)
	}))
	defer srv.Close()

	c := NewPromptClient(srv.URL, srv.Client())
	_, err := c.CreatePrompt(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "application/json", capturedContentType)
}

func TestCreatePrompt_RequestBodyKey(t *testing.T) {
	// Invariant: the body always has exactly the key "prompt"
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewPromptClient(srv.URL, srv.Client())
	_, _ = c.CreatePrompt(context.Background(), "sentinel")

	var m map[string]any
	require.NoError(t, json.Unmarshal(rawBody, &m))
	assert.Equal(t, "sentinel", m["prompt"])
	assert.Len(t, m, 1, "body should have exactly one key")
}

// ---------------------------------------------------------------------------
// GetResponse
// ---------------------------------------------------------------------------

func TestGetResponse(t *testing.T) {
	tests := []struct {
		name           string
		promptIndex    int
		serverResponse string
		serverStatus   int
		wantResult     map[string]any
		wantErr        bool
	}{
		{
			name:           "valid index returns parsed JSON",
			promptIndex:    0,
			serverResponse: `{"id": 0, "prompt": "What is life?"}`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"id": float64(0), "prompt": "What is life?"},
		},
		{
			name:           "valid index 5",
			promptIndex:    5,
			serverResponse: `{"id": 5, "prompt": "Another prompt"}`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"id": float64(5), "prompt": "Another prompt"},
		},
		{
			name:           "non-JSON response returns standard error object",
			promptIndex:    0,
			serverResponse: `not valid json`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:           "empty body returns standard error object",
			promptIndex:    1,
			serverResponse: ``,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"error": "Invalid response from the server"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var capturedMethod, capturedPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.serverStatus)
				fmt.Fprint(w, tc.serverResponse)
			}))
			defer srv.Close()

			c := NewPromptClient(srv.URL, srv.Client())
			result, err := c.GetResponse(context.Background(), tc.promptIndex)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, http.MethodGet, capturedMethod)
			assert.Equal(t, fmt.Sprintf("/get/%d", tc.promptIndex), capturedPath)
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func TestGetResponse_NoRequestBody(t *testing.T) {
	// GET should not send a body and should NOT set Content-Type
	var bodyBytes []byte
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ = io.ReadAll(r.Body)
		contentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewPromptClient(srv.URL, srv.Client())
	_, err := c.GetResponse(context.Background(), 3)
	require.NoError(t, err)
	assert.Empty(t, bodyBytes)
	assert.Empty(t, contentType)
}

// ---------------------------------------------------------------------------
// UpdatePrompt
// ---------------------------------------------------------------------------

func TestUpdatePrompt(t *testing.T) {
	tests := []struct {
		name           string
		promptIndex    int
		newPrompt      string
		serverResponse string
		serverStatus   int
		wantResult     map[string]any
		wantErr        bool
	}{
		{
			name:           "valid update returns parsed JSON",
			promptIndex:    1,
			newPrompt:      "Who is Goku?",
			serverResponse: `{"id": 1, "prompt": "Who is Goku?", "status": "updated"}`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"id": float64(1), "prompt": "Who is Goku?", "status": "updated"},
		},
		{
			name:           "update index 0",
			promptIndex:    0,
			newPrompt:      "New prompt text",
			serverResponse: `{"message": "updated successfully"}`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"message": "updated successfully"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var capturedMethod, capturedPath string
			var capturedBody map[string]any

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &capturedBody)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.serverStatus)
				fmt.Fprint(w, tc.serverResponse)
			}))
			defer srv.Close()

			c := NewPromptClient(srv.URL, srv.Client())
			result, err := c.UpdatePrompt(context.Background(), tc.promptIndex, tc.newPrompt)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, http.MethodPut, capturedMethod)
			assert.Equal(t, fmt.Sprintf("/update/%d", tc.promptIndex), capturedPath)
			assert.Equal(t, map[string]any{"new_prompt": tc.newPrompt}, capturedBody)
			assert.Equal(t, tc.wantResult, result)
		})
	}
}

func TestUpdatePrompt_RequestBodyKey(t *testing.T) {
	// Invariant: body has exactly key "new_prompt"
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewPromptClient(srv.URL, srv.Client())
	_, _ = c.UpdatePrompt(context.Background(), 2, "updated text")

	var m map[string]any
	require.NoError(t, json.Unmarshal(rawBody, &m))
	assert.Equal(t, "updated text", m["new_prompt"])
	assert.Len(t, m, 1, "body should have exactly one key")
}

func TestUpdatePrompt_ContentTypeHeader(t *testing.T) {
	var capturedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c := NewPromptClient(srv.URL, srv.Client())
	_, err := c.UpdatePrompt(context.Background(), 1, "new")
	require.NoError(t, err)
	assert.Equal(t, "application/json", capturedContentType)
}

// ---------------------------------------------------------------------------
// DeletePrompt
// ---------------------------------------------------------------------------

func TestDeletePrompt(t *testing.T) {
	tests := []struct {
		name           string
		promptIndex    int
		serverResponse string
		serverStatus   int
		wantResult     map[string]any
		wantErr        bool
	}{
		{
			name:           "valid delete returns parsed JSON",
			promptIndex:    0,
			serverResponse: `{"message": "deleted", "id": 0}`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"message": "deleted", "id": float64(0)},
		},
		{
			name:           "delete index 3",
			promptIndex:    3,
			serverResponse: `{"status": "ok"}`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]any{"status": "ok"},
		},
		{
			name:           "non-JSON response returns standard error object",
			promptIndex:    0,
			serverResponse: `not valid json`,
			serverStatus:   http.StatusOK,
			wantResult:     map[string]