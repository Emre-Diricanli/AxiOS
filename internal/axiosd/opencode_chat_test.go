package axiosd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/axios-os/axios/pkg/opencode"
)

func TestChatPromptCreatesAndReusesSession(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)
	m.opts.Model = "xai/grok-4.5"
	sink := &fakeSink{}

	if err := m.ChatPrompt("chat-1", "add a README", "", sink); err != nil {
		t.Fatalf("ChatPrompt: %v", err)
	}
	if err := m.ChatPrompt("chat-1", "now add tests", "", sink); err != nil {
		t.Fatalf("second ChatPrompt: %v", err)
	}

	client.mu.Lock()
	if client.sessionSeq != 1 {
		t.Errorf("sessions created = %d, want 1 (reuse)", client.sessionSeq)
	}
	if len(client.prompts) != 2 || !strings.HasPrefix(client.prompts[1], "ses_1:") {
		t.Errorf("prompts = %v, want both on ses_1", client.prompts)
	}
	if client.promptModels[0] != "xai/grok-4.5" {
		t.Errorf("model = %q, want the configured default", client.promptModels[0])
	}
	client.mu.Unlock()

	// A different chat session gets its own opencode session.
	if err := m.ChatPrompt("chat-2", "hello", "", &fakeSink{}); err != nil {
		t.Fatalf("ChatPrompt chat-2: %v", err)
	}
	client.mu.Lock()
	if client.sessionSeq != 2 {
		t.Errorf("sessions after second chat = %d, want 2", client.sessionSeq)
	}
	client.mu.Unlock()
}

func TestCodeDeltaStreamsToSubscriber(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)
	sink := &fakeSink{}
	if err := m.ChatPrompt("chat-1", "task", "", sink); err != nil {
		t.Fatal(err)
	}

	props, _ := json.Marshal(opencode.PartDelta{SessionID: "ses_1", Field: "text", Delta: "Hello"})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartDelta, Properties: props})

	// Non-text fields and unknown sessions are ignored.
	props2, _ := json.Marshal(opencode.PartDelta{SessionID: "ses_1", Field: "reasoning", Delta: "hmm"})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartDelta, Properties: props2})
	props3, _ := json.Marshal(opencode.PartDelta{SessionID: "ses_other", Field: "text", Delta: "nope"})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartDelta, Properties: props3})

	got := sink.byType("assistant")
	if len(got) != 1 || got[0].Content != "Hello" || got[0].Provider != "opencode" {
		t.Errorf("assistant deltas = %+v", got)
	}
}

func TestCodeToolEventsForwardedAndDeduped(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)
	sink := &fakeSink{}
	if err := m.ChatPrompt("chat-1", "task", "", sink); err != nil {
		t.Fatal(err)
	}

	send := func(status, output string) {
		state, _ := json.Marshal(map[string]any{"status": status, "input": map[string]any{"command": "go test"}, "output": output})
		props, _ := json.Marshal(map[string]any{
			"sessionID": "ses_1",
			"part":      map[string]any{"type": "tool", "callID": "call-1", "tool": "bash", "state": json.RawMessage(state)},
		})
		m.handleEvent(opencode.Event{Type: opencode.EventMessagePartUpdated, Properties: props})
	}

	send("pending", "")
	send("running", "")
	send("running", "") // repeated running update must not duplicate tool_use
	send("completed", "ok: 7 tests passed")

	uses := sink.byType("tool_use")
	results := sink.byType("tool_result")
	if len(uses) != 1 || uses[0].ToolName != "bash" || !strings.Contains(uses[0].Content, "go test") {
		t.Errorf("tool_use = %+v", uses)
	}
	if len(results) != 1 || results[0].Content != "ok: 7 tests passed" {
		t.Errorf("tool_result = %+v", results)
	}
}

func TestCodeQuestionFlow(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)
	sink := &fakeSink{}
	if err := m.ChatPrompt("chat-1", "refactor", "", sink); err != nil {
		t.Fatal(err)
	}

	q := opencode.QuestionAsked{
		ID:        "que_1",
		SessionID: "ses_1",
		Questions: []opencode.QuestionInfo{{
			Question: "Which framework?",
			Header:   "Framework",
			Options:  []opencode.QuestionOption{{Label: "stdlib"}, {Label: "testify"}},
		}},
	}
	props, _ := json.Marshal(q)
	m.handleEvent(opencode.Event{Type: opencode.EventQuestionAsked, Properties: props})

	// Question rendered + turn released back to the user.
	assistants := sink.byType("assistant")
	if len(assistants) != 1 || !strings.Contains(assistants[0].Content, "Which framework?") ||
		!strings.Contains(assistants[0].Content, "stdlib") {
		t.Fatalf("question message = %+v", assistants)
	}
	if len(sink.byType("status")) != 1 {
		t.Error("question should release the turn with a done status")
	}

	// session.idle while a question is pending must NOT double-send done.
	idleProps, _ := json.Marshal(map[string]string{"sessionID": "ses_1"})
	m.handleEvent(opencode.Event{Type: opencode.EventSessionIdle, Properties: idleProps})
	if n := len(sink.byType("status")); n != 1 {
		t.Errorf("status count after idle-with-pending-question = %d, want 1", n)
	}

	// The next user message answers the question instead of prompting.
	if err := m.ChatPrompt("chat-1", "testify", "", sink); err != nil {
		t.Fatalf("answer ChatPrompt: %v", err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.questionReplies) != 1 || client.questionReplies[0] != `que_1=[["testify"]]` {
		t.Errorf("question replies = %v", client.questionReplies)
	}
	if len(client.prompts) != 1 {
		t.Errorf("prompts = %v — the answer must not start a new prompt", client.prompts)
	}
}

