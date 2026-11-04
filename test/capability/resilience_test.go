//go:build capability

package capability_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"agentrun/internal/engine"
	"agentrun/internal/verify"
)

func TestCancelBeforeContainerRunningStopsContainer(t *testing.T) {
	repo := newRepoWithCommit(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "sleep", Repo: repo, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	cancelErr := make(chan error, 1)
	go func() {
		nameFile := filepath.Join(run.Dir, "container.name")
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if readFileOrEmpty(nameFile) != "" {
				cancelErr <- engine.Cancel(dataDir, run.ID)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancelErr <- fmt.Errorf("container.name never appeared")
	}()

	start := time.Now()
	_ = engine.Execute(t.Context(), run, engine.ExecutionOptions{
		AgentCmd:    "sleep 120",
		Checks:      []verify.Check{{Name: "true", Command: "true"}},
		MaxAttempts: 1,
		Container:   appleEngineOptions(),
	})
	elapsed := time.Since(start)

	if err := <-cancelErr; err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if elapsed > 60*time.Second {
		t.Errorf("Execute took %s: a cancel during container start did not stop the container", elapsed)
	}
	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusCancelled {
		t.Errorf("status = %q, want %q", status, engine.StatusCancelled)
	}
}

func TestRunEndsWhenContainerServiceRestarts(t *testing.T) {
	repo := newRepoWithCommit(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "sleep", Repo: repo, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	t.Cleanup(func() {
		_ = exec.Command("container", "system", "start").Run()
	})

	stopped := make(chan error, 1)
	go func() {
		nameFile := filepath.Join(run.Dir, "container.name")
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			name := strings.TrimSpace(readFileOrEmpty(nameFile))
			if name != "" && containerRunning(name) {
				stopped <- exec.Command("container", "system", "stop").Run()
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		stopped <- fmt.Errorf("container never reached running state")
	}()

	start := time.Now()
	_ = engine.Execute(t.Context(), run, engine.ExecutionOptions{
		AgentCmd:    "sleep 120",
		Checks:      []verify.Check{{Name: "true", Command: "true"}},
		MaxAttempts: 1,
		Container:   appleEngineOptions(),
	})
	elapsed := time.Since(start)

	if err := <-stopped; err != nil {
		t.Fatalf("container system stop failed: %v", err)
	}
	if elapsed > 110*time.Second {
		t.Errorf("Execute took %s after the service stopped; it waited for the agent", elapsed)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status == engine.StatusRunning || status == engine.StatusQueued {
		t.Errorf("status = %q after the service stopped; a run must end in a terminal state", status)
	}

	if out, err := exec.Command("container", "system", "start").CombinedOutput(); err != nil {
		t.Fatalf("container system start: %s: %v", out, err)
	}
	name := strings.TrimSpace(readFileOrEmpty(filepath.Join(run.Dir, "container.name")))
	if containerRunning(name) {
		t.Errorf("container %s still runs after the service restarted", name)
	}
}

var runIDPattern = regexp.MustCompile(`(?m)^run (\w+) (\w+)$`)

func TestSubmitCommandRunsEndToEnd(t *testing.T) {
	egressImage := buildEgressImage(t)
	binary := filepath.Join(t.TempDir(), "agentrun")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/agentrun")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build agentrun: %s: %v", out, err)
	}

	repo := newRepoWithCommit(t)
	dataDir := t.TempDir()

	cmd := exec.Command(binary, "submit",
		"--repo", repo,
		"--data", dataDir,
		"--agent", "claude",
		"--agent-cmd", "cat >/dev/null; echo done > note.txt",
		"--image", pinnedImage,
		"--egress-image", egressImage,
		"--memory", "512M",
		"--max-attempts", "1",
		"--check", "note=test -f note.txt",
		"add note",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("submit failed: %s: %v", out, err)
	}

	match := runIDPattern.FindStringSubmatch(string(out))
	if match == nil || match[2] != "succeeded" {
		t.Fatalf("submit output lacks a succeeded run line:\n%s", out)
	}
	patch := readFileOrEmpty(filepath.Join(dataDir, "runs", match[1], "patch.diff"))
	if !strings.Contains(patch, "note.txt") {
		t.Errorf("patch.diff does not contain note.txt:\n%s", patch)
	}
}
