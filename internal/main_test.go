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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers / mocks
// ---------------------------------------------------------------------------

// mockCompleter is a configurable Completer used across tests.
type mockCompleter struct {
	// calls records every prompt passed to Complete.
	calls []string
	// response is the value returned on a successful completion.
	response string
	// err is returned instead of response when non-nil.
	err error
}

func (m *mockCompleter) Complete(prompt string) (string, error) {
	m.calls = append(m.calls, prompt)
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// newBot constructs a ChatGPTBotAPI with the given completer and pre-seeds
// it with the supplied prompts.
func newBot(completer Completer, prompts ...string) *ChatGPTBotAPI {
	bot := NewChatGPTBotAPI(completer)
	for _, p := range prompts {
		bot.AddPrompt(p)
	}
	return bot
}

// jsonBody encodes v as a JSON reader (for request bodies).
func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

// decodeJSON decodes the response body into the supplied target.
func decodeJSON(t *testing.T, body []byte, target any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(body, target))
}

// ---------------------------------------------------------------------------
// Unit tests – ChatGPTBotAPI methods
// ---------------------------------------------------------------------------

func TestAddPrompt(t *testing.T) {
	tests := []struct {
		name           string
		initial        []string
		add            string
		wantLen        int
		wantLastPrompt string
	}{
		{
			name:           "adds first prompt to empty store",
			initial:        nil,
			add:            "hello world",
			wantLen:        1,
			wantLastPrompt: "hello world",
		},
		{
			name:           "appends to existing prompts",
			initial:        []string{"first"},
			add:            "second",
			wantLen:        2,
			wantLastPrompt: "second",
		},
		{
			name:           "multiple additions preserve order",
			initial:        []string{"a", "b"},
			add:            "c",
			wantLen:        3,
			wantLastPrompt: "c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bot := newBot(nil, tc.initial...)
			bot.AddPrompt(tc.add)

			bot.mu.RLock()
			defer bot.mu.RUnlock()
			assert.Len(t, bot.prompts, tc.wantLen)
			assert.Equal(t, tc.wantLastPrompt, bot.prompts[len(bot.prompts)-1])
		})
	}
}

func TestGetResponse(t *testing.T) {
	tests := []struct {
		name          string
		prompts       []string
		index         int
		completerResp string
		completerErr  error
		wantResp      string
		wantErr       error
		wantAPICalled bool
	}{
		{
			name:          "valid index returns completion",
			prompts:       []string{"what is Go?"},
			index:         0,
			completerResp: "Go is a programming language",
			wantResp:      "Go is a programming language",
			wantAPICalled: true,
		},
		{
			name:          "valid index non-zero",
			prompts:       []string{"first", "second prompt"},
			index:         1,
			completerResp: "completion for second",
			wantResp:      "completion for second",
			wantAPICalled: true,
		},
		{
			name:    "negative index returns ErrInvalidPromptIndex",
			prompts: []string{"a"},
			index:   -1,
			wantErr: ErrInvalidPromptIndex,
		},
		{
			name:    "index equal to length returns ErrInvalidPromptIndex",
			prompts: []string{"a"},
			index:   1,
			wantErr: ErrInvalidPromptIndex,
		},
		{
			name:    "index out of range on empty store",
			prompts: nil,
			index:   0,
			wantErr: ErrInvalidPromptIndex,
		},
		{
			name:          "completer error is propagated",
			prompts:       []string{"prompt"},
			index:         0,
			completerErr:  errors.New("upstream error"),
			wantErr:       errors.New("completion failed: upstream error"),
			wantAPICalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mc := &mockCompleter{
				response: tc.completerResp,
				err:      tc.completerErr,
			}
			bot := newBot(mc, tc.prompts...)

			resp, err := bot.GetResponse(tc.index)

			if tc.wantErr != nil {
				require.Error(t, err)
				if tc.wantErr == ErrInvalidPromptIndex {
					assert.ErrorIs(t, err, ErrInvalidPromptIndex)
				} else {
					assert.Contains(t, err.Error(), tc.completerErr.Error())
				}
				assert.Empty(t, resp)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResp, resp)
			}

			if tc.wantAPICalled {
				assert.Len(t, mc.calls, 1)
				assert.Equal(t, tc.prompts[tc.index], mc.calls[0])
			} else {
				assert.Empty(t, mc.calls, "API should not have been called")
			}

			// Invariant: prompt list unchanged
			bot.mu.RLock()
			assert.Len(t, bot.prompts, len(tc.prompts))
			bot.mu.RUnlock()
		})
	}
}

