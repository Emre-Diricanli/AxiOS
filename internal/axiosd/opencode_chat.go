package axiosd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/axios-os/axios/pkg/opencode"
)

// Interactive code chat: a persistent opencode session per AxiOS chat
// session, with the SSE stream fanned out to the owning websocket as
// ordinary chat messages (assistant deltas, tool_use/tool_result blocks,
// a "done" status on idle). This is what makes opencode the backend of the
// coding chat rather than a fire-and-forget task runner.

// codeChatState tracks the live side of the bridge. Session mappings persist
// (opencode sessions survive restarts server-side); subscribers and
// tool-state dedup are per-process.
type codeChatState struct {
	mu sync.Mutex
	// chatToSession maps AxiOS chat session id -> opencode session id.
	chatToSession map[string]string
	// subscribers maps opencode session id -> the websocket that renders it.
	subscribers map[string]wsSink
	// toolStates dedups tool part updates: opencode session id -> callID ->
	// last forwarded status.
	toolStates map[string]map[string]string
	// partTypes remembers each part's type (opencode session id -> part id
	// -> type) so text deltas can be told apart from reasoning deltas —
	// both stream with field "text", and reasoning must not render as the
	// assistant's answer.
	partTypes map[string]map[string]string
	// pendingQuestions maps opencode session id -> the question awaiting the
	// user's next chat message.
	pendingQuestions map[string]opencode.QuestionAsked
	// onTurnComplete is invoked with the final assistant text when a code
	// turn finishes (bound by Server to persist the transcript).
	onTurnComplete func(chatSessionID, text string)
}

// codeSessionsFile is the persisted chat->opencode session mapping.
type codeSessionsFile struct {
	Version  int               `json:"version"`
	Sessions map[string]string `json:"sessions"`
}

func (m *OpencodeManager) codeChat() *codeChatState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.chat == nil {
		m.chat = &codeChatState{
			chatToSession:    map[string]string{},
			subscribers:      map[string]wsSink{},
			toolStates:       map[string]map[string]string{},
			partTypes:        map[string]map[string]string{},
			pendingQuestions: map[string]opencode.QuestionAsked{},
		}
		m.loadCodeSessions()
	}
	return m.chat
}

// bindTurnComplete registers the transcript-persistence callback.
func (m *OpencodeManager) bindTurnComplete(fn func(chatSessionID, text string)) {
	cc := m.codeChat()
	cc.mu.Lock()
	cc.onTurnComplete = fn
	cc.mu.Unlock()
}

func (m *OpencodeManager) codeSessionsPath() string {
	if m.settingsPath == "" {
		return ""
	}
	return strings.TrimSuffix(m.settingsPath, "opencode_settings.json") + "opencode_chat_sessions.json"
}

func (m *OpencodeManager) loadCodeSessions() {
	path := m.codeSessionsPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var file codeSessionsFile
	if err := json.Unmarshal(data, &file); err != nil {
		m.logger.Warn("failed to parse code chat sessions", "path", path, "error", err)
		return
	}
	if file.Sessions != nil {
		m.chat.chatToSession = file.Sessions
	}
}

func (cc *codeChatState) saveLocked(path string, logger interface{ Error(string, ...any) }) {
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(codeSessionsFile{Version: 1, Sessions: cc.chatToSession}, "", "  ")
	if err != nil {
		logger.Error("failed to marshal code chat sessions", "error", err)
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		logger.Error("failed to save code chat sessions", "path", path, "error", err)
	}
}

// ChatPrompt sends one user turn into the chat session's opencode session
// (creating it on first use), streaming progress back on sink. When the
// agent has a clarifying question pending, the message answers it instead
// of starting a new prompt.
func (m *OpencodeManager) ChatPrompt(chatSessionID, text, dir string, sink wsSink) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("message must not be empty")
	}
	if err := m.available(); err != nil {
		return err
	}
	cc := m.codeChat()

	cc.mu.Lock()
	sessionID, exists := cc.chatToSession[chatSessionID]
	var pending *opencode.QuestionAsked
	if exists {
		if q, ok := cc.pendingQuestions[sessionID]; ok {
			pending = &q
			delete(cc.pendingQuestions, sessionID)
		}
	}
	cc.mu.Unlock()

	// Answer a pending clarifying question with the user's message. Every
	// question gets the same free-text answer — crude for multi-question
	// asks, but those are rare in practice.
	if pending != nil {
		cc.mu.Lock()
		cc.subscribers[sessionID] = sink
		cc.mu.Unlock()
		answers := make([][]string, len(pending.Questions))
		for i := range answers {
			answers[i] = []string{text}
		}
		if err := m.client.ReplyQuestion(pending.ID, answers); err != nil {
			return fmt.Errorf("failed to answer the agent's question: %w", err)
		}
		m.logger.Info("code chat question answered", "chat_session", chatSessionID, "question", pending.ID)
		return nil
	}

	if !exists {
		if dir == "" {
			dir = m.opts.Workspace
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
		sess, err := m.client.CreateSession(dir, taskTitle(text))
		if err != nil {
			return fmt.Errorf("failed to create code session: %w", err)
		}
		sessionID = sess.ID
		cc.mu.Lock()
		cc.chatToSession[chatSessionID] = sessionID
		cc.saveLocked(m.codeSessionsPath(), m.logger)
		cc.mu.Unlock()
		m.logger.Info("code chat session created", "chat_session", chatSessionID, "opencode_session", sessionID, "dir", dir)
	}

	cc.mu.Lock()
	cc.subscribers[sessionID] = sink
	cc.mu.Unlock()

	// Chat turns prefer the pinned chat model (picker selection), then the
	// delegated-task default, then opencode's own default.
	var model *opencode.ModelRef
	if pinned := m.ChatModel(); pinned != "" {
		model = parseModelRef(pinned)
	} else if def := m.DefaultModel(); def != "" {
		model = parseModelRef(def)
	}
	if err := m.client.PromptAsync(sessionID, model, text); err != nil {
		return fmt.Errorf("failed to send message to opencode: %w", err)
	}
	return nil
}

