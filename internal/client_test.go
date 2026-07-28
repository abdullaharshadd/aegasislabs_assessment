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

// jsonHandler returns an httptest handler that responds with the given status
// code and JSON-encodes body as the response payload.
func jsonHandler(t *testing.T, statusCode int, body any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("jsonHandler: encode: %v", err)
		}
	}
}

// rawHandler returns an httptest handler that writes raw bytes verbatim.
func rawHandler(statusCode int, payload string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		fmt.Fprint(w, payload)
	}
}

// decodeBody is a test helper that reads and JSON-decodes an http.Request body.
func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
	return m
}

// newTestClient builds a Client wired to the supplied httptest.Server.
func newTestClient(srv *httptest.Server) *Client {
	return NewClient(srv.URL, srv.Client())
}

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	t.Run("empty baseURL falls back to DefaultBaseURL", func(t *testing.T) {
		c := NewClient("", nil)
		assert.Equal(t, DefaultBaseURL, c.baseURL)
	})

	t.Run("nil httpClient falls back to http.DefaultClient", func(t *testing.T) {
		c := NewClient("http://example.com", nil)
		assert.Equal(t, http.DefaultClient, c.hc)
	})

	t.Run("explicit values are preserved", func(t *testing.T) {
		hc := &http.Client{}
		c := NewClient("http://custom.host", hc)
		assert.Equal(t, "http://custom.host", c.baseURL)
		assert.Equal(t, hc, c.hc)
	})
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		serverStatus   int
		serverBody     any
		serverRaw      string // non-empty means use rawHandler instead of jsonHandler
		wantResult     map[string]any
		wantErrContain string
	}{
		{
			name:         "valid prompt returns parsed JSON",
			prompt:       "What is life?",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"id": float64(0), "prompt": "What is life?"},
			wantResult:   map[string]any{"id": float64(0), "prompt": "What is life?"},
		},
		{
			name:         "server returns created status",
			prompt:       "Hello world",
			serverStatus: http.StatusCreated,
			serverBody:   map[string]any{"message": "created", "index": float64(1)},
			wantResult:   map[string]any{"message": "created", "index": float64(1)},
		},
		{
			name:         "server returns error JSON still parsed",
			prompt:       "bad",
			serverStatus: http.StatusBadRequest,
			serverBody:   map[string]any{"error": "missing field"},
			wantResult:   map[string]any{"error": "missing field"},
		},
		{
			name:         "non-JSON server response returns invalid-response map",
			prompt:       "test",
			serverStatus: http.StatusInternalServerError,
			serverRaw:    "<html>Internal Server Error</html>",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedBody map[string]any

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/create", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				capturedBody = decodeBody(t, r)

				if tc.serverRaw != "" {
					rawHandler(tc.serverStatus, tc.serverRaw)(w, r)
					return
				}
				jsonHandler(t, tc.serverStatus, tc.serverBody)(w, r)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			result, err := c.CreatePrompt(context.Background(), tc.prompt)

			if tc.wantErrContain != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContain)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)

			// invariant: body always has key "prompt"
			if capturedBody != nil {
				assert.Equal(t, tc.prompt, capturedBody["prompt"])
				assert.Len(t, capturedBody, 1, "request body should only contain 'prompt'")
			}
		})
	}
}

func TestCreatePrompt_RequestMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method, "CreatePrompt must use POST")
		assert.Equal(t, "/create", r.URL.Path)
		jsonHandler(t, http.StatusOK, map[string]any{"ok": true})(w, r)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.CreatePrompt(context.Background(), "test")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// GetResponse
// ---------------------------------------------------------------------------

