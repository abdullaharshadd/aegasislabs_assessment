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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// capturedRequest records details about the HTTP request received by a test
// handler so that the test body can make assertions about method, path, and
// payload without coupling to the order in which httptest calls happen.
type capturedRequest struct {
	method      string
	path        string
	body        []byte
	contentType string
	accept      string
}

// newTestServer returns an httptest.Server and a pointer to the most-recently
// captured request. The handler function h receives the ResponseWriter and the
// captured request and is responsible for writing the response.
func newTestServer(
	t *testing.T,
	h func(w http.ResponseWriter, cr *capturedRequest),
) (*httptest.Server, *capturedRequest) {
	t.Helper()
	var last capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		last = capturedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			body:        raw,
			contentType: r.Header.Get("Content-Type"),
			accept:      r.Header.Get("Accept"),
		}
		h(w, &last)
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

// writeJSON is a helper that serialises v and writes it with status 200.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeNonJSON writes a plain-text body that is not valid JSON.
func writeNonJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "not json at all")
}

// newClientFor returns a Client whose base URL points at srv.
func newClientFor(srv *httptest.Server) *Client {
	return NewClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
}

// ---------------------------------------------------------------------------
// NewClient
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("", nil)
	assert.Equal(t, DefaultBaseURL, c.baseURL)
	assert.NotNil(t, c.http)
}

func TestNewClient_CustomValues(t *testing.T) {
	custom := &http.Client{Timeout: 1 * time.Second}
	c := NewClient("http://example.com", custom)
	assert.Equal(t, "http://example.com", c.baseURL)
	assert.Equal(t, custom, c.http)
}

func TestDefaultBaseURL(t *testing.T) {
	assert.Equal(t, "http://127.0.0.1:5000", DefaultBaseURL)
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	type wantReq struct {
		method  string
		path    string
		bodyKey string
		bodyVal string
		ct      string
	}

	tests := []struct {
		name        string
		prompt      string
		serverResp  func(w http.ResponseWriter, cr *capturedRequest)
		wantResp    PromptResponse
		wantErr     bool
		wantReqSpec wantReq
	}{
		{
			name:   "valid prompt returns json body",
			prompt: "What is life?",
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{"id": 1, "prompt": "What is life?"})
			},
			wantResp: PromptResponse{"id": float64(1), "prompt": "What is life?"},
			wantReqSpec: wantReq{
				method:  http.MethodPost,
				path:    "/create",
				bodyKey: "prompt",
				bodyVal: "What is life?",
				ct:      "application/json",
			},
		},
		{
			name:   "server returns non-json raises error (no tolerate)",
			prompt: "Hello",
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeNonJSON(w)
			},
			wantErr: true,
			wantReqSpec: wantReq{
				method:  http.MethodPost,
				path:    "/create",
				bodyKey: "prompt",
				bodyVal: "Hello",
				ct:      "application/json",
			},
		},
		{
			name:   "server returns empty json object",
			prompt: "empty",
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{})
			},
			wantResp: PromptResponse{},
			wantReqSpec: wantReq{
				method: http.MethodPost,
				path:   "/create",
			},
		},
		{
			name:   "prompt text with special characters",
			prompt: "What is the capital of Pakistan?",
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{"status": "created"})
			},
			wantResp: PromptResponse{"status": "created"},
			wantReqSpec: wantReq{
				method:  http.MethodPost,
				path:    "/create",
				bodyKey: "prompt",
				bodyVal: "What is the capital of Pakistan?",
				ct:      "application/json",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, captured := newTestServer(t, tc.serverResp)
			c := newClientFor(srv)

			got, err := c.CreatePrompt(context.Background(), tc.prompt)

			// request assertions
			if tc.wantReqSpec.method != "" {
				assert.Equal(t, tc.wantReqSpec.method, captured.method, "HTTP method")
			}
			if tc.wantReqSpec.path != "" {
				assert.Equal(t, tc.wantReqSpec.path, captured.path, "endpoint path")
			}
			if tc.wantReqSpec.ct != "" {
				assert.Equal(t, tc.wantReqSpec.ct, captured.contentType, "Content-Type header")
			}
			if tc.wantReqSpec.bodyKey != "" {
				var bodyMap map[string]string
				require.NoError(t, json.Unmarshal(captured.body, &bodyMap))
				assert.Equal(t, tc.wantReqSpec.bodyVal, bodyMap[tc.wantReqSpec.bodyKey], "request body field")
			}
			assert.Equal(t, "application/json", captured.accept, "Accept header")

			// response assertions
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResp, got)
			}
		})
	}
}

func TestCreatePrompt_RequestBodyContainsPromptKey(t *testing.T) {
	srv, captured := newTestServer(t, func(w http.ResponseWriter, _ *capturedRequest) {
		writeJSON(w, map[string]any{"ok": true})
	})
	c := newClientFor(srv)

	_, err := c.CreatePrompt(context.Background(), "invariant check")
	require.NoError(t, err)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(captured.body, &payload))
	assert.Contains(t, payload, "prompt", "body must contain 'prompt' key")
	assert.Equal(t, "invariant check", payload["prompt"])
}

