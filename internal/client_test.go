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

// newTestClient returns a Client wired to the given httptest.Server.
func newTestClient(ts *httptest.Server) *Client {
	return NewClient(
		WithBaseURL(ts.URL),
		WithHTTPClient(ts.Client()),
	)
}

// jsonHandler returns an http.HandlerFunc that writes statusCode and marshals
// body as JSON.
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

// plainHandler returns an http.HandlerFunc that writes statusCode and a raw
// (non-JSON) body string.
func plainHandler(statusCode int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = io.WriteString(w, body)
	}
}

// captureHandler wraps inner and records every request that passes through.
type captureHandler struct {
	inner    http.Handler
	requests []*http.Request
	bodies   [][]byte
}

func (ch *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	ch.requests = append(ch.requests, r)
	ch.bodies = append(ch.bodies, raw)
	ch.inner.ServeHTTP(w, r)
}

// ---------------------------------------------------------------------------
// NewClient / Option tests
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient()
	assert.Equal(t, DefaultBaseURL, c.baseURL)
	assert.Equal(t, http.DefaultClient, c.http)
}

func TestNewClient_WithOptions(t *testing.T) {
	custom := &http.Client{}
	c := NewClient(
		WithBaseURL("http://example.com"),
		WithHTTPClient(custom),
	)
	assert.Equal(t, "http://example.com", c.baseURL)
	assert.Equal(t, custom, c.http)
}

// ---------------------------------------------------------------------------
// invalidResponse
// ---------------------------------------------------------------------------