func TestUpdatePrompt(t *testing.T) {
	tests := []struct {
		name      string
		prompts   []string
		index     int
		newPrompt string
		wantErr   error
		wantList  []string
	}{
		{
			name:      "valid index updates prompt",
			prompts:   []string{"old"},
			index:     0,
			newPrompt: "new",
			wantList:  []string{"new"},
		},
		{
			name:      "updates middle element",
			prompts:   []string{"a", "b", "c"},
			index:     1,
			newPrompt: "B",
			wantList:  []string{"a", "B", "c"},
		},
		{
			name:      "negative index returns error",
			prompts:   []string{"a"},
			index:     -1,
			newPrompt: "x",
			wantErr:   ErrInvalidPromptIndex,
			wantList:  []string{"a"},
		},
		{
			name:      "index equal to length returns error",
			prompts:   []string{"a"},
			index:     1,
			newPrompt: "x",
			wantErr:   ErrInvalidPromptIndex,
			wantList:  []string{"a"},
		},
		{
			name:      "empty store returns error",
			prompts:   nil,
			index:     0,
			newPrompt: "x",
			wantErr:   ErrInvalidPromptIndex,
			wantList:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bot := newBot(nil, tc.prompts...)
			err := bot.UpdatePrompt(tc.index, tc.newPrompt)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, ErrInvalidPromptIndex)
			} else {
				require.NoError(t, err)
			}

			bot.mu.RLock()
			// Invariant: list length never changes
			assert.Len(t, bot.prompts, len(tc.wantList))
			for i, p := range tc.wantList {
				assert.Equal(t, p, bot.prompts[i])
			}
			bot.mu.RUnlock()
		})
	}
}

