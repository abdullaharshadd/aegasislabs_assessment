package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// PromptResponder abstracts the ability to turn a stored prompt into a
// generated completion. This is the interface the HTTP layer depends on so
// that the OpenAI-backed implementation can be swapped out in tests.
//
// MIGRATION_NOTE: The Python source called openai.Completion.create directly
// inside get_response. We keep the network call behind this interface. The
// concrete DavinciResponder below preserves the original engine/max_tokens
// behavior; wire in a real OpenAI client where noted.
type PromptResponder interface {
	// Respond returns the model completion for the given prompt text.
	Respond(ctx context.Context, prompt string) (string, error)
}

// PromptStore holds the in-memory list of prompts. It replaces the mutable
// self.prompts slice on the Python ChatGPTBotAPI singleton and is safe for
// concurrent use, since Go's HTTP server handles each request in its own
// goroutine (unlike Flask's default single-threaded dev server).
type PromptStore struct {
	mu      sync.Mutex
	prompts []string
}

// NewPromptStore constructs an empty PromptStore.
func NewPromptStore() *PromptStore {
	return &PromptStore{prompts: make([]string, 0)}
}

// Add appends a prompt to the store, mirroring create_prompt.
func (s *PromptStore) Add(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// Get returns the prompt at index and whether the index was valid. This
// replaces the Python bounds check that returned "Invalid prompt index".
func (s *PromptStore) Get(index int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return "", false
	}
	return s.prompts[index], true
}

// UpdateChecked atomically validates the index and replaces the prompt in a
// single locked critical section.
//
// MIGRATION_NOTE (CHANGE 15): The Python update_prompt did a bounds check
// followed by an assignment in two separate steps. In a concurrent Go server
// that is a TOCTOU race (a concurrent Delete could shrink the slice between
// the check and the write). We collapse both into one locked operation and
// return ok=false for an invalid index.
func (s *PromptStore) UpdateChecked(index int, newPrompt string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts[index] = newPrompt
	return true
}

// DeleteChecked atomically validates the index and removes the prompt.
// It returns false for an invalid index, mirroring delete_prompt.
func (s *PromptStore) DeleteChecked(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return true
}

// DavinciResponder is the production PromptResponder backed by the OpenAI
// completion API, preserving the source engine and token settings.
//
// MIGRATION_NOTE: The Python code used the global openai module configured
// with an API key. The Go OpenAI client is not part of this migration batch,
// so Respond currently returns an explicit error indicating the client must
// be wired in. Replace the body with a real API call (engine
// "text-davinci-002", max_tokens 150) once the client dependency is added.
type DavinciResponder struct {
	APIKey string
}

// NewDavinciResponder constructs a DavinciResponder with the given API key.
func NewDavinciResponder(apiKey string) *DavinciResponder {
	return &DavinciResponder{APIKey: apiKey}
}

// Respond implements PromptResponder.
func (d *DavinciResponder) Respond(ctx context.Context, prompt string) (string, error) {
	if d.APIKey == "" {
		return "", fmt.Errorf("openai api key not configured")
	}
	// MIGRATION_NOTE: Implement the real openai.Completion.create call here:
	//   engine="text-davinci-002", prompt=prompt, max_tokens=150
	// and return response.Choices[0].Text.
	return "", fmt.Errorf("openai completion client not yet wired in")
}

// PromptHandler holds the dependencies needed to serve the prompt HTTP API.
// It replaces the Flask module-level singletons (app + chatbot_api).
type PromptHandler struct {
	store     *PromptStore
	responder PromptResponder
}

// NewPromptHandler constructs a PromptHandler with an in-memory store and the
// given responder.
func NewPromptHandler(responder PromptResponder) *PromptHandler {
	return &PromptHandler{
		store:     NewPromptStore(),
		responder: responder,
	}
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// write500HTML writes a generic 500 response.
//
// MIGRATION_NOTE (CHANGE 14): Flask's debug server renders an HTML traceback
// page on unhandled errors. We do not leak stack traces; instead we emit a
// minimal, safe HTML error page. middleware.Recoverer handles panics, but
// this helper is used for handler-level internal failures (e.g. the responder
// returning an error).
func write500HTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprintln(w, "<!doctype html><html><head><title>500 Internal Server Error</title></head><body><h1>Internal Server Error</h1></body></html>")
}

// parsePromptIndex extracts and validates the {prompt_index} URL parameter.
// It returns (index, true) on success, or writes a 400 response and returns
// (0, false) if the value is not a valid integer.
func parsePromptIndex(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid prompt index"})
		return 0, false
	}
	return idx, true
}

// CreatePromptHandler handles POST /create.
//
// MIGRATION_NOTE: mirrors Flask create_prompt: reads JSON body field
// "prompt", rejects empty/missing with 400, stores it and returns 201.
func (h *PromptHandler) CreatePromptHandler(w http.ResponseWriter, r *http.Request) {
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
	h.store.Add(body.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// GetResponseHandler handles GET /get/{prompt_index}.
//
// MIGRATION_NOTE: mirrors Flask get_response. The Python version returned the
// string "Invalid prompt index" inside a 200 JSON body for out-of-range
// indexes, so we preserve that exact status-and-body behavior rather than
// switching to a 404.
func (h *PromptHandler) GetResponseHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parsePromptIndex(w, r)
	if !ok {
		return
	}
	prompt, valid := h.store.Get(idx)
	if !valid {
		writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
		return
	}
	resp, err := h.responder.Respond(r.Context(), prompt)
	if err != nil {
		write500HTML(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

// DeletePromptHandler handles DELETE /delete/{prompt_index}.
//
// MIGRATION_NOTE: mirrors Flask delete_prompt, which always returned 200 with
// a message string ("Prompt deleted successfully" or "Invalid prompt index").
func (h *PromptHandler) DeletePromptHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parsePromptIndex(w, r)
	if !ok {
		return
	}
	if h.store.DeleteChecked(idx) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
}

// UpdatePromptHandler handles PUT /update/{prompt_index}.
//
// MIGRATION_NOTE (CHANGE 16): The source used validation shape B — it read
// request.get_json(), pulled the "new_prompt" field, and returned a 400 with
// {"error": "New prompt not provided"} when it was missing/empty; otherwise it
// delegated to update_prompt and returned 200 with a message string that could
// itself be "Invalid prompt index". We reproduce exactly that: 400 on missing
// body/field, else 200 with the outcome message.
func (h *PromptHandler) UpdatePromptHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parsePromptIndex(w, r)
	if !ok {
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
	if h.store.UpdateChecked(idx, body.NewPrompt) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
}

// buildRouter constructs the fully-wired chi router for the prompt API.
// cmd/server/main.go calls this directly.
//
// MIGRATION_NOTE: The OpenAI API key was a hard-coded placeholder in the
// Python source ("YOUR_CHATGPT_API_KEY_HERE"). Load it from configuration/env
// in production; here we construct the responder with an empty key so the
// server still boots and CRUD routes work without OpenAI credentials.
func buildRouter() http.Handler {
	handler := NewPromptHandler(NewDavinciResponder(""))

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})

	r.Post("/create", handler.CreatePromptHandler)
	r.Get("/get/{prompt_index}", handler.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", handler.DeletePromptHandler)
	r.Put("/update/{prompt_index}", handler.UpdatePromptHandler)

	return r
}

// BuildRouter is an exported wrapper around buildRouter so that cmd/server/main.go
// can call it from outside the internal package.
func BuildRouter() http.Handler {
	return buildRouter()
}