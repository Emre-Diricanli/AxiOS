package claused

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// AuthType indicates how the client authenticates with the Anthropic API.
type AuthType string

const (
	AuthAPIKey AuthType = "api_key" // Standard API key (sk-ant-api03-...)
	AuthOAuth  AuthType = "oauth"   // OAuth token from claude setup-token (sk-ant-oat01-...)
)

// AnthropicClient communicates with the Anthropic Messages API.
type AnthropicClient struct {
	token      string
	authType   AuthType
	model      string
	httpClient *http.Client
}

// NewAnthropicClient creates a client for the Anthropic API.
// Automatically detects whether the token is an API key or OAuth token.
func NewAnthropicClient(token, model string) *AnthropicClient {
	authType := DetectAuthType(token)
	return &AnthropicClient{
		token:      token,
		authType:   authType,
		model:      model,
		httpClient: &http.Client{},
	}
}

// DetectAuthType determines the auth type from the token prefix.
func DetectAuthType(token string) AuthType {
	if len(token) >= 14 && token[:14] == "sk-ant-oat01-" {
		return AuthOAuth
	}
	return AuthAPIKey
}

// Message represents a conversation message.
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// MessagesRequest is the request body for the Anthropic Messages API.
type MessagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	Tools     []any     `json:"tools,omitempty"`
}

// StreamEvent represents a server-sent event from the streaming API.
type StreamEvent struct {
	Type  string          `json:"type"`
	Delta json.RawMessage `json:"delta,omitempty"`
	Index int             `json:"index,omitempty"`
}

// SendMessage sends a non-streaming message and returns the response.
func (c *AnthropicClient) SendMessage(system string, messages []Message, tools []any) (*http.Response, error) {
	reqBody := MessagesRequest{
		Model:     c.model,
		MaxTokens: 8192,
		System:    system,
		Messages:  messages,
		Stream:    false,
		Tools:     tools,
	}

	return c.doRequest(reqBody)
}

// StreamMessage sends a streaming message request.
// Returns a reader that yields server-sent events.
func (c *AnthropicClient) StreamMessage(system string, messages []Message, tools []any) (io.ReadCloser, error) {
	reqBody := MessagesRequest{
		Model:     c.model,
		MaxTokens: 8192,
		System:    system,
		Messages:  messages,
		Stream:    true,
		Tools:     tools,
	}

	resp, err := c.doRequest(reqBody)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// ParseSSEStream reads server-sent events from a streaming response.
func ParseSSEStream(reader io.Reader, onEvent func(eventType string, data []byte)) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		if len(line) == 0 {
			continue
		}

		if bytes.HasPrefix([]byte(line), []byte("data: ")) {
			data := []byte(line[6:])
			var event struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(data, &event); err == nil {
				onEvent(event.Type, data)
			}
		}
	}
	return scanner.Err()
}

func (c *AnthropicClient) doRequest(reqBody MessagesRequest) (*http.Response, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	switch c.authType {
	case AuthOAuth:
		req.Header.Set("Authorization", "Bearer "+c.token)
	default:
		req.Header.Set("x-api-key", c.token)
	}

	return c.httpClient.Do(req)
}
