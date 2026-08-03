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

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockCompletionCreator is a controllable completionCreator for tests.
type mockCompletionCreator struct {
	response string
	err      error
	calls    []string // records every prompt received
}

func (m *mockCompletionCreator) CreateCompletion(_ context.Context, prompt string) (string, error) {
	m.calls = append(m.calls, prompt)
	return m.response, m.err
}

// ---------------------------------------------------------------------------
// Helper: build a chi router wired to the given handler without middleware
// ---------------------------------------------------------------------------

func newTestRouter(h *PromptHandler) http.Handler {
	r := chi.NewRouter()
	r.Post("/create", h.CreatePromptHandler)
	r.Get("/get/{prompt_index}", h.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", h.DeletePromptHandler)
	r.Put("/update/{prompt_index}", h.UpdatePromptHandler)
	return r
}

// newHandlerWithMock returns a ready-to-use router and the underlying mock so
// tests can seed prompts directly.
func newHandlerWithMock(mock *mockCompletionCreator) (*PromptHandler, http.Handler) {
	store := NewChatGPTBotStore(mock)
	h := NewPromptHandler(store)
	return h, newTestRouter(h)
}

// decodeBody is a small helper that unmarshals a response body into a map.
func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	return m
}

// ---------------------------------------------------------------------------
// Unit tests: ChatGPTBotStore (store-layer)
// ---------------------------------------------------------------------------

func TestCreatePromptEntry(t *testing.T) {
	tests := []struct {
		name    string
		prompts []string
	}{
		{
			name:    "single prompt appended",
			prompts: []string{"hello"},
		},
		{
			name:    "multiple prompts appended in order",
			prompts: []string{"first", "second", "third"},
		},
		{
			name:    "empty prompt string is stored",
			prompts: []string{""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewChatGPTBotStore(&mockCompletionCreator{})
			for i, p := range tc.prompts {
				store.CreatePromptEntry(p)
				store.mu.RLock()
				assert.Equal(t, i+1, len(store.prompts), "list length after %d insertions", i+1)
				store.mu.RUnlock()
			}
			store.mu.RLock()
			assert.Equal(t, tc.prompts, store.prompts)
			store.mu.RUnlock()
		})
	}
}

func TestGetResponseAt(t *testing.T) {
	errAPI := fmt.Errorf("api error")

	tests := []struct {
		name         string
		seed         []string
		index        int
		mockResp     string
		mockErr      error
		wantText     string
		wantInRange  bool
		wantErr      bool
		wantAPICalls int
	}{
		{
			name:         "in-range index returns completion text",
			seed:         []string{"tell me a joke"},
			index:        0,
			mockResp:     "Why did the chicken cross the road?",
			wantText:     "Why did the chicken cross the road?",
			wantInRange:  true,
			wantAPICalls: 1,
		},
		{
			name:         "in-range index second element",
			seed:         []string{"first", "second prompt"},
			index:        1,
			mockResp:     "response for second",
			wantText:     "response for second",
			wantInRange:  true,
			wantAPICalls: 1,
		},
		{
			name:         "out-of-range positive index",
			seed:         []string{"only one"},
			index:        5,
			wantInRange:  false,
			wantAPICalls: 0,
		},
		{
			name:         "negative index",
			seed:         []string{"only one"},
			index:        -1,
			wantInRange:  false,
			wantAPICalls: 0,
		},
		{
			name:         "empty store any index",
			seed:         []string{},
			index:        0,
			wantInRange:  false,
			wantAPICalls: 0,
		},
		{
			name:         "API error propagated",
			seed:         []string{"will fail"},
			index:        0,
			mockErr:      errAPI,
			wantInRange:  true,
			wantErr:      true,
			wantAPICalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockCompletionCreator{response: tc.mockResp, err: tc.mockErr}
			store := NewChatGPTBotStore(mock)
			for _, p := range tc.seed {
				store.CreatePromptEntry(p)
			}

			text, inRange, err := store.GetResponseAt(context.Background(), tc.index)

			assert.Equal(t, tc.wantInRange, inRange)
			assert.Equal(t, tc.wantAPICalls, len(mock.calls), "unexpected number of API calls")

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tc.wantInRange && !tc.wantErr {
				assert.Equal(t, tc.wantText, text)
			}
		})
	}
}

func TestUpdatePromptAt(t *testing.T) {
	tests := []struct {
		name       string
		seed       []string
		index      int
		newPrompt  string
		wantOK     bool
		wantList   []string
	}{
		{
			name:      "in-range update replaces prompt",
			seed:      []string{"old"},
			index:     0,
			newPrompt: "new",
			wantOK:    true,
			wantList:  []string{"new"},
		},
		{
			name:      "update middle element",
			seed:      []string{"a", "b", "c"},
			index:     1,
			newPrompt: "B",
			wantOK:    true,
			wantList:  []string{"a", "B", "c"},
		},
		{
			name:      "out-of-range positive",
			seed:      []string{"only"},
			index:     10,
			newPrompt: "x",
			wantOK:    false,
			wantList:  []string{"only"},
		},
		{
			name:      "negative index",
			seed:      []string{"only"},
			index:     -1,
			newPrompt: "x",
			wantOK:    false,
			wantList:  []string{"only"},
		},
		{
			name:      "empty store",
			seed:      []string{},
			index:     0,
			newPrompt: "x",
			wantOK:    false,
			wantList:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewChatGPTBotStore(&mockCompletionCreator{})
			for _, p := range tc.seed {
				store.CreatePromptEntry(p)
			}

			ok := store.UpdatePromptAt(tc.index, tc.newPrompt)
			assert.Equal(t, tc.wantOK, ok)

			store.mu.RLock()
			assert.Equal(t, tc.wantList, store.prompts)
			store.mu.RUnlock()
		})
	}
}

