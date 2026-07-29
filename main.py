package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// DefaultBaseURL is the default base URL for the API server.
const DefaultBaseURL = "http://127.0.0.1:5000"

// prompt stores a single prompt string.
type prompt struct {
	Text string
}

// ChatGPTBotAPI manages prompts and calls the OpenAI API.
type ChatGPTBotAPI struct {
	mu          sync.Mutex
	prompts     []prompt
	openAIKey   string
	openAIURL   string
}

// newChatGPTBotAPI creates a new ChatGPTBotAPI.
func newChatGPTBotAPI(openAIKey string) *ChatGPTBotAPI {
	return &ChatGPTBotAPI{
		openAIKey: openAIKey,
		openAIURL: "https://api.openai.com/v1/completions",
	}
}

func (c *ChatGPTBotAPI) createPrompt(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, prompt{Text: text})
}

func (c *ChatGPTBotAPI) getResponse(index int) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.prompts) {
		return "Invalid prompt index", nil
	}
	p := c.prompts[index].Text

	// MIGRATION_NOTE: The legacy Completions endpoint and text-davinci-002 have been
	// deprecated. Using gpt-3.5-turbo-instruct as a replacement for the legacy completions API.
	type completionRequest struct {
		Model     string `json:"model"`
		Prompt    string `json:"prompt"`
		MaxTokens int    `json:"max_tokens"`
	}
	type completionChoice struct {
		Text string `json:"text"`
	}
	type completionResponse struct {
		Choices []completionChoice `json:"choices"`
	}

	reqBody, err := json.Marshal(completionRequest{
		Model:     "gpt-3.5-turbo-instruct",
		Prompt:    p,
		MaxTokens: 150,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.openAIURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.openAIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var cr completionResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices returned")
	}
	return cr.Choices[0].Text, nil
}

func (c *ChatGPTBotAPI) updatePrompt(index int, newText string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.prompts) {
		return "Invalid prompt index"
	}
	c.prompts[index].Text = newText
	return "Prompt updated successfully"
}

func (c *ChatGPTBotAPI) deletePrompt(index int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.prompts) {
		return "Invalid prompt index"
	}
	c.prompts = append(c.prompts[:index], c.prompts[index+1:]...)
	return "Prompt deleted successfully"
}

// global chatbot instance
var chatbotAPI *ChatGPTBotAPI

// BuildRouter builds the HTTP router and returns it.
func BuildRouter() http.Handler {
	openAIKey := os.Getenv("OPENAI_API_KEY")
	if openAIKey == "" {
		openAIKey = "YOUR_CHATGPT_API_KEY_HERE"
	}
	chatbotAPI = newChatGPTBotAPI(openAIKey)

	mux := http.NewServeMux()
	mux.HandleFunc("/create", createPromptHandler)
	mux.HandleFunc("/get/", getResponseHandler)
	mux.HandleFunc("/delete/", deletePromptHandler)
	mux.HandleFunc("/update/", updatePromptHandler)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("failed to write JSON response")
	}
}

func parseIndexFromPath(path string, prefix string) (int, error) {
	idxStr := path[len(prefix):]
	var idx int
	_, err := fmt.Sscanf(idxStr, "%d", &idx)
	if err != nil {
		return 0, fmt.Errorf("invalid index: %w", err)
	}
	return idx, nil
}

func createPromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	p, ok := data["prompt"]
	if !ok || p == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	chatbotAPI.createPrompt(p)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

func getResponseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	idx, err := parseIndexFromPath(r.URL.Path, "/get/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	resp, err := chatbotAPI.getResponse(idx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"response": resp})
}

func deletePromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	idx, err := parseIndexFromPath(r.URL.Path, "/delete/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	msg := chatbotAPI.deletePrompt(idx)
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

func updatePromptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	idx, err := parseIndexFromPath(r.URL.Path, "/update/")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid prompt index"})
		return
	}
	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	newPrompt, ok := data["new_prompt"]
	if !ok || newPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	msg := chatbotAPI.updatePrompt(idx, newPrompt)
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

// Ensure context import is used (referenced in cmd/server/main.go via BuildRouter).
var _ = context.Background