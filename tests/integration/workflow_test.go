package integration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cexll/agentsdk-go/pkg/approval"
	"github.com/cexll/agentsdk-go/pkg/model"
	"github.com/cexll/agentsdk-go/pkg/session"
	"github.com/cexll/agentsdk-go/pkg/workflow"
)

func TestStateGraphComplexFlow(t *testing.T) {
	t.Parallel()
	g := workflow.NewGraph()
	var parallel int32
	if err := g.AddNode(workflow.NewAction("counter", func(ctx *workflow.ExecutionContext) error {
		val, _ := ctx.Get("count")
		next, _ := val.(int)
		next++
		ctx.Set("count", next)
		return nil
	})); err != nil {
		t.Fatalf("add counter: %v", err)
	}
	if err := g.AddNode(workflow.NewAction("gate", func(ctx *workflow.ExecutionContext) error {
		ctx.Set("gate", true)
		return nil
	})); err != nil {
		t.Fatalf("add gate: %v", err)
	}
	if err := g.AddNode(workflow.NewParallel("fanout", "left", "right")); err != nil {
		t.Fatalf("add fanout: %v", err)
	}
	if err := g.AddNode(workflow.NewAction("left", func(ctx *workflow.ExecutionContext) error {
		atomic.AddInt32(&parallel, 1)
		return nil
	})); err != nil {
		t.Fatalf("add left: %v", err)
	}
	if err := g.AddNode(workflow.NewAction("right", func(ctx *workflow.ExecutionContext) error {
		atomic.AddInt32(&parallel, 1)
		return nil
	})); err != nil {
		t.Fatalf("add right: %v", err)
	}
	if err := g.AddNode(workflow.NewAction("done", func(ctx *workflow.ExecutionContext) error {
		ctx.Set("complete", true)
		return nil
	})); err != nil {
		t.Fatalf("add done: %v", err)
	}
	atMost := func(limit int) workflow.Condition {
		return func(ctx *workflow.ExecutionContext) (bool, error) {
			val, _ := ctx.Get("count")
			n, _ := val.(int)
			return n < limit, nil
		}
	}
	atLeast := func(limit int) workflow.Condition {
		return func(ctx *workflow.ExecutionContext) (bool, error) {
			val, _ := ctx.Get("count")
			n, _ := val.(int)
			return n >= limit, nil
		}
	}
	if err := g.AddTransition("counter", "counter", atMost(3)); err != nil {
		t.Fatalf("loop: %v", err)
	}
	if err := g.AddTransition("counter", "gate", atLeast(3)); err != nil {
		t.Fatalf("exit loop: %v", err)
	}
	if err := g.AddTransition("gate", "fanout", workflow.Always()); err != nil {
		t.Fatalf("fanout transition: %v", err)
	}
	if err := g.AddTransition("left", "done", workflow.Always()); err != nil {
		t.Fatalf("left done: %v", err)
	}
	if err := g.AddTransition("right", "done", workflow.Always()); err != nil {
		t.Fatalf("right done: %v", err)
	}

	exec := workflow.NewExecutor(g, workflow.WithInitialData(map[string]any{"count": 0}))
	if err := exec.Run(context.Background()); err != nil {
		t.Fatalf("executor run: %v", err)
	}
	if got := atomic.LoadInt32(&parallel); got != 2 {
		t.Fatalf("expected both parallel branches to run, got %d", got)
	}
}