// subscriberFor returns the websocket rendering an opencode session, if any.
func (m *OpencodeManager) subscriberFor(sessionID string) (wsSink, bool) {
	cc := m.codeChat()
	cc.mu.Lock()
	defer cc.mu.Unlock()
	sink, ok := cc.subscribers[sessionID]
	return sink, ok
}

// chatSessionFor reverse-maps an opencode session to its chat session id.
func (m *OpencodeManager) chatSessionFor(sessionID string) (string, bool) {
	cc := m.codeChat()
	cc.mu.Lock()
	defer cc.mu.Unlock()
	for chat, oc := range cc.chatToSession {
		if oc == sessionID {
			return chat, true
		}
	}
	return "", false
}

// codeModelLabel is the Model shown on streamed code-chat messages.
func (m *OpencodeManager) codeModelLabel() string {
	label := "opencode"
	if pinned := m.ChatModel(); pinned != "" {
		label = pinned
	} else if def := m.DefaultModel(); def != "" {
		label = def
	}
	return strings.TrimPrefix(label, "xai/")
}

// codeProviderLabel is the Provider shown on streamed code-chat messages:
// a pinned subscription model reads as SuperGrok, not as opencode plumbing.
func (m *OpencodeManager) codeProviderLabel() string {
	if m.ChatModel() != "" {
		return "SuperGrok"
	}
	return "opencode"
}

// handleCodeDelta forwards streamed deltas to the session's websocket.
// Reasoning parts stream with field "text" just like answer parts, so the
// part type (learned from message.part.updated) decides whether a delta is
// the assistant's answer or its thinking.
func (m *OpencodeManager) handleCodeDelta(props json.RawMessage) {
	var d opencode.PartDelta
	if err := json.Unmarshal(props, &d); err != nil || d.Field != "text" || d.Delta == "" {
		return
	}
	sink, ok := m.subscriberFor(d.SessionID)
	if !ok {
		return
	}

	cc := m.codeChat()
	cc.mu.Lock()
	partType := cc.partTypes[d.SessionID][d.PartID]
	cc.mu.Unlock()

	msgType := "assistant"
	if partType == "reasoning" {
		msgType = "thinking"
	}
	m.writeSink(sink, ChatMessage{
		Type:     msgType,
		Content:  d.Delta,
		Provider: m.codeProviderLabel(),
		Model:    m.codeModelLabel(),
	})
}

// handleCodePartUpdated forwards tool part state transitions as
// tool_use/tool_result blocks, deduplicating repeated updates per callID.
func (m *OpencodeManager) handleCodePartUpdated(props json.RawMessage) {
	var u opencode.PartUpdated
	if err := json.Unmarshal(props, &u); err != nil {
		return
	}

	// Remember every part's type so delta classification (answer vs
	// thinking) works; parts announce themselves here before streaming.
	if u.Part.ID != "" && u.Part.Type != "" {
		cc := m.codeChat()
		cc.mu.Lock()
		types := cc.partTypes[u.SessionID]
		if types == nil {
			types = map[string]string{}
			cc.partTypes[u.SessionID] = types
		}
		types[u.Part.ID] = u.Part.Type
		cc.mu.Unlock()
	}

	st := u.Part.ToolState()
	if st == nil {
		return
	}
	sink, ok := m.subscriberFor(u.SessionID)
	if !ok {
		return
	}

	cc := m.codeChat()
	cc.mu.Lock()
	states := cc.toolStates[u.SessionID]
	if states == nil {
		states = map[string]string{}
		cc.toolStates[u.SessionID] = states
	}
	last := states[u.Part.CallID]
	states[u.Part.CallID] = st.Status
	cc.mu.Unlock()

	if st.Status == last {
		return // repeated update within the same phase (metadata churn)
	}

	switch st.Status {
	case "running":
		input, _ := json.Marshal(st.Input)
		m.writeSink(sink, ChatMessage{
			Type:     "tool_use",
			ToolName: u.Part.Tool,
			ToolID:   u.Part.CallID,
			Content:  string(input),
		})
	case "completed":
		m.writeSink(sink, ChatMessage{
			Type:     "tool_result",
			ToolName: u.Part.Tool,
			ToolID:   u.Part.CallID,
			Content:  st.Output,
		})
	case "error":
		m.writeSink(sink, ChatMessage{
			Type:     "tool_result",
			ToolName: u.Part.Tool,
			ToolID:   u.Part.CallID,
			Content:  "error: " + st.Error,
		})
	}
}

