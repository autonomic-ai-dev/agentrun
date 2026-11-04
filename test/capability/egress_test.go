//go:build capability

package capability_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agentrun/internal/engine"
	"agentrun/internal/verify"
)

const egressTestImage = "agentrun-egress:0.1.0-test"

func buildEgressImage(t *testing.T) string {
	t.Helper()
	ctxDir := t.TempDir()

	build := exec.Command("go", "build", "-o", filepath.Join(ctxDir, "agentrun-egress"), "../../cmd/agentrun-egress")
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build egress binary: %s: %v", out, err)
	}

	dockerfile, err := os.ReadFile("../../images/egress/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctxDir, "Dockerfile"), dockerfile, 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command("container", "build", "-t", egressTestImage, ctxDir).CombinedOutput(); err != nil {
		t.Fatalf("build egress image: %s: %v", out, err)
	}
	return egressTestImage
}

const egressProbe = `cat >/dev/null
P=${HTTPS_PROXY#http://}; H=${P%:*}; PORT=${P##*:}
echo "PROXY $HTTPS_PROXY"
nc -z -w 3 1.1.1.1 443 && echo DIRECT_OPEN || echo DIRECT_BLOCKED
nc -z -w 3 "$H" "$PORT" && echo PROXY_OPEN || echo PROXY_CLOSED
printf 'CONNECT evil.example:443 HTTP/1.1\r\nHost: evil.example:443\r\n\r\n' | nc -w 5 "$H" "$PORT" | head -1 | sed 's/^/DENIED /'
printf 'CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n' | nc -w 10 "$H" "$PORT" | head -1 | sed 's/^/ALLOWED /'
true`

func TestEgressSidecarBlocksDirectAndUnlistedTraffic(t *testing.T) {
	image := buildEgressImage(t)
	repo := newRepoWithCommit(t)
	dataDir := t.TempDir()

	run, err := engine.CreateRun(engine.SubmitConfig{Prompt: "probe egress", Repo: repo, DataDir: dataDir})
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	options := appleEngineOptions()
	options.Network = ""
	options.Egress = &engine.EgressOptions{
		Image:   image,
		Network: "agentrun-internal",
		Allow:   []string{"example.com:443"},
	}

	err = engine.Execute(t.Context(), run, engine.ExecutionOptions{
		AgentCmd:    egressProbe,
		Checks:      []verify.Check{{Name: "true", Command: "true"}},
		MaxAttempts: 1,
		Container:   options,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	log := readFileOrEmpty(filepath.Join(run.Dir, "logs", "agent.log"))
	for _, want := range []string{"DIRECT_BLOCKED", "PROXY_OPEN", "DENIED HTTP/1.1 403"} {
		if !strings.Contains(log, want) {
			t.Errorf("agent log lacks %q:\n%s", want, log)
		}
	}
	if !strings.Contains(log, "ALLOWED HTTP/1.1 200") {
		t.Errorf("allowed host was not tunnelled through the sidecar:\n%s", log)
	}
	if strings.Contains(log, "DIRECT_OPEN") {
		t.Errorf("agent reached the internet directly:\n%s", log)
	}
}
