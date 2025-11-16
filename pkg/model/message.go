package model

// Message represents a single conversational turn exchanged with a model.
type Message struct {
	Role      string
	Content   string
	ToolCalls []ToolCall
}

// ToolCall captures a tool invocation emitted by assistant messages.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}
