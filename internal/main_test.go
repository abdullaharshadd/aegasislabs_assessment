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
// Mock Completer
// ---------------------------------------------------------------------------

type mockCompleter struct {
	response string
	err      error
	called   bool
	lastPrompt string
}

func (m *mockCompleter) Complete(_ context.Context, prompt string) (string, error) {
	m.called = true
	m.lastPrompt = prompt
	return m.response, m.err
}

// ---------------------------------------------------------------------------
// Helper: build a router backed by a given ChatGPTBotAPI
// ---------------------------------------------------------------------------

func buildTestRouter(api *ChatGPTBotAPI) http.Handler {
	r := newChiRouter(api)
	return r
}

// newChiRouter mirrors buildRouter but accepts an external *ChatGPTBotAPI so
// tests can inject a mock completer.
func newChiRouter(api *ChatGPTBotAPI) http.Handler {
	// We re-use the same chi wiring as buildRouter but without reading env vars.
	// Because buildRouter is unexported and wires its own API, we recreate the
	// routing here inline using the public handler methods.
	import_chi_router := func() http.Handler {
		// We cannot import chi directly from the test file without it being in
		// go.mod, so we call buildRouter indirectly by using the package-level
		// handlers. Instead, we construct a plain http.ServeMux.
		mux := http.NewServeMux()
		mux.HandleFunc("/create", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.NotFound(w, r)
				return
			}
			api.CreatePromptHandler(w, r)
		})
		// For chi URL params we need the real chi router; fall back to using
		// a thin wrapper that sets the chi context.
		return mux
	}
	_ = import_chi_router // suppress unused warning; we use chi below.

	// Use the same chi router construction as the production code.
	// This requires chi to be available (it is, since it's in go.mod).
	return buildAPIRouter(api)
}

// buildAPIRouter replicates the route wiring from buildRouter with an
// injectable *ChatGPTBotAPI.
func buildAPIRouter(api *ChatGPTBotAPI) http.Handler {
	// Import chi inline via the already-imported package used by main.go.
	// Since we're in the same package we can call helpers defined in main.go
	// (writeJSON, parsePromptIndex) and build our own chi router.
	r := chiNewRouter()
	r.Post("/create", api.CreatePromptHandler)
	r.Get("/get/{prompt_index}", api.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", api.DeletePromptHandler)
	r.Put("/update/{prompt_index}", api.UpdatePromptHandler)
	return r
}

// ---------------------------------------------------------------------------
// Unit tests: ChatGPTBotAPI methods
// ---------------------------------------------------------------------------

func TestCreatePromptEntry(t *testing.T) {
	tests := []struct {
		name          string
		initial       []string
		prompt        string
		wantLen       int
		wantLastPrompt string
	}{
		{
			name:          "append to empty list",
			initial:       nil,
			prompt:        "hello world",
			wantLen:       1,
			wantLastPrompt: "hello world",
		},
		{
			name:          "append to non-empty list",
			initial:       []string{"first"},
			prompt:        "second",
			wantLen:       2,
			wantLastPrompt: "second",
		},
		{
			name:          "append empty string",
			initial:       []string{"a"},
			prompt:        "",
			wantLen:       2,
			wantLastPrompt: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc := &mockCompleter{}
			api := NewChatGPTBotAPI(mc)
			api.prompts = append(api.prompts, tc.initial...)

			api.CreatePromptEntry(tc.prompt)

			assert.Len(t, api.prompts, tc.wantLen)
			assert.Equal(t, tc.wantLastPrompt, api.prompts[len(api.prompts)-1])
		})
	}
}

