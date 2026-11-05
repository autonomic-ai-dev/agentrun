package engine_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"agentrun/internal/container"
	"agentrun/internal/container/containertest"
	"agentrun/internal/egress"
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

	recordedName, err := os.ReadFile(filepath.Join(run.Dir, "container.name"))
	if err != nil {
		t.Fatalf("container.name missing: %v", err)
	}
	lastSpec := fake.Specs[len(fake.Specs)-1]
	if strings.TrimSpace(string(recordedName)) != lastSpec.Name || lastSpec.Name == "" {
		t.Errorf("container.name = %q, want the name of the last container started, %q", recordedName, lastSpec.Name)
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

func TestExecuteInContainerRequiresChecksWhenRepoHasNoVerifyScript(t *testing.T) {
	run, _ := createQueuedRun(t)
	fake := &containertest.Fake{}

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:  "true",
		Container: containerOptions(fake),
	})
	if !errors.Is(err, engine.ErrNoContainerChecks) {
		t.Fatalf("Execute() error = %v, want ErrNoContainerChecks", err)
	}
	if len(fake.Specs) != 0 {
		t.Errorf("container ran %d times before the checks guard", len(fake.Specs))
	}
	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != engine.StatusQueued {
		t.Errorf("status = %q, want %q; the run never started", status, engine.StatusQueued)
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

func egressOptions(create func(context.Context, string) error, start func(context.Context, egress.SidecarConfig) (*egress.Sidecar, error)) *engine.EgressOptions {
	return &engine.EgressOptions{
		Image:         "agentrun-egress:test",
		Network:       "agentrun-internal",
		Allow:         []string{"example.com:443"},
		CreateNetwork: create,
		DeleteNetwork: func(context.Context, string) error { return nil },
		StartSidecar:  start,
	}
}

func TestExecuteEgressRunsContainersBehindSidecarProxyOnRunNetwork(t *testing.T) {
	run, _ := createQueuedRun(t)
	fake := &containertest.Fake{Results: []container.Result{{ExitCode: 0}, {ExitCode: 0}}}
	var created string
	var started egress.SidecarConfig
	var deleted []string
	const proxyURL = "http://192.168.129.2:3128"
	wantNetwork := "agentrun-internal-" + run.ID

	co := containerOptions(fake)
	co.Egress = egressOptions(
		func(_ context.Context, name string) error {
			created = name
			return nil
		},
		func(_ context.Context, cfg egress.SidecarConfig) (*egress.Sidecar, error) {
			started = cfg
			return &egress.Sidecar{Name: cfg.Name, ProxyURL: proxyURL}, nil
		},
	)
	co.Egress.DeleteNetwork = func(_ context.Context, name string) error {
		deleted = append(deleted, name)
		return nil
	}

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:    "true",
		Checks:      []verify.Check{{Name: "true", Command: "true"}},
		MaxAttempts: 1,
		Container:   co,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if created != wantNetwork {
		t.Errorf("created network = %q, want %q", created, wantNetwork)
	}
	wantCfg := egress.SidecarConfig{
		Name:            "agentrun-egress-" + run.ID,
		Image:           "agentrun-egress:test",
		Allow:           []string{"example.com:443"},
		InternalNetwork: wantNetwork,
	}
	if !reflect.DeepEqual(started, wantCfg) {
		t.Errorf("sidecar config = %+v, want %+v", started, wantCfg)
	}
	if len(fake.Specs) != 2 {
		t.Fatalf("container ran %d times, want 2", len(fake.Specs))
	}
	for _, spec := range fake.Specs {
		if !slices.Equal(spec.Networks, []string{wantNetwork}) {
			t.Errorf("spec networks = %v, want only %s", spec.Networks, wantNetwork)
		}
		if spec.Env["HTTPS_PROXY"] != proxyURL {
			t.Errorf("spec env = %v, want HTTPS_PROXY=%s", spec.Env, proxyURL)
		}
	}
	if !slices.Equal(deleted, []string{wantNetwork}) {
		t.Errorf("deleted networks = %v, want the run network %s after the run", deleted, wantNetwork)
	}
}

func TestExecuteEgressFailsRunWithoutEgressNetwork(t *testing.T) {
	run, _ := createQueuedRun(t)
	fake := &containertest.Fake{}
	co := containerOptions(fake)
	co.Egress = &engine.EgressOptions{Image: "agentrun-egress:test"}

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:  "true",
		Checks:    []verify.Check{{Name: "true", Command: "true"}},
		Container: co,
	})
	if !errors.Is(err, engine.ErrNoEgressNetwork) {
		t.Fatalf("Execute() error = %v, want ErrNoEgressNetwork", err)
	}
	if len(fake.Specs) != 0 {
		t.Errorf("container ran %d times without an egress network", len(fake.Specs))
	}
	assertStatus(t, run, engine.StatusFailed)
}

func TestExecuteEgressFailsRunWhenNetworkCannotBeEnsured(t *testing.T) {
	run, _ := createQueuedRun(t)
	fake := &containertest.Fake{}
	networkErr := errors.New("network service is not running")
	sidecarStarted := false
	co := containerOptions(fake)
	co.Egress = egressOptions(
		func(context.Context, string) error { return networkErr },
		func(context.Context, egress.SidecarConfig) (*egress.Sidecar, error) {
			sidecarStarted = true
			return nil, nil
		},
	)

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:  "true",
		Checks:    []verify.Check{{Name: "true", Command: "true"}},
		Container: co,
	})
	if !errors.Is(err, networkErr) {
		t.Fatalf("Execute() error = %v, want %v", err, networkErr)
	}
	if sidecarStarted {
		t.Error("sidecar started although the network could not be ensured")
	}
	if len(fake.Specs) != 0 {
		t.Errorf("container ran %d times without a network", len(fake.Specs))
	}
	assertStatus(t, run, engine.StatusFailed)
}

func TestExecuteEgressFailsRunWhenSidecarCannotStart(t *testing.T) {
	run, _ := createQueuedRun(t)
	fake := &containertest.Fake{}
	sidecarErr := errors.New("sidecar image is missing")
	var deleted []string
	co := containerOptions(fake)
	co.Egress = egressOptions(
		func(context.Context, string) error { return nil },
		func(context.Context, egress.SidecarConfig) (*egress.Sidecar, error) { return nil, sidecarErr },
	)
	co.Egress.DeleteNetwork = func(_ context.Context, name string) error {
		deleted = append(deleted, name)
		return nil
	}

	err := engine.Execute(context.Background(), run, engine.ExecutionOptions{
		AgentCmd:  "true",
		Checks:    []verify.Check{{Name: "true", Command: "true"}},
		Container: co,
	})
	if !errors.Is(err, sidecarErr) {
		t.Fatalf("Execute() error = %v, want %v", err, sidecarErr)
	}
	if len(fake.Specs) != 0 {
		t.Errorf("container ran %d times without a sidecar", len(fake.Specs))
	}
	if want := []string{"agentrun-internal-" + run.ID}; !slices.Equal(deleted, want) {
		t.Errorf("deleted networks = %v, want %v after the sidecar failed", deleted, want)
	}
	assertStatus(t, run, engine.StatusFailed)
}

func assertStatus(t *testing.T, run *engine.Run, want engine.Status) {
	t.Helper()
	status, err := engine.GetStatus(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Errorf("status = %q, want %q", status, want)
	}
}
