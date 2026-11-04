package engine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"agentrun/internal/engine"
)

func TestCancelQueuedRunMarksCancelled(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "task", Repo: repoDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	if err := engine.Cancel(dataDir, run.ID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status != engine.StatusCancelled {
		t.Errorf("status = %q, want %q", status, engine.StatusCancelled)
	}
}

func TestCancelTerminalRunIsRejected(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "task", Repo: repoDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}
	if err := engine.SetStatus(run.Dir, engine.StatusSucceeded); err != nil {
		t.Fatal(err)
	}

	err = engine.Cancel(dataDir, run.ID)
	if !errors.Is(err, engine.ErrRunAlreadyTerminal) {
		t.Fatalf("Cancel() error = %v, want ErrRunAlreadyTerminal", err)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status != engine.StatusSucceeded {
		t.Errorf("status = %q, want %q to remain", status, engine.StatusSucceeded)
	}
}

func TestCancelUnknownRunIsAnError(t *testing.T) {
	if err := engine.Cancel(t.TempDir(), "a1b2c3"); err == nil {
		t.Fatal("Cancel() on an unknown run returned no error")
	}
}

func TestCancelTerminatesRecordedAgentProcess(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "task", Repo: repoDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}
	if err := engine.SetStatus(run.Dir, engine.StatusRunning); err != nil {
		t.Fatal(err)
	}

	agent := exec.Command("sleep", "30")
	if err := agent.Start(); err != nil {
		t.Fatalf("start agent stand-in: %v", err)
	}
	pidPath := filepath.Join(run.Dir, "agent.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(agent.Process.Pid)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := engine.Cancel(dataDir, run.ID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	waitErr := agent.Wait()
	if waitErr == nil {
		t.Fatal("agent stand-in exited cleanly; expected it to be terminated by Cancel")
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.Exited() {
		t.Errorf("agent stand-in wait error = %v, want termination by signal", waitErr)
	}
}
