package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"agentrun/internal/mode"
)

const (
	digestPrefix = "sha256:"
	hostChars    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-_"
)

type Provenance struct {
	Mode         mode.Mode `json:"mode"`
	Host         string    `json:"host"`
	PolicyDigest string    `json:"policy_digest"`
	PlanDigest   string    `json:"plan_digest,omitempty"`
}

func NewProvenance(m mode.Mode, host, policyDigest string) (Provenance, error) {
	if _, err := mode.Parse(string(m)); err != nil {
		return Provenance{}, err
	}
	if !validHost(host) {
		return Provenance{}, errors.New("provenance host must be a non-empty hostname of letters, digits, dots, dashes, or underscores")
	}
	if !validDigest(policyDigest) {
		return Provenance{}, errors.New("provenance policy digest must be sha256 followed by 64 lowercase hex characters")
	}
	return Provenance{Mode: m, Host: host, PolicyDigest: policyDigest}, nil
}

func (p Provenance) WithPlanDigest(planDigest string) (Provenance, error) {
	if planDigest == "" {
		p.PlanDigest = ""
		return p, nil
	}
	normalized := planDigest
	if !strings.HasPrefix(normalized, digestPrefix) {
		normalized = digestPrefix + normalized
	}
	if !validDigest(normalized) {
		return Provenance{}, errors.New("provenance plan digest must be sha256 followed by 64 lowercase hex characters")
	}
	p.PlanDigest = normalized
	return p, nil
}

func (p *Provenance) UnmarshalJSON(data []byte) error {
	type plain Provenance
	var raw plain
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	validated, err := NewProvenance(raw.Mode, raw.Host, raw.PolicyDigest)
	if err != nil {
		return err
	}
	if raw.PlanDigest != "" {
		validated, err = validated.WithPlanDigest(raw.PlanDigest)
		if err != nil {
			return err
		}
	}
	*p = validated
	return nil
}

func validHost(host string) bool {
	if host == "" {
		return false
	}
	for _, r := range host {
		if !strings.ContainsRune(hostChars, r) {
			return false
		}
	}
	return true
}

func validDigest(d string) bool {
	hexPart, ok := strings.CutPrefix(d, digestPrefix)
	if !ok || strings.ToLower(hexPart) != hexPart {
		return false
	}
	b, err := hex.DecodeString(hexPart)
	return err == nil && len(b) == sha256.Size
}
