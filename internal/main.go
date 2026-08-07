package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

var ErrInvalidPromptIndex = errors.New("Invalid prompt index")

type PromptStore struct {
	mu        sync.Mutex
	prompts   []string
	completer Completer
}

type Completer interface {
	Complete(prompt string) (string, error)
}

func NewPromptStore(completer Completer) *PromptStore {
	return &PromptStore{
		prompts:   make([]string, 0),
		completer: completer,
	}
}

func (s *PromptStore) CreatePromptEntry(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
}

func (s *PromptStore) GetResponseAt(index int) (string, error) {
	s.mu.Lock()
	prompt, ok := s.promptAt(index)
	s.mu.Unlock()
	if !ok {
		return "", ErrInvalidPromptIndex
	}
	if s.completer == nil {
		return "", errors.New("completion provider not configured")
	}
	text, err := s.completer.Complete(prompt)
	if err != nil {
		return "", fmt.Errorf("completion failed: %w", err)
	}
	return text, nil
}

func (s *PromptStore) UpdatePromptAt(index int, newPrompt string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.promptAt(index); !ok {
		return "", ErrInvalidPromptIndex
	}
	s.prompts[index] = newPrompt
	return "Prompt updated successfully", nil
}

func (s *PromptStore) DeletePromptAt(index int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.promptAt(index); !ok {
		return "", ErrInvalidPromptIndex
	}
	s.prompts = append(s.prompts[:index], s.prompts[index+1:]...)
	return "Prompt deleted successfully", nil
}

func (s *PromptStore) promptAt(index int) (string, bool) {
	if index < 0 || index >= len(s.prompts) {
		return "", false
	}
	return s.prompts[index], true
}

type createPromptRequest struct {
	Prompt string `json:"prompt"`
}

type updatePromptRequest struct {
	NewPrompt string `json:"new_prompt"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parsePromptIndex(r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "prompt_index")
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return index, true
}

type PromptHandlers struct {
	store *PromptStore
}

func NewPromptHandlers(store *PromptStore) *PromptHandlers {
	return &PromptHandlers{store: store}
}

func (h *PromptHandlers) CreatePromptHandler(w http.ResponseWriter, r *http.Request) {
	var req createPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	if req.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	h.store.CreatePromptEntry(req.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

func (h *PromptHandlers) GetResponseHandler(w http.ResponseWriter, r *http.Request) {
	index, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"response": ErrInvalidPromptIndex.Error()})
		return
	}
	response, err := h.store.GetResponseAt(index)
	if err != nil {
		if errors.Is(err, ErrInvalidPromptIndex) {
			writeJSON(w, http.StatusOK, map[string]string{"response": ErrInvalidPromptIndex.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "completion provider error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

func (h *PromptHandlers) DeletePromptHandler(w http.ResponseWriter, r *http.Request) {
	index, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": ErrInvalidPromptIndex.Error()})
		return
	}
	message, err := h.store.DeletePromptAt(index)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

func (h *PromptHandlers) UpdatePromptHandler(w http.ResponseWriter, r *http.Request) {
	var req updatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	if req.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	index, ok := parsePromptIndex(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"message": ErrInvalidPromptIndex.Error()})
		return
	}
	message, err := h.store.UpdatePromptAt(index, req.NewPrompt)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": message})
}

type noopCompleter struct{}

func (noopCompleter) Complete(string) (string, error) {
	return "", errors.New("no completion provider configured")
}

func BuildRouter() http.Handler {
	store := NewPromptStore(noopCompleter{})
	handlers := NewPromptHandlers(store)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Post("/create", handlers.CreatePromptHandler)
	r.Get("/get/{prompt_index}", handlers.GetResponseHandler)
	r.Delete("/delete/{prompt_index}", handlers.DeletePromptHandler)
	r.Put("/update/{prompt_index}", handlers.UpdatePromptHandler)

	return r
}