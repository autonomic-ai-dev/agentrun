package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"agentrun/internal/container"
	"agentrun/internal/egress"
	"agentrun/internal/verify"
)

var (
	ErrNoBackend         = errors.New("container options need a backend")
	ErrNoContainerChecks = errors.New("container runs need explicit checks or a .verify/run.sh in the repository")
	ErrNoEgressNetwork   = errors.New("egress options need an internal network name")
)

const (
	containerNameFile     = "container.name"
	containerCLI          = "container"
	containerStopInterval = 250 * time.Millisecond
)

type ContainerOptions struct {
	NewBackend func(output io.Writer) container.Backend
	Image      string
	User       string
	Network    string
	Memory     string
	Egress     *EgressOptions

	networks []string
	env      map[string]string
}

type EgressOptions struct {
	Image         string
	Network       string
	Allow         []string
	CreateNetwork func(ctx context.Context, name string) error
	DeleteNetwork func(ctx context.Context, name string) error
	StartSidecar  func(ctx context.Context, cfg egress.SidecarConfig) (*egress.Sidecar, error)
}

func (eg EgressOptions) withDefaults() EgressOptions {
	if eg.CreateNetwork == nil {
		eg.CreateNetwork = egress.CreateInternalNetwork
	}
	if eg.DeleteNetwork == nil {
		eg.DeleteNetwork = egress.DeleteNetwork
	}
	if eg.StartSidecar == nil {
		eg.StartSidecar = egress.StartSidecar
	}
	return eg
}

func (eg EgressOptions) runNetwork(run *Run) string {
	return eg.Network + "-" + run.ID
}

func (co ContainerOptions) spec(run *Run, command string) container.Spec {
	return container.Spec{
		Image:    co.Image,
		User:     co.User,
		ReadOnly: true,
		Tmpfs:    []string{"/tmp"},
		Network:  co.Network,
		Networks: co.networks,
		Memory:   co.Memory,
		WorkDir:  "/work",
		Remove:   true,
		Labels:   map[string]string{"agentrun.run": run.ID},
		Env:      co.env,
		Mounts: []container.Mount{
			{Host: run.Worktree, Guest: "/work"},
			{Host: filepath.Join(run.Dir, "prompt"), Guest: "/agentrun", ReadOnly: true},
		},
		Cmd: []string{"sh", "-c", command},
	}
}

func (co ContainerOptions) behindProxy(network, proxyURL string) ContainerOptions {
	co.networks = []string{network}
	co.env = map[string]string{"HTTPS_PROXY": proxyURL}
	return co
}

func startEgress(ctx context.Context, run *Run, eg EgressOptions) (*egress.Sidecar, error) {
	if eg.Network == "" {
		return nil, ErrNoEgressNetwork
	}
	network := eg.runNetwork(run)
	if err := eg.CreateNetwork(ctx, network); err != nil {
		return nil, err
	}
	sidecar, err := eg.StartSidecar(ctx, egress.SidecarConfig{
		Name:            "agentrun-egress-" + run.ID,
		Image:           eg.Image,
		Allow:           eg.Allow,
		InternalNetwork: network,
	})
	if err != nil {
		_ = eg.DeleteNetwork(context.Background(), network)
		return nil, err
	}
	return sidecar, nil
}

func runContainer(ctx context.Context, run *Run, co ContainerOptions, command string, output io.Writer) (container.Result, error) {
	spec := co.spec(run, command)
	spec.Name = fmt.Sprintf("agentrun-%s-%d", run.ID, time.Now().UnixNano())
	if err := os.WriteFile(filepath.Join(run.Dir, containerNameFile), []byte(spec.Name+"\n"), 0644); err != nil {
		return container.Result{}, err
	}

	done := make(chan struct{})
	defer close(done)
	go stopWhenCancelled(run, spec.Name, done)

	return co.NewBackend(output).Run(ctx, spec)
}

func stopWhenCancelled(run *Run, name string, done <-chan struct{}) {
	ticker := time.NewTicker(containerStopInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if status, err := GetStatus(run.Dir); err == nil && status == StatusCancelled {
				_ = exec.Command(containerCLI, "stop", name).Run()
			}
		}
	}
}

func containerAgent(run *Run, co ContainerOptions, agentCmd string) verify.Agent {
	return func(ctx context.Context, prompt string) error {
		promptDir := filepath.Join(run.Dir, "prompt")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(promptDir, "prompt.txt"), []byte(prompt), 0644); err != nil {
			return err
		}

		logFile, err := os.OpenFile(filepath.Join(run.Dir, "logs", "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer logFile.Close()

		result, err := runContainer(ctx, run, co, "cat /agentrun/prompt.txt | "+agentCmd, logFile)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(run.Dir, "exit_code"), []byte(strconv.Itoa(result.ExitCode)+"\n"), 0644); err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("agent exited with code %d", result.ExitCode)
		}
		return nil
	}
}

func containerChecks(run *Run, co ContainerOptions) verify.CheckRunner {
	return func(ctx context.Context, c verify.Check) (verify.Result, error) {
		var output bytes.Buffer
		result, err := runContainer(ctx, run, co, c.Command, &output)
		if err != nil {
			return verify.Result{}, fmt.Errorf("run check %q: %w", c.Name, err)
		}
		return verify.NewResult(c.Name, result.ExitCode, output.String()), nil
	}
}
