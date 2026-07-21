// Package internal implements the ChatGPT prompt management REST API.
//
// MIGRATION_NOTE: The original source was a Flask application (main.py) that
// stored a list of text prompts in memory and forwarded them to the OpenAI
// Completion API. It has been ported to idiomatic Go using the chi router with
// dependency injection for the prompt store, the LLM client, and the HTTP
// handlers. The verb-in-path URL contract is preserved exactly:
//   POST   /create
//   GET    /get/{prompt_index}
//   PUT    /update/{prompt_index}
//   DELETE /delete/{prompt_index}
//
// MIGRATION_NOTE: The Python code mutated a module-level singleton without any
// synchronization. Flask's default dev server is single-threaded, but Go's
// net/http serves each request in its own goroutine, so the in-memory store is
// guarded with a sync.RWMutex to remain race-free.
//
// MIGRATION_NOTE: The original called openai.Completion.create directly. That
// concern is abstracted behind the LLMClient interface so the OpenAI
// integration can be injected (and mocked in tests). A stub implementation is
// provided; wiring a real OpenAI client requires manual review of the API key
// configuration and the desired SDK.
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ErrInvalidPromptIndex indicates that a prompt index is out of range.
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// LLMClient abstracts the language-model completion call so it can be injected
// and mocked in tests.
type LLMClient interface {
	// Complete returns the model completion for the given prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// PromptStore stores user-provided prompts in memory in a concurrency-safe way.
//
// MIGRATION_NOTE: This replaces the ChatGPTBotAPI.prompts list from the Python
// source. Deletion preserves the positional-index semantics of the original
// (Python's `del self.prompts[i]`).
type PromptStore struct {
	mu      sync.RWMutex
	prompts []string
}

// NewPromptStore constructs an empty PromptStore.
func NewPromptStore() *PromptStore {
	return &PromptStore{prompts: make([]string, 0)}
}

// Create appends a new prompt and returns its index.
func (s *PromptStore) Create(prompt string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
	return len(s.prompts) - 1
}

// Get returns the prompt at the given index, or ErrInvalidPromptIndex if the
// index is out of range.
func (s *PromptStore) Get(index int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= len(s.prompts) {
		return "", ErrInvalidPromptIndex
	}
	return s.prompts[index], nil
}

// Update replaces the prompt at the given index. It returns
// ErrInvalidPromptIndex if the index is out of range.
func (s *PromptStore) Update(index int, newPrompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts[index] = newPrompt
	return nil
}

// Delete removes the prompt at the given index. It returns
// ErrInvalidPromptIndex if the index is out of range.
func (s *PromptStore) Delete(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return nil
}

// stubLLMClient is a placeholder LLMClient used when no real client is injected.
//
// MIGRATION_NOTE: Replace this with a real OpenAI-backed implementation. The
// original used engine "text-davinci-002" with max_tokens=150.
type stubLLMClient struct{}

// Complete echoes back a canned completion for the supplied prompt.
func (stubLLMClient) Complete(_ context.Context, prompt string) (string, error) {
	return fmt.Sprintf("completion for: %s", prompt), nil
}

// PromptHandler wires HTTP requests to the PromptStore and LLMClient.
type PromptHandler struct {
	store *PromptStore
	llm   LLMClient
}

// NewPromptHandler constructs a PromptHandler with the given dependencies. If
// llm is nil, a stub client is used.
func NewPromptHandler(store *PromptStore, llm LLMClient) *PromptHandler {
	if store == nil {
		store = NewPromptStore()
	}
	if llm == nil {
		llm = stubLLMClient{}
	}
	return &PromptHandler{store: store, llm: llm}
}

// createRequest is the JSON body for POST /create.
type createRequest struct {
	Prompt string `json:"prompt"`
}

// updateRequest is the JSON body for PUT /update/{prompt_index}.
type updateRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// writeJSON serializes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parseIndex extracts and validates the {prompt_index} URL parameter.
func parseIndex(r *http.Request) (int, error) {
	raw := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid prompt_index %q: %w", raw, err)
	}
	return idx, nil
}

// CreatePrompt handles POST /create.
func (h *PromptHandler) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	h.store.Create(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// GetResponse handles GET /get/{prompt_index}.
func (h *PromptHandler) GetResponse(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		// MIGRATION_NOTE: Flask's <int:...> converter 404s on non-integer
		// input; here we return the original "Invalid prompt index" message
		// with a 200 to match the Python response shape for out-of-range
		// values.
		writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
		return
	}

	prompt, err := h.store.Get(idx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
		return
	}

	completion, err := h.llm.Complete(r.Context(), prompt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate response"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": completion})
}

// DeletePrompt handles DELETE /delete/{prompt_index}.
func (h *PromptHandler) DeletePrompt(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	if err := h.store.Delete(idx); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// UpdatePrompt handles PUT /update/{prompt_index}.
func (h *PromptHandler) UpdatePrompt(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}

	if err := h.store.Update(idx, req.NewPrompt); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// buildRouter constructs the fully-wired HTTP router for the application. It is
// called directly by cmd/server/main.go.
func buildRouter() http.Handler {
	store := NewPromptStore()

	// MIGRATION_NOTE: A stub LLM client is injected here. Replace with a real
	// OpenAI-backed implementation and source the API key from configuration
	// (the original hard-coded "YOUR_CHATGPT_API_KEY_HERE", which must not be
	// committed).
	handler := NewPromptHandler(store, stubLLMClient{})

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	r.Post("/create", handler.CreatePrompt)
	r.Get("/get/{prompt_index}", handler.GetResponse)
	r.Delete("/delete/{prompt_index}", handler.DeletePrompt)
	r.Put("/update/{prompt_index}", handler.UpdatePrompt)

	return r
}
