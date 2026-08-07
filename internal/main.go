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

type promptStore struct {
	mu      sync.Mutex
	prompts []string
	apiKey  string
}

func newPromptStore() *promptStore {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "YOUR_CHATGPT_API_KEY_HERE"
	}
	return &promptStore{apiKey: apiKey}
}

func (s *promptStore) add(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

func (s *promptStore) get(index int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return "", false
	}
	return s.prompts[index], true
}

func (s *promptStore) update(index int, newPrompt string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts[index] = newPrompt
	return true
}

func (s *promptStore) delete(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parsePromptIndex(r *http.Request) (int, error) {
	raw := chi.URLParam(r, "prompt_index")
	return strconv.Atoi(raw)
}

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

	if s.apiKey == "" || s.apiKey == "YOUR_CHATGPT_API_KEY_HERE" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "OpenAI API key not configured"})
		return
	}

	client := NewClient()
	resp, err := client.GetResponse(r.Context(), prompt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

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

func BuildRouter() http.Handler {
	return buildRouter()
}