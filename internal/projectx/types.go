package projectx

import "time"

type ValidationResult struct {
	PlanName    string   `json:"plan_name"`
	APIVersion  string   `json:"api_version"`
	Digest      string   `json:"digest"`
	StepsCount  int      `json:"steps_count"`
	WavesCount  int      `json:"waves_count"`
	Topological []string `json:"topological"`
}

type StepSummary struct {
	Tool         string `json:"tool"`
	Status       string `json:"status"`
	DurationMs   int64  `json:"duration_ms"`
	Attempts     int    `json:"attempts"`
	Verification string `json:"verification"`
}

type ReceiptSummary struct {
	PlanID        string                 `json:"plan_id"`
	SchemaVersion string                 `json:"schema_version"`
	PlanDigest    string                 `json:"plan_digest"`
	ExecutionID   string                 `json:"execution_id"`
	Status        string                 `json:"status"`
	DurationMs    int64                  `json:"duration_ms"`
	StartedAt     time.Time              `json:"started_at"`
	CompletedAt   time.Time              `json:"completed_at"`
	Steps         map[string]StepSummary `json:"steps"`
}
