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

// anthropicVersion is the pinned Messages API version header value.
const anthropicVersion = "2023-06-01"

// defaultMaxTokens is the max_tokens sent to the Anthropic Messages API,
// which requires the field on every request.
const defaultMaxTokens = 8192

// providerDataAnthropicBlocks is the ProviderData key under which the
// ordered Anthropic content blocks of an assistant turn are preserved for
// verbatim replay (required for signed thinking blocks).
const providerDataAnthropicBlocks = "anthropic_content_blocks"

// anthropicTransport speaks the Anthropic Messages wire protocol
// (POST {baseURL}/messages). Canonical messages are converted to content
// blocks (text/tool_use/tool_result) at this boundary only.
type anthropicTransport struct{}

func (t *anthropicTransport) BuildRequest(ctx context.Context, p *Profile, apiKey, baseURL, model string,
	system string, msgs []Message, tools []ToolDef, stream bool) (*http.Request, error) {

	wireMsgs, extraSystem := anthropicWireMessages(msgs)
	if extraSystem != "" {
		if system != "" {
			system += "\n\n"
		}
		system += extraSystem
	}

	req := map[string]any{
		"model":      model,
		"max_tokens": defaultMaxTokens,
		"messages":   wireMsgs,
		"stream":     stream,
	}
	if system != "" {
		req["system"] = system
	}
	if len(tools) > 0 {
		wireTools := make([]map[string]any, 0, len(tools))
		for _, td := range tools {
			wireTools = append(wireTools, map[string]any{
				"name":         td.Name,
				"description":  td.Description,
				"input_schema": td.InputSchema,
			})
		}
		req["tools"] = wireTools
	}
	prepareRequestHook(p, req, model)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	if p != nil {
		for k, v := range p.DefaultHeaders {
			httpReq.Header.Set(k, v)
		}
	}
	if apiKey != "" {
		// Auth quirk: OAuth tokens (sk-ant-oat01-...) use Authorization: Bearer,
		// API keys use x-api-key. A profile hook may override this.
		var h, v string
		if p != nil && p.AuthHeader != nil {
			h, v = p.AuthHeader(apiKey)
		} else {
			h, v = anthropicAuthHeader(apiKey)
		}
		httpReq.Header.Set(h, v)
	}
	return httpReq, nil
}

// anthropicWireMessages converts canonical messages into Anthropic wire
// messages. Consecutive same-role turns are merged (the API requires
// alternating roles); role:"tool" results become tool_result blocks in a
// user turn; system messages found mid-conversation are folded into the
// returned system string.
func anthropicWireMessages(msgs []Message) ([]map[string]any, string) {
	type turn struct {
		role   string
		blocks []any
	}
	var turns []turn
	var systemParts []string

	appendBlocks := func(role string, blocks ...any) {
		if len(blocks) == 0 {
			return
		}
		if n := len(turns); n > 0 && turns[n-1].role == role {
			turns[n-1].blocks = append(turns[n-1].blocks, blocks...)
			return
		}
		turns = append(turns, turn{role: role, blocks: blocks})
	}

	for _, m := range msgs {
		switch m.Role {
		case "system":
			if m.Content != "" {
				systemParts = append(systemParts, m.Content)
			}
		case "tool":
			appendBlocks("user", map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     m.Content,
			})
		case "assistant":
			// Replay preserved content blocks verbatim when present (keeps
			// signed thinking blocks and exact ordering).
			if raw, ok := m.ProviderData[providerDataAnthropicBlocks]; ok {
				var blocks []any
				if err := json.Unmarshal(raw, &blocks); err == nil && len(blocks) > 0 {
					appendBlocks("assistant", blocks...)
					continue
				}
			}
			var blocks []any
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Arguments)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			appendBlocks("assistant", blocks...)
		default: // user
			if m.Content == "" {
				continue
			}
			appendBlocks("user", map[string]any{"type": "text", "text": m.Content})
		}
	}

	wire := make([]map[string]any, 0, len(turns))
	for _, tn := range turns {
		wire = append(wire, map[string]any{"role": tn.role, "content": tn.blocks})
	}
	return wire, strings.Join(systemParts, "\n\n")
}

// anthropicBlock is the decoded form of one content block.
type anthropicBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
}

// mapAnthropicStop maps Anthropic stop_reason values onto canonical finish reasons.
func mapAnthropicStop(stopReason string, hasToolCalls bool) string {
	switch stopReason {
	case "end_turn", "stop_sequence":
		return FinishStop
	case "tool_use":
		return FinishToolCalls
	case "max_tokens":
		return FinishLength
	case "refusal":
		return FinishContentFilter
	case "":
		if hasToolCalls {
			return FinishToolCalls
		}
		return FinishStop
	default:
		return FinishStop
	}
}