func TestCodeQuestionRejectedWithoutSubscriber(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)

	q := opencode.QuestionAsked{ID: "que_9", SessionID: "ses_unknown",
		Questions: []opencode.QuestionInfo{{Question: "?"}}}
	props, _ := json.Marshal(q)
	m.handleEvent(opencode.Event{Type: opencode.EventQuestionAsked, Properties: props})

	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.questionRejects) != 1 || client.questionRejects[0] != "que_9" {
		t.Errorf("rejects = %v, want que_9 auto-rejected", client.questionRejects)
	}
}

func TestFinishCodeTurn(t *testing.T) {
	client := &fakeOpencodeClient{
		messages: map[string][]opencode.Message{
			"ses_1": {{
				Info:  opencode.MessageInfo{Role: "assistant"},
				Parts: []opencode.Part{{Type: "text", Text: "All done — README added."}},
			}},
		},
	}
	m := newTestOpencodeManager(t, client)
	var persistedSession, persistedText string
	m.bindTurnComplete(func(chatSessionID, text string) {
		persistedSession, persistedText = chatSessionID, text
	})
	sink := &fakeSink{}
	if err := m.ChatPrompt("chat-1", "add a README", "", sink); err != nil {
		t.Fatal(err)
	}

	idleProps, _ := json.Marshal(map[string]string{"sessionID": "ses_1"})
	m.handleEvent(opencode.Event{Type: opencode.EventSessionIdle, Properties: idleProps})

	if persistedSession != "chat-1" || persistedText != "All done — README added." {
		t.Errorf("persisted = (%q, %q)", persistedSession, persistedText)
	}
	if len(sink.byType("status")) != 1 {
		t.Error("idle should send exactly one done status")
	}

	// Error path: error message + done, no transcript persistence.
	persistedText = ""
	errProps, _ := json.Marshal(map[string]string{"sessionID": "ses_1", "error": "boom"})
	m.handleEvent(opencode.Event{Type: opencode.EventSessionError, Properties: errProps})
	if persistedText != "" {
		t.Error("errored turn must not persist a transcript")
	}
	if errs := sink.byType("error"); len(errs) != 1 || !strings.Contains(errs[0].Content, "code session error") {
		t.Errorf("errors = %+v", errs)
	}
}

func TestCodeSessionMappingPersists(t *testing.T) {
	dir := t.TempDir()
	tasksPath := dir + "/opencode_tasks.json"

	m1 := NewOpencodeManager(OpencodeOptions{Enabled: true, Workspace: t.TempDir()}, nil, tasksPath, testLogger())
	m1.client = &fakeOpencodeClient{}
	m1.ready = true
	if err := m1.ChatPrompt("chat-1", "hello", "", &fakeSink{}); err != nil {
		t.Fatal(err)
	}

	m2 := NewOpencodeManager(OpencodeOptions{Enabled: true, Workspace: t.TempDir()}, nil, tasksPath, testLogger())
	client2 := &fakeOpencodeClient{}
	m2.client = client2
	m2.ready = true
	if err := m2.ChatPrompt("chat-1", "continue", "", &fakeSink{}); err != nil {
		t.Fatal(err)
	}
	client2.mu.Lock()
	defer client2.mu.Unlock()
	if client2.sessionSeq != 0 {
		t.Errorf("restarted manager created a new session; want reuse of the persisted mapping")
	}
	if len(client2.prompts) != 1 || !strings.HasPrefix(client2.prompts[0], "ses_1:") {
		t.Errorf("prompts = %v, want continuation on ses_1", client2.prompts)
	}
}

