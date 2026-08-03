package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// invalidPromptIndexMessage is the message returned when a prompt index is out
// of range. MIGRATION_NOTE: The original Flask app returns this string with an
// HTTP 200 status (not 404); that behavior is preserved intentionally.
const invalidPromptIndexMessage = "Invalid prompt index"

// completionEngine is the OpenAI completion engine used for generating
// responses, mirroring the "text-davinci-002" engine from the source app.
const completionEngine = "text-davinci-002"

// completionMaxTokens is the max_tokens value used for OpenAI completions.
const completionMaxTokens = 150

// completionCreator abstracts the OpenAI completion call so the store can be
// tested without hitting the real API. It returns the generated text for the
// given prompt.
type completionCreator interface {
	// CreateCompletion returns the generated completion text for prompt.
	CreateCompletion(ctx context.Context, prompt string) (string, error)
}

// ChatGPTBotStore holds the in-memory prompt state and coordinates access to
// the OpenAI completion API. It is the Go equivalent of the source
// ChatGPTBotAPI class, but is concurrency-safe since Go HTTP handlers run in
// separate goroutines.
type ChatGPTBotStore struct {
	mu       sync.RWMutex
	prompts  []string
	client   completionCreator
	apiKey   string
}

// NewChatGPTBotStore constructs a ChatGPTBotStore.
//
// MIGRATION_NOTE: The OpenAI API key is read from the OPENAI_API_KEY env var
// rather than being hard-coded. When it is empty the placeholder-500 path is
// preserved: completion calls return an error which the handler surfaces as a
// 500-equivalent response, matching the reference app's out-of-the-box
// behavior with a placeholder key.
func NewChatGPTBotStore(client completionCreator) *ChatGPTBotStore {
	return &ChatGPTBotStore{
		prompts: make([]string, 0),
		client:  client,
		apiKey:  os.Getenv("OPENAI_API_KEY"),
	}
}

// CreatePromptEntry stores a user-provided prompt for later interactions.
func (s *ChatGPTBotStore) CreatePromptEntry(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// GetResponseAt returns the OpenAI-generated response for the stored prompt at
// the given index. The boolean is false when the index is out of range.
func (s *ChatGPTBotStore) GetResponseAt(ctx context.Context, index int) (string, bool, error) {
	s.mu.RLock()
	if index < 0 || index >= len(s.prompts) {
		s.mu.RUnlock()
		return "", false, nil
	}
	prompt := s.prompts[index]
	s.mu.RUnlock()

	text, err := s.client.CreateCompletion(ctx, prompt)
	if err != nil {
		return "", true, err
	}
	return text, true, nil
}

// UpdatePromptAt replaces the prompt at the given index. The boolean is false
// when the index is out of range.
func (s *ChatGPTBotStore) UpdatePromptAt(index int, newPrompt string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts[index] = newPrompt
	return true
}

// DeletePromptAt removes the prompt at the given index. The boolean is false
// when the index is out of range.
func (s *ChatGPTBotStore) DeletePromptAt(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return true
}

// stubCompletionCreator is a placeholder completion backend used when no real
// OpenAI integration is wired up.
//
// MIGRATION_NOTE: The source app calls openai.Completion.create directly. There
// is no first-party Go OpenAI SDK bundled here, so this stub returns an error
// to preserve the placeholder-500 path when OPENAI_API_KEY is unset. Wire a
// real implementation of completionCreator (e.g. github.com/sashabaranov/go-openai)
// for production use.
type stubCompletionCreator struct {
	apiKey string
}

// CreateCompletion returns an error when no API key is configured, otherwise it
// echoes a deterministic placeholder response. REQUIRES MANUAL REVIEW: replace
// with a real OpenAI client call.
func (c stubCompletionCreator) CreateCompletion(_ context.Context, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", errMissingAPIKey
	}
	return "response for: " + prompt, nil
}

// errMissingAPIKey is returned by the stub completion creator when no API key
// is configured.
var errMissingAPIKey = &completionError{msg: "OpenAI API key not configured"}

// completionError is a simple error type for completion failures.
type completionError struct {
	msg string
}

// Error implements the error interface.
func (e *completionError) Error() string { return e.msg }

// PromptHandler bundles the store with HTTP handlers for the prompt REST API.
type PromptHandler struct {
	store *ChatGPTBotStore
}

// NewPromptHandler constructs a PromptHandler backed by the given store.
func NewPromptHandler(store *ChatGPTBotStore) *PromptHandler {
	return &PromptHandler{store: store}
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parsePromptIndex extracts and parses the prompt_index URL parameter.
func parsePromptIndex(r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return idx, true
}

// CreatePromptHandler handles POST /create. It reads the JSON body field
// "prompt", returning 400 when missing and 201 on success.
func (h *PromptHandler) CreatePromptHandler(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body = map[string]any{}
	}

	prompt, ok := coercePrompt(body["prompt"])
	if !ok || prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Prompt not provided"})
		return
	}

	h.store.CreatePromptEntry(prompt)
	writeJSON(w, http.StatusCreated, map[string]any{"message": "Prompt created successfully"})
}

// GetResponseHandler handles GET /get/{prompt_index}. It returns the OpenAI
// response for the stored prompt at the given index.
func (h *PromptHandler) GetResponseHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"response": invalidPromptIndexMessage})
		return
	}

	text, inRange, err := h.store.GetResponseAt(r.Context(), idx)
	if !inRange {
		writeJSON(w, http.StatusOK, map[string]any{"response": invalidPromptIndexMessage})
		return
	}
	if err != nil {
		// MIGRATION_NOTE: Preserves the placeholder-500 path from the source app
		// where a missing/invalid OpenAI key causes a server error.
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Invalid response from the server"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"response": text})
}

// DeletePromptHandler handles DELETE /delete/{prompt_index}.
func (h *PromptHandler) DeletePromptHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"message": invalidPromptIndexMessage})
		return
	}

	if !h.store.DeletePromptAt(idx) {
		writeJSON(w, http.StatusOK, map[string]any{"message": invalidPromptIndexMessage})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Prompt deleted successfully"})
}

// UpdatePromptHandler handles PUT /update/{prompt_index}. It reads the JSON
// body field "new_prompt", returning 400 when missing.
func (h *PromptHandler) UpdatePromptHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"message": invalidPromptIndexMessage})
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body = map[string]any{}
	}

	newPrompt, valid := coercePrompt(body["new_prompt"])
	if !valid || newPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "New prompt not provided"})
		return
	}

	if !h.store.UpdatePromptAt(idx, newPrompt) {
		writeJSON(w, http.StatusOK, map[string]any{"message": invalidPromptIndexMessage})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Prompt updated successfully"})
}

// coercePrompt converts a decoded JSON value into a prompt string. The boolean
// is false when the value is nil or not a string.
//
// MIGRATION_NOTE: This mirrors Python's data.get('prompt') semantics where a
// missing key yields None (treated here as ok=false) and a truthiness check
// then rejects empty values.
func coercePrompt(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

// buildRouter constructs the fully-wired HTTP router for the prompt API. It is
// called directly by cmd/server/main.go.
func buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	store := NewChatGPTBotStore(stubCompletionCreator{apiKey: os.Getenv("OPENAI_API_KEY")})
	h := NewPromptHandler(store)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Post("/create", h.CreatePromptHandler)
	r.Get("/get/{prompt_index}", h.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", h.DeletePromptHandler)
	r.Put("/update/{prompt_index}", h.UpdatePromptHandler)

	return r
}
