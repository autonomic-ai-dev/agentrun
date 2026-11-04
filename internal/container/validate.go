package container

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func (s Spec) Validate() error {
	if err := validateImage(s.Image); err != nil {
		return err
	}
	if err := validateUser(s.User); err != nil {
		return err
	}
	if !s.ReadOnly {
		return fmt.Errorf("read_only is false: %w", ErrWritableRoot)
	}
	if !slices.Contains(s.Tmpfs, "/tmp") {
		return fmt.Errorf("tmpfs %v has no /tmp: %w", s.Tmpfs, ErrMissingTmpfs)
	}
	if s.Network == "" && len(s.Networks) == 0 {
		return fmt.Errorf("network is empty: %w", ErrNoNetwork)
	}
	for _, net := range s.Networks {
		if net == "" {
			return fmt.Errorf("network is empty: %w", ErrNoNetwork)
		}
		if strings.ContainsAny(net, ",=") {
			return fmt.Errorf("network %q contains a comma or equals sign: %w", net, ErrNoNetwork)
		}
	}
	for k, v := range s.Env {
		if k == "" || strings.Contains(k, "=") || strings.Contains(k, "\x00") || strings.Contains(v, "\x00") {
			return fmt.Errorf("env %q=%q is invalid: %w", k, v, ErrBadEnv)
		}
	}
	for _, m := range s.Mounts {
		if isGitDir(m.Host) || isGitDir(m.Guest) {
			return fmt.Errorf("mount %q:%q exposes .git: %w", m.Host, m.Guest, ErrGitDirMounted)
		}
		if m.Host == "" || m.Guest == "" {
			return fmt.Errorf("mount %q:%q has an empty path: %w", m.Host, m.Guest, ErrEmptyMountPath)
		}
		for _, p := range []string{m.Host, m.Guest} {
			if strings.ContainsAny(p, ",=") {
				return fmt.Errorf("mount path %q contains a comma or equals sign: %w", p, ErrBadMountPath)
			}
		}
	}
	if strings.ContainsAny(s.Name, ",=") {
		return fmt.Errorf("name %q contains a comma or equals sign: %w", s.Name, ErrBadName)
	}
	for k, v := range s.Labels {
		for _, p := range []string{k, v} {
			if p == "" || strings.ContainsAny(p, ",=") {
				return fmt.Errorf("label %q=%q is invalid: %w", k, v, ErrBadLabel)
			}
		}
	}
	if len(s.Cmd) == 0 {
		return fmt.Errorf("command is empty: %w", ErrEmptyCommand)
	}
	return nil
}

func validateImage(image string) error {
	if image == "" {
		return fmt.Errorf("image is empty: %w", ErrEmptyImage)
	}
	ref, digest, _ := strings.Cut(image, "@")
	last := ref[strings.LastIndex(ref, "/")+1:]
	_, tag, _ := strings.Cut(last, ":")
	if tag == "latest" {
		return fmt.Errorf("image %q uses the latest tag: %w", image, ErrLatestImage)
	}
	if tag == "" && digest == "" {
		return fmt.Errorf("image %q has no tag or digest: %w", image, ErrLatestImage)
	}
	return nil
}

func validateUser(user string) error {
	if user == "" {
		return fmt.Errorf("user is empty: %w", ErrEmptyUser)
	}
	uid, gid, hasGID := strings.Cut(user, ":")
	if uid == "root" {
		return fmt.Errorf("user %q runs as root: %w", user, ErrRootUser)
	}
	if !hasGID {
		return fmt.Errorf("user %q is not UID:GID", user)
	}
	uidNum, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		return fmt.Errorf("user %q has non-numeric UID", user)
	}
	if uidNum == 0 {
		return fmt.Errorf("user %q runs as UID 0: %w", user, ErrRootUser)
	}
	if _, err := strconv.ParseUint(gid, 10, 64); err != nil {
		return fmt.Errorf("user %q has non-numeric GID", user)
	}
	return nil
}

func isGitDir(path string) bool {
	return slices.Contains(strings.Split(path, "/"), ".git")
}
