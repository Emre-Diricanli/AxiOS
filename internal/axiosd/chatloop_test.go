package axiosd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/axios-os/axios/pkg/providers"
)

// fakeSink collects every message written by the loop.
type fakeSink struct {
	mu       sync.Mutex
	messages []ChatMessage
}

func (f *fakeSink) WriteJSON(v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	msg, ok := v.(ChatMessage)
	if !ok {
		return fmt.Errorf("unexpected sink payload type %T", v)
	}
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeSink) byType(t string) []ChatMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ChatMessage
	for _, m := range f.messages {
		if m.Type == t {
			out = append(out, m)
		}
	}
	return out
}

// fakeStep is one scripted provider turn: either a response or an error.
type fakeStep struct {
	resp   *providers.NormalizedResponse
	err    error
	deltas []string // streamed before returning resp
}

// fakeProvider implements chatClient with a scripted sequence of steps.
// The last step repeats once the script is exhausted.
type fakeProvider struct {
	name  string
	model string
	steps []fakeStep
	calls int
}

func (f *fakeProvider) Name() string  { return f.name }
func (f *fakeProvider) Model() string { return f.model }

func (f *fakeProvider) Stream(ctx context.Context, system string, msgs []providers.Message, tools []providers.ToolDef, onDelta func(string)) (*providers.NormalizedResponse, error) {
	idx := f.calls
	if idx >= len(f.steps) {
		idx = len(f.steps) - 1
	}
	f.calls++
	step := f.steps[idx]
	if step.err != nil {
		return nil, step.err
	}
	for _, d := range step.deltas {
		if onDelta != nil {
			onDelta(d)
		}
	}
	return step.resp, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newTestLoop(client chatClient) *chatLoop {
	return &chatLoop{
		client:      client,
		system:      "test system prompt",
		logger:      testLogger(),
		sleep:       func(time.Duration) {},
		backoffBase: time.Nanosecond,
	}
}

func userSession(t *testing.T, content string) *Session {
	t.Helper()
	s := NewSession("test")
	s.AddMessage(providers.Message{Role: "user", Content: content})
	return s
}

func TestChatLoopMultiIterationToolCalls(t *testing.T) {
	fake := &fakeProvider{
		name:  "openai",
		model: "gpt-4o",
		steps: []fakeStep{
			{
				resp: &providers.NormalizedResponse{
					FinishReason: providers.FinishToolCalls,
					ToolCalls: []providers.ToolCall{
						{ID: "call_1", Name: "axios-system__system_info", Arguments: `{}`},
						{ID: "call_2", Name: "axios-fs__read_file", Arguments: `{"path":"/etc/hostname"}`},
					},
				},
			},
			{
				deltas: []string{"All ", "done."},
				resp: &providers.NormalizedResponse{
					Content:      "All done.",
					FinishReason: providers.FinishStop,
				},
			},
		},
	}

	var executed []string
	loop := newTestLoop(fake)
	loop.execTool = func(toolName, toolID string, rawInput json.RawMessage) string {
		executed = append(executed, toolName)
		return "result for " + toolName
	}

	sink := &fakeSink{}
	session := userSession(t, "what's my hostname?")
	loop.run(context.Background(), sink, session)

	if fake.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", fake.calls)
	}
	if len(executed) != 2 || executed[0] != "axios-system__system_info" || executed[1] != "axios-fs__read_file" {
		t.Fatalf("executed tools = %v", executed)
	}

	// Session history: user, assistant(tool_calls), tool, tool, assistant.
	msgs := session.GetMessages()
	wantRoles := []string{"user", "assistant", "tool", "tool", "assistant"}
	if len(msgs) != len(wantRoles) {
		t.Fatalf("session has %d messages, want %d: %+v", len(msgs), len(wantRoles), msgs)
	}
	for i, want := range wantRoles {
		if msgs[i].Role != want {
			t.Errorf("message[%d].Role = %q, want %q", i, msgs[i].Role, want)
		}
	}
	if msgs[2].ToolCallID != "call_1" || msgs[2].Content != "result for axios-system__system_info" {
		t.Errorf("tool result message wrong: %+v", msgs[2])
	}
	if len(msgs[1].ToolCalls) != 2 {
		t.Errorf("assistant message should carry 2 tool calls, got %d", len(msgs[1].ToolCalls))
	}

	// Sink: two tool_use, two tool_result, streamed assistant deltas with
	// real provider/model names.
	if got := len(sink.byType("tool_use")); got != 2 {
		t.Errorf("tool_use messages = %d, want 2", got)
	}
	if got := len(sink.byType("tool_result")); got != 2 {
		t.Errorf("tool_result messages = %d, want 2", got)
	}
	assistants := sink.byType("assistant")
	if len(assistants) != 2 {
		t.Fatalf("assistant messages = %d, want 2 streamed deltas", len(assistants))
	}
	for _, a := range assistants {
		if a.Model != "gpt-4o" || a.Provider != "openai" {
			t.Errorf("assistant message carries model=%q provider=%q, want real names", a.Model, a.Provider)
		}
	}
	if assistants[0].Content+assistants[1].Content != "All done." {
		t.Errorf("streamed content = %q", assistants[0].Content+assistants[1].Content)
	}
	if got := len(sink.byType("error")); got != 0 {
		t.Errorf("unexpected error messages: %v", sink.byType("error"))
	}
}