// handleCodeQuestion surfaces a clarifying question in the chat; the user's
// next message answers it (see ChatPrompt).
func (m *OpencodeManager) handleCodeQuestion(props json.RawMessage) {
	var q opencode.QuestionAsked
	if err := json.Unmarshal(props, &q); err != nil || q.SessionID == "" || len(q.Questions) == 0 {
		return
	}
	sink, ok := m.subscriberFor(q.SessionID)
	if !ok {
		// Nobody to ask — reject so the agent moves on instead of hanging.
		if err := m.client.RejectQuestion(q.ID); err != nil {
			m.logger.Warn("failed to reject unattended question", "question", q.ID, "error", err)
		}
		return
	}

	cc := m.codeChat()
	cc.mu.Lock()
	cc.pendingQuestions[q.SessionID] = q
	cc.mu.Unlock()

	var b strings.Builder
	for i, question := range q.Questions {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(question.Question)
		if len(question.Options) > 0 {
			b.WriteString("\n")
			for _, opt := range question.Options {
				b.WriteString("\n- **" + opt.Label + "**")
				if opt.Description != "" {
					b.WriteString(" — " + opt.Description)
				}
			}
		}
	}
	b.WriteString("\n\n_Reply in chat to answer._")

	m.writeSink(sink, ChatMessage{
		Type:     "assistant",
		Content:  b.String(),
		Provider: m.codeProviderLabel(),
		Model:    m.codeModelLabel(),
	})
	m.writeSink(sink, ChatMessage{Type: "status", Content: "done"})
}

// finishCodeTurn closes out one code-chat turn on session.idle/error:
// persists the assistant text via the bound callback and signals "done".
func (m *OpencodeManager) finishCodeTurn(sessionID, errDetail string) {
	sink, subscribed := m.subscriberFor(sessionID)
	chatSessionID, mapped := m.chatSessionFor(sessionID)
	if !subscribed && !mapped {
		return
	}

	// A pending question means the turn paused rather than finished —
	// handleCodeQuestion already signalled the UI.
	cc := m.codeChat()
	cc.mu.Lock()
	_, questionPending := cc.pendingQuestions[sessionID]
	delete(cc.toolStates, sessionID)
	delete(cc.partTypes, sessionID)
	onComplete := cc.onTurnComplete
	cc.mu.Unlock()
	if questionPending && errDetail == "" {
		return
	}

	if errDetail != "" && subscribed {
		m.writeSink(sink, ChatMessage{Type: "error", Content: "code session error: " + trimForChat(errDetail)})
	}

	// Persist the final assistant text into the AxiOS chat history.
	if mapped && onComplete != nil && errDetail == "" {
		if msgs, err := m.client.Messages(sessionID); err == nil {
			if text := lastAssistantText(msgs); text != "" {
				onComplete(chatSessionID, text)
			}
		}
	}

	if subscribed {
		m.writeSink(sink, ChatMessage{Type: "status", Content: "done"})
	}
}

// isCodeSession reports whether an opencode session belongs to the chat
// bridge (as opposed to the task lane).
func (m *OpencodeManager) isCodeSession(sessionID string) bool {
	_, ok := m.chatSessionFor(sessionID)
	return ok
}

// writeSink writes one chat message, logging (not failing on) errors.
func (m *OpencodeManager) writeSink(sink wsSink, msg ChatMessage) {
	if err := sink.WriteJSON(msg); err != nil {
		m.logger.Error("code chat sink write failed", "error", err)
	}
}

// lastAssistantText extracts the last assistant message's text parts.
func lastAssistantText(msgs []opencode.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Info.Role != "assistant" {
			continue
		}
		var texts []string
		for _, part := range msgs[i].Parts {
			if part.Type == "text" && part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

// trimForChat bounds error payloads forwarded into the chat stream.
func trimForChat(s string) string {
	if len(s) > 2000 {
		return s[:2000] + "…"
	}
	return s
}
