package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunWithoutSubmitPrintsUsage(t *testing.T) {
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"status"}, &bytes.Buffer{}, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: agentrun submit") {
		t.Errorf("stderr = %q, want the usage line", stderr.String())
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
