package engine_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"agentrun/internal/engine"
)

func TestGetResultReportsChangedFilesAndPatch(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "add note", Repo: repoDir, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(run.Worktree, "note.txt"), []byte("done\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "note"}} {
		if out, err := exec.Command("git", append([]string{"-C", run.Worktree}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	if err := os.WriteFile(filepath.Join(run.Dir, "patch.diff"), []byte("diff\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetStatus(run.Dir, engine.StatusSucceeded); err != nil {
		t.Fatal(err)
	}

	res, err := engine.GetResult(dataDir, run.ID)
	if err != nil {
		t.Fatalf("GetResult failed: %v", err)
	}
	if res.Status != engine.StatusSucceeded {
		t.Errorf("Status = %q, want %q", res.Status, engine.StatusSucceeded)
	}
	if res.Branch != "agentrun/"+run.ID {
		t.Errorf("Branch = %q, want %q", res.Branch, "agentrun/"+run.ID)
	}
	if res.Commit == "" {
		t.Error("Commit is empty for a run with a commit")
	}
	if len(res.ChangedFiles) != 1 || res.ChangedFiles[0] != "note.txt" {
		t.Errorf("ChangedFiles = %v, want [note.txt]", res.ChangedFiles)
	}
	if res.PatchPath != filepath.Join(run.Dir, "patch.diff") {
		t.Errorf("PatchPath = %q, want the run's patch.diff", res.PatchPath)
	}
}

func TestGetResultUnknownRunIsAnError(t *testing.T) {
	_, err := engine.GetResult(t.TempDir(), "a1b2c3")
	if err == nil {
		t.Fatal("GetResult on an unknown run returned no error")
	}
}

func TestCreateRunRejectsRepositoryWithoutCommits(t *testing.T) {
	repoDir := t.TempDir()
	if out, err := exec.Command("git", "-C", repoDir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}

	_, err := engine.CreateRun(engine.SubmitConfig{Prompt: "task", Repo: repoDir, DataDir: t.TempDir()})
	if !errors.Is(err, engine.ErrNoCommits) {
		t.Errorf("CreateRun() error = %v, want ErrNoCommits", err)
	}
}

func TestCreateRunAcceptsDirtyTreeWhenIncluded(t *testing.T) {
	repoDir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("wip\n"), 0644); err != nil {
		t.Fatal(err)
	}

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "task", Repo: repoDir, DataDir: t.TempDir(), IncludeDirty: true})
	if err != nil {
		t.Fatalf("CreateRun with IncludeDirty failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(run.Worktree, "scratch.txt")); !os.IsNotExist(err) {
		t.Errorf("uncommitted file was copied into the run worktree: %v", err)
	}
}
