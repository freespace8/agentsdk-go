package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cexll/agentsdk-go/pkg/tool"
)

const todoWriteDescription = `Use this tool to create and manage a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.
It also helps the user understand the progress of the task and overall progress of their requests.

## When to Use This Tool
Use this tool proactively in these scenarios:

1. Complex multi-step tasks - When a task requires 3 or more distinct steps or actions
2. Non-trivial and complex tasks - Tasks that require careful planning or multiple operations
3. User explicitly requests todo list - When the user directly asks you to use the todo list
4. User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)
5. After receiving new instructions - Immediately capture user requirements as todos
6. When you start working on a task - Mark it as in_progress BEFORE beginning work. Ideally you should only have one todo as in_progress at a time
7. After completing a task - Mark it as completed and add any new follow-up tasks discovered during implementation

## When NOT to Use This Tool

Skip using this tool when:
1. There is only a single, straightforward task
2. The task is trivial and tracking it provides no organizational benefit
3. The task can be completed in less than 3 trivial steps
4. The task is purely conversational or informational

NOTE that you should not use this tool if there is only one trivial task to do. In this case you are better off just doing the task directly.

## Examples of When to Use the Todo List

<example>
User: I want to add a dark mode toggle to the application settings. Make sure you run the tests and build when you're done!
Assistant: I'll help add a dark mode toggle to your application settings. Let me create a todo list to track this implementation.
*Creates todo list with the following items:*
1. Creating dark mode toggle component in Settings page
2. Adding dark mode state management (context/store)
3. Implementing CSS-in-JS styles for dark theme
4. Updating existing components to support theme switching
5. Running tests and build process, addressing any failures or errors that occur
*Begins working on the first task*

<reasoning>
The assistant used the todo list because:
1. Adding dark mode is a multi-step feature requiring UI, state management, and styling changes
2. The user explicitly requested tests and build be run afterward
3. The assistant inferred that tests and build need to pass by adding "Ensure tests and build succeed" as the final task
</reasoning>
</example>

<example>
User: Help me rename the function getCwd to getCurrentWorkingDirectory across my project
Assistant: Let me first search through your codebase to find all occurrences of 'getCwd'.
*Uses grep or search tools to locate all instances of getCwd in the codebase*
Assistant: I've found 15 instances of 'getCwd' across 8 different files. Let me create a todo list to track these changes.
*Creates todo list with specific items for each file that needs updating*

<reasoning>
The assistant used the todo list because:
1. First, the assistant searched to understand the scope of the task
2. Upon finding multiple occurrences across different files, it determined this was a complex task with multiple steps
3. The todo list helps ensure every instance is tracked and updated systematically
4. This approach prevents missing any occurrences and maintains code consistency
</reasoning>
</example>


<example>
User: I need to implement these features for my e-commerce site: user registration, product catalog, shopping cart, and checkout flow.
Assistant: I'll help implement these features. First, let's add all the features to the todo list.
*Creates a todo list breaking down each feature into specific tasks based on the project architecture*
Assistant: Let's start with user registration. This will involve creating database models, API endpoints, and frontend forms that integrate with your existing authentication system.

<reasoning>
The assistant used the todo list because:
1. The user provided multiple complex features to implement in a comma separated list
2. The todo list helps organize these large features into manageable tasks
3. This approach allows for tracking progress across the entire implementation
</reasoning>
</example>

<example>
User: Can you help optimize my React application? It's rendering slowly and has performance issues.</user>
Assistant: I'll help optimize your React application. First, let me examine your codebase to identify potential performance bottlenecks.
*Reviews component structure, render patterns, state management, and data fetching*
Assistant: After analyzing your codebase, I've identified several performance issues. Let me create a todo list to track our optimization efforts.
*Creates todo list with items like: 1) Implementing memoization for expensive calculations in derived components, 2) Splitting large bundles, 3) Adding Suspense boundaries for slow data fetches*

<reasoning>
The assistant used the todo list because:
1. The user described a complex, multi-faceted optimization effort
2. The todo list helps organize the investigation and fixes into discrete steps
3. Progress can be communicated back to the user step-by-step
</reasoning>
</example>
`

var todoWriteSchema = &tool.JSONSchema{
	Type: "object",
	Properties: map[string]interface{}{
		"todos": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{
						"type":      "string",
						"minLength": 1,
					},
					"status": map[string]interface{}{
						"type": "string",
						"enum": []string{
							"pending",
							"in_progress",
							"completed",
						},
					},
					"activeForm": map[string]interface{}{
						"type":      "string",
						"minLength": 1,
					},
				},
				"required": []string{"content", "status", "activeForm"},
			},
			"description": "The updated todo list",
		},
	},
	Required: []string{"todos"},
}

