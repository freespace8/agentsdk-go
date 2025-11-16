package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/cexll/agentsdk-go/pkg/tool/mcp"
)

// Registry keeps the mapping between tool names and implementations.
type Registry struct {
	mu        sync.RWMutex
	tools     map[string]Tool
	mcpClient *mcp.Client
	validator Validator
}

// NewRegistry creates a registry backed by the default validator.
func NewRegistry() *Registry {
	return &Registry{
		tools:     make(map[string]Tool),
		validator: DefaultValidator{},
	}
}

// Register inserts a tool when its name is not in use.
func (r *Registry) Register(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("tool is nil")
	}
	name := tool.Name()
	if name == "" {
		return fmt.Errorf("tool name is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %s already registered", name)
	}

	r.tools[name] = tool
	return nil
}

// Get fetches a tool by name.
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool %s not found", name)
	}
	return tool, nil
}

// List produces a snapshot of all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// SetValidator swaps the validator instance used before execution.
func (r *Registry) SetValidator(v Validator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validator = v
}

// Execute runs a registered tool after optional schema validation.
func (r *Registry) Execute(ctx context.Context, name string, params map[string]interface{}) (*ToolResult, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	if schema := tool.Schema(); schema != nil {
		r.mu.RLock()
		validator := r.validator
		r.mu.RUnlock()

		if validator != nil {
			if err := validator.Validate(params, schema); err != nil {
				return nil, fmt.Errorf("tool %s validation failed: %w", name, err)
			}
		}
	}

	return tool.Execute(ctx, params)
}
