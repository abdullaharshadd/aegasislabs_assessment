package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ErrInvalidPromptIndex is returned when a prompt index is out of range.
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// ResponseGetter abstracts the dependency that turns a prompt string into a
// generated response. The concrete implementation is client.Client from
// internal/client.go, but using an interface keeps the handlers testable.
//
// MIGRATION_NOTE: In the Python original, get_response called openai.Completion
// directly on the stored prompt. Here we defer that to an injected getter so the
// OpenAI wiring lives in the already-migrated client package.
type ResponseGetter interface {
	// GetResponse returns the generated text for the supplied prompt.
	GetResponse(ctx context.Context, prompt string) (string, error)
}

// promptStore is a concurrency-safe, in-memory store of prompt strings.
//
// MIGRATION_NOTE: Replaces the Python list `self.prompts` on the global
// ChatGPTBotAPI singleton. A mutex is added because Go HTTP handlers run
// concurrently, whereas Flask's default dev server did not require it.
type promptStore struct {
	mu      sync.RWMutex
	prompts []string
}

// newPromptStore constructs an empty promptStore.
func newPromptStore() *promptStore {
	return &promptStore{prompts: make([]string, 0)}
}

// Create appends a new prompt to the store.
func (s *promptStore) Create(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// Get returns the prompt at the given index, or an error if out of range.
func (s *promptStore) Get(index int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= len(s.prompts) {
		return "", ErrInvalidPromptIndex
	}
	return s.prompts[index], nil
}

// Update replaces the prompt at the given index, or returns an error if out of range.
func (s *promptStore) Update(index int, newPrompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts[index] = newPrompt
	return nil
}

// Delete removes the prompt at the given index, or returns an error if out of range.
func (s *promptStore) Delete(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return nil
}

// PromptServer holds the dependencies for the prompt CRUD HTTP handlers.
type PromptServer struct {
	store  *promptStore
	getter ResponseGetter
}

// NewPromptServer constructs a PromptServer with an empty in-memory store and the
// supplied ResponseGetter (typically a *client.Client).
func NewPromptServer(getter ResponseGetter) *PromptServer {
	return &PromptServer{
		store:  newPromptStore(),
		getter: getter,
	}
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parseIndex extracts and validates the prompt_index URL parameter.
func parseIndex(r *http.Request) (int, error) {
	raw := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid prompt_index %q: %w", raw, err)
	}
	return idx, nil
}

// createPromptRequest is the JSON body for POST /create.
type createPromptRequest struct {
	Prompt string `json:"prompt"`
}

// updatePromptRequest is the JSON body for PUT /update/{prompt_index}.
type updatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// CreatePrompt handles POST /create, storing a new prompt from the JSON body.
func (s *PromptServer) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	var req createPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	s.store.Create(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// GetResponse handles GET /get/{prompt_index}, returning the generated response
// for the stored prompt.
func (s *PromptServer) GetResponse(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		// MIGRATION_NOTE: Flask's <int:prompt_index> converter 404s on non-integer
		// paths before reaching the handler. Here we return the same class of error
		// as an invalid index for consistency with the original response shape.
		writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
		return
	}

	prompt, err := s.store.Get(idx)
	if err != nil {
		// Preserve original behaviour: invalid index returns 200 with a message string.
		writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	response, err := s.getter.GetResponse(ctx, prompt)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("failed to get response: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

// DeletePrompt handles DELETE /delete/{prompt_index}.
func (s *PromptServer) DeletePrompt(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	if err := s.store.Delete(idx); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// UpdatePrompt handles PUT /update/{prompt_index}, replacing the stored prompt.
func (s *PromptServer) UpdatePrompt(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}

	var req updatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}

	if err := s.store.Update(idx, req.NewPrompt); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// noopResponseGetter is a fallback ResponseGetter used when no real OpenAI-backed
// getter is injected. It keeps BuildRouter usable without external configuration.
//
// MIGRATION_NOTE: The original code hard-coded "YOUR_CHATGPT_API_KEY_HERE". A real
// deployment should construct a client.Client (internal/client.go) and pass it to
// NewPromptServer instead of relying on this stub.
type noopResponseGetter struct{}

// GetResponse returns an error indicating the getter is not configured.
func (n noopResponseGetter) GetResponse(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("no ResponseGetter configured: set OPENAI_API_KEY and inject a real client")
}

// BuildRouter constructs the fully-wired HTTP handler for the application.
//
// This exported wrapper is required by cmd/server/main.go.
func BuildRouter() http.Handler {
	return buildRouter()
}

// buildRouter constructs the fully-wired HTTP handler for the application.
func buildRouter() http.Handler {
	server := NewPromptServer(noopResponseGetter{})
	return buildRouterWith(server)
}

// buildRouterWith wires the routes for the supplied PromptServer. Extracted so
// tests can inject a fake ResponseGetter.
func buildRouterWith(server *PromptServer) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Prompt CRUD routes migrated from main.py's Flask app.
	r.Post("/create", server.CreatePrompt)
	r.Get("/get/{prompt_index}", server.GetResponse)
	r.Delete("/delete/{prompt_index}", server.DeletePrompt)
	r.Put("/update/{prompt_index}", server.UpdatePrompt)

	return r
}