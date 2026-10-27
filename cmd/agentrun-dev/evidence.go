package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agentrun/internal/errors"
	"agentrun/internal/projectx"
)

type evidenceFile struct {
	Status     string            `json:"status"`
	PlanDigest string            `json:"plan_digest,omitempty"`
	Attempts   []evidenceAttempt `json:"attempts"`
}

type evidenceAttempt struct {
	Number   int             `json:"number"`
	AgentErr string          `json:"agent_err"`
	Results  []evidenceCheck `json:"results"`
}

type evidenceCheck struct {
	Name     string `json:"Name"`
	Passed   bool   `json:"Passed"`
	ExitCode int    `json:"ExitCode"`
}

func evidence(stdout io.Writer, _ bool, args []string) error {
	if len(args) != 1 {
		return errors.New(
			"no run directory given",
			"evidence reads outcome.json from exactly one directory",
			"run agentrun-dev evidence <dir>",
			"https://agentrun.dev/docs/troubleshooting#evidence",
		)
	}
	dir := args[0]

	data, err := os.ReadFile(filepath.Join(dir, "outcome.json"))
	if os.IsNotExist(err) {
		return errors.New(
			fmt.Sprintf("no outcome.json in %q", dir),
			"the directory has no saved verify outcome",
			"run the verify loop in that directory, or pass the directory that holds outcome.json",
			"https://agentrun.dev/docs/troubleshooting#evidence",
		)
	}
	if err != nil {
		return fmt.Errorf("read outcome.json: %w", err)
	}

	var file evidenceFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("decode outcome.json: %w", err)
	}

	var receiptSummary *projectx.ReceiptSummary
	receiptPath := filepath.Join(dir, "receipt.json")
	if _, err := os.Stat(receiptPath); err == nil {
		runner := projectx.NewRunner("")
		if s, err := runner.InspectReceipt(context.Background(), receiptPath); err == nil {
			receiptSummary = s
		}
	}

	_, err = io.WriteString(stdout, formatEvidence(file, receiptSummary))
	return err
}

func formatEvidence(file evidenceFile, receipt *projectx.ReceiptSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "status: %s\n", file.Status)
	if file.PlanDigest != "" {
		fmt.Fprintf(&b, "plan_digest: %s\n", file.PlanDigest)
	}
	for _, a := range file.Attempts {
		fmt.Fprintf(&b, "attempt %d\n", a.Number)
		if a.AgentErr != "" {
			fmt.Fprintf(&b, "  agent: %s\n", a.AgentErr)
		}
		for _, c := range a.Results {
			verdict := "fail"
			if c.Passed {
				verdict = "pass"
			}
			fmt.Fprintf(&b, "  %s: %s (exit %d)\n", c.Name, verdict, c.ExitCode)
		}
	}
	if receipt != nil {
		fmt.Fprintf(&b, "project-x receipt: %s (status: %s, duration: %dms)\n", receipt.PlanID, receipt.Status, receipt.DurationMs)
		if receipt.PlanDigest != "" {
			fmt.Fprintf(&b, "  digest: %s\n", receipt.PlanDigest)
		}
		keys := make([]string, 0, len(receipt.Steps))
		for k := range receipt.Steps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, id := range keys {
			s := receipt.Steps[id]
			fmt.Fprintf(&b, "  step %s: %s (tool: %s, duration: %dms, verified: %s)\n", id, s.Status, s.Tool, s.DurationMs, s.Verification)
		}
	}
	return b.String()
}
