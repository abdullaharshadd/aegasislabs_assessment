// Package client provides an HTTP client for consuming the prompts REST API,
// and — in this file — the *server* side that implements those routes.
//
// MIGRATION_NOTE: The original Python file (main.py) was a Flask application that
// wrapped the OpenAI Completion API. It stored prompts in memory and exposed
// CRUD-style routes. This Go file preserves that business logic:
//   - An in-memory, concurrency-safe prompt store (PromptStore).
//   - An OpenAI completion service abstraction (Completer) so the external call
//     is testable and mockable.
//   - chi HTTP handlers wired in buildRouter, matching the exact routes/methods.
//
// MIGRATION_NOTE: The original hardcoded OpenAI API key
// ("YOUR_CHATGPT_API_KEY_HERE") is a security anti-pattern. In Go we read it from
// the OPENAI_API_KEY environment variable. If unset, GetResponse returns an error
// rather than panicking.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ErrInvalidPromptIndex is returned when a prompt index is out of range.
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// ErrMissingAPIKey is returned when the OpenAI API key is not configured.
var ErrMissingAPIKey = errors.New("openai api key not configured")

// Completer abstracts the text-completion capability originally provided by
// openai.Completion.create. It exists so the server logic can be tested without
// contacting the real OpenAI API.
type Completer interface {
	// Complete returns generated text for the given prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// OpenAICompleter is a Completer backed by the OpenAI Completions endpoint.
//
// MIGRATION_NOTE: The Python code used the openai SDK directly with engine
// "text-davinci-002" and max_tokens=150. Go has no first-party OpenAI SDK in the
// standard library, so this is a thin HTTP implementation over the completions
// endpoint. The exact request/response wire format may require manual review
// against the current OpenAI API version.
type OpenAICompleter struct {
	apiKey string
	engine string
	maxTokens int
}

// NewOpenAICompleter constructs an OpenAICompleter, reading the API key from the
// OPENAI_API_KEY environment variable.
func NewOpenAICompleter() *OpenAICompleter {
	return &OpenAICompleter{
		apiKey: os.Getenv("OPENAI_API_KEY"),
		engine: "text-davinci-002",
		maxTokens: 150,
	}
}

// Complete calls the OpenAI Completions API and returns the first choice's text.
//
// MIGRATION_NOTE: This is a stub-level implementation of the external call. The
// real network request should be wired here (or replaced with an official SDK).
// It currently returns ErrMissingAPIKey when no key is set so misconfiguration is
// explicit rather than silent.
func (c *OpenAICompleter) Complete(ctx context.Context, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", ErrMissingAPIKey
	}
	// TODO(manual-review): Perform the actual HTTP request to the OpenAI
	// completions endpoint using ctx, c.engine, c.maxTokens and c.apiKey, then
	// decode response.choices[0].text.
	return "", fmt.Errorf("openai completion not implemented")
}

// PromptStore is a concurrency-safe, in-memory store of prompts.
//
// MIGRATION_NOTE: The Python code used a plain list (self.prompts) with no
// locking. Flask's dev server is effectively single-threaded per request in the
// default config, but Go's HTTP server serves requests concurrently, so a mutex
// is required to avoid data races.
type PromptStore struct {
	mu sync.RWMutex
	prompts []string
	completer Completer
}

// NewPromptStore constructs a PromptStore backed by the given Completer.
func NewPromptStore(completer Completer) *PromptStore {
	return &PromptStore{completer: completer}
}

// CreatePrompt stores a new prompt and returns its index.
func (s *PromptStore) CreatePrompt(prompt string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
	return len(s.prompts) - 1
}

// GetResponse returns the completion for the prompt at the given index.
func (s *PromptStore) GetResponse(ctx context.Context, index int) (string, error) {
	s.mu.RLock()
	if index < 0 || index >= len(s.prompts) {
		s.mu.RUnlock()
		return "", ErrInvalidPromptIndex
	}
	prompt := s.prompts[index]
	s.mu.RUnlock()

	return s.completer.Complete(ctx, prompt)
}

// UpdatePrompt replaces the prompt at the given index with newPrompt.
func (s *PromptStore) UpdatePrompt(index int, newPrompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts[index] = newPrompt
	return nil
}

// DeletePrompt removes the prompt at the given index.
func (s *PromptStore) DeletePrompt(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return nil
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parseIndex extracts the prompt_index URL parameter as an int.
func parseIndex(r *http.Request) (int, error) {
	raw := chi.URLParam(r, "prompt_index")
	return strconv.Atoi(raw)
}

// PromptHandler holds the dependencies for the prompt HTTP handlers.
type PromptHandler struct {
	store *PromptStore
}

// NewPromptHandler constructs a PromptHandler.
func NewPromptHandler(store *PromptStore) *PromptHandler {
	return &PromptHandler{store: store}
}

// Create handles POST /create.
func (h *PromptHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	h.store.CreatePrompt(body.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// GetResponse handles GET /get/{prompt_index}.
func (h *PromptHandler) GetResponse(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	resp, err := h.store.GetResponse(r.Context(), index)
	if err != nil {
		// MIGRATION_NOTE: The Python code returned "Invalid prompt index" as a
		// 200-OK response body. Here invalid index maps to a JSON response but
		// keeps status 200 to preserve original behavior for the index case.
		if errors.Is(err, ErrInvalidPromptIndex) {
			writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

// Delete handles DELETE /delete/{prompt_index}.
func (h *PromptHandler) Delete(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	if err := h.store.DeletePrompt(index); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// Update handles PUT /update/{prompt_index}.
func (h *PromptHandler) Update(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	var body struct {
		NewPrompt string `json:"new_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if body.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	if err := h.store.UpdatePrompt(index, body.NewPrompt); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// buildRouter constructs the fully-wired HTTP router for the prompts service.
//
// It is called directly by cmd/server/main.go.
func buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	store := NewPromptStore(NewOpenAICompleter())
	h := NewPromptHandler(store)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	r.Post("/create", h.Create)
	r.Get("/get/{prompt_index}", h.GetResponse)
	r.Delete("/delete/{prompt_index}", h.Delete)
	r.Put("/update/{prompt_index}", h.Update)

	return r
}