func TestInvalidResponse(t *testing.T) {
	got := invalidResponse()
	assert.Equal(t, map[string]any{"error": "Invalid response from the server"}, got)
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		handlerStatus  int
		handlerBody    any
		rawBody        string // non-empty ⇒ use plainHandler instead
		wantResult     map[string]any
		wantErr        bool
		wantMethod     string
		wantPath       string
		wantReqBodyKey string // JSON key expected in request body
	}{
		{
			name:           "valid prompt returns parsed JSON",
			prompt:         "What is life?",
			handlerStatus:  http.StatusOK,
			handlerBody:    map[string]any{"id": float64(0), "status": "created"},
			wantResult:     map[string]any{"id": float64(0), "status": "created"},
			wantMethod:     http.MethodPost,
			wantPath:       "/create",
			wantReqBodyKey: "prompt",
		},
		{
			name:          "server returns non-JSON falls back to invalidResponse",
			prompt:        "hello",
			rawBody:       "Internal Server Error",
			handlerStatus: http.StatusInternalServerError,
			wantResult:    map[string]any{"error": "Invalid response from the server"},
			wantMethod:    http.MethodPost,
			wantPath:      "/create",
		},
		{
			name:          "server returns empty body falls back to invalidResponse",
			prompt:        "empty",
			rawBody:       "",
			handlerStatus: http.StatusOK,
			wantResult:    map[string]any{"error": "Invalid response from the server"},
			wantMethod:    http.MethodPost,
			wantPath:      "/create",
		},
		{
			name:          "server returns 201 with JSON",
			prompt:        "Another prompt",
			handlerStatus: http.StatusCreated,
			handlerBody:   map[string]any{"id": float64(1)},
			wantResult:    map[string]any{"id": float64(1)},
			wantMethod:    http.MethodPost,
			wantPath:      "/create",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cap := &captureHandler{}

			var inner http.Handler
			if tc.rawBody != "" || (tc.handlerBody == nil && tc.rawBody == "") {
				// use plain handler for non-JSON scenarios
				inner = plainHandler(tc.handlerStatus, tc.rawBody)
			} else {
				inner = jsonHandler(t, tc.handlerStatus, tc.handlerBody)
			}
			cap.inner = inner

			ts := httptest.NewServer(cap)
			defer ts.Close()

			c := newTestClient(ts)
			got, err := c.CreatePrompt(context.Background(), tc.prompt)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)

			// validate request shape
			require.Len(t, cap.requests, 1)
			req := cap.requests[0]
			assert.Equal(t, tc.wantMethod, req.Method)
			assert.Equal(t, tc.wantPath, req.URL.Path)
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

			if tc.wantReqBodyKey != "" {
				var reqBody map[string]any
				require.NoError(t, json.Unmarshal(cap.bodies[0], &reqBody))
				assert.Equal(t, tc.prompt, reqBody["prompt"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetResponse
// ---------------------------------------------------------------------------

func TestGetResponse(t *testing.T) {
	tests := []struct {
		name          string
		promptIndex   int
		handlerStatus int
		handlerBody   any
		rawBody       string
		useRaw        bool
		wantResult    map[string]any
		wantErr       bool
		wantMethod    string
		wantPath      string
		wantNoBody    bool // GET should send no request body
	}{
		{
			name:          "valid index returns parsed JSON",
			promptIndex:   0,
			handlerStatus: http.StatusOK,
			handlerBody:   map[string]any{"prompt": "What is life?", "response": "42"},
			wantResult:    map[string]any{"prompt": "What is life?", "response": "42"},
			wantMethod:    http.MethodGet,
			wantPath:      "/get/0",
			wantNoBody:    true,
		},
		{
			name:          "index 3 maps to correct path",
			promptIndex:   3,
			handlerStatus: http.StatusOK,
			handlerBody:   map[string]any{"index": float64(3)},
			wantResult:    map[string]any{"index": float64(3)},
			wantMethod:    http.MethodGet,
			wantPath:      "/get/3",
			wantNoBody:    true,
		},
		{
			name:          "non-JSON response returns invalidResponse fallback",
			promptIndex:   1,
			useRaw:        true,
			rawBody:       "not json at all",
			handlerStatus: http.StatusInternalServerError,
			wantResult:    map[string]any{"error": "Invalid response from the server"},
			wantMethod:    http.MethodGet,
			wantPath:      "/get/1",
		},
		{
			name:          "HTML response returns invalidResponse fallback",
			promptIndex:   2,
			useRaw:        true,
			rawBody:       "<html><body>error</body></html>",
			handlerStatus: http.StatusOK,
			wantResult:    map[string]any{"error": "Invalid response from the server"},
			wantMethod:    http.MethodGet,
			wantPath:      "/get/2",
		},
		{
			name:          "server returns error JSON",
			promptIndex:   99,
			handlerStatus: http.StatusNotFound,
			handlerBody:   map[string]any{"error": "not found"},
			wantResult:    map[string]any{"error": "not found"},
			wantMethod:    http.MethodGet,
			wantPath:      "/get/99",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cap := &captureHandler{}

			var inner http.Handler
			if tc.useRaw {
				inner = plainHandler(tc.handlerStatus, tc.rawBody)
			} else {
				inner = jsonHandler(t, tc.handlerStatus, tc.handlerBody)
			}
			cap.inner = inner

			ts := httptest.NewServer(cap)
			defer ts.Close()

			c := newTestClient(ts)
			got, err := c.GetResponse(context.Background(), tc.promptIndex)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)

			require.Len(t, cap.requests, 1)
			req := cap.requests[0]
			assert.Equal(t, tc.wantMethod, req.Method)
			assert.Equal(t, tc.wantPath, req.URL.Path)

			if tc.wantNoBody {
				assert.Empty(t, cap.bodies[0], "GET request should not have a body")
				assert.Empty(t, req.Header.Get("Content-Type"))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdatePrompt
// ---------------------------------------------------------------------------

func TestUpdatePrompt(t *testing.T) {
	tests := []struct {
		name          string
		promptIndex   int
		newPrompt     string
		handlerStatus int
		handlerBody   any
		rawBody       string
		useRaw        bool
		wantResult    map[string]any
		wantErr       bool
		wantMethod    string
		wantPath      string
	}{
		{
			name:          "valid update returns parsed JSON",
			promptIndex:   1,
			newPrompt:     "Who is Goku?",
			handlerStatus: http.StatusOK,
			handlerBody:   map[string]any{"status": "updated"},
			wantResult:    map[string]any{"status": "updated"},
			wantMethod:    http.MethodPut,
			wantPath:      "/update/1",
		},
		{
			name:          "index 0 maps to correct path",
			promptIndex:   0,
			newPrompt:     "New text",
			handlerStatus: http.StatusOK,
			handlerBody:   map[string]any{"ok": true},
			wantResult:    map[string]any{"ok": true},
			wantMethod:    http.MethodPut,
			wantPath:      "/update/0",
		},
		{
			name:          "non-JSON response returns invalidResponse fallback",
			promptIndex:   5,
			newPrompt:     "test",
			useRaw:        true,
			rawBody:       "server error",
			handlerStatus: http.StatusInternalServerError,
			wantResult:    map[string]any{"error": "Invalid response from the server"},
			wantMethod:    http.MethodPut,
			wantPath:      "/update/5",
		},
		{
			name:          "server returns 404 JSON error",
			promptIndex:   42,
			newPrompt:     "anything",
			handlerStatus: http.StatusNotFound,
			handlerBody:   map[string]any{"error": "prompt not found"},
			wantResult:    map[string]any{"error": "prompt not found"},
			wantMethod:    http.MethodPut,
			wantPath:      "/update/42",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cap := &captureHandler{}

			var inner http.Handler
			if tc.useRaw {
				inner = plainHandler(tc.handlerStatus, tc.rawBody)
			} else {
				inner = jsonHandler(t, tc.handlerStatus, tc.handlerBody)
			}
			cap.inner = inner

			ts := httptest.NewServer(cap)
			defer ts.Close()

			c := newTestClient(ts)
			got, err := c.UpdatePrompt(context.Background(), tc.promptIndex, tc.newPrompt)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)

			require.Len(t, cap.requests, 1)
			req := cap.requests[0]
			assert.Equal(t, tc.wantMethod, req.Method)
			assert.Equal(t, tc.wantPath, req.URL.Path)
			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

			var reqBody map[string]any
			require.NoError(t, json.Unmarshal(cap.bodies[0], &reqBody))
			assert.Equal(t, tc.newPrompt, reqBody["new_prompt"])
		})
	}
}

// ---------------------------------------------------------------------------
// DeletePrompt
// ---------------------------------------------------------------------------

func TestDeletePrompt(t *testing.T) {
	tests := []struct {
		name          string
		promptIndex   int
		handlerStatus int
		handlerBody   any
		rawBody       string
		useRaw        bool
		wantResult    map[string]any
		wantErr       bool
		wantMethod    string
		wantPath      string
		wantNoBody    bool
	}{
		{
			name:          "valid delete returns parsed JSON",
			promptIndex:   0,
			handlerStatus: http.StatusOK,
			handlerBody:   map[string]any{"status": "deleted"},
			wantResult:    map[string]any{"status": "deleted"},
			wantMethod:    http.MethodDelete,
			wantPath:      "/delete/0",
			wantNoBody:    true,
		},
		{
			name:          "index 7 maps to correct path",
			promptIndex:   7,
			handlerStatus: http.StatusOK,
			handlerBody:   map[string]any{"id": float64(7)},
			wantResult:    map[string]any{"id": float64(7)},
			wantMethod:    http.MethodDelete,
			wantPath:      "/delete/7",
			wantNoBody:    true,
		},
		{
			name:          "non-JSON response returns invalidResponse fallback",