```go
package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock LLM client
// ---------------------------------------------------------------------------

type mockLLMClient struct {
	completeFunc func(ctx context.Context, prompt string) (string, error)
	called       bool
	calledWith   string
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	m.called = true
	m.calledWith = prompt
	if m.completeFunc != nil {
		return m.completeFunc(ctx, prompt)
	}
	return fmt.Sprintf("completion for: %s", prompt), nil
}

// ---------------------------------------------------------------------------
// PromptStore unit tests
// ---------------------------------------------------------------------------

func TestPromptStore_Create(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		newPrompt      string
		wantIndex      int
		wantLen        int
	}{
		{
			name:      "append to empty store returns index 0",
			wantIndex: 0,
			wantLen:   1,
			newPrompt: "hello",
		},
		{
			name:           "append to non-empty store returns correct index",
			initialPrompts: []string{"a", "b"},
			newPrompt:      "c",
			wantIndex:      2,
			wantLen:        3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Create(p)
			}
			got := s.Create(tc.newPrompt)
			assert.Equal(t, tc.wantIndex, got)
			assert.Len(t, s.prompts, tc.wantLen)
			assert.Equal(t, tc.newPrompt, s.prompts[tc.wantIndex])
		})
	}
}

func TestPromptStore_Get(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		index          int
		wantPrompt     string
		wantErr        error
	}{
		{
			name:           "valid index returns prompt",
			initialPrompts: []string{"alpha", "beta"},
			index:          0,
			wantPrompt:     "alpha",
		},
		{
			name:           "valid second index returns prompt",
			initialPrompts: []string{"alpha", "beta"},
			index:          1,
			wantPrompt:     "beta",
		},
		{
			name:           "index out of range returns error",
			initialPrompts: []string{"alpha"},
			index:          5,
			wantErr:        ErrInvalidPromptIndex,
		},
		{
			name:           "negative index returns error",
			initialPrompts: []string{"alpha"},
			index:          -1,
			wantErr:        ErrInvalidPromptIndex,
		},
		{
			name:    "empty store returns error",
			index:   0,
			wantErr: ErrInvalidPromptIndex,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Create(p)
			}
			got, err := s.Get(tc.index)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantPrompt, got)
			}
		})
	}
}

func TestPromptStore_Update(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		index          int
		newPrompt      string
		wantErr        error
		wantPrompts    []string
	}{
		{
			name:           "valid index updates prompt",
			initialPrompts: []string{"old"},
			index:          0,
			newPrompt:      "new",
			wantPrompts:    []string{"new"},
		},
		{
			name:           "update middle element",
			initialPrompts: []string{"a", "b", "c"},
			index:          1,
			newPrompt:      "B",
			wantPrompts:    []string{"a", "B", "c"},
		},
		{
			name:           "index out of range returns error",
			initialPrompts: []string{"a"},
			index:          9,
			newPrompt:      "x",
			wantErr:        ErrInvalidPromptIndex,
			wantPrompts:    []string{"a"},
		},
		{
			name:        "negative index returns error",
			index:       -1,
			newPrompt:   "x",
			wantErr:     ErrInvalidPromptIndex,
			wantPrompts: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Create(p)
			}
			err := s.Update(tc.index, tc.newPrompt)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantPrompts, s.prompts)
		})
	}
}

func TestPromptStore_Delete(t *testing.T) {
	tests := []struct {
		name           string
		initialPrompts []string
		index          int
		wantErr        error
		wantPrompts    []string
	}{
		{
			name:           "delete only element",
			initialPrompts: []string{"a"},
			index:          0,
			wantPrompts:    []string{},
		},
		{
			name:           "delete first element shifts others",
			initialPrompts: []string{"a", "b", "c"},
			index:          0,
			wantPrompts:    []string{"b", "c"},
		},
		{
			name:           "delete middle element",
			initialPrompts: []string{"a", "b", "c"},
			index:          1,
			wantPrompts:    []string{"a", "c"},
		},
		{
			name:           "delete last element",
			initialPrompts: []string{"a", "b", "c"},
			index:          2,
			wantPrompts:    []string{"a", "b"},
		},
		{
			name:           "out-of-range index returns error, list unchanged",
			initialPrompts: []string{"a"},
			index:          5,
			wantErr:        ErrInvalidPromptIndex,
			wantPrompts:    []string{"a"},
		},
		{
			name:        "negative index returns error",
			index:       -1,
			wantErr:     ErrInvalidPromptIndex,
			wantPrompts: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewPromptStore()
			for _, p := range tc.initialPrompts {
				s.Create(p)
			}
			err := s.Delete(tc.index)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantPrompts, s.prompts)
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: build router with controllable dependencies
// ---------------------------------------------------------------------------

func newTestRouter(store *PromptStore, llm LLMClient) http.Handler {
	handler := NewPromptHandler(store, llm)

	r := buildRouterWith(handler)
	return r
}

// buildRouterWith builds the chi router wired to the given handler (avoids
// re-opening the package-private buildRouter to keep test isolation).
func buildRouterWith(handler *PromptHandler) http.Handler {
	// We re-use the exported router builder but bypass it by constructing our
	// own minimal router that matches the same paths.
	mux := http.NewServeMux()

	// We need chi for path params. Replicate the routing here.
	import_chi_router := func() http.Handler {
		// Use the real buildRouter shape but inject our handler by constructing
		// another chi router inline.
		return newChiRouter(handler)
	}
	return import_chi_router()
}

// newChiRouter replicates buildRouter but accepts an injected PromptHandler.
func newChiRouter(handler *PromptHandler) http.Handler {
	// Import the real chi router via the same package.
	// Since we are in the same package (internal) we can just build it directly.
	r := chi_new_router()
	r.Post("/create", handler.CreatePrompt)
	r.Get("/get/{prompt_index}", handler.GetResponse)
	r.Delete("/delete/{prompt_index}", handler.DeletePrompt)
	r.Put("/update/{prompt_index}", handler.UpdatePrompt)
	return r
}

// ---------------------------------------------------------------------------
// We are in the same package so we can call chi directly.
// ---------------------------------------------------------------------------

import (
	"github.com/go-chi/chi/v5"
)

func chi_new_router() *chi.Mux {
	return chi.NewRouter()
}

// ---------------------------------------------------------------------------
// HTTP handler tests – POST /create
// ---------------------------------------------------------------------------

func TestCreatePrompt(t *testing.T) {
	tests := []struct {
		name           string
		body           any
		wantStatus     int
		wantBody       map[string]string
		wantStoreLen   int
		preSeedPrompts []string
	}{
		{
			name:         "valid prompt returns 201 and message",
			body:         map[string]string{"prompt": "tell me a joke"},
			wantStatus:   http.StatusCreated,
			wantBody:     map[string]string{"message": "Prompt created successfully"},
			wantStoreLen: 1,
		},
		{
			name:         "missing prompt field returns 400",
			body:         map[string]string{},
			wantStatus:   http.StatusBadRequest,
			wantBody:     map[string]string{"error": "Prompt not provided"},
			wantStoreLen: 0,
		},
		{
			name:         "empty string prompt returns 400",
			body:         map[string]string{"prompt": ""},
			wantStatus:   http.StatusBadRequest,
			wantBody:     map[string]string{"error": "Prompt not provided"},
			wantStoreLen: 0,
		},
		{
			name:         "malformed JSON returns 400",
			body:         "not-json",
			wantStatus:   http.StatusBadRequest,
			wantBody:     map[string]string{"error": "invalid JSON body"},
			wantStoreLen: 0,
		},
		{
			name:           "appends to existing prompts",
			preSeedPrompts: []string{"existing"},
			body:           map[string]string{"prompt": "new"},
			wantStatus:     http.StatusCreated,
			wantBody:       map[string]string{"message": "Prompt created successfully"},
			wantStoreLen:   2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewPromptStore()
			for _, p := range tc.preSeedPrompts {
				store.Create(p)
			}
			llm := &mockLLMClient{}
			router := newChiRouter(NewPromptHandler(store, llm))

			var bodyBytes []byte
			switch v := tc.body.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				var err error
				bodyBytes, err = json.Marshal(v)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)

			var got map[string]string
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
			assert.Equal(t, tc.wantBody, got)

			store.mu.RLock()
			defer store.mu.RUnlock()
			assert.Len(t, store.prompts, tc.wantStoreLen)
		})
	}
}

// TestCreatePrompt_AppendsAtHighestIndex verifies the invariant that a new
// prompt is always appended at the highest index.
func TestCreatePrompt_AppendsAtHighestIndex(t *testing.T) {
	store := NewPromptStore()
	store.Create("first")
	store.Create("second")

	router := newChiRouter(NewPromptHandler(store, &mockLLMClient{}))

	body, _ := json.Marshal(map[string]string{"prompt": "third"})
	req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	store.mu.RLock()
	defer store.mu.RUnlock()
	assert.Equal(t, "third", store.prompts[2])
}

// ---------------------------------------------------------------------------
// HTTP handler tests – GET /get/{prompt_index}
// ---------------------------------------------------------------------------

func TestGetResponse(t *testing.T) {
	tests := []struct {
		name           string
		preSeedPrompts []string
		urlIndex       string
		llmResponse    string
		llmErr         error
		wantStatus     int
		wantBody       map[string]string
		wantLLMCalled  bool
	}{
		{
			name:           "valid index returns LLM completion",
			preSeedPrompts: []string{"say hello"},
			urlIndex:       "0",
			llmResponse:    "Hello there!",
			wantStatus:     http.StatusOK,
			wantBody:       map[string]string{"response": "Hello there!"},
			wantLLMCalled:  true,
		},
		{
			name:           "valid non-zero index",
			preSeedPrompts: []string{"a", "b", "c"},
			urlIndex:       "2",
			llmResponse:    "completion for c",
			wantStatus:     http.StatusOK,
			wantBody:       map[string]string{"response": "completion for c"},
			wantLLMCalled:  true,
		},
		{
			name:           "index out of range returns 200 with invalid message",
			preSeedPrompts: []string{"a"},
			urlIndex:       "99",
			wantStatus:     http.StatusOK,
			wantBody:       map[string]string{"response": "Invalid prompt index"},
			wantLLMCalled:  false,
		},
		{
			name:          "negative index returns 200 with invalid message",
			urlIndex:      "-1",
			wantStatus:    http.StatusOK,
			wantBody:      map[string]string{"response