func TestGetResponse(t *testing.T) {
	tests := []struct {
		name         string
		index        int
		serverStatus int
		serverBody   any
		serverRaw    string
		wantResult   map[string]any
	}{
		{
			name:         "valid index returns parsed JSON",
			index:        0,
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"prompt": "What is life?", "index": float64(0)},
			wantResult:   map[string]any{"prompt": "What is life?", "index": float64(0)},
		},
		{
			name:         "index 5 targets correct endpoint",
			index:        5,
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"prompt": "hello", "index": float64(5)},
			wantResult:   map[string]any{"prompt": "hello", "index": float64(5)},
		},
		{
			name:         "non-JSON response returns invalid-response map",
			index:        99,
			serverStatus: http.StatusNotFound,
			serverRaw:    "404 page not found",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:         "HTML 500 response returns invalid-response map",
			index:        0,
			serverStatus: http.StatusInternalServerError,
			serverRaw:    "<html>Error</html>",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:         "server returns JSON error (invalid index) — passed through",
			index:        999,
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"error": "invalid index"},
			wantResult:   map[string]any{"error": "invalid index"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectedPath := fmt.Sprintf("/get/%d", tc.index)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, expectedPath, r.URL.Path)

				if tc.serverRaw != "" {
					rawHandler(tc.serverStatus, tc.serverRaw)(w, r)
					return
				}
				jsonHandler(t, tc.serverStatus, tc.serverBody)(w, r)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			result, err := c.GetResponse(context.Background(), tc.index)

			require.NoError(t, err, "GetResponse should never return an error for JSON-decode failures")
			assert.Equal(t, tc.wantResult, result)
			assert.NotNil(t, result, "result must always be a non-nil map")
		})
	}
}

func TestGetResponse_NoRequestBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		body, _ := io.ReadAll(r.Body)
		assert.Empty(t, body, "GET request should have no body")
		jsonHandler(t, http.StatusOK, map[string]any{"ok": true})(w, r)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetResponse(context.Background(), 0)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// UpdatePrompt
// ---------------------------------------------------------------------------

func TestUpdatePrompt(t *testing.T) {
	tests := []struct {
		name           string
		index          int
		newPrompt      string
		serverStatus   int
		serverBody     any
		serverRaw      string
		wantResult     map[string]any
		wantErrContain string
	}{
		{
			name:         "valid update returns parsed JSON",
			index:        1,
			newPrompt:    "Who is Goku?",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"message": "updated", "index": float64(1)},
			wantResult:   map[string]any{"message": "updated", "index": float64(1)},
		},
		{
			name:         "index 0 update",
			index:        0,
			newPrompt:    "New prompt here",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"message": "updated"},
			wantResult:   map[string]any{"message": "updated"},
		},
		{
			name:         "server error JSON passed through",
			index:        5,
			newPrompt:    "prompt",
			serverStatus: http.StatusBadRequest,
			serverBody:   map[string]any{"error": "index out of range"},
			wantResult:   map[string]any{"error": "index out of range"},
		},
		{
			name:         "non-JSON response returns invalid-response map",
			index:        2,
			newPrompt:    "test",
			serverStatus: http.StatusInternalServerError,
			serverRaw:    "server error",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectedPath := fmt.Sprintf("/update/%d", tc.index)
			var capturedBody map[string]any

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, expectedPath, r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				capturedBody = decodeBody(t, r)

				if tc.serverRaw != "" {
					rawHandler(tc.serverStatus, tc.serverRaw)(w, r)
					return
				}
				jsonHandler(t, tc.serverStatus, tc.serverBody)(w, r)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			result, err := c.UpdatePrompt(context.Background(), tc.index, tc.newPrompt)

			if tc.wantErrContain != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContain)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, result)

			// invariant: body always has exactly key "new_prompt"
			if capturedBody != nil {
				assert.Equal(t, tc.newPrompt, capturedBody["new_prompt"])
				assert.Len(t, capturedBody, 1, "request body should only contain 'new_prompt'")
			}
		})
	}
}

func TestUpdatePrompt_RequestMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method, "UpdatePrompt must use PUT")
		jsonHandler(t, http.StatusOK, map[string]any{"ok": true})(w, r)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.UpdatePrompt(context.Background(), 0, "hello")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// DeletePrompt
// ---------------------------------------------------------------------------

func TestDeletePrompt(t *testing.T) {
	tests := []struct {
		name         string
		index        int
		serverStatus int
		serverBody   any
		serverRaw    string
		wantResult   map[string]any
	}{
		{
			name:         "valid index returns parsed JSON",
			index:        0,
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"message": "deleted", "index": float64(0)},
			wantResult:   map[string]any{"message": "deleted", "index": float64(0)},
		},
		{
			name:         "index 3 targets correct endpoint",
			index:        3,
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"message": "deleted"},
			wantResult:   map[string]any{"message": "deleted"},
		},
		{
			name:         "non-JSON response returns invalid-response map",
			index:        0,
			serverStatus: http.StatusNotFound,
			serverRaw:    "Not Found",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:         "HTML 500 response returns invalid-response map",
			index:        0,
			serverStatus: http.StatusIn