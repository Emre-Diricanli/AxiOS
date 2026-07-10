package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ollamaTransport speaks Ollama's native chat protocol
// (POST {baseURL}/api/chat, NDJSON when streaming). Conversion semantics are
// ported from the daemon's ConvertMessagesForOllama/ConvertToolsForOllama:
// system is prepended as its own message and tool results are flattened into
// a plain-text user turn (local models handle this far more reliably than
// structured tool-result messages).
type ollamaTransport struct{}

// ollamaWireToolCall is a tool call in Ollama's wire format; arguments are a
// decoded JSON object rather than a string.
type ollamaWireToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

func (t *ollamaTransport) BuildRequest(ctx context.Context, p *Profile, apiKey, baseURL, model string,
	system string, msgs []Message, tools []ToolDef, stream bool) (*http.Request, error) {

	wireMsgs := make([]map[string]any, 0, len(msgs)+1)
	if system != "" {
		wireMsgs = append(wireMsgs, map[string]any{"role": "system", "content": system})
	}

	var pendingToolResults []string
	flushToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		combined := ""
		for _, r := range pendingToolResults {
			combined += r + "\n"
		}
		wireMsgs = append(wireMsgs, map[string]any{
			"role":    "user",
			"content": "Tool results:\n" + combined,
		})
		pendingToolResults = nil
	}

	for _, m := range msgs {
		if m.Role == "tool" {
			pendingToolResults = append(pendingToolResults, m.Content)
			continue
		}
		flushToolResults()

		wm := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				args := map[string]any{}
				if tc.Arguments != "" {
					// Best-effort decode; Ollama wants a JSON object, not a string.
					_ = json.Unmarshal([]byte(tc.Arguments), &args)
				}
				calls = append(calls, map[string]any{
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": args,
					},
				})
			}
			wm["tool_calls"] = calls
		}
		wireMsgs = append(wireMsgs, wm)
	}
	flushToolResults()

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

	url := strings.TrimRight(baseURL, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	applyProfileHeaders(httpReq, p, apiKey)
	return httpReq, nil
}

// ollamaResponseBody mirrors one /api/chat JSON object (the whole response
// when non-streaming, one NDJSON line when streaming).
type ollamaResponseBody struct {
	Model   string `json:"model"`
	Message struct {
		Role      string               `json:"role"`
		Content   string               `json:"content"`
		Thinking  string               `json:"thinking"`
		ToolCalls []ollamaWireToolCall `json:"tool_calls"`
	} `json:"message"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// mapOllamaFinish maps Ollama's done_reason onto canonical finish reasons.
func mapOllamaFinish(doneReason string, hasToolCalls bool) string {
	if hasToolCalls {
		return FinishToolCalls
	}
	switch doneReason {
	case "length":
		return FinishLength
	default:
		return FinishStop
	}
}

// ollamaToolCalls converts wire tool calls to canonical ones. Ollama does not
// assign call IDs, so deterministic synthetic IDs are generated.
func ollamaToolCalls(calls []ollamaWireToolCall, startIndex int) []ToolCall {
	var out []ToolCall
	for i, tc := range calls {
		args := "{}"
		if tc.Function.Arguments != nil {
			if raw, err := json.Marshal(tc.Function.Arguments); err == nil {
				args = string(raw)
			}
		}
		out = append(out, ToolCall{
			ID:        fmt.Sprintf("ollama-call-%d", startIndex+i),
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	return out
}

func (t *ollamaTransport) ParseResponse(body io.Reader) (*NormalizedResponse, error) {
	var resp ollamaResponseBody
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	out := &NormalizedResponse{
		Content:   resp.Message.Content,
		Reasoning: resp.Message.Thinking,
		ToolCalls: ollamaToolCalls(resp.Message.ToolCalls, 0),
		Usage: Usage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
		},
	}
	out.FinishReason = mapOllamaFinish(resp.DoneReason, len(out.ToolCalls) > 0)
	return out, nil
}

func (t *ollamaTransport) ParseStream(body io.Reader, onDelta func(text string)) (*NormalizedResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	out := &NormalizedResponse{}
	var content, reasoning strings.Builder
	doneReason := ""

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk ollamaResponseBody
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue // tolerate malformed frames, matching ParseOllamaStream
		}

		if chunk.Message.Content != "" {
			content.WriteString(chunk.Message.Content)
			if onDelta != nil {
				onDelta(chunk.Message.Content)
			}
		}
		if chunk.Message.Thinking != "" {
			reasoning.WriteString(chunk.Message.Thinking)
		}
		if len(chunk.Message.ToolCalls) > 0 {
			out.ToolCalls = append(out.ToolCalls, ollamaToolCalls(chunk.Message.ToolCalls, len(out.ToolCalls))...)
		}
		if chunk.Done {
			doneReason = chunk.DoneReason
			out.Usage.InputTokens = chunk.PromptEvalCount
			out.Usage.OutputTokens = chunk.EvalCount
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ollama stream: %w", err)
	}

	out.Content = content.String()
	out.Reasoning = reasoning.String()
	out.FinishReason = mapOllamaFinish(doneReason, len(out.ToolCalls) > 0)
	return out, nil
}
