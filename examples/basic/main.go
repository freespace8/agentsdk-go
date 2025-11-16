package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cexll/agentsdk-go/pkg/agent"
	modelpkg "github.com/cexll/agentsdk-go/pkg/model"
	"github.com/cexll/agentsdk-go/pkg/model/anthropic"
	"github.com/cexll/agentsdk-go/pkg/tool/builtin"
)

const (
	defaultModel        = "claude-3-5-sonnet-20241022"
	defaultAnthropicURL = "https://api.anthropic.com"
	basicCommandPrompt  = "请执行命令 'echo Hello from agentsdk-go' 并返回结果"
)

func main() {
	ctx := context.Background()
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is not set")
	}

	// 1) Materialise an Anthropic-backed model to verify credentials.
	claudeModel, err := newAnthropicModel(ctx, apiKey)
	if err != nil {
		log.Fatalf("create anthropic model: %v", err)
	}
	fmt.Printf("Anthropic model ready: %T (%s)\n", claudeModel, defaultModel)

	// 2) Configure a minimal agent runtime.
	ag, err := agent.New(agent.Config{
		Name:        "basic-example-agent",
		Description: "Runs simple commands through registered tools.",
		DefaultContext: agent.RunContext{
			SessionID:     "basic-example-session",
			WorkDir:       ".",
			MaxIterations: 1,
		},
	})
	if err != nil {
		log.Fatalf("create agent: %v", err)
	}

	// 3) Add built-in Bash + File tools to give the agent real capabilities.
	if err := ag.AddTool(toolbuiltin.NewBashTool()); err != nil {
		log.Fatalf("add bash tool: %v", err)
	}
	if err := ag.AddTool(toolbuiltin.NewFileTool()); err != nil {
		log.Fatalf("add file tool: %v", err)
	}

	// 4) Run a simple task that executes a harmless echo command.
	result, err := ag.Run(ctx, basicCommandPrompt)
	if err != nil {
		log.Fatalf("agent run: %v", err)
	}

	// 5) Print the agent output together with token accounting.
	fmt.Println("---- Agent Output ----")
	fmt.Println(result.Output)
	fmt.Println("---- Token Usage ----")
	fmt.Printf("input=%d output=%d total=%d cache=%d\n",
		result.Usage.InputTokens,
		result.Usage.OutputTokens,
		result.Usage.TotalTokens,
		result.Usage.CacheTokens,
	)
}

// newAnthropicModel wires the factory + provider stack required for Anthropic.
func newAnthropicModel(ctx context.Context, apiKey string) (modelpkg.Model, error) {
	// Support overriding the Anthropic API endpoint so users can point to proxies.
	baseURL := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultAnthropicURL
	}
	log.Printf("Anthropic base URL: %s", baseURL)
	log.Printf("Anthropic model: %s", defaultModel)

	factory := modelpkg.NewFactory(anthropic.NewProvider(nil))
	cfg := modelpkg.ModelConfig{
		Name:     "default",
		Provider: "anthropic",
		Model:    defaultModel,
		APIKey:   apiKey,
	}

	if opt := WithBaseURL(baseURL); opt != nil {
		opt(&cfg)
	}

	return factory.NewModel(ctx, cfg)
}

// WithBaseURL propagates a custom endpoint down to the provider.
func WithBaseURL(baseURL string) func(*modelpkg.ModelConfig) {
	return func(cfg *modelpkg.ModelConfig) {
		cfg.BaseURL = baseURL
	}
}
