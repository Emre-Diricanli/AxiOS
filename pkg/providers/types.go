// Package providers implements the model-agnostic provider layer for AxiOS.
//
// The design follows a two-layer split (modeled on Nous Research's
// hermes-agent): declarative ProviderProfiles describe each vendor
// (endpoints, credentials, quirks) while wire-protocol Transports convert a
// single canonical message format at the HTTP boundary only.
//
// The canonical internal format is the OpenAI Chat Completions shape:
// messages with role/content, assistant tool_calls, role:"tool" results, and
// tools as OpenAI function-calling JSON schema. Nothing outside a Transport
// ever sees a provider wire format.
package providers

import "encoding/json"

// Message is the canonical conversation message (OpenAI Chat Completions shape).
type Message struct {
	Role       string     `json:"role"` // system|user|assistant|tool
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role:"tool"
	Name       string     `json:"name,omitempty"`
	// ProviderData holds protocol replay state (e.g. Anthropic signed thinking
	// blocks) keyed by transport; stripped before sending to a different transport.
	ProviderData map[string]json.RawMessage `json:"provider_data,omitempty"`
}

// ToolCall is a model-initiated tool invocation in canonical form.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ToolDef describes a tool offered to the model (OpenAI function-calling schema).
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Usage holds token accounting for a single completion.
type Usage struct{ InputTokens, OutputTokens int }

// NormalizedResponse is the provider-independent result of one completion,
// identical whether it was produced by ParseResponse or ParseStream.
type NormalizedResponse struct {
	Content      string
	Reasoning    string
	ToolCalls    []ToolCall
	FinishReason string // stop|tool_calls|length|content_filter
	Usage        Usage
	ProviderData map[string]json.RawMessage
}

// Canonical finish reasons.
const (
	FinishStop          = "stop"
	FinishToolCalls     = "tool_calls"
	FinishLength        = "length"
	FinishContentFilter = "content_filter"
)
