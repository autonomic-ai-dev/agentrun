package container

import (
	"maps"
	"slices"
)

func RunArgs(spec Spec) ([]string, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	args := []string{"--user", spec.User, "--read-only"}
	for _, path := range spec.Tmpfs {
		args = append(args, "--tmpfs", path)
	}
	if len(spec.Networks) > 0 {
		for _, net := range spec.Networks {
			args = append(args, "--network", net)
		}
	} else if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}
	for _, key := range slices.Sorted(maps.Keys(spec.Labels)) {
		args = append(args, "--label", key+"="+spec.Labels[key])
	}
	if spec.Memory != "" {
		args = append(args, "--memory", spec.Memory)
	}
	for _, key := range slices.Sorted(maps.Keys(spec.Env)) {
		args = append(args, "--env", key+"="+spec.Env[key])
	}
	for _, m := range spec.Mounts {
		args = append(args, "--mount", mountFlag(m))
	}
	if spec.WorkDir != "" {
		args = append(args, "--workdir", spec.WorkDir)
	}
	if spec.Remove {
		args = append(args, "--rm")
	}
	args = append(args, spec.Image)
	args = append(args, spec.Cmd...)
	return args, nil
}

func mountFlag(m Mount) string {
	flag := "type=bind,source=" + m.Host + ",target=" + m.Guest
	if m.ReadOnly {
		flag += ",readonly"
	}
	return flag
}
