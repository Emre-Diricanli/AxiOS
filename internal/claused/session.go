package claused

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Session holds the state of a single chat conversation.
type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
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
func (s *Session) AddMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()

	// Set title from first user message
	if s.Title == "New Chat" && msg.Role == "user" {
		if text, ok := msg.Content.(string); ok && text != "" {
			title := text
			if len(title) > 60 {
				title = title[:60] + "..."
			}
			s.Title = title
		}
	}
}

// GetMessages returns a copy of the conversation history.
func (s *Session) GetMessages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]Message, len(s.Messages))
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
}

// NewSessionStore creates a new session store that persists to ~/.axios/sessions.json.
func NewSessionStore() *SessionStore {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".axios")
	os.MkdirAll(dir, 0755)

	ss := &SessionStore{
		sessions: make(map[string]*Session),
		filePath: filepath.Join(dir, "sessions.json"),
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

// Save persists all sessions to disk.
func (ss *SessionStore) Save() error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	// Build a serializable map
	data := make(map[string]*sessionData, len(ss.sessions))
	for id, s := range ss.sessions {
		s.mu.Lock()
		data[id] = &sessionData{
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
	return os.WriteFile(ss.filePath, b, 0644)
}

// load reads sessions from disk.
func (ss *SessionStore) load() {
	b, err := os.ReadFile(ss.filePath)
	if err != nil {
		return // File doesn't exist yet
	}

	var data map[string]*sessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return
	}

	for id, sd := range data {
		ss.sessions[id] = &Session{
			ID:        sd.ID,
			Title:     sd.Title,
			CreatedAt: sd.CreatedAt,
			UpdatedAt: sd.UpdatedAt,
			Messages:  sd.Messages,
		}
	}
}

// ClearAll removes all sessions and deletes the persistence file.
func (ss *SessionStore) ClearAll() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.sessions = make(map[string]*Session)
	os.Remove(ss.filePath)
}

// sessionData is the JSON-serializable form of a Session.
type sessionData struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}
