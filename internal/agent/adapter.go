package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AgentType string

const (
	TypeCursor AgentType = "cursor"
	TypeAgy    AgentType = "agy"
	TypeClaude AgentType = "claude"
)

var (
	ErrUnknownAgent       = errors.New("unknown agent type")
	ErrMissingCredentials = errors.New("missing API credentials for agent")
	ErrEmptyPrompt        = errors.New("prompt cannot be empty")
)

func CommandFor(agent AgentType, prompt string) ([]string, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, ErrEmptyPrompt
	}
	switch agent {
	case TypeCursor:
		return []string{"agent", "-p", prompt, "--trust"}, nil
	case TypeAgy:
		return []string{"agy", "-p", prompt, "--add-dir", ".", "--dangerously-skip-permissions"}, nil
	case TypeClaude:
		return []string{"claude", "-p", prompt, "--permission-mode", "acceptEdits"}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownAgent, agent)
	}
}

func AllowlistFor(agent AgentType) []string {
	switch agent {
	case TypeAgy:
		return []string{
			"generativelanguage.googleapis.com:443",
		}
	case TypeCursor:
		return []string{
			"api2.cursor.sh:443",
			"agentn.global.api5.cursor.sh:443",
		}
	case TypeClaude:
		return []string{
			"api.anthropic.com:443",
		}
	default:
		return nil
	}
}

func ProvisionCredentials(agent AgentType, getenv func(string) string, homeDir string) (map[string]string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	env := make(map[string]string)

	switch agent {
	case TypeAgy:
		key := getenv("GEMINI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("%w: GEMINI_API_KEY required for agy", ErrMissingCredentials)
		}
		env["GEMINI_API_KEY"] = key

		if homeDir != "" {
			settingsDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
			if err := os.MkdirAll(settingsDir, 0755); err != nil {
				return nil, err
			}
			settingsPath := filepath.Join(settingsDir, "settings.json")
			settings := map[string]string{"modelProvider": "gemini"}
			data, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(settingsPath, data, 0644); err != nil {
				return nil, err
			}
		}

	case TypeCursor:
		key := getenv("CURSOR_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("%w: CURSOR_API_KEY required for cursor", ErrMissingCredentials)
		}
		env["CURSOR_API_KEY"] = key

	case TypeClaude:
		if token := getenv("CLAUDE_CODE_OAUTH_TOKEN"); token != "" {
			env["CLAUDE_CODE_OAUTH_TOKEN"] = token
		} else {
			key := getenv("ANTHROPIC_API_KEY")
			if key == "" {
				return nil, fmt.Errorf("%w: CLAUDE_CODE_OAUTH_TOKEN or ANTHROPIC_API_KEY required for claude", ErrMissingCredentials)
			}
			env["ANTHROPIC_API_KEY"] = key
		}

	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownAgent, agent)
	}

	return env, nil
}
