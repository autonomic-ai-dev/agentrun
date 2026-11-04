package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GetResult(dataDir, id string) (*Result, error) {
	run, err := GetRun(dataDir, id)
	if err != nil {
		return nil, err
	}

	res := &Result{
		ID:     run.ID,
		Status: run.Status,
		Branch: "agentrun/" + run.ID,
	}

	headCmd := exec.Command("git", "-C", run.Worktree, "rev-parse", "--short", "HEAD")
	if out, err := headCmd.Output(); err == nil {
		res.Commit = strings.TrimSpace(string(out))
	}

	changedCmd := exec.Command("git", "-C", run.Worktree, "diff", "--name-only", run.Base, "HEAD")
	if out, err := changedCmd.Output(); err == nil {
		raw := strings.TrimSpace(string(out))
		if raw != "" {
			res.ChangedFiles = strings.Split(raw, "\n")
		}
	}

	patchPath := filepath.Join(run.Dir, "patch.diff")
	if _, err := os.Stat(patchPath); err == nil {
		res.PatchPath = patchPath
	}

	return res, nil
}
