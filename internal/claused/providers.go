package claused

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
)

// CloudProvider represents an AI API provider (Anthropic, OpenAI, Google, etc.).
type CloudProvider struct {
	ID         string   `json:"id"`         // "anthropic", "openai", "google", etc.
	Name       string   `json:"name"`       // Display name
	BaseURL    string   `json:"base_url"`   // API endpoint
	APIKey     string   `json:"-"`          // Never expose in JSON
	HasKey     bool     `json:"has_key"`    // Whether a key is configured
	Models     []string `json:"models"`     // Available models
	Active     bool     `json:"active"`     // Currently selected
	Compatible string   `json:"compatible"` // "anthropic" or "openai" (API format)
}

// providerCatalog returns the hardcoded list of supported providers.
func providerCatalog() []*CloudProvider {
	return []*CloudProvider{
		{
			ID:         "anthropic",
			Name:       "Anthropic",
			BaseURL:    "https://api.anthropic.com",
			Models:     []string{"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-4-5"},
			Compatible: "anthropic",
		},
		{
			ID:         "openai",
			Name:       "OpenAI",
			BaseURL:    "https://api.openai.com",
			Models:     []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "o1", "o1-mini", "o3-mini"},
			Compatible: "openai",
		},
		{
			ID:         "google",
			Name:       "Google (Gemini)",
			BaseURL:    "https://generativelanguage.googleapis.com/v1beta/openai",
			Models:     []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"},
			Compatible: "openai",
		},
		{
			ID:         "mistral",
			Name:       "Mistral",
			BaseURL:    "https://api.mistral.ai",
			Models:     []string{"mistral-large-latest", "mistral-medium-latest", "mistral-small-latest", "codestral-latest"},
			Compatible: "openai",
		},
		{
			ID:         "groq",
			Name:       "Groq",
			BaseURL:    "https://api.groq.com/openai",
			Models:     []string{"llama-3.1-70b-versatile", "llama-3.1-8b-instant", "mixtral-8x7b-32768", "gemma2-9b-it"},
			Compatible: "openai",
		},
		{
			ID:         "together",
			Name:       "Together AI",
			BaseURL:    "https://api.together.xyz",
			Models:     []string{"meta-llama/Llama-3.1-70B-Instruct", "meta-llama/Llama-3.1-8B-Instruct", "mistralai/Mixtral-8x7B-Instruct-v0.1"},
			Compatible: "openai",
		},
		{
			ID:         "openrouter",
			Name:       "OpenRouter",
			BaseURL:    "https://openrouter.ai/api",
			Models:     []string{"anthropic/claude-sonnet-4", "openai/gpt-4o", "google/gemini-2.5-pro", "meta-llama/llama-3.1-70b-instruct"},
			Compatible: "openai",
		},
		{
			ID:         "deepseek",
			Name:       "DeepSeek",
			BaseURL:    "https://api.deepseek.com",
			Models:     []string{"deepseek-chat", "deepseek-coder", "deepseek-reasoner"},
			Compatible: "openai",
		},
		{
			ID:         "xai",
			Name:       "xAI (Grok)",
			BaseURL:    "https://api.x.ai",
			Models:     []string{"grok-2", "grok-2-mini"},
			Compatible: "openai",
		},
		{
			ID:         "cohere",
			Name:       "Cohere",
			BaseURL:    "https://api.cohere.com/compatibility/v1",
			Models:     []string{"command-r-plus", "command-r", "command-light"},
			Compatible: "openai",
		},
		{
			ID:         "perplexity",
			Name:       "Perplexity",
			BaseURL:    "https://api.perplexity.ai",
			Models:     []string{"sonar-pro", "sonar", "sonar-reasoning"},
			Compatible: "openai",
		},
	}
}

// ProviderStore manages the collection of cloud API providers.
type ProviderStore struct {
	providers   map[string]*CloudProvider
	activeID    string
	activeModel string
	mu          sync.RWMutex
	filePath    string
}

// NewProviderStore creates a new provider store initialized with the full catalog.
func NewProviderStore(filePath string) *ProviderStore {
	ps := &ProviderStore{
		providers: make(map[string]*CloudProvider),
		filePath:  filePath,
	}
	for _, p := range providerCatalog() {
		ps.providers[p.ID] = p
	}
	return ps
}

