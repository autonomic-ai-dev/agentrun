package container_test

import (
	"errors"
	"testing"

	"agentrun/internal/container"
)

func TestSpecValidate(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*container.Spec)
		wantErr    error
		wantAnyErr bool
	}{
		{name: "valid spec", mutate: func(*container.Spec) {}},
		{name: "digest pinned image", mutate: func(s *container.Spec) {
			s.Image = "ghcr.io/example/agent@sha256:abcdef"
		}},
		{name: "tag and digest image", mutate: func(s *container.Spec) {
			s.Image = "ghcr.io/example/agent:1.2.3@sha256:abcdef"
		}},
		{name: "empty image", mutate: func(s *container.Spec) {
			s.Image = ""
		}, wantErr: container.ErrEmptyImage},
		{name: "untagged image", mutate: func(s *container.Spec) {
			s.Image = "ghcr.io/example/agent"
		}, wantErr: container.ErrLatestImage},
		{name: "registry port without tag", mutate: func(s *container.Spec) {
			s.Image = "localhost:5000/agent"
		}, wantErr: container.ErrLatestImage},
		{name: "latest tag", mutate: func(s *container.Spec) {
			s.Image = "ghcr.io/example/agent:latest"
		}, wantErr: container.ErrLatestImage},
		{name: "latest tag with digest", mutate: func(s *container.Spec) {
			s.Image = "ghcr.io/example/agent:latest@sha256:abcdef"
		}, wantErr: container.ErrLatestImage},
		{name: "empty user", mutate: func(s *container.Spec) {
			s.User = ""
		}, wantErr: container.ErrEmptyUser},
		{name: "root name", mutate: func(s *container.Spec) {
			s.User = "root"
		}, wantErr: container.ErrRootUser},
		{name: "root name with group", mutate: func(s *container.Spec) {
			s.User = "root:root"
		}, wantErr: container.ErrRootUser},
		{name: "uid 0 and gid 0", mutate: func(s *container.Spec) {
			s.User = "0:0"
		}, wantErr: container.ErrRootUser},
		{name: "uid 0 with non-root gid", mutate: func(s *container.Spec) {
			s.User = "0:1000"
		}, wantErr: container.ErrRootUser},
		{name: "user without gid", mutate: func(s *container.Spec) {
			s.User = "1000"
		}, wantAnyErr: true},
		{name: "non-numeric gid", mutate: func(s *container.Spec) {
			s.User = "1000:staff"
		}, wantAnyErr: true},
		{name: "writable root", mutate: func(s *container.Spec) {
			s.ReadOnly = false
		}, wantErr: container.ErrWritableRoot},
		{name: "tmpfs missing", mutate: func(s *container.Spec) {
			s.Tmpfs = nil
		}, wantErr: container.ErrMissingTmpfs},
		{name: "tmpfs without tmp", mutate: func(s *container.Spec) {
			s.Tmpfs = []string{"/run"}
		}, wantErr: container.ErrMissingTmpfs},
		{name: "empty network", mutate: func(s *container.Spec) {
			s.Network = ""
		}, wantErr: container.ErrNoNetwork},
		{name: "git dir as host path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: ".git", Guest: "/work/.git"}}
		}, wantErr: container.ErrGitDirMounted},
		{name: "git dir suffix as host path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo/.git", Guest: "/work/repo"}}
		}, wantErr: container.ErrGitDirMounted},
		{name: "git dir as guest path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo", Guest: "/work/.git"}}
		}, wantErr: container.ErrGitDirMounted},
		{name: "git dir as read-only mount", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo/.git", Guest: "/git", ReadOnly: true}}
		}, wantErr: container.ErrGitDirMounted},
		{name: "git hooks inside host git dir", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo/.git/hooks", Guest: "/hooks"}}
		}, wantErr: container.ErrGitDirMounted},
		{name: "git config inside guest git dir", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo/config", Guest: "/work/.git/config"}}
		}, wantErr: container.ErrGitDirMounted},
		{name: "relative git config path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: ".git/config", Guest: "/work/config"}}
		}, wantErr: container.ErrGitDirMounted},
		{name: "dotgithub directory is allowed", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo/.github", Guest: "/work/.github", ReadOnly: true}}
		}},
		{name: "gitignore file is allowed", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo/.gitignore", Guest: "/work/.gitignore", ReadOnly: true}}
		}},
		{name: "empty host mount path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "", Guest: "/work"}}
		}, wantErr: container.ErrEmptyMountPath},
		{name: "empty guest mount path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo", Guest: ""}}
		}, wantErr: container.ErrEmptyMountPath},
		{name: "comma in host mount path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/work/a,readonly", Guest: "/work"}}
		}, wantErr: container.ErrBadMountPath},
		{name: "comma in guest mount path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo", Guest: "/work/a,readonly"}}
		}, wantErr: container.ErrBadMountPath},
		{name: "equals sign in host mount path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/work/source=/evil", Guest: "/work"}}
		}, wantErr: container.ErrBadMountPath},
		{name: "equals sign in guest mount path", mutate: func(s *container.Spec) {
			s.Mounts = []container.Mount{{Host: "/repo", Guest: "/work=x"}}
		}, wantErr: container.ErrBadMountPath},
		{name: "empty command", mutate: func(s *container.Spec) {
			s.Cmd = nil
		}, wantErr: container.ErrEmptyCommand},
		{name: "command elements with spaces are allowed", mutate: func(s *container.Spec) {
			s.Cmd = []string{"sh", "-c", "echo a b"}
		}},
		{name: "valid multi-network", mutate: func(s *container.Spec) {
			s.Network = ""
			s.Networks = []string{"net1", "net2"}
		}},
		{name: "empty networks slice and empty network", mutate: func(s *container.Spec) {
			s.Network = ""
			s.Networks = []string{}
		}, wantErr: container.ErrNoNetwork},
		{name: "empty network element in networks", mutate: func(s *container.Spec) {
			s.Network = ""
			s.Networks = []string{""}
		}, wantErr: container.ErrNoNetwork},
		{name: "network with comma", mutate: func(s *container.Spec) {
			s.Networks = []string{"net,1"}
		}, wantErr: container.ErrNoNetwork},
		{name: "valid env", mutate: func(s *container.Spec) {
			s.Env = map[string]string{"A": "B", "C": "D"}
		}},
		{name: "empty env key", mutate: func(s *container.Spec) {
			s.Env = map[string]string{"": "val"}
		}, wantErr: container.ErrBadEnv},
		{name: "env key with equals", mutate: func(s *container.Spec) {
			s.Env = map[string]string{"A=B": "val"}
		}, wantErr: container.ErrBadEnv},
		{name: "env with null byte", mutate: func(s *container.Spec) {
			s.Env = map[string]string{"A": "v\x00al"}
		}, wantErr: container.ErrBadEnv},
		{name: "first violation wins", mutate: func(s *container.Spec) {
			s.Image = ""
			s.User = "root"
			s.ReadOnly = false
		}, wantErr: container.ErrEmptyImage},
		{name: "user checked before read-only", mutate: func(s *container.Spec) {
			s.User = "0:0"
			s.ReadOnly = false
		}, wantErr: container.ErrRootUser},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := validSpec()
			tt.mutate(&spec)

			err := spec.Validate()

			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
				}
			case tt.wantAnyErr:
				if err == nil {
					t.Fatalf("Validate() error = nil, want an error")
				}
			default:
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
			}
		})
	}
}
