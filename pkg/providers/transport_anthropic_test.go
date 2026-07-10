package providers

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAnthropicAuthHeader(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		wantHeader string
		wantValue  string
	}{
		{"api key uses x-api-key", "sk-ant-api03-abc", "x-api-key", "sk-ant-api03-abc"},
		{"oauth token uses bearer", "sk-ant-oat01-xyz", "Authorization", "Bearer sk-ant-oat01-xyz"},
		{"unknown shape uses x-api-key", "some-key", "x-api-key", "some-key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, v := anthropicAuthHeader(tt.key)
			if h != tt.wantHeader || v != tt.wantValue {
				t.Errorf("got (%q, %q), want (%q, %q)", h, v, tt.wantHeader, tt.wantValue)
			}
		})
	}
}

func TestAnthropicBuildRequest(t *testing.T) {
	tr := GetTransport(APIModeAnthropicMessages)
	msgs := []Message{
		{Role: "user", Content: "check disk"},
		{Role: "assistant", Content: "Checking.", ToolCalls: []ToolCall{
			{ID: "toolu_1", Name: "axios-system__disk_usage", Arguments: `{"path":"/"}`},
			{ID: "toolu_2", Name: "axios-system__system_info", Arguments: ""},
		}},
		{Role: "tool", ToolCallID: "toolu_1", Content: "42% used"},
		{Role: "tool", ToolCallID: "toolu_2", Content: "linux amd64"},
	}
	tools := []ToolDef{
		{Name: "axios-system__disk_usage", Description: "Disk usage",
			InputSchema: map[string]any{"type": "object"}},
	}

	req, err := tr.BuildRequest(context.Background(), mustGet(t, "anthropic"), "sk-ant-api03-abc",
		"https://api.anthropic.com/v1", "claude-sonnet-4-6", "You are AxiOS.", msgs, tools, true)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	if got := req.URL.String(); got != "https://api.anthropic.com/v1/messages" {
		t.Errorf("URL = %q", got)
	}
	if got := req.Header.Get("x-api-key"); got != "sk-ant-api03-abc" {
		t.Errorf("x-api-key = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be unset for API keys, got %q", got)
	}
	if got := req.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q", got)
	}

	body := decodeBody(t, req.Body)
	if body["model"] != "claude-sonnet-4-6" || body["system"] != "You are AxiOS." {
		t.Errorf("model/system = %v / %v", body["model"], body["system"])
	}
	if body["max_tokens"] != float64(defaultMaxTokens) {
		t.Errorf("max_tokens = %v, want %d", body["max_tokens"], defaultMaxTokens)
	}
	if body["stream"] != true {
		t.Errorf("stream = %v", body["stream"])
	}

	wireMsgs := body["messages"].([]any)
	if len(wireMsgs) != 3 { // user, assistant, merged tool-result user turn
		t.Fatalf("len(messages) = %d, want 3: %v", len(wireMsgs), wireMsgs)
	}

	asst := wireMsgs[1].(map[string]any)
	blocks := asst["content"].([]any)
	if len(blocks) != 3 {
		t.Fatalf("assistant blocks = %d, want 3", len(blocks))
	}
	if b := blocks[0].(map[string]any); b["type"] != "text" || b["text"] != "Checking." {
		t.Errorf("text block = %v", b)
	}
	toolUse := blocks[1].(map[string]any)
	if toolUse["type"] != "tool_use" || toolUse["id"] != "toolu_1" || toolUse["name"] != "axios-system__disk_usage" {
		t.Errorf("tool_use block = %v", toolUse)
	}
	if input := toolUse["input"].(map[string]any); input["path"] != "/" {
		t.Errorf("tool_use input = %v", input)
	}
	// Empty arguments become an empty object.
	if input := blocks[2].(map[string]any)["input"].(map[string]any); len(input) != 0 {
		t.Errorf("empty-arg input = %v, want {}", input)
	}

	// Consecutive role:"tool" messages merge into one user turn with two tool_result blocks.
	toolTurn := wireMsgs[2].(map[string]any)
	if toolTurn["role"] != "user" {
		t.Errorf("tool result role = %v", toolTurn["role"])
	}
	resultBlocks := toolTurn["content"].([]any)
	if len(resultBlocks) != 2 {
		t.Fatalf("tool_result blocks = %d, want 2", len(resultBlocks))
	}
	r0 := resultBlocks[0].(map[string]any)
	if r0["type"] != "tool_result" || r0["tool_use_id"] != "toolu_1" || r0["content"] != "42% used" {
		t.Errorf("tool_result[0] = %v", r0)
	}

	wireTools := body["tools"].([]any)
	td := wireTools[0].(map[string]any)
	if td["name"] != "axios-system__disk_usage" || td["description"] != "Disk usage" {
		t.Errorf("tool def = %v", td)
	}
	if _, ok := td["input_schema"].(map[string]any); !ok {
		t.Errorf("input_schema missing: %v", td)
	}
}

