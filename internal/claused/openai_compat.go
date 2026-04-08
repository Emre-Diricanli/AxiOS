package claused

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// --- OpenAI-compatible API for Open WebUI and other clients ---
//
// These endpoints allow any OpenAI-compatible client (Open WebUI, LangChain,
// curl, etc.) to use claused as an AI backend. claused aggregates all
// configured providers (cloud + Ollama) and routes requests accordingly.
//
// Endpoints:
//   GET  /v1/models           — list all available models
//   POST /v1/chat/completions — chat with streaming SSE support

// oaiModel is a single model entry in the OpenAI /v1/models response.
type oaiModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// oaiModelList is the response for GET /v1/models.
type oaiModelList struct {
	Object string     `json:"object"`
	Data   []oaiModel `json:"data"`
}

// oaiCompletionRequest is the incoming request for POST /v1/chat/completions.
type oaiCompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIMessage  `json:"messages"`
	Stream      bool             `json:"stream"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	Stop        any              `json:"stop,omitempty"`
}

// oaiCompletionResponse is the non-streaming response for POST /v1/chat/completions.
type oaiCompletionResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []oaiCompletionChoice `json:"choices"`
	Usage   oaiUsage            `json:"usage"`
}

type oaiCompletionChoice struct {
	Index        int            `json:"index"`
	Message      openAIMessage  `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// oaiStreamChunk is a single chunk in a streaming response.
type oaiStreamChunk struct {
	ID      string                `json:"id"`
	Object  string                `json:"object"`
	Created int64                 `json:"created"`
	Model   string                `json:"model"`
	Choices []oaiStreamChoice     `json:"choices"`
}

type oaiStreamChoice struct {
	Index        int            `json:"index"`
	Delta        oaiStreamDelta `json:"delta"`
	FinishReason *string        `json:"finish_reason"`
}

type oaiStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// resolveModel finds which provider and model to use for a given model ID.
// Model IDs use the format "provider/model" (e.g., "openai/gpt-4o") or
// just "model" which matches against Ollama and the active cloud provider.
type resolvedModel struct {
	providerID string // "anthropic", "openai", "ollama", etc.
	model      string // the actual model name to send to the provider
	backend    Backend // BackendCloud or BackendLocal
}

