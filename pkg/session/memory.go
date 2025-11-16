package session

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemorySession persists state in-process for fast prototyping and tests.
type MemorySession struct {
	id          string
	mu          sync.RWMutex
	messages    []Message
	checkpoints map[string][]Message
	seq         uint64
	closed      bool
	now         func() time.Time
}

// NewMemorySession constructs a MemorySession with the provided identifier.
func NewMemorySession(id string) (*MemorySession, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil, ErrInvalidSessionID
	}
	return &MemorySession{
		id:          trimmed,
		messages:    make([]Message, 0, 16),
		checkpoints: make(map[string][]Message),
		now:         time.Now,
	}, nil
}

// ID returns the stable identifier for the session.
func (s *MemorySession) ID() string {
	return s.id
}

// Append adds the message to the in-memory transcript.
func (s *MemorySession) Append(msg Message) error {
	if strings.TrimSpace(msg.Role) == "" {
		return fmt.Errorf("%w: role is required", ErrInvalidMessage)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	s.seq++
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%s-%06d", s.id, s.seq)
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = s.now().UTC()
	} else {
		msg.Timestamp = msg.Timestamp.UTC()
	}
	msg.ToolCalls = cloneToolCalls(msg.ToolCalls)
	s.messages = append(s.messages, msg)
	return nil
}

// List enumerates messages filtered by the provided predicate.
func (s *MemorySession) List(filter Filter) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrSessionClosed
	}

	role := strings.TrimSpace(filter.Role)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit < 0 {
		limit = 0
	}

	var (
		start time.Time
		end   time.Time
	)
	hasStart := filter.StartTime != nil
	if hasStart {
		start = filter.StartTime.UTC()
	}
	hasEnd := filter.EndTime != nil
	if hasEnd {
		end = filter.EndTime.UTC()
	}

	var result []Message
	skipped := 0

	for _, msg := range s.messages {
		if role != "" && msg.Role != role {
			continue
		}
		if hasStart && msg.Timestamp.Before(start) {
			continue
		}
		if hasEnd && msg.Timestamp.After(end) {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		result = append(result, cloneMessage(msg))
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result, nil
}

// Checkpoint stores a snapshot of the transcript under the provided name.
func (s *MemorySession) Checkpoint(name string) error {
	normalized, err := normalizeCheckpointName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	s.checkpoints[normalized] = cloneMessages(s.messages)
	return nil
}

// Resume replaces the current transcript with the stored checkpoint.
func (s *MemorySession) Resume(name string) error {
	normalized, err := normalizeCheckpointName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	snapshot, ok := s.checkpoints[normalized]
	if !ok {
		return fmt.Errorf("%w: %s", ErrCheckpointNotFound, normalized)
	}
	s.messages = cloneMessages(snapshot)
	return nil
}

// Fork clones the current state into a new MemorySession with the provided id.
func (s *MemorySession) Fork(id string) (Session, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil, ErrInvalidSessionID
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrSessionClosed
	}
	return &MemorySession{
		id:          trimmed,
		messages:    cloneMessages(s.messages),
		checkpoints: cloneCheckpointMap(s.checkpoints),
		now:         s.now,
		seq:         uint64(len(s.messages)),
	}, nil
}

// Close releases references held by the session. Subsequent operations fail.
func (s *MemorySession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.messages = nil
	s.checkpoints = nil
	return nil
}

func normalizeCheckpointName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", ErrInvalidCheckpointName
	}
	return trimmed, nil
}

func cloneMessages(src []Message) []Message {
	if len(src) == 0 {
		return nil
	}
	dst := make([]Message, len(src))
	for i, msg := range src {
		dst[i] = cloneMessage(msg)
	}
	return dst
}

func cloneCheckpointMap(src map[string][]Message) map[string][]Message {
	if len(src) == 0 {
		return make(map[string][]Message)
	}
	dst := make(map[string][]Message, len(src))
	for name, msgs := range src {
		dst[name] = cloneMessages(msgs)
	}
	return dst
}

func cloneMessage(msg Message) Message {
	cloned := msg
	cloned.ToolCalls = cloneToolCalls(msg.ToolCalls)
	return cloned
}

func cloneToolCalls(calls []ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	dst := make([]ToolCall, len(calls))
	for i, call := range calls {
		dst[i] = cloneToolCall(call)
	}
	return dst
}

func cloneToolCall(call ToolCall) ToolCall {
	cloned := call
	if call.Arguments != nil {
		cloned.Arguments = cloneMap(call.Arguments)
	}
	if call.Metadata != nil {
		cloned.Metadata = cloneMap(call.Metadata)
	}
	return cloned
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// ensure interface compliance at compile time.
var _ Session = (*MemorySession)(nil)