// TodoItem represents a single todo entry exposed in tool responses.
type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
}

// TodoWriteTool provides in-memory todo list management.
type TodoWriteTool struct {
	mu       sync.RWMutex
	todos    []TodoItem
	revision uint64
}

// NewTodoWriteTool initialises the todo write tool.
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

func (t *TodoWriteTool) Name() string { return "TodoWrite" }

func (t *TodoWriteTool) Description() string { return todoWriteDescription }

func (t *TodoWriteTool) Schema() *tool.JSONSchema { return todoWriteSchema }

func (t *TodoWriteTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	items, err := parseTodoItems(params)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	snapshot, revision := t.store(items)
	counts := countByStatus(snapshot)
	output := formatTodoOutput(snapshot, counts)

	data := map[string]interface{}{
		"todos":    snapshot,
		"counts":   counts,
		"revision": revision,
		"total":    len(snapshot),
	}

	return &tool.ToolResult{
		Success: true,
		Output:  output,
		Data:    data,
	}, nil
}

// Snapshot exposes a copy of the current list (used in tests).
func (t *TodoWriteTool) Snapshot() []TodoItem {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return cloneTodos(t.todos)
}

func (t *TodoWriteTool) store(items []TodoItem) ([]TodoItem, uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.todos = cloneTodos(items)
	t.revision++
	return cloneTodos(t.todos), t.revision
}

func parseTodoItems(params map[string]interface{}) ([]TodoItem, error) {
	if params == nil {
		return nil, errors.New("params is nil")
	}
	raw, ok := params["todos"]
	if !ok {
		return nil, errors.New("todos is required")
	}
	list, ok := raw.([]interface{})
	if !ok {
		if arr, ok := raw.([]map[string]interface{}); ok {
			list = make([]interface{}, len(arr))
			for i := range arr {
				list[i] = arr[i]
			}
		} else {
			return nil, fmt.Errorf("todos must be an array, got %T", raw)
		}
	}
	items := make([]TodoItem, len(list))
	for i, entry := range list {
		obj, ok := entry.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("todos[%d] must be object, got %T", i, entry)
		}
		content, err := readRequiredString(obj, "content")
		if err != nil {
			return nil, fmt.Errorf("todos[%d].content: %w", i, err)
		}
		status, err := readRequiredString(obj, "status")
		if err != nil {
			return nil, fmt.Errorf("todos[%d].status: %w", i, err)
		}
		activeForm, err := readRequiredString(obj, "activeForm")
		if err != nil {
			return nil, fmt.Errorf("todos[%d].activeForm: %w", i, err)
		}
		status = normalizeStatus(status)
		if status == "" {
			return nil, fmt.Errorf("todos[%d].status must be one of pending, in_progress, completed", i)
		}
		items[i] = TodoItem{
			Content:    content,
			Status:     status,
			ActiveForm: activeForm,
		}
	}
	return items, nil
}

func readRequiredString(obj map[string]interface{}, key string) (string, error) {
	raw, ok := obj[key]
	if !ok {
		return "", errors.New("field is required")
	}
	value, err := coerceString(raw)
	if err != nil {
		return "", fmt.Errorf("must be string: %w", err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("cannot be empty")
	}
	return value, nil
}

func normalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending":
		return "pending"
	case "in_progress", "in-progress":
		return "in_progress"
	case "completed", "complete", "done":
		return "completed"
	default:
		return ""
	}
}

func cloneTodos(items []TodoItem) []TodoItem {
	if len(items) == 0 {
		return nil
	}
	dup := make([]TodoItem, len(items))
	copy(dup, items)
	return dup
}

func countByStatus(items []TodoItem) map[string]int {
	counts := map[string]int{
		"pending":     0,
		"in_progress": 0,
		"completed":   0,
	}
	for _, item := range items {
		counts[item.Status]++
	}
	return counts
}

func formatTodoOutput(items []TodoItem, counts map[string]int) string {
	if len(items) == 0 {
		return "todo list cleared"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d todos (pending:%d, in_progress:%d, completed:%d)\n", len(items), counts["pending"], counts["in_progress"], counts["completed"])
	for idx, item := range items {
		fmt.Fprintf(&b, "%d. [%s] %s", idx+1, item.Status, item.Content)
		if strings.TrimSpace(item.ActiveForm) != "" {
			fmt.Fprintf(&b, " (form=%s)", item.ActiveForm)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// OrderedTodoContent exposes content strings in stable order (used by tests).
func OrderedTodoContent(items []TodoItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Content)
	}
	sort.Strings(out)
	return out
}
