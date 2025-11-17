package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cexll/agentsdk-go/pkg/agent"
	"github.com/cexll/agentsdk-go/pkg/event"
	"github.com/cexll/agentsdk-go/pkg/model"
	"github.com/cexll/agentsdk-go/pkg/model/anthropic"
	"github.com/cexll/agentsdk-go/pkg/session"
	toolbuiltin "github.com/cexll/agentsdk-go/pkg/tool/builtin"
)

const (
	defaultAnthropicModel = "claude-3-5-sonnet-20241022"
	defaultMaxTokens      = 4096

	baseToolSystemPrompt = "" +
		"你是 agentsdk-integration 验证员，必须严格按照用户提供的步骤执行任务。" +
		"当需要运行 shell 命令时，只能调用 bash_execute 并将 command 参数设置为用户给出的内容；" +
		"当需要读写文件时，只能调用 file_operation 且路径必须使用用户给出的绝对路径。" +
		"所有结论都要基于工具返回的真实输出，完成全部步骤后再用简洁中文总结。"
)

var (
	anthropicAPIKey     string
	anthropicModelName  string
	integrationRepoRoot string
)

func TestAgenticWhileLoopIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip agentic integration test when -short is set")
	}

	anthropicAPIKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if anthropicAPIKey == "" {
		t.Skip("ANTHROPIC_API_KEY is not configured; skipping agentic integration tests")
	}
	anthropicModelName = strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	if anthropicModelName == "" {
		anthropicModelName = defaultAnthropicModel
	}
	integrationRepoRoot = locateRepoRoot(t)
	goModPath := filepath.ToSlash(filepath.Join(integrationRepoRoot, "go.mod"))
	tmpFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("agentsdk-loop-%d.txt", time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(tmpFilePath) })
	tmpFileForPrompt := filepath.ToSlash(tmpFilePath)

	t.Run("multi-turn conversation", func(t *testing.T) {
		prompt := fmt.Sprintf("请依次完成下列操作:\n1. 使用 bash_execute 将 command 设为 `echo Hello`。\n2. 接着调用 file_operation 读取 %s 并告诉我 go.mod 声明的 module 名称。\n完成全部步骤后再总结。", goModPath)
		res, sess := runAgenticConversation(t, "multi-turn", baseToolSystemPrompt+" 不允许跳过任何工具调用。", prompt)
		if res.StopReason != "complete" {
			t.Fatalf("expected stop reason complete, got %s", res.StopReason)
		}
		bashIdx := toolCallIndex(res.ToolCalls, "bash_execute")
		fileIdx := toolCallIndex(res.ToolCalls, "file_operation")
		if bashIdx == -1 || fileIdx == -1 {
			t.Fatalf("expected both bash_execute and file_operation tool calls, got %+v", res.ToolCalls)
		}
		if bashIdx >= fileIdx {
			t.Fatalf("bash_execute should run before file_operation: %+v", res.ToolCalls)
		}
		if countProgressStage(res.Events, "iteration_start") < 2 {
			t.Fatalf("expected at least two agentic iterations, got events=%+v", res.Events)
		}
		bashCall, _ := findToolCall(res.ToolCalls, "bash_execute")
		if cmd := fmt.Sprint(bashCall.Params["command"]); !strings.Contains(cmd, "echo Hello") {
			t.Fatalf("bash command mismatch: %s", cmd)
		}
		fileCall, _ := findToolCall(res.ToolCalls, "file_operation")
		if op := strings.TrimSpace(fmt.Sprint(fileCall.Params["operation"])); op != "read" {
			t.Fatalf("expected file_operation to read go.mod, got operation=%s", op)
		}
		if path := fmt.Sprint(fileCall.Params["path"]); !samePath(path, goModPath) {
			t.Fatalf("file_operation path mismatch: got %s want %s", path, goModPath)
		}
		if !strings.Contains(res.Output, "github.com/cexll/agentsdk-go") {
			t.Fatalf("final output missing module name: %s", res.Output)
		}
		if sess == nil {
			t.Fatal("memory session must be attached")
		}
	})

	t.Run("tool chain write-read", func(t *testing.T) {
		command := fmt.Sprintf("python3 -c 'open(\"%s\",\"w\").write(\"test content\")'", tmpFileForPrompt)
		prompt := fmt.Sprintf("先调用 bash_execute 执行命令: %s。\n然后调用 file_operation 读取 %s (operation=read)。\n完成后请确认内容就是 test content。", command, tmpFileForPrompt)
		res, _ := runAgenticConversation(t, "tool-chain", baseToolSystemPrompt+" 所有路径已经是绝对路径。", prompt)
		if len(res.ToolCalls) < 2 {
			t.Fatalf("expected at least two tool calls, got %d", len(res.ToolCalls))
		}
		bashCall, ok := findToolCall(res.ToolCalls, "bash_execute")
		if !ok {
			t.Fatalf("bash_execute call missing: %+v", res.ToolCalls)
		}
		if got := fmt.Sprint(bashCall.Params["command"]); got != command {
			t.Fatalf("bash command mismatch: got %s want %s", got, command)
		}
		fileCall, ok := findToolCall(res.ToolCalls, "file_operation")
		if !ok {
			t.Fatalf("file_operation call missing: %+v", res.ToolCalls)
		}
		if op := strings.TrimSpace(fmt.Sprint(fileCall.Params["operation"])); op != "read" {
			t.Fatalf("file_operation must read temp file, got %s", op)
		}
		if path := fmt.Sprint(fileCall.Params["path"]); !samePath(path, tmpFileForPrompt) {
			t.Fatalf("file_operation path mismatch: got %s want %s", path, tmpFileForPrompt)
		}
		data, err := os.ReadFile(tmpFilePath)
		if err != nil {
			t.Fatalf("read temp file: %v", err)
		}
		if string(data) != "test content" {
			t.Fatalf("temp file content mismatch: %q", data)
		}
		if !strings.Contains(res.Output, "test content") {
			t.Fatalf("final output missing verification: %s", res.Output)
		}
	})

	t.Run("stop condition without tools", func(t *testing.T) {
		prompt := "简单回答：1+1等于几？只需要直接给出数字，不要做任何工具调用。"
		res, _ := runAgenticConversation(t, "stop-condition", baseToolSystemPrompt+" 如果问题只是心算题，直接回答不要调用工具。", prompt)
		if len(res.ToolCalls) != 0 {
			t.Fatalf("expected zero tool calls, got %d", len(res.ToolCalls))
		}
		if res.StopReason != "complete" {
			t.Fatalf("stop reason mismatch, got %s", res.StopReason)
		}
		if !strings.Contains(res.Output, "2") {
			t.Fatalf("expected numeric answer, got %s", res.Output)
		}
	})

	t.Run("session persistence", func(t *testing.T) {
		prompt := fmt.Sprintf("先运行 bash_execute: `echo session-check`，然后用 file_operation 读取 %s。最后总结观察结果。", goModPath)
		res, sess := runAgenticConversation(t, "session-persistence", baseToolSystemPrompt, prompt)
		if sess == nil {
			t.Fatal("memory session missing")
		}
		assertSessionTranscript(t, sess, prompt, res.ToolCalls)
	})
}