func TestDeletePrompt(t *testing.T) {
	tests := []struct {
		name     string
		prompts  []string
		index    int
		wantErr  error
		wantList []string
	}{
		{
			name:     "deletes only element",
			prompts:  []string{"only"},
			index:    0,
			wantList: []string{},
		},
		{
			name:     "deletes first element",
			prompts:  []string{"a", "b", "c"},
			index:    0,
			wantList: []string{"b", "c"},
		},
		{
			name:     "deletes middle element",
			prompts:  []string{"a", "b", "c"},
			index:    1,
			wantList: []string{"a", "c"},
		},
		{
			name:     "deletes last element",
			prompts:  []string{"a", "b", "c"},
			index:    2,
			wantList: []string{"a", "b"},
		},
		{
			name:     "negative index returns error",
			prompts:  []string{"a"},
			index:    -1,
			wantErr:  ErrInvalidPromptIndex,
			wantList: []string{"a"},
		},
		{
			name:     "index equal to length returns error",
			prompts:  []string{"a"},
			index:    1,
			wantErr:  ErrInvalidPromptIndex,
			wantList: []string{"a"},
		},
		{
			name:     "empty store returns error",
			prompts:  nil,
			index:    0,
			wantErr:  ErrInvalidPromptIndex,
			wantList: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bot := newBot(nil, tc.prompts...)
			initialLen := len(tc.prompts)
			err := bot.DeletePrompt(tc.index)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, ErrInvalidPromptIndex)
				// List must not have changed
				bot.mu.RLock()
				assert.Len(t, bot.prompts, initialLen)
				bot.mu.RUnlock()
			} else {
				require.NoError(t, err)
				bot.mu.RLock()
				assert.Len(t, bot.prompts, initialLen-1)
				for i, p := range tc.wantList {
					assert.Equal(t, p, bot.prompts[i])
				}
				bot.mu.RUnlock()
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HTTP handler tests via buildRouter / httptest
// ---------------------------------------------------------------------------

// newTestServer returns an httptest.Server backed by the production router,
// but with the given completer injected. Because buildRouter creates its own
// bot internally, we test via a custom mini-router instead – reusing the same
// handler wiring as buildRouter but with an injectable bot.
func newTestRouter(completer Completer) http.Handler {
	bot := NewChatGPTBotAPI(completer)

	r := newChiRouter(bot)
	return r
}

// newChiRouter builds the same route table as buildRouter but against a caller-
// supplied bot so tests can inject their own completer / state.
func newChiRouter(bot *ChatGPTBotAPI) http.Handler {
	// Re-use buildRouter's exact wiring but with our own bot.
	// We replicate the route registration here to allow injection.
	import_ := func() http.Handler {
		// chi is imported transitively via the main file; we just re-build.
		return buildRouterWithBot(bot)
	}
	return import_()
}

// buildRouterWithBot mirrors buildRouter but accepts an existing bot instance.
func buildRouterWithBot(bot *ChatGPTBotAPI) http.Handler {
	// We need chi here; it is already a dependency of the package.
	// To avoid duplicating the router setup we simply delegate to a local
	// helper that mirrors the routes.
	mux := http.NewServeMux()

	mux.HandleFunc("POST /create", bot.handleCreatePrompt)
	// Because net/http ServeMux (Go 1.22+) supports method+path patterns we
	// use it to avoid importing chi again in the test file. However, chi URL
	// params would be missing. Use a thin wrapper that extracts {prompt_index}.
	// Actually – chi is already available in the package under test. We call
	// the production buildRouter but patch via the completer before construction.
	// The cleanest approach is to expose a testable builder. Since we cannot
	// modify the source file, let's use the chi router from the package directly.
	_ = mux
	return nil // replaced below
}

// init-time reset: we use the real buildRouter helper from main.go and wrap it
// so we can inject a different bot per test. The trick: we create a *real* chi
// router with the same routes but a custom bot.

func newHandler(t *testing.T, completer Completer, seedPrompts ...string) http.Handler {
	t.Helper()
	// Build the chi router the same way buildRouter does, but with injection.
	bot := NewChatGPTBotAPI(completer)
	for _, p := range seedPrompts {
		bot.AddPrompt(p)
	}
	return buildRouterForBot(bot)
}

// buildRouterForBot is the same as buildRouter but accepts an existing bot.
// We duplicate the minimal router setup here; the source cannot be modified.
func buildRouterForBot(bot *ChatGPTBotAPI) http.Handler {
	// Import chi from the already-imported dependency in the package.
	// We call the package-level buildRouter as a baseline and overlay a fresh
	// mux. Actually the cleanest approach: just re-create the chi.Router here
	// mirroring buildRouter, which is in the same package (internal), so we
	// CAN access it.
	//
	// Since this test file is in the same package, we re-use the unexported
	// helpers directly.
	r := chiNewRouter()
	r.Post("/create", bot.handleCreatePrompt)
	r.Get("/get/{prompt_index}", bot.handleGetResponse)
	r.Delete("/delete/{prompt_index}", bot.handleDeletePrompt)
	r.Put("/update/{prompt_index}", bot.handleUpdatePrompt)
	return r
}

// chiNewRouter returns a new chi.Router. We call it indirectly to avoid
// importing chi in the test file (it is already imported by the package under
// test and available via the same binary).
func chiNewRouter() interface {
	http.Handler
	Post(pattern string, handlerFn http.HandlerFunc)
	Get(pattern string, handlerFn http.HandlerFunc)
	Delete(pattern string, handlerFn http.HandlerFunc)
	Put(pattern string, handlerFn http.HandlerFunc)
} {
	// Use the chi import from the package. Since we are in the same package
	// we simply call chi.NewRouter directly.