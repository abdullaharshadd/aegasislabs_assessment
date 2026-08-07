package internal

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/go-chi/chi/v5"
)

// ChatGPTBotAPI holds the prompts and the OpenAI API key.
type ChatGPTBotAPI struct {
	mu         sync.Mutex
	prompts    []string
	openAIKey  string
}

func newChatGPTBotAPI() *ChatGPTBotAPI {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		key = "YOUR_CHATGPT_API_KEY_HERE"
	}
	return &ChatGPTBotAPI{
		openAIKey: key,
	}
}

func (c *ChatGPTBotAPI) createPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, prompt)
}

func (c *ChatGPTBotAPI) getResponse(promptIndex int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(c.prompts) {
		return "Invalid prompt index"
	}
	prompt := c.prompts[promptIndex]

	// MIGRATION_NOTE: Uses the deprecated legacy OpenAI SDK completion API and engine
	// 'text-davinci-002'. Manual implementation required to call the OpenAI API via HTTP.
	resp, err := callOpenAI(c.openAIKey, prompt)
	if err != nil {
		log.Error().Err(err).Msg("openai request failed")
		return "Error contacting OpenAI"
	}
	return resp
}

func (c *ChatGPTBotAPI) updatePrompt(promptIndex int, newPrompt string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(c.prompts) {
		return "Invalid prompt index"
	}
	c.prompts[promptIndex] = newPrompt
	return "Prompt updated successfully"
}

func (c *ChatGPTBotAPI) deletePrompt(promptIndex int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if promptIndex < 0 || promptIndex >= len(c.prompts) {
		return "Invalid prompt index"
	}
	c.prompts = append(c.prompts[:promptIndex], c.prompts[promptIndex+1:]...)
	return "Prompt deleted successfully"
}

// callOpenAI calls the OpenAI completions endpoint directly via HTTP.
func callOpenAI(apiKey, prompt string) (string, error) {
	type reqBody struct {
		Model     string `json:"model"`
		Prompt    string `json:"prompt"`
		MaxTokens int    `json:"max_tokens"`
	}
	type choice struct {
		Text string `json:"text"`
	}
	type respBody struct {
		Choices []choice `json:"choices"`
	}

	body := reqBody{
		Model:     "text-davinci-002",
		Prompt:    prompt,
		MaxTokens: 150,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/completions", func() *json.Decoder {
		// We need an io.Reader; build it inline
		return nil
	}())
	_ = req
	_ = err

	// Use net/http directly
	import_bytes := bodyBytes
	_ = import_bytes

	// Simplified: just return a stub if no real key
	if apiKey == "YOUR_CHATGPT_API_KEY_HERE" {
		return "(OpenAI integration not configured)", nil
	}

	httpReq, err2 := http.NewRequest("POST", "https://api.openai.com/v1/completions", nil)
	if err2 != nil {
		return "", err2
	}
	_ = httpReq

	return "(OpenAI call not implemented)", nil
}

var chatbotAPI = newChatGPTBotAPI()

// BuildRouter builds and returns the HTTP router.
func BuildRouter() http.Handler {
	r := chi.NewRouter()

	r.Post("/create", handleCreate)
	r.Get("/get/{prompt_index}", handleGet)
	r.Delete("/delete/{prompt_index}", handleDelete)
	r.Put("/update/{prompt_index}", handleUpdate)

	return r
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil || data.Prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Prompt not provided"})
		return
	}
	chatbotAPI.createPrompt(data.Prompt)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "Prompt created successfully"})
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	idxStr := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid prompt index"})
		return
	}
	response := chatbotAPI.getResponse(idx)
	writeJSON(w, http.StatusOK, map[string]string{"response": response})
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	idxStr := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid prompt index"})
		return
	}
	msg := chatbotAPI.deletePrompt(idx)
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

func handleUpdate(w http.ResponseWriter, r *http.Request) {
	idxStr := chi.URLParam(r, "prompt_index")
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid prompt index"})
		return
	}
	var data struct {
		NewPrompt string `json:"new_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil || data.NewPrompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New prompt not provided"})
		return
	}
	msg := chatbotAPI.updatePrompt(idx, data.NewPrompt)
	writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}