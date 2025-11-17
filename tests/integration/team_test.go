package integration

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/cexll/agentsdk-go/pkg/agent"
	"github.com/cexll/agentsdk-go/pkg/event"
	"github.com/cexll/agentsdk-go/pkg/telemetry"
	"github.com/cexll/agentsdk-go/pkg/tool"
)

func TestTeamAgentCollaborationModes(t *testing.T) {
	t.Parallel()
	progress := make(chan event.Event, 32)
	control := make(chan event.Event, 32)
	monitor := make(chan event.Event, 32)
	bus := event.NewEventBus(progress, control, monitor)

	mgr, exporter := newTelemetryManager(t)
	leader := newLeafAgent(t, "leader", mgr)
	worker := newLeafAgent(t, "worker", mgr)
	reviewer := newLeafAgent(t, "reviewer", mgr)
	team, err := agent.NewTeamAgent(agent.TeamConfig{
		Name: "squad",
		Members: []agent.TeamMemberConfig{
			{Name: "leader", Role: agent.TeamRoleLeader, Agent: leader, Capabilities: []string{"plan"}},
			{Name: "worker", Role: agent.TeamRoleWorker, Agent: worker, Capabilities: []string{"build"}},
			{Name: "reviewer", Role: agent.TeamRoleReviewer, Agent: reviewer, Capabilities: []string{"qa"}},
		},
		ShareTools: true,
		EventBus:   bus,
	})
	if err != nil {
		t.Fatalf("team agent: %v", err)
	}
	toolImpl := &integrationTool{name: "echo"}
	if err := team.AddTool(toolImpl); err != nil {
		t.Fatalf("add tool: %v", err)
	}

	tasks := []agent.TeamTask{
		{Name: "Plan", Instruction: "tool:echo {\"msg\":\"plan\"}", Role: agent.TeamRoleLeader},
		{Name: "Build", Instruction: "tool:echo {\"msg\":\"build\"}", Role: agent.TeamRoleWorker},
		{Name: "Review", Instruction: "tool:echo {\"msg\":\"qa\"}", Role: agent.TeamRoleReviewer},
	}
	hierTasks := []agent.TeamTask{
		{Name: "Plan", Instruction: "plan sprint goals", Role: agent.TeamRoleLeader},
		{Name: "Build", Instruction: "implement features", Role: agent.TeamRoleWorker},
		{Name: "Review", Instruction: "verify outputs", Role: agent.TeamRoleReviewer},
	}
	runCtx := agent.WithRunContext(context.Background(), agent.RunContext{SessionID: "team-int"})
	runCtx = agent.WithTeamRunConfig(runCtx, agent.TeamRunConfig{
		Mode:          agent.CollaborationSequential,
		Strategy:      agent.StrategyRoundRobin,
		Tasks:         tasks,
		ShareSession:  true,
		ShareEventBus: true,
	})
	res, err := team.Run(runCtx, "ignored")
	if err != nil {
		t.Fatalf("team run sequential: %v", err)
	}
	if res.StopReason == "" {
		t.Fatalf("expected stop reason for sequential run")
	}
	if len(res.ToolCalls) != len(tasks) {
		t.Fatalf("expected %d tool calls, got %d", len(tasks), len(res.ToolCalls))
	}

	if len(res.Events) == 0 {
		t.Fatalf("expected aggregated events from team run")
	}

	// Switch to parallel mode and ensure no error.
	runCtx = agent.WithTeamRunConfig(agent.WithRunContext(context.Background(), agent.RunContext{SessionID: "team-int"}), agent.TeamRunConfig{
		Mode:     agent.CollaborationParallel,
		Tasks:    tasks,
		Strategy: agent.StrategyLeastLoaded,
	})
	if _, err := team.Run(runCtx, "ignored"); err != nil {
		t.Fatalf("team parallel run: %v", err)
	}

	// Hierarchical mode exercises reviewers.
	runCtx = agent.WithTeamRunConfig(agent.WithRunContext(context.Background(), agent.RunContext{SessionID: "team-int"}), agent.TeamRunConfig{
		Mode:     agent.CollaborationHierarchical,
		Tasks:    hierTasks,
		Strategy: agent.StrategyCapability,
	})
	if _, err := team.Run(runCtx, "ignored"); err != nil {
		t.Fatalf("team hierarchical run: %v", err)
	}

	// Verify OTEL spans were produced.
	if spans := exporter.GetSpans(); len(spans) == 0 {
		t.Fatal("expected telemetry spans recorded for team run")
	}
}

func newTelemetryManager(t *testing.T) (*telemetry.Manager, *tracetest.InMemoryExporter) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(spanExporter)))
	mgr, err := telemetry.NewManager(telemetry.Config{
		ServiceName:    "team-int",
		MeterProvider:  mp,
		TracerProvider: tp,
	})
	if err != nil {
		t.Fatalf("telemetry manager: %v", err)
	}
	t.Cleanup(func() {
		_ = mgr.Shutdown(context.Background())
	})
	return mgr, spanExporter
}

func newLeafAgent(t *testing.T, name string, mgr *telemetry.Manager) agent.Agent {
	t.Helper()
	ag, err := agent.New(agent.Config{
		Name:           name,
		DefaultContext: agent.RunContext{SessionID: name},
	}, agent.WithTelemetry(mgr))
	if err != nil {
		t.Fatalf("new agent %s: %v", name, err)
	}
	return ag
}

type integrationTool struct {
	name string
}

func (i *integrationTool) Name() string        { return i.name }
func (i *integrationTool) Description() string { return "integration echo" }
func (i *integrationTool) Schema() *tool.JSONSchema {
	return nil
}

func (i *integrationTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	msg, _ := params["msg"].(string)
	return &tool.ToolResult{Output: msg, Success: true}, nil
}