func TestChatLoopSendsBufferedContentWhenNoDeltas(t *testing.T) {
	fake := &fakeProvider{
		name:  "anthropic",
		model: "claude-sonnet-4-6",
		steps: []fakeStep{
			{resp: &providers.NormalizedResponse{Content: "hello", FinishReason: providers.FinishStop}},
		},
	}
	loop := newTestLoop(fake)
	loop.execTool = func(string, string, json.RawMessage) string { return "" }

	sink := &fakeSink{}
	loop.run(context.Background(), sink, userSession(t, "hi"))

	assistants := sink.byType("assistant")
	if len(assistants) != 1 || assistants[0].Content != "hello" {
		t.Fatalf("assistant messages = %+v, want single buffered message", assistants)
	}
	if assistants[0].Provider != "anthropic" || assistants[0].Model != "claude-sonnet-4-6" {
		t.Errorf("assistant message names = %q/%q", assistants[0].Provider, assistants[0].Model)
	}
}

func TestChatLoopMaxIterationStop(t *testing.T) {
	fake := &fakeProvider{
		name:  "openai",
		model: "gpt-4o",
		steps: []fakeStep{
			{
				resp: &providers.NormalizedResponse{
					FinishReason: providers.FinishToolCalls,
					ToolCalls:    []providers.ToolCall{{ID: "loop", Name: "axios-system__system_info", Arguments: `{}`}},
				},
			},
		},
	}
	loop := newTestLoop(fake)
	executions := 0
	loop.execTool = func(string, string, json.RawMessage) string {
		executions++
		return "again"
	}

	sink := &fakeSink{}
	loop.run(context.Background(), sink, userSession(t, "loop forever"))

	if fake.calls != maxLoopIterations {
		t.Fatalf("provider calls = %d, want %d (max iterations)", fake.calls, maxLoopIterations)
	}
	if executions != maxLoopIterations {
		t.Fatalf("tool executions = %d, want %d", executions, maxLoopIterations)
	}
}

func TestChatLoopRetriesRetryableErrors(t *testing.T) {
	retryable := &providers.ClassifiedError{
		Reason:         providers.ReasonOverloaded,
		Provider:       "openai",
		Model:          "gpt-4o",
		Message:        "overloaded",
		Retryable:      true,
		ShouldFallback: true,
	}
	fake := &fakeProvider{
		name:  "openai",
		model: "gpt-4o",
		steps: []fakeStep{
			{err: retryable},
			{err: retryable},
			{resp: &providers.NormalizedResponse{Content: "recovered", FinishReason: providers.FinishStop}},
		},
	}
	loop := newTestLoop(fake)
	loop.execTool = func(string, string, json.RawMessage) string { return "" }
	slept := 0
	loop.sleep = func(time.Duration) { slept++ }

	sink := &fakeSink{}
	loop.run(context.Background(), sink, userSession(t, "hi"))

	if fake.calls != maxProviderAttempts {
		t.Fatalf("provider calls = %d, want %d (retries)", fake.calls, maxProviderAttempts)
	}
	if slept != maxProviderAttempts-1 {
		t.Errorf("sleep calls = %d, want %d", slept, maxProviderAttempts-1)
	}
	assistants := sink.byType("assistant")
	if len(assistants) != 1 || assistants[0].Content != "recovered" {
		t.Fatalf("assistant messages = %+v", assistants)
	}
	if got := len(sink.byType("error")); got != 0 {
		t.Errorf("unexpected error messages: %v", sink.byType("error"))
	}
}

