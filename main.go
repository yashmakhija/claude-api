package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config
var (
	anthropicAPIKey string
	port            string
	defaultModel    = "claude-sonnet-4-20250514"
	clientAPIKey    = "" // Set via CLIENT_API_KEY env var
)

// Request/Response types
type ChatRequest struct {
	Message   string   `json:"message"`
	Model     string   `json:"model,omitempty"`
	System    string   `json:"system,omitempty"`
	MaxTokens int      `json:"max_tokens,omitempty"`
	Stream    bool     `json:"stream,omitempty"`
	History   []Message `json:"history,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Response  string `json:"response"`
	Model     string `json:"model"`
	Usage     Usage  `json:"usage"`
	LatencyMs int64  `json:"latency_ms"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Anthropic API types
type AnthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream,omitempty"`
}

type AnthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type StreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta,omitempty"`
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

func main() {
	// Load config from env
	anthropicAPIKey = os.Getenv("ANTHROPIC_API_KEY")
	if anthropicAPIKey == "" {
		// Try to read from OpenClaw config
		anthropicAPIKey = readOpenClawKey()
	}
	if anthropicAPIKey == "" {
		log.Fatal("ANTHROPIC_API_KEY not set")
	}

	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if m := os.Getenv("DEFAULT_MODEL"); m != "" {
		defaultModel = m
	}

	// Require client API key from env
	clientAPIKey = os.Getenv("CLIENT_API_KEY")
	if clientAPIKey == "" {
		log.Fatal("CLIENT_API_KEY not set")
	}

	// Routes (with auth middleware)
	http.HandleFunc("/health", healthHandler) // health is public
	http.HandleFunc("/chat", requireAuth(chatHandler))
	http.HandleFunc("/stream", requireAuth(streamHandler))
	http.HandleFunc("/messages", requireAuth(messagesHandler)) // Transparent proxy for tool use

	log.Printf("🚀 Claude API running on port %s", port)
	log.Printf("📡 Endpoints: /chat, /stream, /messages (POST), /health (GET)")
	log.Printf("🔐 API Key required: X-API-Key header")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func readOpenClawKey() string {
	// Try OpenClaw auth-profiles.json (where setup-token stores the key)
	authPaths := []string{
		os.Getenv("HOME") + "/.openclaw/agents/main/agent/auth-profiles.json",
		"/root/.openclaw/agents/main/agent/auth-profiles.json",
	}

	for _, path := range authPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var auth map[string]interface{}
		if err := json.Unmarshal(data, &auth); err != nil {
			continue
		}

		// Get token from profiles.anthropic:default.token
		if profiles, ok := auth["profiles"].(map[string]interface{}); ok {
			if anthropic, ok := profiles["anthropic:default"].(map[string]interface{}); ok {
				if token, ok := anthropic["token"].(string); ok {
					log.Printf("✅ Found Anthropic token from OpenClaw auth-profiles")
					return token
				}
			}
		}
	}

	// Fallback: try config.json
	configPaths := []string{
		os.Getenv("HOME") + "/.openclaw/config.json",
		"/root/.openclaw/config.json",
	}

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var config map[string]interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			continue
		}

		if providers, ok := config["providers"].(map[string]interface{}); ok {
			if anthropic, ok := providers["anthropic"].(map[string]interface{}); ok {
				if key, ok := anthropic["apiKey"].(string); ok {
					return key
				}
			}
		}
	}

	return ""
}

