package claused

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaClient communicates with the Ollama API.
type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaClient creates a client for the Ollama API.
func NewOllamaClient(host string, port int, model string) *OllamaClient {
	return &OllamaClient{
		baseURL:    fmt.Sprintf("http://%s:%d", host, port),
		model:      model,
		httpClient: &http.Client{},
	}
}

// ollamaChatRequest is the request body for Ollama's /api/chat endpoint.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Tools    []ollamaTool        `json:"tools,omitempty"`
}

type ollamaChatMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

// ollamaChatResponse is the response from Ollama's /api/chat endpoint.
type ollamaChatResponse struct {
	Model     string            `json:"model"`
	Message   ollamaChatMessage `json:"message"`
	Done      bool              `json:"done"`
	DoneReason string           `json:"done_reason,omitempty"`
}

// ConvertTools converts Claude-format tool definitions to Ollama format.
func ConvertToolsForOllama(tools []any) []ollamaTool {
	var result []ollamaTool
	for _, t := range tools {
		toolMap, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := toolMap["name"].(string)
		desc, _ := toolMap["description"].(string)
		schema := toolMap["input_schema"]

		result = append(result, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        name,
				Description: desc,
				Parameters:  schema,
			},
		})
	}
	return result
}

// ConvertMessages converts session messages to Ollama format.
// This flattens Claude's content block format into simple role/content messages.
func ConvertMessagesForOllama(messages []Message) []ollamaChatMessage {
	var result []ollamaChatMessage
	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case string:
			result = append(result, ollamaChatMessage{
				Role:    msg.Role,
				Content: content,
			})
		case []contentBlock:
			// Assistant message with content blocks
			var text string
			for _, block := range content {
				if block.Type == "text" {
					text += block.Text
				}
			}
			if text != "" {
				result = append(result, ollamaChatMessage{
					Role:    msg.Role,
					Content: text,
				})
			}
		case []any:
			// Tool results — flatten into a single user message
			var parts []string
			for _, item := range content {
				if m, ok := item.(map[string]any); ok {
					if c, ok := m["content"].(string); ok {
						parts = append(parts, c)
					}
				}
			}
			if len(parts) > 0 {
				combined := ""
				for _, p := range parts {
					combined += p + "\n"
				}
				result = append(result, ollamaChatMessage{
					Role:    "user",
					Content: "Tool results:\n" + combined,
				})
			}
		default:
			// Try to marshal as JSON string
			data, err := json.Marshal(content)
			if err == nil {
				result = append(result, ollamaChatMessage{
					Role:    msg.Role,
					Content: string(data),
				})
			}
		}
	}
	return result
}

// Chat sends a non-streaming chat request to Ollama.
func (c *OllamaClient) Chat(system string, messages []ollamaChatMessage, tools []ollamaTool) (*ollamaChatResponse, error) {
	// Prepend system message
	allMessages := make([]ollamaChatMessage, 0, len(messages)+1)
	if system != "" {
		allMessages = append(allMessages, ollamaChatMessage{Role: "system", Content: system})
	}
	allMessages = append(allMessages, messages...)

	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: allMessages,
		Stream:   false,
		Tools:    tools,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &chatResp, nil
}

// StreamChat sends a streaming chat request to Ollama.
func (c *OllamaClient) StreamChat(system string, messages []ollamaChatMessage) (io.ReadCloser, error) {
	allMessages := make([]ollamaChatMessage, 0, len(messages)+1)
	if system != "" {
		allMessages = append(allMessages, ollamaChatMessage{Role: "system", Content: system})
	}
	allMessages = append(allMessages, messages...)

	reqBody := ollamaChatRequest{
		Model:    c.model,
		Messages: allMessages,
		Stream:   true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama stream request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(respBody))
	}

	return resp.Body, nil
}

// ParseOllamaStream reads streaming responses from Ollama.
// Each line is a JSON object with a "message" field.
func ParseOllamaStream(reader io.Reader, onChunk func(text string, done bool)) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var resp ollamaChatResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			continue
		}
		onChunk(resp.Message.Content, resp.Done)
		if resp.Done {
			break
		}
	}
	return scanner.Err()
}

// ListModels returns the names of all locally available models.
func (c *OllamaClient) ListModels() ([]string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var names []string
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// Ping checks if Ollama is reachable and the model is available.
func (c *OllamaClient) Ping() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	return nil
}