func TestAnthropicBuildRequestOAuthBearer(t *testing.T) {
	tr := GetTransport(APIModeAnthropicMessages)
	req, err := tr.BuildRequest(context.Background(), mustGet(t, "anthropic"), "sk-ant-oat01-tok",
		"https://api.anthropic.com/v1", "claude-sonnet-4-6", "", []Message{{Role: "user", Content: "hi"}}, nil, false)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-ant-oat01-tok" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key should be unset for OAuth tokens, got %q", got)
	}
}

func TestAnthropicBuildRequestReplaysProviderData(t *testing.T) {
	tr := GetTransport(APIModeAnthropicMessages)
	preserved := json.RawMessage(`[{"type":"thinking","thinking":"hmm","signature":"sig1"},{"type":"text","text":"Done."}]`)
	msgs := []Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Content: "Done.",
			ProviderData: map[string]json.RawMessage{providerDataAnthropicBlocks: preserved}},
	}
	req, err := tr.BuildRequest(context.Background(), mustGet(t, "anthropic"), "k",
		"https://api.anthropic.com/v1", "claude-sonnet-4-6", "", msgs, nil, false)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	body := decodeBody(t, req.Body)
	asst := body["messages"].([]any)[1].(map[string]any)
	blocks := asst["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (replayed verbatim)", len(blocks))
	}
	thinking := blocks[0].(map[string]any)
	if thinking["type"] != "thinking" || thinking["signature"] != "sig1" {
		t.Errorf("replayed thinking block = %v", thinking)
	}
}

func TestAnthropicParseResponse(t *testing.T) {
	tr := GetTransport(APIModeAnthropicMessages)

	tests := []struct {
		name string
		body string
		want NormalizedResponse
	}{
		{
			name: "end_turn maps to stop",
			body: `{"content":[{"type":"text","text":"Hi there."}],"stop_reason":"end_turn",
				"usage":{"input_tokens":12,"output_tokens":4}}`,
			want: NormalizedResponse{Content: "Hi there.", FinishReason: "stop",
				Usage: Usage{InputTokens: 12, OutputTokens: 4}},
		},
		{
			name: "tool_use maps to tool_calls",
			body: `{"content":[{"type":"text","text":"Checking."},
				{"type":"tool_use","id":"toolu_1","name":"disk_usage","input":{"path":"/"}}],
				"stop_reason":"tool_use","usage":{"input_tokens":20,"output_tokens":15}}`,
			want: NormalizedResponse{Content: "Checking.", FinishReason: "tool_calls",
				ToolCalls: []ToolCall{{ID: "toolu_1", Name: "disk_usage", Arguments: `{"path":"/"}`}},
				Usage:     Usage{InputTokens: 20, OutputTokens: 15}},
		},
		{
			name: "max_tokens maps to length",
			body: `{"content":[{"type":"text","text":"trunc"}],"stop_reason":"max_tokens"}`,
			want: NormalizedResponse{Content: "trunc", FinishReason: "length"},
		},
		{
			name: "refusal maps to content_filter",
			body: `{"content":[],"stop_reason":"refusal"}`,
			want: NormalizedResponse{FinishReason: "content_filter"},
		},
		{
			name: "stop_sequence maps to stop",
			body: `{"content":[{"type":"text","text":"x"}],"stop_reason":"stop_sequence"}`,
			want: NormalizedResponse{Content: "x", FinishReason: "stop"},
		},
		{
			name: "thinking becomes reasoning",
			body: `{"content":[{"type":"thinking","thinking":"let me see","signature":"s"},
				{"type":"text","text":"Answer."}],"stop_reason":"end_turn"}`,
			want: NormalizedResponse{Content: "Answer.", Reasoning: "let me see", FinishReason: "stop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tr.ParseResponse(strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			pd := got.ProviderData
			got.ProviderData = nil
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("got %+v, want %+v", *got, tt.want)
			}
			// Ordered content blocks must be preserved for replay whenever
			// the response had content.
			if strings.Contains(tt.body, `"type"`) {
				if _, ok := pd[providerDataAnthropicBlocks]; !ok {
					t.Errorf("ProviderData[%q] missing", providerDataAnthropicBlocks)
				}
			}
		})
	}
}

