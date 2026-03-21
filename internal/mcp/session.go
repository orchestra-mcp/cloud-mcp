package mcp

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Session represents an active MCP SSE session.
type Session struct {
	ID        string
	UserID    uint   // 0 for anonymous
	RawToken  string // Bearer token from initialize — forwarded for admin tool calls
	CreatedAt time.Time
	LastSeen  time.Time
	send      chan []byte
	done      chan struct{}
}

// Send enqueues a message for delivery over the SSE stream.
// Non-blocking — drops the message if the channel is full.
func (s *Session) Send(data []byte) {
	select {
	case s.send <- data:
	default:
	}
}

// Done returns a channel that is closed when the session ends.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Close signals the session to terminate.
func (s *Session) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

// SessionStore manages active SSE sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore creates an empty session store.
func NewSessionStore() *SessionStore {
	ss := &SessionStore{
		sessions: make(map[string]*Session),
	}
	go ss.reaper()
	return ss
}

// Create allocates and registers a new session. Returns the session.
func (ss *SessionStore) Create(userID uint, rawToken string) *Session {
	s := &Session{
		ID:        uuid.New().String(),
		UserID:    userID,
		RawToken:  rawToken,
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
		send:      make(chan []byte, 64),
		done:      make(chan struct{}),
	}
	ss.mu.Lock()
	ss.sessions[s.ID] = s
	ss.mu.Unlock()
	return s
}

// Get returns a session by ID.
func (ss *SessionStore) Get(id string) (*Session, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	s, ok := ss.sessions[id]
	return s, ok
}

// Touch updates the last-seen timestamp for a session.
func (ss *SessionStore) Touch(id string) {
	ss.mu.Lock()
	if s, ok := ss.sessions[id]; ok {
		s.LastSeen = time.Now()
	}
	ss.mu.Unlock()
}

// Remove closes and deletes a session.
func (ss *SessionStore) Remove(id string) {
	ss.mu.Lock()
	if s, ok := ss.sessions[id]; ok {
		s.Close()
		delete(ss.sessions, id)
	}
	ss.mu.Unlock()
}

// reaper runs every minute and removes sessions idle for more than 10 minutes.
func (ss *SessionStore) reaper() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		ss.mu.Lock()
		for id, s := range ss.sessions {
			if s.LastSeen.Before(cutoff) {
				s.Close()
				delete(ss.sessions, id)
			}
		}
		ss.mu.Unlock()
	}
}
