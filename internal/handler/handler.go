package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ErrInvalidPromptIndex is returned when a prompt index is out of range.
// MIGRATION_NOTE: In Python these were returned as the string "Invalid prompt index";
// here we model it as a sentinel error and preserve the original message on the wire.
var ErrInvalidPromptIndex = errors.New("Invalid prompt index")

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
func NewOpenAICompletionClient(apiKey string) CompletionGenerator {
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

// PromptStore holds the in-memory collection of prompts. It is safe for concurrent use.
//
// MIGRATION_NOTE: This replaces the ChatGPTBotAPI singleton. Business logic (index
// bounds checking, append/update/delete) is preserved exactly.
type PromptStore struct {
	mu        sync.RWMutex
	prompts   []string
	generator CompletionGenerator
}

// NewPromptStore constructs a PromptStore backed by the supplied CompletionGenerator.
func NewPromptStore(generator CompletionGenerator) *PromptStore {
	return &PromptStore{generator: generator}
}

// CreatePrompt stores a new prompt for later interactions.
func (s *PromptStore) CreatePrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// GetResponse returns the generated response for the prompt at the given index.
// It returns ErrInvalidPromptIndex if the index is out of range.
func (s *PromptStore) GetResponse(ctx context.Context, index int) (string, error) {
	s.mu.RLock()
	if index < 0 || index >= len(s.prompts) {
		s.mu.RUnlock()
		return "", ErrInvalidPromptIndex
	}
	prompt := s.prompts[index]
	s.mu.RUnlock()

	return s.generator.Generate(ctx, prompt)
}

// UpdatePrompt replaces the prompt at the given index with newPrompt.
// It returns ErrInvalidPromptIndex if the index is out of range.
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
// It returns ErrInvalidPromptIndex if the index is out of range.
func (s *PromptStore) DeletePrompt(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return nil
}

// Server wires the PromptStore into HTTP handlers.
type Server struct {
	store *PromptStore
}

// NewServer constructs a Server for the given PromptStore.
func NewServer(store *PromptStore) *Server {
	return &Server{store: store}
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
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
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
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	index, err := parseIndex(r)
	if err != nil {
		// MIGRATION_NOTE: Flask's <int:...> converter rejects non-integers with a 404.
		// We mirror that here.
		http.NotFound(w, r)
		return
	}

	response, err := s.store.GetResponse(r.Context(), index)
	if err != nil {
		if errors.Is(err, ErrInvalidPromptIndex) {
			// Preserve the original Python behavior: 200 with the message as the response.
			writeJSON(w, http.StatusOK, map[string]string{"response": ErrInvalidPromptIndex.Error()})
			return
		}
		log.Printf("get response: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate response"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

// handleDelete implements DELETE /delete/{prompt_index}.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
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
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
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
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /create", s.handleCreate)
	mux.HandleFunc("GET /get/{prompt_index}", s.handleGet)
	mux.HandleFunc("DELETE /delete/{prompt_index}", s.handleDelete)
	mux.HandleFunc("PUT /update/{prompt_index}", s.handleUpdate)
	return mux
}

// NewFromEnv constructs a Server wired from the OPENAI_API_KEY environment variable.
// MIGRATION_NOTE: Python hardcoded "YOUR_CHATGPT_API_KEY_HERE". We require the key
// via env var and fail fast rather than shipping a placeholder.
func NewFromEnv(apiKey string) *Server {
	store := NewPromptStore(NewOpenAICompletionClient(apiKey))
	return NewServer(store)
}