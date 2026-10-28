package projectx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"agentrun/internal/errors"
)

var (
	ansiRegex    = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	stepsRegex   = regexp.MustCompile(`Steps:\s+(\d+)\s+\((\d+)\s+execution wave\[s\]\)`)
	topologRegex = regexp.MustCompile(`Topological:\s+(.+)`)
)

type Runner struct {
	binPath string
	mu      sync.RWMutex
	cache   map[string]*ValidationResult
}

func NewRunner(binPath string) *Runner {
	if binPath == "" {
		if p, err := exec.LookPath("project-x"); err == nil {
			binPath = p
		} else {
			binPath = "project-x"
		}
	}
	return &Runner{
		binPath: binPath,
		cache:   make(map[string]*ValidationResult),
	}
}

func (r *Runner) BinPath() string {
	return r.binPath
}

func (r *Runner) ValidatePlan(ctx context.Context, planPath string) (*ValidationResult, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf("plan file not found %q", filepath.Base(planPath)),
			err.Error(),
			"verify the plan file path exists and is readable",
			"https://agentrun.dev/docs/troubleshooting#project-x-plan",
		)
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(data))

	r.mu.RLock()
	if cached, ok := r.cache[digest]; ok {
		r.mu.RUnlock()
		cp := *cached
		return &cp, nil
	}
	r.mu.RUnlock()

	res, err := r.runValidate(ctx, planPath)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[digest] = res
	r.mu.Unlock()

	return res, nil
}

func (r *Runner) ValidatePlanBytes(ctx context.Context, planContent []byte) (*ValidationResult, error) {
	digest := fmt.Sprintf("%x", sha256.Sum256(planContent))

	r.mu.RLock()
	if cached, ok := r.cache[digest]; ok {
		r.mu.RUnlock()
		cp := *cached
		return &cp, nil
	}
	r.mu.RUnlock()

	tmp, err := os.CreateTemp("", "agentrun-plan-*.yaml")
	if err != nil {
		return nil, errors.New(
			"failed to create temporary plan file",
			err.Error(),
			"ensure the system temporary directory is writable",
			"https://agentrun.dev/docs/troubleshooting#project-x-plan",
		)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(planContent); err != nil {
		_ = tmp.Close()
		return nil, errors.New(
			"failed to write temporary plan file",
			err.Error(),
			"check disk space and permissions in temp dir",
			"https://agentrun.dev/docs/troubleshooting#project-x-plan",
		)
	}
	_ = tmp.Close()

	res, err := r.runValidate(ctx, tmp.Name())
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[digest] = res
	r.mu.Unlock()

	return res, nil
}

func (r *Runner) runValidate(ctx context.Context, planPath string) (*ValidationResult, error) {
	cmd := exec.CommandContext(ctx, r.binPath, "validate", planPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	combined := stdout.String() + "\n" + stderr.String()
	cleaned := strings.TrimSpace(ansiRegex.ReplaceAllString(combined, ""))

	if err != nil {
		errMsg := cleaned
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, errors.New(
			fmt.Sprintf("plan validation rejected %q", filepath.Base(planPath)),
			errMsg,
			"fix the plan DAG, schema, or step permissions so project-x validate exits 0",
			"https://agentrun.dev/docs/troubleshooting#project-x-plan",
		)
	}

	res := &ValidationResult{
		Topological: make([]string, 0),
	}

	lines := strings.Split(cleaned, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "✅ Plan is valid:") {
			res.PlanName = strings.TrimSpace(strings.TrimPrefix(line, "✅ Plan is valid:"))
		} else if strings.HasPrefix(line, "API Version:") {
			res.APIVersion = strings.TrimSpace(strings.TrimPrefix(line, "API Version:"))
		} else if strings.HasPrefix(line, "Digest:") {
			res.Digest = strings.TrimSpace(strings.TrimPrefix(line, "Digest:"))
		} else if m := stepsRegex.FindStringSubmatch(line); len(m) == 3 {
			res.StepsCount, _ = strconv.Atoi(m[1])
			res.WavesCount, _ = strconv.Atoi(m[2])
		} else if m := topologRegex.FindStringSubmatch(line); len(m) == 2 {
			parts := strings.Split(m[1], "->")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					res.Topological = append(res.Topological, trimmed)
				}
			}
		}
	}

	return res, nil
}

type rawReceipt struct {
	PlanID        string `json:"planId"`
	SchemaVersion string `json:"schemaVersion"`
	PlanDigest    string `json:"planDigest"`
	ExecutionID   string `json:"executionId"`
	Status        string `json:"status"`
	Timestamps    struct {
		StartedAt   string `json:"startedAt"`
		CompletedAt string `json:"completedAt"`
		DurationMs  int64  `json:"durationMs"`
	} `json:"timestamps"`
	Steps map[string]struct {
		Tool         string `json:"tool"`
		Status       string `json:"status"`
		DurationMs   int64  `json:"durationMs"`
		Attempts     int    `json:"attempts"`
		Verification string `json:"verification"`
	} `json:"steps"`
}

func (r *Runner) InspectReceipt(_ context.Context, receiptPath string) (*ReceiptSummary, error) {
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf("receipt file not found %q", filepath.Base(receiptPath)),
			err.Error(),
			"ensure the task executed and generated a valid receipt.json",
			"https://agentrun.dev/docs/troubleshooting#project-x-receipt",
		)
	}

	var raw rawReceipt
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New(
			fmt.Sprintf("invalid receipt JSON %q", filepath.Base(receiptPath)),
			err.Error(),
			"verify the receipt file was produced by Project X without corruption",
			"https://agentrun.dev/docs/troubleshooting#project-x-receipt",
		)
	}

	started, _ := time.Parse(time.RFC3339Nano, raw.Timestamps.StartedAt)
	completed, _ := time.Parse(time.RFC3339Nano, raw.Timestamps.CompletedAt)

	steps := make(map[string]StepSummary, len(raw.Steps))
	for k, v := range raw.Steps {
		steps[k] = StepSummary{
			Tool:         v.Tool,
			Status:       v.Status,
			DurationMs:   v.DurationMs,
			Attempts:     v.Attempts,
			Verification: v.Verification,
		}
	}

	return &ReceiptSummary{
		PlanID:        raw.PlanID,
		SchemaVersion: raw.SchemaVersion,
		PlanDigest:    raw.PlanDigest,
		ExecutionID:   raw.ExecutionID,
		Status:        raw.Status,
		DurationMs:    raw.Timestamps.DurationMs,
		StartedAt:     started,
		CompletedAt:   completed,
		Steps:         steps,
	}, nil
}

func (r *Runner) InspectReceiptCLI(ctx context.Context, receiptPath string) (string, error) {
	cmd := exec.CommandContext(ctx, r.binPath, "inspect", receiptPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", errors.New(
			fmt.Sprintf("receipt inspect failed for %q", filepath.Base(receiptPath)),
			strings.TrimSpace(stderr.String()+"\n"+stdout.String()),
			"verify the receipt file is a valid Project X ExecutionReceipt",
			"https://agentrun.dev/docs/troubleshooting#project-x-receipt",
		)
	}

	return ansiRegex.ReplaceAllString(stdout.String(), ""), nil
}
