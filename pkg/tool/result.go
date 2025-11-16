package tool

// ToolResult captures the outcome of a tool invocation.
type ToolResult struct {
	Success bool
	Output  string
	Data    interface{}
	Error   error
}
