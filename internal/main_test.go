```go
package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock PromptResponder
// ---------------------------------------------------------------------------

type mockResponder struct {
	response string
	err      error
	called   bool
	lastPrompt string
}

func (m *mockResponder) Respond(_ context.Context, prompt string) (string, error) {
	m.called = true
	m.lastPrompt = prompt
	return m.response, m.err
}

// ---------------------------------------------------------------------------
// PromptStore unit tests
// ---------------------------------------------------------------------------

func TestPromptStore_Add(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		addPrompt      string
		wantLen        int
		wantLast       string
	}{
		{
			name:      "add to empty store",
			addPrompt: "hello world",
			wantLen:   1,
			wantLast:  "hello world",
		},
		{
			name:           "add to non-empty store",
			initialPrompts: []string{"first"},
			addPrompt:      "second",
			wantLen:        2,
			wantLast:       "second",
		},
		{
			name:           "add empty string",
			initialPrompts: []string{"existing"},
			addPrompt:      "",
			wantLen:        2,
			wantLast:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Add(p)
			}

			lenBefore := len(s.prompts)
			s.Add(tc.addPrompt)

			assert.Equal(t, lenBefore+1, len(s.prompts), "length must increase by exactly 1")
			assert.Equal(t, tc.wantLen, len(s.prompts))
			assert.Equal(t, tc.wantLast, s.prompts[len(s.prompts)-1], "new prompt must be at last index")
		})
	}
}

func TestPromptStore_Get(t *testing.T) {
	tests := []struct {
		name    string
		prompts []string
		index   int
		wantVal string
		wantOK  bool
	}{
		{
			name:    "valid index 0",
			prompts: []string{"alpha"},
			index:   0,
			wantVal: "alpha",
			wantOK:  true,
		},
		{
			name:    "valid index in middle",
			prompts: []string{"a", "b", "c"},
			index:   1,
			wantVal: "b",
			wantOK:  true,
		},
		{
			name:    "valid last index",
			prompts: []string{"a", "b", "c"},
			index:   2,
			wantVal: "c",
			wantOK:  true,
		},
		{
			name:    "negative index",
			prompts: []string{"a"},
			index:   -1,
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "index equal to length",
			prompts: []string{"a", "b"},
			index:   2,
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "index greater than length",
			prompts: []string{"a"},
			index:   99,
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "empty store",
			prompts: []string{},
			index:   0,
			wantVal: "",
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.prompts {
				s.Add(p)
			}

			val, ok := s.Get(tc.index)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantVal, val)
			// invariant: Get never modifies the store
			assert.Equal(t, len(tc.prompts), len(s.prompts), "Get must not modify the store")
		})
	}
}

func TestPromptStore_UpdateChecked(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		index          int
		newPrompt      string
		wantOK         bool
		wantPrompts    []string
	}{
		{
			name:           "valid index updates value",
			initialPrompts: []string{"old"},
			index:          0,
			newPrompt:      "new",
			wantOK:         true,
			wantPrompts:    []string{"new"},
		},
		{
			name:           "valid index in middle",
			initialPrompts: []string{"a", "b", "c"},
			index:          1,
			newPrompt:      "updated",
			wantOK:         true,
			wantPrompts:    []string{"a", "updated", "c"},
		},
		{
			name:           "negative index",
			initialPrompts: []string{"a"},
			index:          -1,
			newPrompt:      "x",
			wantOK:         false,
			wantPrompts:    []string{"a"},
		},
		{
			name:           "index equal to length",
			initialPrompts: []string{"a", "b"},
			index:          2,
			newPrompt:      "x",
			wantOK:         false,
			wantPrompts:    []string{"a", "b"},
		},
		{
			name:           "empty store",
			initialPrompts: []string{},
			index:          0,
			newPrompt:      "x",
			wantOK:         false,
			wantPrompts:    []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Add(p)
			}

			ok := s.UpdateChecked(tc.index, tc.newPrompt)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantPrompts, s.prompts, "store state after update")
			// invariant: length never changes
			assert.Equal(t, len(tc.initialPrompts), len(s.prompts), "length must be unchanged")
		})
	}
}

func TestPromptStore_DeleteChecked(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		index          int
		wantOK         bool
		wantPrompts    []string
	}{
		{
			name:           "delete only element",
			initialPrompts: []string{"only"},
			index:          0,
			wantOK:         true,
			wantPrompts:    []string{},
		},
		{
			name:           "delete first element shifts others",
			initialPrompts: []string{"a", "b", "c"},
			index:          0,
			wantOK:         true,
			wantPrompts:    []string{"b", "c"},
		},
		{
			name:           "delete middle element",
			initialPrompts: []string{"a", "b", "c"},
			index:          1,
			wantOK:         true,
			wantPrompts:    []string{"a", "c"},
		},
		{
			name:           "delete last element",
			initialPrompts: []string{"a", "b", "c"},
			index:          2,
			wantOK:         true,
			wantPrompts:    []string{"a", "b"},
		},
		{
			name:           "negative index",
			initialPrompts: []string{"a"},
			index:          -1,
			wantOK:         false,
			wantPrompts:    []string{"a"},
		},
		{
			name:           "index equal to length",
			initialPrompts: []string{"a", "b"},
			index:          2,
			wantOK:         false,
			wantPrompts:    []string{"a", "b"},
		},
		{
			name:           "empty store",
			initialPrompts: []string{},
			index:          0,
			wantOK:         false,
			wantPrompts:    []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Add(p)
			}

			ok := s.DeleteChecked(tc.index)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantPrompts, s.prompts)

			if tc.wantOK {
				assert.Equal(t, len(tc.initialPrompts)-1, len(s.prompts), "length must decrease by 1 on success")
			} else {
				assert.Equal(t, len(tc.initialPrompts), len(s.prompts), "length must be unchanged on failure")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP handler helpers
// ---------------------------------------------------------------------------

// newTestServer returns an httptest.Server backed by buildRouter() with a
// custom PromptHandler injected so we can control the responder per test.
func newHandlerWithResponder(responder PromptResponder) http.Handler {
	handler := NewPromptHandler(responder)

	r := buildTestRouter(handler)
	return r
}

// buildTestRouter wires a PromptHandler into a chi router without Logger middleware
// so test output is cleaner, but keeps Recoverer.
func buildTestRouter(h *PromptHandler) http.Handler {
	// Re-use buildRouter shape but swap the handler.  Since buildRouter
	// constructs its own PromptHandler internally, we build a minimal version
	// here that mirrors the route registrations.
	mux := http.NewServeMux()

	// We directly exercise the handlers via httptest; use a minimal chi-like
	// setup.  Because chi is already a dependency we can use it directly.
	import_chi_and_build := func() http.Handler {
		// Import inline via an anonymous function to avoid import cycle; in
		// reality all imports are at the top of the file.
		return buildRouterWithHandler(h)
	}
	_ = mux
	return import_chi_and_build()
}

// buildRouterWithHandler is a test-internal variant of buildRouter that
// accepts an externally constructed PromptHandler.
func buildRouterWithHandler(h *PromptHandler) http.Handler {
	// We reproduce the same chi wiring as buildRouter but inject h.
	// This avoids duplicating the real production router.
	r := newChiRouter(h)
	return r
}

// ---------------------------------------------------------------------------
// POST /create
// ---------------------------------------------------------------------------

func TestCreatePromptHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantBody       map[string]string
		wantStoreGrows bool
	}{
		{
			name:           "valid prompt",
			body:           `{"prompt": "tell me a joke"}`,
			wantStatus:     http.StatusCreated,
			wantBody:       map[string]string{"message": "Prompt created successfully"},
			wantStoreGrows: true,
		},
		{
			name:           "missing prompt field",
			body:           `{}`,
			wantStatus:     http.StatusBadRequest,
			wantBody:       map[string]string{"error": "Prompt not provided"},
			wantStoreGrows: false,
		},
		{
			name:           "empty prompt value",
			body:           `{"prompt": ""}`,
			wantStatus:     http.StatusBadRequest,
			wantBody:       map[string]string{"error": "Prompt not provided"},
			wantStoreGrows: false,
		},
		{
			name:           "invalid json",
			body:           `not-json`,
			wantStatus:     http.StatusBadRequest,
			wantBody:       map[string]string{"error": "Prompt not provided"},
			wantStoreGrows: false,
		},
		{
			name:           "empty body",
			body:           ``,
			wantStatus:     http.StatusBadRequest,
			wantBody:       map[string]string{"error": "Prompt not provided"},
			wantStoreGrows: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockResponder{}
			h := NewPromptHandler(mock)
			svr := newChiRouter(h)

			req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			svr.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assertJSONBody(t, rec, tc.wantBody)

			if tc.wantStoreGrows {
				assert.Equal(t, 1, len(h.store.prompts), "store should have one prompt after creation")
			} else {
				assert.Equal(t, 0, len(h.store.prompts), "store should be unchanged on error")
			}
		})
	}
}

func TestCreatePromptHandler_MultipleCreates(t *testing.T) {
	mock := &mockResponder{}
	h := NewPromptHandler(mock)
	svr := newChiRouter(h)

	prompts := []string{"first", "second", "third"}
	for i, p := range prompts {
		body := fmt.Sprintf(`{"prompt": %q}`, p)
		req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		svr.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.Equal(t, i+1, len(h.store.prompts), "store length must grow by 1 each call")
		assert.Equal(t, p, h.store.prompts[i], "new prompt must be at last index")
	}
}

// ---------------------------------------------------------------------------
// GET /get/{prompt_index}
// ---------------------------------------------------------------------------

func TestGetResponseHandler(t *testing.T) {
	tests := []struct {
		name           string
		seedPrompts    []string
		urlIndex       string
		responderResp  string
		responderErr   error
		wantStatus     int
		wantBody       map[string]string
		wantAPICalled  bool
	}{
		{
			name:          "valid index returns completion",
			seedPrompts:   []string{"what is Go?"},
			urlIndex:      "0",
			responderResp: "Go is a statically typed language.",
			wantStatus:    http.StatusOK,
			wantBody:      map[string]string{"response": "Go is a statically typed language."},
			wantAPICalled: true,
		},
		{
			name:          "negative index returns invalid message without API call",
			seedPrompts:   []string{"hello"},
			urlIndex:      "-1