func TestChatModelPinRoutesAndPersists(t *testing.T) {
	dir := t.TempDir()
	tasksPath := dir + "/opencode_tasks.json"

	m := NewOpencodeManager(OpencodeOptions{Enabled: true, Workspace: t.TempDir()}, nil, tasksPath, testLogger())
	client := &fakeOpencodeClient{}
	m.client = client
	m.ready = true

	s := &Server{logger: testLogger()}
	s.SetOpencodeManager(m)

	// Pin via the switch endpoint (bare name gets the xai/ prefix).
	rec := httptest.NewRecorder()
	s.handleSwitchModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/switch",
		strings.NewReader(`{"model":"grok-4.5","backend":"supergrok"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch status = %d, body %s", rec.Code, rec.Body)
	}
	if got := m.ChatModel(); got != "xai/grok-4.5" {
		t.Fatalf("ChatModel = %q", got)
	}

	// Current-model reflects the pin.
	rec = httptest.NewRecorder()
	s.handleCurrentModel(rec, httptest.NewRequest(http.MethodGet, "/api/models/current", nil))
	var cur struct{ Model, Backend, Provider string }
	if err := json.Unmarshal(rec.Body.Bytes(), &cur); err != nil {
		t.Fatal(err)
	}
	if cur.Model != "grok-4.5" || cur.Backend != "supergrok" {
		t.Errorf("current = %+v", cur)
	}

	// Chat turns use the pinned model through the bridge.
	if err := m.ChatPrompt("chat-1", "hello", "", &fakeSink{}); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	if client.promptModels[0] != "xai/grok-4.5" {
		t.Errorf("prompt model = %q", client.promptModels[0])
	}
	client.mu.Unlock()

	// The pin survives a restart and coexists with the delegated default.
	if err := m.SetDefaultModel("xai/grok-4.3"); err != nil {
		t.Fatal(err)
	}
	m2 := NewOpencodeManager(OpencodeOptions{Enabled: true}, nil, tasksPath, testLogger())
	if m2.ChatModel() != "xai/grok-4.5" || m2.DefaultModel() != "xai/grok-4.3" {
		t.Errorf("after restart: chat=%q default=%q", m2.ChatModel(), m2.DefaultModel())
	}
}

func TestReasoningDeltasStreamAsThinking(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)
	sink := &fakeSink{}
	if err := m.ChatPrompt("chat-1", "hi", "", sink); err != nil {
		t.Fatal(err)
	}

	// The reasoning part announces itself via message.part.updated first.
	partProps, _ := json.Marshal(map[string]any{
		"sessionID": "ses_1",
		"part":      map[string]any{"id": "prt_r1", "type": "reasoning"},
	})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartUpdated, Properties: partProps})
	textProps, _ := json.Marshal(map[string]any{
		"sessionID": "ses_1",
		"part":      map[string]any{"id": "prt_t1", "type": "text"},
	})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartUpdated, Properties: textProps})

	// Both stream deltas with field "text" — only the part type separates them.
	d1, _ := json.Marshal(opencode.PartDelta{SessionID: "ses_1", PartID: "prt_r1", Field: "text", Delta: "The user is greeting me. "})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartDelta, Properties: d1})
	d2, _ := json.Marshal(opencode.PartDelta{SessionID: "ses_1", PartID: "prt_t1", Field: "text", Delta: "Hey! What do you need?"})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartDelta, Properties: d2})

	thinking := sink.byType("thinking")
	answers := sink.byType("assistant")
	if len(thinking) != 1 || !strings.Contains(thinking[0].Content, "greeting") {
		t.Errorf("thinking = %+v", thinking)
	}
	if len(answers) != 1 || answers[0].Content != "Hey! What do you need?" {
		t.Errorf("assistant = %+v", answers)
	}
}

func TestPinnedChatUsesSuperGrokLabels(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)
	if err := m.SetChatModel("xai/grok-4.5"); err != nil {
		t.Fatal(err)
	}
	sink := &fakeSink{}
	if err := m.ChatPrompt("chat-1", "hi", "", sink); err != nil {
		t.Fatal(err)
	}
	d, _ := json.Marshal(opencode.PartDelta{SessionID: "ses_1", PartID: "prt_1", Field: "text", Delta: "Hello"})
	m.handleEvent(opencode.Event{Type: opencode.EventMessagePartDelta, Properties: d})

	got := sink.byType("assistant")
	if len(got) != 1 || got[0].Provider != "SuperGrok" || got[0].Model != "grok-4.5" {
		t.Errorf("labels = %+v, want SuperGrok / grok-4.5", got)
	}
}

func TestAbortChatTurnAndChatDiff(t *testing.T) {
	client := &fakeOpencodeClient{
		diffs: map[string][]opencode.FileDiff{
			"ses_1": {{File: "main.go", Additions: 2, Deletions: 1, Status: "modified"}},
		},
	}
	m := newTestOpencodeManager(t, client)

	// No code session yet: abort errors, diff is empty (not an error).
	if err := m.AbortChatTurn("chat-1"); err == nil {
		t.Error("abort without a code session should error")
	}
	if diff, err := m.ChatDiff("chat-1"); err != nil || diff != nil {
		t.Errorf("diff without a code session = (%v, %v), want (nil, nil)", diff, err)
	}

	if err := m.ChatPrompt("chat-1", "work", "", &fakeSink{}); err != nil {
		t.Fatal(err)
	}
	if err := m.AbortChatTurn("chat-1"); err != nil {
		t.Fatalf("AbortChatTurn: %v", err)
	}
	client.mu.Lock()
	aborted := append([]string(nil), client.aborted...)
	client.mu.Unlock()
	if len(aborted) != 1 || aborted[0] != "ses_1" {
		t.Errorf("aborted = %v, want [ses_1]", aborted)
	}

	diff, err := m.ChatDiff("chat-1")
	if err != nil || len(diff) != 1 || diff[0].File != "main.go" {
		t.Errorf("ChatDiff = (%+v, %v)", diff, err)
	}
}
