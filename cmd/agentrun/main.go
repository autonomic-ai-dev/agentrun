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

const usage = `usage: agentrun <command> [flags]

Commands:
  submit "<task>"   start a run (required flags: --agent, --agent-cmd, --image, --egress-image)
  status <id>       print a run's status
  result <id>       print a run's branch, commit, and changed files
  logs <id>         print the agent's log
  cancel <id>       stop a run

Run "agentrun <command> -h" for the flags of a command.
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
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "submit":
		return runSubmit(ctx, args[1:], stdout, stderr)
	case "status":
		return runRunCommand("status", args[1:], stderr, func(dataDir, id string) error {
			run, err := engine.GetRun(dataDir, id)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "run %s %s\n", run.ID, run.Status)
			return nil
		})
	case "result":
		return runRunCommand("result", args[1:], stderr, func(dataDir, id string) error {
			res, err := engine.GetResult(dataDir, id)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "run %s %s\nbranch %s\ncommit %s\n", res.ID, res.Status, res.Branch, res.Commit)
			for _, file := range res.ChangedFiles {
				fmt.Fprintf(stdout, "changed %s\n", file)
			}
			return nil
		})
	case "logs":
		return runRunCommand("logs", args[1:], stderr, func(dataDir, id string) error {
			run, err := engine.GetRun(dataDir, id)
			if err != nil {
				return err
			}
			log, err := os.ReadFile(filepath.Join(run.Dir, "logs", "agent.log"))
			if err != nil {
				return err
			}
			_, err = stdout.Write(log)
			return err
		})
	case "cancel":
		return runRunCommand("cancel", args[1:], stderr, func(dataDir, id string) error {
			if err := engine.Cancel(dataDir, id); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "run %s cancelled\n", id)
			return nil
		})
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

func runRunCommand(name string, args []string, stderr io.Writer, do func(dataDir, id string) error) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data", "", "data directory; defaults to AGENTRUN_DATA or ~/.local/share/agentrun")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "usage: agentrun %s [-data dir] <run id>\n", name)
		return 2
	}
	if err := do(*dataDir, fs.Arg(0)); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runSubmit(ctx context.Context, args []string, stdout, stderr io.Writer) int {
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
	if err := fs.Parse(args); err != nil {
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
