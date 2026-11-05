package egress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var (
	ErrMissingSidecarName  = errors.New("sidecar container name is required")
	ErrMissingSidecarImage = errors.New("sidecar image is required")
	ErrEmptyAllowList      = errors.New("sidecar allow list cannot be empty")
	ErrInternalIPNotFound  = errors.New("internal network IP address not found")
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

type inspectEntry struct {
	Status struct {
		Networks []struct {
			Network     string `json:"network"`
			IPv4Address string `json:"ipv4Address"`
		} `json:"networks"`
	} `json:"status"`
}

func ExtractInternalIP(inspectData []byte, network string) (string, error) {
	var entries []inspectEntry
	if err := json.Unmarshal(inspectData, &entries); err != nil {
		return "", fmt.Errorf("parse inspect JSON: %w", err)
	}
	for _, entry := range entries {
		for _, attachment := range entry.Status.Networks {
			addr, _, _ := strings.Cut(attachment.IPv4Address, "/")
			if attachment.Network == network && addr != "" {
				return addr, nil
			}
		}
	}
	return "", ErrInternalIPNotFound
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
		return nil, ErrMissingSidecarImage
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
			ip, ipErr := ExtractInternalIP(inspectOut, intNet)
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