func TestCreatePrompt_AlwaysUsesPostMethod(t *testing.T) {
	methods := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		writeJSON(w, map[string]any{})
	}))
	defer srv.Close()

	c := newClientFor(srv)
	for i := 0; i < 3; i++ {
		_, err := c.CreatePrompt(context.Background(), fmt.Sprintf("prompt-%d", i))
		require.NoError(t, err)
	}
	for _, m := range methods {
		assert.Equal(t, http.MethodPost, m)
	}
}

// ---------------------------------------------------------------------------
// GetResponse
// ---------------------------------------------------------------------------

func TestGetResponse(t *testing.T) {
	tests := []struct {
		name        string
		index       int
		serverResp  func(w http.ResponseWriter, cr *capturedRequest)
		wantResp    PromptResponse
		wantErr     bool
		wantPath    string
		wantMethod  string
	}{
		{
			name:  "valid json response returns parsed body",
			index: 0,
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{"id": 0, "prompt": "What is life?"})
			},
			wantResp:   PromptResponse{"id": float64(0), "prompt": "What is life?"},
			wantPath:   "/get/0",
			wantMethod: http.MethodGet,
		},
		{
			name:  "non-json response returns error map (tolerateNonJSON)",
			index: 1,
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeNonJSON(w)
			},
			wantResp:   PromptResponse{"error": "Invalid response from the server"},
			wantPath:   "/get/1",
			wantMethod: http.MethodGet,
		},
		{
			name:  "index 5 builds correct path",
			index: 5,
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{"id": 5})
			},
			wantResp:   PromptResponse{"id": float64(5)},
			wantPath:   "/get/5",
			wantMethod: http.MethodGet,
		},
		{
			name:  "server returns error json still decoded",
			index: 99,
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				w.WriteHeader(http.StatusNotFound)
				writeJSON(w, map[string]any{"error": "not found"})
			},
			wantResp:   PromptResponse{"error": "not found"},
			wantPath:   "/get/99",
			wantMethod: http.MethodGet,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, captured := newTestServer(t, tc.serverResp)
			c := newClientFor(srv)

			got, err := c.GetResponse(context.Background(), tc.index)

			assert.Equal(t, tc.wantMethod, captured.method, "HTTP method")
			assert.Equal(t, tc.wantPath, captured.path, "endpoint path")
			assert.Equal(t, "application/json", captured.accept, "Accept header")

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResp, got)
			}
		})
	}
}

func TestGetResponse_NeverPropagatesJSONDecodeError(t *testing.T) {
	srv, _ := newTestServer(t, func(w http.ResponseWriter, _ *capturedRequest) {
		writeNonJSON(w)
	})
	c := newClientFor(srv)

	got, err := c.GetResponse(context.Background(), 0)
	assert.NoError(t, err, "JSON decode errors must not be propagated")
	assert.Equal(t, PromptResponse{"error": "Invalid response from the server"}, got)
}

func TestGetResponse_AlwaysUsesGetMethod(t *testing.T) {
	methods := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		writeJSON(w, map[string]any{})
	}))
	defer srv.Close()

	c := newClientFor(srv)
	for i := 0; i < 3; i++ {
		_, err := c.GetResponse(context.Background(), i)
		require.NoError(t, err)
	}
	for _, m := range methods {
		assert.Equal(t, http.MethodGet, m)
	}
}

func TestGetResponse_PathContainsIndex(t *testing.T) {
	indices := []int{0, 1, 42, 100}
	for _, idx := range indices {
		t.Run(fmt.Sprintf("index_%d", idx), func(t *testing.T) {
			srv, captured := newTestServer(t, func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{})
			})
			c := newClientFor(srv)
			_, err := c.GetResponse(context.Background(), idx)
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("/get/%d", idx), captured.path)
		})
	}
}

// ---------------------------------------------------------------------------
// UpdatePrompt
// ---------------------------------------------------------------------------

func TestUpdatePrompt(t *testing.T) {
	tests := []struct {
		name       string
		index      int
		newPrompt  string
		serverResp func(w http.ResponseWriter, cr *capturedRequest)
		wantResp   PromptResponse
		wantErr    bool
		wantPath   string
		wantMethod string
	}{
		{
			name:      "valid update returns json body",
			index:     1,
			newPrompt: "Who is Goku?",
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{"updated": true, "new_prompt": "Who is Goku?"})
			},
			wantResp:   PromptResponse{"updated": true, "new_prompt": "Who is Goku?"},
			wantPath:   "/update/1",
			wantMethod: http.MethodPut,
		},
		{
			name:      "non-json response raises error (no tolerate)",
			index:     2,
			newPrompt: "new text",
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeNonJSON(w)
			},
			wantErr:    true,
			wantPath:   "/update/2",
			wantMethod: http.MethodPut,
		},
		{
			name:      "index 0 builds correct path",
			index:     0,
			newPrompt: "updated prompt",
			serverResp: func(w http.ResponseWriter, _ *capturedRequest) {
				writeJSON(w, map[string]any{"status": "ok"})
			},
			wantResp:   PromptResponse{"status": "ok"},