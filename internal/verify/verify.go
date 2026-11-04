package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"unicode/utf8"
)

const outputLimit = 4096

const (
	outcomeFileName = "outcome.json"
	agentResultName = "agent"
)

var (
	ErrNoChecks        = errors.New("verify: no checks configured")
	ErrInvalidAttempts = errors.New("verify: max attempts must be at least 1")
)

type Check struct {
	Name    string
	Command string
}

type Result struct {
	Name     string
	Passed   bool
	ExitCode int
	Output   string
}

type Agent func(ctx context.Context, prompt string) error

type CheckRunner func(ctx context.Context, c Check) (Result, error)

type Config struct {
	Dir         string
	Task        string
	Checks      []Check
	MaxAttempts int
	Agent       Agent
	CheckRunner CheckRunner
}

type Status string

const (
	Passed     Status = "passed"
	Exhausted  Status = "exhausted"
	NoProgress Status = "no_progress"
)

type Attempt struct {
	Number   int
	AgentErr error
	Results  []Result
}

type Outcome struct {
	Status   Status
	Attempts []Attempt
}

func RunChecks(ctx context.Context, dir string, checks []Check) ([]Result, error) {
	return runChecks(ctx, checks, func(ctx context.Context, c Check) (Result, error) {
		return runCheck(ctx, dir, c)
	})
}

func runChecks(ctx context.Context, checks []Check, run CheckRunner) ([]Result, error) {
	var results []Result
	for _, c := range checks {
		res, err := run(ctx, c)
		if err != nil {
			return results, err
		}
		results = append(results, res)
		if !res.Passed {
			break
		}
	}
	return results, nil
}

func NewResult(name string, exitCode int, output string) Result {
	return Result{Name: name, Passed: exitCode == 0, ExitCode: exitCode, Output: tail(output)}
}

func runCheck(ctx context.Context, dir string, c Check) (Result, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", c.Command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	res := Result{Name: c.Name, Passed: err == nil, Output: tail(string(out))}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return Result{}, fmt.Errorf("run check %q: %w", c.Name, err)
	}
	return res, nil
}

func Loop(ctx context.Context, cfg Config) (Outcome, error) {
	if len(cfg.Checks) == 0 {
		return Outcome{}, ErrNoChecks
	}
	if cfg.MaxAttempts < 1 {
		return Outcome{}, ErrInvalidAttempts
	}

	var out Outcome
	var previous Result
	prompt := cfg.Task
	for n := 1; n <= cfg.MaxAttempts; n++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}

		agentErr := cfg.Agent(ctx, prompt)
		var results []Result
		if agentErr != nil {
			results = []Result{{Name: agentResultName, ExitCode: 1, Output: agentErr.Error()}}
		} else {
			var err error
			results, err = runChecks(ctx, cfg.Checks, checkRunner(cfg))
			if err != nil {
				return out, err
			}
		}
		out.Attempts = append(out.Attempts, Attempt{Number: n, AgentErr: agentErr, Results: results})

		last := results[len(results)-1]
		if last.Passed {
			out.Status = Passed
			return out, nil
		}
		if n > 1 && last == previous {
			out.Status = NoProgress
			return out, nil
		}
		previous = last
		prompt = repairPrompt(cfg.Task, last)
	}
	out.Status = Exhausted
	return out, nil
}

func checkRunner(cfg Config) CheckRunner {
	if cfg.CheckRunner != nil {
		return cfg.CheckRunner
	}
	return func(ctx context.Context, c Check) (Result, error) {
		return runCheck(ctx, cfg.Dir, c)
	}
}

type outcomeFile struct {
	Status   Status        `json:"status"`
	Attempts []attemptFile `json:"attempts"`
}

type attemptFile struct {
	Number   int      `json:"number"`
	AgentErr string   `json:"agent_err,omitempty"`
	Results  []Result `json:"results"`
}

func SaveOutcome(dir string, outcome Outcome) error {
	file := outcomeFile{Status: outcome.Status}
	for _, a := range outcome.Attempts {
		af := attemptFile{Number: a.Number, Results: a.Results}
		if a.AgentErr != nil {
			af.AgentErr = a.AgentErr.Error()
		}
		file.Attempts = append(file.Attempts, af)
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal outcome: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, outcomeFileName), data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", outcomeFileName, err)
	}
	return nil
}

func repairPrompt(task string, failed Result) string {
	return fmt.Sprintf(
		"%s\n\nThe check %q failed with exit code %d. Its output ended with:\n%s\nFix the code so every check passes. Do not weaken, skip, or delete a check.",
		task, failed.Name, failed.ExitCode, failed.Output,
	)
}

func tail(s string) string {
	if len(s) <= outputLimit {
		return s
	}
	cut := len(s) - outputLimit
	for !utf8.RuneStart(s[cut]) {
		cut++
	}
	return s[cut:]
}
