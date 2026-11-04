package engine_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"agentrun/internal/engine"
	"agentrun/internal/verify"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()

	runGit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s: %v", args, out, err)
		}
	}

	runGit("init")
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "initial commit")

	return repoDir
}

func TestCreateRunAndStoreLayout(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	cfg := engine.SubmitConfig{
		Prompt:  "fix typo in README",
		Repo:    repoDir,
		DataDir: dataDir,
	}

	run, err := engine.CreateRun(cfg)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	if !engine.ValidID(run.ID) {
		t.Errorf("run ID %q is not valid 6-char hex", run.ID)
	}

	expectedRunDir := filepath.Join(dataDir, "runs", run.ID)
	if run.Dir != expectedRunDir {
		t.Errorf("run.Dir = %q, want %q", run.Dir, expectedRunDir)
	}

	for _, rel := range []string{"task.md", "base", "status", "logs", "worktree"} {
		p := filepath.Join(run.Dir, rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s in run directory: %v", rel, err)
		}
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status != engine.StatusQueued {
		t.Errorf("initial status = %q, want %q", status, engine.StatusQueued)
	}

	runs, err := engine.ListRuns(dataDir)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("ListRuns returned %v, want [%s]", runs, run.ID)
	}
}

func TestDirtyTreeRejection(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	dirtyFile := filepath.Join(repoDir, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("uncommitted"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := engine.SubmitConfig{
		Prompt:       "task",
		Repo:         repoDir,
		DataDir:      dataDir,
		IncludeDirty: false,
	}

	_, err := engine.CreateRun(cfg)
	if err == nil {
		t.Fatal("expected error on dirty tree without --include-dirty")
	}

	cfg.IncludeDirty = true
	run, err := engine.CreateRun(cfg)
	if err != nil {
		t.Fatalf("unexpected error with IncludeDirty=true: %v", err)
	}
	if run == nil {
		t.Fatal("expected run instance")
	}
}

func TestPipelineStopTheLineSuccess(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	cfg := engine.SubmitConfig{
		Prompt:  "create hello.txt",
		Repo:    repoDir,
		DataDir: dataDir,
	}
	run, err := engine.CreateRun(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	opts := engine.ExecutionOptions{
		Checks: []verify.Check{
			{Name: "check-hello", Command: "test -f hello.txt"},
		},
		Agent: func(ctx context.Context, prompt string) error {
			return os.WriteFile(filepath.Join(run.Worktree, "hello.txt"), []byte("hello world\n"), 0644)
		},
	}

	if err := engine.Execute(ctx, run, opts); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusSucceeded {
		t.Errorf("status = %q, want %q", status, engine.StatusSucceeded)
	}

	if _, err := os.Stat(filepath.Join(run.Dir, "outcome.json")); err != nil {
		t.Errorf("outcome.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "patch.diff")); err != nil {
		t.Errorf("patch.diff missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "decisions.jsonl")); err != nil {
		t.Errorf("decisions.jsonl missing: %v", err)
	}

	res, err := engine.GetResult(dataDir, run.ID)
	if err != nil {
		t.Fatalf("GetResult failed: %v", err)
	}
	if res.Status != engine.StatusSucceeded {
		t.Errorf("res.Status = %s, want succeeded", res.Status)
	}
	if len(res.ChangedFiles) == 0 || res.ChangedFiles[0] != "hello.txt" {
		t.Errorf("ChangedFiles = %v, want [hello.txt]", res.ChangedFiles)
	}
}

func TestPipelineStopTheLineFailure(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	cfg := engine.SubmitConfig{
		Prompt:  "fail task",
		Repo:    repoDir,
		DataDir: dataDir,
	}
	run, err := engine.CreateRun(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	opts := engine.ExecutionOptions{
		Checks: []verify.Check{
			{Name: "always-fails", Command: "exit 1"},
		},
		MaxAttempts: 2,
		Agent: func(ctx context.Context, prompt string) error {
			return nil
		},
	}

	err = engine.Execute(ctx, run, opts)
	if err == nil {
		t.Fatal("expected Execute error on failing check")
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusFailed {
		t.Errorf("status = %q, want %q", status, engine.StatusFailed)
	}

	if _, err := os.Stat(filepath.Join(run.Dir, "outcome.json")); err != nil {
		t.Errorf("outcome.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "patch.diff")); !os.IsNotExist(err) {
		t.Errorf("patch.diff should NOT exist on failure")
	}
}

func TestCancelRaceConditionBeforeRunStarts(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	cfg := engine.SubmitConfig{
		Prompt:  "cancel before start",
		Repo:    repoDir,
		DataDir: dataDir,
	}
	run, err := engine.CreateRun(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.Cancel(dataDir, run.ID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	agentCalled := false
	ctx := context.Background()
	opts := engine.ExecutionOptions{
		Checks: []verify.Check{{Name: "dummy", Command: "true"}},
		Agent: func(ctx context.Context, prompt string) error {
			agentCalled = true
			return nil
		},
	}

	if err := engine.Execute(ctx, run, opts); err != nil {
		t.Fatalf("Execute on cancelled run returned error: %v", err)
	}

	if agentCalled {
		t.Error("agent was called even though run was cancelled before execution")
	}

	status, _ := engine.GetStatus(run.Dir)
	if status != engine.StatusCancelled {
		t.Errorf("status = %q, want %q", status, engine.StatusCancelled)
	}
}

func TestParallelRunsConcurrency(t *testing.T) {
	repoDir := initGitRepo(t)
	dataDir := t.TempDir()

	const count = 3
	var runs []*engine.Run
	for i := 0; i < count; i++ {
		cfg := engine.SubmitConfig{
			Prompt:  "concurrent task",
			Repo:    repoDir,
			DataDir: dataDir,
		}
		r, err := engine.CreateRun(cfg)
		if err != nil {
			t.Fatalf("CreateRun %d failed: %v", i, err)
		}
		runs = append(runs, r)
	}

	var wg sync.WaitGroup
	errs := make([]error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		idx := i
		run := runs[idx]
		go func() {
			defer wg.Done()
			ctx := context.Background()
			targetFile := filepath.Join(run.Worktree, "file.txt")
			opts := engine.ExecutionOptions{
				Checks: []verify.Check{
					{Name: "file-exists", Command: "test -f file.txt"},
				},
				Agent: func(ctx context.Context, prompt string) error {
					return os.WriteFile(targetFile, []byte("data\n"), 0644)
				},
			}
			errs[idx] = engine.Execute(ctx, run, opts)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("run %d execution failed: %v", i, err)
		}
		st, _ := engine.GetStatus(runs[i].Dir)
		if st != engine.StatusSucceeded {
			t.Errorf("run %d status = %s, want succeeded", i, st)
		}
	}

	listed, err := engine.ListRuns(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != count {
		t.Errorf("ListRuns returned %d runs, want %d", len(listed), count)
	}
}