func (s *Server) resolveModel(modelID string) (*resolvedModel, error) {
	// Check for "provider/model" format (e.g., "anthropic/claude-sonnet-4-6")
	if parts := strings.SplitN(modelID, "/", 2); len(parts) == 2 {
		providerID := parts[0]
		modelName := parts[1]

		// Check if it's an Ollama host
		if providerID == "ollama" {
			return &resolvedModel{providerID: "ollama", model: modelName, backend: BackendLocal}, nil
		}

		// Check cloud providers
		if s.providerStore != nil {
			providers := s.providerStore.GetProviders()
			for _, p := range providers {
				if p.ID == providerID && p.HasKey {
					return &resolvedModel{providerID: providerID, model: modelName, backend: BackendCloud}, nil
				}
			}
		}

		return nil, fmt.Errorf("provider %q not found or has no API key", providerID)
	}

	// No prefix — check Ollama models first
	if s.ollama != nil {
		if models, err := s.ollama.ListModels(); err == nil {
			for _, m := range models {
				if m == modelID {
					return &resolvedModel{providerID: "ollama", model: modelID, backend: BackendLocal}, nil
				}
			}
		}
	}

	// Check all Ollama hosts
	if s.hostStore != nil {
		for _, host := range s.hostStore.GetHosts() {
			for _, m := range host.Models {
				if m == modelID {
					return &resolvedModel{providerID: "ollama", model: modelID, backend: BackendLocal}, nil
				}
			}
		}
	}

	// Check cloud providers for a matching model
	if s.providerStore != nil {
		for _, p := range s.providerStore.GetProviders() {
			if !p.HasKey {
				continue
			}
			for _, m := range p.Models {
				if m == modelID {
					return &resolvedModel{providerID: p.ID, model: modelID, backend: BackendCloud}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("model %q not found in any provider", modelID)
}

// handleV1Models handles GET /v1/models — returns all available models in OpenAI format.
func (s *Server) handleV1Models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.oaiError(w, "method not allowed", "invalid_request_error", http.StatusMethodNotAllowed)
		return
	}

	now := time.Now().Unix()
	var models []oaiModel

	// Cloud providers
	if s.providerStore != nil {
		for _, p := range s.providerStore.GetProviders() {
			if !p.HasKey {
				continue
			}
			for _, m := range p.Models {
				models = append(models, oaiModel{
					ID:      p.ID + "/" + m,
					Object:  "model",
					Created: now,
					OwnedBy: p.Name,
				})
			}
		}
	}

	// Local Ollama models (from the active client)
	if s.ollama != nil {
		if ollamaModels, err := s.ollama.ListModels(); err == nil {
			for _, m := range ollamaModels {
				models = append(models, oaiModel{
					ID:      "ollama/" + m,
					Object:  "model",
					Created: now,
					OwnedBy: "ollama",
				})
			}
		}
	}

	// Models from remote Ollama hosts (avoid duplicates with local)
	if s.hostStore != nil {
		localModels := make(map[string]bool)
		if s.ollama != nil {
			if lm, err := s.ollama.ListModels(); err == nil {
				for _, m := range lm {
					localModels[m] = true
				}
			}
		}

		for _, host := range s.hostStore.GetHosts() {
			if host.Status != "online" {
				continue
			}
			for _, m := range host.Models {
				if localModels[m] {
					continue // already listed from local Ollama
				}
				models = append(models, oaiModel{
					ID:      "ollama/" + m,
					Object:  "model",
					Created: now,
					OwnedBy: "ollama (" + host.Name + ")",
				})
				localModels[m] = true // prevent dupes from multiple hosts
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(oaiModelList{
		Object: "list",
		Data:   models,
	})
}

// handleV1ChatCompletions handles POST /v1/chat/completions.
// Routes to the appropriate backend based on the model requested.
func (s *Server) handleV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.oaiError(w, "method not allowed", "invalid_request_error", http.StatusMethodNotAllowed)
		return
	}

	var req oaiCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.oaiError(w, "invalid JSON: "+err.Error(), "invalid_request_error", http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		s.oaiError(w, "model is required", "invalid_request_error", http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		s.oaiError(w, "messages is required", "invalid_request_error", http.StatusBadRequest)
		return
	}

	resolved, err := s.resolveModel(req.Model)
	if err != nil {
		s.oaiError(w, err.Error(), "model_not_found", http.StatusNotFound)
		return
	}

	s.logger.Info("v1/chat/completions", "model", req.Model, "provider", resolved.providerID, "stream", req.Stream, "messages", len(req.Messages))

	switch resolved.backend {
	case BackendLocal:
		s.handleV1ChatOllama(w, r, req, resolved)
	case BackendCloud:
		if resolved.providerID == "anthropic" {
			s.handleV1ChatAnthropic(w, r, req, resolved)
		} else {
			s.handleV1ChatOpenAIProxy(w, r, req, resolved)
		}
	}
}

// handleV1ChatOllama proxies a chat request to Ollama and returns in OpenAI format.
func (s *Server) handleV1ChatOllama(w http.ResponseWriter, r *http.Request, req oaiCompletionRequest, resolved *resolvedModel) {
	if s.ollama == nil {
		s.oaiError(w, "Ollama is not configured", "server_error", http.StatusServiceUnavailable)
		return
	}

	// Convert OpenAI messages to Ollama format
	var system string
	var ollamaMessages []ollamaChatMessage
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			system = msg.Content
			continue
		}
		ollamaMessages = append(ollamaMessages, ollamaChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	if req.Stream {
		s.handleV1ChatOllamaStream(w, system, ollamaMessages, resolved.model)
		return
	}

	// Non-streaming: use a temporary client with the requested model
	client := NewOllamaClient(
		strings.TrimPrefix(s.ollama.baseURL, "http://"),
		0, // port is embedded in baseURL
		resolved.model,
	)
	// Re-parse the baseURL to get host and port for the client
	client.baseURL = s.ollama.baseURL
	client.model = resolved.model

	resp, err := client.Chat(system, ollamaMessages, nil)
	if err != nil {
		s.oaiError(w, "Ollama error: "+err.Error(), "server_error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(oaiCompletionResponse{
		ID:      "chatcmpl-axios-" + fmt.Sprint(time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resolved.model,
		Choices: []oaiCompletionChoice{
			{
				Index: 0,
				Message: openAIMessage{
					Role:    "assistant",
					Content: resp.Message.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: oaiUsage{}, // Ollama doesn't return token counts in this format
	})
}

// handleV1ChatOllamaStream handles streaming Ollama responses as SSE.
func (s *Server) handleV1ChatOllamaStream(w http.ResponseWriter, system string, messages []ollamaChatMessage, model string) {
	client := &OllamaClient{
		baseURL:    s.ollama.baseURL,
		model:      model,
		httpClient: &http.Client{},
	}

	body, err := client.StreamChat(system, messages)
	if err != nil {
		s.oaiError(w, "Ollama stream error: "+err.Error(), "server_error", http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.oaiError(w, "streaming not supported", "server_error", http.StatusInternalServerError)
		return
	}

	chatID := "chatcmpl-axios-" + fmt.Sprint(time.Now().UnixNano())
	now := time.Now().Unix()

	// Send initial role chunk
	writeSSEChunk(w, flusher, oaiStreamChunk{
		ID:      chatID,
		Object:  "chat.completion.chunk",
		Created: now,
		Model:   model,
		Choices: []oaiStreamChoice{
			{Index: 0, Delta: oaiStreamDelta{Role: "assistant"}, FinishReason: nil},
		},
	})

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		var ollamaResp ollamaChatResponse
		if err := json.Unmarshal(scanner.Bytes(), &ollamaResp); err != nil {
			continue
		}

		if ollamaResp.Done {
			stop := "stop"
			writeSSEChunk(w, flusher, oaiStreamChunk{
				ID:      chatID,
				Object:  "chat.completion.chunk",
				Created: now,
				Model:   model,
				Choices: []oaiStreamChoice{
					{Index: 0, Delta: oaiStreamDelta{}, FinishReason: &stop},
				},
			})
			break
		}

		if ollamaResp.Message.Content != "" {
			writeSSEChunk(w, flusher, oaiStreamChunk{
				ID:      chatID,
				Object:  "chat.completion.chunk",
				Created: now,
				Model:   model,
				Choices: []oaiStreamChoice{
					{Index: 0, Delta: oaiStreamDelta{Content: ollamaResp.Message.Content}, FinishReason: nil},
				},
			})
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleV1ChatAnthropic handles chat requests routed to Anthropic.
// Converts OpenAI format → Anthropic format, calls API, converts response back.
func (s *Server) handleV1ChatAnthropic(w http.ResponseWriter, r *http.Request, req oaiCompletionRequest, resolved *resolvedModel) {
	// Get the API key for Anthropic
	provider := s.getProviderWithKey("anthropic")
	if provider == nil {
		s.oaiError(w, "Anthropic API key not configured", "authentication_error", http.StatusUnauthorized)
		return
	}

	client := NewAnthropicClient(provider.APIKey, resolved.model)

	// Separate system message from conversation
	var system string
	var messages []Message
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			system = msg.Content
			continue
		}
		messages = append(messages, Message{Role: msg.Role, Content: msg.Content})
	}

	if req.Stream {
		s.handleV1ChatAnthropicStream(w, client, system, messages, resolved.model)
		return
	}

	// Non-streaming
	resp, err := client.SendMessage(system, messages, nil)
	if err != nil {
		s.oaiError(w, "Anthropic error: "+err.Error(), "server_error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.oaiError(w, "failed to read response", "server_error", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		s.oaiError(w, "Anthropic API error: "+string(body), "server_error", resp.StatusCode)
		return
	}

	// Parse Anthropic response
	var anthropicResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		s.oaiError(w, "failed to parse Anthropic response", "server_error", http.StatusBadGateway)
		return
	}

	// Extract text
	var text string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	finishReason := "stop"
	if anthropicResp.StopReason == "max_tokens" {
		finishReason = "length"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(oaiCompletionResponse{
		ID:      "chatcmpl-axios-" + fmt.Sprint(time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resolved.model,
		Choices: []oaiCompletionChoice{
			{
				Index:        0,
				Message:      openAIMessage{Role: "assistant", Content: text},
				FinishReason: finishReason,
			},
		},
		Usage: oaiUsage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	})
}

// handleV1ChatAnthropicStream streams Anthropic responses as OpenAI-format SSE.
func (s *Server) handleV1ChatAnthropicStream(w http.ResponseWriter, client *AnthropicClient, system string, messages []Message, model string) {
	stream, err := client.StreamMessage(system, messages, nil)
	if err != nil {
		s.oaiError(w, "Anthropic stream error: "+err.Error(), "server_error", http.StatusBadGateway)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.oaiError(w, "streaming not supported", "server_error", http.StatusInternalServerError)
		return
	}

	chatID := "chatcmpl-axios-" + fmt.Sprint(time.Now().UnixNano())
	now := time.Now().Unix()

	// Send initial role chunk
	writeSSEChunk(w, flusher, oaiStreamChunk{
		ID:      chatID,
		Object:  "chat.completion.chunk",
		Created: now,
		Model:   model,
		Choices: []oaiStreamChoice{
			{Index: 0, Delta: oaiStreamDelta{Role: "assistant"}, FinishReason: nil},
		},
	})

	ParseSSEStream(stream, func(eventType string, data []byte) {
		switch eventType {
		case "content_block_delta":
			var delta struct {
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(data, &delta); err != nil {
				return
			}
			if delta.Delta.Text != "" {
				writeSSEChunk(w, flusher, oaiStreamChunk{
					ID:      chatID,
					Object:  "chat.completion.chunk",
					Created: now,
					Model:   model,
					Choices: []oaiStreamChoice{
						{Index: 0, Delta: oaiStreamDelta{Content: delta.Delta.Text}, FinishReason: nil},
					},
				})
			}

		case "message_stop":
			stop := "stop"
			writeSSEChunk(w, flusher, oaiStreamChunk{
				ID:      chatID,
				Object:  "chat.completion.chunk",
				Created: now,
				Model:   model,
				Choices: []oaiStreamChoice{
					{Index: 0, Delta: oaiStreamDelta{}, FinishReason: &stop},
				},
			})
		}
	})

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleV1ChatOpenAIProxy proxies requests to OpenAI-compatible providers.
// Since the request is already in OpenAI format, we mostly forward it.
func (s *Server) handleV1ChatOpenAIProxy(w http.ResponseWriter, r *http.Request, req oaiCompletionRequest, resolved *resolvedModel) {
	provider := s.getProviderWithKey(resolved.providerID)
	if provider == nil {
		s.oaiError(w, fmt.Sprintf("provider %q has no API key", resolved.providerID), "authentication_error", http.StatusUnauthorized)
		return
	}

	// Build the upstream request — forward as-is since it's already OpenAI format
	reqBody := openAIChatRequest{
		Model:    resolved.model,
		Messages: req.Messages,
		Stream:   req.Stream,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		s.oaiError(w, "failed to encode request", "server_error", http.StatusInternalServerError)
		return
	}

	url := provider.BaseURL + "/v1/chat/completions"
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		s.oaiError(w, "failed to create upstream request", "server_error", http.StatusInternalServerError)
		return
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+provider.APIKey)

	upstreamResp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		s.oaiError(w, "upstream provider error: "+err.Error(), "server_error", http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	if req.Stream {
		// Stream: pipe SSE directly from upstream to client
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(upstreamResp.StatusCode)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		scanner := bufio.NewScanner(upstreamResp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(w, "%s\n", line)
			if line == "" {
				flusher.Flush()
			}
		}
		flusher.Flush()
	} else {
		// Non-streaming: forward the response as-is
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upstreamResp.StatusCode)
		io.Copy(w, upstreamResp.Body)
	}
}

// getProviderWithKey returns a provider by ID with its API key populated.
func (s *Server) getProviderWithKey(providerID string) *CloudProvider {
	if s.providerStore == nil {
		return nil
	}

	s.providerStore.mu.RLock()
	defer s.providerStore.mu.RUnlock()

	p, exists := s.providerStore.providers[providerID]
	if !exists || !p.HasKey {
		return nil
	}
	return p
}

// oaiError sends an error response in OpenAI API format.
func (s *Server) oaiError(w http.ResponseWriter, message, errType string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    errType,
			"code":    status,
		},
	})
}

// writeSSEChunk writes a single SSE data line and flushes.
func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, chunk oaiStreamChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
