// Package configgen renders explicit, non-secret integration configurations.
package configgen

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Anurag607/amoeba/runtimekit"
	"gopkg.in/yaml.v3"
)

// Render creates configuration content for a supported host target.
func Render(target, binaryPath, configPath string) ([]byte, string, error) {
	if binaryPath == "" {
		binaryPath = "agentic-moe"
	}
	args := []string{"mcp", "stdio"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	switch strings.ToLower(target) {
	case "generic":
		body, err := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"agentic-moe": map[string]any{"command": binaryPath, "args": args}}}, "", "  ")
		return append(body, '\n'), ".mcp.json", err
	case "vscode":
		body, err := json.MarshalIndent(map[string]any{"servers": map[string]any{"agentic-moe": map[string]any{"type": "stdio", "command": binaryPath, "args": args}}}, "", "  ")
		return append(body, '\n'), filepath.Join(".vscode", "mcp.json"), err
	case "codex":
		quotedArgs := make([]string, len(args))
		for i, arg := range args {
			quotedArgs[i] = fmt.Sprintf("%q", arg)
		}
		body := fmt.Sprintf("[mcp_servers.agentic-moe]\ncommand = %q\nargs = [%s]\n", binaryPath, strings.Join(quotedArgs, ", "))
		return []byte(body), "agentic-moe.codex.toml", nil
	case "ollama":
		cfg := runtimekit.DefaultConfig()
		cfg.Ollama.Enabled = true
		body, err := yaml.Marshal(cfg)
		return body, "agentic-moe.yaml", err
	default:
		return nil, "", fmt.Errorf("config generator: unknown target %q", target)
	}
}
