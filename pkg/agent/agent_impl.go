package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cexll/agentsdk-go/pkg/event"
	"github.com/cexll/agentsdk-go/pkg/tool"
)

// New constructs the default Agent implementation backed by basicAgent.
func New(cfg Config) (Agent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &basicAgent{
		cfg:   cfg,
		tools: map[string]tool.Tool{},
	}, nil
}

type basicAgent struct {
	cfg    Config
	hooks  []Hook
	tools  map[string]tool.Tool
	toolMu sync.RWMutex
}

func (a *basicAgent) Run(ctx context.Context, input string) (*RunResult, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	sanitized, err := sanitizeInput(input)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	override, _ := GetRunContext(ctx)
	runCtx := a.cfg.ResolveContext(override)
	if runCtx.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runCtx.Timeout)
		defer cancel()
	}

	result := &RunResult{
		StopReason: "complete",
		Events: []event.Event{
			progressEvent(runCtx.SessionID, "accepted", "input accepted", nil),
		},
	}

	if err := runHooks(a.hooks, false, func(h Hook) error {
		return h.PreRun(ctx, sanitized)
	}); err != nil {
		return nil, err
	}

	name, params, wantsTool, parseErr := parseToolInstruction(sanitized)
	if parseErr != nil {
		result.StopReason = "input_error"
		result.Events = append(result.Events, errorEvent(runCtx.SessionID, "input", parseErr, false))
		if err := a.runPostHooks(ctx, result); err != nil {
			parseErr = errors.Join(parseErr, err)
		}
		return result, parseErr
	}

	if wantsTool {
		result.Events = append(result.Events, event.NewEvent(
			event.EventToolCall,
			runCtx.SessionID,
			event.ToolCallData{
				Name:   name,
				Params: maps.Clone(params),
			},
		))
		call, toolErr := a.executeTool(ctx, name, params)
		result.ToolCalls = append(result.ToolCalls, call)
		result.Events = append(result.Events, event.NewEvent(
			event.EventToolResult,
			runCtx.SessionID,
			event.ToolResultData{
				Name:     call.Name,
				Output:   call.Output,
				Error:    call.Error,
				Duration: call.Duration,
			},
		))
		if toolErr != nil {
			result.StopReason = "tool_error"
			result.Events = append(result.Events, errorEvent(runCtx.SessionID, "tool", toolErr, false))
			if err := a.runPostHooks(ctx, result); err != nil {
				toolErr = errors.Join(toolErr, err)
			}
			return result, toolErr
		}
		result.Output = stringify(call.Output)
		result.StopReason = "tool_call"
	} else {
		result.Output = a.defaultResponse(sanitized, runCtx)
	}

	result.Usage = estimateUsage(sanitized, result.Output)
	result.Events = append(result.Events,
		progressEvent(runCtx.SessionID, "completed", "run completed", map[string]any{
			"stop_reason": result.StopReason,
		}),
		event.NewEvent(event.EventCompletion, runCtx.SessionID, completionSummary(result)),
	)

	if err := a.runPostHooks(ctx, result); err != nil {
		return result, err
	}
	return result, nil
}

func (a *basicAgent) RunStream(ctx context.Context, input string) (<-chan event.Event, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	if _, err := sanitizeInput(input); err != nil {
		return nil, err
	}
	override, _ := GetRunContext(ctx)
	runCtx := a.cfg.ResolveContext(override)
	sessionID := runCtx.SessionID

	ch := make(chan event.Event, a.cfg.streamBuffer())
	go func() {
		defer close(ch)
		if !emitEvent(ctx, ch, progressEvent(sessionID, "started", "stream started", nil)) {
			return
		}
		res, err := a.Run(ctx, input)
		if err != nil {
			emitEvent(ctx, ch, errorEvent(sessionID, "run", err, errors.Is(err, context.Canceled)))
			return
		}
		for _, evt := range res.Events {
			if !emitEvent(ctx, ch, evt) {
				return
			}
		}
	}()

	return ch, nil
}

func (a *basicAgent) AddTool(t tool.Tool) error {
	if t == nil {
		return errors.New("tool is nil")
	}
	name := strings.TrimSpace(t.Name())
	if name == "" {
		return errors.New("tool name is empty")
	}
	a.toolMu.Lock()
	defer a.toolMu.Unlock()
	if a.tools == nil {
		a.tools = map[string]tool.Tool{}
	}
	if _, exists := a.tools[name]; exists {
		return fmt.Errorf("tool %s already registered", name)
	}
	a.tools[name] = t
	return nil
}

func (a *basicAgent) WithHook(h Hook) Agent {
	if h == nil {
		return a
	}
	clone := *a
	clone.hooks = append(append([]Hook(nil), a.hooks...), h)
	return &clone
}

