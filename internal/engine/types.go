package engine

import (
	"agentrun/internal/verify"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Run struct {
	ID       string `json:"id"`
	Dir      string `json:"dir"`
	Task     string `json:"task"`
	Base     string `json:"base"`
	Status   Status `json:"status"`
	Worktree string `json:"worktree"`
}

type SubmitConfig struct {
	Prompt       string
	Repo         string
	IncludeDirty bool
	AgentCmd     string
	Checks       []verify.Check
	MaxAttempts  int
	DataDir      string
}

type Result struct {
	ID           string   `json:"id"`
	Status       Status   `json:"status"`
	Branch       string   `json:"branch"`
	Commit       string   `json:"commit"`
	ChangedFiles []string `json:"changed_files"`
	PatchPath    string   `json:"patch_path,omitempty"`
}
