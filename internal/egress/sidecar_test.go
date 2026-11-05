package egress_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentrun/internal/egress"
)

func TestExtractInternalIP(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		network string
		want    string
		wantErr error
	}{
		{
			name:    "extracts the address of the named network",
			json:    `[{"status":{"networks":[{"network":"default","ipv4Address":"192.168.64.5/24"},{"network":"agentrun-internal-7f3a2b","ipv4Address":"192.168.129.2/24"}]}}]`,
			network: "agentrun-internal-7f3a2b",
			want:    "192.168.129.2",
		},
		{
			name:    "ignores addresses on other networks",
			json:    `[{"status":{"networks":[{"network":"agentrun-internal-7f3a2b","ipv4Address":"192.168.129.2/24"},{"network":"default","ipv4Address":"192.168.64.5/24"}]}}]`,
			network: "default",
			want:    "192.168.64.5",
		},
		{
			name:    "fails on invalid json",
			json:    `{not valid json`,
			network: "default",
			wantErr: errors.New("parse inspect JSON"),
		},
		{
			name:    "fails when the named network is not attached",
			json:    `[{"status":{"networks":[{"network":"default","ipv4Address":"192.168.64.5/24"}]}}]`,
			network: "agentrun-internal-7f3a2b",
			wantErr: egress.ErrInternalIPNotFound,
		},
		{
			name:    "fails when the attachment has no address",
			json:    `[{"status":{"networks":[{"network":"agentrun-internal-7f3a2b","ipv4Address":""}]}}]`,
			network: "agentrun-internal-7f3a2b",
			wantErr: egress.ErrInternalIPNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := egress.ExtractInternalIP([]byte(tt.json), tt.network)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStartSidecarValidation(t *testing.T) {
	ctx := context.Background()
	_, err := egress.StartSidecar(ctx, egress.SidecarConfig{
		Allow: []string{"example.com:443"},
	})
	if !errors.Is(err, egress.ErrMissingSidecarName) {
		t.Errorf("expected ErrMissingSidecarName, got %v", err)
	}

	_, err = egress.StartSidecar(ctx, egress.SidecarConfig{
		Name: "sidecar-1",
	})
	if !errors.Is(err, egress.ErrEmptyAllowList) {
		t.Errorf("expected ErrEmptyAllowList, got %v", err)
	}

	_, err = egress.StartSidecar(ctx, egress.SidecarConfig{
		Name:  "sidecar-1",
		Allow: []string{"example.com:443"},
	})
	if !errors.Is(err, egress.ErrMissingSidecarImage) {
		t.Errorf("expected ErrMissingSidecarImage, got %v", err)
	}
}

func TestStartAndStopSidecarWithFakeBinary(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "container.log")
	fakeScript := filepath.Join(tempDir, "fake-container")

	scriptContent := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %q
case "$1" in
  run)
    exit 0
    ;;
  inspect)
    echo '[{"status":{"networks":[{"network":"agentrun-internal","ipv4Address":"192.168.128.42/24"}]}}]'
    exit 0
    ;;
  stop|delete)
    exit 0
    ;;
  *)
    exit 1
    ;;
esac
`, logFile)

	if err := os.WriteFile(fakeScript, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cfg := egress.SidecarConfig{
		Binary: fakeScript,
		Name:   "test-sidecar",
		Image:  "agentrun-egress:0.1.0-test",
		Allow:  []string{"api.github.com:443"},
		Port:   3128,
	}

	sidecar, err := egress.StartSidecar(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSidecar failed: %v", err)
	}

	if sidecar.InternalIP != "192.168.128.42" {
		t.Errorf("InternalIP = %q, want 192.168.128.42", sidecar.InternalIP)
	}
	if sidecar.ProxyURL != "http://192.168.128.42:3128" {
		t.Errorf("ProxyURL = %q, want http://192.168.128.42:3128", sidecar.ProxyURL)
	}

	if err := sidecar.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	loggedBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	logged := string(loggedBytes)
	if !strings.Contains(logged, "run -d --name test-sidecar --network default --network agentrun-internal") {
		t.Errorf("missing expected run invocation in log: %s", logged)
	}
	if !strings.Contains(logged, "inspect test-sidecar") {
		t.Errorf("missing expected inspect invocation in log: %s", logged)
	}
	if !strings.Contains(logged, "stop -t 2 test-sidecar") {
		t.Errorf("missing expected stop invocation in log: %s", logged)
	}
	if !strings.Contains(logged, "delete test-sidecar") {
		t.Errorf("missing expected delete invocation in log: %s", logged)
	}
}
