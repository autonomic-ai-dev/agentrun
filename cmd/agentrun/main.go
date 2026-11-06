package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"agentrun/internal/agent"
	"agentrun/internal/container"
	"agentrun/internal/container/apple"
	"agentrun/internal/engine"
	"agentrun/internal/verify"
)

const usage = `usage: agentrun submit [flags] "<task>"

Required flags: --agent, --agent-cmd, --image, --egress-image.
Run "agentrun submit -h" for every flag.
`

var errUnknownAgent = errors.New("unknown agent type")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

type checkList []verify.Check

func (c *checkList) String() string {
	return fmt.Sprint(*c)
}

func (c *checkList) Set(value string) error {
	name, command, ok := strings.Cut(value, "=")
	if !ok || name == "" || command == "" {
		return fmt.Errorf("check %q must be name=command", value)
	}
	*c = append(*c, verify.Check{Name: name, Command: command})
	return nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if len(args) == 0 || args[0] != "submit" {
		fmt.Fprint(stderr, usage)
		return 2
	}

	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}
	repo := fs.String("repo", ".", "git repository to run against")
	includeDirty := fs.Bool("include-dirty", false, "start from HEAD even when the repository has uncommitted changes")
	agentName := fs.String("agent", "", "agent type: claude, agy, or cursor")
	agentCmd := fs.String("agent-cmd", "", "shell command that reads the prompt on stdin, run inside the agent container")
	image := fs.String("image", "", "agent image reference with an explicit tag or digest")
	egressImage := fs.String("egress-image", "", "egress sidecar image reference with an explicit tag")
	network := fs.String("network", "agentrun-internal", "prefix for each run's internal network, named <prefix>-<run id>")
	memory := fs.String("memory", "", "memory limit for each container, e.g. 1G")
	user := fs.String("user", "", "UID:GID for containers; defaults to the current user")
	maxAttempts := fs.Int("max-attempts", 3, "attempts before the run is marked failed")
	dataDir := fs.String("data", "", "data directory; defaults to AGENTRUN_DATA or ~/.local/share/agentrun")
	var checks checkList
	fs.Var(&checks, "check", "name=command run in a container after each attempt; repeatable")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "submit takes exactly one task argument")
		return 2
	}
	required := []struct{ name, value string }{
		{"agent", *agentName},
		{"agent-cmd", *agentCmd},
		{"image", *image},
		{"egress-image", *egressImage},
	}
	for _, r := range required {
		if r.value == "" {
			fmt.Fprintf(stderr, "submit needs --%s\n", r.name)
			return 2
		}
	}

	allow := agent.AllowlistFor(agent.AgentType(*agentName))
	if allow == nil {
		fmt.Fprintf(stderr, "%v: %q\n", errUnknownAgent, *agentName)
		return 2
	}
	if *user == "" {
		*user = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}

	created, err := engine.CreateRun(engine.SubmitConfig{
		Prompt:       fs.Arg(0),
		Repo:         *repo,
		IncludeDirty: *includeDirty,
		DataDir:      *dataDir,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	execErr := engine.Execute(ctx, created, engine.ExecutionOptions{
		AgentCmd:    *agentCmd,
		Checks:      checks,
		MaxAttempts: *maxAttempts,
		Container: &engine.ContainerOptions{
			NewBackend: func(output io.Writer) container.Backend {
				return &apple.Apple{Binary: "container", Output: output}
			},
			Image:  *image,
			User:   *user,
			Memory: *memory,
			Egress: &engine.EgressOptions{Image: *egressImage, Network: *network, Allow: allow},
		},
	})

	status, statusErr := engine.GetStatus(created.Dir)
	if statusErr != nil {
		fmt.Fprintln(stderr, statusErr)
		return 1
	}
	fmt.Fprintf(stdout, "run %s %s\nbranch agentrun/%s\nlogs %s\n", created.ID, status, created.ID, filepath.Join(created.Dir, "logs"))
	if execErr != nil {
		fmt.Fprintln(stderr, execErr)
	}
	if status != engine.StatusSucceeded {
		return 1
	}
	return 0
}
