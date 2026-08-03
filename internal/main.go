package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// ChatGPTBotAPI is an in-memory store of text prompts together with the logic
// to generate AI completions for them. It replaces the Python ChatGPTBotAPI
// class from the original Flask application.
//
// MIGRATION_NOTE: The original Python code stored the OpenAI API key on the
// package-global `openai` module. Here the key is held per-instance. The actual
// OpenAI Completion call is intentionally not reproduced with a specific SDK;
// instead a pluggable Completer is used so callers can inject a real OpenAI
// client (or a test double). See generateCompletion below.
type ChatGPTBotAPI struct {
	mu        sync.RWMutex
	prompts   []string
	completer Completer
}

// Completer abstracts the text-completion backend (e.g. the OpenAI Completion
// API). Implementations must be safe for concurrent use.
type Completer interface {
	// Complete returns a completion for the given prompt.
	Complete(prompt string) (string, error)
}

// CompleterFunc adapts an ordinary function to the Completer interface.
type CompleterFunc func(prompt string) (string, error)

// Complete calls f(prompt).
func (f CompleterFunc) Complete(prompt string) (string, error) {
	return f(prompt)
}

// NewChatGPTBotAPI constructs a ChatGPTBotAPI. If completer is nil, a default
// completer is used that echoes back a placeholder response.
//
// MIGRATION_NOTE: The Python constructor took an openai_api_key string and set
// openai.api_key globally. Wire a real OpenAI-backed Completer here in
// production instead of the placeholder default.
func NewChatGPTBotAPI(completer Completer) *ChatGPTBotAPI {
	if completer == nil {
		completer = CompleterFunc(func(prompt string) (string, error) {
			return fmt.Sprintf("completion for: %s", prompt), nil
		})
	}
	return &ChatGPTBotAPI{completer: completer}
}

// ErrInvalidPromptIndex indicates the supplied prompt index is out of range.
//
// MIGRATION_NOTE: The Python code returned the string "Invalid prompt index"
// as the response body. To preserve that exact behaviour at the HTTP boundary,
// handlers detect this sentinel and reproduce the original string response.
type invalidIndexError struct{}

func (invalidIndexError) Error() string { return "Invalid prompt index" }

// ErrInvalidPromptIndex is the sentinel returned when a prompt index is out of
// range.
var ErrInvalidPromptIndex error = invalidIndexError{}

// AddPrompt stores a user-provided prompt for later interactions.
func (c *ChatGPTBotAPI) AddPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, prompt)
}

// GetResponse returns the completion for the prompt at the given index. It
// returns ErrInvalidPromptIndex if the index is out of range.
func (c *ChatGPTBotAPI) GetResponse(index int) (string, error) {
	c.mu.RLock()
	if index < 0 || index >= len(c.prompts) {
		c.mu.RUnlock()
		return "", ErrInvalidPromptIndex
	}
	prompt := c.prompts[index]
	c.mu.RUnlock()

	resp, err := c.completer.Complete(prompt)
	if err != nil {
		return "", fmt.Errorf("completion failed: %w", err)
	}
	return resp, nil
}

// UpdatePrompt replaces the prompt at index with newPrompt. It returns
// ErrInvalidPromptIndex if the index is out of range.
//
// MIGRATION_NOTE (box 5): The Python UpdatePrompt validates the index BEFORE
// mutating, then assigns. This ordering is preserved here — bounds check first,
// assignment second.
func (c *ChatGPTBotAPI) UpdatePrompt(index int, newPrompt string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.prompts) {
		return ErrInvalidPromptIndex
	}
	c.prompts[index] = newPrompt
	return nil
}

// DeletePrompt removes the prompt at index. It returns ErrInvalidPromptIndex if
// the index is out of range.
func (c *ChatGPTBotAPI) DeletePrompt(index int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.prompts) {
		return ErrInvalidPromptIndex
	}
	c.prompts = append(c.prompts[:index], c.prompts[index+1:]...)
	return nil
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parseIndex extracts and parses the {prompt_index} URL parameter.
//
// MIGRATION_NOTE (boxes 1-2): The Flask routes used <int:prompt_index> path
// converters, so the index comes from the URL path (not query/body) and must be
// an integer. chi does not coerce types, so we parse explicitly.
func parseIndex(r *http.Request) (int, error) {
	raw := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid prompt index %q: %w", raw, err)
	}
	return idx, nil
}

// handleCreatePrompt implements POST /create.
func (c *ChatGPTBotAPI) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	c.AddPrompt(body.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// handleGetResponse implements GET /get/{prompt_index}.
//
// MIGRATION_NOTE (box 7): The Python get_response returns the string
// "Invalid prompt index" inside a 200 JSON body for out-of-range indices
// (it never returns a 4xx for that case). That exact behaviour is preserved:
// bad index yields 200 with {"response": "Invalid prompt index"}.
func (c *ChatGPTBotAPI) handleGetResponse(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"response": ErrInvalidPromptIndex.Error()})
		return
	}
	resp, err := c.GetResponse(idx)
	if err == ErrInvalidPromptIndex {
		writeJSON(w, http.StatusOK, map[string]string{"response": ErrInvalidPromptIndex.Error()})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

// handleDeletePrompt implements DELETE /delete/{prompt_index}.
func (c *ChatGPTBotAPI) handleDeletePrompt(w http.ResponseWriter, r *http.Request) {
	idx, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": ErrInvalidPromptIndex.Error()})
		return
	}
	if err := c.DeletePrompt(idx); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// handleUpdatePrompt implements PUT /update/{prompt_index}.
func (c *ChatGPTBotAPI) handleUpdatePrompt(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewPrompt string `json:"new_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	idx, err := parseIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": ErrInvalidPromptIndex.Error()})
		return
	}
	if err := c.UpdatePrompt(idx, body.NewPrompt); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// buildRouter constructs the fully-wired HTTP handler for the prompt API. It is
// called directly by cmd/server/main.go.
func buildRouter() http.Handler {
	// MIGRATION_NOTE: The Python app instantiated a single global ChatGPTBotAPI.
	// We do the same here with one shared instance backing all requests. Inject a
	// real OpenAI-backed Completer in place of the nil default for production.
	bot := NewChatGPTBotAPI(nil)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	r.Post("/create", bot.handleCreatePrompt)
	r.Get("/get/{prompt_index}", bot.handleGetResponse)
	r.Delete("/delete/{prompt_index}", bot.handleDeletePrompt)
	r.Put("/update/{prompt_index}", bot.handleUpdatePrompt)

	return r
}

// BuildRouter is the exported wrapper around buildRouter for use by
// cmd/server/main.go.
func BuildRouter() http.Handler {
	return buildRouter()
}