// SetAPIKey sets the API key for a provider.
func (ps *ProviderStore) SetAPIKey(providerID, apiKey string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	p, exists := ps.providers[providerID]
	if !exists {
		return fmt.Errorf("unknown provider %q", providerID)
	}

	p.APIKey = apiKey
	p.HasKey = apiKey != ""
	return nil
}

// RemoveAPIKey removes the API key for a provider.
// If the provider is currently active, it is deactivated.
func (ps *ProviderStore) RemoveAPIKey(providerID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	p, exists := ps.providers[providerID]
	if !exists {
		return fmt.Errorf("unknown provider %q", providerID)
	}

	p.APIKey = ""
	p.HasKey = false

	// Deactivate if this was the active provider
	if ps.activeID == providerID {
		p.Active = false
		ps.activeID = ""
		ps.activeModel = ""
	}

	return nil
}

// GetProviders returns all providers with API keys masked.
func (ps *ProviderStore) GetProviders() []*CloudProvider {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]*CloudProvider, 0, len(ps.providers))
	for _, p := range ps.providers {
		cp := *p
		cp.APIKey = "" // never expose
		cp.Active = (p.ID == ps.activeID)
		result = append(result, &cp)
	}

	// Sort by ID for stable ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// SetActive sets the active cloud provider and model.
func (ps *ProviderStore) SetActive(providerID, model string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	p, exists := ps.providers[providerID]
	if !exists {
		return fmt.Errorf("unknown provider %q", providerID)
	}

	if !p.HasKey {
		return fmt.Errorf("provider %q has no API key configured", providerID)
	}

	// Validate the model is in the provider's list
	validModel := false
	for _, m := range p.Models {
		if m == model {
			validModel = true
			break
		}
	}
	if !validModel {
		return fmt.Errorf("model %q is not available for provider %q", model, providerID)
	}

	// Deactivate old
	if old, ok := ps.providers[ps.activeID]; ok {
		old.Active = false
	}

	ps.activeID = providerID
	ps.activeModel = model
	p.Active = true

	return nil
}

// GetActive returns the active provider, or nil if none is set.
func (ps *ProviderStore) GetActive() *CloudProvider {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if ps.activeID == "" {
		return nil
	}
	p, exists := ps.providers[ps.activeID]
	if !exists {
		return nil
	}
	cp := *p
	cp.Active = true
	return &cp
}

// GetActiveModel returns the name of the currently active model.
func (ps *ProviderStore) GetActiveModel() string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.activeModel
}

// --- Persistence ---

// providersFile is the JSON structure saved to disk.
type providersFile struct {
	ActiveID    string            `json:"active_id"`
	ActiveModel string           `json:"active_model"`
	Keys        map[string]string `json:"keys"` // provider ID -> base64-encoded API key
}

// SaveToFile persists API keys to a JSON file.
// TODO: Replace base64 encoding with proper encryption (e.g., AES-GCM with a machine-specific key).
func (ps *ProviderStore) SaveToFile() error {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	data := providersFile{
		ActiveID:    ps.activeID,
		ActiveModel: ps.activeModel,
		Keys:        make(map[string]string),
	}

	for id, p := range ps.providers {
		if p.APIKey != "" {
			data.Keys[id] = base64.StdEncoding.EncodeToString([]byte(p.APIKey))
		}
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal providers: %w", err)
	}
	if err := os.WriteFile(ps.filePath, raw, 0600); err != nil {
		return fmt.Errorf("write providers file: %w", err)
	}
	return nil
}

// LoadFromFile loads API keys from a JSON file.
func (ps *ProviderStore) LoadFromFile() error {
	raw, err := os.ReadFile(ps.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file — nothing to load
		}
		return fmt.Errorf("read providers file: %w", err)
	}

	var data providersFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse providers file: %w", err)
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	for id, encoded := range data.Keys {
		p, exists := ps.providers[id]
		if !exists {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue // skip corrupted entries
		}
		p.APIKey = string(decoded)
		p.HasKey = true
	}

	// Restore active provider/model only if valid
	if p, exists := ps.providers[data.ActiveID]; exists && p.HasKey {
		ps.activeID = data.ActiveID
		ps.activeModel = data.ActiveModel
		p.Active = true
	}

	return nil
}

// --- OpenAI-compatible client ---

// OpenAIClient is a generic client that works with any OpenAI-compatible API.
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIClient creates a client for an OpenAI-compatible API.
func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// openAIChatRequest is the request body for the OpenAI chat completions API.
type openAIChatRequest struct {
	Model    string           `json:"model"`
	Messages []openAIMessage  `json:"messages"`
	Stream   bool             `json:"stream"`
	Tools    []openAITool     `json:"tools,omitempty"`
}

type openAIMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content,omitempty"`
	ToolCalls  []openAIToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string           `json:"type"`
	Function openAIFunctionDef `json:"function"`
}

type openAIFunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIResponse is the response body from the OpenAI chat completions API.
type OpenAIResponse struct {
	ID      string           `json:"id"`
	Choices []openAIChoice   `json:"choices"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// SendMessage sends a non-streaming message to an OpenAI-compatible API.
func (c *OpenAIClient) SendMessage(system string, messages []Message, tools []any) (*http.Response, error) {
	// Convert internal messages to OpenAI format
	oaiMessages := convertToOpenAIMessages(system, messages)

	// Convert tool definitions to OpenAI function calling format
	oaiTools := convertToOpenAITools(tools)

	reqBody := openAIChatRequest{
		Model:    c.model,
		Messages: oaiMessages,
		Stream:   false,
		Tools:    oaiTools,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	return c.httpClient.Do(req)
}

// convertToOpenAIMessages converts internal Message format to OpenAI message format.
func convertToOpenAIMessages(system string, messages []Message) []openAIMessage {
	var oaiMessages []openAIMessage

	// Add system message first
	if system != "" {
		oaiMessages = append(oaiMessages, openAIMessage{
			Role:    "system",
			Content: system,
		})
	}

	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case string:
			oaiMessages = append(oaiMessages, openAIMessage{
				Role:    msg.Role,
				Content: content,
			})

		case []any:
			// This could be Anthropic-style content blocks (tool_use, tool_result, text)
			if msg.Role == "assistant" {
				// Extract text and tool_calls from content blocks
				oaiMsg := openAIMessage{Role: "assistant"}
				var toolCalls []openAIToolCall

				for _, block := range content {
					blockMap, ok := block.(map[string]any)
					if !ok {
						// Try contentBlock struct
						if cb, ok := block.(contentBlock); ok {
							blockMap = map[string]any{"type": cb.Type, "text": cb.Text, "id": cb.ID, "name": cb.Name, "input": cb.Input}
						} else {
							continue
						}
					}

					blockType, _ := blockMap["type"].(string)
					switch blockType {
					case "text":
						text, _ := blockMap["text"].(string)
						oaiMsg.Content = text
					case "tool_use":
						id, _ := blockMap["id"].(string)
						name, _ := blockMap["name"].(string)
						inputRaw, _ := json.Marshal(blockMap["input"])
						toolCalls = append(toolCalls, openAIToolCall{
							ID:   id,
							Type: "function",
							Function: openAIFunctionCall{
								Name:      name,
								Arguments: string(inputRaw),
							},
						})
					}
				}

				if len(toolCalls) > 0 {
					oaiMsg.ToolCalls = toolCalls
				}
				oaiMessages = append(oaiMessages, oaiMsg)

			} else if msg.Role == "user" {
				// Tool results from the user role
				for _, block := range content {
					blockMap, ok := block.(map[string]any)
					if !ok {
						continue
					}
					blockType, _ := blockMap["type"].(string)
					if blockType == "tool_result" {
						toolUseID, _ := blockMap["tool_use_id"].(string)
						resultContent, _ := blockMap["content"].(string)
						oaiMessages = append(oaiMessages, openAIMessage{
							Role:       "tool",
							Content:    resultContent,
							ToolCallID: toolUseID,
						})
					}
				}
			}

		// Handle []contentBlock (the typed version)
		case []contentBlock:
			if msg.Role == "assistant" {
				oaiMsg := openAIMessage{Role: "assistant"}
				var toolCalls []openAIToolCall

				for _, cb := range content {
					switch cb.Type {
					case "text":
						oaiMsg.Content = cb.Text
					case "tool_use":
						toolCalls = append(toolCalls, openAIToolCall{
							ID:   cb.ID,
							Type: "function",
							Function: openAIFunctionCall{
								Name:      cb.Name,
								Arguments: string(cb.Input),
							},
						})
					}
				}

				if len(toolCalls) > 0 {
					oaiMsg.ToolCalls = toolCalls
				}
				oaiMessages = append(oaiMessages, oaiMsg)
			}

		default:
			// Fallback: try to stringify
			oaiMessages = append(oaiMessages, openAIMessage{
				Role:    msg.Role,
				Content: fmt.Sprintf("%v", content),
			})
		}
	}

	return oaiMessages
}

// convertToOpenAITools converts Anthropic-style tool definitions to OpenAI function calling format.
func convertToOpenAITools(tools []any) []openAITool {
	if len(tools) == 0 {
		return nil
	}

	var oaiTools []openAITool
	for _, t := range tools {
		toolMap, ok := t.(map[string]any)
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		description, _ := toolMap["description"].(string)
		inputSchema := toolMap["input_schema"]

		oaiTools = append(oaiTools, openAITool{
			Type: "function",
			Function: openAIFunctionDef{
				Name:        name,
				Description: description,
				Parameters:  inputSchema,
			},
		})
	}

	return oaiTools
}

// --- Server integration ---

// SetProviderStore sets the provider store on the server.
func (s *Server) SetProviderStore(store *ProviderStore) {
	s.providerStore = store
}

// handleOpenAICloudMessage handles the agentic loop using an OpenAI-compatible provider.
func (s *Server) handleOpenAICloudMessage(conn interface{ WriteJSON(v any) error; ReadJSON(v any) error }, session *Session) {
	for i := 0; i < 20; i++ { // Max 20 iterations to prevent infinite loops
		messages := session.GetMessages()
		s.logger.Info("sending to openai-compatible provider", "message_count", len(messages), "iteration", i)

		resp, err := s.openaiClient.SendMessage(s.system, messages, s.tools)
		if err != nil {
			s.logger.Error("openai-compatible request failed", "error", err)
			s.sendWSMessage(conn, ChatMessage{
				Type:    "error",
				Content: fmt.Sprintf("Failed to connect to provider: %v", err),
			})
			if s.router.mode == RouteAuto {
				s.router.SetCloudAvailable(false)
			}
			return
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			s.logger.Error("read response body failed", "error", err)
			return
		}

		if resp.StatusCode != http.StatusOK {
			s.logger.Error("openai-compatible API error", "status", resp.StatusCode, "body", string(body))
			s.sendWSMessage(conn, ChatMessage{
				Type:    "error",
				Content: fmt.Sprintf("API error %d: %s", resp.StatusCode, string(body)),
			})
			return
		}

		var apiResp OpenAIResponse
		if err := json.Unmarshal(body, &apiResp); err != nil {
			s.logger.Error("unmarshal openai response failed", "error", err)
			return
		}

		if len(apiResp.Choices) == 0 {
			s.logger.Error("openai response has no choices")
			return
		}

		choice := apiResp.Choices[0]
		s.logger.Info("openai-compatible response", "finish_reason", choice.FinishReason, "tool_calls", len(choice.Message.ToolCalls))

		// Process the response
		var hasToolUse bool
		var assistantContentBlocks []any
		var toolResults []any

		// Handle text content
		if choice.Message.Content != "" {
			s.sendWSMessage(conn, ChatMessage{
				Type:    "assistant",
				Content: choice.Message.Content,
				Model:   s.openaiClient.model,
			})
			assistantContentBlocks = append(assistantContentBlocks, map[string]any{
				"type": "text",
				"text": choice.Message.Content,
			})
		}

		// Handle tool calls
		for _, tc := range choice.Message.ToolCalls {
			hasToolUse = true
			s.logger.Info("tool use requested", "tool", tc.Function.Name, "id", tc.ID)

			// Tell the UI what tool is being called
			s.sendWSMessage(conn, ChatMessage{
				Type:     "tool_use",
				ToolName: tc.Function.Name,
				ToolID:   tc.ID,
				Content:  tc.Function.Arguments,
			})

			// Execute the tool via MCP
			result := s.executeTool(tc.Function.Name, tc.ID, json.RawMessage(tc.Function.Arguments))

			// Tell the UI the result
			s.sendWSMessage(conn, ChatMessage{
				Type:     "tool_result",
				ToolID:   tc.ID,
				ToolName: tc.Function.Name,
				Content:  result,
			})

			// Build Anthropic-style content blocks for session storage
			assistantContentBlocks = append(assistantContentBlocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": json.RawMessage(tc.Function.Arguments),
			})

			toolResults = append(toolResults, map[string]any{
				"type":        "tool_result",
				"tool_use_id": tc.ID,
				"content":     result,
			})
		}

		// Add assistant message to session
		if len(assistantContentBlocks) > 0 {
			session.AddMessage(Message{Role: "assistant", Content: assistantContentBlocks})
		} else {
			session.AddMessage(Message{Role: "assistant", Content: choice.Message.Content})
		}

		if !hasToolUse || (choice.FinishReason != "tool_calls" && choice.FinishReason != "function_call") {
			s.logger.Info("loop done", "hasToolUse", hasToolUse, "finish_reason", choice.FinishReason)
			return
		}

		s.logger.Info("continuing agentic loop with tool results", "tool_count", len(toolResults))

		// Add tool results to session and loop again
		session.AddMessage(Message{Role: "user", Content: toolResults})
	}

	s.logger.Warn("hit max agentic loop iterations")
}

// sendWSMessage sends a ChatMessage to a WebSocket-like connection.
func (s *Server) sendWSMessage(conn interface{ WriteJSON(v any) error; ReadJSON(v any) error }, msg ChatMessage) {
	if err := conn.WriteJSON(msg); err != nil {
		s.logger.Error("websocket write failed", "error", err)
	}
}

// --- HTTP Handlers ---

// handleProviders handles GET /api/providers — list all providers with has_key status.
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	if s.providerStore == nil {
		s.jsonError(w, "provider management not initialized", http.StatusServiceUnavailable)
		return
	}

	providers := s.providerStore.GetProviders()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"providers":    providers,
		"active_model": s.providerStore.GetActiveModel(),
	})
}

// handleProviderKey handles POST /api/providers/key (set key) and DELETE /api/providers/key (remove key).
func (s *Server) handleProviderKey(w http.ResponseWriter, r *http.Request) {
	if s.providerStore == nil {
		s.jsonError(w, "provider management not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Provider string `json:"provider"`
			APIKey   string `json:"api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Provider == "" || req.APIKey == "" {
			s.jsonError(w, "provider and api_key are required", http.StatusBadRequest)
			return
		}

		if err := s.providerStore.SetAPIKey(req.Provider, req.APIKey); err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// If this is the anthropic provider, update the AnthropicClient too
		if req.Provider == "anthropic" {
			model := "claude-sonnet-4-6"
			if s.anthropic != nil {
				model = s.anthropic.model
			}
			s.anthropic = NewAnthropicClient(req.APIKey, model)
			s.router.SetCloudAvailable(true)
		}

		s.logger.Info("API key set", "provider", req.Provider)

		// Auto-save
		if err := s.providerStore.SaveToFile(); err != nil {
			s.logger.Error("failed to save providers", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "provider": req.Provider})

	case http.MethodDelete:
		providerID := r.URL.Query().Get("provider")
		if providerID == "" {
			s.jsonError(w, "provider query parameter required", http.StatusBadRequest)
			return
		}

		if err := s.providerStore.RemoveAPIKey(providerID); err != nil {
			s.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// If removing anthropic key, clear the client
		if providerID == "anthropic" {
			s.anthropic = nil
			// Check if any other provider has a key
			active := s.providerStore.GetActive()
			if active == nil {
				s.router.SetCloudAvailable(false)
			}
		}

		s.logger.Info("API key removed", "provider", providerID)

		// Auto-save
		if err := s.providerStore.SaveToFile(); err != nil {
			s.logger.Error("failed to save providers", "error", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "removed": providerID})

	default:
		s.jsonError(w, "POST or DELETE required", http.StatusMethodNotAllowed)
	}
}

// handleProviderActivate handles POST /api/providers/activate — activate a provider and model.
func (s *Server) handleProviderActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	if s.providerStore == nil {
		s.jsonError(w, "provider management not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Provider == "" || req.Model == "" {
		s.jsonError(w, "provider and model are required", http.StatusBadRequest)
		return
	}

	if err := s.providerStore.SetActive(req.Provider, req.Model); err != nil {
		s.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Update the appropriate client
	active := s.providerStore.GetActive()
	if active != nil {
		if active.Compatible == "anthropic" {
			s.anthropic = NewAnthropicClient(active.APIKey, req.Model)
			s.openaiClient = nil
		} else {
			s.openaiClient = NewOpenAIClient(active.BaseURL, active.APIKey, req.Model)
		}
		s.router.SetCloudAvailable(true)
		s.router.mode = RouteCloudOnly
	}

	s.logger.Info("provider activated", "provider", req.Provider, "model", req.Model)

	// Auto-save
	if err := s.providerStore.SaveToFile(); err != nil {
		s.logger.Error("failed to save providers", "error", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"provider": req.Provider,
		"model":    req.Model,
	})
}
