package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// promptStore holds the in-memory list of prompts and the OpenAI client.
//
// MIGRATION_NOTE: The original Flask app kept module-level mutable state
// (a Python list) mutated by request handlers. Go's HTTP server handles
// requests concurrently, so all access to the slice is guarded by a mutex.
// Even so, the caller was warned that index-based operations are unsafe
// under concurrency because deletes shift indices; the mutex serializes
// access but does not change that indices are position-dependent.
type promptStore struct {
	mu      sync.Mutex
	prompts []string

	// apiKey is validated lazily (only when GetResponse is first called),
	// matching the original behavior where a missing/placeholder key does
	// not fail at startup but fails at the first /get request.
	apiKey string
}

// newPromptStore constructs a promptStore, reading the OpenAI API key from
// the environment. It does not validate the key; validation is deferred to
// the first /get request to preserve the original lazy-failure semantics.
func newPromptStore() *promptStore {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		// MIGRATION_NOTE: The Python source hardcoded the placeholder
		// "YOUR_CHATGPT_API_KEY_HERE". We fall back to that same placeholder
		// so behavior (lazy failure at /get) is preserved when no env var set.
		apiKey = "YOUR_CHATGPT_API_KEY_HERE"
	}
	return &promptStore{apiKey: apiKey}
}

// add appends a prompt to the in-memory store.
func (s *promptStore) add(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

// get returns the prompt at the given index. The bool reports whether the
// index was valid, mirroring the Python "Invalid prompt index" guard.
func (s *promptStore) get(index int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return "", false
	}
	return s.prompts[index], true
}

// update replaces the prompt at the given index. It returns false if the
// index is out of range.
func (s *promptStore) update(index int, newPrompt string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts[index] = newPrompt
	return true
}

// delete removes the prompt at the given index. It returns false if the
// index is out of range.
func (s *promptStore) delete(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return true
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parsePromptIndex extracts the integer prompt_index URL parameter.
func parsePromptIndex(r *http.Request) (int, error) {
	raw := chi.URLParam(r, "prompt_index")
	return strconv.Atoi(raw)
}

// handleCreate implements POST /create. It reads a JSON body with a
// "prompt" field and stores it. Returns 400 if the prompt is missing and
// 201 on success.
func (s *promptStore) handleCreate(w http.ResponseWriter, r *http.Request) {
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
	s.add(body.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

// handleGet implements GET /get/{prompt_index}. It fetches the OpenAI
// completion for the stored prompt at the given index.
//
// MIGRATION_NOTE: The Python source distinguished two failure shapes:
//   - an out-of-range index returned {"response": "Invalid prompt index"}
//     with HTTP 200 (the 400-gated path)
//   - an actual OpenAI/API error surfaced via a synthetic "error" key with
//     HTTP 500.
//
// Both shapes are preserved below.
func (s *promptStore) handleGet(w http.ResponseWriter, r *http.Request) {
	index, err := parsePromptIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
		return
	}

	prompt, ok := s.get(index)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"response": "Invalid prompt index"})
		return
	}

	// Lazy API-key validation: fails here at first /get, not at startup.
	if s.apiKey == "" || s.apiKey == "YOUR_CHATGPT_API_KEY_HERE" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "OpenAI API key not configured"})
		return
	}

	// MIGRATION_NOTE: The original called openai.Completion.create with the
	// text-davinci-002 engine and max_tokens=150. That call is delegated to
	// the already-migrated Client (internal/client.go) via GetResponse. The
	// synthetic "error" key with HTTP 500 is used for real API failures,
	// distinct from the 200 "Invalid prompt index" path above.
	client := NewClient()
	resp, err := client.GetResponse(r.Context(), prompt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

// handleUpdate implements PUT /update/{prompt_index}. It reads a JSON body
// with a "new_prompt" field and replaces the prompt at the index. Returns
// 400 if new_prompt is missing.
func (s *promptStore) handleUpdate(w http.ResponseWriter, r *http.Request) {
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

	index, err := parsePromptIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}

	if ok := s.update(index, body.NewPrompt); !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt updated successfully"})
}

// handleDelete implements DELETE /delete/{prompt_index}. It deletes the
// prompt at the given index.
//
// MIGRATION_NOTE: The Delete handler must NOT read a request body (matching
// the original Flask handler which never called request.get_json()).
func (s *promptStore) handleDelete(w http.ResponseWriter, r *http.Request) {
	index, err := parsePromptIndex(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}

	if ok := s.delete(index); !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Invalid prompt index"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Prompt deleted successfully"})
}

// buildRouter constructs the fully-wired HTTP handler for the prompt API.
//
// MIGRATION_NOTE: The store is created once here so all handlers share the
// same in-memory slice, mirroring the module-level singleton in the Python
// source. The server must run as a single instance; the shared index-based
// state is not distributable.
func buildRouter() http.Handler {
	store := newPromptStore()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	r.Post("/create", store.handleCreate)
	r.Get("/get/{prompt_index}", store.handleGet)
	r.Delete("/delete/{prompt_index}", store.handleDelete)
	r.Put("/update/{prompt_index}", store.handleUpdate)

	return r
}

// BuildRouter is an exported wrapper around buildRouter so that the
// cmd/server entrypoint can wire the handler without duplicating logic.
func BuildRouter() http.Handler {
	return buildRouter()
}
