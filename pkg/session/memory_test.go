package session

import (
	"errors"
	"testing"
	"time"
)

func TestMemorySessionAppend(t *testing.T) {
	fixed := time.Unix(1_700_000_000, 0).UTC()
	tests := []struct {
		name    string
		msg     Message
		prepare func(*MemorySession)
		wantErr error
		assert  func(t *testing.T, sess *MemorySession)
	}{
		{
			name: "auto fill id and timestamp",
			msg:  Message{Role: "user", Content: "hi"},
			assert: func(t *testing.T, sess *MemorySession) {
				t.Helper()
				msgs, err := sess.List(Filter{})
				if err != nil {
					t.Fatalf("list: %v", err)
				}
				if len(msgs) != 1 {
					t.Fatalf("messages len = %d", len(msgs))
				}
				if msgs[0].ID != "chat-000001" {
					t.Fatalf("id = %s", msgs[0].ID)
				}
				if !msgs[0].Timestamp.Equal(fixed) {
					t.Fatalf("timestamp = %s", msgs[0].Timestamp)
				}
			},
		},
		{
			name:    "missing role rejected",
			msg:     Message{Content: "hi"},
			wantErr: ErrInvalidMessage,
		},
		{
			name:    "closed session prevents append",
			msg:     Message{Role: "user", Content: "after"},
			prepare: func(sess *MemorySession) { _ = sess.Close() },
			wantErr: ErrSessionClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newMemorySessionForTest(t)
			sess.now = func() time.Time { return fixed }
			if tt.prepare != nil {
				tt.prepare(sess)
			}
			err := sess.Append(tt.msg)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("append failed: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, sess)
			}
		})
	}
}

func TestMemorySessionCheckpoint(t *testing.T) {
	fixed := time.Unix(1_700_010_000, 0).UTC()
	tests := []struct {
		name    string
		cpName  string
		prepare func(*MemorySession)
		wantErr error
		assert  func(t *testing.T, sess *MemorySession)
	}{
		{
			name:   "resume restores checkpoint snapshot",
			cpName: "alpha",
			assert: func(t *testing.T, sess *MemorySession) {
				t.Helper()
				msgs, err := sess.List(Filter{})
				if err != nil {
					t.Fatalf("list: %v", err)
				}
				if len(msgs) != 1 || msgs[0].Content != "before" {
					t.Fatalf("messages after resume = %+v", msgs)
				}
			},
		},
		{
			name:    "invalid checkpoint name",
			cpName:  "   ",
			wantErr: ErrInvalidCheckpointName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newMemorySessionForTest(t)
			sess.now = func() time.Time { return fixed }
			if err := sess.Append(Message{Role: "user", Content: "before"}); err != nil {
				t.Fatalf("append: %v", err)
			}
			if tt.prepare != nil {
				tt.prepare(sess)
			}
			err := sess.Checkpoint(tt.cpName)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkpoint failed: %v", err)
			}
			if err := sess.Append(Message{Role: "assistant", Content: "after"}); err != nil {
				t.Fatalf("append after checkpoint: %v", err)
			}
			if err := sess.Resume(tt.cpName); err != nil {
				t.Fatalf("resume: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, sess)
			}
		})
	}
}

func TestMemorySessionFork(t *testing.T) {
	fixed := time.Unix(1_700_020_000, 0).UTC()
	tests := []struct {
		name    string
		forkID  string
		prepare func(*MemorySession)
		wantErr error
		assert  func(t *testing.T, parent *MemorySession, child Session)
	}{
		{
			name:   "fork clones transcript",
			forkID: "branch",
			assert: func(t *testing.T, parent *MemorySession, child Session) {
				t.Helper()
				fork, ok := child.(*MemorySession)
				if !ok {
					t.Fatalf("unexpected fork type %T", child)
				}
				if fork.ID() != "branch" {
					t.Fatalf("fork id = %s", fork.ID())
				}
				msgsFork, err := fork.List(Filter{})
				if err != nil {
					t.Fatalf("fork list: %v", err)
				}
				msgsParent, err := parent.List(Filter{})
				if err != nil {
					t.Fatalf("parent list: %v", err)
				}
				if len(msgsFork) != len(msgsParent) {
					t.Fatalf("fork len %d parent len %d", len(msgsFork), len(msgsParent))
				}
				if err := fork.Append(Message{Role: "assistant", Content: "forked"}); err != nil {
					t.Fatalf("fork append: %v", err)
				}
				msgsParentAfter, _ := parent.List(Filter{})
				if len(msgsParentAfter) != len(msgsParent) {
					t.Fatalf("parent mutated by fork append")
				}
			},
		},
		{
			name:    "invalid fork id",
			forkID:  "",
			wantErr: ErrInvalidSessionID,
		},
		{
			name:    "closed session cannot fork",
			forkID:  "child",
			prepare: func(sess *MemorySession) { _ = sess.Close() },
			wantErr: ErrSessionClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newMemorySessionForTest(t)
			sess.now = func() time.Time { return fixed }
			if err := sess.Append(Message{Role: "user", Content: "seed"}); err != nil {
				t.Fatalf("append: %v", err)
			}
			if tt.prepare != nil {
				tt.prepare(sess)
			}
			child, err := sess.Fork(tt.forkID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("fork failed: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, sess, child)
			}
		})
	}
}

func newMemorySessionForTest(t *testing.T) *MemorySession {
	t.Helper()
	sess, err := NewMemorySession("chat")
	if err != nil {
		t.Fatalf("new memory session: %v", err)
	}
	return sess
}
