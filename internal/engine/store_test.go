package engine_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"agentrun/internal/engine"
)

func TestValidID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "six lowercase hex", id: "a1b2c3", want: true},
		{name: "too short", id: "a1b2c", want: false},
		{name: "too long", id: "a1b2c3d", want: false},
		{name: "uppercase hex", id: "A1B2C3", want: false},
		{name: "non-hex letter", id: "a1b2cg", want: false},
		{name: "path separator", id: "../a1b2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := engine.ValidID(tt.id); got != tt.want {
				t.Errorf("ValidID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestDefaultDataDirHonorsEnvironment(t *testing.T) {
	t.Setenv("AGENTRUN_DATA", "/custom/agentrun")
	if got := engine.DefaultDataDir(); got != "/custom/agentrun" {
		t.Errorf("DefaultDataDir() = %q, want %q", got, "/custom/agentrun")
	}
}

func TestRunsDirFallsBackToDefaultDataDir(t *testing.T) {
	t.Setenv("AGENTRUN_DATA", "/custom/agentrun")
	if got := engine.RunsDir(""); got != filepath.Join("/custom/agentrun", "runs") {
		t.Errorf("RunsDir(\"\") = %q, want %q", got, filepath.Join("/custom/agentrun", "runs"))
	}
}

func TestGetStatusRejectsCorruptFile(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "status"), []byte("paused\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := engine.GetStatus(runDir)
	if !errors.Is(err, engine.ErrCorruptStatusFile) {
		t.Errorf("GetStatus() error = %v, want ErrCorruptStatusFile", err)
	}
}

func TestGetRunRejectsInvalidID(t *testing.T) {
	_, err := engine.GetRun(t.TempDir(), "../etc")
	if !errors.Is(err, engine.ErrInvalidRunID) {
		t.Errorf("GetRun() error = %v, want ErrInvalidRunID", err)
	}
}

func TestGetRunMissingRunIsAnError(t *testing.T) {
	_, err := engine.GetRun(t.TempDir(), "a1b2c3")
	if err == nil {
		t.Fatal("GetRun() on a missing run returned no error")
	}
}

func TestGetRunReturnsSubmittedRun(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	created, err := engine.CreateRun(engine.SubmitConfig{Prompt: "  fix typo  ", Repo: repoDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	got, err := engine.GetRun(dataDir, created.ID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if got.Task != "fix typo" {
		t.Errorf("Task = %q, want %q", got.Task, "fix typo")
	}
	if got.Base != created.Base {
		t.Errorf("Base = %q, want %q", got.Base, created.Base)
	}
	if got.Status != engine.StatusQueued {
		t.Errorf("Status = %q, want %q", got.Status, engine.StatusQueued)
	}
	if got.Worktree != created.Worktree {
		t.Errorf("Worktree = %q, want %q", got.Worktree, created.Worktree)
	}
}

func TestListRunsWithoutRunsDirIsEmpty(t *testing.T) {
	runs, err := engine.ListRuns(t.TempDir())
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("ListRuns returned %d runs, want 0", len(runs))
	}
}

func TestListRunsSkipsNonRunEntries(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	first, err := engine.CreateRun(engine.SubmitConfig{Prompt: "first", Repo: repoDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(engine.RunsDir(dataDir), "notes.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(engine.RunsDir(dataDir), "zzzzzz"), 0755); err != nil {
		t.Fatal(err)
	}

	runs, err := engine.ListRuns(dataDir)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != first.ID {
		t.Errorf("ListRuns = %+v, want only run %s", runs, first.ID)
	}
}

func TestCreateRunRejectsMissingPrompt(t *testing.T) {
	_, err := engine.CreateRun(engine.SubmitConfig{Prompt: "   ", Repo: initGitRepo(t), DataDir: t.TempDir()})
	if !errors.Is(err, engine.ErrMissingPrompt) {
		t.Errorf("CreateRun() error = %v, want ErrMissingPrompt", err)
	}
}

func TestCreateRunRejectsNonGitDirectory(t *testing.T) {
	_, err := engine.CreateRun(engine.SubmitConfig{Prompt: "task", Repo: t.TempDir(), DataDir: t.TempDir()})
	if !errors.Is(err, engine.ErrNotGitRepo) {
		t.Errorf("CreateRun() error = %v, want ErrNotGitRepo", err)
	}
}
