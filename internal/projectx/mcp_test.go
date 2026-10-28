package projectx_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agentrun/internal/projectx"
)

func TestDefaultMCPConfig(t *testing.T) {
	cfg := projectx.DefaultMCPConfig("")
	if srv, ok := cfg.MCPServers["project-x"]; !ok {
		t.Fatal("expected 'project-x' key in mcpServers")
	} else {
		if srv.Command != "/usr/local/bin/project-x" {
			t.Errorf("Command = %q, want /usr/local/bin/project-x", srv.Command)
		}
		if len(srv.Args) != 1 || srv.Args[0] != "mcp" {
			t.Errorf("Args = %v, want ['mcp']", srv.Args)
		}
	}
}

func TestGenerateMCPConfigJSON(t *testing.T) {
	data, err := projectx.GenerateMCPConfigJSON("/opt/bin/project-x")
	if err != nil {
		t.Fatalf("GenerateMCPConfigJSON error: %v", err)
	}

	var parsed projectx.MCPConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if parsed.MCPServers["project-x"].Command != "/opt/bin/project-x" {
		t.Errorf("Command = %q, want /opt/bin/project-x", parsed.MCPServers["project-x"].Command)
	}
}

func TestWriteAgentMCPConfig(t *testing.T) {
	dir := t.TempDir()

	for _, agent := range []string{"cursor", "agy", "claude"} {
		if err := projectx.WriteAgentMCPConfig(dir, agent, "/usr/bin/project-x"); err != nil {
			t.Fatalf("WriteAgentMCPConfig for %s failed: %v", agent, err)
		}
	}

	cursorPath := filepath.Join(dir, ".cursor", "mcp.json")
	if _, err := os.Stat(cursorPath); err != nil {
		t.Errorf("expected cursor mcp.json at %s", cursorPath)
	}

	agyPath := filepath.Join(dir, ".gemini", "antigravity-cli", "mcp.json")
	if _, err := os.Stat(agyPath); err != nil {
		t.Errorf("expected agy mcp.json at %s", agyPath)
	}

	claudePath := filepath.Join(dir, ".claude", "claude_desktop_config.json")
	if _, err := os.Stat(claudePath); err != nil {
		t.Errorf("expected claude config at %s", claudePath)
	}

	if err := projectx.WriteAgentMCPConfig(dir, "invalid", ""); err == nil {
		t.Error("expected error for unsupported agent type, got nil")
	}
}
