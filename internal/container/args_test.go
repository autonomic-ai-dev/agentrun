package container_test

import (
	"errors"
	"slices"
	"testing"

	"agentrun/internal/container"
)

func TestRunArgs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*container.Spec)
		want    []string
		wantErr error
	}{
		{
			name:   "minimal spec",
			mutate: func(*container.Spec) {},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "multiple tmpfs and mounts keep order",
			mutate: func(s *container.Spec) {
				s.Tmpfs = []string{"/tmp", "/run"}
				s.Mounts = []container.Mount{
					{Host: "/work/ws", Guest: "/work"},
					{Host: "/work/ro", Guest: "/ro", ReadOnly: true},
				}
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--tmpfs", "/run",
				"--network", "agentrun-internal",
				"--mount", "type=bind,source=/work/ws,target=/work",
				"--mount", "type=bind,source=/work/ro,target=/ro,readonly",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "command elements with spaces stay single arguments",
			mutate: func(s *container.Spec) {
				s.Cmd = []string{"python", "-c", "print('a b')", "two words"}
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"ghcr.io/example/agent:1.2.3",
				"python", "-c", "print('a b')", "two words",
			},
		},
		{
			name: "invalid spec returns validation error",
			mutate: func(s *container.Spec) {
				s.ReadOnly = false
			},
			wantErr: container.ErrWritableRoot,
		},
		{
			name: "git dir mount is rejected",
			mutate: func(s *container.Spec) {
				s.Mounts = []container.Mount{{Host: "/repo/.git", Guest: "/git"}}
			},
			wantErr: container.ErrGitDirMounted,
		},
		{
			name: "name is emitted after network",
			mutate: func(s *container.Spec) {
				s.Name = "agent-run-1"
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"--name", "agent-run-1",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "labels are sorted by key and emitted as key=value",
			mutate: func(s *container.Spec) {
				s.Labels = map[string]string{"run": "2d0ce0", "agent": "cursor"}
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"--label", "agent=cursor",
				"--label", "run=2d0ce0",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "memory is emitted",
			mutate: func(s *container.Spec) {
				s.Memory = "2g"
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"--memory", "2g",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "name labels and memory precede mounts",
			mutate: func(s *container.Spec) {
				s.Name = "agent-run-1"
				s.Labels = map[string]string{"run": "2d0ce0"}
				s.Memory = "2g"
				s.Mounts = []container.Mount{{Host: "/work/ws", Guest: "/work"}}
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"--name", "agent-run-1",
				"--label", "run=2d0ce0",
				"--memory", "2g",
				"--mount", "type=bind,source=/work/ws,target=/work",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "empty name labels and memory emit nothing",
			mutate: func(s *container.Spec) {
				s.Name = ""
				s.Labels = map[string]string{}
				s.Memory = ""
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "label with empty key is rejected",
			mutate: func(s *container.Spec) {
				s.Labels = map[string]string{"": "v"}
			},
			wantErr: container.ErrBadLabel,
		},
		{
			name: "label with empty value is rejected",
			mutate: func(s *container.Spec) {
				s.Labels = map[string]string{"k": ""}
			},
			wantErr: container.ErrBadLabel,
		},
		{
			name: "label key with equals sign is rejected",
			mutate: func(s *container.Spec) {
				s.Labels = map[string]string{"a=b": "v"}
			},
			wantErr: container.ErrBadLabel,
		},
		{
			name: "label key with comma is rejected",
			mutate: func(s *container.Spec) {
				s.Labels = map[string]string{"a,b": "v"}
			},
			wantErr: container.ErrBadLabel,
		},
		{
			name: "label value with equals sign is rejected",
			mutate: func(s *container.Spec) {
				s.Labels = map[string]string{"k": "a=b"}
			},
			wantErr: container.ErrBadLabel,
		},
		{
			name: "label value with comma is rejected",
			mutate: func(s *container.Spec) {
				s.Labels = map[string]string{"k": "a,b"}
			},
			wantErr: container.ErrBadLabel,
		},
		{
			name: "name with comma is rejected",
			mutate: func(s *container.Spec) {
				s.Name = "a,b"
			},
			wantErr: container.ErrBadName,
		},
		{
			name: "name with equals sign is rejected",
			mutate: func(s *container.Spec) {
				s.Name = "a=b"
			},
			wantErr: container.ErrBadName,
		},
		{
			name: "multiple networks emitted in order",
			mutate: func(s *container.Spec) {
				s.Network = ""
				s.Networks = []string{"default", "agentrun-internal"}
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "default",
				"--network", "agentrun-internal",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "env variables sorted by key",
			mutate: func(s *container.Spec) {
				s.Env = map[string]string{
					"HTTPS_PROXY":    "http://192.168.128.2:3128",
					"GEMINI_API_KEY": "secret-key",
				}
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"--env", "GEMINI_API_KEY=secret-key",
				"--env", "HTTPS_PROXY=http://192.168.128.2:3128",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name: "invalid env key with equals is rejected",
			mutate: func(s *container.Spec) {
				s.Env = map[string]string{"A=B": "val"}
			},
			wantErr: container.ErrBadEnv,
		},
		{
			name: "workdir and remove are emitted before the image",
			mutate: func(s *container.Spec) {
				s.WorkDir = "/work"
				s.Remove = true
			},
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"--workdir", "/work",
				"--rm",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
		{
			name:   "workdir and remove are omitted when unset",
			mutate: func(s *container.Spec) { s.WorkDir = ""; s.Remove = false },
			want: []string{
				"--user", "1000:1000",
				"--read-only",
				"--tmpfs", "/tmp",
				"--network", "agentrun-internal",
				"ghcr.io/example/agent:1.2.3",
				"sh", "-c", "echo hi",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := validSpec()
			tt.mutate(&spec)

			got, err := container.RunArgs(spec)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("RunArgs() error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("RunArgs() args = %q, want nil on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RunArgs() error = %v, want nil", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("RunArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}
