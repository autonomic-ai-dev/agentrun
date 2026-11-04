//go:build capability

package capability_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agentrun/internal/container"
	"agentrun/internal/container/apple"
	"agentrun/internal/engine"
	"agentrun/internal/verify"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
	return string(out)
}

func newRepoWithCommit(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "README.md")
	gitIn(t, repo, "commit", "-q", "-m", "initial")
	return repo
}

func appleEngineOptions() *engine.ContainerOptions {
	return &engine.ContainerOptions{
		NewBackend: func(output io.Writer) container.Backend {
			return &apple.Apple{Binary: "container", Output: output}
		},
		Image:   pinnedImage,
		User:    fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		Network: "default",
		Memory:  "512M",
	}
}

func TestSubmitToPatchRunsAgentInIsolatedContainer(t *testing.T) {
	repo := newRepoWithCommit(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "add note", Repo: repo, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	err = engine.Execute(t.Context(), run, engine.ExecutionOptions{
		AgentCmd:    "echo done > note.txt",
		Checks:      []verify.Check{{Name: "note", Command: "test -f note.txt"}},
		MaxAttempts: 1,
		Container:   appleEngineOptions(),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusSucceeded {
		t.Fatalf("status = %q, want %q; agent log:\n%s", status, engine.StatusSucceeded, readFileOrEmpty(filepath.Join(run.Dir, "logs", "agent.log")))
	}

	exitCode := strings.TrimSpace(readFileOrEmpty(filepath.Join(run.Dir, "exit_code")))
	if exitCode != "0" {
		t.Errorf("exit_code = %q, want 0", exitCode)
	}

	patch := readFileOrEmpty(filepath.Join(run.Dir, "patch.diff"))
	if !strings.Contains(patch, "note.txt") {
		t.Errorf("patch.diff does not contain note.txt:\n%s", patch)
	}
	if _, err := os.Stat(filepath.Join(repo, "note.txt")); !os.IsNotExist(err) {
		t.Errorf("agent output reached the source repository: %v", err)
	}
}

func TestAgentExitCodeIsRecordedWhileAttached(t *testing.T) {
	repo := newRepoWithCommit(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "try a write outside the workspace", Repo: repo, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	_ = engine.Execute(t.Context(), run, engine.ExecutionOptions{
		AgentCmd:    "echo x > /etc/agentrun-probe",
		Checks:      []verify.Check{{Name: "note", Command: "test -f note.txt"}},
		MaxAttempts: 1,
		Container:   appleEngineOptions(),
	})

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusFailed {
		t.Errorf("status = %q, want %q", status, engine.StatusFailed)
	}
	exitCode := strings.TrimSpace(readFileOrEmpty(filepath.Join(run.Dir, "exit_code")))
	if exitCode == "" || exitCode == "0" {
		t.Errorf("exit_code = %q, want the non-zero exit of the agent", exitCode)
	}
	if outcome := readFileOrEmpty(filepath.Join(run.Dir, "outcome.json")); !strings.Contains(outcome, "agent_err") {
		t.Errorf("outcome.json does not record the agent error:\n%s", outcome)
	}
}

func readFileOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
