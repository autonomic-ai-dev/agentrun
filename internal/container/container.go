package container

import (
	"context"
	"errors"
)

var (
	ErrRootUser       = errors.New("container user must not be root")
	ErrEmptyUser      = errors.New("container user is empty")
	ErrWritableRoot   = errors.New("container root filesystem must be read-only")
	ErrMissingTmpfs   = errors.New("container must mount tmpfs at /tmp")
	ErrNoNetwork      = errors.New("container network is empty")
	ErrGitDirMounted  = errors.New("container must not mount a .git directory")
	ErrLatestImage    = errors.New("container image must not use the latest tag or no tag")
	ErrEmptyImage     = errors.New("container image is empty")
	ErrEmptyCommand   = errors.New("container command is empty")
	ErrEmptyMountPath = errors.New("container mount path is empty")
	ErrBadMountPath   = errors.New("container mount path contains a comma or equals sign")
	ErrBadLabel       = errors.New("container label key or value is empty or contains a comma or equals sign")
	ErrBadName        = errors.New("container name contains a comma or equals sign")
	ErrBadEnv         = errors.New("container environment variable key is empty or contains an equals sign")
)

type Mount struct {
	Host     string
	Guest    string
	ReadOnly bool
}

type Spec struct {
	Image    string
	User     string
	ReadOnly bool
	Tmpfs    []string
	Mounts   []Mount
	Network  string
	Networks []string
	Name     string
	Labels   map[string]string
	Memory   string
	Env      map[string]string
	Cmd      []string
}

type Result struct {
	ExitCode int
}

type Backend interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}
