package engine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	runnererrors "agentrun/internal/errors"
)

var (
	ErrInvalidRunID      = errors.New("invalid run id")
	ErrMissingPrompt     = errors.New("prompt is required")
	ErrNotGitRepo        = errors.New("not a git repository")
	ErrNoCommits         = errors.New("repository has no commits")
	ErrWorktreeFailed    = errors.New("could not create worktree")
	ErrCorruptStatusFile = errors.New("corrupt status file")
)

func DefaultDataDir() string {
	if data := os.Getenv("AGENTRUN_DATA"); data != "" {
		return data
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agentrun")
	}
	return filepath.Join(home, ".local", "share", "agentrun")
}

func RunsDir(dataDir string) string {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	return filepath.Join(dataDir, "runs")
}

func ValidID(id string) bool {
	if len(id) != 6 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func GenerateID(runsDir string) (string, error) {
	var b [3]byte
	for i := 0; i < 100; i++ {
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		id := hex.EncodeToString(b[:])
		if _, err := os.Stat(filepath.Join(runsDir, id)); os.IsNotExist(err) {
			return id, nil
		}
	}
	return "", errors.New("failed to generate unique run id")
}

func SetStatus(runDir string, status Status) error {
	tmpFile := filepath.Join(runDir, fmt.Sprintf("status.tmp.%d", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte(string(status)+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filepath.Join(runDir, "status"))
}

func GetStatus(runDir string) (Status, error) {
	data, err := os.ReadFile(filepath.Join(runDir, "status"))
	if err != nil {
		return "", err
	}
	status := Status(strings.TrimSpace(string(data)))
	switch status {
	case StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusCancelled:
		return status, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrCorruptStatusFile, status)
	}
}

func CreateRun(cfg SubmitConfig) (*Run, error) {
	if strings.TrimSpace(cfg.Prompt) == "" {
		return nil, ErrMissingPrompt
	}
	repo := cfg.Repo
	if repo == "" {
		var err error
		repo, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}

	checkGitCmd := exec.Command("git", "-C", absRepo, "rev-parse", "--is-inside-work-tree")
	if err := checkGitCmd.Run(); err != nil {
		return nil, ErrNotGitRepo
	}

	checkHeadCmd := exec.Command("git", "-C", absRepo, "rev-parse", "--verify", "-q", "HEAD")
	baseOut, err := checkHeadCmd.Output()
	if err != nil {
		return nil, ErrNoCommits
	}
	base := strings.TrimSpace(string(baseOut))

	if !cfg.IncludeDirty {
		statusCmd := exec.Command("git", "-C", absRepo, "status", "--porcelain")
		statusOut, err := statusCmd.Output()
		if err != nil {
			return nil, err
		}
		if len(strings.TrimSpace(string(statusOut))) > 0 {
			return nil, runnererrors.Catalog()[runnererrors.ErrDirtyWorkingTree]
		}
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	runsDir := RunsDir(dataDir)
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		return nil, err
	}

	id, err := GenerateID(runsDir)
	if err != nil {
		return nil, err
	}

	runDir := filepath.Join(runsDir, id)
	logsDir := filepath.Join(runDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, err
	}

	worktreeDir := filepath.Join(runDir, "worktree")
	branchName := "agentrun/" + id
	worktreeCmd := exec.Command("git", "-C", absRepo, "-c", "core.hooksPath=/dev/null", "worktree", "add", "-q", "-b", branchName, worktreeDir, base)
	if out, err := worktreeCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(runDir)
		return nil, fmt.Errorf("%w: %s", ErrWorktreeFailed, strings.TrimSpace(string(out)))
	}

	if err := os.WriteFile(filepath.Join(runDir, "task.md"), []byte(cfg.Prompt+"\n"), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(runDir, "base"), []byte(base+"\n"), 0644); err != nil {
		return nil, err
	}

	if err := SetStatus(runDir, StatusQueued); err != nil {
		return nil, err
	}

	return &Run{
		ID:       id,
		Dir:      runDir,
		Task:     cfg.Prompt,
		Base:     base,
		Status:   StatusQueued,
		Worktree: worktreeDir,
	}, nil
}

func GetRun(dataDir, id string) (*Run, error) {
	if !ValidID(id) {
		return nil, ErrInvalidRunID
	}
	runDir := filepath.Join(RunsDir(dataDir), id)
	if fi, err := os.Stat(runDir); err != nil || !fi.IsDir() {
		return nil, runnererrors.Catalog()[runnererrors.ErrRunNotFound]
	}

	status, err := GetStatus(runDir)
	if err != nil {
		return nil, err
	}

	taskBytes, err := os.ReadFile(filepath.Join(runDir, "task.md"))
	if err != nil {
		return nil, err
	}
	baseBytes, err := os.ReadFile(filepath.Join(runDir, "base"))
	if err != nil {
		return nil, err
	}

	return &Run{
		ID:       id,
		Dir:      runDir,
		Task:     strings.TrimSpace(string(taskBytes)),
		Base:     strings.TrimSpace(string(baseBytes)),
		Status:   status,
		Worktree: filepath.Join(runDir, "worktree"),
	}, nil
}

func ListRuns(dataDir string) ([]*Run, error) {
	runsDir := RunsDir(dataDir)
	entries, err := os.ReadDir(runsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var runs []*Run
	for _, entry := range entries {
		if !entry.IsDir() || !ValidID(entry.Name()) {
			continue
		}
		run, err := GetRun(dataDir, entry.Name())
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].ID < runs[j].ID
	})

	return runs, nil
}