func TestAnthropicParseResponsePreservesBlockOrder(t *testing.T) {
	tr := GetTransport(APIModeAnthropicMessages)
	body := `{"content":[{"type":"thinking","thinking":"t","signature":"sig"},
		{"type":"text","text":"a"},
		{"type":"tool_use","id":"toolu_1","name":"f","input":{}}],"stop_reason":"tool_use"}`
	got, err := tr.ParseResponse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	var blocks []map[string]any
	if err := json.Unmarshal(got.ProviderData[providerDataAnthropicBlocks], &blocks); err != nil {
		t.Fatalf("unmarshal preserved blocks: %v", err)
	}
	wantTypes := []string{"thinking", "text", "tool_use"}
	if len(blocks) != len(wantTypes) {
		t.Fatalf("preserved %d blocks, want %d", len(blocks), len(wantTypes))
	}
	for i, wt := range wantTypes {
		if blocks[i]["type"] != wt {
			t.Errorf("block[%d].type = %v, want %s", i, blocks[i]["type"], wt)
		}
	}
	if blocks[0]["signature"] != "sig" {
		t.Errorf("signature not preserved: %v", blocks[0])
	}
}

// anthropicSSE builds an SSE frame in Anthropic's "event:\ndata:\n\n" framing.
func anthropicSSE(event, data string) string {
	return "event: " + event + "\ndata: " + data + "\n\n"
}

func TestAnthropicParseStream(t *testing.T) {
	tr := GetTransport(APIModeAnthropicMessages)

	stream := anthropicSSE("message_start", `{"type":"message_start","message":{"usage":{"input_tokens":20}}}`) +
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Check"}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ing."}}`) +
		anthropicSSE("content_block_stop", `{"type":"content_block_stop","index":0}`) +
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"disk_usage","input":{}}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"/\"}"}}`) +
		anthropicSSE("content_block_stop", `{"type":"content_block_stop","index":1}`) +
		anthropicSSE("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":15}}`) +
		anthropicSSE("message_stop", `{"type":"message_stop"}`)

	var deltas []string
	got, err := tr.ParseStream(strings.NewReader(stream), func(text string) { deltas = append(deltas, text) })
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}

	if joined := strings.Join(deltas, ""); joined != "Checking." {
		t.Errorf("deltas = %q", joined)
	}

	// Streaming accumulation must equal the non-streaming shape.
	nonStreaming := `{"content":[{"type":"text","text":"Checking."},
		{"type":"tool_use","id":"toolu_1","name":"disk_usage","input":{"path":"/"}}],
		"stop_reason":"tool_use","usage":{"input_tokens":20,"output_tokens":15}}`
	want, err := tr.ParseResponse(strings.NewReader(nonStreaming))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	gotPD, wantPD := got.ProviderData, want.ProviderData
	got.ProviderData, want.ProviderData = nil, nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stream %+v != non-stream %+v", *got, *want)
	}

	// ProviderData blocks must be semantically identical (JSON-equal).
	var gotBlocks, wantBlocks any
	if err := json.Unmarshal(gotPD[providerDataAnthropicBlocks], &gotBlocks); err != nil {
		t.Fatalf("unmarshal stream blocks: %v", err)
	}
	if err := json.Unmarshal(wantPD[providerDataAnthropicBlocks], &wantBlocks); err != nil {
		t.Fatalf("unmarshal non-stream blocks: %v", err)
	}
	if !reflect.DeepEqual(gotBlocks, wantBlocks) {
		t.Errorf("preserved blocks differ:\nstream: %v\nnon-stream: %v", gotBlocks, wantBlocks)
	}
}

func TestAnthropicParseStreamThinking(t *testing.T) {
	tr := GetTransport(APIModeAnthropicMessages)
	stream := anthropicSSE("message_start", `{"type":"message_start","message":{"usage":{"input_tokens":5}}}`) +
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm "}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"ok"}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig1"}}`) +
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`) +
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Done"}}`) +
		anthropicSSE("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`) +
		anthropicSSE("message_stop", `{"type":"message_stop"}`)

	got, err := tr.ParseStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	if got.Reasoning != "hmm ok" || got.Content != "Done" || got.FinishReason != "stop" {
		t.Errorf("got %+v", *got)
	}
	var blocks []map[string]any
	if err := json.Unmarshal(got.ProviderData[providerDataAnthropicBlocks], &blocks); err != nil {
		t.Fatalf("unmarshal blocks: %v", err)
	}
	if blocks[0]["type"] != "thinking" || blocks[0]["signature"] != "sig1" {
		t.Errorf("signed thinking block not preserved: %v", blocks[0])
	}
}