func (t *anthropicTransport) ParseResponse(body io.Reader) (*NormalizedResponse, error) {
	var resp struct {
		Content    []json.RawMessage `json:"content"`
		StopReason string            `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}

	out := &NormalizedResponse{
		Usage: Usage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens},
	}
	var content, reasoning strings.Builder
	for _, raw := range resp.Content {
		var block anthropicBlock
		if err := json.Unmarshal(raw, &block); err != nil {
			continue
		}
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "thinking":
			reasoning.WriteString(block.Thinking)
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}
	out.Content = content.String()
	out.Reasoning = reasoning.String()
	out.FinishReason = mapAnthropicStop(resp.StopReason, len(out.ToolCalls) > 0)

	if len(resp.Content) > 0 {
		if rawBlocks, err := json.Marshal(resp.Content); err == nil {
			out.ProviderData = map[string]json.RawMessage{
				providerDataAnthropicBlocks: rawBlocks,
			}
		}
	}
	return out, nil
}

// parseAnthropicSSE reads Anthropic server-sent events
// ("event: <type>\ndata: <json>\n\n") and invokes onEvent per data frame.
// Ported from the daemon's ParseSSEStream.
func parseAnthropicSSE(reader io.Reader, onEvent func(eventType string, data []byte)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()

		if len(line) == 0 {
			currentEvent = ""
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			currentEvent = line[7:]
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			data := []byte(line[6:])
			// Use the event type from the "event:" line if available,
			// otherwise fall back to the "type" field in the JSON.
			eventType := currentEvent
			if eventType == "" {
				var parsed struct {
					Type string `json:"type"`
				}
				if err := json.Unmarshal(data, &parsed); err == nil {
					eventType = parsed.Type
				}
			}
			if eventType != "" {
				onEvent(eventType, data)
			}
		}
	}
	return scanner.Err()
}

// streamedBlock accumulates one content block across streaming events.
type streamedBlock struct {
	blockType string
	id, name  string
	text      strings.Builder
	thinking  strings.Builder
	signature strings.Builder
	inputJSON strings.Builder
}

func (t *anthropicTransport) ParseStream(body io.Reader, onDelta func(text string)) (*NormalizedResponse, error) {
	blocks := make(map[int]*streamedBlock)
	out := &NormalizedResponse{}
	stopReason := ""

	getBlock := func(index int) *streamedBlock {
		b, ok := blocks[index]
		if !ok {
			b = &streamedBlock{}
			blocks[index] = b
		}
		return b
	}

	err := parseAnthropicSSE(body, func(eventType string, data []byte) {
		switch eventType {
		case "message_start":
			var ev struct {
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if json.Unmarshal(data, &ev) == nil {
				out.Usage.InputTokens = ev.Message.Usage.InputTokens
			}

		case "content_block_start":
			var ev struct {
				Index        int            `json:"index"`
				ContentBlock anthropicBlock `json:"content_block"`
			}
			if json.Unmarshal(data, &ev) != nil {
				return
			}
			b := getBlock(ev.Index)
			b.blockType = ev.ContentBlock.Type
			b.id = ev.ContentBlock.ID
			b.name = ev.ContentBlock.Name
			b.text.WriteString(ev.ContentBlock.Text)
			b.thinking.WriteString(ev.ContentBlock.Thinking)
			if len(ev.ContentBlock.Input) > 0 && string(ev.ContentBlock.Input) != "{}" {
				b.inputJSON.WriteString(string(ev.ContentBlock.Input))
			}

		case "content_block_delta":
			var ev struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
					Thinking    string `json:"thinking"`
					Signature   string `json:"signature"`
				} `json:"delta"`
			}
			if json.Unmarshal(data, &ev) != nil {
				return
			}
			b := getBlock(ev.Index)
			switch ev.Delta.Type {
			case "text_delta":
				b.text.WriteString(ev.Delta.Text)
				if onDelta != nil && ev.Delta.Text != "" {
					onDelta(ev.Delta.Text)
				}
			case "input_json_delta":
				b.inputJSON.WriteString(ev.Delta.PartialJSON)
			case "thinking_delta":
				b.thinking.WriteString(ev.Delta.Thinking)
			case "signature_delta":
				b.signature.WriteString(ev.Delta.Signature)
			}

		case "message_delta":
			var ev struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(data, &ev) != nil {
				return
			}
			if ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
			if ev.Usage.OutputTokens > 0 {
				out.Usage.OutputTokens = ev.Usage.OutputTokens
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("read anthropic stream: %w", err)
	}

	indexes := make([]int, 0, len(blocks))
	for idx := range blocks {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	var content, reasoning strings.Builder
	var rawBlocks []json.RawMessage
	for _, idx := range indexes {
		b := blocks[idx]
		switch b.blockType {
		case "text":
			content.WriteString(b.text.String())
			raw, _ := json.Marshal(map[string]any{"type": "text", "text": b.text.String()})
			rawBlocks = append(rawBlocks, raw)
		case "thinking":
			reasoning.WriteString(b.thinking.String())
			blockMap := map[string]any{"type": "thinking", "thinking": b.thinking.String()}
			if sig := b.signature.String(); sig != "" {
				blockMap["signature"] = sig
			}
			raw, _ := json.Marshal(blockMap)
			rawBlocks = append(rawBlocks, raw)
		case "tool_use":
			args := b.inputJSON.String()
			if args == "" {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: b.id, Name: b.name, Arguments: args})
			raw, _ := json.Marshal(map[string]any{
				"type":  "tool_use",
				"id":    b.id,
				"name":  b.name,
				"input": json.RawMessage(args),
			})
			rawBlocks = append(rawBlocks, raw)
		}
	}

	out.Content = content.String()
	out.Reasoning = reasoning.String()
	out.FinishReason = mapAnthropicStop(stopReason, len(out.ToolCalls) > 0)
	if len(rawBlocks) > 0 {
		if raw, err := json.Marshal(rawBlocks); err == nil {
			out.ProviderData = map[string]json.RawMessage{providerDataAnthropicBlocks: raw}
		}
	}
	return out, nil
}