func runAgenticConversation(t *testing.T, sessionTag, systemPrompt, userPrompt string) (*agent.RunResult, *session.MemorySession) {
	t.Helper()
	ag, sess := newAgenticTestAgent(t, sessionTag, systemPrompt)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := ag.Run(ctx, userPrompt)
	if err != nil {
		t.Fatalf("agent run failed: %v", err)
	}
	if res == nil {
		t.Fatal("agent returned nil result")
	}
	return res, sess
}

func newAgenticTestAgent(t *testing.T, sessionTag, systemPrompt string) (agent.Agent, *session.MemorySession) {
	t.Helper()
	sessionID := fmt.Sprintf("agentic-%s-%d", sanitizeSessionTag(sessionTag), time.Now().UnixNano())
	sess, err := session.NewMemorySession(sessionID)
	if err != nil {
		t.Fatalf("memory session: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	model := buildAnthropicModel(systemPrompt)
	cfg := agent.Config{
		Name:        "agentic-loop-integration",
		Description: "Validates agentic loop with Anthropic SDKModel",
		DefaultContext: agent.RunContext{
			SessionID:     sessionID,
			WorkDir:       integrationRepoRoot,
			MaxIterations: 8,
		},
	}
	ag, err := agent.New(cfg, agent.WithModel(model), agent.WithSession(sess))
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	if err := ag.AddTool(toolbuiltin.NewBashToolWithRoot(integrationRepoRoot)); err != nil {
		t.Fatalf("add bash tool: %v", err)
	}
	if err := ag.AddTool(toolbuiltin.NewFileToolWithRoot("/")); err != nil {
		t.Fatalf("add file tool: %v", err)
	}
	return ag, sess
}

func buildAnthropicModel(systemPrompt string) model.Model {
	m := anthropic.NewSDKModel(anthropicAPIKey, anthropicModelName, anthropicMaxTokens())
	if trimmed := strings.TrimSpace(systemPrompt); trimmed != "" {
		m.SetSystem(trimmed)
	}
	return m
}

func locateRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	orig := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod from %s", orig)
		}
		dir = parent
	}
}

