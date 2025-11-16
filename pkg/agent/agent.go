package agent

import (
	"context"

	"github.com/cexll/agentsdk-go/pkg/event"
	"github.com/cexll/agentsdk-go/pkg/tool"
)

// Agent exposes the minimal runtime surface required by callers.
type Agent interface {
	// Run executes a single turn interaction until completion.
	Run(ctx context.Context, input string) (*RunResult, error)

	// RunStream executes a turn and streams progress events.
	RunStream(ctx context.Context, input string) (<-chan event.Event, error)

	// AddTool registers a tool that can be invoked during execution.
	AddTool(tool tool.Tool) error

	// WithHook returns a shallow copy of the agent with an extra hook.
	WithHook(hook Hook) Agent
}

// Hook allows callers to intercept important lifecycle moments.
type Hook interface {
	PreRun(ctx context.Context, input string) error
	PostRun(ctx context.Context, result *RunResult) error
	PreToolCall(ctx context.Context, toolName string, params map[string]any) error
	PostToolCall(ctx context.Context, toolName string, call ToolCall) error
}

// NopHook offers a convenient zero-cost implementation for optional methods.
type NopHook struct{}

func (NopHook) PreRun(context.Context, string) error                      { return nil }
func (NopHook) PostRun(context.Context, *RunResult) error                 { return nil }
func (NopHook) PreToolCall(context.Context, string, map[string]any) error { return nil }
func (NopHook) PostToolCall(context.Context, string, ToolCall) error      { return nil }
