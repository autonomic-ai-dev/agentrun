package egress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var (
	ErrMissingSidecarName  = errors.New("sidecar container name is required")
	ErrEmptyAllowList      = errors.New("sidecar allow list cannot be empty")
	ErrInternalIPNotFound  = errors.New("internal network IP address (192.168.128.x) not found")
	ErrSidecarStartTimeout = errors.New("timed out waiting for sidecar to start")
)

type SidecarConfig struct {
	Binary          string
	Name            string
	Image           string
	Allow           []string
	DefaultNetwork  string
	InternalNetwork string
	Port            int
}

type Sidecar struct {
	Name       string
	InternalIP string
	ProxyURL   string
	binary     string
}

func ExtractInternalIP(inspectData []byte) (string, error) {
	var raw any
	if err := json.Unmarshal(inspectData, &raw); err != nil {
		return "", fmt.Errorf("parse inspect JSON: %w", err)
	}

	re := regexp.MustCompile(`\b192\.168\.128\.([2-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-4])\b`)
	loc := re.Find(inspectData)
	if loc == nil {
		return "", ErrInternalIPNotFound
	}
	return string(loc), nil
}

func StartSidecar(ctx context.Context, cfg SidecarConfig) (*Sidecar, error) {
	if cfg.Name == "" {
		return nil, ErrMissingSidecarName
	}
	if len(cfg.Allow) == 0 {
		return nil, ErrEmptyAllowList
	}
	bin := cfg.Binary
	if bin == "" {
		bin = "container"
	}
	image := cfg.Image
	if image == "" {
		image = "agentrun-egress:latest"
	}
	defNet := cfg.DefaultNetwork
	if defNet == "" {
		defNet = "default"
	}
	intNet := cfg.InternalNetwork
	if intNet == "" {
		intNet = "agentrun-internal"
	}
	port := cfg.Port
	if port <= 0 {
		port = 3128
	}

	runArgs := []string{
		"run", "-d",
		"--name", cfg.Name,
		"--network", defNet,
		"--network", intNet,
		image,
		"-listen", fmt.Sprintf(":%d", port),
	}
	for _, allow := range cfg.Allow {
		runArgs = append(runArgs, "-allow", allow)
	}

	cmd := exec.CommandContext(ctx, bin, runArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("start sidecar container %s: %s: %w", cfg.Name, strings.TrimSpace(string(out)), err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		if ctx.Err() != nil {
			_ = exec.Command(bin, "delete", "-f", cfg.Name).Run()
			return nil, ctx.Err()
		}

		inspectCmd := exec.CommandContext(ctx, bin, "inspect", cfg.Name)
		inspectOut, err := inspectCmd.Output()
		if err == nil {
			ip, ipErr := ExtractInternalIP(inspectOut)
			if ipErr == nil && ip != "" {
				return &Sidecar{
					Name:       cfg.Name,
					InternalIP: ip,
					ProxyURL:   fmt.Sprintf("http://%s:%d", ip, port),
					binary:     bin,
				}, nil
			}
		}

		if time.Now().After(deadline) {
			_ = exec.Command(bin, "delete", "-f", cfg.Name).Run()
			return nil, ErrSidecarStartTimeout
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Sidecar) Stop(ctx context.Context) error {
	stopCmd := exec.CommandContext(ctx, s.binary, "stop", "-t", "2", s.Name)
	_ = stopCmd.Run()

	delCmd := exec.CommandContext(ctx, s.binary, "delete", s.Name)
	if out, err := delCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("delete sidecar container %s: %s: %w", s.Name, strings.TrimSpace(string(out)), err)
	}
	return nil
}
