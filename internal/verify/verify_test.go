package verify_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentrun/internal/verify"
)

func TestSaveOutcomeWritesOutcomeJSONReadableBack(t *testing.T) {
	dir := t.TempDir()
	outcome := verify.Outcome{
		Status: verify.Exhausted,
		Attempts: []verify.Attempt{
			{Number: 1, Results: []verify.Result{{Name: "unit", ExitCode: 1, Output: "fail"}}},
			{Number: 2, AgentErr: errors.New("agent exited 1"), Results: []verify.Result{{Name: "unit", ExitCode: 1, Output: "fail"}}},
		},
	}

	if err := verify.SaveOutcome(dir, outcome); err != nil {
		t.Fatalf("SaveOutcome() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "outcome.json"))
	if err != nil {
		t.Fatalf("read outcome.json: %v", err)
	}
	var saved struct {
		Status   string `json:"status"`
		Attempts []struct {
			Number   int    `json:"number"`
			AgentErr string `json:"agent_err"`
		} `json:"attempts"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("outcome.json is not valid JSON: %v", err)
	}

	if saved.Status != string(verify.Exhausted) {
		t.Errorf("saved status = %q, want %q", saved.Status, verify.Exhausted)
	}
	if len(saved.Attempts) != 2 {
		t.Errorf("saved attempts = %d, want 2", len(saved.Attempts))
	}
	if saved.Attempts[1].AgentErr != "agent exited 1" {
		t.Errorf("saved agent_err = %q, want %q", saved.Attempts[1].AgentErr, "agent exited 1")
	}
}

func TestRunChecksStopsAtFirstFailure(t *testing.T) {
	dir := t.TempDir()
	checks := []verify.Check{
		{Name: "lint", Command: "true"},
		{Name: "unit", Command: "echo boom; exit 3"},
		{Name: "e2e", Command: "touch ran-after-failure"},
	}

	results, err := verify.RunChecks(context.Background(), dir, checks)
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("RunChecks() returned %d results, want 2 (stop at first failure)", len(results))
	}
	failed := results[1]
	if failed.Passed || failed.ExitCode != 3 || !strings.Contains(failed.Output, "boom") {
		t.Errorf("failing result = %+v, want exit 3 with output containing boom", failed)
	}
	if _, err := os.Stat(filepath.Join(dir, "ran-after-failure")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("check after the failure ran; stat error = %v", err)
	}
}

func TestRunChecksKeepsOnlyTailOfLongOutput(t *testing.T) {
	dir := t.TempDir()
	checks := []verify.Check{{Name: "noisy", Command: "head -c 10000 /dev/zero | tr '\\0' a; echo END; exit 1"}}

	results, err := verify.RunChecks(context.Background(), dir, checks)
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}

	output := results[0].Output
	if len(output) > 4096 {
		t.Errorf("Output length = %d, want at most 4096", len(output))
	}
	if !strings.HasSuffix(output, "END\n") {
		t.Errorf("Output = %q, want the tail ending in END", output[max(0, len(output)-20):])
	}
}

func TestLoopFinishesOnFirstAttemptWhenChecksPass(t *testing.T) {
	calls := 0
	outcome, err := verify.Loop(context.Background(), verify.Config{
		Dir:         t.TempDir(),
		Task:        "add greeting",
		Checks:      []verify.Check{{Name: "build", Command: "true"}},
		MaxAttempts: 3,
		Agent: func(ctx context.Context, prompt string) error {
			calls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Loop() error = %v", err)
	}

	if outcome.Status != verify.Passed {
		t.Errorf("Status = %q, want %q", outcome.Status, verify.Passed)
	}
	if calls != 1 || len(outcome.Attempts) != 1 {
		t.Errorf("agent calls = %d, attempts = %d; want 1 and 1", calls, len(outcome.Attempts))
	}
}

func TestLoopFeedsFailureOutputBackToAgent(t *testing.T) {
	dir := t.TempDir()
	checks := []verify.Check{{Name: "artifact", Command: "test -f done.txt || { echo 'missing done.txt'; exit 1; }"}}
	var prompts []string

	outcome, err := verify.Loop(context.Background(), verify.Config{
		Dir:         dir,
		Task:        "create done.txt",
		Checks:      checks,
		MaxAttempts: 3,
		Agent: func(ctx context.Context, prompt string) error {
			prompts = append(prompts, prompt)
			if strings.Contains(prompt, "missing done.txt") {
				return os.WriteFile(filepath.Join(dir, "done.txt"), []byte("ok"), 0o600)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Loop() error = %v", err)
	}

	if outcome.Status != verify.Passed {
		t.Fatalf("Status = %q, want %q", outcome.Status, verify.Passed)
	}
	if len(prompts) != 2 {
		t.Fatalf("agent called %d times, want 2", len(prompts))
	}
	if prompts[0] != "create done.txt" {
		t.Errorf("first prompt = %q, want the bare task", prompts[0])
	}
	if !strings.Contains(prompts[1], "create done.txt") || !strings.Contains(prompts[1], `"artifact"`) {
		t.Errorf("repair prompt = %q, want the task and the failing check name", prompts[1])
	}
}

func TestLoopReportsExhaustedWhenFailureChangesEachAttempt(t *testing.T) {
	dir := t.TempDir()
	checks := []verify.Check{{Name: "counter", Command: "n=$(cat n 2>/dev/null || echo 0); n=$((n+1)); echo $n > n; echo \"still broken $n\"; exit 1"}}
	calls := 0

	outcome, err := verify.Loop(context.Background(), verify.Config{
		Dir:         dir,
		Task:        "fix it",
		Checks:      checks,
		MaxAttempts: 3,
		Agent: func(ctx context.Context, prompt string) error {
			calls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Loop() error = %v", err)
	}

	if outcome.Status != verify.Exhausted {
		t.Errorf("Status = %q, want %q", outcome.Status, verify.Exhausted)
	}
	if calls != 3 || len(outcome.Attempts) != 3 {
		t.Errorf("agent calls = %d, attempts = %d; want 3 and 3", calls, len(outcome.Attempts))
	}
}

func TestLoopStopsWhenSameFailureRepeats(t *testing.T) {
	calls := 0

	outcome, err := verify.Loop(context.Background(), verify.Config{
		Dir:         t.TempDir(),
		Task:        "fix it",
		Checks:      []verify.Check{{Name: "unit", Command: "echo same failure; exit 1"}},
		MaxAttempts: 5,
		Agent: func(ctx context.Context, prompt string) error {
			calls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Loop() error = %v", err)
	}

	if outcome.Status != verify.NoProgress {
		t.Errorf("Status = %q, want %q", outcome.Status, verify.NoProgress)
	}
	if calls != 2 {
		t.Errorf("agent calls = %d, want 2 (stop once the failure repeats)", calls)
	}
}

func TestLoopFailsAttemptWhenAgentExitsWithError(t *testing.T) {
	agentErr := errors.New("agent exited 1")
	checkRan := false

	outcome, err := verify.Loop(context.Background(), verify.Config{
		Dir:         t.TempDir(),
		Task:        "fix it",
		Checks:      []verify.Check{{Name: "build", Command: "true"}},
		MaxAttempts: 1,
		Agent: func(ctx context.Context, prompt string) error {
			return agentErr
		},
		CheckRunner: func(_ context.Context, c verify.Check) (verify.Result, error) {
			checkRan = true
			return verify.Result{Name: c.Name, Passed: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("Loop() error = %v", err)
	}

	if outcome.Status == verify.Passed {
		t.Errorf("Status = %q: an agent that exits with an error must not pass, even when checks pass", outcome.Status)
	}
	if checkRan {
		t.Error("checks ran after the agent failed; they must not decide a failed attempt")
	}
	if !errors.Is(outcome.Attempts[0].AgentErr, agentErr) {
		t.Errorf("AgentErr = %v, want %v", outcome.Attempts[0].AgentErr, agentErr)
	}
}

func TestLoopRejectsInvalidConfig(t *testing.T) {
	check := []verify.Check{{Name: "build", Command: "true"}}
	agent := func(ctx context.Context, prompt string) error { return nil }

	tests := []struct {
		name    string
		config  verify.Config
		wantErr error
	}{
		{
			name:    "no checks",
			config:  verify.Config{Dir: t.TempDir(), MaxAttempts: 1, Agent: agent},
			wantErr: verify.ErrNoChecks,
		},
		{
			name:    "zero attempts",
			config:  verify.Config{Dir: t.TempDir(), Checks: check, Agent: agent},
			wantErr: verify.ErrInvalidAttempts,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verify.Loop(context.Background(), tt.config)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Loop() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoopStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0

	_, err := verify.Loop(ctx, verify.Config{
		Dir:         t.TempDir(),
		Task:        "fix it",
		Checks:      []verify.Check{{Name: "build", Command: "true"}},
		MaxAttempts: 3,
		Agent: func(ctx context.Context, prompt string) error {
			calls++
			return nil
		},
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Loop() error = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Errorf("agent called %d times after cancellation, want 0", calls)
	}
}
