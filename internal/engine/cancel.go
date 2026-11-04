package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrRunAlreadyTerminal = errors.New("run is already terminal")

func Cancel(dataDir, id string) error {
	run, err := GetRun(dataDir, id)
	if err != nil {
		return err
	}

	if run.Status == StatusSucceeded || run.Status == StatusFailed || run.Status == StatusCancelled {
		return fmt.Errorf("%w: %s", ErrRunAlreadyTerminal, run.Status)
	}

	if err := SetStatus(run.Dir, StatusCancelled); err != nil {
		return err
	}

	if data, err := os.ReadFile(filepath.Join(run.Dir, containerNameFile)); err == nil {
		_ = exec.Command("container", "stop", strings.TrimSpace(string(data))).Run()
	}

	pidPath := filepath.Join(run.Dir, "agent.pid")
	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGTERM)
			_ = syscall.Kill(pid, syscall.SIGTERM)
			time.Sleep(200 * time.Millisecond)
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	return nil
}
