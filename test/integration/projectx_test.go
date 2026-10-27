package integration_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentrun/internal/projectx"
	"agentrun/internal/verify"
)

func projectXBinary(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/Volumes/Seagate/developer/personal/autonomic-ai-dev/project-x/target/release/project-x",
		"/Volumes/Seagate/developer/personal/autonomic-ai-dev/project-x/target/debug/project-x",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := exec.LookPath("project-x"); err == nil {
		return p
	}
	t.Skip("project-x binary not found; skipping integration test")
	return ""
}

func fixturesDir(t *testing.T) string {
	t.Helper()
	dir := "/Volumes/Seagate/developer/personal/autonomic-ai-dev/project-x/verify/fixtures"
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	t.Skip("fixtures directory not found; skipping integration test")
	return ""
}

func TestEndToEndProjectXValidationAndEvidence(t *testing.T) {
	bin := projectXBinary(t)
	fixtures := fixturesDir(t)
	runner := projectx.NewRunner(bin)

	validPlan := filepath.Join(fixtures, "valid-multi-step.yaml")
	cyclicPlan := filepath.Join(fixtures, "corpus/05-validation-failures/13-cycle-dependency-error.yaml")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	val, err := runner.ValidatePlan(ctx, validPlan)
	if err != nil {
		t.Fatalf("ValidatePlan failed on valid fixture: %v", err)
	}
	if val.PlanName != "valid-multi-step" || val.StepsCount != 2 {
		t.Fatalf("Unexpected validation result: %+v", val)
	}

	_, err = runner.ValidatePlan(ctx, cyclicPlan)
	if err == nil {
		t.Fatal("expected failure on cyclic plan, got nil")
	}

	workDir := t.TempDir()
	receiptPath := filepath.Join(workDir, "receipt.json")

	runCmd := exec.CommandContext(ctx, bin, "run", validPlan, "--out", receiptPath)
	if out, err := runCmd.CombinedOutput(); err != nil {
		t.Fatalf("project-x run failed: %v\nOutput: %s", err, string(out))
	}

	receipt, err := runner.InspectReceipt(ctx, receiptPath)
	if err != nil {
		t.Fatalf("InspectReceipt failed on generated receipt: %v", err)
	}
	if receipt.PlanID != "valid-multi-step" {
		t.Errorf("PlanID = %q, want 'valid-multi-step'", receipt.PlanID)
	}

	if err := verify.SaveOutcome(workDir, verify.Outcome{
		Status: verify.Passed,
		Attempts: []verify.Attempt{
			{Number: 1, Results: []verify.Result{
				{Name: "preflight-plan", Passed: true, ExitCode: 0},
				{Name: "plan-receipt", Passed: true, ExitCode: 0},
			}},
		},
	}); err != nil {
		t.Fatalf("SaveOutcome error: %v", err)
	}

	evidenceCmd := exec.Command("go", "run", "./cmd/agentrun-dev", "evidence", workDir)
	evidenceCmd.Dir = filepath.Clean(filepath.Join(fixtures, "../../../agentrun"))
	var stdout, stderr bytes.Buffer
	evidenceCmd.Stdout = &stdout
	evidenceCmd.Stderr = &stderr

	if err := evidenceCmd.Run(); err != nil {
		t.Fatalf("agentrun-dev evidence failed: %v\nStderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "status: passed") {
		t.Errorf("expected 'status: passed' in evidence output: %s", out)
	}
	if !strings.Contains(out, "project-x receipt: valid-multi-step") {
		t.Errorf("expected 'project-x receipt: valid-multi-step' in evidence output: %s", out)
	}
	if !strings.Contains(out, "step deployment:") {
		t.Errorf("expected 'step deployment:' in evidence output: %s", out)
	}
}