func TestGetResponseFor(t *testing.T) {
	tests := []struct {
		name           string
		prompts        []string
		index          int
		mockResponse   string
		mockErr        error
		wantResponse   string
		wantErr        bool
		wantCompleterCalled bool
	}{
		{
			name:                "valid index returns completion",
			prompts:             []string{"tell me a joke"},
			index:               0,
			mockResponse:        "Why did the chicken cross the road?",
			mockErr:             nil,
			wantResponse:        "Why did the chicken cross the road?",
			wantErr:             false,
			wantCompleterCalled: true,
		},
		{
			name:                "negative index returns Invalid prompt index",
			prompts:             []string{"prompt"},
			index:               -1,
			wantResponse:        "Invalid prompt index",
			wantErr:             false,
			wantCompleterCalled: false,
		},
		{
			name:                "index equal to length returns Invalid prompt index",
			prompts:             []string{"prompt"},
			index:               1,
			wantResponse:        "Invalid prompt index",
			wantErr:             false,
			wantCompleterCalled: false,
		},
		{
			name:                "index greater than length returns Invalid prompt index",
			prompts:             []string{"a", "b"},
			index:               5,
			wantResponse:        "Invalid prompt index",
			wantErr:             false,
			wantCompleterCalled: false,
		},
		{
			name:                "completer error propagates",
			prompts:             []string{"prompt"},
			index:               0,
			mockResponse:        "",
			mockErr:             fmt.Errorf("API failure"),
			wantResponse:        "",
			wantErr:             true,
			wantCompleterCalled: true,
		},
		{
			name:                "empty prompts list",
			prompts:             []string{},
			index:               0,
			wantResponse:        "Invalid prompt index",
			wantErr:             false,
			wantCompleterCalled: false,
		},
		{
			name:                "last valid index",
			prompts:             []string{"a", "b", "c"},
			index:               2,
			mockResponse:        "completion for c",
			wantResponse:        "completion for c",
			wantErr:             false,
			wantCompleterCalled: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc := &mockCompleter{response: tc.mockResponse, err: tc.mockErr}
			api := NewChatGPTBotAPI(mc)
			api.prompts = append(api.prompts, tc.prompts...)

			got, err := api.GetResponseFor(context.Background(), tc.index)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.wantResponse, got)
			assert.Equal(t, tc.wantCompleterCalled, mc.called)
		})
	}
}

func TestUpdatePromptEntry(t *testing.T) {
	tests := []struct {
		name          string
		initial       []string
		index         int
		newPrompt     string
		wantReturn    string
		wantPrompts   []string
	}{
		{
			name:        "valid index updates prompt",
			initial:     []string{"old"},
			index:       0,
			newPrompt:   "new",
			wantReturn:  "Prompt updated successfully",
			wantPrompts: []string{"new"},
		},
		{
			name:        "negative index returns error string",
			initial:     []string{"a"},
			index:       -1,
			newPrompt:   "x",
			wantReturn:  "Invalid prompt index",
			wantPrompts: []string{"a"},
		},
		{
			name:        "index equal to length returns error string",
			initial:     []string{"a"},
			index:       1,
			newPrompt:   "x",
			wantReturn:  "Invalid prompt index",
			wantPrompts: []string{"a"},
		},
		{
			name:        "middle index update",
			initial:     []string{"a", "b", "c"},
			index:       1,
			newPrompt:   "B",
			wantReturn:  "Prompt updated successfully",
			wantPrompts: []string{"a", "B", "c"},
		},
		{
			name:        "collection length unchanged after update",
			initial:     []string{"x", "y"},
			index:       0,
			newPrompt:   "z",
			wantReturn:  "Prompt updated successfully",
			wantPrompts: []string{"z", "y"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc := &mockCompleter{}
			api := NewChatGPTBotAPI(mc)
			api.prompts = append(api.prompts, tc.initial...)

			got := api.UpdatePromptEntry(tc.index, tc.newPrompt)

			assert.Equal(t, tc.wantReturn, got)
			assert.Equal(t, tc.wantPrompts, api.prompts)
		})
	}
}

