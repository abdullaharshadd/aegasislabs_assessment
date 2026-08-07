```go
package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock Completer
// ---------------------------------------------------------------------------

type mockCompleter struct {
	response string
	err      error
	called   bool
}

func (m *mockCompleter) Complete(prompt string) (string, error) {
	m.called = true
	return m.response, m.err
}

// ---------------------------------------------------------------------------
// Helper: build a chi router wired to PromptHandlers backed by the given store.
// ---------------------------------------------------------------------------

func newTestRouter(store *PromptStore) http.Handler {
	h := NewPromptHandlers(store)

	r := chi.NewRouter()
	r.Post("/create", h.CreatePromptHandler)
	r.Get("/get/{prompt_index}", h.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", h.DeletePromptHandler)
	r.Put("/update/{prompt_index}", h.UpdatePromptHandler)

	return r
}

// ---------------------------------------------------------------------------
// Unit tests: PromptStore.CreatePromptEntry
// ---------------------------------------------------------------------------

func TestCreatePromptEntry(t *testing.T) {
	tests := []struct {
		name            string
		prompts         []string
		expectedLen     int
		expectedPrompts []string
	}{
		{
			name:            "single prompt appended",
			prompts:         []string{"hello world"},
			expectedLen:     1,
			expectedPrompts: []string{"hello world"},
		},
		{
			name:            "multiple prompts appended in order",
			prompts:         []string{"first", "second", "third"},
			expectedLen:     3,
			expectedPrompts: []string{"first", "second", "third"},
		},
		{
			name:            "empty string prompt stored",
			prompts:         []string{""},
			expectedLen:     1,
			expectedPrompts: []string{""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewPromptStore(nil)
			initialLen := len(store.prompts)

			for _, p := range tc.prompts {
				store.CreatePromptEntry(p)
			}

			assert.Equal(t, initialLen+tc.expectedLen, len(store.prompts))
			for i, expected := range tc.expectedPrompts {
				assert.Equal(t, expected, store.prompts[i])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unit tests: PromptStore.GetResponseAt
// ---------------------------------------------------------------------------

func TestGetResponseAt(t *testing.T) {
	tests := []struct {
		name            string
		seedPrompts     []string
		index           int
		completer       *mockCompleter
		expectedText    string
		expectedErr     error
		expectCompleter bool
	}{
		{
			name:            "valid index returns completion",
			seedPrompts:     []string{"tell me a joke"},
			index:           0,
			completer:       &mockCompleter{response: "Why did the chicken cross the road?"},
			expectedText:    "Why did the chicken cross the road?",
			expectCompleter: true,
		},
		{
			name:            "negative index returns ErrInvalidPromptIndex",
			seedPrompts:     []string{"some prompt"},
			index:           -1,
			completer:       &mockCompleter{},
			expectedErr:     ErrInvalidPromptIndex,
			expectCompleter: false,
		},
		{
			name:            "index equal to length returns ErrInvalidPromptIndex",
			seedPrompts:     []string{"a", "b"},
			index:           2,
			completer:       &mockCompleter{},
			expectedErr:     ErrInvalidPromptIndex,
			expectCompleter: false,
		},
		{
			name:            "index greater than length returns ErrInvalidPromptIndex",
			seedPrompts:     []string{"only one"},
			index:           5,
			completer:       &mockCompleter{},
			expectedErr:     ErrInvalidPromptIndex,
			expectCompleter: false,
		},
		{
			name:            "completer error is wrapped and returned",
			seedPrompts:     []string{"prompt"},
			index:           0,
			completer:       &mockCompleter{err: errors.New("api timeout")},
			expectedErr:     errors.New("completion failed: api timeout"),
			expectCompleter: true,
		},
		{
			name:            "nil completer returns error",
			seedPrompts:     []string{"prompt"},
			index:           0,
			completer:       nil,
			expectedErr:     errors.New("completion provider not configured"),
			expectCompleter: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var completer Completer
			if tc.completer != nil {
				completer = tc.completer
			}
			store := NewPromptStore(completer)
			for _, p := range tc.seedPrompts {
				store.CreatePromptEntry(p)
			}

			text, err := store.GetResponseAt(tc.index)

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr.Error())
				assert.Empty(t, text)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedText, text)
			}

			if tc.completer != nil {
				assert.Equal(t, tc.expectCompleter, tc.completer.called)
			}

			// Verify prompts list is not modified
			assert.Equal(t, tc.seedPrompts, store.prompts)
		})
	}
}

// ---------------------------------------------------------------------------
// Unit tests: PromptStore.UpdatePromptAt
// ---------------------------------------------------------------------------

func TestUpdatePromptAt(t *testing.T) {
	tests := []struct {
		name            string
		seedPrompts     []string
		index           int
		newPrompt       string
		expectedMessage string
		expectedErr     error
		expectedPrompts []string
	}{
		{
			name:            "valid index updates prompt and returns success message",
			seedPrompts:     []string{"old prompt"},
			index:           0,
			newPrompt:       "new prompt",
			expectedMessage: "Prompt updated successfully",
			expectedPrompts: []string{"new prompt"},
		},
		{
			name:            "valid index in multi-element list",
			seedPrompts:     []string{"a", "b", "c"},
			index:           1,
			newPrompt:       "B",
			expectedMessage: "Prompt updated successfully",
			expectedPrompts: []string{"a", "B", "c"},
		},
		{
			name:            "negative index returns ErrInvalidPromptIndex",
			seedPrompts:     []string{"prompt"},
			index:           -1,
			newPrompt:       "x",
			expectedErr:     ErrInvalidPromptIndex,
			expectedPrompts: []string{"prompt"},
		},
		{
			name:            "out-of-range index returns ErrInvalidPromptIndex",
			seedPrompts:     []string{"prompt"},
			index:           1,
			newPrompt:       "x",
			expectedErr:     ErrInvalidPromptIndex,
			expectedPrompts: []string{"prompt"},
		},
		{
			name:            "empty store with index 0 returns ErrInvalidPromptIndex",
			seedPrompts:     []string{},
			index:           0,
			newPrompt:       "x",
			expectedErr:     ErrInvalidPromptIndex,
			expectedPrompts: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewPromptStore(nil)
			for _, p := range tc.seedPrompts {
				store.CreatePromptEntry(p)
			}
			initialLen := len(store.prompts)

			message, err := store.UpdatePromptAt(tc.index, tc.newPrompt)

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.expectedErr))
				assert.Empty(t, message)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedMessage, message)
			}

			// Length must be unchanged
			assert.Equal(t, initialLen, len(store.prompts))

			// Verify final state
			assert.Equal(t, tc.expectedPrompts, store.prompts)
		})
	}
}

// ---------------------------------------------------------------------------
// Unit tests: PromptStore.DeletePromptAt
// ---------------------------------------------------------------------------

func TestDeletePromptAt(t *testing.T) {
	tests := []struct {
		name            string
		seedPrompts     []string
		index           int
		expectedMessage string
		expectedErr     error
		expectedPrompts []string
	}{
		{
			name:            "delete only element",
			seedPrompts:     []string{"only"},
			index:           0,
			expectedMessage: "Prompt deleted successfully",
			expectedPrompts: []string{},
		},
		{
			name:            "delete first of many",
			seedPrompts:     []string{"a", "b", "c"},
			index:           0,
			expectedMessage: "Prompt deleted successfully",
			expectedPrompts: []string{"b", "c"},
		},
		{
			name:            "delete middle element shifts down",
			seedPrompts:     []string{"a", "b", "c"},
			index:           1,
			expectedMessage: "Prompt deleted successfully",
			expectedPrompts: []string{"a", "c"},
		},
		{
			name:            "delete last element",
			seedPrompts:     []string{"a", "b", "c"},
			index:           2,
			expectedMessage: "Prompt deleted successfully",
			expectedPrompts: []string{"a", "b"},
		},
		{
			name:            "negative index returns ErrInvalidPromptIndex",
			seedPrompts:     []string{"a"},
			index:           -1,
			expectedErr:     ErrInvalidPromptIndex,
			expectedPrompts: []string{"a"},
		},
		{
			name:            "out-of-range index returns ErrInvalidPromptIndex",
			seedPrompts:     []string{"a"},
			index:           1,
			expectedErr:     ErrInvalidPromptIndex,
			expectedPrompts: []string{"a"},
		},
		{
			name:            "empty store returns ErrInvalidPromptIndex",
			seedPrompts:     []string{},
			index:           0,
			expectedErr:     ErrInvalidPromptIndex,
			expectedPrompts: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewPromptStore(nil)
			for _, p := range tc.seedPrompts {
				store.CreatePromptEntry(p)
			}

			message, err := store.DeletePromptAt(tc.index)

			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.expectedErr))
				assert.Empty(t, message)
				// List length must not change
				assert.Equal(t, len(tc.seedPrompts), len(store.prompts))
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedMessage, message)
				// List length must decrease by one
				assert.Equal(t, len(tc.seedPrompts)-1, len(store.prompts))
			}

			assert.Equal(t, tc.expectedPrompts, store.prompts)
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP tests: POST /create
// ---------------------------------------------------------------------------

func TestCreatePromptHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedBody   map[string]string
		expectedLen    int
	}{
		{
			name:           "valid prompt returns 201",
			body:           `{"prompt":"hello world"}`,
			expectedStatus: http.StatusCreated,
			expectedBody:   map[string]string{"message": "Prompt created successfully"},
			expectedLen:    1,
		},
		{
			name:           "missing prompt field returns 400",
			body:           `{}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]string{"error": "Prompt not provided"},
			expectedLen:    0,
		},
		{
			name:           "empty prompt string returns 400",
			body:           `{"prompt":""}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]string{"error": "Prompt not provided"},
			expectedLen:    0,
		},
		{
			name:           "malformed JSON returns 400",
			body:           `not json`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]string{"error": "Prompt not provided"},
			expectedLen:    0,
		},
		{
			name:           "empty body returns 400",
			body:           ``,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   map[string]string{"error": "Prompt not provided"},
			expectedLen:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewPromptStore(nil)
			router := newTestRouter(store)

			req := httptest.NewRequest(http.MethodPost, "/create", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			var got map[string]string
			err := json.NewDecoder(rr.Body).Decode(&got)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedBody, got)

			store.mu.Lock()
			assert.Equal(t, tc.expectedLen, len(store.prompts))
			store.mu.Unlock()
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP tests: GET /get/{prompt_index}
// ---------------------------------------------------------------------------

func TestGetResponseHandler(t *testing.T) {
	tests := []struct {
		name           string
		seedPrompts    []string
		completer      *mockCompleter
		path           string
		expectedStatus int
		expectedBody   map[string]string
		expectCalled   bool
	}{
		{
			name:           "valid index returns completion",
			seedPrompts:    []string{"say hello"},
			completer:      &mockCompleter{response: "Hello!"},
			path:           "/get/0",
			expectedStatus: http.StatusOK,
			expectedBody:   map[string]string{"response": "Hello!"},
			expectCalled:   true,
		},
		{
			name:           "out-of-range index returns 200 with Invalid prompt index",
			seedPrompts:    []string{"say hello"},
			completer:      &mockCompleter{},
			path:           "/get/5",
			expectedStatus: http.StatusOK,
			expectedBody:   map[string]string{"response": "Invalid prompt index"},
			expectCalled:   false,
		},
		{
			name:           "negative index returns 200 with Invalid prompt index",
			seedPrompts:    []string{"say hello"},
			completer:      &mockCompleter{},
			path:           "/get/-1",
			expected