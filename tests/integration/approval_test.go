package integration

import (
	"context"
	"testing"
	"time"

	"github.com/cexll/agentsdk-go/pkg/approval"
	"github.com/cexll/agentsdk-go/pkg/workflow"
)

func TestApprovalFlowIntegration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := approval.NewRecordLog(dir)
	if err != nil {
		t.Fatalf("record log: %v", err)
	}
	queue := approval.NewQueue(store, approval.NewWhitelist())
	mw := workflow.NewApprovalMiddleware(
		queue,
		workflow.WithApprovalPollInterval(5*time.Millisecond),
		workflow.WithApprovalContextKeys("int.requests", "int.results", "int.session"),
	)

	execCtx := workflow.NewExecutionContext(context.Background(), map[string]any{
		"int.session": "sess-int",
		"int.requests": workflow.ApprovalRequest{
			Tool:   "fs.write",
			Params: map[string]any{"path": "/tmp/data", "mode": "0644"},
			Reason: "needs elevated privileges",
		},
	}, nil)

	errCh := make(chan error, 1)
	go func() {
		errCh <- mw.BeforeStepContext(execCtx, workflow.Step{Name: "fs.write"})
	}()

	var pending approval.Record
	select {
	case <-time.After(100 * time.Millisecond):
		t.Fatal("approval request never enqueued")
	case <-time.After(10 * time.Millisecond):
	}
	list := queue.Pending("sess-int")
	if len(list) != 1 {
		t.Fatalf("expected one pending request, got %d", len(list))
	}
	pending = list[0]
	if pending.Decision != approval.DecisionPending {
		t.Fatalf("pending decision mismatch: %+v", pending)
	}
	if _, err := queue.Approve(pending.ID, "granted"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	raw, ok := execCtx.Get("int.results")
	if !ok {
		t.Fatal("missing approval results in context")
	}
	results, ok := raw.([]approval.Record)
	if !ok || len(results) != 1 {
		t.Fatalf("unexpected results payload: %#v", raw)
	}
	if results[0].Decision != approval.DecisionApproved || results[0].Comment != "granted" {
		t.Fatalf("unexpected decision %+v", results[0])
	}

	// Second run should be auto-approved via whitelist.
	execCtx.Set("int.requests", workflow.ApprovalRequest{
		Tool:   "fs.write",
		Params: map[string]any{"path": "/tmp/data", "mode": "0644"},
	})
	if err := mw.BeforeStepContext(execCtx, workflow.Step{Name: "fs.write"}); err != nil {
		t.Fatalf("auto approval failed: %v", err)
	}
	raw, _ = execCtx.Get("int.results")
	results = raw.([]approval.Record)
	if !results[0].Auto {
		t.Fatalf("expected whitelist auto approval: %+v", results[0])
	}
}

func TestApprovalWhitelistPersistsAcrossRecovery(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := approval.NewRecordLog(dir)
	if err != nil {
		t.Fatalf("record log: %v", err)
	}
	queue := approval.NewQueue(store, approval.NewWhitelist())
	rec, auto, err := queue.Request("sess-recover", "tool.echo", map[string]any{"msg": "hi"})
	if err != nil || auto {
		t.Fatalf("initial request: err=%v auto=%v", err, auto)
	}
	if _, err := queue.Approve(rec.ID, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := queue.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	store2, err := approval.NewRecordLog(dir)
	if err != nil {
		t.Fatalf("re-open store: %v", err)
	}
	queue2 := approval.NewQueue(store2, approval.NewWhitelist())
	rec2, auto, err := queue2.Request("sess-recover", "tool.echo", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if !auto || rec2.Decision != approval.DecisionApproved {
		t.Fatalf("expected whitelist auto decision after recovery, got %+v", rec2)
	}

	// Pending records should also survive restarts.
	rec3, auto, err := queue2.Request("sess-recover", "tool.rm", map[string]any{"path": "/tmp"})
	if err != nil || auto {
		t.Fatalf("pending request: err=%v auto=%v", err, auto)
	}
	if err := queue2.Close(); err != nil {
		t.Fatalf("close queue2: %v", err)
	}
	store3, err := approval.NewRecordLog(dir)
	if err != nil {
		t.Fatalf("re-open store3: %v", err)
	}
	queue3 := approval.NewQueue(store3, approval.NewWhitelist())
	if pending := queue3.Pending("sess-recover"); len(pending) != 1 || pending[0].ID != rec3.ID {
		t.Fatalf("pending request lost across recovery: %+v", pending)
	}
}
