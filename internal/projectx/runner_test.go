package projectx_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentrun/internal/errors"
	"agentrun/internal/projectx"
)

func findProjectXBin(t *testing.T) string {
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
	t.Skip("project-x binary not found; skipping CLI integration test")
	return ""
}

func findFixturesDir(t *testing.T) string {
	t.Helper()
	dir := "/Volumes/Seagate/developer/personal/autonomic-ai-dev/project-x/verify/fixtures"
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	t.Skip("fixtures directory not found; skipping")
	return ""
}

func TestValidatePlanValid(t *testing.T) {
	bin := findProjectXBin(t)
	fixtures := findFixturesDir(t)
	planPath := filepath.Join(fixtures, "valid-multi-step.yaml")

	runner := projectx.NewRunner(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	res, err := runner.ValidatePlan(ctx, planPath)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("ValidatePlan failed unexpectedly: %v", err)
	}

	if duration > 100*time.Millisecond {
		t.Logf("Notice: ValidatePlan took %v", duration)
	}

	if res.PlanName != "valid-multi-step" {
		t.Errorf("PlanName = %q, want 'valid-multi-step'", res.PlanName)
	}
	if res.APIVersion != "x.dev/v1alpha1" {
		t.Errorf("APIVersion = %q, want 'x.dev/v1alpha1'", res.APIVersion)
	}
	if len(res.Digest) != 64 {
		t.Errorf("Digest length = %d, want 64", len(res.Digest))
	}
	if res.StepsCount != 2 {
		t.Errorf("StepsCount = %d, want 2", res.StepsCount)
	}
	if res.WavesCount != 2 {
		t.Errorf("WavesCount = %d, want 2", res.WavesCount)
	}
	if len(res.Topological) != 2 || res.Topological[0] != "deployment" || res.Topological[1] != "logs" {
		t.Errorf("Topological = %+v, want ['deployment', 'logs']", res.Topological)
	}
}

func TestValidatePlanCyclicDependency(t *testing.T) {
	bin := findProjectXBin(t)
	fixtures := findFixturesDir(t)
	planPath := filepath.Join(fixtures, "corpus/05-validation-failures/13-cycle-dependency-error.yaml")

	runner := projectx.NewRunner(bin)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := runner.ValidatePlan(ctx, planPath)
	if err == nil {
		t.Fatal("expected error for cyclic plan, got nil")
	}

	errStruct, ok := err.(errors.Error)
	if !ok {
		t.Fatalf("expected errors.Error type, got %T: %v", err, err)
	}

	if !strings.Contains(errStruct.Cause, "Cyclic dependency detected") {
		t.Errorf("Cause = %q, want containing 'Cyclic dependency detected'", errStruct.Cause)
	}
}

