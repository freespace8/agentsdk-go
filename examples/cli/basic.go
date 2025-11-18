package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cexll/agentsdk-go/pkg/api"
	modelpkg "github.com/cexll/agentsdk-go/pkg/model"
)

const minimalConfig = "version: v0.0.1\ndescription: agentsdk-go CLI example\nenvironment: {}\n"

// basic example showing how the unified API powers CLI-style runs.
func main() {
	projectRoot, cleanup, err := resolveProjectRoot()
	if err != nil {
		log.Fatalf("init project root: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	provider := &modelpkg.AnthropicProvider{ModelName: "claude-3-5-sonnet-20241022"}
	rt, err := api.New(context.Background(), api.Options{
		EntryPoint:   api.EntryPointCLI,
		ProjectRoot:  projectRoot,
		ModelFactory: provider,
	})
	if err != nil {
		log.Fatalf("build runtime: %v", err)
	}
	defer rt.Close()

	req := api.Request{
		Prompt: "用一句中文介绍 agentsdk-go 项目。",
		Mode: api.ModeContext{
			EntryPoint: api.EntryPointCLI,
			CLI:        &api.CLIContext{User: os.Getenv("USER")},
		},
	}
	resp, err := rt.Run(context.Background(), req)
	if err != nil {
		log.Fatalf("run: %v", err)
	}
	if resp.Result != nil {
		fmt.Println(resp.Result.Output)
	}
}

func resolveProjectRoot() (string, func(), error) {
	if root := strings.TrimSpace(os.Getenv("AGENTSDK_PROJECT_ROOT")); root != "" {
		return root, nil, nil
	}
	tmp, err := os.MkdirTemp("", "agentsdk-cli-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	if err := scaffoldMinimalConfig(tmp); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp, cleanup, nil
}

func scaffoldMinimalConfig(root string) error {
	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return err
	}
	configPath := filepath.Join(claudeDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(configPath, []byte(minimalConfig), 0o644)
}