func TestWorkflowMiddlewareChain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store, err := approval.NewRecordLog(dir)
	if err != nil {
		t.Fatalf("record log: %v", err)
	}
	queue := approval.NewQueue(store, approval.NewWhitelist())
	approvalMW := workflow.NewApprovalMiddleware(queue, workflow.WithApprovalPollInterval(5*time.Millisecond))
	todoMW := workflow.NewTodoListMiddleware()
	summaryMW := workflow.NewSummarizationMiddleware(
		stubModel{},
		workflow.WithSummaryThreshold(1),
		workflow.WithSummaryKeys("int.summary.messages", "int.summary.current", "int.summary.history"),
	)
	subExecutor := &stubSubAgent{}
	subAgentMW := workflow.NewSubAgentMiddleware(subExecutor)

	g := workflow.NewGraph()
	if err := g.AddNode(workflow.NewAction("entry", func(ctx *workflow.ExecutionContext) error {
		ctx.Set("workflow.todo.text", "- [ ] write tests\n- [x] plan design")
		ctx.Set("workflow.summary.manual", true)
		ctx.Set("workflow.subagent.requests", []workflow.SubAgentRequest{{
			Instruction: "delegate task",
			Metadata:    map[string]any{"priority": "high"},
		}})
		ctx.Set("workflow.approval.requests", workflow.ApprovalRequest{
			SessionID: "sess-mw",
			Tool:      "deploy",
			Params:    map[string]any{"env": "dev"},
		})
		return nil
	})); err != nil {
		t.Fatalf("add entry: %v", err)
	}
	if err := g.AddNode(workflow.NewAction("exit", func(ctx *workflow.ExecutionContext) error {
		return nil
	})); err != nil {
		t.Fatalf("add exit: %v", err)
	}
	if err := g.AddTransition("entry", "exit", workflow.Always()); err != nil {
		t.Fatalf("transition: %v", err)
	}

	ctx := workflow.NewExecutionContext(context.Background(), map[string]any{
		"workflow.session.id": "sess-mw",
		"int.summary.messages": []session.Message{
			{Role: "user", Content: "previous turn"},
		},
	}, nil)
	ctx.Set("workflow.todo.text", "- [ ] write tests\n- [x] plan design")
	ctx.Set("workflow.summary.manual", true)
	ctx.Set("workflow.subagent.requests", []workflow.SubAgentRequest{{
		Instruction: "delegate task",
		Metadata:    map[string]any{"priority": "high"},
	}})
	ctx.Set("workflow.approval.requests", workflow.ApprovalRequest{
		SessionID: "sess-mw",
		Tool:      "deploy",
		Params:    map[string]any{"env": "dev"},
	})
	step := workflow.Step{Name: "entry"}
	if err := todoMW.BeforeStepContext(ctx, step); err != nil {
		t.Fatalf("todo before: %v", err)
	}
	if err := todoMW.AfterStepContext(ctx, step, nil); err != nil {
		t.Fatalf("todo after: %v", err)
	}
	if err := summaryMW.BeforeStepContext(ctx, step); err != nil {
		t.Fatalf("summary before: %v", err)
	}
	if err := subAgentMW.BeforeStepContext(ctx, step); err != nil {
		t.Fatalf("subagent before: %v", err)
	}
	if err := subAgentMW.AfterStepContext(ctx, step, nil); err != nil {
		t.Fatalf("subagent after: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- approvalMW.BeforeStepContext(ctx, step)
	}()
	time.Sleep(10 * time.Millisecond)
	pending := queue.Pending("sess-mw")
	if len(pending) != 1 {
		t.Fatalf("expected pending approval, got %d", len(pending))
	}
	if _, err := queue.Approve(pending[0].ID, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("approval middleware error: %v", err)
	}
	if snapshot := todoMW.List(); len(snapshot) != 2 {
		t.Fatalf("todo list not populated: %+v", snapshot)
	}
	if len(subExecutor.calls) != 1 || subExecutor.calls[0].Instruction != "delegate task" {
		t.Fatalf("subagent executor did not run: %+v", subExecutor.calls)
	}
}

func TestSessionPersistenceRecovery(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sess, err := session.NewFileSession("integration", root)
	if err != nil {
		t.Fatalf("file session: %v", err)
	}
	if err := sess.Append(session.Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	if err := sess.AppendApproval(approval.Record{
		SessionID: "integration",
		Tool:      "echo",
		Decision:  approval.DecisionApproved,
		Requested: time.Now(),
	}); err != nil {
		t.Fatalf("append approval: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	reloaded, err := session.NewFileSession("integration", root)
	if err != nil {
		t.Fatalf("reopen session: %v", err)
	}
	msgs, err := reloaded.List(session.Filter{})
	if err != nil || len(msgs) != 1 {
		t.Fatalf("list message: %+v err=%v", msgs, err)
	}
	approvals, err := reloaded.ListApprovals(approval.Filter{})
	if err != nil || len(approvals) != 1 {
		t.Fatalf("list approvals: %+v err=%v", approvals, err)
	}
}

type stubModel struct{}

func (stubModel) Generate(context.Context, []model.Message) (model.Message, error) {
	return model.Message{Content: "Session: recap\nStage: plan"}, nil
}

func (stubModel) GenerateStream(context.Context, []model.Message, model.StreamCallback) error {
	return errors.New("not implemented")
}

type stubSubAgent struct {
	calls []workflow.SubAgentRequest
}

func (s *stubSubAgent) Delegate(ctx context.Context, req workflow.SubAgentRequest) (workflow.SubAgentResult, error) {
	s.calls = append(s.calls, req)
	return workflow.SubAgentResult{Output: "delegate task done"}, nil
}
