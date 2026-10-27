package record_test

import (
	"encoding/json"
	"strings"
	"testing"

	"agentrun/internal/mode"
	"agentrun/internal/policy"
	"agentrun/internal/record"
)

func TestNewProvenanceCarriesModeHostAndPolicyDigest(t *testing.T) {
	digest := policy.Digest([]byte("network: none"))

	cases := []struct {
		name string
		mode mode.Mode
		host string
	}{
		{name: "lite on local host", mode: mode.Lite, host: "local"},
		{name: "full on shared host", mode: mode.Full, host: "team-mac-mini"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := record.NewProvenance(tc.mode, tc.host, digest)
			if err != nil {
				t.Fatalf("NewProvenance returned error: %v", err)
			}
			if got.Mode != tc.mode || got.Host != tc.host || got.PolicyDigest != digest {
				t.Errorf("NewProvenance = %+v, want mode=%q host=%q digest=%q", got, tc.mode, tc.host, digest)
			}
		})
	}
}

func TestNewProvenanceRejectsInvalidFields(t *testing.T) {
	digest := policy.Digest([]byte("network: none"))
	validHex := strings.Repeat("a", 64)

	cases := []struct {
		name   string
		mode   mode.Mode
		host   string
		digest string
	}{
		{name: "empty host", mode: mode.Lite, host: "", digest: digest},
		{name: "unknown mode", mode: mode.Mode("enterprise"), host: "local", digest: digest},
		{name: "empty mode", mode: mode.Mode(""), host: "local", digest: digest},
		{name: "empty digest", mode: mode.Lite, host: "local", digest: ""},
		{name: "digest without algorithm prefix", mode: mode.Lite, host: "local", digest: validHex},
		{name: "digest too short", mode: mode.Lite, host: "local", digest: "sha256:abcd"},
		{name: "digest too long", mode: mode.Lite, host: "local", digest: "sha256:" + validHex + "00"},
		{name: "digest with non-hex characters", mode: mode.Lite, host: "local", digest: "sha256:" + strings.Repeat("z", 64)},
		{name: "uppercase hex digest names the same hash under a second spelling", mode: mode.Lite, host: "local", digest: "sha256:" + strings.ToUpper(strings.Repeat("ab", 32))},
		{name: "whitespace-only host", mode: mode.Lite, host: "   ", digest: digest},
		{name: "host with trailing space", mode: mode.Lite, host: "local ", digest: digest},
		{name: "host with embedded newline", mode: mode.Lite, host: "team\nfix: rm", digest: digest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := record.NewProvenance(tc.mode, tc.host, tc.digest); err == nil {
				t.Errorf("NewProvenance(%q, %q, %q) accepted invalid input", tc.mode, tc.host, tc.digest)
			}
		})
	}
}

func TestUnmarshalJSONRejectsRecordsThatNewProvenanceRejects(t *testing.T) {
	digest := policy.Digest([]byte("network: none"))

	cases := map[string]string{
		"unknown mode":   `{"mode":"enterprise","host":"local","policy_digest":"` + digest + `"}`,
		"empty mode":     `{"mode":"","host":"local","policy_digest":"` + digest + `"}`,
		"empty host":     `{"mode":"lite","host":"","policy_digest":"` + digest + `"}`,
		"missing digest": `{"mode":"lite","host":"local","policy_digest":""}`,
		"truncated JSON": `{"mode":`,
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			var p record.Provenance
			if err := json.Unmarshal([]byte(raw), &p); err == nil {
				t.Errorf("Unmarshal accepted %s: %+v", name, p)
			}
		})
	}
}

func TestJSONRoundTripPreservesValidProvenance(t *testing.T) {
	digest := policy.Digest([]byte("network: none"))
	want, err := record.NewProvenance(mode.Lite, "local", digest)
	if err != nil {
		t.Fatalf("NewProvenance returned error: %v", err)
	}

	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var got record.Provenance
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal rejected a record it wrote: %v", err)
	}

	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestProvenanceJSONExposesModeHostAndPolicyDigest(t *testing.T) {
	digest := policy.Digest([]byte("network: none"))
	p, err := record.NewProvenance(mode.Full, "team-mac-mini", digest)
	if err != nil {
		t.Fatalf("NewProvenance returned error: %v", err)
	}

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var fields map[string]string
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	want := map[string]string{
		"mode":          "full",
		"host":          "team-mac-mini",
		"policy_digest": digest,
	}
	for key, value := range want {
		if fields[key] != value {
			t.Errorf("json field %q = %q, want %q", key, fields[key], value)
		}
	}
	if len(fields) != len(want) {
		t.Errorf("json has %d fields, want %d: %s", len(fields), len(want), raw)
	}
}

func TestProvenanceWithPlanDigest(t *testing.T) {
	digest := policy.Digest([]byte("network: none"))
	p, err := record.NewProvenance(mode.Lite, "local", digest)
	if err != nil {
		t.Fatalf("NewProvenance returned error: %v", err)
	}

	validHash := strings.Repeat("f", 64)
	withPlan, err := p.WithPlanDigest(validHash)
	if err != nil {
		t.Fatalf("WithPlanDigest failed: %v", err)
	}
	if withPlan.PlanDigest != "sha256:"+validHash {
		t.Errorf("PlanDigest = %q, want 'sha256:%s'", withPlan.PlanDigest, validHash)
	}

	raw, err := json.Marshal(withPlan)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var unmarshaled record.Provenance
	if err := json.Unmarshal(raw, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if unmarshaled.PlanDigest != withPlan.PlanDigest {
		t.Errorf("Roundtrip PlanDigest = %q, want %q", unmarshaled.PlanDigest, withPlan.PlanDigest)
	}

	if _, err := p.WithPlanDigest("invalid-short-hash"); err == nil {
		t.Error("expected error for invalid plan digest, got nil")
	}
}
