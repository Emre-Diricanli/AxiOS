package axiosd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/axios-os/axios/pkg/providers"
)

// --- OpenAI-compatible API for Open WebUI and other clients ---
//
// These endpoints allow any OpenAI-compatible client (Open WebUI, LangChain,
// curl, etc.) to use axiosd as an AI backend. axiosd aggregates all
// configured providers (cloud + Ollama) and routes every request through the
// model-agnostic provider layer (pkg/providers) — there is no per-vendor
// translation code here.
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

// oaiChatMessage is one incoming/outgoing message on the facade. The facade
// accepts plain text conversations; tool calls are the chat WebSocket's job.
type oaiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// oaiCompletionRequest is the incoming request for POST /v1/chat/completions.
type oaiCompletionRequest struct {
	Model       string           `json:"model"`
	Messages    []oaiChatMessage `json:"messages"`
	Stream      bool             `json:"stream"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	Stop        any              `json:"stop,omitempty"`
}

// oaiCompletionResponse is the non-streaming response for POST /v1/chat/completions.
type oaiCompletionResponse struct {
	ID      string                `json:"id"`
	Object  string                `json:"object"`
	Created int64                 `json:"created"`
	Model   string                `json:"model"`
	Choices []oaiCompletionChoice `json:"choices"`
	Usage   oaiUsage              `json:"usage"`
}

type oaiCompletionChoice struct {
	Index        int            `json:"index"`
	Message      oaiChatMessage `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// oaiStreamChunk is a single chunk in a streaming response.
type oaiStreamChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []oaiStreamChoice `json:"choices"`
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
	providerID string  // "anthropic", "openai", "ollama", etc.
	model      string  // the actual model name to send to the provider
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

// splitOAIMessages separates the system prompt from the conversation and
// converts the remainder into canonical messages.
func splitOAIMessages(in []oaiChatMessage) (system string, msgs []providers.Message) {
	for _, m := range in {
		if m.Role == "system" {
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			continue
		}
		msgs = append(msgs, providers.Message{Role: m.Role, Content: m.Content})
	}
	return system, msgs
}

// handleV1ChatCompletions handles POST /v1/chat/completions.
// Every backend is served by the same provider-layer client.
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

	if s.runtime == nil {
		s.oaiError(w, "provider runtime not initialized", "server_error", http.StatusServiceUnavailable)
		return
	}
	client, err := s.runtime.ClientFor(resolved.providerID, resolved.model)
	if err != nil {
		s.oaiError(w, err.Error(), "authentication_error", http.StatusUnauthorized)
		return
	}

	system, msgs := splitOAIMessages(req.Messages)

	if req.Stream {
		s.streamV1Chat(w, r, client, system, msgs, resolved.model)
		return
	}

	resp, err := client.Complete(r.Context(), system, msgs, nil)
	if err != nil {
		s.oaiError(w, err.Error(), "server_error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(oaiCompletionResponse{
		ID:      newChatCompletionID(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resolved.model,
		Choices: []oaiCompletionChoice{
			{
				Index:        0,
				Message:      oaiChatMessage{Role: "assistant", Content: resp.Content},
				FinishReason: oaiFinishReason(resp.FinishReason),
			},
		},
		Usage: oaiUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	})
}

// streamV1Chat streams one completion as OpenAI-format SSE chunks.
func (s *Server) streamV1Chat(w http.ResponseWriter, r *http.Request, client *providers.Client, system string, msgs []providers.Message, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.oaiError(w, "streaming not supported", "server_error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	chatID := newChatCompletionID()
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

	resp, err := client.Stream(r.Context(), system, msgs, nil, func(text string) {
		if text == "" {
			return
		}
		writeSSEChunk(w, flusher, oaiStreamChunk{
			ID:      chatID,
			Object:  "chat.completion.chunk",
			Created: now,
			Model:   model,
			Choices: []oaiStreamChoice{
				{Index: 0, Delta: oaiStreamDelta{Content: text}, FinishReason: nil},
			},
		})
	})
	if err != nil {
		s.logger.Error("v1 stream failed", "provider", client.Name(), "model", model, "error", err)
		// The SSE stream is already open; surface the error in-band.
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "server_error"},
		}))
		flusher.Flush()
		return
	}

	finish := oaiFinishReason(resp.FinishReason)
	writeSSEChunk(w, flusher, oaiStreamChunk{
		ID:      chatID,
		Object:  "chat.completion.chunk",
		Created: now,
		Model:   model,
		Choices: []oaiStreamChoice{
			{Index: 0, Delta: oaiStreamDelta{}, FinishReason: &finish},
		},
	})

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// oaiFinishReason maps a canonical finish reason onto the OpenAI wire value.
// The canonical values already use the OpenAI vocabulary; this only guards
// against gaps.
func oaiFinishReason(reason string) string {
	if reason == "" {
		return "stop"
	}
	return reason
}

// newChatCompletionID generates a facade response ID.
func newChatCompletionID() string {
	return "chatcmpl-axios-" + fmt.Sprint(time.Now().UnixNano())
}

// mustJSON marshals v, returning "{}" on failure (used for in-band SSE errors).
func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
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
