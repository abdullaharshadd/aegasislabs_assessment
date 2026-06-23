// Package main provides an HTTP server exposing CRUD endpoints for managing
// prompts that are proxied to an OpenAI-style completion API to act as a
// ChatGPT bot.
//
// MIGRATION_NOTE: The original Python file (main.py) was a Flask application
// that combined three concerns into a single module:
//
//  1. A ChatGPTBotAPI service holding in-memory prompts and talking to OpenAI.
//  2. Flask route handlers (@app.route) for /create, /get, /update, /delete.
//  3. A module-level app singleton and a __main__ entrypoint (app.run).
//
// In this Go port these concerns are separated:
//
//   - The OpenAI-facing CRUD logic was already migrated into package client
//     (see internal/client.py.go): Client, CreateRequest, UpdateRequest,
//     Response, NewClient, CreatePrompt, GetResponse, UpdatePrompt,
//     DeletePrompt. We DO NOT re-implement that here; we depend on it.
//
//   - This file contains the HTTP transport layer (router + handlers) and the
//     server bootstrap, which is the Go analogue of the Flask routes and the
//     __main__ guard.
//
// MIGRATION_NOTE: The original ChatGPTBotAPI stored prompts in an in-memory
// list ([]). That mutable shared state is preserved here via a PromptStore
// guarded by a sync.Mutex so it is safe under net/http's concurrent request
// handling (Flask's default dev server is effectively single-threaded for this
// code, so the original had no locking).
//
// MIGRATION_NOTE: The OpenAI API key was hardcoded as a placeholder in the
// Python source. It is externalized here via the OPENAI_API_KEY environment
// variable. NEVER hardcode credentials.
//
// REQUIRES MANUAL REVIEW: The exact integration with package client depends on
// the final signatures in internal/client.py.go (only a truncated snippet was
// provided). The handler bodies below assume the documented function names but
// may need their argument lists adjusted to match the real Client methods.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// ErrInvalidPromptIndex is returned when a prompt index is out of range. It
// mirrors the original "Invalid prompt index" string responses.
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// Completer is the minimal behaviour this server needs from the OpenAI-backed
// client. It is defined here (consumer side) so handlers depend on an
// interface rather than the concrete *client.Client.
//
// MIGRATION_NOTE: This interface should be satisfied by the migrated
// internal/client.py.go Client. Adjust the method set to match its real
// signatures.
type Completer interface {
	// Complete returns the model completion for the supplied prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// PromptStore is the in-memory replacement for the Python list of prompts. All
// access is synchronized so the store is safe for concurrent HTTP handlers.
type PromptStore struct {
	mu      sync.Mutex
	prompts []string
}

// NewPromptStore constructs an empty PromptStore.
func NewPromptStore() *PromptStore {
	return &PromptStore{prompts: make([]string, 0)}
}

// Create appends a prompt and returns its index.
func (s *PromptStore) Create(prompt string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
	return len(s.prompts) - 1
}

// Get returns the prompt at index. The bool reports whether the index was
// valid, replacing the Python "Invalid prompt index" sentinel.
func (s *PromptStore) Get(index int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return "", false
	}
	return s.prompts[index], true
}

// Update replaces the prompt at index. It returns an error if the index is out
// of range.
func (s *PromptStore) Update(index int, newPrompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts[index] = newPrompt
	return nil
}

// Delete removes the prompt at index. It returns an error if the index is out
// of range.
func (s *PromptStore) Delete(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return nil
}

// BotService bundles the prompt store and the completion client. It is the Go
// equivalent of the Python ChatGPTBotAPI class.
type BotService struct {
	store     *PromptStore
	completer Completer
}

// NewBotService constructs a BotService from a store and a Completer.
func NewBotService(store *PromptStore, completer Completer) *BotService {
	return &BotService{store: store, completer: completer}
}

// GetResponse looks up the prompt at index and asks the completer for a
// response. It returns ErrInvalidPromptIndex when the index is out of range.
func (b *BotService) GetResponse(ctx context.Context, index int) (string, error) {
	prompt, ok := b.store.Get(index)
	if !ok {
		return "", ErrInvalidPromptIndex
	}
	resp, err := b.completer.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("completing prompt %d: %w", index, err)
	}
	return resp, nil
}

// createRequest is the JSON body for POST /create.
type createRequest struct {
	Prompt string `json:"prompt"`
}

// updateRequest is the JSON body for PUT /update/{index}.
type updateRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// Server wires the BotService into an http.Handler exposing the CRUD routes.
type Server struct {
	bot *BotService
}

// NewServer constructs a Server backed by the given BotService.
func NewServer(bot *BotService) *Server {
	return &Server{bot: bot}
}

// Routes returns the configured http.Handler with all endpoints registered.
//
// MIGRATION_NOTE: Flask's <int:prompt_index> path converter is replaced with
// Go 1.22+ method-and-pattern routing ("GET /get/{index}") plus explicit
// strconv.Atoi parsing.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", s.handleCreate)
	mux.HandleFunc("GET /get/{index}", s.handleGetResponse)
	mux.HandleFunc("DELETE /delete/{index}", s.handleDelete)
	mux.HandleFunc("PUT /update/{index}", s.handleUpdate)
	return mux
}

// writeJSON serializes payload as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encoding JSON response: %v", err)
	}
}

// parseIndex extracts and validates the {index} path value.
func parseIndex(r *http.Request) (int, error) {
	raw := r.PathValue("index")
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parsing index %q: %w", raw, err)
	}
	return index, nil
}

// handleCreate implements POST /create.
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
	s.bot.store.Create(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// handleGetResponse implements GET /get/{index}.
func (s *Server) handleGetResponse(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	resp, err := s.bot.GetResponse(r.Context(), index)
	if err != nil {
		if errors.Is(err, ErrInvalidPromptIndex) {
			// MIGRATION_NOTE: The Python code returned HTTP 200 with the body
			// {"response": "Invalid prompt index"} even for bad indices. That
			// behaviour is preserved here for compatibility.
			writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
			return
		}
		log.Printf("get response for index %d: %v", index, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get response"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

// handleDelete implements DELETE /delete/{index}.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	if err := s.bot.store.Delete(index); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// handleUpdate implements PUT /update/{index}.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
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
	if err := s.bot.store.Update(index, req.NewPrompt); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// openAICompleter is a placeholder adapter that should wrap the migrated
// client.Client so it satisfies Completer.
//
// MIGRATION_NOTE: REQUIRES MANUAL REVIEW. The provided client.py.go snippet was
// truncated and its CRUD methods (CreatePrompt/GetResponse/etc.) target a
// remote REST API rather than the OpenAI Completion API directly. Wire this to
// the real OpenAI completion call (engine "text-davinci-002", max_tokens 150)
// or to client.GetResponse as appropriate.
type openAICompleter struct {
	apiKey string
}

// Complete is a stub that must be implemented against the real OpenAI client.
func (c *openAICompleter) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return "", errors.New("openAICompleter.Complete not implemented: wire to internal/client.py.go or the OpenAI SDK")
}

// run starts the HTTP server and blocks until an interrupt signal triggers a
// graceful shutdown. It is the Go equivalent of the Python `app.run(debug=True)`
// __main__ block.
func run() error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return errors.New("OPENAI_API_KEY environment variable is required")
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":5000"
	}

	store := NewPromptStore()
	completer := &openAICompleter{apiKey: apiKey}
	bot := NewBotService(store, completer)
	srv := NewServer(bot)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
