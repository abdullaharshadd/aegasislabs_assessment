// Package main implements a Flask-style REST API that wraps the OpenAI
// Completion API. Clients can create, retrieve, update, and delete text prompts
// and get ChatGPT-style responses for stored prompts.
//
// MIGRATION_NOTE: The original file was a Flask application with a global app
// instance, route decorators, and an in-memory list as the data store. This Go
// migration preserves the business logic while adopting idiomatic patterns:
//   - A concrete PromptService with a mutex-guarded in-memory store (the Flask
//     app was effectively single-instance global state; we make concurrency
//     safety explicit since Go's net/http serves requests concurrently).
//   - Explicit error handling and (T, error) returns instead of sentinel
//     "Invalid prompt index" strings.
//   - Context propagation into the OpenAI call.
//   - Constructor functions (NewXxx) instead of struct literals.
//   - net/http standard-library routing instead of Flask decorators.
//
// MIGRATION_NOTE: The Python code called openai.Completion.create directly. Go
// has no official OpenAI SDK bundled here, so the completion call is abstracted
// behind a CompletionClient interface. Wire in a real implementation (e.g.
// github.com/sashabaranov/go-openai) during manual review.
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
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// ErrPromptNotProvided is returned when a request omits the prompt field.
var ErrPromptNotProvided = errors.New("prompt not provided")

// CompletionClient abstracts the third-party completion API (originally
// openai.Completion.create). Implementations perform the actual network call.
//
// MIGRATION_NOTE: Replace the stub implementation with a real OpenAI client
// during manual review.
type CompletionClient interface {
	// Complete returns the generated text for the given prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// PromptService stores prompts in memory and produces completions for them.
//
// MIGRATION_NOTE: The original ChatGPTBotAPI kept a plain Python list. Because
// net/http handles requests concurrently, access to the slice is guarded by a
// mutex here.
type PromptService struct {
	mu      sync.RWMutex
	prompts []string
	client  CompletionClient
	engine  string
	maxTok  int
}

// NewPromptService constructs a PromptService with the given completion client.
func NewPromptService(client CompletionClient) *PromptService {
	return &PromptService{
		prompts: make([]string, 0),
		client:  client,
		engine:  "text-davinci-002",
		maxTok:  150,
	}
}

// CreatePrompt stores a user-provided prompt for later interactions.
func (s *PromptService) CreatePrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// GetResponse returns the completion for the prompt at the given index.
func (s *PromptService) GetResponse(ctx context.Context, promptIndex int) (string, error) {
	s.mu.RLock()
	if promptIndex < 0 || promptIndex >= len(s.prompts) {
		s.mu.RUnlock()
		return "", ErrInvalidPromptIndex
	}
	prompt := s.prompts[promptIndex]
	s.mu.RUnlock()

	text, err := s.client.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("completion request failed: %w", err)
	}
	return text, nil
}

// UpdatePrompt replaces the prompt at the given index with a new prompt.
func (s *PromptService) UpdatePrompt(promptIndex int, newPrompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts[promptIndex] = newPrompt
	return nil
}

// DeletePrompt removes the prompt at the given index.
func (s *PromptService) DeletePrompt(promptIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:promptIndex], s.prompts[promptIndex+1:]...)
	return nil
}

// stubCompletionClient is a placeholder implementation of CompletionClient.
//
// MIGRATION_NOTE: Swap this out for a real OpenAI-backed implementation. It
// exists only so the package compiles and the HTTP wiring can be exercised.
type stubCompletionClient struct {
	apiKey string
	engine string
	maxTok int
}

// newStubCompletionClient constructs a placeholder completion client.
func newStubCompletionClient(apiKey string) *stubCompletionClient {
	return &stubCompletionClient{apiKey: apiKey, engine: "text-davinci-002", maxTok: 150}
}

// Complete returns an error indicating the client is not yet implemented.
func (c *stubCompletionClient) Complete(ctx context.Context, prompt string) (string, error) {
	return "", errors.New("CompletionClient not implemented: wire in a real OpenAI client")
}

// Handler holds the HTTP handlers for the prompt API.
type Handler struct {
	svc *PromptService
}

// NewHandler constructs a Handler backed by the given PromptService.
func NewHandler(svc *PromptService) *Handler {
	return &Handler{svc: svc}
}

// writeJSON serializes v to the response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// createPromptRequest is the JSON body for POST /create.
type createPromptRequest struct {
	Prompt string `json:"prompt"`
}

// updatePromptRequest is the JSON body for PUT /update/{index}.
type updatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// CreatePrompt handles POST /create.
func (h *Handler) CreatePrompt(w http.ResponseWriter, r *http.Request) {
	var req createPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
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
	idx, ok := parseIndex(w, r, "/get/")
	if !ok {
		return
	}
	resp, err := h.svc.GetResponse(r.Context(), idx)
	if err != nil {
		if errors.Is(err, ErrInvalidPromptIndex) {
			writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

// DeletePrompt handles DELETE /delete/{index}.
func (h *Handler) DeletePrompt(w http.ResponseWriter, r *http.Request) {
	idx, ok := parseIndex(w, r, "/delete/")
	if !ok {
		return
	}
	if err := h.svc.DeletePrompt(idx); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// UpdatePrompt handles PUT /update/{index}.
func (h *Handler) UpdatePrompt(w http.ResponseWriter, r *http.Request) {
	idx, ok := parseIndex(w, r, "/update/")
	if !ok {
		return
	}
	var req updatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	if err := h.svc.UpdatePrompt(idx, req.NewPrompt); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// parseIndex extracts the integer index from the URL path after the given prefix.
//
// MIGRATION_NOTE: Flask's <int:prompt_index> converter is replicated manually
// here. When on Go 1.22+, prefer http.ServeMux path wildcards
// (e.g. "GET /get/{index}") and r.PathValue("index").
func parseIndex(w http.ResponseWriter, r *http.Request, prefix string) (int, bool) {
	raw := r.URL.Path[len(prefix):]
	idx, err := strconv.Atoi(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return 0, false
	}
	return idx, true
}

// registerRoutes wires the handlers onto the given mux.
func registerRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/create", methodGuard(http.MethodPost, h.CreatePrompt))
	mux.HandleFunc("/get/", methodGuard(http.MethodGet, h.GetResponse))
	mux.HandleFunc("/delete/", methodGuard(http.MethodDelete, h.DeletePrompt))
	mux.HandleFunc("/update/", methodGuard(http.MethodPut, h.UpdatePrompt))
}

// methodGuard rejects requests whose method does not match the expected one.
func methodGuard(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		next(w, r)
	}
}

// main initializes the service and starts the HTTP server.
//
// MIGRATION_NOTE: The original hardcoded "YOUR_CHATGPT_API_KEY_HERE" is replaced
// by the OPENAI_API_KEY environment variable. Flask's debug server on port 5000
// is replaced by a standard net/http server with sensible timeouts.
func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Println("warning: OPENAI_API_KEY is not set")
	}

	svc := NewPromptService(newStubCompletionClient(apiKey))
	handler := NewHandler(svc)

	mux := http.NewServeMux()
	registerRoutes(mux, handler)

	srv := &http.Server{
		Addr:         ":5000",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Println("listening on :5000")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
