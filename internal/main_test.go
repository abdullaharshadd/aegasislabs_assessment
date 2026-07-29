```go
package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// PromptStore unit tests
// ---------------------------------------------------------------------------

func TestPromptStore_Add(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		add            string
		wantLen        int
		wantLast       string
	}{
		{
			name:           "add to empty store",
			initialPrompts: nil,
			add:            "hello",
			wantLen:        1,
			wantLast:       "hello",
		},
		{
			name:           "add to non-empty store appends at end",
			initialPrompts: []string{"first"},
			add:            "second",
			wantLen:        2,
			wantLast:       "second",
		},
		{
			name:           "add empty string",
			initialPrompts: nil,
			add:            "",
			wantLen:        1,
			wantLast:       "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Add(p)
			}
			s.Add(tc.add)
			assert.Equal(t, tc.wantLen, len(s.prompts))
			assert.Equal(t, tc.wantLast, s.prompts[len(s.prompts)-1])
		})
	}
}

func TestPromptStore_Get(t *testing.T) {
	tests := []struct {
		name    string
		seed    []string
		index   int
		wantVal string
		wantOK  bool
	}{
		{
			name:    "valid index 0",
			seed:    []string{"alpha"},
			index:   0,
			wantVal: "alpha",
			wantOK:  true,
		},
		{
			name:    "valid index mid",
			seed:    []string{"a", "b", "c"},
			index:   1,
			wantVal: "b",
			wantOK:  true,
		},
		{
			name:   "index out of range high",
			seed:   []string{"a"},
			index:  1,
			wantOK: false,
		},
		{
			name:   "negative index",
			seed:   []string{"a"},
			index:  -1,
			wantOK: false,
		},
		{
			name:   "empty store",
			seed:   nil,
			index:  0,
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.seed {
				s.Add(p)
			}
			got, ok := s.Get(tc.index)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantVal, got)
			}
		})
	}
}

func TestPromptStore_Update(t *testing.T) {
	tests := []struct {
		name      string
		seed      []string
		index     int
		newPrompt string
		wantOK    bool
		wantStore []string
	}{
		{
			name:      "valid index updated",
			seed:      []string{"old"},
			index:     0,
			newPrompt: "new",
			wantOK:    true,
			wantStore: []string{"new"},
		},
		{
			name:      "valid index mid",
			seed:      []string{"a", "b", "c"},
			index:     1,
			newPrompt: "B",
			wantOK:    true,
			wantStore: []string{"a", "B", "c"},
		},
		{
			name:      "index out of range",
			seed:      []string{"a"},
			index:     5,
			newPrompt: "x",
			wantOK:    false,
			wantStore: []string{"a"},
		},
		{
			name:      "negative index",
			seed:      []string{"a"},
			index:     -1,
			newPrompt: "x",
			wantOK:    false,
			wantStore: []string{"a"},
		},
		{
			name:      "empty store",
			seed:      nil,
			index:     0,
			newPrompt: "x",
			wantOK:    false,
			wantStore: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.seed {
				s.Add(p)
			}
			ok := s.Update(tc.index, tc.newPrompt)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantStore, s.prompts)
		})
	}
}

func TestPromptStore_Delete(t *testing.T) {
	tests := []struct {
		name      string
		seed      []string
		index     int
		wantOK    bool
		wantStore []string
	}{
		{
			name:      "delete only element",
			seed:      []string{"a"},
			index:     0,
			wantOK:    true,
			wantStore: []string{},
		},
		{
			name:      "delete first of many",
			seed:      []string{"a", "b", "c"},
			index:     0,
			wantOK:    true,
			wantStore: []string{"b", "c"},
		},
		{
			name:      "delete middle",
			seed:      []string{"a", "b", "c"},
			index:     1,
			wantOK:    true,
			wantStore: []string{"a", "c"},
		},
		{
			name:      "delete last",
			seed:      []string{"a", "b", "c"},
			index:     2,
			wantOK:    true,
			wantStore: []string{"a", "b"},
		},
		{
			name:      "out of range high",
			seed:      []string{"a"},
			index:     1,
			wantOK:    false,
			wantStore: []string{"a"},
		},
		{
			name:      "negative index",
			seed:      []string{"a"},
			index:     -1,
			wantOK:    false,
			wantStore: []string{"a"},
		},
		{
			name:      "empty store",
			seed:      nil,
			index:     0,
			wantOK:    false,
			wantStore: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.seed {
				s.Add(p)
			}
			ok := s.Delete(tc.index)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantStore, s.prompts)
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP handler helpers
// ---------------------------------------------------------------------------

func newRouter() http.Handler {
	return buildRouter()
}

// doRequest sends a request through a fresh router and returns the recorder.
func doRequest(t *testing.T, router http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var m map[string]string
	err := json.NewDecoder(rr.Body).Decode(&m)
	require.NoError(t, err, "response body should be valid JSON")
	return m
}

// ---------------------------------------------------------------------------
// POST /create
// ---------------------------------------------------------------------------

func TestCreatePromptHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantBodyKey    string
		wantBodyValue  string
		wantSideEffect bool // whether a prompt should be stored
	}{
		{
			name:           "valid prompt stored",
			body:           `{"prompt":"hello world"}`,
			wantStatus:     http.StatusCreated,
			wantBodyKey:    "message",
			wantBodyValue:  msgCreated,
			wantSideEffect: true,
		},
		{
			name:          "missing prompt key",
			body:          `{}`,
			wantStatus:    http.StatusBadRequest,
			wantBodyKey:   "error",
			wantBodyValue: msgPromptRequired,
		},
		{
			name:          "empty prompt string",
			body:          `{"prompt":""}`,
			wantStatus:    http.StatusBadRequest,
			wantBodyKey:   "error",
			wantBodyValue: msgPromptRequired,
		},
		{
			name:          "null prompt",
			body:          `{"prompt":null}`,
			wantStatus:    http.StatusBadRequest,
			wantBodyKey:   "error",
			wantBodyValue: msgPromptRequired,
		},
		{
			name:          "malformed JSON body",
			body:          `not-json`,
			wantStatus:    http.StatusBadRequest,
			wantBodyKey:   "error",
			wantBodyValue: msgPromptRequired,
		},
		{
			name:          "empty body",
			body:          "",
			wantStatus:    http.StatusBadRequest,
			wantBodyKey:   "error",
			wantBodyValue: msgPromptRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use a dedicated store/handler so we can inspect side effects.
			store := NewPromptStore()
			h := NewPromptHandler(store)

			req := httptest.NewRequest(http.MethodPost, "/create", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			h.CreatePromptHandler(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			m := decodeJSON(t, rr)
			assert.Equal(t, tc.wantBodyValue, m[tc.wantBodyKey])

			if tc.wantSideEffect {
				assert.Equal(t, 1, len(store.prompts), "prompt should have been appended")
			} else {
				assert.Equal(t, 0, len(store.prompts), "no prompt should be stored on error")
			}
		})
	}
}

func TestCreatePromptHandler_OrderPreserved(t *testing.T) {
	store := NewPromptStore()
	h := NewPromptHandler(store)

	prompts := []string{"first", "second", "third"}
	for _, p := range prompts {
		body := `{"prompt":"` + p + `"}`
		req := httptest.NewRequest(http.MethodPost, "/create", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.CreatePromptHandler(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)
	}

	require.Equal(t, len(prompts), len(store.prompts))
	for i, p := range prompts {
		assert.Equal(t, p, store.prompts[i], "order should be preserved at index %d", i)
	}
}

// ---------------------------------------------------------------------------
// GET /get/{prompt_index}
// ---------------------------------------------------------------------------

func TestGetResponseHandler(t *testing.T) {
	tests := []struct {
		name          string
		seedPrompts   []string
		urlPath       string
		wantStatus    int
		wantBodyKey   string
		wantBodyValue string
	}{
		{
			name:          "valid index returns prompt",
			seedPrompts:   []string{"say something"},
			urlPath:       "/get/0",
			wantStatus:    http.StatusOK,
			wantBodyKey:   "response",
			wantBodyValue: "say something",
		},
		{
			name:          "valid index 1",
			seedPrompts:   []string{"first", "second"},
			urlPath:       "/get/1",
			wantStatus:    http.StatusOK,
			wantBodyKey:   "response",
			wantBodyValue: "second",
		},
		{
			name:          "index out of range high",
			seedPrompts:   []string{"only one"},
			urlPath:       "/get/1",
			wantStatus:    http.StatusOK,
			wantBodyKey:   "response",
			wantBodyValue: msgInvalidIndex,
		},
		{
			name:          "negative index",
			seedPrompts:   []string{"a"},
			urlPath:       "/get/-1",
			wantStatus:    http.StatusOK,
			wantBodyKey:   "response",
			wantBodyValue: msgInvalidIndex,
		},
		{
			name:          "empty store index 0",
			seedPrompts:   nil,
			urlPath:       "/get/0",
			wantStatus:    http.StatusOK,
			wantBodyKey:   "response",
			wantBodyValue: msgInvalidIndex,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := buildRouter()

			// Pre-populate via POST /create so the shared router store has data.
			// We build a fresh router per test to isolate state.
			store := NewPromptStore()
			h := NewPromptHandler(store)

			for _, p := range tc.seedPrompts {
				store.Add(p)
			}

			req := httptest.NewRequest(http.MethodGet, tc.urlPath, nil)
			rr := httptest.NewRecorder()

			// Use chi router wired with our store.
			handlerRouter := buildRouterWithHandler(h)
			handlerRouter.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			m := decodeJSON(t, rr)
			assert.Equal(t, tc.wantBodyValue, m[tc.wantBodyKey])
		})
	}

	// Non-integer path segment — tested via full router.
	t.Run("non-integer path segment returns 404", func(t *testing.T) {
		router := newRouter()
		rr := doRequest(t, router, http.MethodGet, "/get/abc", "")
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

// ---------------------------------------------------------------------------
// DELETE /delete/{prompt_index}
// ---------------------------------------------------------------------------

func TestDeletePromptHandler(t *testing.T) {
	tests := []struct {
		name          string
		seedPrompts   []string