func sanitizeSessionTag(tag string) string {
	if tag == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(tag) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return strings.Trim(b.String(), "-")
}

func countProgressStage(events []event.Event, stage string) int {
	count := 0
	for _, evt := range events {
		if evt.Type != event.EventProgress {
			continue
		}
		if data, ok := evt.Data.(event.ProgressData); ok && data.Stage == stage {
			count++
		}
	}
	return count
}

func findToolCall(calls []agent.ToolCall, name string) (agent.ToolCall, bool) {
	for _, call := range calls {
		if call.Name == name {
			return call, true
		}
	}
	return agent.ToolCall{}, false
}

func toolCallIndex(calls []agent.ToolCall, name string) int {
	for idx, call := range calls {
		if call.Name == name {
			return idx
		}
	}
	return -1
}

func samePath(a, b string) bool {
	return filepath.Clean(filepath.FromSlash(a)) == filepath.Clean(filepath.FromSlash(b))
}

func assertSessionTranscript(t *testing.T, sess *session.MemorySession, prompt string, calls []agent.ToolCall) {
	t.Helper()
	msgs, err := sess.List(session.Filter{})
	if err != nil {
		t.Fatalf("session list: %v", err)
	}
	if len(msgs) < 4 {
		t.Fatalf("unexpectedly short session transcript: %+v", msgs)
	}
	var (
		userSeen      bool
		assistantTool bool
		toolSeen      bool
	)
	toolNames := map[string]bool{}
	for _, call := range calls {
		toolNames[call.Name] = true
	}
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			if strings.Contains(msg.Content, prompt) {
				userSeen = true
			}
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				assistantTool = true
			}
		case "tool":
			if len(msg.ToolCalls) > 0 {
				if toolNames[msg.ToolCalls[0].Name] {
					toolSeen = true
				}
			}
		}
	}
	if !userSeen {
		t.Fatalf("user message missing in session transcript: %+v", msgs)
	}
	if !assistantTool {
		t.Fatalf("assistant tool request missing in session transcript: %+v", msgs)
	}
	if !toolSeen {
		t.Fatalf("tool execution missing or mismatched in session transcript: %+v", msgs)
	}
}

func anthropicMaxTokens() int {
	if val := strings.TrimSpace(os.Getenv("ANTHROPIC_MAX_TOKENS")); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxTokens
}
