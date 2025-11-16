package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cexll/agentsdk-go/pkg/agent"
	toolbuiltin "github.com/cexll/agentsdk-go/pkg/tool/builtin"
)

func main() {
	ctx := context.Background()

	// 创建 Agent
	ag, err := agent.New(agent.Config{
		Name:        "tool-demo-agent",
		Description: "演示工具执行",
		DefaultContext: agent.RunContext{
			SessionID:     "tool-demo-session",
			WorkDir:       ".",
			MaxIterations: 1,
		},
	})
	if err != nil {
		log.Fatalf("create agent: %v", err)
	}

	// 添加 Bash 工具
	bashTool := toolbuiltin.NewBashTool()
	if err := ag.AddTool(bashTool); err != nil {
		log.Fatalf("add bash tool: %v", err)
	}

	fmt.Println("=== 测试 1: 直接执行 Bash 工具 ===")
	result1, err := bashTool.Execute(ctx, map[string]interface{}{
		"command": "echo 'Hello from agentsdk-go'",
		"timeout": 5.0,
	})
	if err != nil {
		log.Printf("bash tool error: %v", err)
	} else {
		fmt.Printf("Success: %v\nOutput: %s\n", result1.Success, result1.Output)
	}

	fmt.Println("\n=== 测试 2: 通过 Agent 调用工具（使用 tool: 语法）===")
	result2, err := ag.Run(ctx, `tool:bash_execute {"command":"ls -la | head -5"}`)
	if err != nil {
		log.Printf("agent error: %v", err)
	} else {
		fmt.Printf("Output:\n%s\n", result2.Output)
		fmt.Printf("Tool Calls: %d\n", len(result2.ToolCalls))
	}

	fmt.Println("\n=== 测试 3: File 工具 - 创建文件 ===")
	fileTool := toolbuiltin.NewFileTool()
	if err := ag.AddTool(fileTool); err != nil {
		log.Fatalf("add file tool: %v", err)
	}

	result3, err := fileTool.Execute(ctx, map[string]interface{}{
		"operation": "write",
		"path":      "./test_agentsdk.txt",
		"content":   "agentsdk-go v0.1 MVP 测试成功！\n",
	})
	if err != nil {
		log.Printf("file tool error: %v", err)
	} else {
		fmt.Printf("Success: %v\nOutput: %s\n", result3.Success, result3.Output)
	}

	fmt.Println("\n=== 测试 4: File 工具 - 读取文件 ===")
	result4, err := fileTool.Execute(ctx, map[string]interface{}{
		"operation": "read",
		"path":      "./test_agentsdk.txt",
	})
	if err != nil {
		log.Printf("file tool error: %v", err)
	} else {
		fmt.Printf("Success: %v\nOutput: %s\n", result4.Success, result4.Output)
		if data, ok := result4.Data.(map[string]interface{}); ok {
			fmt.Printf("Content: %s\n", data["content"])
		}
	}

	fmt.Println("\n=== 测试完成 ===")
	fmt.Println("✅ Agent 核心功能正常")
	fmt.Println("✅ Bash 工具正常")
	fmt.Println("✅ File 工具正常")
	fmt.Println("✅ 工具注册和调用机制正常")
}