func TestDeletePromptEntry(t *testing.T) {
	tests := []struct {
		name        string
		initial     []string
		index       int
		wantReturn  string
		wantPrompts []string
	}{
		{
			name:        "delete only element",
			initial:     []string{"a"},
			index:       0,
			wantReturn:  "Prompt deleted successfully",
			wantPrompts: []string{},
		},
		{
			name:        "delete first element shifts others",
			initial:     []string{"a", "b", "c"},
			index:       0,
			wantReturn:  "Prompt deleted successfully",
			wantPrompts: []string{"b", "c"},
		},
		{
			name:        "delete last element",
			initial:     []string{"a", "b", "c"},
			index:       2,
			wantReturn:  "Prompt deleted successfully",
			wantPrompts: []string{"a", "b"},
		},
		{
			name:        "delete middle element",
			initial:     []string{"a", "b", "c"},
			index:       1,
			wantReturn:  "Prompt deleted successfully",
			wantPrompts: []string{"a", "c"},
		},
		{
			name:        "negative index returns error",
			initial:     []string{"a"},
			index:       -1,
			wantReturn:  "Invalid prompt index",
			wantPrompts: []string{"a"},
		},
		{
			name:        "index equal to length returns error",
			initial:     []string{"a"},
			index:       1,
			wantReturn:  "Invalid prompt index",
			wantPrompts: []string{"a"},
		},
		{
			name:        "empty list returns error",
			initial:     []string{},
			index:       0,
			wantReturn:  "Invalid prompt index",
			wantPrompts: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc := &mockCompleter{}
			api := NewChatGPTBotAPI(mc)
			api.prompts = append(api.prompts, tc.initial...)

			got := api.DeletePromptEntry(tc.index)

			assert.Equal(t, tc.wantReturn, got)
			if tc.wantPrompts == nil {
				tc.wantPrompts = []string{}
			}
			assert.Equal(t, tc.wantPrompts, api.prompts)
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP handler tests
// ---------------------------------------------------------------------------

func setupRouter(t *testing.T, mc *mockCompleter, initialPrompts []string) (http.Handler, *ChatGPTBotAPI) {
	t.Helper()
	api := NewChatGPTBotAPI(mc)
	api.prompts = append(api.prompts, initialPrompts...)
	return buildAPIRouter(api), api
}

func decodeJSON(t *testing.T, body *bytes.Buffer) map[string]string {
	t.Helper()
	var m map[string]string
	require.NoError(t, json.NewDecoder(body).Decode(&m))
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
		wantPromptsLen int
		initialPrompts []string
	}{
		{
			name:           "valid prompt creates entry",
			body:           `{"prompt":"hello"}`,
			wantStatus:     http.StatusCreated,
			wantBodyKey:    "message",
			wantBodyValue:  "Prompt created successfully",
			wantPromptsLen: 1,
			initialPrompts: nil,
		},
		{
			name:           "appends to existing prompts",
			body:           `{"prompt":"second"}`,
			wantStatus:     http.StatusCreated,
			wantBodyKey:    "message",
			wantBodyValue:  "Prompt created successfully",
			wantPromptsLen: 2,
			initialPrompts: []string{"first"},
		},
		{
			name:           "missing prompt key returns 400",
			body:           `{}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantPromptsLen: 0,
			initialPrompts: nil,
		},
		{
			name:           "empty prompt value returns 400",
			body:           `{"prompt":""}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantPromptsLen: 0,
			initialPrompts: nil,
		},
		{
			name:           "malformed JSON returns 400",
			body:           `not-json`,
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantPromptsLen: 0,
			initialPrompts: nil,
		},
		{
			name:           "no body returns 400",
			body:           "",
			wantStatus:     http.StatusBadRequest,
			wantBodyKey:    "error",
			wantBodyValue:  "Prompt not provided",
			wantPromptsLen: 0,
			initialPrompts: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc := &mockCompleter{}
			router, api := setupRouter(t, mc, tc.initialPrompts)

			req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type