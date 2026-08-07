```go
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestClient builds a Client that targets the supplied httptest.Server.
func newTestClient(server *httptest.Server) *Client {
	return NewClient(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
}

// jsonBody is a convenience helper that marshals v and writes it as an
// application/json response.
func jsonBody(t *testing.T, w http.ResponseWriter, status int, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

// ---------------------------------------------------------------------------
// NewClient / functional options
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient()
	assert.Equal(t, DefaultBaseURL, c.baseURL)
	assert.Equal(t, http.DefaultClient, c.httpClient)
}

func TestNewClient_WithOptions(t *testing.T) {
	customHTTP := &http.Client{}
	c := NewClient(
		WithBaseURL("http://example.com"),
		WithHTTPClient(customHTTP),
	)
	assert.Equal(t, "http://example.com", c.baseURL)
	assert.Same(t, customHTTP, c.httpClient)
}

// ---------------------------------------------------------------------------
// decodeResponse (unit tests, no network)
// ---------------------------------------------------------------------------

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name        string
		raw         []byte
		wantResp    string
		wantMsg     string
		wantErr     string
		wantExtraKV map[string]any // optional non-nil expectation
	}{
		{
			name:     "response key",
			raw:      []byte(`{"response":"hello"}`),
			wantResp: "hello",
		},
		{
			name:    "message key",
			raw:     []byte(`{"message":"updated"}`),
			wantMsg: "updated",
		},
		{
			name:    "error key",
			raw:     []byte(`{"error":"not found"}`),
			wantErr: "not found",
		},
		{
			name:        "extra keys preserved",
			raw:         []byte(`{"response":"r","custom_key":"custom_val"}`),
			wantResp:    "r",
			wantExtraKV: map[string]any{"custom_key": "custom_val"},
		},
		{
			name:    "invalid json returns error response",
			raw:     []byte(`not-json`),
			wantErr: "Invalid response from the server",
		},
		{
			name:    "empty json object",
			raw:     []byte(`{}`),
			wantResp: "",
			wantMsg:  "",
			wantErr:  "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeResponse(tc.raw)
			require.NoError(t, err, "decodeResponse must never return a non-nil error")
			require.NotNil(t, got)

			assert.Equal(t, tc.wantResp, got.Response)
			assert.Equal(t, tc.wantMsg, got.Message)
			assert.Equal(t, tc.wantErr, got.Error)

			if tc.wantExtraKV != nil {
				for k, v := range tc.wantExtraKV {
					assert.Equal(t, v, got.Extra[k])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreatePrompt
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		handlerFn      func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantNilResult  bool
		wantResponse   string
		wantMessage    string
		wantErrField   string
		wantClientErr  bool   // true → expect non-nil error returned by CreatePrompt
		wantMethod     string
		wantPath       string
	}{
		{
			name:   "valid prompt returns server response",
			prompt: "What is life?",
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/create", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var req map[string]any
				require.NoError(t, json.Unmarshal(body, &req))
				assert.Equal(t, "What is life?", req["prompt"])

				jsonBody(t, w, http.StatusOK, map[string]string{"message": "created"})
			},
			wantMessage: "created",
			wantMethod:  http.MethodPost,
			wantPath:    "/create",
		},
		{
			name:   "server returns response key",
			prompt: "Hello",
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				jsonBody(t, w, http.StatusOK, map[string]string{"response": "prompt stored"})
			},
			wantResponse: "prompt stored",
		},
		{
			name:   "request body contains only prompt key",
			prompt: "test prompt",
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req map[string]any
				require.NoError(t, json.Unmarshal(body, &req))
				// must have exactly the 'prompt' key
				assert.Len(t, req, 1)
				assert.Equal(t, "test prompt", req["prompt"])
				jsonBody(t, w, http.StatusOK, map[string]string{"message": "ok"})
			},
			wantMessage: "ok",
		},
		{
			name:   "invalid json response is handled",
			prompt: "some prompt",
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not-json"))
			},
			wantErrField: "Invalid response from the server",
		},
		{
			name:          "network error propagated",
			prompt:        "error prompt",
			wantClientErr: true,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				// Close connection abruptly to simulate network error.
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					conn.Close()
					return
				}
				// Fallback: just write bad data
				w.WriteHeader(http.StatusOK)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.handlerFn != nil {
					tc.handlerFn(t, w, r)
				}
			}))
			defer srv.Close()

			client := newTestClient(srv)
			got, err := client.CreatePrompt(context.Background(), tc.prompt)

			if tc.wantClientErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantResponse, got.Response)
			assert.Equal(t, tc.wantMessage, got.Message)
			assert.Equal(t, tc.wantErrField, got.Error)
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
		handlerFn     func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantResponse  string
		wantErrField  string
		wantClientErr bool
	}{
		{
			name:        "valid index returns response",
			promptIndex: 0,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/get/0", r.URL.Path)
				jsonBody(t, w, http.StatusOK, map[string]string{"response": "Life is good"})
			},
			wantResponse: "Life is good",
		},
		{
			name:        "arbitrary index interpolated into path",
			promptIndex: 42,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/get/42", r.URL.Path)
				jsonBody(t, w, http.StatusOK, map[string]string{"response": "answer"})
			},
			wantResponse: "answer",
		},
		{
			name:        "invalid json response returns error object",
			promptIndex: 1,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not valid json"))
			},
			wantErrField: "Invalid response from the server",
		},
		{
			name:        "server returns error key",
			promptIndex: 99,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				jsonBody(t, w, http.StatusNotFound, map[string]string{"error": "not found"})
			},
			wantErrField: "not found",
		},
		{
			name:          "network error propagated",
			promptIndex:   0,
			wantClientErr: true,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					conn.Close()
				}
			},
		},
		{
			name:        "no request body is sent",
			promptIndex: 3,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				assert.Empty(t, body)
				jsonBody(t, w, http.StatusOK, map[string]string{"response": "ok"})
			},
			wantResponse: "ok",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.handlerFn != nil {
					tc.handlerFn(t, w, r)
				}
			}))
			defer srv.Close()

			client := newTestClient(srv)
			got, err := client.GetResponse(context.Background(), tc.promptIndex)

			if tc.wantClientErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantResponse, got.Response)
			assert.Equal(t, tc.wantErrField, got.Error)
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
		handlerFn     func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantMessage   string
		wantErrField  string
		wantClientErr bool
	}{
		{
			name:        "valid update returns message",
			promptIndex: 1,
			newPrompt:   "Who is Goku?",
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPut, r.Method)
				assert.Equal(t, "/update/1", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				body, _ := io.ReadAll(r.Body)
				var req map[string]any
				require.NoError(t, json.Unmarshal(body, &req))
				assert.Equal(t, "Who is Goku?", req["new_prompt"])
				assert.Len(t, req, 1, "body must contain only new_prompt")

				jsonBody(t, w, http.StatusOK, map[string]string{"message": "updated"})
			},
			wantMessage: "updated",
		},
		{
			name:        "arbitrary index interpolated into path",
			promptIndex: 7,
			newPrompt:   "some text",
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/update/7", r.URL.Path)
				jsonBody(t, w, http.StatusOK, map[string]string{"message": "ok"})
			},
			wantMessage: "ok",
		},
		{
			name:        "invalid json response is returned as error",
			promptIndex: 0,
			newPrompt:   "text",
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("bad json"))
			},
			wantErrField: "Invalid response from the server",
		},
		{
			name:          "network error propagated",
			promptIndex:   0,
			newPrompt:     "text",
			wantClientErr: true,
			handlerFn: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					conn.Close()
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.handlerFn != nil {
					tc.handlerFn(t, w, r)
				}
			}))
			defer srv.Close()

			client := newTestClient(srv)
			got, err := client.UpdatePrompt(context.Background(), tc.promptIndex, tc.newPrompt)

			if tc.wantClientErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.