func TestValidatePlanFileNotFound(t *testing.T) {
	bin := findProjectXBin(t)
	runner := projectx.NewRunner(bin)

	_, err := runner.ValidatePlan(context.Background(), "/nonexistent/plan.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}

	errStruct, ok := err.(errors.Error)
	if !ok {
		t.Fatalf("expected errors.Error type, got %T: %v", err, err)
	}
	if !strings.Contains(errStruct.What, "plan file not found") {
		t.Errorf("What = %q, want containing 'plan file not found'", errStruct.What)
	}
}

func TestInspectReceipt(t *testing.T) {
	receiptJSON := `{
  "planId": "test-plan",
  "schemaVersion": "x.dev/v1alpha1",
  "planDigest": "2e291eee7f8d565316bec0b0ad815aa9e4e95b8521d1bba42c10288b4778be6a",
  "executionId": "exec-12345",
  "timestamps": {
    "startedAt": "2026-10-27T10:00:00Z",
    "completedAt": "2026-10-27T10:00:01Z",
    "durationMs": 1000
  },
  "status": "complete",
  "steps": {
    "build": {
      "tool": "go.build",
      "status": "completed",
      "durationMs": 850,
      "attempts": 1,
      "verification": "verified"
    }
  }
}`
	tmp := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(tmp, []byte(receiptJSON), 0644); err != nil {
		t.Fatal(err)
	}

	runner := projectx.NewRunner("")
	summary, err := runner.InspectReceipt(context.Background(), tmp)
	if err != nil {
		t.Fatalf("InspectReceipt failed: %v", err)
	}

	if summary.PlanID != "test-plan" {
		t.Errorf("PlanID = %q, want 'test-plan'", summary.PlanID)
	}
	if summary.Status != "complete" {
		t.Errorf("Status = %q, want 'complete'", summary.Status)
	}
	if summary.DurationMs != 1000 {
		t.Errorf("DurationMs = %d, want 1000", summary.DurationMs)
	}
	step, ok := summary.Steps["build"]
	if !ok {
		t.Fatalf("step 'build' missing from summary")
	}
	if step.Status != "completed" || step.Verification != "verified" {
		t.Errorf("step build = %+v, want status='completed', verification='verified'", step)
	}
}

func TestInspectReceiptCLI(t *testing.T) {
	bin := findProjectXBin(t)
	receiptJSON := `{
  "planId": "cli-test",
  "schemaVersion": "x.dev/v1alpha1",
  "planDigest": "2e291eee7f8d565316bec0b0ad815aa9e4e95b8521d1bba42c10288b4778be6a",
  "executionId": "exec-cli-99",
  "timestamps": {
    "startedAt": "2026-10-27T10:00:00Z",
    "completedAt": "2026-10-27T10:00:01Z",
    "durationMs": 10
  },
  "status": "complete",
  "steps": {},
  "outputs": {},
  "policyDecisions": [],
  "metrics": {
    "totalBytesProcessed": 0,
    "interpreterOverheadMs": 0
  }
}`
	tmp := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(tmp, []byte(receiptJSON), 0644); err != nil {
		t.Fatal(err)
	}

	runner := projectx.NewRunner(bin)
	out, err := runner.InspectReceiptCLI(context.Background(), tmp)
	if err != nil {
		t.Fatalf("InspectReceiptCLI failed: %v", err)
	}
	if !strings.Contains(out, "cli-test") || !strings.Contains(out, "exec-cli-99") {
		t.Errorf("InspectReceiptCLI output missing expected details: %s", out)
	}
}

func TestInspectReceiptInvalidJSON(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(tmp, []byte("not-json"), 0644); err != nil {
		t.Fatal(err)
	}

	runner := projectx.NewRunner("")
	_, err := runner.InspectReceipt(context.Background(), tmp)
	if err == nil {
		t.Fatal("expected error for invalid JSON receipt, got nil")
	}
	if _, ok := err.(errors.Error); !ok {
		t.Errorf("expected errors.Error, got %T", err)
	}
}

func TestValidationResultSerialization(t *testing.T) {
	res := projectx.ValidationResult{
		PlanName:    "demo",
		APIVersion:  "x.dev/v1alpha1",
		Digest:      "abc",
		StepsCount:  1,
		WavesCount:  1,
		Topological: []string{"step1"},
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var unmarshaled projectx.ValidationResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatal(err)
	}
	if unmarshaled.PlanName != "demo" || len(unmarshaled.Topological) != 1 {
		t.Errorf("Serialization roundtrip failed: %+v", unmarshaled)
	}
}

func TestValidatePlanCaching(t *testing.T) {
	bin := findProjectXBin(t)
	fixtures := findFixturesDir(t)
	planPath := filepath.Join(fixtures, "valid-multi-step.yaml")

	runner := projectx.NewRunner(bin)
	ctx := context.Background()

	res1, err := runner.ValidatePlan(ctx, planPath)
	if err != nil {
		t.Fatalf("first validation failed: %v", err)
	}

	start := time.Now()
	res2, err := runner.ValidatePlan(ctx, planPath)
	cachedDuration := time.Since(start)

	if err != nil {
		t.Fatalf("second validation failed: %v", err)
	}
	if res1.Digest != res2.Digest || res1.PlanName != res2.PlanName {
		t.Errorf("cached result mismatch: %+v vs %+v", res1, res2)
	}
	if cachedDuration > 10*time.Millisecond {
		t.Errorf("expected cached validation in < 10ms, took %v", cachedDuration)
	}
}

func TestValidatePlanBytes(t *testing.T) {
	bin := findProjectXBin(t)
	fixtures := findFixturesDir(t)
	planPath := filepath.Join(fixtures, "valid-multi-step.yaml")
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	runner := projectx.NewRunner(bin)
	ctx := context.Background()

	res, err := runner.ValidatePlanBytes(ctx, data)
	if err != nil {
		t.Fatalf("ValidatePlanBytes failed: %v", err)
	}
	if res.PlanName != "valid-multi-step" || res.StepsCount != 2 {
		t.Errorf("unexpected ValidatePlanBytes result: %+v", res)
	}
}
