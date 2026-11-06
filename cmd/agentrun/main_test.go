package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agentrun/internal/engine"
)

func TestUnknownCommandPrintsUsage(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"bogus"}, &bytes.Buffer{}, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: agentrun <command>") {
		t.Errorf("stderr = %q, want the usage line", stderr.String())
	}
}

func TestNoArgumentsPrintsUsage(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), nil, &bytes.Buffer{}, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: agentrun <command>") {
		t.Errorf("stderr = %q, want the usage line", stderr.String())
	}
}

func TestHelpPrintsUsageAndExitsZero(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		var stdout bytes.Buffer
		if code := run(context.Background(), args, &stdout, &bytes.Buffer{}); code != 0 {
			t.Errorf("run(%v) exit code = %d, want 0", args, code)
		}
		if !strings.Contains(stdout.String(), "Commands:") {
			t.Errorf("run(%v) stdout = %q, want the command list", args, stdout.String())
		}
	}
}

func TestSubmitHelpNamesRequiredFlags(t *testing.T) {
	var stdout bytes.Buffer
	if code := run(context.Background(), []string{"help"}, &stdout, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "--agent, --agent-cmd, --image, --egress-image") {
		t.Errorf("stdout = %q, want the required submit flags", stdout.String())
	}
}

func TestSubmitHelpListsFlagsAndExitsZero(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"submit", "-h"}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	for _, flagName := range []string{"-agent-cmd", "-egress-image", "-max-attempts", "-check"} {
		if !strings.Contains(stderr.String(), flagName) {
			t.Errorf("stderr does not list %s: %q", flagName, stderr.String())
		}
	}
}

func TestSubmitUnknownFlagExitsTwoWithUsage(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"submit", "--bogus", "task"}, &bytes.Buffer{}, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: agentrun <command>") {
		t.Errorf("stderr = %q, want the usage line", stderr.String())
	}
}

func TestSubmitReportsMissingRequiredFlagDeterministically(t *testing.T) {
	const want = "submit needs --agent\n"
	for i := 0; i < 50; i++ {
		var stderr bytes.Buffer
		if code := run(context.Background(), []string{"submit", "task"}, &bytes.Buffer{}, &stderr); code != 2 {
			t.Fatalf("exit code = %d, want 2", code)
		}
		if stderr.String() != want {
			t.Fatalf("attempt %d: stderr = %q, want %q", i, stderr.String(), want)
		}
	}
}

func TestSubmitRequiresTaskAndContainerFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no task", args: []string{"submit", "--agent", "claude"}, want: "exactly one task argument"},
		{name: "no agent", args: []string{"submit", "--agent-cmd", "x", "--image", "i:1", "--egress-image", "e:1", "task"}, want: "needs --agent"},
		{name: "no image", args: []string{"submit", "--agent", "claude", "--agent-cmd", "x", "--egress-image", "e:1", "task"}, want: "needs --image"},
		{name: "no egress image", args: []string{"submit", "--agent", "claude", "--agent-cmd", "x", "--image", "i:1", "task"}, want: "needs --egress-image"},
		{name: "unknown agent", args: []string{"submit", "--agent", "gpt", "--agent-cmd", "x", "--image", "i:1", "--egress-image", "e:1", "task"}, want: "unknown agent type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(context.Background(), tt.args, &bytes.Buffer{}, &stderr); code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestSubmitRequiresClaudeCredentialsBeforeCreatingARun(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	repo := t.TempDir()
	dataDir := t.TempDir()
	var stderr bytes.Buffer
	args := []string{"submit", "--repo", repo, "--data", dataDir, "--agent", "claude", "--agent-cmd", "x", "--image", "i:1", "--egress-image", "e:1", "task"}
	if code := run(context.Background(), args, &bytes.Buffer{}, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "required for claude") {
		t.Errorf("stderr = %q, want the missing-credentials error", stderr.String())
	}
	if entries, _ := os.ReadDir(filepath.Join(dataDir, "runs")); len(entries) != 0 {
		t.Errorf("a run directory was created without credentials: %d entries", len(entries))
	}
}

func newRunForCLI(t *testing.T, dataDir string) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	created, err := engine.CreateRun(engine.SubmitConfig{Prompt: "task", Repo: repo, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	return created.ID
}

func TestRunCommandsReadAndCancelARun(t *testing.T) {
	dataDir := t.TempDir()
	id := newRunForCLI(t, dataDir)
	logPath := filepath.Join(dataDir, "runs", id, "logs", "agent.log")
	if err := os.WriteFile(logPath, []byte("agent output\n"), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"status", "-data", dataDir, id}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "run "+id+" queued\n" {
		t.Errorf("status stdout = %q, want the queued status", stdout.String())
	}

	stdout.Reset()
	if code := run(context.Background(), []string{"logs", "-data", dataDir, id}, &stdout, &stderr); code != 0 {
		t.Fatalf("logs exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "agent output\n" {
		t.Errorf("logs stdout = %q, want the agent log", stdout.String())
	}

	stdout.Reset()
	if code := run(context.Background(), []string{"result", "-data", dataDir, id}, &stdout, &stderr); code != 0 {
		t.Fatalf("result exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "branch agentrun/"+id+"\n") {
		t.Errorf("result stdout = %q, want the run branch", stdout.String())
	}

	stdout.Reset()
	if code := run(context.Background(), []string{"cancel", "-data", dataDir, id}, &stdout, &stderr); code != 0 {
		t.Fatalf("cancel exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != "run "+id+" cancelled\n" {
		t.Errorf("cancel stdout = %q", stdout.String())
	}

	stdout.Reset()
	run(context.Background(), []string{"status", "-data", dataDir, id}, &stdout, &bytes.Buffer{})
	if stdout.String() != "run "+id+" cancelled\n" {
		t.Errorf("status after cancel = %q, want cancelled", stdout.String())
	}

	stderr.Reset()
	if code := run(context.Background(), []string{"cancel", "-data", dataDir, id}, &bytes.Buffer{}, &stderr); code != 1 {
		t.Errorf("second cancel exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already terminal") {
		t.Errorf("second cancel stderr = %q, want the terminal-state error", stderr.String())
	}
}

func TestRunCommandsRejectBadArgumentsAndUnknownRuns(t *testing.T) {
	dataDir := t.TempDir()
	tests := []struct {
		name     string
		args     []string
		wantCode int
		want     string
	}{
		{name: "no run id", args: []string{"status", "-data", dataDir}, wantCode: 2, want: "usage: agentrun status"},
		{name: "two run ids", args: []string{"logs", "-data", dataDir, "a", "b"}, wantCode: 2, want: "usage: agentrun logs"},
		{name: "unknown flag", args: []string{"result", "-bogus", "x"}, wantCode: 2, want: "flag provided but not defined"},
		{name: "unknown run", args: []string{"status", "-data", dataDir, "abc123"}, wantCode: 1, want: "not found"},
		{name: "invalid run id", args: []string{"cancel", "-data", dataDir, "../escape"}, wantCode: 1, want: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(context.Background(), tt.args, &bytes.Buffer{}, &stderr); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestCheckListParsesNameAndCommand(t *testing.T) {
	var checks checkList
	if err := checks.Set("unit=go test ./..."); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if len(checks) != 1 || checks[0].Name != "unit" || checks[0].Command != "go test ./..." {
		t.Errorf("checks = %+v, want one unit check with the full command", checks)
	}
	for _, bad := range []string{"no-equals", "=cmd", "name="} {
		if err := checks.Set(bad); err == nil {
			t.Errorf("Set(%q) accepted an invalid check", bad)
		}
	}
}
