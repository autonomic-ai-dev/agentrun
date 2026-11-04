package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"agentrun/internal/container"
	"agentrun/internal/verify"
)

var ErrNoBackend = errors.New("container options need a backend")

type ContainerOptions struct {
	NewBackend func(output io.Writer) container.Backend
	Image      string
	User       string
	Network    string
	Memory     string
}

func (co ContainerOptions) spec(run *Run, command string) container.Spec {
	return container.Spec{
		Image:    co.Image,
		User:     co.User,
		ReadOnly: true,
		Tmpfs:    []string{"/tmp"},
		Network:  co.Network,
		Memory:   co.Memory,
		WorkDir:  "/work",
		Remove:   true,
		Labels:   map[string]string{"agentrun.run": run.ID},
		Mounts: []container.Mount{
			{Host: run.Worktree, Guest: "/work"},
			{Host: filepath.Join(run.Dir, "prompt"), Guest: "/agentrun", ReadOnly: true},
		},
		Cmd: []string{"sh", "-c", command},
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

		result, err := co.NewBackend(logFile).Run(ctx, co.spec(run, "cat /agentrun/prompt.txt | "+agentCmd))
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
		result, err := co.NewBackend(&output).Run(ctx, co.spec(run, c.Command))
		if err != nil {
			return verify.Result{}, fmt.Errorf("run check %q: %w", c.Name, err)
		}
		return verify.NewResult(c.Name, result.ExitCode, output.String()), nil
	}
}
