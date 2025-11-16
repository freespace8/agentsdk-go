package toolbuiltin

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cexll/agentsdk-go/pkg/security"
	"github.com/cexll/agentsdk-go/pkg/tool"
)

const (
	defaultFileDescription = "Perform safe file read/write/delete operations within the workspace."
	defaultMaxFileBytes    = 1 << 20 // 1 MiB
)

var fileSchema = &tool.JSONSchema{
	Type: "object",
	Properties: map[string]interface{}{
		"operation": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"read", "write", "delete"},
			"description": "Operation to perform: read, write, or delete.",
		},
		"path": map[string]interface{}{
			"type":        "string",
			"description": "Path relative to the sandbox root.",
		},
		"content": map[string]interface{}{
			"type":        "string",
			"description": "File contents used when operation is write.",
		},
	},
	Required: []string{"operation", "path"},
}

// FileTool exposes safe file read/write helpers.
type FileTool struct {
	sandbox  *security.Sandbox
	root     string
	maxBytes int64
}

// NewFileTool constructs a FileTool rooted at the current directory.
func NewFileTool() *FileTool {
	return NewFileToolWithRoot("")
}

// NewFileToolWithRoot constructs a FileTool rooted at the provided directory.
func NewFileToolWithRoot(root string) *FileTool {
	resolved := resolveRoot(root)
	return &FileTool{
		sandbox:  security.NewSandbox(resolved),
		root:     resolved,
		maxBytes: defaultMaxFileBytes,
	}
}

func (f *FileTool) Name() string { return "file_operation" }

func (f *FileTool) Description() string { return defaultFileDescription }

func (f *FileTool) Schema() *tool.JSONSchema { return fileSchema }

func (f *FileTool) Execute(ctx context.Context, params map[string]interface{}) (*tool.ToolResult, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	if f == nil || f.sandbox == nil {
		return nil, errors.New("file tool is not initialised")
	}
	op, err := parseOperation(params)
	if err != nil {
		return nil, err
	}
	target, err := f.resolvePath(params)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	switch op {
	case "read":
		return f.readFile(target)
	case "write":
		return f.writeFile(target, params)
	case "delete":
		return f.deleteFile(target)
	default:
		return nil, fmt.Errorf("unsupported operation %s", op)
	}
}

func parseOperation(params map[string]interface{}) (string, error) {
	if params == nil {
		return "", errors.New("params is nil")
	}
	raw, ok := params["operation"]
	if !ok {
		return "", errors.New("operation is required")
	}
	op, err := coerceString(raw)
	if err != nil {
		return "", fmt.Errorf("operation must be string: %w", err)
	}
	op = strings.ToLower(strings.TrimSpace(op))
	switch op {
	case "read", "write", "delete":
		return op, nil
	default:
		return "", fmt.Errorf("operation %q is not supported", op)
	}
}

func (f *FileTool) resolvePath(params map[string]interface{}) (string, error) {
	raw, ok := params["path"]
	if !ok {
		return "", errors.New("path is required")
	}
	pathStr, err := coerceString(raw)
	if err != nil {
		return "", fmt.Errorf("path must be string: %w", err)
	}
	trimmed := strings.TrimSpace(pathStr)
	if trimmed == "" {
		return "", errors.New("path cannot be empty")
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(f.root, candidate)
	}
	candidate = filepath.Clean(candidate)
	if err := f.sandbox.ValidatePath(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

func (f *FileTool) readFile(path string) (*tool.ToolResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}
	if f.maxBytes > 0 && info.Size() > f.maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes limit", f.maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if f.maxBytes > 0 && int64(len(data)) > f.maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes limit", f.maxBytes)
	}
	return &tool.ToolResult{
		Success: true,
		Output:  string(data),
		Data: map[string]interface{}{
			"operation": "read",
			"path":      path,
			"size":      len(data),
		},
	}, nil
}

func (f *FileTool) writeFile(path string, params map[string]interface{}) (*tool.ToolResult, error) {
	raw, ok := params["content"]
	if !ok {
		return nil, errors.New("content is required for write")
	}
	content, err := coerceString(raw)
	if err != nil {
		return nil, fmt.Errorf("content must be string: %w", err)
	}
	data := []byte(content)
	if f.maxBytes > 0 && int64(len(data)) > f.maxBytes {
		return nil, fmt.Errorf("content exceeds %d bytes limit", f.maxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("ensure directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return &tool.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("wrote %d bytes", len(data)),
		Data: map[string]interface{}{
			"operation": "write",
			"path":      path,
			"size":      len(data),
		},
	}, nil
}

func (f *FileTool) deleteFile(path string) (*tool.ToolResult, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("file does not exist: %s", path)
		}
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("refusing to delete directory %s", path)
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("delete file: %w", err)
	}
	return &tool.ToolResult{
		Success: true,
		Output:  "file deleted",
		Data: map[string]interface{}{
			"operation": "delete",
			"path":      path,
		},
	}, nil
}
