package axiosd

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/axios-os/axios/pkg/providers"
)

// Session holds the state of a single chat conversation. Messages are stored
// in the canonical providers.Message format (OpenAI Chat Completions shape).
type Session struct {
	ID        string              `json:"id"`
	Title     string              `json:"title"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	Messages  []providers.Message `json:"messages"`
	mu        sync.Mutex
}

// NewSession creates a new chat session.
func NewSession(id string) *Session {
	now := time.Now()
	return &Session{
		ID:        id,
		Title:     "New Chat",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// AddMessage appends a message to the conversation history.
func (s *Session) AddMessage(msg providers.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()

	// Set title from first user message
	if s.Title == "New Chat" && msg.Role == "user" && msg.Content != "" {
		title := msg.Content
		if len(title) > 60 {
			title = title[:60] + "..."
		}
		s.Title = title
	}
}

// GetMessages returns a copy of the conversation history.
func (s *Session) GetMessages() []providers.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]providers.Message, len(s.Messages))
	copy(msgs, s.Messages)
	return msgs
}

// MessageCount returns the number of messages in the session.
func (s *Session) MessageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Messages)
}

// SessionStore manages active chat sessions.
type SessionStore struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	filePath string
	logger   *slog.Logger
}

// NewSessionStore creates a new session store that persists to ~/.axios/sessions.json.
func NewSessionStore(logger *slog.Logger) *SessionStore {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".axios")
	os.MkdirAll(dir, 0755)
	return newSessionStoreAt(filepath.Join(dir, "sessions.json"), logger)
}

// newSessionStoreAt creates a session store persisting to an explicit path
// (used by tests).
func newSessionStoreAt(path string, logger *slog.Logger) *SessionStore {
	if logger == nil {
		logger = slog.Default()
	}
	ss := &SessionStore{
		sessions: make(map[string]*Session),
		filePath: path,
		logger:   logger,
	}
	ss.load()
	return ss
}

// Get returns an existing session or creates a new one.
func (ss *SessionStore) Get(id string) *Session {
	ss.mu.RLock()
	s, ok := ss.sessions[id]
	ss.mu.RUnlock()
	if ok {
		return s
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	// Double-check after acquiring write lock
	if s, ok := ss.sessions[id]; ok {
		return s
	}
	s = NewSession(id)
	ss.sessions[id] = s
	return s
}

// Create creates a new session with a generated ID and returns it.
func (ss *SessionStore) Create(id string) *Session {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	s := NewSession(id)
	ss.sessions[id] = s
	return s
}

// Delete removes a session by ID.
func (ss *SessionStore) Delete(id string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if _, ok := ss.sessions[id]; !ok {
		return false
	}
	delete(ss.sessions, id)
	return true
}

// List returns metadata for all sessions, sorted by most recently updated.
func (ss *SessionStore) List() []SessionMeta {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	metas := make([]SessionMeta, 0, len(ss.sessions))
	for _, s := range ss.sessions {
		s.mu.Lock()
		metas = append(metas, SessionMeta{
			ID:           s.ID,
			Title:        s.Title,
			MessageCount: len(s.Messages),
			CreatedAt:    s.CreatedAt,
			UpdatedAt:    s.UpdatedAt,
		})
		s.mu.Unlock()
	}

	// Sort by UpdatedAt descending (most recent first)
	for i := 0; i < len(metas); i++ {
		for j := i + 1; j < len(metas); j++ {
			if metas[j].UpdatedAt.After(metas[i].UpdatedAt) {
				metas[i], metas[j] = metas[j], metas[i]
			}
		}
	}
	return metas
}

// SessionMeta is the summary info returned by the list endpoint.
type SessionMeta struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	MessageCount int       `json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// sessionsFileVersion is the current on-disk format version.
const sessionsFileVersion = 2

// sessionsFile is the versioned on-disk structure ({"version":2,...}).
type sessionsFile struct {
	Version  int                     `json:"version"`
	Sessions map[string]*sessionData `json:"sessions"`
}

// sessionData is the JSON-serializable form of a Session.
type sessionData struct {
	ID        string              `json:"id"`
	Title     string              `json:"title"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
	Messages  []providers.Message `json:"messages"`
}

// Save persists all sessions to disk (mode 0600 — conversations may contain
// sensitive command output).
func (ss *SessionStore) Save() error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	data := sessionsFile{
		Version:  sessionsFileVersion,
		Sessions: make(map[string]*sessionData, len(ss.sessions)),
	}
	for id, s := range ss.sessions {
		s.mu.Lock()
		data.Sessions[id] = &sessionData{
			ID:        s.ID,
			Title:     s.Title,
			CreatedAt: s.CreatedAt,
			UpdatedAt: s.UpdatedAt,
			Messages:  s.Messages,
		}
		s.mu.Unlock()
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ss.filePath, b, 0600)
}

// load reads sessions from disk, converting legacy (pre-version-2) files on a
// best-effort basis.
func (ss *SessionStore) load() {
	b, err := os.ReadFile(ss.filePath)
	if err != nil {
		return // File doesn't exist yet
	}

	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &probe); err == nil && probe.Version == sessionsFileVersion {
		var data sessionsFile
		if err := json.Unmarshal(b, &data); err != nil {
			ss.logger.Error("failed to parse sessions file", "path", ss.filePath, "error", err)
			return
		}
		for id, sd := range data.Sessions {
			ss.sessions[id] = &Session{
				ID:        sd.ID,
				Title:     sd.Title,
				CreatedAt: sd.CreatedAt,
				UpdatedAt: sd.UpdatedAt,
				Messages:  sd.Messages,
			}
		}
		return
	}

	ss.loadLegacy(b)
}

// legacySessionData mirrors the pre-version-2 on-disk session shape, where
// message content was either a string or an array of Anthropic content blocks.
type legacySessionData struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Messages  []legacyMessage `json:"messages"`
}

type legacyMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// legacyContentBlock is one Anthropic-style content block from a legacy session.
type legacyContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// loadLegacy converts a pre-version-2 sessions file: text blocks are kept,
// tool_use/tool_result blocks are dropped (and the drops logged).
func (ss *SessionStore) loadLegacy(b []byte) {
	var data map[string]*legacySessionData
	if err := json.Unmarshal(b, &data); err != nil {
		ss.logger.Error("failed to parse legacy sessions file", "path", ss.filePath, "error", err)
		return
	}

	totalSkipped := 0
	for id, sd := range data {
		msgs, skipped := convertLegacyMessages(sd.Messages)
		totalSkipped += skipped
		ss.sessions[id] = &Session{
			ID:        sd.ID,
			Title:     sd.Title,
			CreatedAt: sd.CreatedAt,
			UpdatedAt: sd.UpdatedAt,
			Messages:  msgs,
		}
	}
	ss.logger.Warn("converted legacy sessions file to version 2 (text blocks only)",
		"path", ss.filePath,
		"sessions", len(data),
		"skipped_blocks", totalSkipped,
	)
}

// convertLegacyMessages converts legacy messages to canonical providers.Message
// values, returning how many non-text content blocks were dropped.
func convertLegacyMessages(legacy []legacyMessage) ([]providers.Message, int) {
	var out []providers.Message
	skipped := 0

	for _, lm := range legacy {
		// Plain string content converts directly.
		var text string
		if err := json.Unmarshal(lm.Content, &text); err == nil {
			out = append(out, providers.Message{Role: lm.Role, Content: text})
			continue
		}

		// Otherwise it should be an array of content blocks.
		var blocks []legacyContentBlock
		if err := json.Unmarshal(lm.Content, &blocks); err != nil {
			skipped++
			continue
		}

		var combined string
		for _, blk := range blocks {
			if blk.Type == "text" && blk.Text != "" {
				combined += blk.Text
			} else {
				skipped++
			}
		}
		if combined == "" {
			continue // nothing usable in this message
		}
		out = append(out, providers.Message{Role: lm.Role, Content: combined})
	}
	return out, skipped
}

// ClearAll removes all sessions and deletes the persistence file.
func (ss *SessionStore) ClearAll() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.sessions = make(map[string]*Session)
	os.Remove(ss.filePath)
}
