package benchmark

import (
	"context"
	"fmt"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/agent"
	"github.com/cexll/agentsdk-go/pkg/approval"
	"github.com/cexll/agentsdk-go/pkg/session"
	"github.com/cexll/agentsdk-go/pkg/tool"
	"github.com/cexll/agentsdk-go/pkg/workflow"
)

func BenchmarkAgentRun(b *testing.B) {
	ag := newBenchAgent(b, "bench-run")
	toolImpl := &benchTool{name: "echo"}
	if err := ag.AddTool(toolImpl); err != nil {
		b.Fatalf("add tool: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ag.Run(context.Background(), "tool:echo {\"msg\":\"hi\"}"); err != nil {
			b.Fatalf("run: %v", err)
		}
	}
}

func BenchmarkToolExecution(b *testing.B) {
	ag := newBenchAgent(b, "bench-tool")
	toolImpl := &benchTool{name: "echo"}
	if err := ag.AddTool(toolImpl); err != nil {
		b.Fatalf("add tool: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ag.Run(context.Background(), "tool:echo {\"msg\":\"payload\"}"); err != nil {
			b.Fatalf("run: %v", err)
		}
	}
}

func BenchmarkSessionPersistence(b *testing.B) {
	root := b.TempDir()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("bench-%d", i)
		sess, err := session.NewFileSession(id, root)
		if err != nil {
			b.Fatalf("file session: %v", err)
		}
		if err := sess.Append(session.Message{Role: "user", Content: "hello"}); err != nil {
			b.Fatalf("append message: %v", err)
		}
		if err := sess.AppendApproval(approval.Record{SessionID: id, Tool: "echo", Decision: approval.DecisionApproved}); err != nil {
			b.Fatalf("append approval: %v", err)
		}
		if err := sess.Close(); err != nil {
			b.Fatalf("close session: %v", err)
		}
	}
}

func BenchmarkWorkflowExecution(b *testing.B) {
	g := workflow.NewGraph()
	if err := g.AddNode(workflow.NewAction("start", func(ctx *workflow.ExecutionContext) error {
		val, _ := ctx.Get("count")
		current, _ := val.(int)
		ctx.Set("count", current+1)
		return nil
	})); err != nil {
		b.Fatalf("add node: %v", err)
	}
	if err := g.AddNode(workflow.NewAction("end", func(ctx *workflow.ExecutionContext) error {
		return nil
	})); err != nil {
		b.Fatalf("add end: %v", err)
	}
	if err := g.AddTransition("start", "end", workflow.Always()); err != nil {
		b.Fatalf("transition: %v", err)
	}
	exec := workflow.NewExecutor(g, workflow.WithInitialData(map[string]any{"count": 0}))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec.Run(context.Background()); err != nil {
			b.Fatalf("run executor: %v", err)
		}
	}
}

type benchTool struct {
	name string
}

func (b *benchTool) Name() string             { return b.name }
func (b *benchTool) Description() string      { return "benchmark tool" }
func (b *benchTool) Schema() *tool.JSONSchema { return nil }

func (b *benchTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	msg := params["msg"]
	return &tool.ToolResult{Output: fmt.Sprint(msg), Success: true}, nil
}

func newBenchAgent(tb testing.TB, name string) agent.Agent {
	tb.Helper()
	ag, err := agent.New(agent.Config{Name: name, DefaultContext: agent.RunContext{SessionID: name}})
	if err != nil {
		tb.Fatalf("new agent: %v", err)
	}
	return ag
}
