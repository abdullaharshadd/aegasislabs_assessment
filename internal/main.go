package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// MIGRATION_NOTE: The original Flask app used the OpenAI Python SDK
// (openai.Completion.create). There is no first-party OpenAI Go SDK pinned in
// this migration, so OpenAICompleter is a small explicit HTTP client that hits
// the OpenAI completions endpoint. The model has been changed to gpt-4o-mini
// per the migration notes, and MaxTokens is restored to 150 to preserve the
// observable behavior of every uncached GET /get/<i> call.

// openAICompletionsURL is the OpenAI chat completions endpoint.
const openAICompletionsURL = "https://api.openai.com/v1/chat/completions"

// openAIModel is the model used for completions.
const openAIModel = "gpt-4o-mini"

// Completer produces a text completion for a given prompt.
type Completer interface {
	// Complete returns a completion for the given prompt.
	Complete(ctx context.Context, prompt string) (string, error)
}

// OpenAICompleter is a Completer backed by the OpenAI HTTP API.
type OpenAICompleter struct {
	apiKey     string
	httpClient *http.Client
}

// NewOpenAICompleter constructs an OpenAICompleter with the given API key. If
// httpClient is nil, http.DefaultClient is used.
func NewOpenAICompleter(apiKey string, httpClient *http.Client) *OpenAICompleter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAICompleter{apiKey: apiKey, httpClient: httpClient}
}

// Complete calls the OpenAI API and returns the first completion choice's text.
func (c *OpenAICompleter) Complete(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]any{
		"model": openAIModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		// MaxTokens restored to 150 to match the source's observable behavior.
		"max_tokens": 150,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal completion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAICompletionsURL, bytes.NewReader(payload))
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("completion API returned status %d", resp.StatusCode)
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode completion response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("completion API returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

// ErrInvalidPromptIndex indicates the requested prompt index is out of range.
// MIGRATION_NOTE: The Python code returned the string "Invalid prompt index"
// as a normal (200) response body rather than an error status. To preserve the
// exact observable behavior of the source, the handlers below reproduce that
// string in the JSON body rather than surfacing this as an HTTP error.
var ErrInvalidPromptIndex = fmt.Errorf("invalid prompt index")

// ChatGPTBotAPI holds the in-memory prompt store and a Completer. It is the Go
// equivalent of the Python ChatGPTBotAPI service class.
type ChatGPTBotAPI struct {
	mu        sync.Mutex
	prompts   []string
	completer Completer
}

// NewChatGPTBotAPI constructs a ChatGPTBotAPI backed by the given Completer.
func NewChatGPTBotAPI(completer Completer) *ChatGPTBotAPI {
	return &ChatGPTBotAPI{completer: completer}
}

// CreatePromptEntry stores a user-provided prompt for later interactions.
func (a *ChatGPTBotAPI) CreatePromptEntry(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prompts = append(a.prompts, prompt)
}

// GetResponseFor returns a completion for the prompt at the given index. If the
// index is out of range it returns the literal "Invalid prompt index" string,
// matching the source behavior.
func (a *ChatGPTBotAPI) GetResponseFor(ctx context.Context, index int) (string, error) {
	a.mu.Lock()
	if index < 0 || index >= len(a.prompts) {
		a.mu.Unlock()
		return "Invalid prompt index", nil
	}
	prompt := a.prompts[index]
	a.mu.Unlock()

	return a.completer.Complete(ctx, prompt)
}

// UpdatePromptEntry updates the prompt at the given index. It returns the same
// status strings the source produced.
func (a *ChatGPTBotAPI) UpdatePromptEntry(index int, newPrompt string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if index < 0 || index >= len(a.prompts) {
		return "Invalid prompt index"
	}
	a.prompts[index] = newPrompt
	return "Prompt updated successfully"
}

// DeletePromptEntry deletes the prompt at the given index. It returns the same
// status strings the source produced.
func (a *ChatGPTBotAPI) DeletePromptEntry(index int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if index < 0 || index >= len(a.prompts) {
		return "Invalid prompt index"
	}
	a.prompts = append(a.prompts[:index], a.prompts[index+1:]...)
	return "Prompt deleted successfully"
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parsePromptIndex extracts and validates the integer prompt index URL param.
func parsePromptIndex(r *http.Request) (int, error) {
	raw := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid prompt index %q: %w", raw, err)
	}
	return idx, nil
}

// CreatePromptHandler handles POST /create.
func (a *ChatGPTBotAPI) CreatePromptHandler(w http.ResponseWriter, r *http.Request) {
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
	a.CreatePromptEntry(body.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// GetResponseHandler handles GET /get/{prompt_index}.
//
// GUARDRAIL: like DeletePromptHandler, this handler intentionally does NOT
// parse a request body and does NOT enforce a 415 Unsupported Media Type gate
// on Content-Type. GET carries no JSON body in the source; adding body parsing
// here would be an observable behavioral divergence.
func (a *ChatGPTBotAPI) GetResponseHandler(w http.ResponseWriter, r *http.Request) {
	idx, err := parsePromptIndex(r)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	response, err := a.GetResponseFor(r.Context(), idx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

// DeletePromptHandler handles DELETE /delete/{prompt_index}.
//
// GUARDRAIL: this handler intentionally does NOT parse a request body and does
// NOT enforce a 415 Unsupported Media Type gate on Content-Type, matching the
// source's DELETE endpoint.
func (a *ChatGPTBotAPI) DeletePromptHandler(w http.ResponseWriter, r *http.Request) {
	idx, err := parsePromptIndex(r)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	message := a.DeletePromptEntry(idx)
	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

// UpdatePromptHandler handles PUT /update/{prompt_index}.
func (a *ChatGPTBotAPI) UpdatePromptHandler(w http.ResponseWriter, r *http.Request) {
	idx, err := parsePromptIndex(r)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
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
	message := a.UpdatePromptEntry(idx, body.NewPrompt)
	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

// buildRouter wires up all routes and returns the configured HTTP handler.
//
// MIGRATION_NOTE: The source used a module-level singleton chatbot_api with a
// hard-coded API key placeholder. Here the key is read from the OPENAI_API_KEY
// environment variable, which is the idiomatic Go approach.
func buildRouter() http.Handler {
	apiKey := os.Getenv("OPENAI_API_KEY")
	completer := NewOpenAICompleter(apiKey, nil)
	api := NewChatGPTBotAPI(completer)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	r.Post("/create", api.CreatePromptHandler)
	r.Get("/get/{prompt_index}", api.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", api.DeletePromptHandler)
	r.Put("/update/{prompt_index}", api.UpdatePromptHandler)

	return r
}
