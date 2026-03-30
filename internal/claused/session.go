package claused

import "sync"

// Session holds the state of a single chat conversation.
type Session struct {
	ID       string
	Messages []Message
	mu       sync.Mutex
}

// NewSession creates a new chat session.
func NewSession(id string) *Session {
	return &Session{
		ID: id,
	}
}

// AddMessage appends a message to the conversation history.
func (s *Session) AddMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
}

// GetMessages returns a copy of the conversation history.
func (s *Session) GetMessages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]Message, len(s.Messages))
	copy(msgs, s.Messages)
	return msgs
}

// SessionStore manages active chat sessions.
type SessionStore struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionStore creates a new session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
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
