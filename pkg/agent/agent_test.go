package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/event"
	"github.com/cexll/agentsdk-go/pkg/tool"
)

func TestAgentRun(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		input   string
		setup   func(t *testing.T, ag Agent) *mockTool
		wantErr string
		assert  func(t *testing.T, res *RunResult, stub *mockTool)
	}{
		{
			name:  "default response trims input",
			ctx:   context.Background(),
			input: "   hello  ",
			assert: func(t *testing.T, res *RunResult, _ *mockTool) {
				t.Helper()
				if res.StopReason != "complete" {
					t.Fatalf("stop reason = %s", res.StopReason)
				}
				if res.Output != "session test-session: hello" {
					t.Fatalf("output = %s", res.Output)
				}
			},
		},
		{
			name:  "tool instruction executes registered tool",
			ctx:   context.Background(),
			input: "tool:echo {\"msg\":\"ok\"}",
			setup: func(t *testing.T, ag Agent) *mockTool {
				t.Helper()
				stub := &mockTool{name: "echo", result: &tool.ToolResult{Output: "pong"}}
				if err := ag.AddTool(stub); err != nil {
					t.Fatalf("add tool: %v", err)
				}
				return stub
			},
			assert: func(t *testing.T, res *RunResult, stub *mockTool) {
				t.Helper()
				if res.StopReason != "tool_call" {
					t.Fatalf("stop reason = %s", res.StopReason)
				}
				if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "echo" {
					t.Fatalf("tool calls = %+v", res.ToolCalls)
				}
				if stub.calls != 1 {
					t.Fatalf("tool executions = %d", stub.calls)
				}
				if got := res.ToolCalls[0].Output.(*tool.ToolResult).Output; got != "pong" {
					t.Fatalf("tool output = %v", got)
				}
			},
		},
		{
			name:    "malformed tool payload propagates error",
			ctx:     context.Background(),
			input:   "tool:echo {",
			wantErr: "parse tool params",
			assert: func(t *testing.T, res *RunResult, _ *mockTool) {
				t.Helper()
				if res.StopReason != "input_error" {
					t.Fatalf("stop reason = %s", res.StopReason)
				}
			},
		},
		{
			name:    "nil context rejected",
			ctx:     nil,
			input:   "hello",
			wantErr: "context is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag := newTestAgent(t)
			var stub *mockTool
			if tt.setup != nil {
				stub = tt.setup(t, ag)
			}
			res, err := ag.Run(tt.ctx, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, res, stub)
			}
		})
	}
}

func TestAgentRunStream(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		input   string
		wantErr string
		assert  func(t *testing.T, events []event.Event)
	}{
		{
			name:  "successful stream emits progress and completion",
			ctx:   context.Background(),
			input: "hi",
			assert: func(t *testing.T, events []event.Event) {
				t.Helper()
				if len(events) == 0 {
					t.Fatal("no events emitted")
				}
				if events[0].Type != event.EventProgress {
					t.Fatalf("first event = %s", events[0].Type)
				}
				if events[len(events)-1].Type != event.EventCompletion {
					t.Fatalf("last event = %s", events[len(events)-1].Type)
				}
			},
		},
		{
			name:    "invalid input rejected",
			ctx:     context.Background(),
			input:   "   ",
			wantErr: "input is empty",
		},
		{
			name:    "nil context rejected",
			ctx:     nil,
			input:   "hi",
			wantErr: "context is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag := newTestAgent(t)
			ch, err := ag.RunStream(tt.ctx, tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("run stream failed: %v", err)
			}
			var events []event.Event
			for evt := range ch {
				events = append(events, evt)
			}
			tt.assert(t, events)
		})
	}
}

func TestAgentAddTool(t *testing.T) {
	tests := []struct {
		name        string
		tool        tool.Tool
		preRegister bool
		wantErr     string
		verifyRun   bool
	}{
		{name: "nil tool", tool: nil, wantErr: "tool is nil"},
		{name: "empty name", tool: &mockTool{name: ""}, wantErr: "tool name is empty"},
		{name: "duplicate name", tool: &mockTool{name: "dup"}, preRegister: true, wantErr: "already registered"},
		{name: "success registers callable tool", tool: &mockTool{name: "echo", result: &tool.ToolResult{Output: "done"}}, verifyRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag := newTestAgent(t)
			if tt.preRegister {
				if err := ag.AddTool(tt.tool); err != nil {
					t.Fatalf("setup add failed: %v", err)
				}
			}
			err := ag.AddTool(tt.tool)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("add tool failed: %v", err)
			}
			if tt.verifyRun {
				_, runErr := ag.Run(context.Background(), "tool:echo {}")
				if runErr != nil {
					t.Fatalf("run failed: %v", runErr)
				}
				if stub, ok := tt.tool.(*mockTool); ok {
					if stub.calls != 1 {
						t.Fatalf("tool not invoked, calls=%d", stub.calls)
					}
				}
			}
		})
	}
}

func newTestAgent(t *testing.T) Agent {
	t.Helper()
	ag, err := New(Config{Name: "unit", DefaultContext: RunContext{SessionID: "test-session"}})
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	return ag
}

type mockTool struct {
	name    string
	schema  *tool.JSONSchema
	result  *tool.ToolResult
	err     error
	calls   int
	lastCtx context.Context
	params  map[string]any
}

func (m *mockTool) Name() string             { return strings.TrimSpace(m.name) }
func (m *mockTool) Description() string      { return "mock" }
func (m *mockTool) Schema() *tool.JSONSchema { return m.schema }

func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	m.calls++
	m.lastCtx = ctx
	m.params = map[string]any{}
	for k, v := range params {
		m.params[k] = v
	}
	if m.result == nil {
		m.result = &tool.ToolResult{}
	}
	return m.result, m.err
}
