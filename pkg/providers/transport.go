package providers

import (
	"context"
	"io"
	"net/http"
)

// API modes understood by GetTransport.
const (
	APIModeChatCompletions   = "chat_completions"
	APIModeAnthropicMessages = "anthropic_messages"
	APIModeOllama            = "ollama"
)

// Transport converts between the canonical message format and one wire
// protocol. Implementations are stateless and safe for concurrent use.
type Transport interface {
	BuildRequest(ctx context.Context, p *Profile, apiKey, baseURL, model string,
		system string, msgs []Message, tools []ToolDef, stream bool) (*http.Request, error)
	ParseResponse(body io.Reader) (*NormalizedResponse, error)
	// ParseStream accumulates deltas (calling onDelta for text) and returns the
	// SAME NormalizedResponse shape as ParseResponse — one downstream code path.
	ParseStream(body io.Reader, onDelta func(text string)) (*NormalizedResponse, error)
}

var (
	openAITransportInstance    Transport = &openAITransport{}
	anthropicTransportInstance Transport = &anthropicTransport{}
	ollamaTransportInstance    Transport = &ollamaTransport{}
)

// GetTransport returns the transport for an API mode. Unknown modes fall back
// to the OpenAI Chat Completions transport (the de-facto compatibility wire
// format, used by the "custom" profile).
func GetTransport(apiMode string) Transport {
	switch apiMode {
	case APIModeAnthropicMessages:
		return anthropicTransportInstance
	case APIModeOllama:
		return ollamaTransportInstance
	default:
		return openAITransportInstance
	}
}

// applyProfileHeaders sets Content-Type, the profile's default headers, and
// the auth header (when apiKey is non-empty) on an outgoing request.
func applyProfileHeaders(req *http.Request, p *Profile, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	if p != nil {
		for k, v := range p.DefaultHeaders {
			req.Header.Set(k, v)
		}
	}
	if apiKey != "" {
		if p != nil {
			h, v := p.authHeader(apiKey)
			req.Header.Set(h, v)
		} else {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
}

// prepareRequestHook applies the profile's last-mile request hook, if any.
func prepareRequestHook(p *Profile, req map[string]any, model string) {
	if p != nil && p.PrepareRequest != nil {
		p.PrepareRequest(req, model)
	}
}
