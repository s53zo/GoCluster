package ui

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SearchFilter debounces query updates to protect UI latency.
type SearchFilter struct {
	mu          sync.RWMutex
	query       string
	activeQuery string
	timer       *time.Timer
	ctx         context.Context
}

func NewSearchFilter(ctx context.Context) *SearchFilter {
	return &SearchFilter{ctx: ctx}
}

func (s *SearchFilter) SetQuery(query string, onChange func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.query = strings.ToLower(strings.TrimSpace(query))
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.ctx != nil && s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.timer = time.AfterFunc(250*time.Millisecond, func() {
		if s.ctx != nil && s.ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		s.activeQuery = s.query
		s.mu.Unlock()
		if onChange != nil {
			onChange()
		}
	})
	s.mu.Unlock()
}

func (s *SearchFilter) ActiveQuery() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeQuery
}

func (s *SearchFilter) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
}