// Auth middleware - checks X-API-Key header
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check X-API-Key header
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// Also check Authorization: Bearer
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if apiKey != clientAPIKey {
			log.Printf("❌ AUTH FAILED: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid or missing API key. Use X-API-Key header.",
			})
			return
		}

		// Log successful request
		log.Printf("📥 %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		next(w, r)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"model":  defaultModel,
	})
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	model := req.Model
	if model == "" {
		model = defaultModel
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Build messages
	messages := req.History
	messages = append(messages, Message{Role: "user", Content: req.Message})

	// Call Anthropic
	response, err := callAnthropic(model, req.System, messages, maxTokens, false)
	if err != nil {
		http.Error(w, "API error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	latency := time.Since(start).Milliseconds()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChatResponse{
		Response:  response.Content[0].Text,
		Model:     response.Model,
		Usage: Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
		},
		LatencyMs: latency,
	})
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	model := req.Model
	if model == "" {
		model = defaultModel
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	messages := req.History
	messages = append(messages, Message{Role: "user", Content: req.Message})

	// Set up SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Call Anthropic with streaming
	err := streamAnthropic(w, flusher, model, req.System, messages, maxTokens)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", err.Error())
		flusher.Flush()
	}
}

// messagesHandler - transparent proxy to Anthropic Messages API (for tool use)
func messagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	// Read the entire request body as-is
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse request to log model and tool count
	var reqData map[string]interface{}
	if err := json.Unmarshal(body, &reqData); err == nil {
		model := reqData["model"]
		toolCount := 0
		if tools, ok := reqData["tools"].([]interface{}); ok {
			toolCount = len(tools)
		}
		msgCount := 0
		if msgs, ok := reqData["messages"].([]interface{}); ok {
			msgCount = len(msgs)
		}
		log.Printf("   → model=%v tools=%d messages=%d", model, toolCount, msgCount)
	}

	// Create request to Anthropic
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(body))
	if err != nil {
		http.Error(w, "Failed to create request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Forward to Anthropic
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("   ✗ Anthropic error: %v", err)
		http.Error(w, "Anthropic API error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Log response info
	latency := time.Since(start).Milliseconds()
	var respData map[string]interface{}
	if err := json.Unmarshal(respBody, &respData); err == nil {
		stopReason := respData["stop_reason"]
		// Count tool_use blocks in response
		toolUseCount := 0
		if content, ok := respData["content"].([]interface{}); ok {
			for _, block := range content {
				if b, ok := block.(map[string]interface{}); ok {
					if b["type"] == "tool_use" {
						toolUseCount++
						if name, ok := b["name"].(string); ok {
							log.Printf("   🔧 tool_use: %s", name)
						}
					}
				}
			}
		}
		if usage, ok := respData["usage"].(map[string]interface{}); ok {
			log.Printf("   ✓ %dms | stop=%v | tools_called=%d | in=%v out=%v",
				latency, stopReason, toolUseCount, usage["input_tokens"], usage["output_tokens"])
		}
	}

	// Return response as-is with same status code
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// Claude Code version for stealth mode
const claudeCodeVersion = "2.1.2"

// Set auth headers based on token type
func setAuthHeaders(req *http.Request) {
	if strings.HasPrefix(anthropicAPIKey, "sk-ant-oat") {
		// OAuth token (setup-token): Mimic Claude Code's headers exactly
		req.Header.Set("Authorization", "Bearer "+anthropicAPIKey)
		req.Header.Set("accept", "application/json")
		req.Header.Set("anthropic-dangerous-direct-browser-access", "true")
		req.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20,fine-grained-tool-streaming-2025-05-14")
		req.Header.Set("user-agent", fmt.Sprintf("claude-cli/%s (external, cli)", claudeCodeVersion))
		req.Header.Set("x-app", "cli")
	} else {
		// Regular API key
		req.Header.Set("x-api-key", anthropicAPIKey)
	}
}

func callAnthropic(model, system string, messages []Message, maxTokens int, stream bool) (*AnthropicResponse, error) {
	reqBody := AnthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  messages,
		Stream:    stream,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func streamAnthropic(w http.ResponseWriter, flusher http.Flusher, model, system string, messages []Message, maxTokens int) error {
	reqBody := AnthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  messages,
		Stream:    true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event StreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta.Text != "" {
				fmt.Fprintf(w, "data: {\"text\": %q}\n\n", event.Delta.Text)
				flusher.Flush()
			}
		case "message_stop":
			fmt.Fprintf(w, "data: {\"done\": true}\n\n")
			flusher.Flush()
		}
	}

	return scanner.Err()
}
