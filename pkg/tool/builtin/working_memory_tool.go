package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cexll/agentsdk-go/pkg/memory"
	"github.com/cexll/agentsdk-go/pkg/tool"
)

// UpdateWorkingMemoryTool lets the model persist scoped short-term state.
type UpdateWorkingMemoryTool struct {
	store memory.WorkingMemoryStore
}

// NewUpdateWorkingMemoryTool constructs the tool with the provided store.
func NewUpdateWorkingMemoryTool(store memory.WorkingMemoryStore) *UpdateWorkingMemoryTool {
	return &UpdateWorkingMemoryTool{store: store}
}

func (t *UpdateWorkingMemoryTool) Name() string { return "update_working_memory" }

func (t *UpdateWorkingMemoryTool) Description() string {
	return "更新工作记忆，以跟踪任务上下文、临时变量与进度信息。"
}

// Schema describes the tool input contract expected from the LLM.
func (t *UpdateWorkingMemoryTool) Schema() *tool.JSONSchema {
	return &tool.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"thread_id": map[string]any{
				"type":        "string",
				"description": "线程 ID，默认为当前会话 ID。",
			},
			"resource_id": map[string]any{
				"type":        "string",
				"description": "可选资源 ID，用于同一线程下的子作用域。",
			},
			"data": map[string]any{
				"type":        "object",
				"description": "要合并到工作记忆的 JSON 对象。",
			},
			"ttl_seconds": map[string]any{
				"type":        "number",
				"description": "可选 TTL，单位秒，0 表示不过期。",
			},
		},
		Required: []string{"thread_id", "data"},
	}
}

// Execute merges the provided data into the scoped working memory record.
func (t *UpdateWorkingMemoryTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	if t == nil || t.store == nil {
		return nil, errors.New("working memory store is not configured")
	}
	if params == nil {
		return nil, errors.New("params cannot be nil")
	}
	threadID, err := parseThreadID(params)
	if err != nil {
		return nil, err
	}
	scope := memory.Scope{ThreadID: threadID, ResourceID: parseResourceID(params)}
	updates, err := parseData(params)
	if err != nil {
		return nil, err
	}
	ttl, err := parseTTL(params)
	if err != nil {
		return nil, err
	}

	wm, err := t.store.Get(ctx, scope)
	if err != nil {
		return nil, err
	}
	if wm == nil {
		wm = &memory.WorkingMemory{Data: map[string]any{}}
	}
	if wm.Data == nil {
		wm.Data = map[string]any{}
	}
	for k, v := range updates {
		wm.Data[k] = v
	}
	wm.TTL = ttl

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := t.store.Set(ctx, scope, wm); err != nil {
		return nil, err
	}

	return &tool.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("工作记忆已更新（thread_id=%s）", scope.ThreadID),
		Data: map[string]any{
			"thread_id":   scope.ThreadID,
			"resource_id": scope.ResourceID,
			"ttl_seconds": int64(ttl / time.Second),
		},
	}, nil
}

func parseThreadID(params map[string]interface{}) (string, error) {
	raw, ok := params["thread_id"]
	if !ok {
		return "", errors.New("thread_id is required")
	}
	value, err := coerceString(raw)
	if err != nil {
		return "", fmt.Errorf("thread_id must be string: %w", err)
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New("thread_id cannot be empty")
	}
	return trimmed, nil
}

func parseResourceID(params map[string]interface{}) string {
	raw, ok := params["resource_id"]
	if !ok || raw == nil {
		return ""
	}
	value, err := coerceString(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func parseData(params map[string]interface{}) (map[string]any, error) {
	raw, ok := params["data"]
	if !ok {
		return nil, errors.New("data is required")
	}
	data, ok := raw.(map[string]any)
	if !ok {
		if converted, ok := raw.(map[string]interface{}); ok {
			data = converted
		} else {
			return nil, errors.New("data must be an object")
		}
	}
	updates := make(map[string]any, len(data))
	for k, v := range data {
		updates[k] = v
	}
	return updates, nil
}

func parseTTL(params map[string]interface{}) (time.Duration, error) {
	raw, ok := params["ttl_seconds"]
	if !ok {
		return 0, nil
	}
	dur, err := durationFromParam(raw)
	if err != nil {
		return 0, fmt.Errorf("ttl_seconds invalid: %w", err)
	}
	return dur, nil
}
