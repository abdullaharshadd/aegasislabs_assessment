package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

type ChatGPTBotAPI struct {
	mu      sync.Mutex
	prompts []string
	apiKey  string
}

func NewChatGPTBotAPI(apiKey string) *ChatGPTBotAPI {
	return &ChatGPTBotAPI{
		apiKey:  apiKey,
		prompts: []string{},
	}
}

func (c *ChatGPTBotAPI) CreatePrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, prompt)
}

func (c *ChatGPTBotAPI) GetResponse(promptIndex int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(c.prompts) {
		return "Invalid prompt index"
	}
	// MIGRATION_NOTE: The legacy openai.Completion.create with engine 'text-davinci-002'
	// has been deprecated. Implement with current OpenAI API client as needed.
	prompt := c.prompts[promptIndex]
	_ = prompt
	return "MIGRATION_NOTE: OpenAI completion not implemented - please integrate current OpenAI Go SDK"
}

func (c *ChatGPTBotAPI) UpdatePrompt(promptIndex int, newPrompt string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(c.prompts) {
		return "Invalid prompt index"
	}
	c.prompts[promptIndex] = newPrompt
	return "Prompt updated successfully"
}

func (c *ChatGPTBotAPI) DeletePrompt(promptIndex int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(c.prompts) {
		return "Invalid prompt index"
	}
	c.prompts = append(c.prompts[:promptIndex], c.prompts[promptIndex+1:]...)
	return "Prompt deleted successfully"
}

var chatbotAPI *ChatGPTBotAPI

func BuildRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
			return
		}
		prompt, ok := data["prompt"]
		if !ok || prompt == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Prompt not provided"})
			return
		}
		chatbotAPI.CreatePrompt(prompt)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"message": "Prompt created successfully"})
	})

	mux.HandleFunc("/get/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/get/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid prompt index"})
			return
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid prompt index"})
			return
		}
		response := chatbotAPI.GetResponse(idx)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"response": response})
	})

	mux.HandleFunc("/delete/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/delete/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid prompt index"})
			return
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid prompt index"})
			return
		}
		response := chatbotAPI.DeletePrompt(idx)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": response})
	})

	mux.HandleFunc("/update/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/update/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid prompt index"})
			return
		}
		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid prompt index"})
			return
		}
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
			return
		}
		newPrompt, ok := data["new_prompt"]
		if !ok || newPrompt == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "New prompt not provided"})
			return
		}
		response := chatbotAPI.UpdatePrompt(idx, newPrompt)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": response})
	})

	return mux
}

func main() {
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		openaiAPIKey = "YOUR_CHATGPT_API_KEY_HERE"
	}
	chatbotAPI = NewChatGPTBotAPI(openaiAPIKey)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: BuildRouter(),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	log.Info().Msgf("server started on :%s", port)
	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("shutdown error")
	}
}