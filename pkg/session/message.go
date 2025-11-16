package session

import "time"

// ToolCall captures an assistant-triggered tool invocation that should be replayable.
type ToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Output    any            `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
}

// Message represents a single conversational turn persisted in a session.
type Message struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Timestamp time.Time  `json:"timestamp"`
}

// Filter constrains the message subset returned by Session.List.
type Filter struct {
	StartTime *time.Time
	EndTime   *time.Time
	Role      string
	Limit     int
	Offset    int
}
