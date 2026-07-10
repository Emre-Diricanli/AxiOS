package providers

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func mustGet(t *testing.T, name string) *Profile {
	t.Helper()
	p, ok := Get(name)
	if !ok {
		t.Fatalf("profile %q not registered", name)
	}
	return p
}

func decodeBody(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return m
}

func TestOpenAIBuildRequest(t *testing.T) {
	tr := GetTransport(APIModeChatCompletions)
	msgs := []Message{
		{Role: "user", Content: "list files"},
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "axios-fs__list_directory", Arguments: `{"path":"/tmp"}`},
		}},
		{Role: "tool", ToolCallID: "call_1", Content: "file1.txt"},
		{Role: "assistant", Content: "There is one file."},
	}
	tools := []ToolDef{
		{Name: "axios-fs__list_directory", Description: "List a directory",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}},
	}

	req, err := tr.BuildRequest(context.Background(), mustGet(t, "openai"), "sk-test",
		"https://api.openai.com/v1", "gpt-4o", "You are AxiOS.", msgs, tools, false)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	if got := req.URL.String(); got != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("URL = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}

	body := decodeBody(t, req.Body)
	if body["model"] != "gpt-4o" {
		t.Errorf("model = %v", body["model"])
	}
	if body["stream"] != false {
		t.Errorf("stream = %v", body["stream"])
	}

	wireMsgs := body["messages"].([]any)
	if len(wireMsgs) != 5 { // system + 4
		t.Fatalf("len(messages) = %d, want 5", len(wireMsgs))
	}
	first := wireMsgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "You are AxiOS." {
		t.Errorf("system message = %v", first)
	}
	asst := wireMsgs[2].(map[string]any)
	calls := asst["tool_calls"].([]any)
	call := calls[0].(map[string]any)
	if call["id"] != "call_1" || call["type"] != "function" {
		t.Errorf("tool call = %v", call)
	}
	fn := call["function"].(map[string]any)
	if fn["name"] != "axios-fs__list_directory" || fn["arguments"] != `{"path":"/tmp"}` {
		t.Errorf("function = %v", fn)
	}
	toolMsg := wireMsgs[3].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call_1" || toolMsg["content"] != "file1.txt" {
		t.Errorf("tool result message = %v", toolMsg)
	}

	wireTools := body["tools"].([]any)
	toolDef := wireTools[0].(map[string]any)
	if toolDef["type"] != "function" {
		t.Errorf("tool type = %v", toolDef["type"])
	}
	fnDef := toolDef["function"].(map[string]any)
	if fnDef["name"] != "axios-fs__list_directory" || fnDef["description"] != "List a directory" {
		t.Errorf("tool function = %v", fnDef)
	}
	if _, ok := fnDef["parameters"].(map[string]any); !ok {
		t.Errorf("parameters missing: %v", fnDef)
	}
}

func TestOpenAIParseResponse(t *testing.T) {
	tr := GetTransport(APIModeChatCompletions)

	tests := []struct {
		name string
		body string
		want NormalizedResponse
	}{
		{
			name: "text with stop",
			body: `{"choices":[{"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":10,"completion_tokens":3}}`,
			want: NormalizedResponse{Content: "Hello!", FinishReason: "stop",
				Usage: Usage{InputTokens: 10, OutputTokens: 3}},
		},
		{
			name: "tool calls",
			body: `{"choices":[{"message":{"role":"assistant","content":"",
				"tool_calls":[{"id":"call_9","type":"function","function":{"name":"get_time","arguments":"{\"tz\":\"UTC\"}"}}]},
				"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":7}}`,
			want: NormalizedResponse{FinishReason: "tool_calls",
				ToolCalls: []ToolCall{{ID: "call_9", Name: "get_time", Arguments: `{"tz":"UTC"}`}},
				Usage:     Usage{InputTokens: 5, OutputTokens: 7}},
		},
		{
			name: "legacy function_call finish reason",
			body: `{"choices":[{"message":{"content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{}"}}]},"finish_reason":"function_call"}]}`,
			want: NormalizedResponse{FinishReason: "tool_calls",
				ToolCalls: []ToolCall{{ID: "c1", Name: "f", Arguments: "{}"}}},
		},
		{
			name: "length passthrough",
			body: `{"choices":[{"message":{"content":"trunca"},"finish_reason":"length"}]}`,
			want: NormalizedResponse{Content: "trunca", FinishReason: "length"},
		},
		{
			name: "content_filter passthrough",
			body: `{"choices":[{"message":{"content":""},"finish_reason":"content_filter"}]}`,
			want: NormalizedResponse{FinishReason: "content_filter"},
		},
		{
			name: "reasoning content",
			body: `{"choices":[{"message":{"content":"42","reasoning_content":"thinking hard"},"finish_reason":"stop"}]}`,
			want: NormalizedResponse{Content: "42", Reasoning: "thinking hard", FinishReason: "stop"},
		},
		{
			name: "missing finish reason with tool calls",
			body: `{"choices":[{"message":{"tool_calls":[{"id":"c2","type":"function","function":{"name":"g","arguments":"{}"}}]}}]}`,
			want: NormalizedResponse{FinishReason: "tool_calls",
				ToolCalls: []ToolCall{{ID: "c2", Name: "g", Arguments: "{}"}}},
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

func TestOpenAIParseResponseNoChoices(t *testing.T) {
	tr := GetTransport(APIModeChatCompletions)
	if _, err := tr.ParseResponse(strings.NewReader(`{"choices":[]}`)); err == nil {
		t.Fatal("want error for empty choices")
	}
}

func TestOpenAIParseStream(t *testing.T) {
	tr := GetTransport(APIModeChatCompletions)

	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo!"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_9","function":{"name":"get_time","arguments":"{\"tz\":"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"UTC\"}"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var deltas []string
	got, err := tr.ParseStream(strings.NewReader(stream), func(text string) { deltas = append(deltas, text) })
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}

	want := NormalizedResponse{
		Content:      "Hello!",
		FinishReason: "tool_calls",
		ToolCalls:    []ToolCall{{ID: "call_9", Name: "get_time", Arguments: `{"tz":"UTC"}`}},
		Usage:        Usage{InputTokens: 10, OutputTokens: 3},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("got %+v, want %+v", *got, want)
	}
	if joined := strings.Join(deltas, ""); joined != "Hello!" {
		t.Errorf("deltas = %q, want %q", joined, "Hello!")
	}
}

// TestOpenAIStreamMatchesNonStreaming verifies that streaming accumulation
// yields the same NormalizedResponse as the non-streaming parse.
func TestOpenAIStreamMatchesNonStreaming(t *testing.T) {
	tr := GetTransport(APIModeChatCompletions)

	nonStreaming := `{"choices":[{"message":{"content":"The answer is 4."},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":8,"completion_tokens":6}}`
	streaming := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"The answer"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" is 4."}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":6}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	fromResp, err := tr.ParseResponse(strings.NewReader(nonStreaming))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	fromStream, err := tr.ParseStream(strings.NewReader(streaming), nil)
	if err != nil {
		t.Fatalf("ParseStream: %v", err)
	}
	if !reflect.DeepEqual(fromResp, fromStream) {
		t.Errorf("stream %+v != non-stream %+v", *fromStream, *fromResp)
	}
}
