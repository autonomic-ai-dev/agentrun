//go:build capability

package capability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	image          = "alpine:3.20"
	pinnedImage    = "alpine@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc"
	commandTimeout = 2 * time.Minute
	pollInterval   = 200 * time.Millisecond
	waitTimeout    = 30 * time.Second
)

type result struct {
	stdout string
	stderr string
	exit   int
}

func cli(t *testing.T, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "container", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return result{stdout: stdout.String(), stderr: stderr.String()}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return result{stdout: stdout.String(), stderr: stderr.String(), exit: exitErr.ExitCode()}
	}
	t.Fatalf("container %v could not run: %v", args, err)
	return result{}
}

func newName(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("spike-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cli(t, "stop", "-t", "1", name)
		cli(t, "delete", name)
	})
	return name
}

type containerView struct {
	Configuration struct {
		Labels    map[string]string `json:"labels"`
		Resources struct {
			MemoryInBytes int64 `json:"memoryInBytes"`
		} `json:"resources"`
	} `json:"configuration"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

func inspect(t *testing.T, name string) containerView {
	t.Helper()
	out := cli(t, "inspect", name).stdout
	var views []containerView
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("inspect %s is not JSON: %v\n%s", name, err, out)
	}
	if len(views) != 1 {
		t.Fatalf("inspect %s returned %d entries, want 1", name, len(views))
	}
	return views[0]
}

func waitForState(t *testing.T, name, want string) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for {
		if got := inspect(t, name).Status.State; got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("container %s did not reach state %q within %s", name, want, waitTimeout)
		}
		time.Sleep(pollInterval)
	}
}

func TestServiceIsRunning(t *testing.T) {
	if out := cli(t, "system", "status").stdout; !strings.Contains(out, "running") {
		t.Fatalf("container service is not running:\n%s", out)
	}
}

func TestListingIsMachineReadableJSON(t *testing.T) {
	out := cli(t, "list", "--all", "--format", "json").stdout

	var views []containerView
	if err := json.Unmarshal([]byte(out), &views); err != nil {
		t.Fatalf("list --format json is not parseable: %v", err)
	}
}

func TestForegroundExitCodePropagates(t *testing.T) {
	name := newName(t)

	got := cli(t, "run", "--name", name, "--rm", image, "sh", "-c", "exit 7")

	if got.exit != 7 {
		t.Errorf("exit code = %d, want 7", got.exit)
	}
}

func TestDetachedLogsCaptureStdoutAndStderr(t *testing.T) {
	name := newName(t)
	cli(t, "run", "-d", "--name", name, image, "sh", "-c", "echo out-line; echo err-line >&2")
	waitForState(t, name, "stopped")

	got := cli(t, "logs", name)
	logs := got.stdout + got.stderr
	for _, want := range []string{"out-line", "err-line"} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs missing %q:\n%s", want, logs)
		}
	}
}

func TestLabelsAreStoredAndReadableFromInspect(t *testing.T) {
	name := newName(t)
	cli(t, "run", "-d", "--name", name,
		"--label", "agentrun.run=r-42",
		"--label", "agentrun.attempt=2",
		image, "sleep", "60")

	labels := inspect(t, name).Configuration.Labels

	if labels["agentrun.run"] != "r-42" || labels["agentrun.attempt"] != "2" {
		t.Errorf("labels = %v, want agentrun.run=r-42 and agentrun.attempt=2", labels)
	}
}

func TestDetachedContainerKeepsRunningAfterClientExits(t *testing.T) {
	name := newName(t)
	cli(t, "run", "-d", "--name", name, image, "sleep", "60")

	time.Sleep(2 * time.Second)

	if got := inspect(t, name).Status.State; got != "running" {
		t.Errorf("state = %q after client exit, want running", got)
	}
}

func TestDetachedExitStatusIsUnavailableFromInspect(t *testing.T) {
	name := newName(t)
	cli(t, "run", "-d", "--name", name, image, "sh", "-c", "exit 5")
	waitForState(t, name, "stopped")

	raw := strings.ToLower(cli(t, "inspect", name).stdout)
	for _, key := range []string{`"exitcode"`, `"exitstatus"`} {
		if strings.Contains(raw, key) {
			t.Errorf("inspect now exposes %s; the PRD gap may be closed, re-verify", key)
		}
	}
}

func TestMemoryLimitIsAppliedToContainer(t *testing.T) {
	name := newName(t)
	cli(t, "run", "-d", "--name", name, "-c", "1", "-m", "256M", image, "sleep", "60")

	got := inspect(t, name).Configuration.Resources.MemoryInBytes
	if want := int64(256 << 20); got != want {
		t.Errorf("memoryInBytes = %d, want %d", got, want)
	}
}

func TestMemoryBelowMinimumIsRejected(t *testing.T) {
	name := newName(t)

	got := cli(t, "run", "--name", name, "-m", "128M", image, "true")

	if got.exit == 0 {
		t.Fatal("run with 128M memory succeeded; the 200 MiB minimum is no longer enforced")
	}
	if !strings.Contains(got.stderr, "minimum memory") {
		t.Errorf("stderr does not name the minimum:\n%s", got.stderr)
	}
}

func TestRunawayMemoryIsKilledWithExit137(t *testing.T) {
	name := newName(t)
	hog := "x=$(dd if=/dev/zero bs=1M count=400 2>/dev/null | tr '\\0' a); echo \"len=${#x}\""

	got := cli(t, "run", "--name", name, "--rm", "-m", "200M", image, "sh", "-c", hog)

	if got.exit != 137 {
		t.Errorf("exit code = %d, want 137 (OOM kill)", got.exit)
	}
}

func TestWorktreeMountIsWritableAndReadOnlyMountRejectsWrites(t *testing.T) {
	name := newName(t)
	host := t.TempDir()
	work := filepath.Join(host, "work")
	ro := filepath.Join(host, "ro")
	for _, dir := range []string{work, ro} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	script := "echo ok > /work/w.txt && echo WORK_OK; echo x > /ro/x.txt 2>/dev/null || echo RO_BLOCKED"
	got := cli(t, "run", "--name", name, "--rm",
		"--mount", "type=bind,source="+work+",target=/work",
		"--mount", "type=bind,source="+ro+",target=/ro,readonly",
		image, "sh", "-c", script)

	if !strings.Contains(got.stdout, "WORK_OK") || !strings.Contains(got.stdout, "RO_BLOCKED") {
		t.Fatalf("mount semantics wrong:\n%s\n%s", got.stdout, got.stderr)
	}
	if _, err := os.Stat(filepath.Join(work, "w.txt")); err != nil {
		t.Errorf("write through worktree mount not visible on host: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ro, "x.txt")); err == nil {
		t.Error("read-only mount accepted a write on the host")
	}
}

func TestReadOnlyRootfsBlocksWritesOutsideMounts(t *testing.T) {
	name := newName(t)

	got := cli(t, "run", "--name", name, "--rm", "--read-only", image,
		"sh", "-c", "echo x > /etc/x 2>/dev/null && echo ROOTFS_WRITABLE || echo ROOTFS_RO")

	if !strings.Contains(got.stdout, "ROOTFS_RO") {
		t.Errorf("root filesystem is writable under --read-only:\n%s", got.stdout)
	}
}

func TestTmpfsProvidesWritableTmpUnderReadOnlyRoot(t *testing.T) {
	name := newName(t)

	got := cli(t, "run", "--name", name, "--rm", "--read-only", "--tmpfs", "/tmp", image,
		"sh", "-c", "echo t > /tmp/t && echo TMP_OK")

	if !strings.Contains(got.stdout, "TMP_OK") {
		t.Errorf("tmpfs /tmp is not writable:\n%s\n%s", got.stdout, got.stderr)
	}
}

func TestNonRootUserCannotWriteRootOwnedPaths(t *testing.T) {
	name := newName(t)

	got := cli(t, "run", "--name", name, "--rm", "--read-only", "--tmpfs", "/tmp",
		"-u", "1000:1000", image, "sh", "-c",
		"id -u; echo x > /etc/x 2>/dev/null && echo ETC_WRITABLE || echo ETC_BLOCKED")

	if !strings.Contains(got.stdout, "1000") || !strings.Contains(got.stdout, "ETC_BLOCKED") {
		t.Errorf("non-root user not enforced:\n%s", got.stdout)
	}
}

func TestDigestPinnedImageRuns(t *testing.T) {
	name := newName(t)

	got := cli(t, "run", "--name", name, "--rm", pinnedImage, "echo", "PINNED_OK")

	if !strings.Contains(got.stdout, "PINNED_OK") {
		t.Errorf("digest-pinned run failed:\n%s\n%s", got.stdout, got.stderr)
	}
}

func TestInternalNetworkBlocksEgress(t *testing.T) {
	network := fmt.Sprintf("spike-net-%d", time.Now().UnixNano())
	cli(t, "network", "create", "--internal", network)
	t.Cleanup(func() { cli(t, "network", "delete", network) })
	name := newName(t)

	got := cli(t, "run", "--name", name, "--rm", "--network", network, image,
		"sh", "-c", "wget -T 5 -qO- http://1.1.1.1 >/dev/null 2>&1 && echo EGRESS_OK || echo EGRESS_BLOCKED")

	if !strings.Contains(got.stdout, "EGRESS_BLOCKED") {
		t.Errorf("internal network allowed egress:\n%s", got.stdout)
	}
}

func TestDefaultNetworkHasNoPerContainerAllowlist(t *testing.T) {
	name := newName(t)

	got := cli(t, "run", "--name", name, "--rm", image,
		"sh", "-c", "wget -T 8 -qO- http://1.1.1.1 >/dev/null 2>&1 && echo ARBITRARY_IP_OK || echo ARBITRARY_IP_BLOCKED")

	if !strings.Contains(got.stdout, "ARBITRARY_IP_OK") {
		t.Errorf("default network blocked an arbitrary destination; re-verify the model-api proxy decision:\n%s", got.stdout)
	}
}

func TestSIGTERMIgnoringProcessIsKilledAtGracePeriod(t *testing.T) {
	name := newName(t)
	cli(t, "run", "-d", "--name", name, image, "sh", "-c", `trap "" TERM; while :; do sleep 1; done`)
	waitForState(t, name, "running")

	start := time.Now()
	cli(t, "stop", "-t", "2", name)
	elapsed := time.Since(start)

	if elapsed > 8*time.Second {
		t.Errorf("stop took %s with a 2s grace period", elapsed)
	}
	if got := inspect(t, name).Status.State; got == "running" {
		t.Errorf("state = %q after forced stop", got)
	}
}

func TestSIGTERMHonoringProcessExitsPromptly(t *testing.T) {
	name := newName(t)
	cli(t, "run", "-d", "--name", name, image, "sh", "-c", `trap "exit 3" TERM; while :; do sleep 1; done`)
	waitForState(t, name, "running")

	start := time.Now()
	cli(t, "stop", "-t", "10", name)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("stop took %s for a process that honors SIGTERM", elapsed)
	}
}

func TestLivePromptAgyThroughEgressProxy(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY is not set; skipping live agy capability test")
	}

	name := newName(t)
	got := cli(t, "run", "--name", name, "--rm",
		"-e", "GEMINI_API_KEY="+key,
		"agent-debian:latest",
		"agy", "-p", "Respond only with PONG to this ping.", "--dangerously-skip-permissions")

	if got.exit != 0 {
		t.Fatalf("agy prompt exited %d:\n%s\n%s", got.exit, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stdout, "PONG") {
		t.Errorf("agy output does not contain PONG:\n%s", got.stdout)
	}
}

func TestLivePromptCursorThroughEgressProxy(t *testing.T) {
	key := os.Getenv("CURSOR_API_KEY")
	if key == "" {
		t.Skip("CURSOR_API_KEY is not set; skipping live cursor capability test")
	}

	name := newName(t)
	got := cli(t, "run", "--name", name, "--rm",
		"-e", "CURSOR_API_KEY="+key,
		"agent-debian:latest",
		"agent", "-p", "Respond only with PONG to this ping.", "--trust", "--mode", "plan")

	if got.exit != 0 {
		t.Fatalf("cursor prompt exited %d:\n%s\n%s", got.exit, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stdout, "PONG") {
		t.Errorf("cursor output does not contain PONG:\n%s", got.stdout)
	}
}
