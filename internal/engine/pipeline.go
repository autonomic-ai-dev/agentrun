package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"agentrun/internal/decisionlog"
	"agentrun/internal/verify"
)

type ExecutionOptions struct {
	Agent       verify.Agent
	AgentCmd    string
	Checks      []verify.Check
	MaxAttempts int
	Container   *ContainerOptions
}

func agentCommand(opts ExecutionOptions) string {
	if opts.AgentCmd != "" {
		return opts.AgentCmd
	}
	if env := os.Getenv("AGENTRUN_AGENT_CMD"); env != "" {
		return env
	}
	return "claude -p"
}

func Execute(ctx context.Context, run *Run, opts ExecutionOptions) error {
	if opts.Container != nil && opts.Container.NewBackend == nil {
		return ErrNoBackend
	}

	currentStatus, err := GetStatus(run.Dir)
	if err != nil {
		return err
	}
	if currentStatus == StatusCancelled {
		return nil
	}

	if err := SetStatus(run.Dir, StatusRunning); err != nil {
		return err
	}

	checks := opts.Checks
	if len(checks) == 0 {
		verifyScript := filepath.Join(run.Worktree, ".verify", "run.sh")
		if _, err := os.Stat(verifyScript); err == nil {
			checks = []verify.Check{
				{Name: "verify", Command: "sh .verify/run.sh all"},
			}
		} else {
			checks = []verify.Check{
				{Name: "git-status", Command: "git status"},
			}
		}
	}

	maxAttempts := opts.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 3
	}

	agentFn := opts.Agent
	var checkRunner verify.CheckRunner
	if opts.Container != nil {
		checkRunner = containerChecks(run, *opts.Container)
		if agentFn == nil {
			agentFn = containerAgent(run, *opts.Container, agentCommand(opts))
		}
	}
	if agentFn == nil {
		agentCmd := agentCommand(opts)

		agentFn = func(attemptCtx context.Context, prompt string) error {
			logPath := filepath.Join(run.Dir, "logs", "agent.log")
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer logFile.Close()

			cmd := exec.CommandContext(attemptCtx, "sh", "-c", agentCmd)
			cmd.Dir = run.Worktree
			cmd.Stdin = strings.NewReader(prompt)
			cmd.Stdout = logFile
			cmd.Stderr = logFile

			if err := cmd.Start(); err != nil {
				return err
			}

			pidPath := filepath.Join(run.Dir, "agent.pid")
			_ = os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0644)
			defer os.Remove(pidPath)

			waitErr := cmd.Wait()
			var exitCode int
			if waitErr != nil {
				var exitErr *exec.ExitError
				if errors.As(waitErr, &exitErr) {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = 1
				}
			}
			_ = os.WriteFile(filepath.Join(run.Dir, "exit_code"), []byte(strconv.Itoa(exitCode)+"\n"), 0644)
			return waitErr
		}
	}

	outcome, loopErr := verify.Loop(ctx, verify.Config{
		Dir:         run.Worktree,
		Task:        run.Task,
		Checks:      checks,
		MaxAttempts: maxAttempts,
		Agent:       agentFn,
		CheckRunner: checkRunner,
	})

	statusAfterLoop, _ := GetStatus(run.Dir)
	if statusAfterLoop == StatusCancelled || (ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)) {
		_ = SetStatus(run.Dir, StatusCancelled)
		return nil
	}

	_ = verify.SaveOutcome(run.Dir, outcome)

	logPath := filepath.Join(run.Dir, "decisions.jsonl")
	for _, a := range outcome.Attempts {
		attemptOutcome := "pass"
		for _, r := range a.Results {
			if !r.Passed {
				attemptOutcome = "fail"
				break
			}
		}
		_ = decisionlog.Append(logPath, decisionlog.Entry{
			Decision: fmt.Sprintf("attempt %d", a.Number),
			Reason:   "automated verification attempt",
			Evidence: "outcome.json",
			Outcome:  attemptOutcome,
		})
	}

	if loopErr != nil || outcome.Status != verify.Passed {
		_ = SetStatus(run.Dir, StatusFailed)
		_ = decisionlog.Append(logPath, decisionlog.Entry{
			Decision: "run failed",
			Reason:   "verify loop unsatisfied",
			Evidence: "outcome.json",
			Outcome:  "failed",
		})
		if loopErr != nil {
			return loopErr
		}
		return fmt.Errorf("verification failed with status %s", outcome.Status)
	}

	addCmd := exec.Command("git", "-C", run.Worktree, "add", "-A")
	_ = addCmd.Run()

	diffCached := exec.Command("git", "-C", run.Worktree, "diff", "--cached", "--quiet")
	if diffCached.Run() != nil {
		commitMsg := fmt.Sprintf("agentrun: run %s\n", run.ID)
		commitCmd := exec.Command("git", "-C", run.Worktree, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", commitMsg)
		if out, err := commitCmd.CombinedOutput(); err != nil {
			_ = SetStatus(run.Dir, StatusFailed)
			return fmt.Errorf("git commit: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}

	diffCmd := exec.Command("git", "-C", run.Worktree, "diff", run.Base, "HEAD")
	if diffOut, err := diffCmd.Output(); err == nil && len(diffOut) > 0 {
		_ = os.WriteFile(filepath.Join(run.Dir, "patch.diff"), diffOut, 0644)
	}

	latestStatus, _ := GetStatus(run.Dir)
	if latestStatus != StatusCancelled {
		_ = SetStatus(run.Dir, StatusSucceeded)
		_ = decisionlog.Append(logPath, decisionlog.Entry{
			Decision: "run succeeded",
			Reason:   "verification passed and changes committed",
			Evidence: "outcome.json",
			Outcome:  "succeeded",
		})
	}

	return nil
}
