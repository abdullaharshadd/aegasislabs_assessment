// Package server implements the prompts REST API originally defined in main.py.
//
// This file is the idiomatic Go migration of the original Flask application. The
// source defined a ChatGPTBotAPI class holding an in-memory list of prompts and
// exposed four HTTP routes:
//
//	POST   /create
//	GET    /get/{prompt_index}
//	PUT    /update/{prompt_index}
//	DELETE /delete/{prompt_index}
//
// These routes are registered verbatim below.
//
// MIGRATION_NOTE: The original code combined an OpenAI Completion call
// ("text-davinci-002") with an in-memory prompt store. The OpenAI SDK call has
// no direct standard-library equivalent, so GetResponse delegates to a pluggable
// Completer interface. A stub implementation is provided; wire in a real OpenAI
// client for production. The DEPRECATED text-davinci-002 engine should be
// reviewed and replaced with a current model.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
)

// ErrInvalidPromptIndex is returned when a prompt index is out of range.
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// Completer abstracts the text-completion backend (originally openai.Completion).
//
// MIGRATION_NOTE: The Python code called openai.Completion.create directly.
// This interface lets callers inject a real OpenAI client or a test double.
type Completer interface {
	// Complete returns a completion for the given prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// stubCompleter is a placeholder Completer that echoes the prompt.
//
// MIGRATION_NOTE: Replace with a real OpenAI-backed implementation before
// production use. The original engine "text-davinci-002" and max_tokens=150
// settings are documented here for reference.
type stubCompleter struct{}

// Complete implements Completer by returning a placeholder response.
func (stubCompleter) Complete(_ context.Context, prompt string) (string, error) {
	return fmt.Sprintf("stub completion for: %s", prompt), nil
}

// NewStubCompleter returns a Completer that echoes prompts. Intended for
// development and tests only.
func NewStubCompleter() Completer {
	return stubCompleter{}
}

// PromptStore holds the in-memory list of prompts. It is safe for concurrent
// use, replacing the unsynchronized Python list which was unsafe under Flask's
// threaded server.
type PromptStore struct {
	mu        sync.RWMutex
	prompts   []string
	completer Completer
}

// NewPromptStore constructs a PromptStore backed by the given Completer.
func NewPromptStore(completer Completer) *PromptStore {
	if completer == nil {
		completer = NewStubCompleter()
	}
	return &PromptStore{completer: completer}
}

// CreatePrompt appends a prompt to the store.
func (s *PromptStore) CreatePrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
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

// UpdatePrompt replaces the prompt at the given index.
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

// Server exposes the prompt HTTP API.
type Server struct {
	store *PromptStore
}

// NewServer constructs a Server backed by the given PromptStore.
func NewServer(store *PromptStore) *Server {
	return &Server{store: store}
}

// Routes returns an http.Handler with all API routes registered at their
// original paths.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	// Go 1.22+ pattern routing preserves the exact paths and methods.
	mux.HandleFunc("POST /create", s.handleCreate)
	mux.HandleFunc("GET /get/{prompt_index}", s.handleGet)
	mux.HandleFunc("PUT /update/{prompt_index}", s.handleUpdate)
	mux.HandleFunc("DELETE /delete/{prompt_index}", s.handleDelete)
	return mux
}

// createRequest mirrors the JSON body of POST /create.
type createRequest struct {
	Prompt string `json:"prompt"`
}

// updateRequest mirrors the JSON body of PUT /update/{prompt_index}.
type updateRequest struct {
	NewPrompt string `json:"new_prompt"`
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	s.store.CreatePrompt(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		// MIGRATION_NOTE: Flask's <int:...> converter rejected non-integer paths
		// with a 404. Here we return 400 for clarity.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	response, err := s.store.GetResponse(r.Context(), index)
	if err != nil {
		// Preserve original behavior: the Python code returned the string
		// "Invalid prompt index" with a 200 status inside the response field.
		if errors.Is(err, ErrInvalidPromptIndex) {
			writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
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
	if err := s.store.UpdatePrompt(index, req.NewPrompt); err != nil {
		// Preserve original behavior: 200 with the message string.
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	if err := s.store.DeletePrompt(index); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// parseIndex extracts and parses the {prompt_index} path value.
func parseIndex(r *http.Request) (int, error) {
	raw := r.PathValue("prompt_index")
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parsing prompt index %q: %w", raw, err)
	}
	return index, nil
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
