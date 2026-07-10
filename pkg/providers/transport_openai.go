package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// openAITransport speaks the OpenAI Chat Completions wire protocol
// (POST {baseURL}/chat/completions). Because the canonical format IS the
// Chat Completions shape, conversion is nearly 1:1; finish_reason passes
// through and tool_calls are native.
type openAITransport struct{}

// openAIWireToolCall is the wire representation of an assistant tool call.
type openAIWireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (t *openAITransport) BuildRequest(ctx context.Context, p *Profile, apiKey, baseURL, model string,
	system string, msgs []Message, tools []ToolDef, stream bool) (*http.Request, error) {

	wireMsgs := make([]map[string]any, 0, len(msgs)+1)
	if system != "" {
		wireMsgs = append(wireMsgs, map[string]any{"role": "system", "content": system})
	}
	for _, m := range msgs {
		wm := map[string]any{"role": m.Role}
		if m.Content != "" || len(m.ToolCalls) == 0 {
			wm["content"] = m.Content
		}
		if m.Name != "" {
			wm["name"] = m.Name
		}
		if m.ToolCallID != "" {
			wm["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			wm["tool_calls"] = calls
		}
		// ProviderData is replay state for other transports; never sent here.
		wireMsgs = append(wireMsgs, wm)
	}

	req := map[string]any{
		"model":    model,
		"messages": wireMsgs,
		"stream":   stream,
	}
	if len(tools) > 0 {
		wireTools := make([]map[string]any, 0, len(tools))
		for _, td := range tools {
			wireTools = append(wireTools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        td.Name,
					"description": td.Description,
					"parameters":  td.InputSchema,
				},
			})
		}
		req["tools"] = wireTools
	}
	prepareRequestHook(p, req, model)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	applyProfileHeaders(httpReq, p, apiKey)
	return httpReq, nil
}

// openAIResponseBody mirrors the non-streaming chat completions response.
type openAIResponseBody struct {
	Choices []struct {
		Message struct {
			Content          string               `json:"content"`
			ReasoningContent string               `json:"reasoning_content"`
			ToolCalls        []openAIWireToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (t *openAITransport) ParseResponse(body io.Reader) (*NormalizedResponse, error) {
	var resp openAIResponseBody
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai response has no choices")
	}

	choice := resp.Choices[0]
	out := &NormalizedResponse{
		Content:      choice.Message.Content,
		Reasoning:    choice.Message.ReasoningContent,
		FinishReason: normalizeOpenAIFinish(choice.FinishReason, len(choice.Message.ToolCalls) > 0),
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
	for _, tc := range choice.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out, nil
}

// normalizeOpenAIFinish passes finish_reason through, normalizing only the
// legacy "function_call" spelling and filling gaps left by lax providers.
func normalizeOpenAIFinish(reason string, hasToolCalls bool) string {
	switch reason {
	case "function_call":
		return FinishToolCalls
	case "":
		if hasToolCalls {
			return FinishToolCalls
		}
		return FinishStop
	default:
		return reason
	}
}

// openAIStreamChunk mirrors one "data:" SSE chunk.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (t *openAITransport) ParseStream(body io.Reader, onDelta func(text string)) (*NormalizedResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	out := &NormalizedResponse{}
	var content, reasoning strings.Builder
	type partialCall struct {
		id, name string
		args     strings.Builder
	}
	calls := make(map[int]*partialCall)
	finish := ""

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // tolerate malformed keep-alive frames
		}
		if chunk.Usage != nil {
			out.Usage.InputTokens = chunk.Usage.PromptTokens
			out.Usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.Delta.Content != "" {
			content.WriteString(choice.Delta.Content)
			if onDelta != nil {
				onDelta(choice.Delta.Content)
			}
		}
		if choice.Delta.ReasoningContent != "" {
			reasoning.WriteString(choice.Delta.ReasoningContent)
		}
		for _, tc := range choice.Delta.ToolCalls {
			pc, ok := calls[tc.Index]
			if !ok {
				pc = &partialCall{}
				calls[tc.Index] = pc
			}
			if tc.ID != "" {
				pc.id = tc.ID
			}
			if tc.Function.Name != "" {
				pc.name = tc.Function.Name
			}
			pc.args.WriteString(tc.Function.Arguments)
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			finish = *choice.FinishReason
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read openai stream: %w", err)
	}

	indexes := make([]int, 0, len(calls))
	for idx := range calls {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	for _, idx := range indexes {
		pc := calls[idx]
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        pc.id,
			Name:      pc.name,
			Arguments: pc.args.String(),
		})
	}

	out.Content = content.String()
	out.Reasoning = reasoning.String()
	out.FinishReason = normalizeOpenAIFinish(finish, len(out.ToolCalls) > 0)
	return out, nil
}
