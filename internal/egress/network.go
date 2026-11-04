package egress

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const InternalSubnet = "192.168.128.0/24"

func EnsureInternalNetwork(ctx context.Context, name string) error {
	list, err := exec.CommandContext(ctx, "container", "network", "list").Output()
	if err != nil {
		return fmt.Errorf("list container networks: %w", err)
	}
	for _, line := range strings.Split(string(list), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == name {
			return nil
		}
	}

	out, err := exec.CommandContext(ctx, "container", "network", "create", "--internal", "--subnet", InternalSubnet, name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create internal network %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}
