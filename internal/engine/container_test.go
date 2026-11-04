package engine_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"agentrun/internal/container"
	"agentrun/internal/container/containertest"
	"agentrun/internal/engine"
	"agentrun/internal/verify"
)

func containerOptions(fake *containertest.Fake) *engine.ContainerOptions {
	return &engine.ContainerOptions{
		NewBackend: func(io.Writer) container.Backend { return fake },
		Image:      "alpine:3.20",
		User:       "1000:1000",
		Network:    "agentrun-internal",
	}
}

func TestExecuteInContainerRecordsAgentExitCodeAndRunsChecksInContainer(t *testing.T) {
	run, _ := createQueuedRun(t)
	fake := &containertest.Fake{Results: []container.Result{{ExitCode: 0}, {ExitCode: 0}}}

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:  "echo done > note.txt",
		Checks:    []verify.Check{{Name: "note", Command: "test -f note.txt"}},
		Container: containerOptions(fake),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(fake.Specs) != 2 {
		t.Fatalf("container ran %d times, want 2 (agent and check)", len(fake.Specs))
	}
	agentSpec, checkSpec := fake.Specs[0], fake.Specs[1]

	if got, want := agentSpec.Cmd, []string{"sh", "-c", "cat /agentrun/prompt.txt | echo done > note.txt"}; !slices.Equal(got, want) {
		t.Errorf("agent Cmd = %q, want %q", got, want)
	}
	if got, want := checkSpec.Cmd, []string{"sh", "-c", "test -f note.txt"}; !slices.Equal(got, want) {
		t.Errorf("check Cmd = %q, want %q", got, want)
	}
	for _, spec := range fake.Specs {
		if !spec.ReadOnly || !slices.Equal(spec.Tmpfs, []string{"/tmp"}) {
			t.Errorf("spec lacks read-only root or tmpfs /tmp: %+v", spec)
		}
		if spec.User != "1000:1000" || spec.WorkDir != "/work" || !spec.Remove {
			t.Errorf("spec user, workdir, or remove is wrong: %+v", spec)
		}
		if spec.Network != "agentrun-internal" || spec.Image != "alpine:3.20" {
			t.Errorf("spec network or image is wrong: %+v", spec)
		}
	}
	if len(agentSpec.Mounts) != 2 {
		t.Fatalf("agent mounts = %+v, want worktree and prompt", agentSpec.Mounts)
	}
	if agentSpec.Mounts[0] != (container.Mount{Host: run.Worktree, Guest: "/work"}) {
		t.Errorf("worktree mount = %+v, want read-write mount of the worktree at /work", agentSpec.Mounts[0])
	}
	if !agentSpec.Mounts[1].ReadOnly || agentSpec.Mounts[1].Guest != "/agentrun" {
		t.Errorf("prompt mount = %+v, want read-only /agentrun", agentSpec.Mounts[1])
	}

	prompt, err := os.ReadFile(filepath.Join(run.Dir, "prompt", "prompt.txt"))
	if err != nil {
		t.Fatalf("prompt file missing: %v", err)
	}
	if !strings.Contains(string(prompt), run.Task) {
		t.Errorf("prompt file = %q, want it to contain the task", prompt)
	}

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusSucceeded {
		t.Errorf("status = %q, want %q", status, engine.StatusSucceeded)
	}
}

func TestExecuteInContainerFailsRunWhenAgentExitsNonZero(t *testing.T) {
	run, _ := createQueuedRun(t)
	fake := &containertest.Fake{Results: []container.Result{{ExitCode: 3}, {ExitCode: 1}}}

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:    "exit 3",
		Checks:      []verify.Check{{Name: "note", Command: "test -f note.txt"}},
		MaxAttempts: 1,
		Container:   containerOptions(fake),
	})
	if err == nil {
		t.Fatal("Execute returned no error for a failed run")
	}

	exitCode, err := os.ReadFile(filepath.Join(run.Dir, "exit_code"))
	if err != nil {
		t.Fatalf("exit_code missing: %v", err)
	}
	if strings.TrimSpace(string(exitCode)) != "3" {
		t.Errorf("exit_code = %q, want 3", exitCode)
	}
	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusFailed {
		t.Errorf("status = %q, want %q", status, engine.StatusFailed)
	}
}

func TestExecuteInContainerPropagatesBackendStartFailure(t *testing.T) {
	run, _ := createQueuedRun(t)
	startErr := errors.New("container service is not running")
	fake := &containertest.Fake{StartErr: startErr}

	_ = engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:    "true",
		Checks:      []verify.Check{{Name: "note", Command: "true"}},
		MaxAttempts: 1,
		Container:   containerOptions(fake),
	})

	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusFailed {
		t.Errorf("status = %q, want %q", status, engine.StatusFailed)
	}
	if _, err := os.Stat(filepath.Join(run.Dir, "exit_code")); !os.IsNotExist(err) {
		t.Errorf("exit_code written although the container never started: %v", err)
	}
}

func TestExecuteInContainerRequiresBackend(t *testing.T) {
	run, _ := createQueuedRun(t)

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		Container: &engine.ContainerOptions{Image: "alpine:3.20", User: "1000:1000", Network: "agentrun-internal"},
	})
	if !errors.Is(err, engine.ErrNoBackend) {
		t.Errorf("Execute() error = %v, want ErrNoBackend", err)
	}
}
