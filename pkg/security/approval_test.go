package security

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

func newTestQueue(t *testing.T) (*ApprovalQueue, *fakeClock) {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "approvals.json")
	q, err := NewApprovalQueue(store)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	q.clock = clock.Now
	return q, clock
}

func TestApprovalQueueRequestValidation(t *testing.T) {
	q, _ := newTestQueue(t)
	tests := []struct {
		name    string
		session string
		command string
		wantErr string
	}{
		{name: "missing session", session: "", command: "ls", wantErr: "session id"},
		{name: "missing command", session: "sess", command: "   ", wantErr: "command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := q.Request(tt.session, tt.command, nil); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q got %v", tt.wantErr, err)
			}
		})
	}

	path := filepath.Join(t.TempDir(), "dir", "file.txt")
	rec, err := q.Request("sess", "echo ok", []string{"", path})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if rec.State != ApprovalPending {
		t.Fatalf("expected pending got %s", rec.State)
	}
	if len(rec.Paths) != 1 || rec.Paths[0] != normalizePath(path) {
		t.Fatalf("paths not normalized: %+v", rec.Paths)
	}
}

func TestApprovalQueueAutoWhitelist(t *testing.T) {
	q, clock := newTestQueue(t)
	session := "sess"
	q.mu.Lock()
	q.whitelist[session] = clock.now.Add(time.Minute)
	q.mu.Unlock()

	rec, err := q.Request(session, "rm", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if rec.State != ApprovalApproved || !rec.AutoApproved {
		t.Fatalf("expected auto approved, got %#v", rec)
	}
	if rec.ApprovedAt == nil || !strings.Contains(rec.Reason, "whitelisted") {
		t.Fatalf("auto approval metadata missing: %#v", rec)
	}
}

func TestApprovalQueueApproveFlow(t *testing.T) {
	q, clock := newTestQueue(t)
	rec, err := q.Request("sess", "ls", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	approved, err := q.Approve(rec.ID, "alice", time.Minute)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.State != ApprovalApproved || approved.Approver != "alice" {
		t.Fatalf("unexpected approval state: %#v", approved)
	}
	if approved.ExpiresAt == nil || approved.ExpiresAt.Before(clock.now) {
		t.Fatalf("whitelist expiry missing: %#v", approved)
	}

	if _, err := q.Approve("missing", "ops", 0); err == nil {
		t.Fatalf("expected error for missing approval")
	}
}

func TestApprovalQueueDenyFlow(t *testing.T) {
	q, _ := newTestQueue(t)
	rec, err := q.Request("sess", "ls", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	denied, err := q.Deny(rec.ID, "bob", "unsafe")
	if err != nil {
		t.Fatalf("deny: %v", err)
	}
	if denied.State != ApprovalDenied || denied.Reason != "unsafe" {
		t.Fatalf("unexpected deny state: %#v", denied)
	}

	if _, err := q.Approve(rec.ID, "ops", 0); err == nil {
		t.Fatalf("expected approve after deny to fail")
	}

	approved, err := q.Request("sess2", "date", nil)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if _, err := q.Approve(approved.ID, "ops", 0); err != nil {
		t.Fatalf("approve second: %v", err)
	}
	if _, err := q.Deny(approved.ID, "ops", "late"); err == nil {
		t.Fatalf("expected deny after approval to error")
	}
}

func TestApprovalQueueListPendingAndClone(t *testing.T) {
	q, _ := newTestQueue(t)
	first, _ := q.Request("s1", "cmd1", nil)
	second, _ := q.Request("s2", "cmd2", nil)
	if _, err := q.Approve(second.ID, "ops", 0); err != nil {
		t.Fatalf("approve second: %v", err)
	}

	pending := q.ListPending()
	if len(pending) != 1 || pending[0].ID != first.ID {
		t.Fatalf("expected only first pending, got %#v", pending)
	}
	pending[0].State = ApprovalApproved
	if q.records[first.ID].State != ApprovalPending {
		t.Fatalf("list returned non-clone")
	}
}

func TestApprovalQueueWhitelistExpiry(t *testing.T) {
	q, clock := newTestQueue(t)
	if q.IsWhitelisted("sess") {
		t.Fatalf("unexpected whitelist")
	}

	q.mu.Lock()
	q.whitelist["sess"] = clock.now.Add(30 * time.Second)
	q.mu.Unlock()
	if !q.IsWhitelisted("sess") {
		t.Fatalf("expected whitelist to be true")
	}

	clock.Advance(time.Minute)
	if q.IsWhitelisted("sess") {
		t.Fatalf("expected whitelist to expire")
	}
}
