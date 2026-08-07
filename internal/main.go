package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ErrInvalidPromptIndex is returned when a prompt index is out of range.
//
// MIGRATION_NOTE: The Python code returned the literal string "Invalid prompt
// index" as a normal (200) response body. In idiomatic Go we model this as a
// sentinel error and translate it to an appropriate HTTP status at the
// transport boundary, while preserving the exact message on the wire.
var ErrInvalidPromptIndex = errors.New("Invalid prompt index")

// PromptStore holds the in-memory list of prompts and provides ChatGPT-style
// completions for them. It replaces the Python module-level ChatGPTBotAPI
// singleton with an explicitly constructed, mutex-guarded value.
//
// MIGRATION_NOTE: The original relied on a global mutable list without any
// synchronization. Flask's default dev server is single-threaded, but Go's
// net/http serves requests concurrently, so a sync.Mutex is required to keep
// the slice access safe.
type PromptStore struct {
	mu        sync.Mutex
	prompts   []string
	completer Completer
}

// Completer abstracts the third-party completion provider so handlers can be
// tested without calling a real API.
//
// MIGRATION_NOTE: openai.Completion.create is wrapped behind this interface.
// The concrete provider call must be supplied by the caller; a nil completer
// causes GetResponseAt to return an error rather than panic.
type Completer interface {
	// Complete returns a completion for the given prompt.
	Complete(prompt string) (string, error)
}

// NewPromptStore constructs a PromptStore backed by the supplied Completer.
func NewPromptStore(completer Completer) *PromptStore {
	return &PromptStore{
		prompts:   make([]string, 0),
		completer: completer,
	}
}

// CreatePromptEntry stores a user-provided prompt for later interactions.
func (s *PromptStore) CreatePromptEntry(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// GetResponseAt returns the completion for the prompt stored at the given index.
func (s *PromptStore) GetResponseAt(index int) (string, error) {
	s.mu.Lock()
	prompt, ok := s.promptAt(index)
	s.mu.Unlock()
	if !ok {
		return "", ErrInvalidPromptIndex
	}
	if s.completer == nil {
		return "", errors.New("completion provider not configured")
	}
	text, err := s.completer.Complete(prompt)
	if err != nil {
		return "", fmt.Errorf("completion failed: %w", err)
	}
	return text, nil
}

// UpdatePromptAt replaces the prompt at the given index with a new prompt.
func (s *PromptStore) UpdatePromptAt(index int, newPrompt string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.promptAt(index); !ok {
		return "", ErrInvalidPromptIndex
	}
	s.prompts[index] = newPrompt
	return "Prompt updated successfully", nil
}

// DeletePromptAt removes the prompt at the given index.
func (s *PromptStore) DeletePromptAt(index int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.promptAt(index); !ok {
		return "", ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return "Prompt deleted successfully", nil
}

// promptAt returns the prompt at index and whether it exists. Callers must hold
// the mutex.
func (s *PromptStore) promptAt(index int) (string, bool) {
	if index < 0 || index >= len(s.prompts) {
		return "", false
	}
	return s.prompts[index], true
}

// --- DTOs ---

// createPromptRequest is the body for POST /create.
//
// MIGRATION_NOTE: The create and update paths use different JSON field names
// ("prompt" vs "new_prompt"). They are modeled as two distinct structs so the
// tags cannot be accidentally shared.
type createPromptRequest struct {
	Prompt string `json:"prompt"`
}

// updatePromptRequest is the body for PUT /update/{prompt_index}.
type updatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// --- HTTP transport helpers ---

// writeJSON writes v as a JSON response with the given status code.
//
// MIGRATION_NOTE: Content-Type must be set before WriteHeader; once WriteHeader
// is called the header map is frozen for the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parsePromptIndex extracts and validates the {prompt_index} URL parameter.
func parsePromptIndex(r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "prompt_index")
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return index, true
}

// PromptHandlers wires HTTP handlers to a PromptStore.
type PromptHandlers struct {
	store *PromptStore
}

// NewPromptHandlers constructs PromptHandlers backed by the given store.
func NewPromptHandlers(store *PromptStore) *PromptHandlers {
	return &PromptHandlers{store: store}
}

// CreatePromptHandler handles POST /create.
func (h *PromptHandlers) CreatePromptHandler(w http.ResponseWriter, r *http.Request) {
	var req createPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	h.store.CreatePromptEntry(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// GetResponseHandler handles GET /get/{prompt_index}.
func (h *PromptHandlers) GetResponseHandler(w http.ResponseWriter, r *http.Request) {
	index, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"response": ErrInvalidPromptIndex.Error()})
		return
	}
	response, err := h.store.GetResponseAt(index)
	if err != nil {
		if errors.Is(err, ErrInvalidPromptIndex) {
			writeJSON(w, http.StatusOK, map[string]string{"response": ErrInvalidPromptIndex.Error()})
			return
		}
		// MIGRATION_NOTE: Do not leak the wrapped provider error to the client.
		// Return a generic message; the internal error should be logged instead.
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "completion provider error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

// DeletePromptHandler handles DELETE /delete/{prompt_index}.
func (h *PromptHandlers) DeletePromptHandler(w http.ResponseWriter, r *http.Request) {
	index, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": ErrInvalidPromptIndex.Error()})
		return
	}
	message, err := h.store.DeletePromptAt(index)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

// UpdatePromptHandler handles PUT /update/{prompt_index}.
func (h *PromptHandlers) UpdatePromptHandler(w http.ResponseWriter, r *http.Request) {
	var req updatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	if req.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	index, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": ErrInvalidPromptIndex.Error()})
		return
	}
	message, err := h.store.UpdatePromptAt(index, req.NewPrompt)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

// noopCompleter is a placeholder Completer used until a real provider is wired.
//
// MIGRATION_NOTE: The Python code hard-coded an OpenAI API key
// ("YOUR_CHATGPT_API_KEY_HERE") and called openai.Completion.create directly.
// Hard-coding secrets is unacceptable in production. A real Completer
// implementation should be injected via NewPromptStore, reading the API key
// from configuration/environment. This noop returns an error so the missing
// provider is surfaced explicitly rather than silently returning fake data.
type noopCompleter struct{}

// Complete always returns an error indicating no provider is configured.
func (noopCompleter) Complete(string) (string, error) {
	return "", errors.New("no completion provider configured")
}

// buildRouter constructs the fully-wired chi router for the prompt API.
//
// It is called directly by cmd/server/main.go and must keep this exact name
// and signature.
func buildRouter() http.Handler {
	return BuildRouter()
}

// BuildRouter constructs and returns the fully-wired chi router for the prompt
// API. It is exported so that cmd/server/main.go can call it directly.
func BuildRouter() http.Handler {
	store := NewPromptStore(noopCompleter{})
	handlers := NewPromptHandlers(store)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Post("/create", handlers.CreatePromptHandler)
	r.Get("/get/{prompt_index}", handlers.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", handlers.DeletePromptHandler)
	r.Put("/update/{prompt_index}", handlers.UpdatePromptHandler)

	return r
}
