```go
package internal

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

// newTestClient returns a Client whose baseURL points at the given httptest
// server and whose http.Client talks to that server.
func newTestClient(server *httptest.Server) *Client {
	return NewClient(server.URL, server.Client())
}

// jsonHandler returns an http.HandlerFunc that responds with statusCode and
// marshals body to JSON.
func jsonHandler(t *testing.T, statusCode int, body map[string]any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(body)
	}
}

// plainHandler returns an http.HandlerFunc that responds with a raw (non-JSON)
// string body.
func plainHandler(statusCode int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = io.WriteString(w, body)
	}
}

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("", nil)
	assert.Equal(t, DefaultBaseURL, c.baseURL)
	assert.Equal(t, http.DefaultClient, c.http)
}

func TestNewClient_CustomValues(t *testing.T) {
	custom := &http.Client{}
	c := NewClient("http://example.com", custom)
	assert.Equal(t, "http://example.com", c.baseURL)
	assert.Equal(t, custom, c.http)
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		serverStatus   int
		serverBody     map[string]any
		wantResult     map[string]any
		wantErr        bool
		validateReq    func(t *testing.T, r *http.Request)
	}{
		{
			name:         "valid prompt returns decoded JSON",
			prompt:       "What is life?",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"id": float64(0), "status": "created"},
			wantResult:   map[string]any{"id": float64(0), "status": "created"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/create", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				var got map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
				assert.Equal(t, "What is life?", got["prompt"])
			},
		},
		{
			name:         "prompt key is present in request body",
			prompt:       "Hello world",
			serverStatus: http.StatusCreated,
			serverBody:   map[string]any{"message": "ok"},
			wantResult:   map[string]any{"message": "ok"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				var got map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
				assert.Equal(t, "Hello world", got["prompt"])
			},
		},
		{
			name:         "empty prompt string still sends prompt key",
			prompt:       "",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"result": "empty"},
			wantResult:   map[string]any{"result": "empty"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				var got map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
				_, hasKey := got["prompt"]
				assert.True(t, hasKey, "request body should contain 'prompt' key")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.validateReq != nil {
					tc.validateReq(t, r)
				}
				jsonHandler(t, tc.serverStatus, tc.serverBody)(w, r)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			got, err := c.CreatePrompt(context.Background(), tc.prompt)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)
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
		serverStatus int
		serverBody   string // raw body (allows invalid JSON)
		wantResult   map[string]any
		wantErr      bool
		validateReq  func(t *testing.T, r *http.Request)
	}{
		{
			name:         "valid JSON response is decoded",
			promptIndex:  0,
			serverStatus: http.StatusOK,
			serverBody:   `{"prompt":"What is life?","response":"42"}`,
			wantResult:   map[string]any{"prompt": "What is life?", "response": "42"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/get/0", r.URL.Path)
			},
		},
		{
			name:         "index is embedded in URL path",
			promptIndex:  7,
			serverStatus: http.StatusOK,
			serverBody:   `{"id":7}`,
			wantResult:   map[string]any{"id": float64(7)},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "/get/7", r.URL.Path)
			},
		},
		{
			name:         "invalid JSON body returns error sentinel",
			promptIndex:  2,
			serverStatus: http.StatusOK,
			serverBody:   "not json at all",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "/get/2", r.URL.Path)
			},
		},
		{
			name:         "html error page returns error sentinel",
			promptIndex:  3,
			serverStatus: http.StatusInternalServerError,
			serverBody:   "<html><body>500 Internal Server Error</body></html>",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:         "empty body returns error sentinel",
			promptIndex:  4,
			serverStatus: http.StatusOK,
			serverBody:   "",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:         "no request body is sent for GET",
			promptIndex:  0,
			serverStatus: http.StatusOK,
			serverBody:   `{"ok":true}`,
			wantResult:   map[string]any{"ok": true},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				b, _ := io.ReadAll(r.Body)
				assert.Empty(t, b, "GET request should send no body")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.validateReq != nil {
					tc.validateReq(t, r)
				}
				w.WriteHeader(tc.serverStatus)
				_, _ = io.WriteString(w, tc.serverBody)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			got, err := c.GetResponse(context.Background(), tc.promptIndex)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)
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
		serverStatus int
		serverBody   map[string]any
		wantResult   map[string]any
		wantErr      bool
		validateReq  func(t *testing.T, r *http.Request)
	}{
		{
			name:         "valid index and new prompt",
			promptIndex:  1,
			newPrompt:    "Who is Goku?",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"status": "updated"},
			wantResult:   map[string]any{"status": "updated"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/update/1", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				var got map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
				assert.Equal(t, "Who is Goku?", got["new_prompt"])
			},
		},
		{
			name:         "index is embedded in URL path",
			promptIndex:  42,
			newPrompt:    "new text",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"ok": true},
			wantResult:   map[string]any{"ok": true},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "/update/42", r.URL.Path)
			},
		},
		{
			name:         "new_prompt key is sent in request body",
			promptIndex:  0,
			newPrompt:    "test prompt",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"result": "ok"},
			wantResult:   map[string]any{"result": "ok"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				var got map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
				_, hasKey := got["new_prompt"]
				assert.True(t, hasKey, "request body should contain 'new_prompt' key")
				assert.Equal(t, "test prompt", got["new_prompt"])
			},
		},
		{
			name:         "response body is decoded as JSON",
			promptIndex:  5,
			newPrompt:    "something",
			serverStatus: http.StatusOK,
			serverBody:   map[string]any{"id": float64(5), "prompt": "something"},
			wantResult:   map[string]any{"id": float64(5), "prompt": "something"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.validateReq != nil {
					tc.validateReq(t, r)
				}
				jsonHandler(t, tc.serverStatus, tc.serverBody)(w, r)
			}))
			defer srv.Close()

			c := newTestClient(srv)
			got, err := c.UpdatePrompt(context.Background(), tc.promptIndex, tc.newPrompt)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)
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
		serverStatus int
		serverBody   string
		wantResult   map[string]any
		wantErr      bool
		validateReq  func(t *testing.T, r *http.Request)
	}{
		{
			name:         "valid JSON response is decoded",
			promptIndex:  0,
			serverStatus: http.StatusOK,
			serverBody:   `{"status":"deleted"}`,
			wantResult:   map[string]any{"status": "deleted"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, "/delete/0", r.URL.Path)
			},
		},
		{
			name:         "index is embedded in URL path",
			promptIndex:  99,
			serverStatus: http.StatusOK,
			serverBody:   `{"id":99}`,
			wantResult:   map[string]any{"id": float64(99)},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "/delete/99", r.URL.Path)
			},
		},
		{
			name:         "invalid JSON body returns error sentinel",
			promptIndex:  1,
			serverStatus: http.StatusOK,
			serverBody:   "this is not json",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "/delete/1", r.URL.Path)
			},
		},
		{
			name:         "html error page returns error sentinel",
			promptIndex:  2,
			serverStatus: http.StatusNotFound,
			serverBody:   "<html>Not Found</html>",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:         "empty body returns error sentinel",
			promptIndex:  3,
			serverStatus: http.StatusOK,
			serverBody:   "",
			wantResult:   map[string]any{"error": "Invalid response from the server"},
		},
		{
			name:         "no request body is sent for DELETE",
			promptIndex:  0,
			serverStatus: http.StatusOK,
			serverBody:   `{"deleted":true}`,
			wantResult:   map[string]any{"deleted": true},
			validateReq: func(t *testing.T, r *http.Request) {
				t.Helper()
				b, _ := io.