// Package main implements a ChatGPT prompt-management REST API server.
//
// The server exposes the following endpoints:
//
//	POST   /create              create a new prompt
//	GET    /get/{index}         retrieve a ChatGPT response for a prompt by index
//	PUT    /update/{index}      update an existing prompt
//	DELETE /delete/{index}      delete a prompt by index
//
// MIGRATION_NOTE: The original Python file (main.py) is a Flask *server* that
// wraps the OpenAI API. This is distinct from client.py (already migrated),
// which is a client that consumes a similar API. The two share endpoint shapes
// but serve opposite roles, so this server is kept in its own package.
//
// MIGRATION_NOTE: The API uses integer indices as identifiers rather than
// stable IDs. This scheme is preserved to match the original behaviour, but
// note that deleting a prompt shifts the indices of all subsequent prompts.
// Review whether stable IDs should be adopted instead.
//
// MIGRATION_NOTE: The original hardcoded the OpenAI API key in source. Here it
// is read from the OPENAI_API_KEY environment variable instead, which is the
// idiomatic and secure approach.
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

// ErrInvalidPromptIndex is returned when a prompt index is out of range.
var ErrInvalidPromptIndex = errors.New("invalid prompt index")

// CompletionEngine is the OpenAI engine used for completions.
//
// MIGRATION_NOTE: The original used the deprecated "text-davinci-002" engine
// via the legacy Completion API. This value is preserved for parity; review
// whether to move to the modern chat completions API.
const CompletionEngine = "text-davinci-002"

// maxTokens is the maximum number of tokens requested per completion.
const maxTokens = 150

// CompletionRequester abstracts the call to the OpenAI completion API so it can
// be substituted in tests.
type CompletionRequester interface {
	// Complete returns a completion for the given prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// ChatGPTBot manages a collection of prompts and produces completions for them.
//
// It is safe for concurrent use.
type ChatGPTBot struct {
	mu        sync.RWMutex
	prompts   []string
	requester CompletionRequester
}

// NewChatGPTBot constructs a ChatGPTBot backed by the given CompletionRequester.
func NewChatGPTBot(requester CompletionRequester) *ChatGPTBot {
	return &ChatGPTBot{
		prompts:   make([]string, 0),
		requester: requester,
	}
}

// CreatePrompt stores a user-provided prompt for later interactions.
func (b *ChatGPTBot) CreatePrompt(prompt string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prompts = append(b.prompts, prompt)
}

// GetResponse returns the ChatGPT completion for the prompt at the given index.
//
// It returns ErrInvalidPromptIndex if the index is out of range.
func (b *ChatGPTBot) GetResponse(ctx context.Context, index int) (string, error) {
	b.mu.RLock()
	if index < 0 || index >= len(b.prompts) {
		b.mu.RUnlock()
		return "", ErrInvalidPromptIndex
	}
	prompt := b.prompts[index]
	b.mu.RUnlock()

	text, err := b.requester.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("completing prompt %d: %w", index, err)
	}
	return text, nil
}

// UpdatePrompt replaces the prompt at the given index with newPrompt.
//
// It returns ErrInvalidPromptIndex if the index is out of range.
func (b *ChatGPTBot) UpdatePrompt(index int, newPrompt string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if index < 0 || index >= len(b.prompts) {
		return ErrInvalidPromptIndex
	}
	b.prompts[index] = newPrompt
	return nil
}

// DeletePrompt removes the prompt at the given index.
//
// It returns ErrInvalidPromptIndex if the index is out of range.
//
// MIGRATION_NOTE: Deleting shifts the indices of subsequent prompts, matching
// the original Python list-based behaviour.
func (b *ChatGPTBot) DeletePrompt(index int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if index < 0 || index >= len(b.prompts) {
		return ErrInvalidPromptIndex
	}
	b.prompts = append(b.prompts[:index], b.prompts[index+1:]...)
	return nil
}

// Server holds the HTTP handlers and their dependencies.
type Server struct {
	bot *ChatGPTBot
}

// NewServer constructs a Server backed by the given ChatGPTBot.
func NewServer(bot *ChatGPTBot) *Server {
	return &Server{bot: bot}
}

// createPromptRequest is the JSON body for POST /create.
type createPromptRequest struct {
	Prompt string `json:"prompt"`
}

// updatePromptRequest is the JSON body for PUT /update/{index}.
type updatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// parseIndex extracts a trailing integer path segment following prefix.
func parseIndex(path, prefix string) (int, error) {
	if len(path) <= len(prefix) {
		return 0, errors.New("missing index")
	}
	return strconv.Atoi(path[len(prefix):])
}

// handleCreate handles POST /create.
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req createPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	s.bot.CreatePrompt(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// handleGet handles GET /get/{index}.
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	index, err := parseIndex(r.URL.Path, "/get/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	response, err := s.bot.GetResponse(r.Context(), index)
	if err != nil {
		if errors.Is(err, ErrInvalidPromptIndex) {
			// MIGRATION_NOTE: The original returned HTTP 200 with the string
			// "Invalid prompt index" in the body. We preserve that behaviour
			// for parity rather than returning a 4xx.
			writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

// handleDelete handles DELETE /delete/{index}.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	index, err := parseIndex(r.URL.Path, "/delete/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	if err := s.bot.DeletePrompt(index); err != nil {
		// MIGRATION_NOTE: The original returned HTTP 200 with the message string
		// even on invalid index. Preserved for parity.
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// handleUpdate handles PUT /update/{index}.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	index, err := parseIndex(r.URL.Path, "/update/")
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
	if err := s.bot.UpdatePrompt(index, req.NewPrompt); err != nil {
		// MIGRATION_NOTE: The original returned HTTP 200 with the message string
		// even on invalid index. Preserved for parity.
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// Routes returns an http.Handler with all API routes registered.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/create", s.handleCreate)
	mux.HandleFunc("/get/", s.handleGet)
	mux.HandleFunc("/delete/", s.handleDelete)
	mux.HandleFunc("/update/", s.handleUpdate)
	return mux
}

// openAIRequester is a placeholder CompletionRequester.
//
// MIGRATION_NOTE: The original used the Python `openai` package directly.
// Wire in a real OpenAI SDK client (e.g. github.com/sashabaranov/go-openai)
// here. This stub returns an error so the missing integration is explicit
// rather than silently returning empty completions.
type openAIRequester struct {
	apiKey string
}

// Complete is not yet implemented against a real OpenAI client.
func (o *openAIRequester) Complete(ctx context.Context, prompt string) (string, error) {
	return "", errors.New("openai integration not implemented: wire in a real OpenAI client")
}

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	bot := NewChatGPTBot(&openAIRequester{apiKey: apiKey})
	server := NewServer(bot)

	addr := ":5000"
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
}
