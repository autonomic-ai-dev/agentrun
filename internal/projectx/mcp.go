package projectx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

func DefaultMCPConfig(binPath string) *MCPConfig {
	if binPath == "" {
		binPath = "/usr/local/bin/project-x"
	}
	return &MCPConfig{
		MCPServers: map[string]ServerConfig{
			"project-x": {
				Command: binPath,
				Args:    []string{"mcp"},
			},
		},
	}
}

func GenerateMCPConfigJSON(binPath string) ([]byte, error) {
	cfg := DefaultMCPConfig(binPath)
	return json.MarshalIndent(cfg, "", "  ")
}

func WriteAgentMCPConfig(homeDir string, agentType string, binPath string) error {
	data, err := GenerateMCPConfigJSON(binPath)
	if err != nil {
		return err
	}

	var targetPath string
	switch agentType {
	case "cursor":
		targetPath = filepath.Join(homeDir, ".cursor", "mcp.json")
	case "agy":
		targetPath = filepath.Join(homeDir, ".gemini", "antigravity-cli", "mcp.json")
	case "claude":
		targetPath = filepath.Join(homeDir, ".claude", "claude_desktop_config.json")
	default:
		return fmt.Errorf("unsupported agent type %q", agentType)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(targetPath, data, 0644)
}
