package providers

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestOllamaBuildRequest(t *testing.T) {
	tr := GetTransport(APIModeOllama)
	msgs := []Message{
		{Role: "user", Content: "how much disk space?"},
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "axios-system__disk_usage", Arguments: `{"path":"/"}`},
		}},
		{Role: "tool", ToolCallID: "call_1", Content: "42% used"},
		{Role: "tool", ToolCallID: "call_2", Content: "8 GB free"},
	}
	tools := []ToolDef{
		{Name: "axios-system__disk_usage", Description: "Disk usage",
			InputSchema: map[string]any{"type": "object"}},
	}

	req, err := tr.BuildRequest(context.Background(), mustGet(t, "ollama"), "",
		"http://127.0.0.1:11434", "llama3.1:8b", "You are AxiOS.", msgs, tools, false)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	if got := req.URL.String(); got != "http://127.0.0.1:11434/api/chat" {
		t.Errorf("URL = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization should be unset without an API key, got %q", got)
	}

	body := decodeBody(t, req.Body)
	if body["model"] != "llama3.1:8b" || body["stream"] != false {
		t.Errorf("model/stream = %v / %v", body["model"], body["stream"])
	}

	wireMsgs := body["messages"].([]any)
	if len(wireMsgs) != 4 { // system + user + assistant + flattened tool results
		t.Fatalf("len(messages) = %d, want 4: %v", len(wireMsgs), wireMsgs)
	}
	if sys := wireMsgs[0].(map[string]any); sys["role"] != "system" || sys["content"] != "You are AxiOS." {
		t.Errorf("system message = %v", sys)
	}

	// Assistant tool_calls carry decoded argument objects.
	asst := wireMsgs[2].(map[string]any)
	calls := asst["tool_calls"].([]any)
	fn := calls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "axios-system__disk_usage" {
		t.Errorf("function name = %v", fn["name"])
	}
	if args := fn["arguments"].(map[string]any); args["path"] != "/" {
		t.Errorf("arguments = %v", args)
	}

	// Consecutive tool results flatten into one plain-text user message.
	flat := wireMsgs[3].(map[string]any)
	if flat["role"] != "user" {
		t.Errorf("flattened role = %v", flat["role"])
	}
	content := flat["content"].(string)
	if !strings.HasPrefix(content, "Tool results:\n") ||
		!strings.Contains(content, "42% used") || !strings.Contains(content, "8 GB free") {
		t.Errorf("flattened content = %q", content)
	}

	wireTools := body["tools"].([]any)
	td := wireTools[0].(map[string]any)
	if td["type"] != "function" {
		t.Errorf("tool type = %v", td["type"])
	}
	tf := td["function"].(map[string]any)
	if tf["name"] != "axios-system__disk_usage" || tf["description"] != "Disk usage" {
		t.Errorf("tool function = %v", tf)
	}
}

func TestOllamaParseResponse(t *testing.T) {
	tr := GetTransport(APIModeOllama)

	tests := []struct {
		name string
		body string
		want NormalizedResponse
	}{
		{
			name: "plain text",
			body: `{"model":"llama3.1:8b","message":{"role":"assistant","content":"Disk is 42% full."},
				"done":true,"done_reason":"stop","prompt_eval_count":30,"eval_count":9}`,
			want: NormalizedResponse{Content: "Disk is 42% full.", FinishReason: "stop",
				Usage: Usage{InputTokens: 30, OutputTokens: 9}},
		},
		{
			name: "tool calls get synthetic ids and JSON-string arguments",
			body: `{"model":"llama3.1:8b","message":{"role":"assistant","content":"",
				"tool_calls":[{"function":{"name":"disk_usage","arguments":{"path":"/"}}}]},
				"done":true,"done_reason":"stop"}`,
			want: NormalizedResponse{FinishReason: "tool_calls",
				ToolCalls: []ToolCall{{ID: "ollama-call-0", Name: "disk_usage", Arguments: `{"path":"/"}`}}},
		},
		{
			name: "length done_reason",
			body: `{"message":{"role":"assistant","content":"trunc"},"done":true,"done_reason":"length"}`,
			want: NormalizedResponse{Content: "trunc", FinishReason: "length"},
		},
		{
			name: "thinking becomes reasoning",
			body: `{"message":{"role":"assistant","content":"4","thinking":"2+2"},"done":true,"done_reason":"stop"}`,
			want: NormalizedResponse{Content: "4", Reasoning: "2+2", FinishReason: "stop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tr.ParseResponse(strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestOllamaParseStream(t *testing.T) {
	tr := GetTransport(APIModeOllama)

	stream := strings.Join([]string{
		`{"model":"llama3.1:8b","message":{"role":"assistant","content":"Disk"},"done":false}`,
		`{"model":"llama3.1:8b","message":{"role":"assistant","content":" is 42% full."},"done":false}`,
		`{"model":"llama3.1:8b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":30,"eval_count":9}`,
	}, "\n") + "\n"

	var deltas []string
	got, err := tr.ParseStream(strings.NewReader(stream), func(text string) { deltas = append(deltas, text) })
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}

	if joined := strings.Join(deltas, ""); joined != "Disk is 42% full." {
		t.Errorf("deltas = %q", joined)
	}

	// Streaming accumulation must equal the non-streaming shape.
	nonStreaming := `{"model":"llama3.1:8b","message":{"role":"assistant","content":"Disk is 42% full."},
		"done":true,"done_reason":"stop","prompt_eval_count":30,"eval_count":9}`
	want, err := tr.ParseResponse(strings.NewReader(nonStreaming))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stream %+v != non-stream %+v", *got, *want)
	}
}

func TestOllamaParseStreamToolCalls(t *testing.T) {
	tr := GetTransport(APIModeOllama)
	stream := strings.Join([]string{
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"disk_usage","arguments":{"path":"/"}}}]},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":12,"eval_count":5}`,
	}, "\n") + "\n"

	got, err := tr.ParseStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	want := NormalizedResponse{
		FinishReason: "tool_calls",
		ToolCalls:    []ToolCall{{ID: "ollama-call-0", Name: "disk_usage", Arguments: `{"path":"/"}`}},
		Usage:        Usage{InputTokens: 12, OutputTokens: 5},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}
