package client

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const (
	msgCreated        = "Prompt created successfully"
	msgUpdated        = "Prompt updated successfully"
	msgDeleted        = "Prompt deleted successfully"
	msgPromptRequired = "Prompt not provided"
	msgNewPromptReq   = "New prompt not provided"
	msgInvalidIndex   = "Invalid prompt index"
)

type PromptStore struct {
	mu      sync.RWMutex
	prompts []string
}

func NewPromptStore() *PromptStore {
	return &PromptStore{prompts: make([]string, 0)}
}

func (s *PromptStore) Add(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

func (s *PromptStore) Get(index int) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index < 0 || index >= len(s.prompts) {
		return "", false
	}
	return s.prompts[index], true
}

func (s *PromptStore) Update(index int, newPrompt string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts[index] = newPrompt
	return true
}

func (s *PromptStore) Delete(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.prompts) {
		return false
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return true
}

type PromptHandler struct {
	store *PromptStore
}

func NewPromptHandler(store *PromptStore) *PromptHandler {
	return &PromptHandler{store: store}
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseIndex(r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return idx, true
}

func (h *PromptHandler) CreatePromptHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msgPromptRequired})
		return
	}
	h.store.Add(body.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": msgCreated})
}

func (h *PromptHandler) GetResponseHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parseIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"response": msgInvalidIndex})
		return
	}
	prompt, valid := h.store.Get(idx)
	if !valid {
		writeJSON(w, http.StatusOK, map[string]string{"response": msgInvalidIndex})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": prompt})
}

func (h *PromptHandler) DeletePromptHandler(w http.ResponseWriter, r *http.Request) {
	idx, ok := parseIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": msgInvalidIndex})
		return
	}
	if !h.store.Delete(idx) {
		writeJSON(w, http.StatusOK, map[string]string{"message": msgInvalidIndex})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msgDeleted})
}

func (h *PromptHandler) UpdatePromptHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewPrompt string `json:"new_prompt"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msgNewPromptReq})
		return
	}
	idx, ok := parseIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": msgInvalidIndex})
		return
	}
	if !h.store.Update(idx, body.NewPrompt) {
		writeJSON(w, http.StatusOK, map[string]string{"message": msgInvalidIndex})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": msgUpdated})
}

func buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, "ok")
	})

	store := NewPromptStore()
	h := NewPromptHandler(store)

	r.Post("/create", h.CreatePromptHandler)
	r.Get("/get/{prompt_index}", h.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", h.DeletePromptHandler)
	r.Put("/update/{prompt_index}", h.UpdatePromptHandler)

	return r
}

// BuildRouter is the exported entry point for constructing the HTTP router.
// It is called by cmd/server/main.go.
func BuildRouter() http.Handler {
	return buildRouter()
}