func (a *basicAgent) executeTool(ctx context.Context, name string, params map[string]any) (ToolCall, error) {
	call := ToolCall{Name: name, Params: maps.Clone(params)}
	if call.Params == nil {
		call.Params = map[string]any{}
	}
	a.toolMu.RLock()
	impl := a.tools[name]
	a.toolMu.RUnlock()
	if impl == nil {
		return call, fmt.Errorf("tool %s not registered", name)
	}
	if err := runHooks(a.hooks, false, func(h Hook) error {
		return h.PreToolCall(ctx, name, call.Params)
	}); err != nil {
		return call, err
	}
	started := time.Now()
	output, err := impl.Execute(ctx, call.Params)
	call.Duration = time.Since(started)
	call.Output = output
	if err != nil {
		call.Error = err.Error()
	}
	if hookErr := a.invokePostToolHooks(ctx, name, call); hookErr != nil {
		err = errors.Join(err, hookErr)
		if call.Error == "" {
			call.Error = hookErr.Error()
		}
	}
	return call, err
}

func (a *basicAgent) runPostHooks(ctx context.Context, result *RunResult) error {
	return runHooks(a.hooks, true, func(h Hook) error {
		return h.PostRun(ctx, result)
	})
}

func (a *basicAgent) invokePostToolHooks(ctx context.Context, name string, call ToolCall) error {
	return runHooks(a.hooks, true, func(h Hook) error {
		return h.PostToolCall(ctx, name, call)
	})
}

func (a *basicAgent) defaultResponse(input string, rc RunContext) string {
	if rc.SessionID != "" {
		return fmt.Sprintf("session %s: %s", rc.SessionID, input)
	}
	return fmt.Sprintf("processed: %s", input)
}

func sanitizeInput(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", errors.New("input is empty")
	}
	return trimmed, nil
}

func estimateUsage(input, output string) TokenUsage {
	in := utf8.RuneCountInString(input)
	out := utf8.RuneCountInString(output)
	return TokenUsage{
		InputTokens:  in,
		OutputTokens: out,
		TotalTokens:  in + out,
	}
}

func parseToolInstruction(input string) (string, map[string]any, bool, error) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "tool:") {
		return "", nil, false, nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "tool:"))
	if payload == "" {
		return "", nil, false, errors.New("missing tool name")
	}
	parts := strings.SplitN(payload, " ", 2)
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return "", nil, false, errors.New("tool name is empty")
	}
	params := map[string]any{}
	if len(parts) == 2 {
		raw := strings.TrimSpace(parts[1])
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &params); err != nil {
				return "", nil, false, fmt.Errorf("parse tool params: %w", err)
			}
		}
	}
	return name, params, true, nil
}

func emitEvent(ctx context.Context, ch chan<- event.Event, evt event.Event) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- evt:
		return true
	}
}

func progressEvent(sessionID, stage, message string, details map[string]any) event.Event {
	return event.NewEvent(event.EventProgress, sessionID, event.ProgressData{
		Stage:   stage,
		Message: message,
		Details: maps.Clone(details),
	})
}

func errorEvent(sessionID, kind string, err error, recoverable bool) event.Event {
	if err == nil {
		err = errors.New("unknown error")
	}
	return event.NewEvent(event.EventError, sessionID, event.ErrorData{
		Message:     err.Error(),
		Kind:        kind,
		Recoverable: recoverable,
	})
}

func completionSummary(res *RunResult) event.CompletionData {
	summary := event.CompletionData{
		Output:     res.Output,
		StopReason: res.StopReason,
	}
	if usage := convertUsage(res.Usage); usage != nil {
		summary.Usage = usage
	}
	if len(res.ToolCalls) > 0 {
		summary.ToolCalls = convertToolCalls(res.ToolCalls)
	}
	return summary
}

func convertToolCalls(calls []ToolCall) []event.ToolCallData {
	if len(calls) == 0 {
		return nil
	}
	data := make([]event.ToolCallData, 0, len(calls))
	for _, call := range calls {
		data = append(data, event.ToolCallData{
			Name:   call.Name,
			Params: maps.Clone(call.Params),
		})
	}
	return data
}

func convertUsage(u TokenUsage) *event.UsageData {
	if u == (TokenUsage{}) {
		return nil
	}
	return &event.UsageData{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
		CacheTokens:  u.CacheTokens,
	}
}

func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	case []byte:
		return string(val)
	default:
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		return string(data)
	}
}

func runHooks(hooks []Hook, collect bool, fn func(Hook) error) error {
	var joined error
	for _, hook := range hooks {
		if err := fn(hook); err != nil {
			if !collect {
				return err
			}
			joined = errors.Join(joined, err)
		}
	}
	return joined
}
