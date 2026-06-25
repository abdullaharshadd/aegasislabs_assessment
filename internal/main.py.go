// Package main provides an HTTP REST API server that wraps the OpenAI
// Completion API, allowing clients to create, retrieve, update, and delete
// text prompts and obtain ChatGPT-style completions for stored prompts.
//
// MIGRATION_NOTE: The original main.py was a Flask (not Django) application
// that used a global ChatGPTBotAPI singleton with an in-memory prompt list and
// the openai Python SDK. This migration models it idiomatically in Go:
//   - The Flask route decorators become explicit handler registrations on a
//     net/http ServeMux.
//   - The global singleton becomes a constructor-built service (NewService)
//     injected into the handlers.
//   - The in-memory prompt list is guarded by a sync.RWMutex because Go's
//     net/http serves requests concurrently (Flask's dev server defaults to
//     single-threaded request handling, so the original had no locking).
//   - The openai.Completion.create call is abstracted behind a Completer
//     interface so it can be mocked in tests. A concrete OpenAI HTTP client
//     implementation is left as a stub and flagged for manual review, since
//     the original relied on the openai Python SDK which has no direct Go
//     equivalent in this repository.
//   - The OpenAI API key is read from the OPENAI_API_KEY environment variable
//     rather than being hardcoded.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// ErrInvalidPromptIndex is returned when a prompt index is out of range.
//
// MIGRATION_NOTE: The original Python returned the literal string
// "Invalid prompt index". Here we model that condition as a sentinel error so
// callers can distinguish it via errors.Is and map it to an HTTP status.
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// ErrPromptNotProvided is returned when a request omits a required prompt.
var ErrPromptNotProvided = errors.New("prompt not provided")

// Completer abstracts the text-completion backend (e.g. the OpenAI API).
//
// MIGRATION_NOTE: The original code called openai.Completion.create directly.
// Hiding this behind an interface enables dependency injection and testing.
type Completer interface {
	// Complete returns a completion for the given prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// OpenAICompleter is a Completer backed by the OpenAI Completion API.
//
// MIGRATION_NOTE: This is a stub. The original relied on the openai Python SDK
// (engine "text-davinci-002", max_tokens 150). A real implementation must call
// the OpenAI HTTP API directly or via a Go SDK. REQUIRES MANUAL REVIEW.
type OpenAICompleter struct {
	apiKey     string
	httpClient *http.Client
}

// NewOpenAICompleter constructs an OpenAICompleter with the given API key.
func NewOpenAICompleter(apiKey string) *OpenAICompleter {
	return &OpenAICompleter{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Complete returns a completion for the given prompt.
//
// MIGRATION_NOTE: Not yet implemented. The original parameters were
// engine="text-davinci-002" and max_tokens=150. Wire this to the OpenAI API.
func (c *OpenAICompleter) Complete(ctx context.Context, prompt string) (string, error) {
	return "", errors.New("OpenAICompleter.Complete: not implemented; wire to the OpenAI API")
}

// Service stores prompts and produces completions for them.
//
// MIGRATION_NOTE: This is the Go equivalent of the Python ChatGPTBotAPI class.
type Service struct {
	mu        sync.RWMutex
	prompts   []string
	completer Completer
}

// NewService constructs a Service backed by the given Completer.
func NewService(completer Completer) *Service {
	return &Service{
		prompts:   make([]string, 0),
		completer: completer,
	}
}

// CreatePrompt stores a prompt for later interactions.
func (s *Service) CreatePrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// GetResponse returns the completion for the prompt at the given index.
func (s *Service) GetResponse(ctx context.Context, promptIndex int) (string, error) {
	s.mu.RLock()
	if promptIndex < 0 || promptIndex >= len(s.prompts) {
		s.mu.RUnlock()
		return "", ErrInvalidPromptIndex
	}
	prompt := s.prompts[promptIndex]
	s.mu.RUnlock()

	response, err := s.completer.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("get response for prompt %d: %w", promptIndex, err)
	}
	return response, nil
}

// UpdatePrompt replaces the prompt at the given index with newPrompt.
func (s *Service) UpdatePrompt(promptIndex int, newPrompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts[promptIndex] = newPrompt
	return nil
}

// DeletePrompt removes the prompt at the given index.
func (s *Service) DeletePrompt(promptIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:promptIndex], s.prompts[promptIndex+1:]...)
	return nil
}

// Handler holds the HTTP handlers for the prompt API.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler that delegates to the given Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: encode response: %v", err)
	}
}

// createPromptRequest is the request body for POST /create.
type createPromptRequest struct {
	Prompt string `json:"prompt"`
}

// updatePromptRequest is the request body for PUT /update/{index}.
type updatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// CreatePrompt handles POST /create.
func (h *Handler) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	var req createPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	h.svc.CreatePrompt(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// GetResponse handles GET /get/{index}.
func (h *Handler) GetResponse(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r.PathValue("index"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	response, err := h.svc.GetResponse(r.Context(), index)
	if err != nil {
		if errors.Is(err, ErrInvalidPromptIndex) {
			// MIGRATION_NOTE: The original returned the literal string
			// "Invalid prompt index" with HTTP 200. We preserve that body and
			// status to keep behavior identical for existing clients.
			writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

// DeletePrompt handles DELETE /delete/{index}.
func (h *Handler) DeletePrompt(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r.PathValue("index"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	if err := h.svc.DeletePrompt(index); err != nil {
		// MIGRATION_NOTE: The original always returned HTTP 200 with a message,
		// even for an invalid index. Behavior preserved.
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// UpdatePrompt handles PUT /update/{index}.
func (h *Handler) UpdatePrompt(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r.PathValue("index"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
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
	if err := h.svc.UpdatePrompt(index, req.NewPrompt); err != nil {
		// MIGRATION_NOTE: Original returned HTTP 200 with the message for an
		// invalid index. Behavior preserved.
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// parseIndex parses a path segment into a non-negative-aware integer index.
func parseIndex(raw string) (int, error) {
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse index %q: %w", raw, err)
	}
	return index, nil
}

// Routes builds an http.Handler with all API endpoints registered.
//
// MIGRATION_NOTE: Uses Go 1.22+ method-and-pattern routing in net/http. If the
// project targets an older Go version, replace with chi or gorilla/mux.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", h.CreatePrompt)
	mux.HandleFunc("GET /get/{index}", h.GetResponse)
	mux.HandleFunc("DELETE /delete/{index}", h.DeletePrompt)
	mux.HandleFunc("PUT /update/{index}", h.UpdatePrompt)
	return mux
}

// main starts the HTTP server.
//
// MIGRATION_NOTE: The original used app.run(debug=True). Here we use a standard
// http.Server with graceful-shutdown-friendly defaults. The API key is read
// from OPENAI_API_KEY instead of being hardcoded.
func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	completer := NewOpenAICompleter(apiKey)
	svc := NewService(completer)
	handler := NewHandler(svc)

	addr := ":8080"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
