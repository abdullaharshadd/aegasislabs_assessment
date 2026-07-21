```go
package client

import (
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

// newTestClient wires a Client to a given httptest.Server.
func newTestClient(srv *httptest.Server) *Client {
	return NewClient(
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
}

// jsonBody is a small helper to write a JSON response to a ResponseWriter.
func jsonBody(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------------------
// NewClient / options
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient()
	assert.Equal(t, DefaultBaseURL, c.baseURL)
	assert.NotNil(t, c.httpClient)
}

func TestNewClient_WithBaseURL(t *testing.T) {
	c := NewClient(WithBaseURL("http://example.com"))
	assert.Equal(t, "http://example.com", c.baseURL)
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	custom := &http.Client{}
	c := NewClient(WithHTTPClient(custom))
	assert.Same(t, custom, c.httpClient)
}

func TestNewClient_WithHTTPClient_NilIgnored(t *testing.T) {
	c := NewClient(WithHTTPClient(nil))
	assert.NotNil(t, c.httpClient)
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	type want struct {
		method      string
		path        string
		bodyKey     string
		bodyValue   string
		contentType string
	}

	tests := []struct {
		name           string
		prompt         string
		serverResponse any
		serverStatus   int
		serverRaw      string // if non-empty, write raw bytes instead of JSON
		wantResponse   Response
		wantErr        bool
		wantCapture    want
	}{
		{
			name:           "valid prompt returns server JSON",
			prompt:         "What is life?",
			serverResponse: map[string]any{"id": float64(1), "message": "created"},
			serverStatus:   http.StatusCreated,
			wantResponse:   Response{"id": float64(1), "message": "created"},
			wantCapture: want{
				method:      http.MethodPost,
				path:        "/create",
				bodyKey:     "prompt",
				bodyValue:   "What is life?",
				contentType: "application/json",
			},
		},
		{
			name:           "second prompt gets different id",
			prompt:         "What is the capital of Pakistan?",
			serverResponse: map[string]any{"id": float64(2), "message": "created"},
			serverStatus:   http.StatusCreated,
			wantResponse:   Response{"id": float64(2), "message": "created"},
			wantCapture: want{
				method:    http.MethodPost,
				path:      "/create",
				bodyKey:   "prompt",
				bodyValue: "What is the capital of Pakistan?",
			},
		},
		{
			name:           "server returns empty object",
			prompt:         "hello",
			serverResponse: map[string]any{},
			serverStatus:   http.StatusOK,
			wantResponse:   Response{},
		},
		{
			name:         "server returns non-JSON – returns error map (create does NOT special-case invalid JSON per spec, but doJSON always handles it)",
			prompt:       "test",
			serverRaw:    "not-json",
			serverStatus: http.StatusOK,
			// doJSON always returns the error map for bad JSON
			wantResponse: Response{"error": "Invalid response from the server"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedMethod, capturedPath, capturedContentType string
			var capturedBody map[string]any

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedContentType = r.Header.Get("Content-Type")

				bodyBytes, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(bodyBytes, &capturedBody)

				if tc.serverRaw != "" {
					w.WriteHeader(tc.serverStatus)
					_, _ = w.Write([]byte(tc.serverRaw))
					return
				}
				jsonBody(w, tc.serverStatus, tc.serverResponse)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			resp, err := c.CreatePrompt(context.Background(), tc.prompt)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResponse, resp)

			// Verify HTTP contract
			assert.Equal(t, http.MethodPost, capturedMethod, "HTTP method must be POST")
			assert.Equal(t, "/create", capturedPath, "path must be /create")
			if tc.wantCapture.bodyKey != "" {
				assert.Equal(t, tc.wantCapture.bodyValue, capturedBody[tc.wantCapture.bodyKey],
					"request body must contain prompt key")
				assert.Len(t, capturedBody, 1, "request body must have exactly one key")
				assert.Equal(t, "application/json", capturedContentType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetResponse
// ---------------------------------------------------------------------------

func TestGetResponse(t *testing.T) {
	tests := []struct {
		name         string
		promptIndex  int
		serverRaw    string
		serverJSON   any
		serverStatus int
		wantResponse Response
		wantErr      bool
		wantPath     string
		wantMethod   string
	}{
		{
			name:         "valid index returns parsed JSON",
			promptIndex:  0,
			serverJSON:   map[string]any{"id": float64(0), "prompt": "What is life?"},
			serverStatus: http.StatusOK,
			wantResponse: Response{"id": float64(0), "prompt": "What is life?"},
			wantPath:     "/get/0",
			wantMethod:   http.MethodGet,
		},
		{
			name:         "index 1 encodes correctly in path",
			promptIndex:  1,
			serverJSON:   map[string]any{"id": float64(1), "prompt": "Who is Goku?"},
			serverStatus: http.StatusOK,
			wantResponse: Response{"id": float64(1), "prompt": "Who is Goku?"},
			wantPath:     "/get/1",
			wantMethod:   http.MethodGet,
		},
		{
			name:         "non-JSON response returns error map",
			promptIndex:  0,
			serverRaw:    "this is not json at all",
			serverStatus: http.StatusOK,
			wantResponse: Response{"error": "Invalid response from the server"},
			wantPath:     "/get/0",
			wantMethod:   http.MethodGet,
		},
		{
			name:         "server returns 404 with JSON body",
			promptIndex:  99,
			serverJSON:   map[string]any{"error": "not found"},
			serverStatus: http.StatusNotFound,
			wantResponse: Response{"error": "not found"},
			wantPath:     "/get/99",
			wantMethod:   http.MethodGet,
		},
		{
			name:         "always returns a map (empty JSON object)",
			promptIndex:  5,
			serverJSON:   map[string]any{},
			serverStatus: http.StatusOK,
			wantResponse: Response{},
			wantPath:     "/get/5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedMethod, capturedPath string
			var capturedBodyBytes []byte

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedBodyBytes, _ = io.ReadAll(r.Body)

				if tc.serverRaw != "" {
					w.WriteHeader(tc.serverStatus)
					_, _ = w.Write([]byte(tc.serverRaw))
					return
				}
				jsonBody(w, tc.serverStatus, tc.serverJSON)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			resp, err := c.GetResponse(context.Background(), tc.promptIndex)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResponse, resp)

			// HTTP contract invariants
			assert.Equal(t, http.MethodGet, capturedMethod, "HTTP method must be GET")
			assert.Equal(t, tc.wantPath, capturedPath)
			assert.Empty(t, capturedBodyBytes, "GET must send no request body")

			// Always returns a map
			assert.NotNil(t, resp)
		})
	}
}

// ---------------------------------------------------------------------------
// UpdatePrompt
// ---------------------------------------------------------------------------

func TestUpdatePrompt(t *testing.T) {
	tests := []struct {
		name         string
		promptIndex  int
		newPrompt    string
		serverJSON   any
		serverRaw    string
		serverStatus int
		wantResponse Response
		wantErr      bool
		wantPath     string
	}{
		{
			name:         "valid index and new prompt returns server JSON",
			promptIndex:  1,
			newPrompt:    "Who is Goku?",
			serverJSON:   map[string]any{"message": "updated", "id": float64(1)},
			serverStatus: http.StatusOK,
			wantResponse: Response{"message": "updated", "id": float64(1)},
			wantPath:     "/update/1",
		},
		{
			name:         "index 0 encodes correctly in path",
			promptIndex:  0,
			newPrompt:    "New text",
			serverJSON:   map[string]any{"message": "updated"},
			serverStatus: http.StatusOK,
			wantResponse: Response{"message": "updated"},
			wantPath:     "/update/0",
		},
		{
			name:         "server returns non-JSON – doJSON returns error map",
			promptIndex:  1,
			newPrompt:    "some prompt",
			serverRaw:    "bad response",
			serverStatus: http.StatusOK,
			wantResponse: Response{"error": "Invalid response from the server"},
			wantPath:     "/update/1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedMethod, capturedPath, capturedCT string
			var capturedBody map[string]any

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedCT = r.Header.Get("Content-Type")

				bodyBytes, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(bodyBytes, &capturedBody)

				if tc.serverRaw != "" {
					w.WriteHeader(tc.serverStatus)
					_, _ = w.Write([]byte(tc.serverRaw))
					return
				}
				jsonBody(w, tc.serverStatus, tc.serverJSON)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			resp, err := c.UpdatePrompt(context.Background(), tc.promptIndex, tc.newPrompt)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResponse, resp)

			// HTTP contract invariants
			assert.Equal(t, http.MethodPut, capturedMethod, "HTTP method must be PUT")
			assert.Equal(t, tc.wantPath, capturedPath)
			assert.Equal(t, "application/json", capturedCT)

			if tc.serverRaw == "" {
				// body must have exactly the new_prompt key
				assert.Equal(t, tc.newPrompt, capturedBody["new_prompt"])
				assert.Len(t, capturedBody, 1, "request body must have exactly one key: new_prompt")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DeletePrompt
// ---------------------------------------------------------------------------

func TestDeletePrompt(t *testing.T) {
	tests := []struct {
		name         string
		promptIndex  int
		serverJSON   any
		serverRaw    string
		serverStatus int
		wantResponse Response
		wantErr      bool
		wantPath     string
	}{
		{
			name:         "valid index returns parsed JSON",
			promptIndex:  0,
			serverJSON:   map[string]any{"message": "deleted"},
			serverStatus: http.StatusOK,
			wantResponse: Response{"message": "deleted"},
			wantPath:     "/delete/0",
		},
		{
			name:         "non-JSON response returns error map",
			promptIndex:  0,
			serverRaw:    "not valid json",
			serverStatus: http.StatusOK,
			wantResponse: Response{"error": "Invalid response from the server"},
			wantPath:     "/delete/0",
		},
		{
			name:         "index 3 encodes correctly in path",
			promptIndex:  3,
			serverJSON:   map[string]any{"message": "deleted", "id": float64(3)},
			serverStatus: http.StatusOK,
			wantResponse: Response{"message": "deleted", "id": float64(3)},
			wantPath:     "/delete/3",
		},
		{
			name:         "always returns a map (empty JSON object)",
			promptIndex:  7,
			serverJSON:   map[string]any{},
			serverStatus: http.StatusOK,
			wantResponse: Response{},
			wantPath:     "/delete/7",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedMethod, capturedPath string
			var capturedBodyBytes []byte

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedMethod = r.Method
				capturedPath = r.URL.Path
				capturedBodyBytes, _ = io.ReadAll(r.Body)

				if tc.serverRaw != "" {
					w.WriteHeader(tc.serverStatus)
					_, _ = w.Write([]byte(tc.serverRaw))
					return
				}
				jsonBody(w, tc.serverStatus, tc.serverJSON)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			resp, err := c.DeletePrompt(context.Background(), tc.promptIndex)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResponse