package egress

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func CreateInternalNetwork(ctx context.Context, name string) error {
	out, err := exec.CommandContext(ctx, "container", "network", "create", "--internal", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create internal network %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func DeleteNetwork(ctx context.Context, name string) error {
	out, err := exec.CommandContext(ctx, "container", "network", "delete", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete network %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}
