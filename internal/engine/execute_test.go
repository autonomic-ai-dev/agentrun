package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentrun/internal/engine"
	"agentrun/internal/verify"
)

func createQueuedRun(t *testing.T) (*engine.Run, string) {
	t.Helper()
	dataDir := t.TempDir()
	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "write note", Repo: initGitRepo(t), DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}
	return run, dataDir
}

func TestExecuteSkipsRunCancelledBeforeStart(t *testing.T) {
	run, _ := createQueuedRun(t)
	if err := engine.SetStatus(run.Dir, engine.StatusCancelled); err != nil {
		t.Fatal(err)
	}

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		Agent:  func(context.Context, string) error { t.Fatal("agent must not run for a cancelled run"); return nil },
		Checks: []verify.Check{{Name: "pass", Command: "true"}},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusCancelled {
		t.Errorf("status = %q, want %q", status, engine.StatusCancelled)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "outcome.json")); !os.IsNotExist(err) {
		t.Errorf("outcome.json exists for a run that never started: %v", err)
	}
}

func TestExecuteDefaultAgentCommandCommitsChanges(t *testing.T) {
	run, _ := createQueuedRun(t)

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd: "echo done > note.txt",
		Checks:   []verify.Check{{Name: "note", Command: "test -f note.txt"}},
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusSucceeded {
		t.Errorf("status = %q, want %q", status, engine.StatusSucceeded)
	}

	exitCode, err := os.ReadFile(filepath.Join(run.Dir, "exit_code"))
	if err != nil {
		t.Fatalf("exit_code missing: %v", err)
	}
	if strings.TrimSpace(string(exitCode)) != "0" {
		t.Errorf("exit_code = %q, want 0", exitCode)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "logs", "agent.log")); err != nil {
		t.Errorf("agent.log missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "agent.pid")); !os.IsNotExist(err) {
		t.Errorf("agent.pid should be removed after the agent exits: %v", err)
	}

	patch, err := os.ReadFile(filepath.Join(run.Dir, "patch.diff"))
	if err != nil {
		t.Fatalf("patch.diff missing: %v", err)
	}
	if !strings.Contains(string(patch), "note.txt") {
		t.Errorf("patch.diff does not mention note.txt:\n%s", patch)
	}
}

func TestExecuteFailingAgentCommandRecordsExitCode(t *testing.T) {
	run, _ := createQueuedRun(t)

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:    "exit 3",
		Checks:      []verify.Check{{Name: "note", Command: "test -f note.txt"}},
		MaxAttempts: 1,
	})
	if err == nil {
		t.Fatal("Execute returned no error for a run whose checks never pass")
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusFailed {
		t.Errorf("status = %q, want %q", status, engine.StatusFailed)
	}

	exitCode, err := os.ReadFile(filepath.Join(run.Dir, "exit_code"))
	if err != nil {
		t.Fatalf("exit_code missing: %v", err)
	}
	if strings.TrimSpace(string(exitCode)) != "3" {
		t.Errorf("exit_code = %q, want 3", exitCode)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "patch.diff")); !os.IsNotExist(err) {
		t.Errorf("patch.diff written for a failed run: %v", err)
	}
}

func TestExecuteFailedRunLeavesDecisionTrail(t *testing.T) {
	run, _ := createQueuedRun(t)

	_ = engine.Execute(context.Background(), run, engine.ExecutionOptions{
		Agent:       func(context.Context, string) error { return nil },
		Checks:      []verify.Check{{Name: "never", Command: "false"}},
		MaxAttempts: 2,
	})

	log, err := os.ReadFile(filepath.Join(run.Dir, "decisions.jsonl"))
	if err != nil {
		t.Fatalf("decisions.jsonl missing: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) != 3 {
		t.Fatalf("decisions.jsonl has %d lines, want 3 (two attempts and the failure): %s", len(lines), log)
	}
	if !strings.Contains(lines[len(lines)-1], "run failed") {
		t.Errorf("last decision = %s, want the run failed entry", lines[len(lines)-1])
	}
}
