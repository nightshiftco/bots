package slackbot

import "sync"

// Sessions maps Slack thread_ts → nightshift session_id. MVP is
// in-memory only — pod restart drops the mapping, which is the
// documented trade-off in the plan.
type Sessions struct {
	mu sync.RWMutex
	m  map[string]string
}

func NewSessions() *Sessions { return &Sessions{m: map[string]string{}} }

func (s *Sessions) Get(threadTS string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[threadTS]
	return v, ok
}

func (s *Sessions) Put(threadTS, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[threadTS] = sessionID
}
