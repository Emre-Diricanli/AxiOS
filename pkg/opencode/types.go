package opencode

import "encoding/json"

// Event type names emitted by the opencode /event SSE stream. The server
// sends one of these as the "type" field of every frame; Properties must be
// decoded per-type by the consumer (see PermissionAsked).
const (
	// EventServerConnected is always the first event after a (re)connect.
	EventServerConnected = "server.connected"
	// EventSessionIdle fires when a session finishes processing.
	EventSessionIdle = "session.idle"
	// EventSessionError fires when a session run fails.
	EventSessionError = "session.error"
	// EventPermissionAsked fires when opencode needs approval for a tool
	// invocation; reply with Client.ReplyPermission.
	EventPermissionAsked = "permission.asked"
	// EventQuestionAsked fires when the agent asks the user a question.
	EventQuestionAsked = "question.asked"
	// EventMessagePartDelta streams incremental message-part updates
	// (e.g. assistant text as it is generated).
	EventMessagePartDelta = "message.part.delta"
	// EventMessagePartUpdated fires when a message part changes state —
	// notably tool parts transitioning pending → running → completed/error.
	EventMessagePartUpdated = "message.part.updated"
)

// Session is an opencode session as returned by POST /session.
type Session struct {
	ID        string      `json:"id"`
	ProjectID string      `json:"projectID,omitempty"`
	Directory string      `json:"directory,omitempty"`
	ParentID  string      `json:"parentID,omitempty"`
	Title     string      `json:"title,omitempty"`
	Version   string      `json:"version,omitempty"`
	Time      SessionTime `json:"time"`
}

// SessionTime holds session timestamps (unix milliseconds).
type SessionTime struct {
	Created float64 `json:"created,omitempty"`
	Updated float64 `json:"updated,omitempty"`
}

// ModelRef identifies a provider/model pair in opencode's addressing scheme,
// e.g. {ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"}.
type ModelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// Part is one piece of a message (text, tool call, file, ...). Only the
// fields AxiOS consumes are typed; type-specific payloads such as tool state
// stay raw so unknown part types decode without error.
type Part struct {
	ID        string          `json:"id,omitempty"`
	SessionID string          `json:"sessionID,omitempty"`
	MessageID string          `json:"messageID,omitempty"`
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	CallID    string          `json:"callID,omitempty"`
	State     json.RawMessage `json:"state,omitempty"`
}

// MessageInfo is the metadata half of a message returned by
// GET /session/{id}/message.
type MessageInfo struct {
	ID         string      `json:"id"`
	SessionID  string      `json:"sessionID,omitempty"`
	Role       string      `json:"role"` // "user" | "assistant"
	Time       MessageTime `json:"time"`
	Cost       float64     `json:"cost,omitempty"`
	Tokens     TokenUsage  `json:"tokens"`
	ProviderID string      `json:"providerID,omitempty"`
	ModelID    string      `json:"modelID,omitempty"`
	// Error is the provider/runtime error attached to a failed assistant
	// message. Its shape varies by error kind, so it stays raw.
	Error json.RawMessage `json:"error,omitempty"`
}

// MessageTime holds message timestamps (unix milliseconds).
type MessageTime struct {
	Created   float64 `json:"created,omitempty"`
	Completed float64 `json:"completed,omitempty"`
}

// TokenUsage is opencode's per-message token accounting.
type TokenUsage struct {
	Input     int        `json:"input"`
	Output    int        `json:"output"`
	Reasoning int        `json:"reasoning"`
	Cache     CacheUsage `json:"cache"`
}

// CacheUsage counts prompt-cache reads and writes.
type CacheUsage struct {
	Read  int `json:"read"`
	Write int `json:"write"`
}

// Message pairs message metadata with its content parts, matching the
// {"info":…,"parts":…} envelope returned by GET /session/{id}/message.
type Message struct {
	Info  MessageInfo `json:"info"`
	Parts []Part      `json:"parts"`
}

// SessionStatus is one entry of the map returned by GET /session/status.
// Verified against v1.17.0 (/doc SessionStatus schema): Type is "idle",
// "busy" or "retry"; Attempt, Message and Next accompany "retry".
type SessionStatus struct {
	Type    string `json:"type"` // "idle" | "busy" | "retry"
	Attempt int    `json:"attempt,omitempty"`
	Message string `json:"message,omitempty"`
	Next    int    `json:"next,omitempty"` // unix ms of the next retry
}

// FileDiff is one changed file as reported by GET /session/{id}/diff.
// Verified against v1.17.0 (/doc SnapshotFileDiff schema): a unified patch
// plus addition/deletion counts, NOT before/after file contents.
type FileDiff struct {
	File      string `json:"file,omitempty"`
	Patch     string `json:"patch,omitempty"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status,omitempty"` // "added" | "deleted" | "modified"
}

// ProviderModels is one provider's usable model list as reported by
// GET /config/providers.
type ProviderModels struct {
	ID     string   `json:"id"`
	Models []string `json:"models"`
}

// PartDelta is the decoded Properties payload of a message.part.delta event
// (verified against the v1.17.0 /doc: EventMessagePartDelta).
type PartDelta struct {
	SessionID string `json:"sessionID"`
	MessageID string `json:"messageID"`
	PartID    string `json:"partID"`
	Field     string `json:"field"` // "text" for assistant text deltas
	Delta     string `json:"delta"`
}

// PartUpdated is the decoded Properties payload of a message.part.updated
// event.
type PartUpdated struct {
	SessionID string `json:"sessionID"`
	Part      Part   `json:"part"`
}

// ToolState is a tool part's execution state (verified shape: ToolStatePending/
// Running/Completed/Error in the v1.17.0 /doc).
type ToolState struct {
	Status string         `json:"status"` // pending | running | completed | error
	Input  map[string]any `json:"input,omitempty"`
	Output string         `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
	Title  string         `json:"title,omitempty"`
}

// ToolState decodes the raw state of a tool part; nil for non-tool parts.
func (p *Part) ToolState() *ToolState {
	if p.Type != "tool" || len(p.State) == 0 {
		return nil
	}
	var st ToolState
	if err := json.Unmarshal(p.State, &st); err != nil {
		return nil
	}
	return &st
}

// QuestionOption is one selectable answer of a QuestionInfo.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// QuestionInfo is one question the agent asks the user (question.asked).
type QuestionInfo struct {
	Question string           `json:"question"`
	Header   string           `json:"header,omitempty"`
	Options  []QuestionOption `json:"options,omitempty"`
	Multiple bool             `json:"multiple,omitempty"`
	Custom   bool             `json:"custom,omitempty"`
}

// QuestionAsked is the decoded Properties payload of a question.asked event.
type QuestionAsked struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionID"`
	Questions []QuestionInfo `json:"questions"`
}

// PermissionAsked is the decoded Properties payload of a
// permission.asked event. Metadata is permission-type specific (for "bash"
// it carries the command, for "webfetch" the URL, ...) and stays raw.
type PermissionAsked struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionID"`
	Permission string          `json:"permission"`
	Patterns   []string        `json:"patterns,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}