func TestChatLoopFallbackAdvanceOnClassifiedError(t *testing.T) {
	authErr := &providers.ClassifiedError{
		Reason:         providers.ReasonAuth,
		StatusCode:     401,
		Provider:       "openai",
		Model:          "gpt-4o",
		Message:        "invalid api key",
		Retryable:      false,
		ShouldFallback: true,
	}
	primary := &fakeProvider{
		name:  "openai",
		model: "gpt-4o",
		steps: []fakeStep{{err: authErr}},
	}
	fallback := &fakeProvider{
		name:  "openrouter",
		model: "anthropic/claude-sonnet-4",
		steps: []fakeStep{
			{resp: &providers.NormalizedResponse{Content: "from fallback", FinishReason: providers.FinishStop}},
		},
	}

	loop := newTestLoop(primary)
	loop.execTool = func(string, string, json.RawMessage) string { return "" }
	loop.fallbacks = []FallbackSpec{
		{Provider: "openrouter", Model: "anthropic/claude-sonnet-4"},
	}
	var built []string
	loop.buildClient = func(provider, model string) (chatClient, error) {
		built = append(built, provider+"/"+model)
		return fallback, nil
	}

	sink := &fakeSink{}
	loop.run(context.Background(), sink, userSession(t, "hi"))

	if len(built) != 1 || built[0] != "openrouter/anthropic/claude-sonnet-4" {
		t.Fatalf("built clients = %v", built)
	}
	if primary.calls != 1 {
		t.Errorf("primary calls = %d, want 1 (non-retryable)", primary.calls)
	}
	if fallback.calls != 1 {
		t.Errorf("fallback calls = %d, want 1", fallback.calls)
	}

	// A status message must announce the switch.
	statuses := sink.byType("status")
	if len(statuses) != 1 {
		t.Fatalf("status messages = %+v, want 1 fallback notice", statuses)
	}

	assistants := sink.byType("assistant")
	if len(assistants) != 1 || assistants[0].Content != "from fallback" {
		t.Fatalf("assistant messages = %+v", assistants)
	}
	if assistants[0].Provider != "openrouter" || assistants[0].Model != "anthropic/claude-sonnet-4" {
		t.Errorf("assistant names = %q/%q, want fallback provider names", assistants[0].Provider, assistants[0].Model)
	}
}

func TestChatLoopFallbackChainExhaustedSendsError(t *testing.T) {
	billing := &providers.ClassifiedError{
		Reason:         providers.ReasonBilling,
		Provider:       "openai",
		Model:          "gpt-4o",
		Message:        "insufficient quota",
		Retryable:      false,
		ShouldFallback: true,
	}
	primary := &fakeProvider{name: "openai", model: "gpt-4o", steps: []fakeStep{{err: billing}}}

	loop := newTestLoop(primary)
	loop.execTool = func(string, string, json.RawMessage) string { return "" }
	loop.fallbacks = []FallbackSpec{{Provider: "groq", Model: "llama-3.1-70b-versatile"}}
	loop.buildClient = func(provider, model string) (chatClient, error) {
		return nil, fmt.Errorf("provider %q has no API key configured", provider)
	}

	sink := &fakeSink{}
	loop.run(context.Background(), sink, userSession(t, "hi"))

	errs := sink.byType("error")
	if len(errs) != 1 {
		t.Fatalf("error messages = %+v, want 1", errs)
	}
	if got := len(sink.byType("assistant")); got != 0 {
		t.Errorf("unexpected assistant messages after exhausted chain")
	}
}