func TestDeletePromptAt(t *testing.T) {
	tests := []struct {
		name     string
		seed     []string
		index    int
		wantOK   bool
		wantList []string
	}{
		{
			name:     "delete only element",
			seed:     []string{"only"},
			index:    0,
			wantOK:   true,
			wantList: []string{},
		},
		{
			name:     "delete first element shifts others down",
			seed:     []string{"a", "b", "c"},
			index:    0,
			wantOK:   true,
			wantList: []string{"b", "c"},
		},
		{
			name:     "delete middle element",
			seed:     []string{"a", "b", "c"},
			index:    1,
			wantOK:   true,
			wantList: []string{"a", "c"},
		},
		{
			name:     "delete last element",
			seed:     []string{"a", "b", "c"},
			index:    2,
			wantOK:   true,
			wantList: []string{"a", "b"},
		},
		{
			name:     "out-of-range positive",
			seed:     []string{"only"},
			index:    5,
			wantOK:   false,
			wantList: []string{"only"},
		},
		{
			name:     "negative index",
			seed:     []string{"only"},
			index:    -1,
			wantOK:   false,
			wantList: []string{"only"},
		},
		{
			name:     "empty store",
			seed:     []string{},
			index:    0,
			wantOK:   false,
			wantList: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewChatGPTBotStore(&mockCompletionCreator{})
			for _, p := range tc.seed {
				store.CreatePromptEntry(p)
			}

			ok := store.DeletePromptAt(tc.index)
			assert.Equal(t, tc.wantOK, ok)

			store.mu.RLock()
			assert.Equal(t, tc.wantList, store.prompts)
			store.mu.RUnlock()
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP handler tests: POST /create
// ---------------------------------------------------------------------------

func TestCreatePromptHandler(t *testing.T) {
	tests := []struct {
		name           string
		body           any
		wantStatus     int
		wantBodyKey    string
		wantBodyValue  string
		wantListLength int
	}{
		{
			name:           "valid prompt returns 201 and success message",
			body:           map[string]any{"prompt": "tell me a joke"},
			wantStatus:     http.StatusCreated,
			wantBodyKey:    "message",
			wantBodyValue:  "Prompt created successfully",
			wantListLength: 1,
		},
		{
			name:           "missing prompt key returns 400",
			body:           map[string]any{},
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantListLength: 0,
		},
		{
			name:           "null prompt returns 400",
			body:           map[string]any{"prompt": nil},
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantListLength: 0,
		},
		{
			name:           "empty string prompt returns 400",
			body:           map[string]any{"prompt": ""},
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantListLength: 0,
		},
		{
			name:           "non-string prompt value returns 400",
			body:           map[string]any{"prompt": 42},
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantListLength: 0,
		},
		{
			name:           "invalid JSON body returns 400",
			body:           nil, // signals raw invalid JSON
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantListLength: 0,
		},
		{
			name:           "multiple valid creates accumulate in list",
			body:           map[string]any{"prompt": "first"},
			wantStatus:     http.StatusCreated,
			wantBodyKey:    "message",
			wantBodyValue:  "Prompt created successfully",
			wantListLength: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockCompletionCreator{}
			h, router := newHandlerWithMock(mock)

			var reqBody []byte
			if tc.body == nil {
				reqBody = []byte("not json{{")
			} else {
				var err error
				reqBody, err = json.Marshal(tc.body)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)

			body := decodeBody(t, rec.Body.Bytes())
			assert.Equal(t, tc.wantBodyValue, body[tc.wantBodyKey])

			h.store.mu.RLock()
			assert.Equal(t, tc.wantListLength, len(h.store.prompts))
			h.store.mu.RUnlock()
		})
	}
}

// TestCreatePromptHandlerMultiple verifies the list grows by exactly 1 per
// successful call (invariant).
func TestCreatePromptHandlerMultiple(t *testing.T) {
	mock := &mockCompletionCreator{}
	h, router := newHandlerWithMock(mock)

	prompts := []string{"alpha", "beta", "gamma"}
	for i, p := range prompts {
		body, _ := json.Marshal(map[string]any{"prompt": p})
		req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		h.store.mu.RLock()
		assert.Equal(t, i+1, len(h.store.prompts))
		h.store.mu.RUnlock()
	}

	h.store.mu.RLock()
	assert.Equal(t, prompts, h.store.prompts)
	h.store.mu.RUnlock()
}

// ---------------------------------------------------------------------------
// HTTP handler tests: GET