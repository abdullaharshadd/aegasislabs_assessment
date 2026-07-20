// Package main provides an HTTP REST API for managing text prompts and generating
// responses via the OpenAI Completion API.
//
// MIGRATION_NOTE: The Python source (main.py) is a Flask server. This Go migration
// uses net/http with a chi-style routing pattern implemented via Go 1.22+ method-based
// path patterns in the standard library ServeMux, avoiding external dependencies.
//
// MIGRATION_NOTE: In the Python source, prompts were stored in an in-memory singleton
// (ChatGPTBotAPI). Here, the equivalent state is encapsulated in a promptStore struct
// that is safe for concurrent access (Flask's dev server is single-threaded by default,
// but Go's net/http serves requests concurrently, so a mutex is mandatory).
//
// MIGRATION_NOTE: The OpenAI API key was hardcoded in Python. It is externalized here to
// the OPENAI_API_KEY environment variable. The legacy openai.Completion.create call
// (openai<1.0 SDK) is reimplemented as a direct HTTP call to the completions endpoint,
// since there is no official Go SDK matching that legacy behavior. Review the model name
// ("text-davinci-002") — it is deprecated by OpenAI and may need updating.
package main

import (
	"bytes"
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

// errInvalidPromptIndex is returned when a prompt index is out of range.
// MIGRATION_NOTE: In Python these were returned as the string "Invalid prompt index";
// here we model it as a sentinel error and preserve the original message on the wire.
var errInvalidPromptIndex = errors.New("Invalid prompt index")

// CompletionGenerator abstracts the generation of a completion for a given prompt.
// This interface allows the OpenAI client to be swapped for a fake in tests.
type CompletionGenerator interface {
	// Generate returns the generated text for the supplied prompt.
	Generate(ctx context.Context, prompt string) (string, error)
}

// openAICompletionClient calls the legacy OpenAI Completion API over HTTP.
type openAICompletionClient struct {
	apiKey     string
	engine     string
	maxTokens  int
	httpClient *http.Client
	baseURL    string
}

// NewOpenAICompletionClient constructs a CompletionGenerator backed by the OpenAI
// Completion API. The apiKey must be non-empty.
func NewOpenAICompletionClient(apiKey string) *openAICompletionClient {
	return &openAICompletionClient{
		apiKey:     apiKey,
		engine:     "text-davinci-002",
		maxTokens:  150,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.openai.com/v1/completions",
	}
}

// completionRequest models the JSON body sent to the OpenAI completions endpoint.
type completionRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	MaxTokens int    `json:"max_tokens"`
}

// completionResponse models the relevant subset of the OpenAI completions response.
type completionResponse struct {
	Choices []struct {
		Text string `json:"text"`
	} `json:"choices"`
}

// Generate implements CompletionGenerator by calling the OpenAI Completion API.
func (c *openAICompletionClient) Generate(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(completionRequest{
		Model:     c.engine,
		Prompt:    prompt,
		MaxTokens: c.maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("marshal completion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call completion API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("completion API returned status %d", resp.StatusCode)
	}

	var parsed completionResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode completion response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("completion API returned no choices")
	}
	return parsed.Choices[0].Text, nil
}

// promptStore holds the in-memory collection of prompts. It is safe for concurrent use.
//
// MIGRATION_NOTE: This replaces the ChatGPTBotAPI singleton. Business logic (index
// bounds checking, append/update/delete) is preserved exactly.
type promptStore struct {
	mu        sync.RWMutex
	prompts   []string
	generator CompletionGenerator
}

// NewPromptStore constructs a promptStore backed by the supplied CompletionGenerator.
func NewPromptStore(generator CompletionGenerator) *promptStore {
	return &promptStore{generator: generator}
}

// CreatePrompt stores a new prompt for later interactions.
func (s *promptStore) CreatePrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// GetResponse returns the generated response for the prompt at the given index.
// It returns errInvalidPromptIndex if the index is out of range.
func (s *promptStore) GetResponse(ctx context.Context, index int) (string, error) {
	s.mu.RLock()
	if index < 0 || index >= len(s.prompts) {
		s.mu.RUnlock()
		return "", errInvalidPromptIndex
	}
	prompt := s.prompts[index]
	s.mu.RUnlock()

	return s.generator.Generate(ctx, prompt)
}

// UpdatePrompt replaces the prompt at the given index with newPrompt.
// It returns errInvalidPromptIndex if the index is out of range.
func (s *promptStore) UpdatePrompt(index int, newPrompt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return errInvalidPromptIndex
	}
	s.prompts[index] = newPrompt
	return nil
}

// DeletePrompt removes the prompt at the given index.
// It returns errInvalidPromptIndex if the index is out of range.
func (s *promptStore) DeletePrompt(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return errInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return nil
}

// server wires the promptStore into HTTP handlers.
type server struct {
	store *promptStore
}

// NewServer constructs a server for the given promptStore.
func NewServer(store *promptStore) *server {
	return &server{store: store}
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

// parseIndex extracts and parses the {prompt_index} path value.
func parseIndex(r *http.Request) (int, error) {
	return strconv.Atoi(r.PathValue("prompt_index"))
}

// handleCreate implements POST /create.
func (s *server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	if body.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	s.store.CreatePrompt(body.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// handleGet implements GET /get/{prompt_index}.
func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		// MIGRATION_NOTE: Flask's <int:...> converter rejects non-integers with a 404.
		// We mirror that here.
		http.NotFound(w, r)
		return
	}

	response, err := s.store.GetResponse(r.Context(), index)
	if err != nil {
		if errors.Is(err, errInvalidPromptIndex) {
			// Preserve the original Python behavior: 200 with the message as the response.
			writeJSON(w, http.StatusOK, map[string]string{"response": errInvalidPromptIndex.Error()})
			return
		}
		log.Printf("get response: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate response"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

// handleDelete implements DELETE /delete/{prompt_index}.
func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	msg := "Prompt deleted successfully"
	if err := s.store.DeletePrompt(index); err != nil {
		msg = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

// handleUpdate implements PUT /update/{prompt_index}.
func (s *server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var body struct {
		NewPrompt string `json:"new_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	if body.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}

	msg := "Prompt updated successfully"
	if err := s.store.UpdatePrompt(index, body.NewPrompt); err != nil {
		msg = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

// Routes registers all HTTP routes and returns the configured handler.
//
// MIGRATION_NOTE: All four routes from the Flask source are registered here at their
// exact HTTP methods and paths. Go 1.22+ method+pattern routing is used.
func (s *server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", s.handleCreate)
	mux.HandleFunc("GET /get/{prompt_index}", s.handleGet)
	mux.HandleFunc("DELETE /delete/{prompt_index}", s.handleDelete)
	mux.HandleFunc("PUT /update/{prompt_index}", s.handleUpdate)
	return mux
}

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		// MIGRATION_NOTE: Python hardcoded "YOUR_CHATGPT_API_KEY_HERE". We require the key
		// via env var and fail fast rather than shipping a placeholder.
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":5000" // matches Flask's default dev port
	}

	store := NewPromptStore(NewOpenAICompletionClient(apiKey))
	srv := NewServer(store)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
