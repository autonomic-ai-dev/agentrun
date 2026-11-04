package verify_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"agentrun/internal/verify"
)

func TestLoopUsesConfiguredCheckRunnerInsteadOfHost(t *testing.T) {
	dir := t.TempDir()
	var ran []string

	outcome, err := verify.Loop(context.Background(), verify.Config{
		Dir:         dir,
		Task:        "task",
		MaxAttempts: 1,
		Agent:       func(context.Context, string) error { return nil },
		Checks:      []verify.Check{{Name: "marker", Command: "touch host-ran"}},
		CheckRunner: func(_ context.Context, c verify.Check) (verify.Result, error) {
			ran = append(ran, c.Command)
			return verify.Result{Name: c.Name, Passed: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("Loop() error = %v", err)
	}
	if outcome.Status != verify.Passed {
		t.Errorf("Status = %q, want %q", outcome.Status, verify.Passed)
	}
	if len(ran) != 1 || ran[0] != "touch host-ran" {
		t.Errorf("runner received %q, want the configured check", ran)
	}
	if _, err := os.Stat(filepath.Join(dir, "host-ran")); !os.IsNotExist(err) {
		t.Errorf("check ran on the host although a CheckRunner was set: %v", err)
	}
}

func TestLoopStopsWhenCheckRunnerFails(t *testing.T) {
	runnerErr := errors.New("container unavailable")

	_, err := verify.Loop(context.Background(), verify.Config{
		Dir:         t.TempDir(),
		Task:        "task",
		MaxAttempts: 2,
		Agent:       func(context.Context, string) error { return nil },
		Checks:      []verify.Check{{Name: "unit", Command: "true"}},
		CheckRunner: func(context.Context, verify.Check) (verify.Result, error) {
			return verify.Result{}, runnerErr
		},
	})
	if !errors.Is(err, runnerErr) {
		t.Errorf("Loop() error = %v, want %v", err, runnerErr)
	}
}
