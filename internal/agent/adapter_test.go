package agent_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"agentrun/internal/agent"
)

func TestCommandFor(t *testing.T) {
	tests := []struct {
		name      string
		agent     agent.AgentType
		prompt    string
		wantFirst string
		wantErr   error
	}{
		{
			name:      "cursor command",
			agent:     agent.TypeCursor,
			prompt:    "fix bug",
			wantFirst: "agent",
		},
		{
			name:      "agy command",
			agent:     agent.TypeAgy,
			prompt:    "fix bug",
			wantFirst: "agy",
		},
		{
			name:      "claude command",
			agent:     agent.TypeClaude,
			prompt:    "fix bug",
			wantFirst: "claude",
		},
		{
			name:    "empty prompt error",
			agent:   agent.TypeCursor,
			prompt:  "  ",
			wantErr: agent.ErrEmptyPrompt,
		},
		{
			name:    "unknown agent",
			agent:   agent.AgentType("unknown"),
			prompt:  "fix bug",
			wantErr: agent.ErrUnknownAgent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := agent.CommandFor(tt.agent, tt.prompt)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CommandFor() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CommandFor() unexpected err = %v", err)
			}
			if len(cmd) == 0 || cmd[0] != tt.wantFirst {
				t.Errorf("CommandFor() = %v, want first arg %s", cmd, tt.wantFirst)
			}
		})
	}
}

func TestAllowlistFor(t *testing.T) {
	if len(agent.AllowlistFor(agent.TypeAgy)) == 0 {
		t.Errorf("AllowlistFor(Agy) should not be empty")
	}
	if len(agent.AllowlistFor(agent.TypeCursor)) == 0 {
		t.Errorf("AllowlistFor(Cursor) should not be empty")
	}
	if len(agent.AllowlistFor(agent.TypeClaude)) == 0 {
		t.Errorf("AllowlistFor(Claude) should not be empty")
	}
	if agent.AllowlistFor(agent.AgentType("unknown")) != nil {
		t.Errorf("AllowlistFor(unknown) should be nil")
	}
}

func TestProvisionCredentials(t *testing.T) {
	fakeEnv := map[string]string{
		"GEMINI_API_KEY":    "gemini-secret",
		"CURSOR_API_KEY":    "cursor-secret",
		"ANTHROPIC_API_KEY": "claude-secret",
	}
	getenv := func(key string) string {
		return fakeEnv[key]
	}

	tmpDir := t.TempDir()

	env, err := agent.ProvisionCredentials(agent.TypeAgy, getenv, tmpDir)
	if err != nil {
		t.Fatalf("ProvisionCredentials(Agy) err = %v", err)
	}
	if env["GEMINI_API_KEY"] != "gemini-secret" {
		t.Errorf("GEMINI_API_KEY = %s, want gemini-secret", env["GEMINI_API_KEY"])
	}

	settingsPath := filepath.Join(tmpDir, ".gemini", "antigravity-cli", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json missing: %v", err)
	}
	var settings map[string]string
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json parse error: %v", err)
	}
	if settings["modelProvider"] != "gemini" {
		t.Errorf("settings modelProvider = %s, want gemini", settings["modelProvider"])
	}

	cursorEnv, err := agent.ProvisionCredentials(agent.TypeCursor, getenv, tmpDir)
	if err != nil {
		t.Fatalf("ProvisionCredentials(Cursor) err = %v", err)
	}
	if cursorEnv["CURSOR_API_KEY"] != "cursor-secret" {
		t.Errorf("CURSOR_API_KEY = %s, want cursor-secret", cursorEnv["CURSOR_API_KEY"])
	}

	claudeEnv, err := agent.ProvisionCredentials(agent.TypeClaude, getenv, tmpDir)
	if err != nil {
		t.Fatalf("ProvisionCredentials(Claude) err = %v", err)
	}
	if claudeEnv["ANTHROPIC_API_KEY"] != "claude-secret" {
		t.Errorf("ANTHROPIC_API_KEY = %s, want claude-secret", claudeEnv["ANTHROPIC_API_KEY"])
	}

	emptyGetenv := func(key string) string { return "" }
	_, err = agent.ProvisionCredentials(agent.TypeAgy, emptyGetenv, tmpDir)
	if !errors.Is(err, agent.ErrMissingCredentials) {
		t.Errorf("expected ErrMissingCredentials, got %v", err)
	}
}

func TestProvisionClaudeCredentials(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantKey  string
		wantVal  string
		wantErr  bool
		wantNone string
	}{
		{
			name:     "oauth token",
			env:      map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "oauth-secret"},
			wantKey:  "CLAUDE_CODE_OAUTH_TOKEN",
			wantVal:  "oauth-secret",
			wantNone: "ANTHROPIC_API_KEY",
		},
		{
			name:     "api key",
			env:      map[string]string{"ANTHROPIC_API_KEY": "api-secret"},
			wantKey:  "ANTHROPIC_API_KEY",
			wantVal:  "api-secret",
			wantNone: "CLAUDE_CODE_OAUTH_TOKEN",
		},
		{
			name:     "oauth token preferred over api key",
			env:      map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "oauth-secret", "ANTHROPIC_API_KEY": "api-secret"},
			wantKey:  "CLAUDE_CODE_OAUTH_TOKEN",
			wantVal:  "oauth-secret",
			wantNone: "ANTHROPIC_API_KEY",
		},
		{
			name:    "neither set",
			env:     map[string]string{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			env, err := agent.ProvisionCredentials(agent.TypeClaude, getenv, t.TempDir())
			if tt.wantErr {
				if !errors.Is(err, agent.ErrMissingCredentials) {
					t.Fatalf("expected ErrMissingCredentials, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ProvisionCredentials(Claude) err = %v", err)
			}
			if env[tt.wantKey] != tt.wantVal {
				t.Errorf("%s = %q, want %q", tt.wantKey, env[tt.wantKey], tt.wantVal)
			}
			if _, ok := env[tt.wantNone]; ok {
				t.Errorf("%s was provisioned, want it absent", tt.wantNone)
			}
		})
	}
}
